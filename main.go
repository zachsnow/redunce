package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ZachSnow/redunce/internal/chunker"
	"github.com/ZachSnow/redunce/internal/cluster"
	"github.com/ZachSnow/redunce/internal/embedder"
	"github.com/ZachSnow/redunce/internal/output"
	"github.com/ZachSnow/redunce/internal/scanner"
	"github.com/ZachSnow/redunce/internal/store"
	"github.com/ZachSnow/redunce/internal/util"
)

//go:embed claude/commands/*.md
var claudeCommands embed.FS

const (
	// BatchSize is the number of chunks to embed in one API/processing batch.
	// Tuned for OpenAI rate limits and memory efficiency.
	BatchSize = 64

	// MaxClusterIndex is the maximum cluster index accepted by --ignore flag.
	// Indices above this are treated as SHA prefixes instead of cluster numbers.
	MaxClusterIndex = 1000

	// Version is the application version (source of truth for releases).
	Version = "0.1.0"
)

// Global verbose flag
var verbose bool

// logVerbose prints to stderr if verbose mode is enabled
func logVerbose(format string, args ...interface{}) {
	if verbose {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// findChunkBySHAPrefix finds chunks matching a SHA prefix.
func findChunkBySHAPrefix(chunks []*store.Chunk, prefix string) []*store.Chunk {
	var matches []*store.Chunk
	for _, chunk := range chunks {
		if len(chunk.SHA) >= len(prefix) && chunk.SHA[:len(prefix)] == prefix {
			matches = append(matches, chunk)
		}
	}
	return matches
}

// findClusterBySHAPrefix finds clusters whose ID (canonical chunk SHA) matches a prefix.
func findClusterBySHAPrefix(clusters []*cluster.Cluster, prefix string) []*cluster.Cluster {
	var matches []*cluster.Cluster
	for _, cl := range clusters {
		if len(cl.ClusterID) >= len(prefix) && cl.ClusterID[:len(prefix)] == prefix {
			matches = append(matches, cl)
		}
	}
	return matches
}

type Config struct {
	Threshold      float64
	IgnoreThreshold   float64
	EmbedMethod    string
	Format         string
	Score          string
	LocalMinLines  int
	LocalMaxLines  int
	LocalStepLines int
	TreesitterMin  int
	TreesitterMax  int
	OpenAIAPIKey   string
	DBPath         string
	Reset          bool
	Query          string
	IgnoreCluster     string
	PrintIgnore       bool
	PrintFiles        bool
	PrintChunks       bool
	PrintChunk        string
	PrintCluster      string
	MinChunkLength    int
	Verbose           bool
	LocalRefreezeThreshold float64
	LocalRefreeze          bool
	Limit                  int
	NoDefaultIgnore        bool
}

func main() {
	cfg := Config{}
	var showVersion bool
	var installClaudeCommands bool

	// Define flags
	flag.BoolVar(&showVersion, "version", false, "show version and exit")
	flag.BoolVar(&installClaudeCommands, "install-claude-commands", false, "install Claude Code slash commands (usage: --install-claude-commands [directory], defaults to ~)")
	flag.Float64Var(&cfg.Threshold, "threshold", 0.85, "similarity threshold (0-1)")
	flag.Float64Var(&cfg.IgnoreThreshold, "ignore-threshold", 0.95, "ignore similarity threshold (0-1), higher than clustering threshold")
	flag.StringVar(&cfg.EmbedMethod, "embed", "local", "embedding method: local|openai")
	flag.StringVar(&cfg.Format, "format", "md", "output format: json|md")
	flag.StringVar(&cfg.Score, "score", "default", "cluster scoring strategy: old|impact|default")
	flag.IntVar(&cfg.Limit, "limit", 10, "limit output to top N clusters (0 = no limit)")
	flag.IntVar(&cfg.LocalMinLines, "local-min-lines", 3, "minimum lines per chunk for local")
	flag.IntVar(&cfg.LocalMaxLines, "local-max-lines", 30, "maximum lines per chunk for local")
	flag.IntVar(&cfg.LocalStepLines, "local-step-lines", 3, "line step size for local chunker")
	flag.IntVar(&cfg.TreesitterMin, "treesitter-min", 5, "minimum node size for tree-sitter")
	flag.IntVar(&cfg.TreesitterMax, "treesitter-max", 10, "maximum node size for tree-sitter")
	flag.IntVar(&cfg.MinChunkLength, "min-chunk-length", 0, "minimum chunk length in lines (0 = no filter, also overrides local-min-lines)")
	flag.StringVar(&cfg.OpenAIAPIKey, "openai-api-key", "", "OpenAI API key (or set OPENAI_API_KEY env var)")
	flag.StringVar(&cfg.DBPath, "db", "redunce.db", "database file path")
	flag.BoolVar(&cfg.Reset, "reset", false, "reset database before running")
	flag.StringVar(&cfg.Query, "q", "", "search query mode")
	flag.StringVar(&cfg.IgnoreCluster, "ignore", "", "mark a cluster as ignored by index (1-1000) or cluster ID (SHA)")
	flag.BoolVar(&cfg.PrintIgnore, "print-ignore", false, "print resolved ignore patterns and exit")
	flag.BoolVar(&cfg.PrintFiles, "print-files", false, "print scanned files and exit")
	flag.BoolVar(&cfg.PrintChunks, "print-chunks", false, "print chunks and exit")
	flag.StringVar(&cfg.PrintChunk, "print-chunk", "", "print a specific chunk by SHA (partial or full) and exit")
	flag.StringVar(&cfg.PrintCluster, "print-cluster", "", "print a specific cluster by cluster ID (partial or full) and exit")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "enable verbose progress output")
	flag.Float64Var(&cfg.LocalRefreezeThreshold, "refreeze-threshold", 0.20, "auto-refreeze vocabulary when new chunks exceed this fraction of corpus (0.0-1.0)")
	flag.BoolVar(&cfg.LocalRefreeze, "refreeze", false, "manually trigger vocabulary refreeze and re-embed all chunks")
	flag.BoolVar(&cfg.NoDefaultIgnore, "no-default-ignore", false, "skip default ignore patterns, use only user/project ignore files")

	flag.Parse()

	// Handle version flag
	if showVersion {
		fmt.Printf("redunce version %s\n", Version)
		os.Exit(0)
	}

	// Get remaining arguments as files/directories
	args := flag.Args()

	// Handle install-claude-commands flag
	if installClaudeCommands {
		// Check if a directory was provided as an argument
		targetDir := "~"
		if len(args) > 0 {
			targetDir = args[0]
		}

		if err := installClaudeCommandsToDir(targetDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to install Claude commands: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if cfg.PrintIgnore {
		// Use current directory if no args provided
		baseDir := "."
		if len(args) > 0 {
			baseDir = args[0]
		}
		absDir, err := filepath.Abs(baseDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to get absolute path: %v\n", err)
			os.Exit(1)
		}
		patterns, err := scanner.GetResolvedIgnorePatterns(absDir, cfg.NoDefaultIgnore)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to get ignore patterns: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("# Resolved ignore patterns (combined from all sources)")
		if cfg.NoDefaultIgnore {
			fmt.Println("# Order: user global -> .gitignore -> .redunceignore (no defaults)")
		} else {
			fmt.Println("# Order: embedded defaults -> user global -> .gitignore -> .redunceignore")
		}
		fmt.Printf("# Base directory: %s\n\n", absDir)
		for _, pattern := range patterns {
			fmt.Println(pattern)
		}
		return
	}

	// Handle --print-files flag
	if cfg.PrintFiles {
		if len(args) == 0 {
			args = []string{"."}
		}
		files, err := scanner.ScanPaths(args, cfg.NoDefaultIgnore)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to scan paths: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("# Scanned files (%d total)\n\n", len(files))
		for _, file := range files {
			fmt.Println(file)
		}
		return
	}

	// Handle --print-chunks flag
	if cfg.PrintChunks {
		if len(args) == 0 {
			args = []string{"."}
		}
		files, err := scanner.ScanPaths(args, cfg.NoDefaultIgnore)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to scan paths: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("# Processing %d files\n\n", len(files))
		totalChunks := 0
		for _, file := range files {
			chunks, err := chunkFile(cfg, file)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to chunk %s: %v\n", file, err)
				continue
			}
			for _, chunk := range chunks {
				totalChunks++
				fmt.Printf("## Chunk %d\n", totalChunks)
				fmt.Printf("File: %s\n", chunk.Path)
				fmt.Printf("Language: %s\n", chunk.Language)
				fmt.Printf("Lines: %d-%d\n", chunk.StartLine, chunk.EndLine)
				fmt.Printf("```%s\n%s\n```\n\n", chunk.Language, chunk.Code)
			}
		}
		fmt.Printf("# Total chunks: %d\n", totalChunks)
		return
	}

	// Handle --ignore flag
	if cfg.IgnoreCluster != "" {
		// Open database
		db, err := store.NewStore(cfg.DBPath, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()

		var targetChunk *store.Chunk

		// Check if the input is a small integer (cluster index)
		if clusterIndex, err := strconv.Atoi(cfg.IgnoreCluster); err == nil && clusterIndex > 0 && clusterIndex <= MaxClusterIndex {
			// Treat as cluster index - load existing clusters
			clusters, err := cluster.LoadClusters(db, cluster.ScoreStrategy(cfg.Score))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to load clusters: %v\n", err)
				os.Exit(1)
			}

			if len(clusters) == 0 {
				fmt.Fprintf(os.Stderr, "Error: no clusters found\n")
				os.Exit(1)
			}

			if clusterIndex > len(clusters) {
				fmt.Fprintf(os.Stderr, "Error: cluster index %d out of range (only %d clusters found)\n", clusterIndex, len(clusters))
				os.Exit(1)
			}

			// Use the nth cluster (1-indexed)
			targetCluster := clusters[clusterIndex-1]
			targetChunk = targetCluster.CanonicalChunk
			fmt.Printf("Ignoring cluster %d: %s\n", clusterIndex, util.ShortSHA(targetChunk.SHA))
		} else {
			// Treat as SHA (partial or full)
			chunks, err := db.GetAllChunks()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to get chunks: %v\n", err)
				os.Exit(1)
			}

			matches := findChunkBySHAPrefix(chunks, cfg.IgnoreCluster)
			if len(matches) == 0 {
				fmt.Fprintf(os.Stderr, "Error: cluster ID %s not found\n", cfg.IgnoreCluster)
				os.Exit(1)
			}
			if len(matches) > 1 {
				fmt.Fprintf(os.Stderr, "Error: ambiguous cluster ID %s matches %d clusters:\n", cfg.IgnoreCluster, len(matches))
				for _, match := range matches {
					fmt.Fprintf(os.Stderr, "  %s\n", util.ShortSHA(match.SHA))
				}
				fmt.Fprintf(os.Stderr, "Please provide a longer prefix.\n")
				os.Exit(1)
			}
			targetChunk = matches[0]
		}

		// Add to ignore list
		err = db.AddIgnore(targetChunk.SHA, targetChunk.Code, targetChunk.Embedding)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to add ignore: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Cluster %s has been marked as ignored.\n", util.ShortSHA(targetChunk.SHA))
		return
	}

	// Handle --print-chunk flag
	if cfg.PrintChunk != "" {
		db, err := store.NewStore(cfg.DBPath, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()

		chunks, err := db.GetAllChunks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to get chunks: %v\n", err)
			os.Exit(1)
		}

		matches := findChunkBySHAPrefix(chunks, cfg.PrintChunk)
		if len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "Error: chunk %s not found\n", cfg.PrintChunk)
			os.Exit(1)
		}
		if len(matches) > 1 {
			fmt.Fprintf(os.Stderr, "Error: ambiguous chunk ID %s matches %d chunks:\n", cfg.PrintChunk, len(matches))
			for _, match := range matches {
				fmt.Fprintf(os.Stderr, "  %s (%s:%d-%d)\n", util.ShortSHA(match.SHA), match.Path, match.StartLine, match.EndLine)
			}
			fmt.Fprintf(os.Stderr, "Please provide a longer prefix.\n")
			os.Exit(1)
		}
		chunk := matches[0]
		fmt.Printf("# Chunk: %s\n\n", util.ShortSHA(chunk.SHA))
		fmt.Printf("**Path**: `%s`  \n", chunk.Path)
		fmt.Printf("**Language**: %s  \n", chunk.Language)
		fmt.Printf("**Lines**: %d-%d\n\n", chunk.StartLine, chunk.EndLine)
		fmt.Printf("```%s\n%s\n```\n", chunk.Language, chunk.Code)
		return
	}

	// Handle --print-cluster flag
	if cfg.PrintCluster != "" {
		db, err := store.NewStore(cfg.DBPath, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()

		clusters, err := cluster.LoadClusters(db, cluster.ScoreStrategy(cfg.Score))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load clusters: %v\n", err)
			os.Exit(1)
		}

		if len(clusters) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no clusters found\n")
			os.Exit(1)
		}

		matches := findClusterBySHAPrefix(clusters, cfg.PrintCluster)
		if len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "Error: cluster %s not found\n", cfg.PrintCluster)
			os.Exit(1)
		}
		if len(matches) > 1 {
			fmt.Fprintf(os.Stderr, "Error: ambiguous cluster ID %s matches %d clusters:\n", cfg.PrintCluster, len(matches))
			for _, match := range matches {
				fmt.Fprintf(os.Stderr, "  %s\n", util.ShortSHA(match.ClusterID))
			}
			fmt.Fprintf(os.Stderr, "Please provide a longer prefix.\n")
			os.Exit(1)
		}

		formatter, err := createFormatter(cfg.Format)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		err = formatter.Format(os.Stdout, matches)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to format output: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// If no arguments and no query, print usage
	if len(args) == 0 && cfg.Query == "" {
		printUsage()
		os.Exit(1)
	}

	// Get OpenAI API key from env if not provided
	if cfg.OpenAIAPIKey == "" {
		cfg.OpenAIAPIKey = os.Getenv("OPENAI_API_KEY")
	}

	if cfg.EmbedMethod == "openai" && cfg.OpenAIAPIKey == "" {
		fmt.Fprintf(os.Stderr, "Error: OpenAI API key required. Set --openai-api-key or OPENAI_API_KEY env var\n")
		os.Exit(1)
	}

	// Run the tool
	if err := run(cfg, args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func createFormatter(format string) (output.Formatter, error) {
	switch format {
	case "json":
		return output.NewJSONFormatter(), nil
	case "md":
		return output.NewMarkdownFormatter(), nil
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
}

func run(cfg Config, paths []string) error {
	// Set global verbose flag
	verbose = cfg.Verbose

	// Initialize database
	db, err := store.NewStore(cfg.DBPath, cfg.Reset)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	// Handle query mode
	if cfg.Query != "" {
		return handleQuery(cfg, db)
	}

	// Scan files
	logVerbose("Scanning files...\n")
	files, err := scanner.ScanPaths(paths, cfg.NoDefaultIgnore)
	if err != nil {
		return fmt.Errorf("failed to scan paths: %w", err)
	}
	logVerbose("Found %d files\n", len(files))

	// Process files based on embed method
	var totalChunks int
	var newChunkIDs []int64
	switch cfg.EmbedMethod {
	case "openai":
		emb := embedder.NewOpenAIEmbedder(cfg.OpenAIAPIKey)
		totalChunks, newChunkIDs, err = processFilesStreaming(cfg, db, files, emb)
		if err != nil {
			return err
		}
	case "local":
		emb := embedder.NewTFIDFEmbedder(db, store.EmbeddingDimension, cfg.LocalRefreezeThreshold, cfg.LocalRefreeze)
		totalChunks, newChunkIDs, err = processFiles(cfg, db, files, emb)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown embed method: %s", cfg.EmbedMethod)
	}

	logVerbose("Total new chunks: %d\n", totalChunks)

	// Cluster new chunks incrementally
	logVerbose("Clustering %d new chunks...\n", len(newChunkIDs))
	clustered, err := cluster.ClusterNewChunks(db, newChunkIDs, cfg.Threshold, cfg.IgnoreThreshold, cfg.Verbose)
	if err != nil {
		return fmt.Errorf("failed to cluster chunks: %w", err)
	}
	logVerbose("Clustered %d new chunks\n", clustered)

	// Load all clusters for output
	clusters, err := cluster.LoadClusters(db, cluster.ScoreStrategy(cfg.Score))
	if err != nil {
		return fmt.Errorf("failed to load clusters: %w", err)
	}
	logVerbose("Total clusters: %d\n", len(clusters))

	// Apply limit if specified
	if cfg.Limit > 0 && len(clusters) > cfg.Limit {
		clusters = clusters[:cfg.Limit]
		logVerbose("Limiting output to top %d clusters\n", cfg.Limit)
	}

	// Output results
	logVerbose("Generating output...\n")
	formatter, err := createFormatter(cfg.Format)
	if err != nil {
		return err
	}

	return formatter.Format(os.Stdout, clusters)
}

func chunkFile(cfg Config, path string) ([]*store.Chunk, error) {
	// Determine language from extension
	ext := filepath.Ext(path)
	lang := chunker.LanguageFromExtension(ext)

	// Apply min-chunk-length override to local-min-lines if set
	minLines := cfg.LocalMinLines
	if cfg.MinChunkLength > 0 {
		minLines = cfg.MinChunkLength
	}

	// Use tree-sitter for supported languages
	// If tree-sitter parsing fails, we return the error (no fallback)
	if lang != "" {
		chunks, err := chunker.ChunkWithTreeSitter(path, lang, cfg.TreesitterMin, cfg.TreesitterMax)
		if err != nil {
			return nil, fmt.Errorf("tree-sitter chunking failed for %s: %w", lang, err)
		}

		// Filter chunks by minimum length if specified
		if cfg.MinChunkLength > 0 {
			filtered := make([]*store.Chunk, 0, len(chunks))
			for _, chunk := range chunks {
				lineCount := chunk.EndLine - chunk.StartLine + 1
				if lineCount >= cfg.MinChunkLength {
					filtered = append(filtered, chunk)
				}
			}
			return filtered, nil
		}

		return chunks, nil
	}

	// Use line-based chunker only for file types we don't have tree-sitter support for
	// This is expected behavior for unsupported extensions
	logVerbose("  Using line-based chunking (no tree-sitter parser for %s)\n", ext)
	return chunker.ChunkByLines(path, minLines, cfg.LocalMaxLines, cfg.LocalStepLines)
}

func handleQuery(cfg Config, db *store.Store) error {
	// Quantize vectors for fast searching (if sqlite-vector is loaded)
	if err := db.QuantizeVectors(); err != nil {
		return fmt.Errorf("failed to quantize vectors: %w", err)
	}

	var queryEmbedding []float64

	switch cfg.EmbedMethod {
	case "openai":
		emb := embedder.NewOpenAIEmbedder(cfg.OpenAIAPIKey)
		queryChunk := &store.Chunk{Code: cfg.Query}
		embeddings, err := emb.EmbedBatch([]*store.Chunk{queryChunk})
		if err != nil {
			return fmt.Errorf("failed to embed query: %w", err)
		}
		queryEmbedding = embeddings[0]

	case "local":
		// For TF-IDF, load the frozen vocabulary from database
		fmt.Println("Loading TF-IDF vocabulary from database...")
		vocab, idf, err := db.LoadVocabulary()
		if err != nil {
			return fmt.Errorf("failed to load vocabulary: %w", err)
		}

		// Create TF-IDF embedder and set vocabulary
		tfidf := embedder.NewTFIDFEmbedder(db, store.EmbeddingDimension, 0, false)
		tfidf.SetVocabulary(vocab, idf)

		fmt.Println("Embedding query...")
		queryChunk := &store.Chunk{Code: cfg.Query}
		embeddings, err := tfidf.EmbedBatch([]*store.Chunk{queryChunk})
		if err != nil {
			return fmt.Errorf("failed to embed query: %w", err)
		}
		queryEmbedding = embeddings[0]

	default:
		return fmt.Errorf("unknown embed method: %s", cfg.EmbedMethod)
	}

	// Search for similar chunks
	results, err := db.SearchSimilar(queryEmbedding, cluster.DefaultSimilarChunksLimit, cfg.Threshold)
	if err != nil {
		return fmt.Errorf("failed to search: %w", err)
	}

	// Output results
	formatter, err := createFormatter(cfg.Format)
	if err != nil {
		return err
	}

	// Convert results to clusters format for output
	clusters := []*cluster.Cluster{}
	if len(results) > 0 {
		cl := &cluster.Cluster{
			CanonicalChunk: results[0],
			Chunks:         results,
			AvgSimilarity:  0,
			MaxSimilarity:  0,
		}
		// Calculate similarities
		for _, r := range results {
			if r.Similarity > cl.MaxSimilarity {
				cl.MaxSimilarity = r.Similarity
			}
			cl.AvgSimilarity += r.Similarity
		}
		if len(results) > 0 {
			cl.AvgSimilarity /= float64(len(results))
		}
		clusters = append(clusters, cl)
	}

	// Apply limit if specified
	if cfg.Limit > 0 && len(clusters) > cfg.Limit {
		clusters = clusters[:cfg.Limit]
	}

	return formatter.Format(os.Stdout, clusters)
}

func processFilesStreaming(cfg Config, db *store.Store, files []string, emb embedder.Embedder) (int, []int64, error) {
	fmt.Println("Processing files (streaming mode)...")
	totalChunks := 0
	var newChunkIDs []int64

	for i, file := range files {
		fmt.Printf("Processing [%d/%d]: %s\n", i+1, len(files), file)

		// Check if file needs updating
		needsUpdate, err := db.NeedsUpdate(file)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to check if file needs update: %w", err)
		}

		if !needsUpdate {
			fmt.Printf("  Skipping (up to date)\n")
			continue
		}

		// Delete old chunks for this file
		if err := db.DeleteChunksForFile(file); err != nil {
			return 0, nil, fmt.Errorf("failed to delete old chunks: %w", err)
		}

		// Chunk the file
		chunks, err := chunkFile(cfg, file)
		if err != nil {
			fmt.Printf("  Warning: failed to chunk file: %v\n", err)
			continue
		}

		if len(chunks) == 0 {
			continue
		}

		// Embed chunks in batches
		for j := 0; j < len(chunks); j += BatchSize {
			end := j + BatchSize
			if end > len(chunks) {
				end = len(chunks)
			}
			batch := chunks[j:end]

			embeddings, err := emb.EmbedBatch(batch)
			if err != nil {
				return 0, nil, fmt.Errorf("failed to embed chunks: %w", err)
			}

			// Store chunks with embeddings
			for k, chunk := range batch {
				chunk.Embedding = embeddings[k]
				if err := db.InsertChunk(chunk); err != nil {
					return 0, nil, fmt.Errorf("failed to insert chunk: %w", err)
				}
				newChunkIDs = append(newChunkIDs, chunk.ID)
			}
		}

		totalChunks += len(chunks)
		fmt.Printf("  Created %d chunks\n", len(chunks))
	}

	return totalChunks, newChunkIDs, nil
}

func processFiles(cfg Config, db *store.Store, files []string, emb embedder.Embedder) (int, []int64, error) {
	logVerbose("Processing files...\n")

	// Step 1: Chunk new files
	logVerbose("Step 1: Chunking files...\n")
	var allNewChunks []*store.Chunk
	var filesToUpdate []string

	for i, file := range files {
		logVerbose("Chunking [%d/%d]: %s\n", i+1, len(files), file)

		// Check if file needs updating
		needsUpdate, err := db.NeedsUpdate(file)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to check if file needs update: %w", err)
		}

		if !needsUpdate {
			logVerbose("  Skipping (up to date)\n")
			continue
		}

		// Chunk the file
		chunks, err := chunkFile(cfg, file)
		if err != nil {
			logVerbose("  Warning: failed to chunk file: %v\n", err)
			continue
		}

		if len(chunks) == 0 {
			continue
		}

		allNewChunks = append(allNewChunks, chunks...)
		filesToUpdate = append(filesToUpdate, file)
		logVerbose("  Created %d chunks\n", len(chunks))
	}

	if len(allNewChunks) == 0 {
		logVerbose("No new chunks to process\n")
		return 0, nil, nil
	}

	// Step 2: Prepare embedder and decide whether to re-embed all chunks
	logVerbose("Step 2: Preparing embedder...\n")
	shouldReembedAll, err := emb.PrepareEmbed(allNewChunks)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to prepare embedder: %w", err)
	}

	var chunksToEmbed []*store.Chunk

	if shouldReembedAll {
		// REEMBED ALL: Delete everything and re-embed all chunks
		logVerbose("Re-embedding all chunks\n")

		// Delete clusters (will be rebuilt later)
		if err := db.DeleteAllClusters(); err != nil {
			return 0, nil, fmt.Errorf("failed to delete clusters: %w", err)
		}

		// Load existing chunks
		existingChunks, err := db.GetAllChunks()
		if err != nil {
			return 0, nil, fmt.Errorf("failed to get existing chunks: %w", err)
		}
		logVerbose("Found %d existing chunks\n", len(existingChunks))

		// Delete all chunks (CASCADE handles cluster memberships)
		if err := db.DeleteAllChunks(); err != nil {
			return 0, nil, fmt.Errorf("failed to delete all chunks: %w", err)
		}

		// Combine existing and new chunks for re-embedding
		chunksToEmbed = append(existingChunks, allNewChunks...)
		logVerbose("Re-embedding %d total chunks...\n", len(chunksToEmbed))

	} else {
		// INCREMENTAL: Embed only new chunks
		logVerbose("Embedding only new chunks\n")

		// Delete old chunks for updated files
		for _, file := range filesToUpdate {
			if err := db.DeleteChunksForFile(file); err != nil {
				return 0, nil, fmt.Errorf("failed to delete old chunks: %w", err)
			}
		}

		chunksToEmbed = allNewChunks
		logVerbose("Embedding %d new chunks...\n", len(chunksToEmbed))
	}

	// Step 3: Embed chunks in batches and collect new chunk IDs
	var newChunkIDs []int64
	for i := 0; i < len(chunksToEmbed); i += BatchSize {
		end := i + BatchSize
		if end > len(chunksToEmbed) {
			end = len(chunksToEmbed)
		}
		batch := chunksToEmbed[i:end]

		embeddings, err := emb.EmbedBatch(batch)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to embed chunks: %w", err)
		}

		for j, chunk := range batch {
			chunk.Embedding = embeddings[j]
			if err := db.InsertChunk(chunk); err != nil {
				return 0, nil, fmt.Errorf("failed to insert chunk: %w", err)
			}
			newChunkIDs = append(newChunkIDs, chunk.ID)
		}

		logVerbose("  Embedded and stored %d/%d chunks\n", end, len(chunksToEmbed))
	}

	// Step 4: If we re-embedded all chunks, also re-embed ignored chunks
	if shouldReembedAll {
		ignoredChunks, err := db.GetAllIgnores()
		if err != nil {
			return 0, nil, fmt.Errorf("failed to get ignored chunks: %w", err)
		}

		if len(ignoredChunks) > 0 {
			logVerbose("Re-embedding %d ignored chunks...\n", len(ignoredChunks))

			for i := 0; i < len(ignoredChunks); i += BatchSize {
				end := i + BatchSize
				if end > len(ignoredChunks) {
					end = len(ignoredChunks)
				}
				batch := ignoredChunks[i:end]

				embeddings, err := emb.EmbedBatch(batch)
				if err != nil {
					return 0, nil, fmt.Errorf("failed to embed ignored chunks: %w", err)
				}

				// Update embeddings in the ignores table
				for j, chunk := range batch {
					chunk.Embedding = embeddings[j]
					// Remove and re-add the ignore with new embedding
					if err := db.RemoveIgnore(chunk.SHA); err != nil {
						return 0, nil, fmt.Errorf("failed to remove old ignore: %w", err)
					}
					if err := db.AddIgnore(chunk.SHA, chunk.Code, chunk.Embedding); err != nil {
						return 0, nil, fmt.Errorf("failed to update ignored chunk: %w", err)
					}
				}

				logVerbose("  Re-embedded %d/%d ignored chunks\n", end, len(ignoredChunks))
			}
		}
	}

	logVerbose("Processing complete\n")
	return len(allNewChunks), newChunkIDs, nil
}

