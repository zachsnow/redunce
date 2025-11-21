package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// List of common binary and non-source file extensions to skip
var skipExtensions = map[string]bool{
	".exe":   true,
	".dll":   true,
	".so":    true,
	".dylib": true,
	".a":     true,
	".o":     true,
	".db":    true, // Database files
	".sqlite": true,
	".sqlite3": true,
	".pdf":   true,
	".jpg":   true,
	".jpeg":  true,
	".png":   true,
	".gif":   true,
	".bmp":   true,
	".ico":   true,
	".zip":   true,
	".tar":   true,
	".gz":    true,
	".rar":   true,
	".7z":    true,
	".mp3":   true,
	".mp4":   true,
	".avi":   true,
	".mov":   true,
	".ttf":   true,
	".woff":  true,
	".woff2": true,
	".eot":   true,
}

// Common directories to skip
var skipDirs = map[string]bool{
	".git":         true,
	".svn":         true,
	".hg":          true,
	"node_modules": true,
	".venv":        true,
	"venv":         true,
	"__pycache__":  true,
	".pytest_cache": true,
	".mypy_cache":  true,
	"build":        true,
	"dist":         true,
	"target":       true,
	".gradle":      true,
	".idea":        true,
	".vscode":      true,
	".DS_Store":    true,
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
			// Load .gitignore from this directory
			gitignorePath := filepath.Join(absPath, ".gitignore")
			gitignorePatterns, err := loadGitignore(gitignorePath)
			if err != nil {
				return nil, fmt.Errorf("failed to load .gitignore: %w", err)
			}
			// Walk directory
			err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				// Skip directories we don't want to scan
				if info.IsDir() {
					if skipDirs[info.Name()] || strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
						return filepath.SkipDir
					}
					return nil
				}

				// Skip hidden files (except .gitignore which we want to analyze)
				if strings.HasPrefix(info.Name(), ".") && info.Name() != ".gitignore" {
					return nil
				}

				// Skip files matching .gitignore patterns (but not .gitignore itself)
				if info.Name() != ".gitignore" && len(gitignorePatterns) > 0 {
					if matchesGitignore(path, absPath, gitignorePatterns) {
						return nil
					}
				}

				// Skip binary and non-source files by extension
				ext := strings.ToLower(filepath.Ext(path))
				if skipExtensions[ext] {
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
				if !isBinaryFile(absPath) {
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

// loadGitignore reads a .gitignore file and returns the patterns
func loadGitignore(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	return patterns, scanner.Err()
}

// matchesGitignore checks if a path matches any gitignore pattern
func matchesGitignore(path string, baseDir string, patterns []string) bool {
	// Get relative path from base directory
	relPath, err := filepath.Rel(baseDir, path)
	if err != nil {
		return false
	}

	for _, pattern := range patterns {
		// Handle negation patterns (!)
		if strings.HasPrefix(pattern, "!") {
			continue // Skip negation for simplicity
		}

		// Remove leading slash
		pattern = strings.TrimPrefix(pattern, "/")

		// Simple pattern matching
		matched, err := filepath.Match(pattern, filepath.Base(relPath))
		if err == nil && matched {
			return true
		}

		// Check if pattern matches full path
		matched, err = filepath.Match(pattern, relPath)
		if err == nil && matched {
			return true
		}

		// Check directory patterns (ending with /)
		if strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")
			if strings.Contains(relPath, dirPattern+string(filepath.Separator)) {
				return true
			}
			if filepath.Base(path) == dirPattern {
				return true
			}
		}

		// Check wildcard patterns like *.db
		if strings.Contains(pattern, "*") {
			matched, _ = filepath.Match(pattern, filepath.Base(path))
			if matched {
				return true
			}
		}
	}

	return false
}
