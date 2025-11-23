package store

import (
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/mattn/go-sqlite3"
)

func init() {
	// Register a custom driver that loads the vector extension via ConnectHook
	sql.Register("sqlite3_with_extensions",
		&sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				// Try to load the extension
				// Note: SQLite adds platform extension (.dylib/.so) automatically
				extensionPaths := []string{
					"./libvector",                      // Current directory
					"libvector",                        // System library paths
					"/opt/homebrew/lib/libvector",      // Homebrew Apple Silicon
					"/usr/local/lib/libvector",         // Homebrew Intel / Linux
					"/usr/lib/libvector",               // Standard Linux path
				}

				var lastErr error
				for _, path := range extensionPaths {
					err := conn.LoadExtension(path, "sqlite3_vector_init")
					if err == nil {
						// Successfully loaded extension
						return nil
					}
					lastErr = err
				}
				// Fail if extension can't be loaded
				return fmt.Errorf("failed to load sqlite-vector extension (tried: %v): %w", extensionPaths, lastErr)
			},
		})
}

const (
	// EmbeddingDimension is the dimension of embedding vectors (OpenAI text-embedding-3-small)
	EmbeddingDimension = 1536
)

type Chunk struct {
	ID         int64
	Path       string
	ModifiedAt int64
	Language   string
	StartLine  int
	EndLine    int
	Code       string
	SHA        string    // Git-style SHA of the chunk content
	Embedding  []float64
	Similarity float64 // Used for search results
}

type Store struct {
	db           *sql.DB
	vectorLoaded bool
}

// ComputeChunkSHA computes a git-style SHA-1 hash of the chunk content
func ComputeChunkSHA(code string) string {
	h := sha1.New()
	h.Write([]byte(code))
	return hex.EncodeToString(h.Sum(nil))
}

// ShortSHA returns the first 8 characters of a SHA (like git)
func ShortSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

// NewStore creates a new store and initializes the database
func NewStore(dbPath string, reset bool) (*Store, error) {
	// Remove database if reset is true
	if reset {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove database: %w", err)
		}
	}

	// Open database with extension loading enabled
	db, err := sql.Open("sqlite3_with_extensions", dbPath+"?_foreign_keys=1&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Extension loading happens in ConnectHook - if we got here, it succeeded
	store := &Store{db: db, vectorLoaded: true}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

func (s *Store) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS chunks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT NOT NULL,
		modifiedAt INTEGER NOT NULL,
		language TEXT NOT NULL,
		start_line INTEGER NOT NULL,
		end_line INTEGER NOT NULL,
		code TEXT NOT NULL,
		sha TEXT NOT NULL,
		embedding BLOB
	);

	CREATE INDEX IF NOT EXISTS chunks_path_idx ON chunks(path);
	CREATE INDEX IF NOT EXISTS chunks_modifiedAt_idx ON chunks(modifiedAt);
	CREATE INDEX IF NOT EXISTS chunks_sha_idx ON chunks(sha);

	CREATE TABLE IF NOT EXISTS clusters (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		canonical_chunk_id INTEGER,
		avg_similarity REAL,
		max_similarity REAL,
		size INTEGER,
		FOREIGN KEY (canonical_chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS cluster_chunks (
		cluster_id INTEGER NOT NULL,
		chunk_id INTEGER NOT NULL,
		similarity REAL NOT NULL,
		PRIMARY KEY (cluster_id, chunk_id),
		FOREIGN KEY (cluster_id) REFERENCES clusters(id) ON DELETE CASCADE,
		FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS vocabulary (
		term TEXT PRIMARY KEY,
		term_index INTEGER NOT NULL,
		idf REAL NOT NULL
	);

	CREATE TABLE IF NOT EXISTS vocabulary_metadata (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		vocabulary_frozen_at TEXT,
		total_chunks_at_freeze INTEGER,
		last_refreeze_reason TEXT
	);

	CREATE TABLE IF NOT EXISTS ignores (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sha TEXT NOT NULL UNIQUE,
		code TEXT NOT NULL,
		embedding BLOB NOT NULL,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS ignores_sha_idx ON ignores(sha);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Initialize vector search with sqlite-vector
	query := fmt.Sprintf("SELECT vector_init('chunks', 'embedding', 'type=FLOAT32,dimension=%d,distance=COSINE')", EmbeddingDimension)
	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to initialize vector search: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// NeedsUpdate checks if a file needs to be re-analyzed
func (s *Store) NeedsUpdate(path string) (bool, error) {
	// Get file modification time
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("failed to stat file: %w", err)
	}
	modTime := info.ModTime().Unix()

	// Check if we have chunks for this file
	var storedModTime int64
	err = s.db.QueryRow("SELECT modifiedAt FROM chunks WHERE path = ? LIMIT 1", path).Scan(&storedModTime)
	if err == sql.ErrNoRows {
		// No chunks for this file, needs update
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to query chunks: %w", err)
	}

	// Compare modification times
	return modTime > storedModTime, nil
}

// DeleteChunksForFile deletes all chunks for a given file
// Cluster memberships and clusters are automatically deleted via ON DELETE CASCADE
func (s *Store) DeleteChunksForFile(path string) error {
	_, err := s.db.Exec("DELETE FROM chunks WHERE path = ?", path)
	if err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
	}
	return nil
}

// DeleteAllChunks deletes all chunks from the database
// Cluster memberships and clusters are automatically deleted via ON DELETE CASCADE
func (s *Store) DeleteAllChunks() error {
	_, err := s.db.Exec("DELETE FROM chunks")
	if err != nil {
		return fmt.Errorf("failed to delete all chunks: %w", err)
	}
	return nil
}

// serializeVector converts float64 slice to bytes for sqlite-vec
func serializeVector(vec []float64) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(float32(v)))
	}
	return buf
}

