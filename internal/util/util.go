package util

import "math"

// GetProgressInterval calculates an appropriate progress reporting interval
func GetProgressInterval(total int) int {
	interval := total / 10
	if interval > 100 {
		return 100
	}
	if interval < 1 {
		return 1
	}
	return interval
}

// ShortSHA returns the first 8 characters of a SHA (like git)
func ShortSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
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
