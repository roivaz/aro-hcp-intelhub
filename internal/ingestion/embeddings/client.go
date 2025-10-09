package embeddings

import (
	"context"
	"errors"
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
	to    time.Duration
}

func NewClient(baseURL, model string, timeout time.Duration) *Client {
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
		to:    timeout,
	}
}

func (c *Client) EmbedTexts(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no inputs provided for embedding")
	}
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	start := time.Now()
	log.Printf("ollama: embedding %d input(s) with model %s", len(inputs), c.model)

	vectors, err := c.llm.CreateEmbedding(ctx, inputs)
	if err != nil {
		annotated := c.annotateError(err)
		log.Printf("ollama: embedding failed after %s: %v", time.Since(start), annotated)
		return nil, fmt.Errorf("create embedding: %w", annotated)
	}

	log.Printf("ollama: embedded %d input(s) in %s", len(vectors), time.Since(start))
	return vectors, nil
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.to <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.to)
}

func (c *Client) annotateError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("embedding call timed out after %s: %w", c.to, err)
	}
	return err
}
