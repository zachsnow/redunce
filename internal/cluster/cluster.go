package cluster

import (
	"fmt"

	"github.com/ZachSnow/redunce/internal/store"
)

// Cluster represents a group of similar chunks
type Cluster struct {
	ID             int64
	CanonicalChunk *store.Chunk
	Chunks         []*store.Chunk
	AvgSimilarity  float64
	MaxSimilarity  float64
}

// FindClusters finds clusters of similar chunks
func FindClusters(db *store.Store, threshold float64) ([]*Cluster, error) {
	// Delete existing clusters
	if err := db.DeleteAllClusters(); err != nil {
		return nil, fmt.Errorf("failed to delete existing clusters: %w", err)
	}

	// Get all chunks ordered by code length descending
	chunks, err := db.GetAllChunks()
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}

	var clusters []*Cluster

	// Process each chunk
	for _, chunk := range chunks {
		// Check if chunk is already in a cluster
		inCluster, err := db.IsChunkInCluster(chunk.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check if chunk is in cluster: %w", err)
		}
		if inCluster {
			continue
		}

		// Find similar chunks
		similarChunks, err := db.FindSimilarChunks(chunk.ID, 10)
		if err != nil {
			return nil, fmt.Errorf("failed to find similar chunks: %w", err)
		}

		// Filter by threshold and exclude chunks already in clusters
		var validSimilar []*store.Chunk
		for _, similar := range similarChunks {
			if similar.Similarity >= threshold {
				inCluster, err := db.IsChunkInCluster(similar.ID)
				if err != nil {
					return nil, fmt.Errorf("failed to check if chunk is in cluster: %w", err)
				}
				if !inCluster {
					validSimilar = append(validSimilar, similar)
				}
			}
		}

		// If we have similar chunks, create a cluster
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
				CanonicalChunk: chunk,
				Chunks:         clusterChunks,
				AvgSimilarity:  avgSim,
				MaxSimilarity:  maxSim,
			}

			clusters = append(clusters, cluster)
		}
	}

	return clusters, nil
}
