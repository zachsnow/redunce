package chunker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/ZachSnow/redunce/internal/store"
)

// Language registry - add new languages here by importing the package and adding to the map
var languageParsers = map[string]*tree_sitter.Language{
	"go":         tree_sitter.NewLanguage(tree_sitter_go.Language()),
	"javascript": tree_sitter.NewLanguage(tree_sitter_javascript.Language()),
	"typescript": tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript()),
	"python":     tree_sitter.NewLanguage(tree_sitter_python.Language()),
	// To add more languages:
	// 1. Run: go get github.com/tree-sitter/tree-sitter-<language>@latest
	// 2. Import: tree_sitter_<language> "github.com/tree-sitter/tree-sitter-<language>/bindings/go"
	// 3. Add entry here: "language": tree_sitter.NewLanguage(tree_sitter_<language>.Language()),
}

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

// ChunkWithTreeSitter chunks a file using tree-sitter AST parsing
func ChunkWithTreeSitter(path string, language string, minNodes, maxNodes int) ([]*store.Chunk, error) {
	// Check if we have a parser for this language
	lang, ok := languageParsers[language]
	if !ok {
		return nil, fmt.Errorf("no tree-sitter parser available for language: %s", language)
	}

	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse the file
	parser := tree_sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(lang)
	tree := parser.Parse(content, nil)
	defer tree.Close()

	// Extract chunks from AST
	var chunks []*store.Chunk
	root := tree.RootNode()

	// Node types that represent meaningful code units
	meaningfulNodeTypes := map[string]bool{
		// Go
		"function_declaration": true,
		"method_declaration":   true,
		"type_declaration":     true,
		"const_declaration":    true,
		"var_declaration":      true,
		"switch_statement":     true,
		// JavaScript/TypeScript
		"function":            true,
		"arrow_function":      true,
		"class_declaration":   true,
		"method_definition":   true,
		"lexical_declaration": true,
		// Python
		"function_definition":  true,
		"class_definition":     true,
		"decorated_definition": true,
		// Common across languages
		"if_statement":    true,
		"for_statement":   true,
		"while_statement": true,
		"with_statement":  true,
	}

	// Collect nodes using BFS to get a variety of depths
	var nodesToProcess []*tree_sitter.Node
	queue := []*tree_sitter.Node{root}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		// Check if this is a meaningful node
		nodeType := node.Kind()
		if meaningfulNodeTypes[nodeType] {
			// Count child nodes
			childCount := int(node.ChildCount())
			if childCount >= minNodes && childCount <= maxNodes {
				nodesToProcess = append(nodesToProcess, node)
			}
		}

		// Add children to queue
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child != nil {
				queue = append(queue, child)
			}
		}
	}

	// Convert nodes to chunks
	for _, node := range nodesToProcess {
		startPos := node.StartPosition()
		endPos := node.EndPosition()

		// Extract code for this node
		code := extractNodeCode(content, node)

		// Skip empty or too small chunks
		if strings.TrimSpace(code) == "" || len(strings.Split(code, "\n")) < 2 {
			continue
		}

		chunk := &store.Chunk{
			Path:      path,
			Language:  language,
			StartLine: int(startPos.Row) + 1, // 1-indexed
			EndLine:   int(endPos.Row) + 1,   // 1-indexed
			Code:      code,
		}
		chunks = append(chunks, chunk)
	}

	// If we didn't find enough chunks, fall back to extracting all meaningful nodes
	if len(chunks) < 5 {
		chunks = nil
		queue = []*tree_sitter.Node{root}

		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]

			nodeType := node.Kind()
			if meaningfulNodeTypes[nodeType] {
				code := extractNodeCode(content, node)
				if strings.TrimSpace(code) != "" {
					startPos := node.StartPosition()
					endPos := node.EndPosition()

					chunk := &store.Chunk{
						Path:      path,
						Language:  language,
						StartLine: int(startPos.Row) + 1,
						EndLine:   int(endPos.Row) + 1,
						Code:      code,
					}
					chunks = append(chunks, chunk)
				}
			}

			// Add children to queue
			for i := uint(0); i < node.ChildCount(); i++ {
				child := node.Child(i)
				if child != nil {
					queue = append(queue, child)
				}
			}
		}
	}

	return chunks, nil
}

// extractNodeCode extracts the source code for a given node
func extractNodeCode(content []byte, node *tree_sitter.Node) string {
	startByte := node.StartByte()
	endByte := node.EndByte()

	if startByte >= uint(len(content)) || endByte > uint(len(content)) {
		return ""
	}

	return string(content[startByte:endByte])
}
