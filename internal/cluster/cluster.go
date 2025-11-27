package cluster

import (
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/ZachSnow/redunce/internal/store"
	"github.com/ZachSnow/redunce/internal/util"
)

const (
	// DefaultSimilarChunksLimit is the default maximum number of similar chunks to find per chunk
	DefaultSimilarChunksLimit = 10
)

// Cluster represents a group of similar chunks
type Cluster struct {
	ID             int64
	ClusterID      string // SHA of the canonical chunk
	CanonicalChunk *store.Chunk
	Chunks         []*store.Chunk
	AvgSimilarity  float64
	MaxSimilarity  float64
	Score          float64 // Computed score for sorting
}

// ScoreStrategy defines how clusters are scored for sorting
type ScoreStrategy string

const (
	// ScoreOld scores by canonical chunk length (original behavior)
	ScoreOld ScoreStrategy = "old"
	// ScoreImpact scores by total lines across all chunks in cluster
	ScoreImpact ScoreStrategy = "impact"
	// ScoreDefault scores by a weighted combination of metrics
	ScoreDefault ScoreStrategy = "default"
)

// TotalLines returns the sum of lines across all chunks in the cluster
func (c *Cluster) TotalLines() int {
	total := 0
	for _, chunk := range c.Chunks {
		total += chunk.EndLine - chunk.StartLine + 1
	}
	return total
}

// CanonicalLines returns the number of lines in the canonical chunk
func (c *Cluster) CanonicalLines() int {
	return c.CanonicalChunk.EndLine - c.CanonicalChunk.StartLine + 1
}

// ComputeScore calculates the cluster's score based on the given strategy
func (c *Cluster) ComputeScore(strategy ScoreStrategy) float64 {
	switch strategy {
	case ScoreOld:
		// Original: just canonical chunk length (in characters, for backwards compat)
		return float64(len(c.CanonicalChunk.Code))

	case ScoreImpact:
		// Focus on total duplicated lines
		return float64(c.TotalLines())

	case ScoreDefault:
		// Weighted combination:
		// - Total lines (impact of duplication)
		// - Number of chunks (spread of duplication)
		// - Average similarity (confidence it's real duplication)
		totalLines := float64(c.TotalLines())
		chunkCount := float64(len(c.Chunks))
		avgSim := c.AvgSimilarity

		// Formula: totalLines * avgSim * (1 + log2(chunkCount))
		// - More lines = higher score
		// - Higher similarity = higher score
		// - More chunks = higher score (with diminishing returns via log)
		logFactor := 1.0
		if chunkCount > 1 {
			logFactor = 1.0 + (math.Log2(chunkCount))
		}
		return totalLines * avgSim * logFactor

	default:
		// Unknown strategy, use default
		return c.ComputeScore(ScoreDefault)
	}
}

