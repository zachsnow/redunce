package embedder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ZachSnow/redunce/internal/store"
)

// Embedder is an interface for embedding text
type Embedder interface {
	EmbedBatch(chunks []*store.Chunk) ([][]float64, error)
}

// OpenAIEmbedder uses OpenAI's API for embeddings
type OpenAIEmbedder struct {
	apiKey string
	client *http.Client
}

type openAIRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// NewOpenAIEmbedder creates a new OpenAI embedder
func NewOpenAIEmbedder(apiKey string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// EmbedBatch embeds a batch of chunks using OpenAI
func (e *OpenAIEmbedder) EmbedBatch(chunks []*store.Chunk) ([][]float64, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	// Prepare input texts
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		// Preprocess code to remove noise
		cleanedCode := preprocessCode(chunk.Code)
		// Format chunk for embedding
		text := fmt.Sprintf("language: %s\npath: %s\nlines: %d-%d\n\n%s",
			chunk.Language, chunk.Path, chunk.StartLine, chunk.EndLine, cleanedCode)
		texts[i] = text
	}

	// Create request
	reqBody := openAIRequest{
		Input:          texts,
		Model:          "text-embedding-3-small",
		EncodingFormat: "float",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make API request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract embeddings in the correct order
	embeddings := make([][]float64, len(chunks))
	for _, data := range apiResp.Data {
		if data.Index >= len(embeddings) {
			return nil, fmt.Errorf("invalid index %d in response", data.Index)
		}
		embeddings[data.Index] = data.Embedding
	}

	return embeddings, nil
}

// preprocessCode cleans code by removing empty lines and lines with only braces
func preprocessCode(code string) string {
	lines := strings.Split(code, "\n")
	var cleaned []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines and lines containing only { or }
		if trimmed == "" || trimmed == "{" || trimmed == "}" {
			continue
		}
		cleaned = append(cleaned, line)
	}

	return strings.Join(cleaned, "\n")
}