// InsertChunk inserts a chunk into the database
func (s *Store) InsertChunk(chunk *Chunk) error {
	// Get file modification time
	info, err := os.Stat(chunk.Path)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	chunk.ModifiedAt = info.ModTime().Unix()

	// Compute SHA if not already set
	if chunk.SHA == "" {
		chunk.SHA = ComputeChunkSHA(chunk.Code)
	}

	// Serialize embedding to BLOB
	var embeddingBlob []byte
	if len(chunk.Embedding) > 0 {
		embeddingBlob = serializeVector(chunk.Embedding)
	}

	// Insert into chunks table with embedding
	result, err := s.db.Exec(
		"INSERT INTO chunks (path, modifiedAt, language, start_line, end_line, code, sha, embedding) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		chunk.Path, chunk.ModifiedAt, chunk.Language, chunk.StartLine, chunk.EndLine, chunk.Code, chunk.SHA, embeddingBlob,
	)
	if err != nil {
		return fmt.Errorf("failed to insert chunk: %w", err)
	}

	chunk.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	return nil
}

// deserializeVector converts bytes from sqlite-vec to float64 slice
func deserializeVector(data []byte) []float64 {
	vec := make([]float64, len(data)/4)
	for i := range vec {
		bits := binary.LittleEndian.Uint32(data[i*4:])
		vec[i] = float64(math.Float32frombits(bits))
	}
	return vec
}