func installClaudeCommandsToDir(baseDir string) error {
	// Expand ~ to home directory
	if baseDir == "~" || baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = homeDir
	}

	// Convert to absolute path
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Create <baseDir>/.claude/commands directory
	targetDir := filepath.Join(absBaseDir, ".claude", "commands")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	// Read embedded command files
	entries, err := claudeCommands.ReadDir("claude/commands")
	if err != nil {
		return fmt.Errorf("failed to read embedded commands: %w", err)
	}

	installed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		sourcePath := filepath.Join("claude/commands", filename)
		targetPath := filepath.Join(targetDir, filename)

		// Read embedded file
		content, err := claudeCommands.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", filename, err)
		}

		// Write to target
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", targetPath, err)
		}

		fmt.Printf("Installed: %s\n", targetPath)
		installed++
	}

	if installed == 0 {
		return fmt.Errorf("no command files found")
	}

	// Determine scope for user message
	homeDir, _ := os.UserHomeDir()
	isUserLevel := targetDir == filepath.Join(homeDir, ".claude", "commands")

	scope := "project"
	availability := "in this project"
	if isUserLevel {
		scope = "user-level"
		availability = "in any project"
	}

	fmt.Printf("\n✓ Successfully installed %d Claude Code command(s) to %s\n", installed, targetDir)
	fmt.Printf("  Scope: %s (%s)\n", scope, availability)
	fmt.Println("\nAvailable commands:")
	fmt.Println("  /redunce         - Interactive mode with approval at each step")
	fmt.Println("  /redunce-approve - Auto-approve mode for autonomous refactoring")
	fmt.Println("\nFor more information, see: https://github.com/ZachSnow/redunce")

	return nil
}

