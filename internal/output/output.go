package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ZachSnow/redunce/internal/cluster"
	"github.com/ZachSnow/redunce/internal/util"
)

// Formatter formats clusters for output
type Formatter interface {
	Format(w io.Writer, clusters []*cluster.Cluster) error
}

// JSONFormatter outputs clusters as JSON
type JSONFormatter struct{}

type jsonCluster struct {
	ID             int64       `json:"id"`
	ClusterID      string      `json:"cluster_id"`
	AvgSimilarity  float64     `json:"avg_similarity"`
	MaxSimilarity  float64     `json:"max_similarity"`
	Size           int         `json:"size"`
	CanonicalChunk jsonChunk   `json:"canonical_chunk"`
	Chunks         []jsonChunk `json:"chunks"`
}

type jsonChunk struct {
	Path       string  `json:"path"`
	Language   string  `json:"language"`
	StartLine  int     `json:"start_line"`
	EndLine    int     `json:"end_line"`
	Code       string  `json:"code"`
	Similarity float64 `json:"similarity,omitempty"`
}

func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{}
}

func (f *JSONFormatter) Format(w io.Writer, clusters []*cluster.Cluster) error {
	var output []jsonCluster

	for _, cl := range clusters {
		jc := jsonCluster{
			ID:            cl.ID,
			ClusterID:     util.ShortSHA(cl.ClusterID),
			AvgSimilarity: cl.AvgSimilarity,
			MaxSimilarity: cl.MaxSimilarity,
			Size:          len(cl.Chunks),
			CanonicalChunk: jsonChunk{
				Path:      cl.CanonicalChunk.Path,
				Language:  cl.CanonicalChunk.Language,
				StartLine: cl.CanonicalChunk.StartLine,
				EndLine:   cl.CanonicalChunk.EndLine,
				Code:      cl.CanonicalChunk.Code,
			},
		}

		for _, chunk := range cl.Chunks {
			jc.Chunks = append(jc.Chunks, jsonChunk{
				Path:       chunk.Path,
				Language:   chunk.Language,
				StartLine:  chunk.StartLine,
				EndLine:    chunk.EndLine,
				Code:       chunk.Code,
				Similarity: chunk.Similarity,
			})
		}

		output = append(output, jc)
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// MarkdownFormatter outputs clusters as Markdown
type MarkdownFormatter struct{}

func NewMarkdownFormatter() *MarkdownFormatter {
	return &MarkdownFormatter{}
}

func (f *MarkdownFormatter) Format(w io.Writer, clusters []*cluster.Cluster) error {
	if len(clusters) == 0 {
		fmt.Fprintln(w, "No clusters found.")
		return nil
	}

	fmt.Fprintf(w, "# Redundant Code Analysis\n\n")
	fmt.Fprintf(w, "Found %d cluster(s) of similar code.\n\n", len(clusters))

	for i, cl := range clusters {
		fmt.Fprintf(w, "## Cluster %d\n\n", i+1)
		fmt.Fprintf(w, "- **Cluster ID**: `%s`\n", util.ShortSHA(cl.ClusterID))
		fmt.Fprintf(w, "- **Size**: %d chunks\n", len(cl.Chunks))
		fmt.Fprintf(w, "- **Average Similarity**: %.2f%%\n", cl.AvgSimilarity*100)
		fmt.Fprintf(w, "- **Max Similarity**: %.2f%%\n\n", cl.MaxSimilarity*100)

		fmt.Fprintf(w, "### Canonical Chunk\n\n")
		fmt.Fprintf(w, "**Language**: %s  \n", cl.CanonicalChunk.Language)
		fmt.Fprintf(w, "**Path**: `%s`  \n", cl.CanonicalChunk.Path)
		fmt.Fprintf(w, "**Lines**: %d-%d\n\n", cl.CanonicalChunk.StartLine, cl.CanonicalChunk.EndLine)
		fmt.Fprintf(w, "```%s\n%s\n```\n\n", cl.CanonicalChunk.Language, cl.CanonicalChunk.Code)

		if len(cl.Chunks) > 1 {
			fmt.Fprintf(w, "### Similar Chunks\n\n")
			for j, chunk := range cl.Chunks {
				// Skip canonical chunk (first one)
				if j == 0 {
					continue
				}

				fmt.Fprintf(w, "#### Chunk %d (Similarity: %.2f%%)\n\n", j, chunk.Similarity*100)
				fmt.Fprintf(w, "**Language**: %s  \n", chunk.Language)
				fmt.Fprintf(w, "**Path**: `%s`  \n", chunk.Path)
				fmt.Fprintf(w, "**Lines**: %d-%d\n\n", chunk.StartLine, chunk.EndLine)
				fmt.Fprintf(w, "```%s\n%s\n```\n\n", chunk.Language, chunk.Code)
			}
		}

		fmt.Fprintln(w, "---\n")
	}

	return nil
}