// GetAllChunks returns all chunks ordered by code length descending
func (s *Store) GetAllChunks() ([]*Chunk, error) {
	rows, err := s.db.Query(`
		SELECT id, path, language, start_line, end_line, code, sha, embedding
		FROM chunks
		ORDER BY LENGTH(code) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var chunks []*Chunk
	for rows.Next() {
		chunk := &Chunk{}
		var embeddingBytes []byte
		err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &chunk.SHA, &embeddingBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		if len(embeddingBytes) > 0 {
			chunk.Embedding = deserializeVector(embeddingBytes)
		}

		chunks = append(chunks, chunk)
	}

	return chunks, rows.Err()
}

// GetChunkByID retrieves a chunk by its ID
func (s *Store) GetChunkByID(id int64) (*Chunk, error) {
	chunk := &Chunk{}
	var embeddingBytes []byte
	err := s.db.QueryRow(`
		SELECT id, path, language, start_line, end_line, code, sha, embedding
		FROM chunks
		WHERE id = ?
	`, id).Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &chunk.SHA, &embeddingBytes)

	if err != nil {
		return nil, fmt.Errorf("failed to get chunk: %w", err)
	}

	if len(embeddingBytes) > 0 {
		chunk.Embedding = deserializeVector(embeddingBytes)
	}

	return chunk, nil
}

// IsSubset returns true if chunk a is a subset of chunk b or vice versa
// A chunk is a subset if it's from the same file and its line range is entirely contained
func IsSubset(a, b *Chunk) bool {
	// Must be from the same file
	if a.Path != b.Path {
		return false
	}

	// Check if a is subset of b (or equal)
	aInB := a.StartLine >= b.StartLine && a.EndLine <= b.EndLine
	// Check if b is subset of a (or equal)
	bInA := b.StartLine >= a.StartLine && b.EndLine <= a.EndLine

	// If they're equal, not a subset
	if aInB && bInA {
		return false
	}

	// If either is contained in the other, it's a subset relationship
	return aInB || bInA
}

// FindSimilarChunks finds chunks similar to the given chunk
func (s *Store) FindSimilarChunks(chunkID int64, limit int) ([]*Chunk, error) {
	// Get the target chunk
	targetChunk, err := s.GetChunkByID(chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target chunk: %w", err)
	}

	// Use sqlite-vector's vector_quantize_scan
	embeddingBlob := serializeVector(targetChunk.Embedding)
	rows, err := s.db.Query(`
		SELECT c.id, c.path, c.language, c.start_line, c.end_line, c.code, c.sha, c.embedding, v.distance
		FROM chunks AS c
		JOIN vector_quantize_scan('chunks', 'embedding', ?, ?) AS v
		ON c.rowid = v.rowid
		WHERE c.id != ?
	`, embeddingBlob, limit, chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar chunks: %w", err)
	}
	defer rows.Close()

	var results []*Chunk
	for rows.Next() {
		chunk := &Chunk{}
		var embeddingBytes []byte
		var distance float64
		err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &chunk.SHA, &embeddingBytes, &distance)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		if len(embeddingBytes) > 0 {
			chunk.Embedding = deserializeVector(embeddingBytes)
		}

		// Skip chunks that are subsets of the target chunk or vice versa
		if IsSubset(chunk, targetChunk) {
			continue
		}

		// Convert distance to similarity
		// For cosine distance: similarity = 1 - distance
		chunk.Similarity = 1.0 - distance

		results = append(results, chunk)
	}

	return results, rows.Err()
}

// SearchSimilar searches for chunks similar to a given embedding
func (s *Store) SearchSimilar(embedding []float64, limit int, threshold float64) ([]*Chunk, error) {
	embeddingBlob := serializeVector(embedding)

	// Use sqlite-vector for similarity search
	rows, err := s.db.Query(`
		SELECT c.id, c.path, c.language, c.start_line, c.end_line, c.code, c.sha, c.embedding, v.distance
		FROM chunks AS c
		JOIN vector_quantize_scan('chunks', 'embedding', ?, ?) AS v
		ON c.rowid = v.rowid
	`, embeddingBlob, limit*2) // Request more to filter by threshold
	if err != nil {
		return nil, fmt.Errorf("failed to search chunks: %w", err)
	}
	defer rows.Close()

	type chunkWithSim struct {
		chunk      *Chunk
		similarity float64
	}

	var candidates []chunkWithSim
	for rows.Next() {
		chunk := &Chunk{}
		var embeddingBytes []byte
		var distance float64
		err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &chunk.SHA, &embeddingBytes, &distance)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		if len(embeddingBytes) > 0 {
			chunk.Embedding = deserializeVector(embeddingBytes)
		}

		// Convert distance to similarity
		similarity := 1.0 - distance
		if similarity >= threshold {
			chunk.Similarity = similarity
			candidates = append(candidates, chunkWithSim{chunk: chunk, similarity: similarity})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Already sorted by distance, just need to return top N
	var results []*Chunk
	for i := 0; i < limit && i < len(candidates); i++ {
		results = append(results, candidates[i].chunk)
	}

	return results, nil
}

// CreateCluster creates a new cluster
func (s *Store) CreateCluster(canonicalChunkID int64, avgSimilarity, maxSimilarity float64, size int) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO clusters (canonical_chunk_id, avg_similarity, max_similarity, size, created_at) VALUES (?, ?, ?, ?, ?)",
		canonicalChunkID, avgSimilarity, maxSimilarity, size, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create cluster: %w", err)
	}

	return result.LastInsertId()
}

// AddChunkToCluster adds a chunk to a cluster
func (s *Store) AddChunkToCluster(clusterID, chunkID int64, similarity float64) error {
	_, err := s.db.Exec(
		"INSERT INTO cluster_chunks (cluster_id, chunk_id, similarity) VALUES (?, ?, ?)",
		clusterID, chunkID, similarity,
	)
	if err != nil {
		return fmt.Errorf("failed to add chunk to cluster: %w", err)
	}
	return nil
}

// DeleteAllClusters deletes all clusters
func (s *Store) DeleteAllClusters() error {
	if _, err := s.db.Exec("DELETE FROM cluster_chunks"); err != nil {
		return fmt.Errorf("failed to delete cluster chunks: %w", err)
	}
	if _, err := s.db.Exec("DELETE FROM clusters"); err != nil {
		return fmt.Errorf("failed to delete clusters: %w", err)
	}
	return nil
}

// IsChunkInCluster checks if a chunk is already in any cluster
func (s *Store) IsChunkInCluster(chunkID int64) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM cluster_chunks WHERE chunk_id = ?", chunkID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if chunk is in cluster: %w", err)
	}
	return count > 0, nil
}

// QuantizeVectors prepares vector data for fast searching
func (s *Store) QuantizeVectors() error {
	_, err := s.db.Exec("SELECT vector_quantize('chunks', 'embedding')")
	if err != nil {
		return fmt.Errorf("failed to quantize vectors: %w", err)
	}

	// Optionally preload quantized data into memory for faster searches
	_, err = s.db.Exec("SELECT vector_quantize_preload('chunks', 'embedding')")
	if err != nil {
		// Preload is optional, just log warning
		fmt.Printf("Warning: Failed to preload quantized vectors: %v\n", err)
	}

	return nil
}

// SaveVocabulary saves the TF-IDF vocabulary to the database
func (s *Store) SaveVocabulary(vocab map[string]int, idf []float64) error {
	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing vocabulary
	if _, err := tx.Exec("DELETE FROM vocabulary"); err != nil {
		return fmt.Errorf("failed to clear vocabulary: %w", err)
	}

	// Insert vocabulary terms
	stmt, err := tx.Prepare("INSERT INTO vocabulary (term, term_index, idf) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for term, index := range vocab {
		if index >= len(idf) {
			continue // Skip invalid indices
		}
		if _, err := stmt.Exec(term, index, idf[index]); err != nil {
			return fmt.Errorf("failed to insert term %s: %w", term, err)
		}
	}

	return tx.Commit()
}

// LoadVocabulary loads the TF-IDF vocabulary from the database
func (s *Store) LoadVocabulary() (map[string]int, []float64, error) {
	rows, err := s.db.Query("SELECT term, term_index, idf FROM vocabulary ORDER BY term_index")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query vocabulary: %w", err)
	}
	defer rows.Close()

	vocab := make(map[string]int)
	var idfList []float64
	maxIndex := -1

	// First pass: find max index
	type vocabEntry struct {
		term  string
		index int
		idf   float64
	}
	var entries []vocabEntry

	for rows.Next() {
		var term string
		var index int
		var idfVal float64
		if err := rows.Scan(&term, &index, &idfVal); err != nil {
			return nil, nil, fmt.Errorf("failed to scan row: %w", err)
		}
		entries = append(entries, vocabEntry{term, index, idfVal})
		if index > maxIndex {
			maxIndex = index
		}
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	if len(entries) == 0 {
		return nil, nil, fmt.Errorf("no vocabulary found")
	}

	// Build IDF array with correct size
	idfList = make([]float64, maxIndex+1)
	for _, entry := range entries {
		vocab[entry.term] = entry.index
		idfList[entry.index] = entry.idf
	}

	return vocab, idfList, nil
}

// HasVocabulary checks if a vocabulary exists in the database
func (s *Store) HasVocabulary() (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM vocabulary").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check vocabulary: %w", err)
	}
	return count > 0, nil
}

// GetChunkCount returns the total number of chunks in the database
func (s *Store) GetChunkCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count chunks: %w", err)
	}
	return count, nil
}

// GetFreezeMetadata returns the vocabulary freeze metadata
func (s *Store) GetFreezeMetadata() (frozenAt time.Time, chunksAtFreeze int, reason string, err error) {
	var frozenAtStr string
	err = s.db.QueryRow("SELECT vocabulary_frozen_at, total_chunks_at_freeze, last_refreeze_reason FROM vocabulary_metadata WHERE id = 1").
		Scan(&frozenAtStr, &chunksAtFreeze, &reason)

	if err == sql.ErrNoRows {
		return time.Time{}, 0, "", nil
	}
	if err != nil {
		return time.Time{}, 0, "", fmt.Errorf("failed to get freeze metadata: %w", err)
	}

	frozenAt, err = time.Parse(time.RFC3339, frozenAtStr)
	if err != nil {
		return time.Time{}, 0, "", fmt.Errorf("failed to parse frozen_at time: %w", err)
	}

	return frozenAt, chunksAtFreeze, reason, nil
}

// SetFreezeMetadata updates the vocabulary freeze metadata
func (s *Store) SetFreezeMetadata(chunksAtFreeze int, reason string) error {
	frozenAt := time.Now().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO vocabulary_metadata (id, vocabulary_frozen_at, total_chunks_at_freeze, last_refreeze_reason)
		VALUES (1, ?, ?, ?)
	`, frozenAt, chunksAtFreeze, reason)

	if err != nil {
		return fmt.Errorf("failed to set freeze metadata: %w", err)
	}
	return nil
}

