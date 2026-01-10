package store

import "testing"

func TestIsSubset(t *testing.T) {
	tests := []struct {
		name     string
		a        *Chunk
		b        *Chunk
		expected bool
	}{
		{
			name:     "a is subset of b (contained within)",
			a:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 20},
			b:        &Chunk{Path: "file.go", StartLine: 5, EndLine: 30},
			expected: true,
		},
		{
			name:     "b is subset of a (contained within)",
			a:        &Chunk{Path: "file.go", StartLine: 5, EndLine: 30},
			b:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 20},
			expected: true,
		},
		{
			name:     "equal ranges are not subsets",
			a:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 20},
			b:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 20},
			expected: false,
		},
		{
			name:     "different files are never subsets",
			a:        &Chunk{Path: "file1.go", StartLine: 10, EndLine: 20},
			b:        &Chunk{Path: "file2.go", StartLine: 5, EndLine: 30},
			expected: false,
		},
		{
			name:     "overlapping but not contained",
			a:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 25},
			b:        &Chunk{Path: "file.go", StartLine: 20, EndLine: 35},
			expected: false,
		},
		{
			name:     "non-overlapping ranges",
			a:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 20},
			b:        &Chunk{Path: "file.go", StartLine: 30, EndLine: 40},
			expected: false,
		},
		{
			name:     "a starts at same line but ends before b",
			a:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 15},
			b:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 20},
			expected: true,
		},
		{
			name:     "a ends at same line but starts after b",
			a:        &Chunk{Path: "file.go", StartLine: 15, EndLine: 20},
			b:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 20},
			expected: true,
		},
		{
			name:     "single line chunk inside larger",
			a:        &Chunk{Path: "file.go", StartLine: 15, EndLine: 15},
			b:        &Chunk{Path: "file.go", StartLine: 10, EndLine: 20},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSubset(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("IsSubset(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestComputeChunkSHA(t *testing.T) {
	// Same input should produce same SHA
	code := "func example() {}"
	sha1 := ComputeChunkSHA(code)
	sha2 := ComputeChunkSHA(code)

	if sha1 != sha2 {
		t.Errorf("Same code produced different SHAs: %s vs %s", sha1, sha2)
	}

	// Different input should produce different SHA
	differentCode := "func different() {}"
	sha3 := ComputeChunkSHA(differentCode)

	if sha1 == sha3 {
		t.Errorf("Different code produced same SHA: %s", sha1)
	}

	// SHA should be 40 characters (hex-encoded SHA-1)
	if len(sha1) != 40 {
		t.Errorf("SHA length = %d, want 40", len(sha1))
	}
}
