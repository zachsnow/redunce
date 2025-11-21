package chunker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZachSnow/redunce/internal/store"
)

// LanguageFromExtension returns the language name for a file extension
func LanguageFromExtension(ext string) string {
	ext = strings.ToLower(ext)
	languages := map[string]string{
		".go":   "go",
		".js":   "javascript",
		".ts":   "typescript",
		".jsx":  "javascript",
		".tsx":  "typescript",
		".py":   "python",
		".rb":   "ruby",
		".java": "java",
		".c":    "c",
		".cpp":  "cpp",
		".cc":   "cpp",
		".cxx":  "cpp",
		".h":    "c",
		".hpp":  "cpp",
		".cs":   "csharp",
		".php":  "php",
		".swift": "swift",
		".kt":   "kotlin",
		".rs":   "rust",
		".scala": "scala",
		".m":    "objectivec",
		".mm":   "objectivec",
		".sh":   "bash",
		".bash": "bash",
		".zsh":  "bash",
		".fish": "bash",
		".sql":  "sql",
		".html": "html",
		".xml":  "xml",
		".css":  "css",
		".scss": "css",
		".sass": "css",
		".json": "json",
		".yaml": "yaml",
		".yml":  "yaml",
		".toml": "toml",
		".md":   "markdown",
	}

	if lang, ok := languages[ext]; ok {
		return lang
	}
	return ""
}

// ChunkByLines chunks a file using line-based windowing
func ChunkByLines(path string, minLines, maxLines, stepLines int) ([]*store.Chunk, error) {
	// Read file
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if len(lines) == 0 {
		return nil, nil
	}

	// Determine language
	ext := filepath.Ext(path)
	lang := LanguageFromExtension(ext)
	if lang == "" {
		lang = "text"
	}

	var chunks []*store.Chunk

	// Generate chunks for each window size
	for windowSize := minLines; windowSize <= maxLines; windowSize += stepLines {
		if windowSize > len(lines) {
			break
		}

		// Slide window across file
		for start := 0; start <= len(lines)-windowSize; start += stepLines {
			end := start + windowSize
			if end > len(lines) {
				end = len(lines)
			}

			// Join lines for this chunk
			code := strings.Join(lines[start:end], "\n")

			// Skip empty or whitespace-only chunks
			if strings.TrimSpace(code) == "" {
				continue
			}

			chunk := &store.Chunk{
				Path:      path,
				Language:  lang,
				StartLine: start + 1, // 1-indexed
				EndLine:   end,       // 1-indexed, inclusive
				Code:      code,
			}

			chunks = append(chunks, chunk)
		}
	}

	return chunks, nil
}

// ChunkWithTreeSitter chunks a file using tree-sitter (placeholder for now)
func ChunkWithTreeSitter(path string, language string, minDepth, maxDepth int) ([]*store.Chunk, error) {
	// For now, we'll implement a simple tree-sitter chunker
	// In a full implementation, this would use the go-tree-sitter library
	// and parse the AST to extract meaningful nodes

	// Since tree-sitter integration is complex and requires language-specific
	// grammars, we'll return an error to fall back to line-based chunking
	return nil, fmt.Errorf("tree-sitter not yet implemented")
}
