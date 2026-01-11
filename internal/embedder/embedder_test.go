package embedder

import (
	"math"
	"testing"

	"github.com/zachsnow/redunce/internal/store"
	"github.com/zachsnow/redunce/internal/util"
)

func TestPreprocessCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes empty lines",
			input:    "line1\n\nline2\n\nline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "removes brace-only lines",
			input:    "func foo() {\n    x := 1\n}",
			expected: "func foo() {\n    x := 1",
		},
		{
			name:     "preserves lines with content and braces",
			input:    "if x { y }",
			expected: "if x { y }",
		},
		{
			name:     "handles whitespace-only lines",
			input:    "line1\n   \nline2",
			expected: "line1\nline2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := preprocessCode(tt.input)
			if result != tt.expected {
				t.Errorf("preprocessCode(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTFIDFTokenize(t *testing.T) {
	embedder := NewTFIDFEmbedder(nil, 100, 0.2, false)

	tests := []struct {
		name          string
		input         string
		shouldHave    []string
		shouldNotHave []string
	}{
		{
			name:       "splits on non-alphanumeric",
			input:      "func_name(arg1, arg2)",
			shouldHave: []string{"func_name", "arg1", "arg2"},
		},
		{
			name:       "converts to lowercase",
			input:      "FunctionName CONSTANT",
			shouldHave: []string{"functionname", "constant"},
		},
		{
			name:          "filters single char tokens",
			input:         "a bb ccc",
			shouldHave:    []string{"bb", "ccc"},
			shouldNotHave: []string{"a"},
		},
		{
			name:       "preserves underscores",
			input:      "my_variable_name",
			shouldHave: []string{"my_variable_name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := embedder.tokenize(tt.input)
			tokenSet := make(map[string]bool)
			for _, tok := range tokens {
				tokenSet[tok] = true
			}

			for _, expected := range tt.shouldHave {
				if !tokenSet[expected] {
					t.Errorf("Expected token %q not found in %v", expected, tokens)
				}
			}

			for _, unexpected := range tt.shouldNotHave {
				if tokenSet[unexpected] {
					t.Errorf("Unexpected token %q found in %v", unexpected, tokens)
				}
			}
		})
	}
}

func TestTFIDFEmbedBatch(t *testing.T) {
	embedder := NewTFIDFEmbedder(nil, 100, 0.2, false)

	chunks := []*store.Chunk{
		{Code: "func foo() { return 1 }"},
		{Code: "func bar() { return 2 }"},
		{Code: "completely different words here"},
	}

	embeddings, err := embedder.EmbedBatch(chunks)
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}

	if len(embeddings) != 3 {
		t.Fatalf("Expected 3 embeddings, got %d", len(embeddings))
	}

	// All embeddings should have same dimension
	dim := len(embeddings[0])
	for i, emb := range embeddings {
		if len(emb) != dim {
			t.Errorf("Embedding %d has dimension %d, expected %d", i, len(emb), dim)
		}
	}

	// Similar chunks (foo and bar) should have higher similarity than dissimilar
	simFooBar := util.CosineSimilarity(embeddings[0], embeddings[1])
	simFooDiff := util.CosineSimilarity(embeddings[0], embeddings[2])

	if simFooBar <= simFooDiff {
		t.Errorf("Expected similar code similarity (%v) > dissimilar code similarity (%v)",
			simFooBar, simFooDiff)
	}
}

func TestTFIDFNormalization(t *testing.T) {
	embedder := NewTFIDFEmbedder(nil, 100, 0.2, false)

	chunks := []*store.Chunk{
		{Code: "func example() { return value }"},
	}

	embeddings, err := embedder.EmbedBatch(chunks)
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}

	// Vector should be normalized to unit length
	norm := 0.0
	for _, v := range embeddings[0] {
		norm += v * v
	}
	norm = math.Sqrt(norm)

	// Allow small floating point error
	if math.Abs(norm-1.0) > 1e-9 && norm != 0 {
		t.Errorf("Vector norm = %v, expected 1.0 (or 0 for empty)", norm)
	}
}
