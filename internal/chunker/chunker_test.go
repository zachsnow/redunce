package chunker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLanguageFromExtension(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		// Standard mappings
		{".go", "go"},
		{".py", "python"},
		{".js", "javascript"},
		{".ts", "typescript"},
		{".tsx", "tsx"},
		{".rs", "rust"},
		{".java", "java"},
		{".rb", "ruby"},
		{".swift", "swift"},
		{".kt", "kotlin"},
		{".scala", "scala"},
		{".cs", "csharp"},

		// C/C++ family
		{".c", "c"},
		{".h", "c"},
		{".cpp", "cpp"},
		{".cc", "cpp"},
		{".hpp", "cpp"},
		{".hxx", "cpp"},

		// Config/data formats
		{".json", "json"},
		{".yaml", "yaml"},
		{".yml", "yaml"},
		{".toml", "toml"},
		{".sql", "sql"},

		// Shell
		{".sh", "bash"},
		{".bash", "bash"},

		// Web
		{".html", "html"},
		{".css", "css"},
		{".svelte", "svelte"},

		// Case insensitivity
		{".GO", "go"},
		{".Py", "python"},

		// Unknown extensions
		{".unknown", ""},
		{".xyz", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := LanguageFromExtension(tt.ext)
			if result != tt.expected {
				t.Errorf("LanguageFromExtension(%q) = %q, want %q", tt.ext, result, tt.expected)
			}
		})
	}
}

func TestChunkByLines(t *testing.T) {
	// Create a temporary file with known content
	content := `line1
line2
line3
line4
line5
line6
line7
line8
line9
line10`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Chunk with window size 3, step 2
	chunks, err := ChunkByLines(tmpFile, 3, 5, 2)
	if err != nil {
		t.Fatalf("ChunkByLines failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk")
	}

	// Verify chunk properties
	for _, chunk := range chunks {
		// All chunks should have the file path
		if chunk.Path != tmpFile {
			t.Errorf("Chunk path = %q, want %q", chunk.Path, tmpFile)
		}

		// Language should be detected as "text" for unknown extensions
		if chunk.Language != "text" {
			t.Errorf("Chunk language = %q, want 'text'", chunk.Language)
		}

		// Lines should be valid (1-indexed)
		if chunk.StartLine < 1 {
			t.Errorf("StartLine = %d, should be >= 1", chunk.StartLine)
		}
		if chunk.EndLine < chunk.StartLine {
			t.Errorf("EndLine (%d) < StartLine (%d)", chunk.EndLine, chunk.StartLine)
		}

		// Code should not be empty
		if chunk.Code == "" {
			t.Error("Chunk code should not be empty")
		}
	}
}

func TestChunkByLinesEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(tmpFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	chunks, err := ChunkByLines(tmpFile, 3, 10, 1)
	if err != nil {
		t.Fatalf("ChunkByLines failed: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty file, got %d", len(chunks))
	}
}

func TestChunkByLinesWindowSizes(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Test that different parameters produce chunks
	chunksA, _ := ChunkByLines(tmpFile, 2, 3, 1)
	chunksB, _ := ChunkByLines(tmpFile, 5, 8, 1)

	// Both should produce chunks
	if len(chunksA) == 0 {
		t.Error("Expected chunks from first configuration")
	}
	if len(chunksB) == 0 {
		t.Error("Expected chunks from second configuration")
	}

	// Verify that chunk sizes are within expected bounds
	for _, chunk := range chunksA {
		lineCount := chunk.EndLine - chunk.StartLine + 1
		if lineCount < 2 || lineCount > 3 {
			t.Errorf("Chunk line count %d outside range [2,3]", lineCount)
		}
	}
	for _, chunk := range chunksB {
		lineCount := chunk.EndLine - chunk.StartLine + 1
		if lineCount < 5 || lineCount > 8 {
			t.Errorf("Chunk line count %d outside range [5,8]", lineCount)
		}
	}
}
