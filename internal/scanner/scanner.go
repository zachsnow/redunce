package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/go-enry/go-enry/v2"
)

// Default ignore patterns - these are always applied before user's .redunceignore
const defaultRedunceignore = `# Default redunce ignore patterns
# Users can override these with ! in their .redunceignore

# Binary files
*.exe
*.dll
*.so
*.dylib
*.a
*.o

# Database files
*.db
*.sqlite
*.sqlite3

# Media files
*.pdf
*.jpg
*.jpeg
*.png
*.gif
*.bmp
*.ico
*.mp3
*.mp4
*.avi
*.mov

# Fonts
*.ttf
*.woff
*.woff2
*.eot

# Archives
*.zip
*.tar
*.gz
*.rar
*.7z

# Package manager lock files
go.sum
package-lock.json
yarn.lock
pnpm-lock.yaml
Cargo.lock
Gemfile.lock
composer.lock
Pipfile.lock
poetry.lock

# Build/config files
go.mod
package.json
requirements.txt
Makefile
Dockerfile
docker-compose.yml
.dockerignore
.gitattributes

# Documentation files
*.md
*.txt
*.rst
*.adoc
README*
LICENSE
COPYING

# Common directories to skip
.git/
.svn/
.hg/
node_modules/
.venv/
venv/
__pycache__/
.pytest_cache/
.mypy_cache/
build/
dist/
target/
.gradle/
.idea/
.vscode/

# Hidden files.
.*
`

// parseIgnorePatterns parses ignore patterns from a string (like file content)
func parseIgnorePatterns(content string) []string {
	var patterns []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// GetResolvedIgnorePatterns returns the combined ignore patterns (default + gitignore + redunceignore)
func GetResolvedIgnorePatterns(baseDir string) ([]string, error) {
	// Start with default patterns
	patterns := parseIgnorePatterns(defaultRedunceignore)

	// Add .gitignore patterns
	gitignorePath := filepath.Join(baseDir, ".gitignore")
	gitignorePatterns, err := loadIgnoreFile(gitignorePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load .gitignore: %w", err)
	}
	patterns = append(patterns, gitignorePatterns...)

	// Add .redunceignore patterns (these come last so they can override)
	redunceignorePath := filepath.Join(baseDir, ".redunceignore")
	redunceignorePatterns, err := loadIgnoreFile(redunceignorePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load .redunceignore: %w", err)
	}
	patterns = append(patterns, redunceignorePatterns...)

	return patterns, nil
}

// shouldAnalyzeFile uses enry to determine if a file should be analyzed
// Returns true for programming language files, false for vendored, generated, docs, or data files
func shouldAnalyzeFile(path string, content []byte) bool {
	// Use enry to detect file type
	language := enry.GetLanguage(filepath.Base(path), content)

	// Skip if not a programming language (e.g., data files, configs)
	// Empty language means enry couldn't detect it
	if language == "" {
		return false
	}

	// Skip vendored files (third-party code)
	if enry.IsVendor(path) {
		return false
	}

	// Skip generated files (auto-generated code)
	if enry.IsGenerated(path, content) {
		return false
	}

	// All other detected programming languages are OK
	return true
}

// ScanPaths recursively scans the given paths and returns all source files
func ScanPaths(paths []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)

	for _, path := range paths {
		// Get absolute path
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for %s: %w", path, err)
		}

		// Check if path exists
		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat %s: %w", absPath, err)
		}

		if info.IsDir() {
			// Get resolved ignore patterns (default + .gitignore + .redunceignore)
			allPatterns, err := GetResolvedIgnorePatterns(absPath)
			if err != nil {
				return nil, err
			}

			// Walk directory
			err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				// Skip if matching ignore patterns (works for both files and directories)
				ignored, _ := matchesIgnorePatterns(path, absPath, allPatterns)
				if ignored {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				// Continue walking into directories
				if info.IsDir() {
					return nil
				}

				// Skip if already seen
				if seen[path] {
					return nil
				}

				// Skip binary files by content
				if isBinaryFile(path) {
					return nil
				}

				// Read file content for enry analysis
				content, err := os.ReadFile(path)
				if err != nil {
					// If we can't read it, skip it
					return nil
				}

				// Use enry to determine if we should analyze this file
				if !shouldAnalyzeFile(path, content) {
					return nil
				}

				// Add file
				files = append(files, path)
				seen[path] = true
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("failed to walk directory %s: %w", absPath, err)
			}
		} else {
			// Single file
			if !seen[absPath] {
				// Skip binary files by content
				if isBinaryFile(absPath) {
					continue
				}

				// Read file content for enry analysis
				content, err := os.ReadFile(absPath)
				if err != nil {
					return nil, fmt.Errorf("failed to read file %s: %w", absPath, err)
				}

				// Use enry to determine if we should analyze this file
				if shouldAnalyzeFile(absPath, content) {
					files = append(files, absPath)
					seen[absPath] = true
				}
			}
		}
	}

	return files, nil
}

