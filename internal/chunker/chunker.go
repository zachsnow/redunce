package chunker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/dockerfile"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/hcl"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	tree_sitter_markdown "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
	"github.com/smacker/go-tree-sitter/ocaml"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/sql"
	"github.com/smacker/go-tree-sitter/svelte"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
	tree_sitter_json "github.com/tree-sitter/tree-sitter-json/bindings/go"

	"github.com/ZachSnow/redunce/internal/store"
)

// Language registry - smacker/go-tree-sitter provides built-in support for 30+ languages
var languageParsers = map[string]*sitter.Language{
	"bash":       bash.GetLanguage(),
	"c":          c.GetLanguage(),
	"cpp":        cpp.GetLanguage(),
	"csharp":     csharp.GetLanguage(),
	"css":        css.GetLanguage(),
	"dockerfile": dockerfile.GetLanguage(),
	"go":         golang.GetLanguage(),
	"hcl":        hcl.GetLanguage(),
	"html":       html.GetLanguage(),
	"java":       java.GetLanguage(),
	"javascript": javascript.GetLanguage(),
	"json":       sitter.NewLanguage(tree_sitter_json.Language()),
	"kotlin":     kotlin.GetLanguage(),
	"lua":        lua.GetLanguage(),
	"markdown":   tree_sitter_markdown.GetLanguage(),
	"ocaml":      ocaml.GetLanguage(),
	"php":        php.GetLanguage(),
	"protobuf":   protobuf.GetLanguage(),
	"python":     python.GetLanguage(),
	"ruby":       ruby.GetLanguage(),
	"rust":       rust.GetLanguage(),
	"scala":      scala.GetLanguage(),
	"sql":        sql.GetLanguage(),
	"svelte":     svelte.GetLanguage(),
	"swift":      swift.GetLanguage(),
	"toml":       toml.GetLanguage(),
	"tsx":        tsx.GetLanguage(),
	"typescript": typescript.GetLanguage(),
	"yaml":       yaml.GetLanguage(),
}

// Default meaningful node types for code-like languages
var defaultMeaningfulNodeTypes = map[string]bool{
	// Go
	"function_declaration": true,
	"method_declaration":   true,
	"type_declaration":     true,
	"const_declaration":    true,
	"var_declaration":      true,
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
	// Common control flow
	"if_statement":     true,
	"for_statement":    true,
	"while_statement":  true,
	"switch_statement": true,
}

// Language-specific meaningful node types
var languageMeaningfulNodeTypes = map[string]map[string]bool{
	"json": {
		"object": true,
		"array":  true,
		"pair":   true,
	},
	"yaml": {
		"block_mapping":  true,
		"block_sequence": true,
		"flow_mapping":   true,
		"flow_sequence":  true,
	},
}

// getMeaningfulNodeTypes returns the meaningful node types for a given language
func getMeaningfulNodeTypes(language string) map[string]bool {
	if types, ok := languageMeaningfulNodeTypes[language]; ok {
		return types
	}
	return defaultMeaningfulNodeTypes
}

// LanguageFromExtension returns the language name for a file extension
func LanguageFromExtension(ext string) string {
	ext = strings.ToLower(ext)
	languages := map[string]string{
		// C/C++
		".c":   "c",
		".h":   "c",
		".cpp": "cpp",
		".cc":  "cpp",
		".cxx": "cpp",
		".hpp": "cpp",
		".hxx": "cpp",
		// Web
		".js":     "javascript",
		".jsx":    "javascript",
		".ts":     "typescript",
		".tsx":    "tsx",
		".html":   "html",
		".css":    "css",
		".scss":   "css",
		".sass":   "css",
		".svelte": "svelte",
		// Systems
		".go":    "go",
		".rs":    "rust",
		".swift": "swift",
		// JVM
		".java":  "java",
		".kt":    "kotlin",
		".scala": "scala",
		// Scripting
		".py":   "python",
		".rb":   "ruby",
		".php":  "php",
		".lua":  "lua",
		".sh":   "bash",
		".bash": "bash",
		".zsh":  "bash",
		".fish": "bash",
		// .NET
		".cs": "csharp",
		// Functional
		".ml":  "ocaml",
		".mli": "ocaml",
		// Data/Config
		".sql":        "sql",
		".json":       "json",
		".yaml":       "yaml",
		".yml":        "yaml",
		".toml":       "toml",
		".md":         "markdown",
		".proto":      "protobuf",
		".hcl":        "hcl",
		".tf":         "hcl",
		".dockerfile": "dockerfile",
	}

	// Special case: files named "Dockerfile" without extension
	if ext == "" {
		return ""
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
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(lang)
	tree := parser.Parse(nil, content)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse file")
	}
	defer tree.Close()

	// Extract chunks from AST
	var chunks []*store.Chunk
	root := tree.RootNode()

	// Get language-specific meaningful node types
	meaningfulNodeTypes := getMeaningfulNodeTypes(language)

	// Collect nodes using BFS to get a variety of depths
	var nodesToProcess []*sitter.Node
	queue := []*sitter.Node{root}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		// Check if this is a meaningful node
		nodeType := node.Type()
		if meaningfulNodeTypes[nodeType] {
			// Count child nodes
			childCount := int(node.ChildCount())
			if childCount >= minNodes && childCount <= maxNodes {
				nodesToProcess = append(nodesToProcess, node)
			}
		}

		// Add children to queue
		for i := uint32(0); i < node.ChildCount(); i++ {
			child := node.Child(int(i))
			if child != nil {
				queue = append(queue, child)
			}
		}
	}

	// Convert nodes to chunks
	for _, node := range nodesToProcess {
		startPos := node.StartPoint()
		endPos := node.EndPoint()

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
		queue = []*sitter.Node{root}

		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]

			nodeType := node.Type()
			if meaningfulNodeTypes[nodeType] {
				code := extractNodeCode(content, node)
				if strings.TrimSpace(code) != "" {
					startPos := node.StartPoint()
					endPos := node.EndPoint()

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
			for i := uint32(0); i < node.ChildCount(); i++ {
				child := node.Child(int(i))
				if child != nil {
					queue = append(queue, child)
				}
			}
		}
	}

	// Deduplicate chunks by location (file path + line range)
	// Keep the chunk with the longest code when duplicates exist
	chunkMap := make(map[string]*store.Chunk)
	for _, chunk := range chunks {
		key := fmt.Sprintf("%s:%d-%d", chunk.Path, chunk.StartLine, chunk.EndLine)
		if existing, ok := chunkMap[key]; !ok || len(chunk.Code) > len(existing.Code) {
			chunkMap[key] = chunk
		}
	}

	// Convert map back to slice
	var deduplicatedChunks []*store.Chunk
	for _, chunk := range chunkMap {
		deduplicatedChunks = append(deduplicatedChunks, chunk)
	}

	return deduplicatedChunks, nil
}

// extractNodeCode extracts the source code for a given node
func extractNodeCode(content []byte, node *sitter.Node) string {
	startByte := node.StartByte()
	endByte := node.EndByte()

	if startByte >= uint32(len(content)) || endByte > uint32(len(content)) {
		return ""
	}

	return string(content[startByte:endByte])
}
