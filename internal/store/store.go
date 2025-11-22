package store

import (
	"database/sql"
	"encoding/binary"
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
					"./libvector",
					"libvector",
				}

				for _, path := range extensionPaths {
					err := conn.LoadExtension(path, "sqlite3_vector_init")
					if err == nil {
						fmt.Printf("Successfully loaded sqlite-vector extension\n")
						return nil
					}
				}
				// Return nil even if extension loading fails - we'll fall back to Go-based similarity
				return nil
			},
		})
}

type Chunk struct {
	ID         int64
	Path       string
	ModifiedAt int64
	Language   string
	StartLine  int
	EndLine    int
	Code       string
	Embedding  []float64
	Similarity float64 // Used for search results
}

type Store struct {
	db           *sql.DB
	vectorLoaded bool
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
	db, err := sql.Open("sqlite3_with_extensions", dbPath+"?_foreign_keys=1")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Check if vector extension was loaded in init()
	// Test by checking if vector functions are available
	var vectorLoaded bool
	var testResult int
	err = db.QueryRow("SELECT 1").Scan(&testResult)
	if err == nil {
		// Extension is loaded based on init() output
		// We'll verify it works when we try to use vector_init later
		vectorLoaded = true
	} else {
		vectorLoaded = false
		fmt.Println("Warning: Database connection test failed")
	}

	store := &Store{db: db, vectorLoaded: vectorLoaded}

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
		embedding BLOB
	);

	CREATE INDEX IF NOT EXISTS chunks_path_idx ON chunks(path);
	CREATE INDEX IF NOT EXISTS chunks_modifiedAt_idx ON chunks(modifiedAt);

	CREATE TABLE IF NOT EXISTS clusters (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		canonical_chunk_id INTEGER,
		avg_similarity REAL,
		max_similarity REAL,
		size INTEGER,
		FOREIGN KEY (canonical_chunk_id) REFERENCES chunks(id)
	);

	CREATE TABLE IF NOT EXISTS cluster_chunks (
		cluster_id INTEGER NOT NULL,
		chunk_id INTEGER NOT NULL,
		similarity REAL NOT NULL,
		PRIMARY KEY (cluster_id, chunk_id),
		FOREIGN KEY (cluster_id) REFERENCES clusters(id),
		FOREIGN KEY (chunk_id) REFERENCES chunks(id)
	);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Initialize vector search if sqlite-vector is loaded
	if s.vectorLoaded {
		_, err := s.db.Exec("SELECT vector_init('chunks', 'embedding', 'type=FLOAT32,dimension=1536,distance=COSINE')")
		if err != nil {
			fmt.Printf("Warning: vector_init failed: %v\n", err)
			s.vectorLoaded = false
		}
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
func (s *Store) DeleteChunksForFile(path string) error {
	_, err := s.db.Exec("DELETE FROM chunks WHERE path = ?", path)
	if err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
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

	// Serialize embedding to BLOB
	var embeddingBlob []byte
	if len(chunk.Embedding) > 0 {
		embeddingBlob = serializeVector(chunk.Embedding)
	}

	// Insert into chunks table with embedding
	result, err := s.db.Exec(
		"INSERT INTO chunks (path, modifiedAt, language, start_line, end_line, code, embedding) VALUES (?, ?, ?, ?, ?, ?, ?)",
		chunk.Path, chunk.ModifiedAt, chunk.Language, chunk.StartLine, chunk.EndLine, chunk.Code, embeddingBlob,
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
		SELECT id, path, language, start_line, end_line, code, embedding
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
		err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingBytes)
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
		SELECT id, path, language, start_line, end_line, code, embedding
		FROM chunks
		WHERE id = ?
	`, id).Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingBytes)

	if err != nil {
		return nil, fmt.Errorf("failed to get chunk: %w", err)
	}

	if len(embeddingBytes) > 0 {
		chunk.Embedding = deserializeVector(embeddingBytes)
	}

	return chunk, nil
}

// CosineSimilarity calculates the cosine similarity between two vectors
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	// Simple sqrt using the math package would be better, but this avoids import
	if x == 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// FindSimilarChunks finds chunks similar to the given chunk
func (s *Store) FindSimilarChunks(chunkID int64, limit int) ([]*Chunk, error) {
	// Get the target chunk
	targetChunk, err := s.GetChunkByID(chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target chunk: %w", err)
	}

	// Try using sqlite-vector's vector_quantize_scan if available
	if s.vectorLoaded {
		embeddingBlob := serializeVector(targetChunk.Embedding)
		rows, err := s.db.Query(`
			SELECT c.id, c.path, c.language, c.start_line, c.end_line, c.code, c.embedding, v.distance
			FROM chunks AS c
			JOIN vector_quantize_scan('chunks', 'embedding', ?, ?) AS v
			ON c.rowid = v.rowid
			WHERE c.id != ?
		`, embeddingBlob, limit, chunkID)

		if err == nil {
			// sqlite-vector is available, use optimized search
			defer rows.Close()
			var results []*Chunk
			for rows.Next() {
				chunk := &Chunk{}
				var embeddingBytes []byte
				var distance float64
				err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingBytes, &distance)
				if err != nil {
					return nil, fmt.Errorf("failed to scan chunk: %w", err)
				}

				if len(embeddingBytes) > 0 {
					chunk.Embedding = deserializeVector(embeddingBytes)
				}

				// Convert distance to similarity
				// For cosine distance: similarity = 1 - distance
				chunk.Similarity = 1.0 - distance

				results = append(results, chunk)
			}
			return results, rows.Err()
		}
		// If vector_distance failed, fall through to Go-based similarity
	}

	// Fall back to Go-based similarity search
	rows, err := s.db.Query(`
		SELECT id, path, language, start_line, end_line, code, embedding
		FROM chunks
		WHERE id != ?
	`, chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
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
		err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		if len(embeddingBytes) > 0 {
			chunk.Embedding = deserializeVector(embeddingBytes)
		}

		// Calculate similarity
		similarity := CosineSimilarity(targetChunk.Embedding, chunk.Embedding)
		chunk.Similarity = similarity

		candidates = append(candidates, chunkWithSim{chunk: chunk, similarity: similarity})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by similarity descending
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].similarity > candidates[i].similarity {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Return top N
	var results []*Chunk
	for i := 0; i < limit && i < len(candidates); i++ {
		results = append(results, candidates[i].chunk)
	}

	return results, nil
}

// SearchSimilar searches for chunks similar to a given embedding
func (s *Store) SearchSimilar(embedding []float64, limit int, threshold float64) ([]*Chunk, error) {
	embeddingBlob := serializeVector(embedding)

	// Try using sqlite-vector if available
	if s.vectorLoaded {
		rows, err := s.db.Query(`
			SELECT c.id, c.path, c.language, c.start_line, c.end_line, c.code, c.embedding, v.distance
			FROM chunks AS c
			JOIN vector_quantize_scan('chunks', 'embedding', ?, ?) AS v
			ON c.rowid = v.rowid
		`, embeddingBlob, limit*2) // Request more to filter by threshold

		if err == nil {
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
				err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingBytes, &distance)
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
		// If vector_distance failed, fall through to Go-based similarity
	}

	// Fall back to Go-based similarity search
	rows, err := s.db.Query(`SELECT id, path, language, start_line, end_line, code, embedding FROM chunks`)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
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
		err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		if len(embeddingBytes) > 0 {
			chunk.Embedding = deserializeVector(embeddingBytes)
		}

		// Calculate similarity
		similarity := CosineSimilarity(embedding, chunk.Embedding)
		if similarity >= threshold {
			chunk.Similarity = similarity
			candidates = append(candidates, chunkWithSim{chunk: chunk, similarity: similarity})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by similarity descending
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].similarity > candidates[i].similarity {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Return top N
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
	if !s.vectorLoaded {
		return nil // Skip if extension not loaded
	}

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