// ShouldRefreeze determines if vocabulary should be refrozen based on corpus growth
func (s *Store) ShouldRefreeze(threshold float64) (bool, float64, error) {
	currentCount, err := s.GetChunkCount()
	if err != nil {
		return false, 0, err
	}

	_, chunksAtFreeze, _, err := s.GetFreezeMetadata()
	if err != nil {
		return false, 0, err
	}

	// If no freeze metadata, should freeze
	if chunksAtFreeze == 0 {
		return true, 0, nil
	}

	// Calculate new chunks ratio
	newChunks := currentCount - chunksAtFreeze
	if newChunks <= 0 {
		return false, 0, nil
	}

	newChunkRatio := float64(newChunks) / float64(currentCount)
	return newChunkRatio > threshold, newChunkRatio, nil
}

// AddIgnore adds a chunk to the ignore list
func (s *Store) AddIgnore(sha, code string, embedding []float64) error {
	embeddingBlob := serializeVector(embedding)
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO ignores (sha, code, embedding, created_at) VALUES (?, ?, ?, ?)",
		sha, code, embeddingBlob, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to add ignore: %w", err)
	}
	return nil
}

// IsIgnoredBySHA checks if a SHA is in the ignore list
func (s *Store) IsIgnoredBySHA(sha string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM ignores WHERE sha = ?", sha).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check ignore: %w", err)
	}
	return count > 0, nil
}