func printUsage() {
	binName := filepath.Base(os.Args[0])
	fmt.Println("REDUNCE - Find potentially redundant code")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s [options] <file-or-directory...>\n", binName)
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s ./src\n", binName)
	fmt.Printf("  %s --threshold 0.9 --format json ./\n", binName)
	fmt.Printf("  %s --min-chunk-length 5 ./\n", binName)
	fmt.Printf("  %s -q \"error handling\" --format md\n", binName)
	fmt.Printf("  %s --print-chunk a6adf109\n", binName)
	fmt.Printf("  %s --print-cluster 1accbc96\n", binName)
	fmt.Println()
	fmt.Println("Claude Code Integration:")
	fmt.Println("  Install Claude Code slash commands:")
	fmt.Printf("     $ %s --install-claude-commands      # Install to ~ (user-level, all projects)\n", binName)
	fmt.Printf("     $ %s --install-claude-commands .    # Install to . (project-level, this project only)\n", binName)
	fmt.Println()
	fmt.Println("  Then use in Claude Code:")
	fmt.Println("     /redunce          # Interactive mode with approval at each step")
	fmt.Println("     /redunce-approve  # Auto-approve mode for autonomous refactoring")
	fmt.Println()
	fmt.Println("LLM-Assisted Iterative Workflow:")
	fmt.Println("  Use redunce with an LLM (like Claude) to systematically eliminate redundancy:")
	fmt.Println()
	fmt.Println("  1. Find redundant code:")
	fmt.Printf("     $ %s .\n", binName)
	fmt.Println()
	fmt.Println("  2. For each reported cluster, tell your LLM to either:")
	fmt.Println("     a) Refactor the code to eliminate duplication, OR")
	fmt.Println("     b) Mark it as intentional/acceptable:")
	fmt.Printf("        $ %s --ignore 1          # Ignore first cluster\n", binName)
	fmt.Printf("        $ %s --ignore a6adf109  # Ignore by SHA\n", binName)
	fmt.Println()
	fmt.Println("  3. Repeat until no clusters remain:")
	fmt.Printf("     $ %s .\n", binName)
	fmt.Println("     # No clusters found.")
	fmt.Println()
	fmt.Println("  Ignored clusters survive refactoring and re-chunking. Use higher")
	fmt.Println("  --ignore-threshold (default 0.95) to catch renamed/slightly modified")
	fmt.Println("  duplicates that were previously ignored.")
}