// isBinaryFile detects if a file is binary by examining its content
// Returns true if the file appears to be binary
func isBinaryFile(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false // If we can't open it, assume it's not binary (will fail later)
	}
	defer file.Close()

	// Read first 512 bytes (or less if file is smaller)
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false
	}
	if n == 0 {
		return false // Empty file is not binary
	}

	buf = buf[:n]

	// Check for null bytes (strong indicator of binary)
	for _, b := range buf {
		if b == 0 {
			return true
		}
	}

	// Check ratio of printable to non-printable characters
	// If more than 30% are non-printable (excluding common whitespace), likely binary
	nonPrintable := 0
	for _, b := range buf {
		r := rune(b)
		// Allow common whitespace characters
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		// Check if printable
		if !unicode.IsPrint(r) && r != '\n' && r != '\r' && r != '\t' {
			nonPrintable++
		}
	}

	ratio := float64(nonPrintable) / float64(len(buf))
	return ratio > 0.30
}

// loadIgnoreFile reads a .gitignore or .redunceignore file and returns the patterns
func loadIgnoreFile(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	return parseIgnorePatterns(string(content)), nil
}

// matchesIgnorePatterns checks if a path matches any ignore pattern (.gitignore or .redunceignore)
// Supports negation patterns with "!" prefix
func matchesIgnorePatterns(path string, baseDir string, patterns []string) (bool, string) {
	// Get relative path from base directory
	relPath, err := filepath.Rel(baseDir, path)
	if err != nil {
		return false, ""
	}

	// Never exclude the root directory itself
	if relPath == "." {
		return false, ""
	}

	ignored := false
	var matchedPattern string

	// Process patterns in order - later patterns can override earlier ones
	for _, pattern := range patterns {
		isNegation := strings.HasPrefix(pattern, "!")
		originalPattern := pattern
		if isNegation {
			pattern = strings.TrimPrefix(pattern, "!")
		}

		// Remove leading slash
		pattern = strings.TrimPrefix(pattern, "/")

		matched := false

		// Simple pattern matching
		m, err := filepath.Match(pattern, filepath.Base(relPath))
		if err == nil && m {
			matched = true
		}

		// Check if pattern matches full path
		if !matched {
			m, err = filepath.Match(pattern, relPath)
			if err == nil && m {
				matched = true
			}
		}

		// Check directory patterns (ending with /)
		if !matched && strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")
			if strings.Contains(relPath, dirPattern+string(filepath.Separator)) {
				matched = true
			}
			if filepath.Base(path) == dirPattern {
				matched = true
			}
		}

		// Check wildcard patterns like *.db
		if !matched && strings.Contains(pattern, "*") {
			m, _ := filepath.Match(pattern, filepath.Base(path))
			if m {
				matched = true
			}
		}

		// Apply the pattern
		if matched {
			if isNegation {
				ignored = false // Negation pattern - un-ignore
				matchedPattern = ""
			} else {
				ignored = true // Normal pattern - ignore
				matchedPattern = originalPattern
			}
		}
	}

	return ignored, matchedPattern
}
