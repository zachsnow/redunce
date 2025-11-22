package embedder

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/ZachSnow/redunce/internal/store"
)

// TFIDFEmbedder uses TF-IDF for local embeddings
type TFIDFEmbedder struct {
	db                *store.Store
	vocabulary        map[string]int // word -> index
	idf               []float64      // inverse document frequency for each term
	vectorDim         int
	refreezeThreshold float64
	manualRefreeze    bool
	tokenizer         *regexp.Regexp
}

// NewTFIDFEmbedder creates a new TF-IDF embedder
func NewTFIDFEmbedder(db *store.Store, vectorDim int, refreezeThreshold float64, manualRefreeze bool) *TFIDFEmbedder {
	return &TFIDFEmbedder{
		db:                db,
		vocabulary:        make(map[string]int),
		vectorDim:         vectorDim,
		refreezeThreshold: refreezeThreshold,
		manualRefreeze:    manualRefreeze,
		// Split on non-alphanumeric characters, keeping underscores
		tokenizer: regexp.MustCompile(`[^a-zA-Z0-9_]+`),
	}
}

// PrepareEmbed prepares the TF-IDF embedder by deciding whether to rebuild vocabulary
// and re-embed all chunks, or to use the frozen vocabulary for only new chunks.
// Returns true if all chunks should be re-embedded.
func (e *TFIDFEmbedder) PrepareEmbed(newChunks []*store.Chunk) (bool, error) {
	shouldRefreeze := e.manualRefreeze
	reason := "manual"

	if !shouldRefreeze {
		hasVocab, err := e.db.HasVocabulary()
		if err != nil {
			return false, fmt.Errorf("failed to check vocabulary existence: %w", err)
		}

		if !hasVocab {
			shouldRefreeze = true
			reason = "initial"
		} else {
			// Check auto-refreeze threshold
			autoRefreeze, newChunkRatio, err := e.db.ShouldRefreeze(e.refreezeThreshold)
			if err != nil {
				return false, fmt.Errorf("failed to check refreeze condition: %w", err)
			}
			if autoRefreeze {
				shouldRefreeze = true
				reason = fmt.Sprintf("%.1f%% corpus growth", newChunkRatio*100)
			}
		}
	}

	if shouldRefreeze {
		// REFREEZE: Build vocabulary from all chunks
		existingChunks, err := e.db.GetAllChunks()
		if err != nil {
			return false, fmt.Errorf("failed to get existing chunks: %w", err)
		}

		allChunks := append(existingChunks, newChunks...)
		e.buildVocabulary(allChunks)

		// Save vocabulary to database
		if err := e.db.SaveVocabulary(e.vocabulary, e.idf); err != nil {
			return false, fmt.Errorf("failed to save vocabulary: %w", err)
		}

		// Update freeze metadata
		if err := e.db.SetFreezeMetadata(len(allChunks), reason); err != nil {
			return false, fmt.Errorf("failed to set freeze metadata: %w", err)
		}

		return true, nil
	}

	// FROZEN: Load existing vocabulary
	vocab, idf, err := e.db.LoadVocabulary()
	if err != nil {
		return false, fmt.Errorf("failed to load vocabulary: %w", err)
	}
	e.vocabulary = vocab
	e.idf = idf

	return false, nil
}

// tokenize splits text into tokens
func (e *TFIDFEmbedder) tokenize(text string) []string {
	// Convert to lowercase and split
	text = strings.ToLower(text)
	parts := e.tokenizer.Split(text, -1)

	// Filter out empty strings and very short tokens
	var tokens []string
	for _, token := range parts {
		token = strings.TrimSpace(token)
		if len(token) >= 2 { // Keep tokens of at least 2 chars
			tokens = append(tokens, token)
		}
	}

	return tokens
}

