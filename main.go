package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZachSnow/redunce/internal/chunker"
	"github.com/ZachSnow/redunce/internal/cluster"
	"github.com/ZachSnow/redunce/internal/embedder"
	"github.com/ZachSnow/redunce/internal/output"
	"github.com/ZachSnow/redunce/internal/scanner"
	"github.com/ZachSnow/redunce/internal/store"
)

type Config struct {
	Threshold      float64
	EmbedMethod    string
	Format         string
	LocalMinLines  int
	LocalMaxLines  int
	LocalStepLines int
	TreesitterMin  int
	TreesitterMax  int
	OpenAIAPIKey   string
	DBPath         string
	Reset          bool
	Query          string
}

func main() {
	cfg := Config{}

	// Define flags
	flag.Float64Var(&cfg.Threshold, "threshold", 0.8, "similarity threshold (0-1)")
	flag.StringVar(&cfg.EmbedMethod, "embed", "openai", "embedding method: local|openai")
	flag.StringVar(&cfg.Format, "format", "md", "output format: json|md")
	flag.IntVar(&cfg.LocalMinLines, "local-min-lines", 3, "minimum lines per chunk for local")
	flag.IntVar(&cfg.LocalMaxLines, "local-max-lines", 30, "maximum lines per chunk for local")
	flag.IntVar(&cfg.LocalStepLines, "local-step-lines", 3, "line step size for local chunker")
	flag.IntVar(&cfg.TreesitterMin, "treesitter-min", 5, "minimum node size for tree-sitter")
	flag.IntVar(&cfg.TreesitterMax, "treesitter-max", 10, "maximum node size for tree-sitter")
	flag.StringVar(&cfg.OpenAIAPIKey, "openai-api-key", "", "OpenAI API key (or set OPENAI_API_KEY env var)")
	flag.StringVar(&cfg.DBPath, "db", "redunce.db", "database file path")
	flag.BoolVar(&cfg.Reset, "reset", false, "reset database before running")
	flag.StringVar(&cfg.Query, "q", "", "search query mode")

	flag.Parse()

	// Get remaining arguments as files/directories
	args := flag.Args()

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

func run(cfg Config, paths []string) error {
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
	fmt.Println("Scanning files...")
	files, err := scanner.ScanPaths(paths)
	if err != nil {
		return fmt.Errorf("failed to scan paths: %w", err)
	}
	fmt.Printf("Found %d files\n", len(files))

	// Process files based on embed method
	var totalChunks int
	switch cfg.EmbedMethod {
	case "openai":
		emb := embedder.NewOpenAIEmbedder(cfg.OpenAIAPIKey)
		totalChunks, err = processFilesStreaming(cfg, db, files, emb)
		if err != nil {
			return err
		}
	case "local":
		totalChunks, err = processFilesWithTFIDF(cfg, db, files)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown embed method: %s", cfg.EmbedMethod)
	}

	fmt.Printf("Total chunks: %d\n", totalChunks)

	// Cluster similar chunks
	fmt.Println("Clustering similar chunks...")
	clusters, err := cluster.FindClusters(db, cfg.Threshold)
	if err != nil {
		return fmt.Errorf("failed to cluster chunks: %w", err)
	}
	fmt.Printf("Found %d clusters\n", len(clusters))

	// Output results
	fmt.Println("Generating output...")
	var formatter output.Formatter
	switch cfg.Format {
	case "json":
		formatter = output.NewJSONFormatter()
	case "md":
		formatter = output.NewMarkdownFormatter()
	default:
		return fmt.Errorf("unknown format: %s", cfg.Format)
	}

	return formatter.Format(os.Stdout, clusters)
}

func chunkFile(cfg Config, path string) ([]*store.Chunk, error) {
	// Determine language from extension
	ext := filepath.Ext(path)
	lang := chunker.LanguageFromExtension(ext)

	// Use tree-sitter for supported languages
	// If tree-sitter parsing fails, we return the error (no fallback)
	if lang != "" {
		fmt.Printf("  Using tree-sitter chunking for %s\n", lang)
		chunks, err := chunker.ChunkWithTreeSitter(path, lang, cfg.TreesitterMin, cfg.TreesitterMax)
		if err != nil {
			return nil, fmt.Errorf("tree-sitter chunking failed for %s: %w", lang, err)
		}
		return chunks, nil
	}

	// Use line-based chunker only for file types we don't have tree-sitter support for
	// This is not an error - just an expected fallback for unsupported extensions
	fmt.Printf("  Using line-based chunking (no tree-sitter parser available for %s)\n", ext)
	return chunker.ChunkByLines(path, cfg.LocalMinLines, cfg.LocalMaxLines, cfg.LocalStepLines)
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
		// For TF-IDF, we need to build vocabulary from existing chunks
		fmt.Println("Loading existing chunks for TF-IDF...")
		existingChunks, err := db.GetAllChunks()
		if err != nil {
			return fmt.Errorf("failed to get existing chunks: %w", err)
		}
		if len(existingChunks) == 0 {
			return fmt.Errorf("no chunks in database. Please run analysis first before searching")
		}

		fmt.Println("Building TF-IDF vocabulary...")
		tfidf := embedder.NewTFIDFEmbedder(1536)
		tfidf.UpdateVocabulary([]*store.Chunk{}, existingChunks)

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
	results, err := db.SearchSimilar(queryEmbedding, 10, cfg.Threshold)
	if err != nil {
		return fmt.Errorf("failed to search: %w", err)
	}

	// Output results
	var formatter output.Formatter
	switch cfg.Format {
	case "json":
		formatter = output.NewJSONFormatter()
	case "md":
		formatter = output.NewMarkdownFormatter()
	default:
		return fmt.Errorf("unknown format: %s", cfg.Format)
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

	return formatter.Format(os.Stdout, clusters)
}

func processFilesStreaming(cfg Config, db *store.Store, files []string, emb embedder.Embedder) (int, error) {
	fmt.Println("Processing files (streaming mode)...")
	totalChunks := 0

	for i, file := range files {
		fmt.Printf("Processing [%d/%d]: %s\n", i+1, len(files), file)

		// Check if file needs updating
		needsUpdate, err := db.NeedsUpdate(file)
		if err != nil {
			return 0, fmt.Errorf("failed to check if file needs update: %w", err)
		}

		if !needsUpdate {
			fmt.Printf("  Skipping (up to date)\n")
			continue
		}

		// Delete old chunks for this file
		if err := db.DeleteChunksForFile(file); err != nil {
			return 0, fmt.Errorf("failed to delete old chunks: %w", err)
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
		batchSize := 64
		for j := 0; j < len(chunks); j += batchSize {
			end := j + batchSize
			if end > len(chunks) {
				end = len(chunks)
			}
			batch := chunks[j:end]

			embeddings, err := emb.EmbedBatch(batch)
			if err != nil {
				return 0, fmt.Errorf("failed to embed chunks: %w", err)
			}

			// Store chunks with embeddings
			for k, chunk := range batch {
				chunk.Embedding = embeddings[k]
				if err := db.InsertChunk(chunk); err != nil {
					return 0, fmt.Errorf("failed to insert chunk: %w", err)
				}
			}
		}

		totalChunks += len(chunks)
		fmt.Printf("  Created %d chunks\n", len(chunks))
	}

	return totalChunks, nil
}

func processFilesWithTFIDF(cfg Config, db *store.Store, files []string) (int, error) {
	fmt.Println("Processing files (TF-IDF mode)...")

	// Step 0: Delete all existing clusters since we'll be re-embedding with new vocabulary
	// This is necessary to avoid foreign key constraint errors
	fmt.Println("Step 0: Clearing existing clusters...")
	if err := db.DeleteAllClusters(); err != nil {
		return 0, fmt.Errorf("failed to delete existing clusters: %w", err)
	}

	// Step 1: Collect all chunks that need processing
	fmt.Println("Step 1: Chunking files...")
	var allNewChunks []*store.Chunk
	var filesToUpdate []string

	for i, file := range files {
		fmt.Printf("Chunking [%d/%d]: %s\n", i+1, len(files), file)

		// Check if file needs updating
		needsUpdate, err := db.NeedsUpdate(file)
		if err != nil {
			return 0, fmt.Errorf("failed to check if file needs update: %w", err)
		}

		if !needsUpdate {
			fmt.Printf("  Skipping (up to date)\n")
			continue
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

		allNewChunks = append(allNewChunks, chunks...)
		filesToUpdate = append(filesToUpdate, file)
		fmt.Printf("  Created %d chunks\n", len(chunks))
	}

	if len(allNewChunks) == 0 {
		fmt.Println("No new chunks to process")
		return 0, nil
	}

	// Step 2: Get existing chunks for vocabulary building
	fmt.Println("Step 2: Loading existing chunks...")
	existingChunks, err := db.GetAllChunks()
	if err != nil {
		return 0, fmt.Errorf("failed to get existing chunks: %w", err)
	}
	fmt.Printf("Found %d existing chunks\n", len(existingChunks))

	// Step 3: Build TF-IDF vocabulary from all chunks
	fmt.Println("Step 3: Building TF-IDF vocabulary...")
	tfidf := embedder.NewTFIDFEmbedder(1536) // Use same dim as OpenAI for consistency

	// Combine existing and new chunks for vocabulary
	allChunks := append(existingChunks, allNewChunks...)
	tfidf.UpdateVocabulary(allNewChunks, existingChunks)
	fmt.Printf("Built vocabulary with %d chunks\n", len(allChunks))

	// Step 4: If we have existing chunks, we need to re-embed them with new vocabulary
	if len(existingChunks) > 0 {
		fmt.Println("Step 4: Re-embedding existing chunks with updated vocabulary...")
		for i := 0; i < len(existingChunks); i += 100 {
			end := i + 100
			if end > len(existingChunks) {
				end = len(existingChunks)
			}
			batch := existingChunks[i:end]

			embeddings, err := tfidf.EmbedBatch(batch)
			if err != nil {
				return 0, fmt.Errorf("failed to embed existing chunks: %w", err)
			}

			// Update embeddings in database
			for j, chunk := range batch {
				chunk.Embedding = embeddings[j]
				// Delete and re-insert to update embedding
				if err := db.DeleteChunksForFile(chunk.Path); err != nil {
					return 0, fmt.Errorf("failed to delete chunk: %w", err)
				}
			}
		}

		// Re-insert existing chunks with new embeddings
		for i, chunk := range existingChunks {
			if err := db.InsertChunk(chunk); err != nil {
				return 0, fmt.Errorf("failed to re-insert chunk: %w", err)
			}
			if (i+1)%100 == 0 {
				fmt.Printf("  Re-inserted %d/%d chunks\n", i+1, len(existingChunks))
			}
		}
	}

	// Step 5: Embed and store new chunks
	fmt.Println("Step 5: Embedding and storing new chunks...")
	for _, file := range filesToUpdate {
		// Delete old chunks for this file
		if err := db.DeleteChunksForFile(file); err != nil {
			return 0, fmt.Errorf("failed to delete old chunks: %w", err)
		}
	}

	// Find chunks for each file and insert them
	for i := 0; i < len(allNewChunks); i += 100 {
		end := i + 100
		if end > len(allNewChunks) {
			end = len(allNewChunks)
		}
		batch := allNewChunks[i:end]

		embeddings, err := tfidf.EmbedBatch(batch)
		if err != nil {
			return 0, fmt.Errorf("failed to embed new chunks: %w", err)
		}

		for j, chunk := range batch {
			chunk.Embedding = embeddings[j]
			if err := db.InsertChunk(chunk); err != nil {
				return 0, fmt.Errorf("failed to insert chunk: %w", err)
			}
		}

		fmt.Printf("  Embedded and stored %d/%d new chunks\n", end, len(allNewChunks))
	}

	return len(allNewChunks), nil
}

func printUsage() {
	fmt.Println("REDUNCE - Find potentially redundant code")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  redunce [options] <file-or-directory...>")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  redunce ./src")
	fmt.Println("  redunce --threshold 0.9 --format json ./")
	fmt.Println("  redunce -q \"error handling\" --format md")
}
