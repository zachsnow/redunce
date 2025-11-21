package embedder

import (
	"math"
	"regexp"
	"strings"

	"github.com/ZachSnow/redunce/internal/store"
)

// TFIDFEmbedder uses TF-IDF for local embeddings
type TFIDFEmbedder struct {
	vocabulary map[string]int // word -> index
	idf        []float64      // inverse document frequency for each term
	vectorDim  int
	tokenizer  *regexp.Regexp
}

// NewTFIDFEmbedder creates a new TF-IDF embedder
func NewTFIDFEmbedder(vectorDim int) *TFIDFEmbedder {
	return &TFIDFEmbedder{
		vocabulary: make(map[string]int),
		vectorDim:  vectorDim,
		// Split on non-alphanumeric characters, keeping underscores
		tokenizer: regexp.MustCompile(`[^a-zA-Z0-9_]+`),
	}
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
		tokens := e.tokenize(chunk.Code)
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

	// Simple bubble sort by frequency (descending)
	for i := 0; i < len(terms)-1; i++ {
		for j := i + 1; j < len(terms); j++ {
			if terms[j].freq > terms[i].freq {
				terms[i], terms[j] = terms[j], terms[i]
			}
		}
	}

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
	tokens := e.tokenize(text)

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

	// Create TF-IDF vector
	vector := make([]float64, len(e.idf))
	for token, freq := range tf {
		if idx, ok := e.vocabulary[token]; ok {
			vector[idx] = freq * e.idf[idx]
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
