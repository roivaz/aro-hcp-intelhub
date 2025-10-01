package embeddings

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms/ollama"
)

type Client struct {
	model string
	llm   *ollama.LLM
}

func NewClient(baseURL, model string) *Client {
	opts := []ollama.Option{ollama.WithModel(model)}
	if trimmed := strings.TrimSpace(baseURL); trimmed != "" {
		opts = append(opts, ollama.WithServerURL(trimmed))
	}
	opts = append(opts, ollama.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}))

	llm, err := ollama.New(opts...)
	if err != nil {
		log.Fatalf("create ollama client: %v", err)
	}

	return &Client{
		model: model,
		llm:   llm,
	}
}

func (c *Client) EmbedTexts(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no inputs provided for embedding")
	}
	start := time.Now()
	log.Printf("ollama: embedding %d input(s) with model %s", len(inputs), c.model)

	vectors, err := c.llm.CreateEmbedding(ctx, inputs)
	if err != nil {
		log.Printf("ollama: embedding failed after %s: %v", time.Since(start), err)
		return nil, fmt.Errorf("create embedding: %w", err)
	}

	log.Printf("ollama: embedded %d input(s) in %s", len(vectors), time.Since(start))
	return vectors, nil
}