// buildVocabulary builds vocabulary and computes IDF from all chunks
func (e *TFIDFEmbedder) buildVocabulary(chunks []*store.Chunk) {
	// Count document frequency for each term
	df := make(map[string]int)
	totalDocs := len(chunks)

	for _, chunk := range chunks {
		// Preprocess code to remove noise
		cleanedCode := preprocessCode(chunk.Code)
		tokens := e.tokenize(cleanedCode)
		// Use a set to count each term only once per document
		seen := make(map[string]bool)
		for _, token := range tokens {
			if !seen[token] {
				df[token]++
				seen[token] = true
			}
		}
	}

	// Build vocabulary with most common terms up to vectorDim
	// Sort by document frequency and take top N
	type termFreq struct {
		term string
		freq int
	}

	var terms []termFreq
	for term, freq := range df {
		terms = append(terms, termFreq{term, freq})
	}

	// Sort by frequency (descending)
	sort.Slice(terms, func(i, j int) bool {
		return terms[i].freq > terms[j].freq
	})

	// Take top vectorDim terms
	limit := e.vectorDim
	if limit > len(terms) {
		limit = len(terms)
	}

	// Build vocabulary and IDF
	e.vocabulary = make(map[string]int)
	e.idf = make([]float64, limit)

	for i := 0; i < limit; i++ {
		term := terms[i].term
		e.vocabulary[term] = i
		// IDF = log(N / df)
		e.idf[i] = math.Log(float64(totalDocs) / float64(terms[i].freq))
	}
}

// computeTFIDF computes TF-IDF vector for a single document
func (e *TFIDFEmbedder) computeTFIDF(text string) []float64 {
	// Preprocess code to remove noise
	cleanedText := preprocessCode(text)
	tokens := e.tokenize(cleanedText)

	// Compute term frequency
	tf := make(map[string]float64)
	for _, token := range tokens {
		tf[token]++
	}

	// Normalize by document length
	docLen := float64(len(tokens))
	if docLen > 0 {
		for token := range tf {
			tf[token] /= docLen
		}
	}

	// Create TF-IDF vector with fixed dimension (vectorDim)
	// This ensures compatibility with OpenAI embeddings (1536 dims)
	vector := make([]float64, e.vectorDim)
	for token, freq := range tf {
		if idx, ok := e.vocabulary[token]; ok {
			if idx < e.vectorDim {
				vector[idx] = freq * e.idf[idx]
			}
		}
	}

	// Normalize vector to unit length (for cosine similarity)
	norm := 0.0
	for _, v := range vector {
		norm += v * v
	}
	norm = math.Sqrt(norm)

	if norm > 0 {
		for i := range vector {
			vector[i] /= norm
		}
	}

	return vector
}

// EmbedBatch embeds a batch of chunks using TF-IDF
// Note: TF-IDF requires seeing all documents to compute IDF, so we need to
// either build vocabulary incrementally or in a two-pass approach
func (e *TFIDFEmbedder) EmbedBatch(chunks []*store.Chunk) ([][]float64, error) {
	// If vocabulary not built yet, build it from this batch
	if len(e.vocabulary) == 0 {
		e.buildVocabulary(chunks)
	}

	// Compute TF-IDF vectors for each chunk
	embeddings := make([][]float64, len(chunks))
	for i, chunk := range chunks {
		embeddings[i] = e.computeTFIDF(chunk.Code)
	}

	return embeddings, nil
}

// UpdateVocabulary updates vocabulary with new chunks
// This should be called when adding new files to include their terms
func (e *TFIDFEmbedder) UpdateVocabulary(newChunks []*store.Chunk, existingChunks []*store.Chunk) {
	allChunks := append(existingChunks, newChunks...)
	e.buildVocabulary(allChunks)
}

// SetVocabulary sets a pre-built vocabulary (e.g., loaded from database)
func (e *TFIDFEmbedder) SetVocabulary(vocab map[string]int, idf []float64) {
	e.vocabulary = vocab
	e.idf = idf
}

// GetVocabulary returns the current vocabulary and IDF values
func (e *TFIDFEmbedder) GetVocabulary() (map[string]int, []float64) {
	return e.vocabulary, e.idf
}
