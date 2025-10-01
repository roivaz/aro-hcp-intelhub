package diff

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

type llmClient struct {
	llm *ollama.LLM
	log logr.Logger
}

func newLLMClient(cfg Config, base logr.Logger) (*llmClient, error) {
	if cfg.ModelName == "" {
		return nil, fmt.Errorf("llm model name is required")
	}

	opts := []ollama.Option{
		ollama.WithModel(cfg.ModelName),
		ollama.WithServerURL(cfg.OllamaURL),
		ollama.WithKeepAlive("5m"),
	}

	client, err := ollama.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create ollama client: %w", err)
	}

	return &llmClient{llm: client, log: base}, nil
}

func (c *llmClient) mapChunk(ctx context.Context, doc Document, meta PRMetadata) (string, error) {
	prompt := strings.ReplaceAll(mapPromptTemplate, "{{.PRTitle}}", meta.Title)
	prompt = strings.ReplaceAll(prompt, "{{.FilePath}}", doc.FilePath)
	prompt = strings.ReplaceAll(prompt, "{{.Text}}", doc.Content)

	messages := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: prompt}},
		},
	}

	resp, err := c.llm.GenerateContent(ctx, messages)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty map response")
	}
	return resp.Choices[0].Content, nil
}

func (c *llmClient) reduceSummary(ctx context.Context, summaries []string, meta PRMetadata) (string, error) {
	joined := strings.Join(summaries, "\n")
	prompt := strings.ReplaceAll(reducePromptTemplate, "{{.PRTitle}}", meta.Title)
	prompt = strings.ReplaceAll(prompt, "{{.PRDescription}}", meta.Body)
	prompt = strings.ReplaceAll(prompt, "{{.Text}}", joined)

	messages := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: prompt}},
		},
	}

	resp, err := c.llm.GenerateContent(ctx, messages)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty reduce response")
	}
	return resp.Choices[0].Content, nil
}