// GetAllIgnores returns all ignored chunks
func (s *Store) GetAllIgnores() ([]*Chunk, error) {
	rows, err := s.db.Query(`
		SELECT sha, code, embedding
		FROM ignores
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query ignores: %w", err)
	}
	defer rows.Close()

	var chunks []*Chunk
	for rows.Next() {
		chunk := &Chunk{}
		var embeddingBytes []byte
		err := rows.Scan(&chunk.SHA, &chunk.Code, &embeddingBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ignore: %w", err)
		}

		if len(embeddingBytes) > 0 {
			chunk.Embedding = deserializeVector(embeddingBytes)
		}

		chunks = append(chunks, chunk)
	}

	return chunks, rows.Err()
}

// RemoveIgnore removes a chunk from the ignore list by SHA
func (s *Store) RemoveIgnore(sha string) error {
	_, err := s.db.Exec("DELETE FROM ignores WHERE sha = ?", sha)
	if err != nil {
		return fmt.Errorf("failed to remove ignore: %w", err)
	}
	return nil
}

// CosineSimilarity computes the cosine similarity between two embedding vectors
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// IsIgnoredBySimilarity checks if a chunk is ignored by SHA or cosine similarity
// Phase 2: Uses similarity threshold to handle minor code changes
func (s *Store) IsIgnoredBySimilarity(chunk *Chunk, threshold float64) (bool, error) {
	// Phase 1: Check exact SHA match
	exactMatch, err := s.IsIgnoredBySHA(chunk.SHA)
	if err != nil {
		return false, err
	}
	if exactMatch {
		return true, nil
	}

	// Phase 2: Check similarity with all ignored chunks
	ignoredChunks, err := s.GetAllIgnores()
	if err != nil {
		return false, fmt.Errorf("failed to get ignored chunks: %w", err)
	}

	// If no embedding, can't do similarity check
	if len(chunk.Embedding) == 0 {
		return false, nil
	}

	for _, ignored := range ignoredChunks {
		if len(ignored.Embedding) == 0 {
			continue
		}

		similarity := CosineSimilarity(chunk.Embedding, ignored.Embedding)
		if similarity >= threshold {
			return true, nil
		}
	}

	return false, nil
}
