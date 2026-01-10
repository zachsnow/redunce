package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIgnorePatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple patterns",
			input:    "*.log\n*.db\nnode_modules/",
			expected: []string{"*.log", "*.db", "node_modules/"},
		},
		{
			name:     "with comments and empty lines",
			input:    "# Comment\n*.log\n\n# Another comment\n*.db",
			expected: []string{"*.log", "*.db"},
		},
		{
			name:     "with whitespace",
			input:    "  *.log  \n  *.db  ",
			expected: []string{"*.log", "*.db"},
		},
		{
			name:     "negation patterns",
			input:    "*.log\n!important.log",
			expected: []string{"*.log", "!important.log"},
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "only comments",
			input:    "# Comment 1\n# Comment 2",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIgnorePatterns(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseIgnorePatterns() returned %d patterns, want %d", len(result), len(tt.expected))
				return
			}
			for i, pattern := range result {
				if pattern != tt.expected[i] {
					t.Errorf("parseIgnorePatterns()[%d] = %q, want %q", i, pattern, tt.expected[i])
				}
			}
		})
	}
}

func TestMatchesIgnorePatterns(t *testing.T) {
	baseDir := "/project"

	tests := []struct {
		name            string
		path            string
		patterns        []string
		expectedIgnored bool
		expectedPattern string
	}{
		{
			name:            "matches wildcard pattern",
			path:            "/project/debug.log",
			patterns:        []string{"*.log"},
			expectedIgnored: true,
			expectedPattern: "*.log",
		},
		{
			name:            "no match",
			path:            "/project/main.go",
			patterns:        []string{"*.log"},
			expectedIgnored: false,
			expectedPattern: "",
		},
		{
			name:            "matches exact filename",
			path:            "/project/Makefile",
			patterns:        []string{"Makefile"},
			expectedIgnored: true,
			expectedPattern: "Makefile",
		},
		{
			name:            "negation overrides previous pattern",
			path:            "/project/important.log",
			patterns:        []string{"*.log", "!important.log"},
			expectedIgnored: false,
			expectedPattern: "",
		},
		{
			name:            "later pattern overrides earlier",
			path:            "/project/test.log",
			patterns:        []string{"!*.log", "*.log"},
			expectedIgnored: true,
			expectedPattern: "*.log",
		},
		{
			name:            "directory pattern",
			path:            "/project/node_modules/package/index.js",
			patterns:        []string{"node_modules/"},
			expectedIgnored: true,
			expectedPattern: "node_modules/",
		},
		{
			name:            "root directory not ignored",
			path:            "/project",
			patterns:        []string{"*"},
			expectedIgnored: false,
			expectedPattern: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ignored, pattern := matchesIgnorePatterns(tt.path, baseDir, tt.patterns)
			if ignored != tt.expectedIgnored {
				t.Errorf("matchesIgnorePatterns() ignored = %v, want %v", ignored, tt.expectedIgnored)
			}
			if pattern != tt.expectedPattern {
				t.Errorf("matchesIgnorePatterns() pattern = %q, want %q", pattern, tt.expectedPattern)
			}
		})
	}
}

func TestIsBinaryFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{
			name:     "text file",
			content:  []byte("Hello, world!\nThis is a text file.\n"),
			expected: false,
		},
		{
			name:     "empty file",
			content:  []byte{},
			expected: false,
		},
		{
			name:     "file with null bytes",
			content:  []byte("Hello\x00World"),
			expected: true,
		},
		{
			name:     "binary file (high non-printable ratio)",
			content:  []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, // All non-printable with null
			expected: true,
		},
		{
			name:     "source code",
			content:  []byte("func main() {\n\tfmt.Println(\"Hello\")\n}\n"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, tt.name)
			if err := os.WriteFile(path, tt.content, 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			result := isBinaryFile(path)
			if result != tt.expected {
				t.Errorf("isBinaryFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLoadIgnoreFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("existing file", func(t *testing.T) {
		path := filepath.Join(tmpDir, ".gitignore")
		content := "*.log\n# comment\n*.db"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		patterns, err := loadIgnoreFile(path)
		if err != nil {
			t.Fatalf("loadIgnoreFile() error = %v", err)
		}

		expected := []string{"*.log", "*.db"}
		if len(patterns) != len(expected) {
			t.Errorf("loadIgnoreFile() returned %d patterns, want %d", len(patterns), len(expected))
			return
		}
		for i, pattern := range patterns {
			if pattern != expected[i] {
				t.Errorf("loadIgnoreFile()[%d] = %q, want %q", i, pattern, expected[i])
			}
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		patterns, err := loadIgnoreFile(filepath.Join(tmpDir, "nonexistent"))
		if err != nil {
			t.Errorf("loadIgnoreFile() error = %v, want nil for non-existent file", err)
		}
		if patterns != nil {
			t.Errorf("loadIgnoreFile() = %v, want nil for non-existent file", patterns)
		}
	})
}

func TestScanPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test directory structure
	if err := os.MkdirAll(filepath.Join(tmpDir, "src"), 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create source files
	goFile := filepath.Join(tmpDir, "src", "main.go")
	if err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a file that should be ignored
	logFile := filepath.Join(tmpDir, "debug.log")
	if err := os.WriteFile(logFile, []byte("log content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create .gitignore
	gitignore := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("*.log\n"), 0644); err != nil {
		t.Fatalf("Failed to create .gitignore: %v", err)
	}

	t.Run("scans source files", func(t *testing.T) {
		files, err := ScanPaths([]string{tmpDir}, false)
		if err != nil {
			t.Fatalf("ScanPaths() error = %v", err)
		}

		// Should find the .go file
		found := false
		for _, f := range files {
			if f == goFile {
				found = true
			}
			// Should not find the .log file
			if f == logFile {
				t.Errorf("ScanPaths() included ignored file %s", logFile)
			}
		}
		if !found {
			t.Errorf("ScanPaths() did not find %s", goFile)
		}
	})

	t.Run("handles single file", func(t *testing.T) {
		files, err := ScanPaths([]string{goFile}, false)
		if err != nil {
			t.Fatalf("ScanPaths() error = %v", err)
		}

		if len(files) != 1 || files[0] != goFile {
			t.Errorf("ScanPaths() = %v, want [%s]", files, goFile)
		}
	})

	t.Run("non-existent path returns error", func(t *testing.T) {
		_, err := ScanPaths([]string{filepath.Join(tmpDir, "nonexistent")}, false)
		if err == nil {
			t.Error("ScanPaths() expected error for non-existent path")
		}
	})
}