// ClusterNewChunks performs incremental clustering for newly added chunks.
// For each new chunk, finds similar chunks and either:
// - Adds the new chunk to an existing cluster (if any similar chunk is clustered), or
// - Creates a new cluster and adds all similar unclustered chunks to it
// Returns the number of chunks that were clustered.
func ClusterNewChunks(db *store.Store, newChunkIDs []int64, threshold float64, ignoreThreshold float64, verbose bool) (int, error) {
	if len(newChunkIDs) == 0 {
		return 0, nil
	}

	// Quantize vectors for fast searching
	if err := db.QuantizeVectors(); err != nil {
		return 0, fmt.Errorf("failed to quantize vectors: %w", err)
	}

	clustered := 0
	total := len(newChunkIDs)
	progressInterval := util.GetProgressInterval(total)

	for i, chunkID := range newChunkIDs {
		if verbose && (i%progressInterval == 0 || i == total-1) {
			fmt.Fprintf(os.Stderr, "  Clustering new chunks: %d/%d\n", i+1, total)
		}

		// Get the new chunk
		newChunk, err := db.GetChunkByID(chunkID)
		if err != nil {
			return clustered, fmt.Errorf("failed to get chunk %d: %w", chunkID, err)
		}

		// Check if this chunk is ignored
		ignored, err := db.IsIgnoredBySimilarity(newChunk, ignoreThreshold)
		if err != nil {
			return clustered, fmt.Errorf("failed to check if chunk is ignored: %w", err)
		}
		if ignored {
			continue
		}

		// Check if already in a cluster (could happen if matched by earlier new chunk)
		inCluster, err := db.IsChunkInCluster(chunkID)
		if err != nil {
			return clustered, fmt.Errorf("failed to check cluster membership: %w", err)
		}
		if inCluster {
			continue
		}

		// Find top N similar chunks
		similarChunks, err := db.FindSimilarChunks(chunkID, DefaultSimilarChunksLimit)
		if err != nil {
			return clustered, fmt.Errorf("failed to find similar chunks: %w", err)
		}

		// Filter by threshold and ignored status
		var validSimilar []*store.Chunk
		for _, similar := range similarChunks {
			if similar.Similarity < threshold {
				continue
			}
			similarIgnored, err := db.IsIgnoredBySimilarity(similar, ignoreThreshold)
			if err != nil {
				return clustered, fmt.Errorf("failed to check if similar is ignored: %w", err)
			}
			if !similarIgnored {
				validSimilar = append(validSimilar, similar)
			}
		}

		if len(validSimilar) == 0 {
			// No similar chunks found above threshold
			continue
		}

		// Track if we've added the new chunk to a cluster yet
		var newChunkClusterID int64

		for _, similar := range validSimilar {
			// Check if the new chunk is already in a cluster
			if newChunkClusterID == 0 {
				newChunkClusterID, err = db.GetClusterIDForChunk(chunkID)
				if err != nil {
					return clustered, fmt.Errorf("failed to get cluster for new chunk: %w", err)
				}
			}

			// Check if similar chunk is in a cluster
			similarClusterID, err := db.GetClusterIDForChunk(similar.ID)
			if err != nil {
				return clustered, fmt.Errorf("failed to get cluster for similar: %w", err)
			}

			if similarClusterID > 0 {
				// Similar chunk is in a cluster
				if newChunkClusterID == 0 {
					// Add new chunk to similar's cluster
					if err := db.AddChunkToCluster(similarClusterID, chunkID, similar.Similarity); err != nil {
						return clustered, fmt.Errorf("failed to add chunk to cluster: %w", err)
					}
					newChunkClusterID = similarClusterID
					clustered++
				}
				// New chunk is now in a cluster, we're done with this chunk
				break
			} else {
				// Similar chunk is NOT in a cluster
				if newChunkClusterID == 0 {
					// Create new cluster with new chunk as canonical
					newChunkClusterID, err = db.CreateCluster(chunkID, similar.Similarity, similar.Similarity, 2)
					if err != nil {
						return clustered, fmt.Errorf("failed to create cluster: %w", err)
					}
					// Add new chunk (canonical)
					if err := db.AddChunkToCluster(newChunkClusterID, chunkID, 1.0); err != nil {
						return clustered, fmt.Errorf("failed to add canonical chunk: %w", err)
					}
					// Add similar chunk
					if err := db.AddChunkToCluster(newChunkClusterID, similar.ID, similar.Similarity); err != nil {
						return clustered, fmt.Errorf("failed to add similar chunk: %w", err)
					}
					clustered++
				} else {
					// Add similar chunk to the new chunk's cluster
					if err := db.AddChunkToCluster(newChunkClusterID, similar.ID, similar.Similarity); err != nil {
						return clustered, fmt.Errorf("failed to add similar to cluster: %w", err)
					}
				}
				// Continue to next similar chunk to potentially add more
			}
		}

		// Update cluster stats if we created/modified a cluster
		if newChunkClusterID > 0 {
			if err := db.UpdateClusterStats(newChunkClusterID); err != nil {
				return clustered, fmt.Errorf("failed to update cluster stats: %w", err)
			}
		}
	}

	// Clean up any clusters that may have become too small
	if err := db.DeleteEmptyClusters(); err != nil {
		return clustered, fmt.Errorf("failed to delete empty clusters: %w", err)
	}

	return clustered, nil
}

// LoadClusters loads all clusters from the database and returns them as Cluster objects.
// Clusters are sorted according to the specified scoring strategy.
func LoadClusters(db *store.Store, scoreStrategy ScoreStrategy) ([]*Cluster, error) {
	rawClusters, err := db.GetAllClustersRaw()
	if err != nil {
		return nil, fmt.Errorf("failed to get clusters: %w", err)
	}

	var clusters []*Cluster
	for _, raw := range rawClusters {
		// Get canonical chunk
		canonical, err := db.GetChunkByID(raw.CanonicalChunkID)
		if err != nil {
			return nil, fmt.Errorf("failed to get canonical chunk: %w", err)
		}

		// Get all chunks in cluster
		var chunks []*store.Chunk
		for i, chunkID := range raw.ChunkIDs {
			chunk, err := db.GetChunkByID(chunkID)
			if err != nil {
				return nil, fmt.Errorf("failed to get chunk: %w", err)
			}
			chunk.Similarity = raw.Similarities[i]
			chunks = append(chunks, chunk)
		}

		c := &Cluster{
			ID:             raw.ID,
			ClusterID:      canonical.SHA,
			CanonicalChunk: canonical,
			Chunks:         chunks,
			AvgSimilarity:  raw.AvgSimilarity,
			MaxSimilarity:  raw.MaxSimilarity,
		}
		// Compute and store score
		c.Score = c.ComputeScore(scoreStrategy)
		clusters = append(clusters, c)
	}

	// Sort by score descending
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Score > clusters[j].Score
	})

	return clusters, nil
}

