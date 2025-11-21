package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

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
	db *sql.DB
}

// NewStore creates a new store and initializes the database
func NewStore(dbPath string, reset bool) (*Store, error) {
	// Remove database if reset is true
	if reset {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove database: %w", err)
		}
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	store := &Store{db: db}

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
		embedding TEXT NOT NULL
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

// InsertChunk inserts a chunk into the database
func (s *Store) InsertChunk(chunk *Chunk) error {
	// Get file modification time
	info, err := os.Stat(chunk.Path)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	chunk.ModifiedAt = info.ModTime().Unix()

	// Serialize embedding as JSON
	embeddingJSON, err := json.Marshal(chunk.Embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	result, err := s.db.Exec(
		"INSERT INTO chunks (path, modifiedAt, language, start_line, end_line, code, embedding) VALUES (?, ?, ?, ?, ?, ?, ?)",
		chunk.Path, chunk.ModifiedAt, chunk.Language, chunk.StartLine, chunk.EndLine, chunk.Code, string(embeddingJSON),
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

// GetAllChunks returns all chunks ordered by code length descending
func (s *Store) GetAllChunks() ([]*Chunk, error) {
	rows, err := s.db.Query("SELECT id, path, language, start_line, end_line, code, embedding FROM chunks ORDER BY LENGTH(code) DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var chunks []*Chunk
	for rows.Next() {
		chunk := &Chunk{}
		var embeddingJSON string
		err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		if err := json.Unmarshal([]byte(embeddingJSON), &chunk.Embedding); err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedding: %w", err)
		}

		chunks = append(chunks, chunk)
	}

	return chunks, rows.Err()
}

// GetChunkByID retrieves a chunk by its ID
func (s *Store) GetChunkByID(id int64) (*Chunk, error) {
	chunk := &Chunk{}
	var embeddingJSON string
	err := s.db.QueryRow(
		"SELECT id, path, language, start_line, end_line, code, embedding FROM chunks WHERE id = ?",
		id,
	).Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingJSON)

	if err != nil {
		return nil, fmt.Errorf("failed to get chunk: %w", err)
	}

	if err := json.Unmarshal([]byte(embeddingJSON), &chunk.Embedding); err != nil {
		return nil, fmt.Errorf("failed to unmarshal embedding: %w", err)
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

	// Get all other chunks
	rows, err := s.db.Query(
		"SELECT id, path, language, start_line, end_line, code, embedding FROM chunks WHERE id != ?",
		chunkID,
	)
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
		var embeddingJSON string
		err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		if err := json.Unmarshal([]byte(embeddingJSON), &chunk.Embedding); err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedding: %w", err)
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
	// Get all chunks
	rows, err := s.db.Query("SELECT id, path, language, start_line, end_line, code, embedding FROM chunks")
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
		var embeddingJSON string
		err := rows.Scan(&chunk.ID, &chunk.Path, &chunk.Language, &chunk.StartLine, &chunk.EndLine, &chunk.Code, &embeddingJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		if err := json.Unmarshal([]byte(embeddingJSON), &chunk.Embedding); err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedding: %w", err)
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
