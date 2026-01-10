package util

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float64{1, 2, 3},
			b:        []float64{1, 2, 3},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float64{1, 2, 3},
			b:        []float64{-1, -2, -3},
			expected: -1.0,
		},
		{
			name:     "scaled vectors (should be 1.0)",
			a:        []float64{1, 2, 3},
			b:        []float64{2, 4, 6},
			expected: 1.0,
		},
		{
			name:     "different length vectors",
			a:        []float64{1, 2},
			b:        []float64{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "zero vector a",
			a:        []float64{0, 0, 0},
			b:        []float64{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "zero vector b",
			a:        []float64{1, 2, 3},
			b:        []float64{0, 0, 0},
			expected: 0.0,
		},
		{
			name:     "partial similarity",
			a:        []float64{1, 0},
			b:        []float64{1, 1},
			expected: 1.0 / math.Sqrt(2), // ~0.707
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("CosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abcdef1234567890", "abcdef12"},
		{"short", "short"},
		{"12345678", "12345678"},
		{"1234567890", "12345678"},
		{"", ""},
	}

	for _, tt := range tests {
		result := ShortSHA(tt.input)
		if result != tt.expected {
			t.Errorf("ShortSHA(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetProgressInterval(t *testing.T) {
	tests := []struct {
		total    int
		expected int
	}{
		{1000, 100},   // 1000/10 = 100, capped at 100
		{5000, 100},   // 5000/10 = 500, capped at 100
		{50, 5},       // 50/10 = 5
		{5, 1},        // 5/10 = 0, floored to 1
		{0, 1},        // 0/10 = 0, floored to 1
	}

	for _, tt := range tests {
		result := GetProgressInterval(tt.total)
		if result != tt.expected {
			t.Errorf("GetProgressInterval(%d) = %d, want %d", tt.total, result, tt.expected)
		}
	}
}