// FindClusters finds clusters of similar chunks
func FindClusters(db *store.Store, threshold float64, ignoreThreshold float64, verbose bool) ([]*Cluster, error) {
	// Delete existing clusters
	if err := db.DeleteAllClusters(); err != nil {
		return nil, fmt.Errorf("failed to delete existing clusters: %w", err)
	}

	// Quantize vectors for fast searching (if sqlite-vector is loaded)
	if err := db.QuantizeVectors(); err != nil {
		return nil, fmt.Errorf("failed to quantize vectors: %w", err)
	}

	// Get all chunks ordered by code length descending
	chunks, err := db.GetAllChunks()
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}

	var clusters []*Cluster
	totalChunks := len(chunks)
	processed := 0
	progressInterval := util.GetProgressInterval(totalChunks)

	// Process each chunk
	for _, chunk := range chunks {
		processed++

		// Show progress if verbose mode is enabled
		if verbose && (processed%progressInterval == 0 || processed == totalChunks) {
			fmt.Fprintf(os.Stderr, "  Processed %d/%d chunks, found %d clusters so far\n", processed, totalChunks, len(clusters))
		}

		// Check if chunk is already in a cluster
		inCluster, err := db.IsChunkInCluster(chunk.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check if chunk is in cluster: %w", err)
		}
		if inCluster {
			continue
		}

		// Check if this chunk is ignored (Phase 1: SHA, Phase 2: similarity)
		ignored, err := db.IsIgnoredBySimilarity(chunk, ignoreThreshold)
		if err != nil {
			return nil, fmt.Errorf("failed to check if chunk is ignored: %w", err)
		}
		if ignored {
			continue
		}

		// Find similar chunks
		similarChunks, err := db.FindSimilarChunks(chunk.ID, DefaultSimilarChunksLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to find similar chunks: %w", err)
		}

		// Filter by threshold and exclude chunks already in clusters or ignored
		var validSimilar []*store.Chunk
		for _, similar := range similarChunks {
			if similar.Similarity >= threshold {
				inCluster, err := db.IsChunkInCluster(similar.ID)
				if err != nil {
					return nil, fmt.Errorf("failed to check if chunk is in cluster: %w", err)
				}
				if inCluster {
					continue
				}

				// Check if this similar chunk is ignored (Phase 1: SHA, Phase 2: similarity)
				ignored, err := db.IsIgnoredBySimilarity(similar, ignoreThreshold)
				if err != nil {
					return nil, fmt.Errorf("failed to check if chunk is ignored: %w", err)
				}
				if !ignored {
					validSimilar = append(validSimilar, similar)
				}
			}
		}

		// Only create a cluster if we have at least one similar chunk
		// (canonical chunk + similar chunks = at least 2 chunks total)
		if len(validSimilar) > 0 {
			// Calculate statistics
			var avgSim, maxSim float64
			for _, similar := range validSimilar {
				avgSim += similar.Similarity
				if similar.Similarity > maxSim {
					maxSim = similar.Similarity
				}
			}
			avgSim /= float64(len(validSimilar))

			// Create cluster in database
			clusterID, err := db.CreateCluster(chunk.ID, avgSim, maxSim, len(validSimilar)+1)
			if err != nil {
				return nil, fmt.Errorf("failed to create cluster: %w", err)
			}

			// Add canonical chunk to cluster
			if err := db.AddChunkToCluster(clusterID, chunk.ID, 1.0); err != nil {
				return nil, fmt.Errorf("failed to add canonical chunk to cluster: %w", err)
			}

			// Add similar chunks to cluster
			clusterChunks := []*store.Chunk{chunk}
			for _, similar := range validSimilar {
				if err := db.AddChunkToCluster(clusterID, similar.ID, similar.Similarity); err != nil {
					return nil, fmt.Errorf("failed to add chunk to cluster: %w", err)
				}
				clusterChunks = append(clusterChunks, similar)
			}

			// Create cluster object
			cluster := &Cluster{
				ID:             clusterID,
				ClusterID:      chunk.SHA, // Use canonical chunk's SHA as cluster ID
				CanonicalChunk: chunk,
				Chunks:         clusterChunks,
				AvgSimilarity:  avgSim,
				MaxSimilarity:  maxSim,
			}

			// Only add clusters with more than 1 chunk
			if len(clusterChunks) > 1 {
				clusters = append(clusters, cluster)
			}
		}
	}

	// Sort clusters by canonical chunk length (descending)
	sort.Slice(clusters, func(i, j int) bool {
		return len(clusters[i].CanonicalChunk.Code) > len(clusters[j].CanonicalChunk.Code)
	})

	return clusters, nil
}
