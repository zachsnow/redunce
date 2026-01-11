package cluster

import (
	"math"
	"testing"

	"github.com/zachsnow/redunce/internal/store"
)

func TestClusterTotalLines(t *testing.T) {
	c := &Cluster{
		Chunks: []*store.Chunk{
			{StartLine: 1, EndLine: 10},  // 10 lines
			{StartLine: 20, EndLine: 30}, // 11 lines
			{StartLine: 50, EndLine: 55}, // 6 lines
		},
	}

	expected := 27
	result := c.TotalLines()
	if result != expected {
		t.Errorf("TotalLines() = %d, want %d", result, expected)
	}
}

func TestClusterCanonicalLines(t *testing.T) {
	c := &Cluster{
		CanonicalChunk: &store.Chunk{StartLine: 10, EndLine: 25},
	}

	expected := 16
	result := c.CanonicalLines()
	if result != expected {
		t.Errorf("CanonicalLines() = %d, want %d", result, expected)
	}
}

func TestComputeScore(t *testing.T) {
	// Create a test cluster
	canonical := &store.Chunk{
		StartLine: 1,
		EndLine:   20,
		Code:      "func example() {\n    // 20 lines of code\n}",
	}
	c := &Cluster{
		CanonicalChunk: canonical,
		Chunks: []*store.Chunk{
			{StartLine: 1, EndLine: 20},    // 20 lines
			{StartLine: 50, EndLine: 70},   // 21 lines
			{StartLine: 100, EndLine: 115}, // 16 lines
		},
		AvgSimilarity: 0.9,
	}

	t.Run("ScoreOld returns code length", func(t *testing.T) {
		score := c.ComputeScore(ScoreOld)
		expected := float64(len(canonical.Code))
		if score != expected {
			t.Errorf("ComputeScore(ScoreOld) = %v, want %v", score, expected)
		}
	})

	t.Run("ScoreImpact returns total lines", func(t *testing.T) {
		score := c.ComputeScore(ScoreImpact)
		expected := float64(57) // 20 + 21 + 16
		if score != expected {
			t.Errorf("ComputeScore(ScoreImpact) = %v, want %v", score, expected)
		}
	})

	t.Run("ScoreDefault uses weighted formula", func(t *testing.T) {
		score := c.ComputeScore(ScoreDefault)
		// totalLines * avgSim * (1 + log2(chunkCount))
		// 57 * 0.9 * (1 + log2(3))
		totalLines := 57.0
		avgSim := 0.9
		logFactor := 1.0 + math.Log2(3)
		expected := totalLines * avgSim * logFactor

		if math.Abs(score-expected) > 1e-9 {
			t.Errorf("ComputeScore(ScoreDefault) = %v, want %v", score, expected)
		}
	})

	t.Run("ScoreDefault with single chunk", func(t *testing.T) {
		singleChunkCluster := &Cluster{
			CanonicalChunk: canonical,
			Chunks:         []*store.Chunk{{StartLine: 1, EndLine: 10}},
			AvgSimilarity:  1.0,
		}
		score := singleChunkCluster.ComputeScore(ScoreDefault)
		// With 1 chunk, logFactor = 1.0
		expected := 10.0 * 1.0 * 1.0
		if math.Abs(score-expected) > 1e-9 {
			t.Errorf("ComputeScore(ScoreDefault) single chunk = %v, want %v", score, expected)
		}
	})
}

func TestScoreStrategy(t *testing.T) {
	// Verify that higher-impact clusters score higher with ScoreDefault
	smallCluster := &Cluster{
		CanonicalChunk: &store.Chunk{StartLine: 1, EndLine: 5, Code: "small"},
		Chunks:         []*store.Chunk{{StartLine: 1, EndLine: 5}, {StartLine: 10, EndLine: 15}},
		AvgSimilarity:  0.95,
	}

	largeCluster := &Cluster{
		CanonicalChunk: &store.Chunk{StartLine: 1, EndLine: 50, Code: "large code block"},
		Chunks: []*store.Chunk{
			{StartLine: 1, EndLine: 50},
			{StartLine: 100, EndLine: 150},
			{StartLine: 200, EndLine: 250},
			{StartLine: 300, EndLine: 350},
		},
		AvgSimilarity: 0.88,
	}

	smallScore := smallCluster.ComputeScore(ScoreDefault)
	largeScore := largeCluster.ComputeScore(ScoreDefault)

	if largeScore <= smallScore {
		t.Errorf("Expected large cluster score (%v) > small cluster score (%v)", largeScore, smallScore)
	}
}
