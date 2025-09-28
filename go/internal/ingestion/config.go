package ingestion

import (
	"strings"
	"time"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/config"
)

type Config struct {
	PostgresURL        string
	OllamaURL          string
	EmbeddingModel     string
	IngestionMode      string
	IngestionLimit     int
	IngestionStartDate *time.Time
	BatchDirection     string
	RecreateMode       string
	RepositoryURL      string
	LocalRepoPath      string
	GitHubToken        string
}

func LoadConfig() (Config, error) {
	cfg := Config{
		PostgresURL:    config.PostgresURL(),
		OllamaURL:      config.OllamaURL(),
		EmbeddingModel: config.EmbeddingModel(),
		IngestionMode:  config.IngestionMode(),
		IngestionLimit: config.IngestionLimit(),
		BatchDirection: strings.ToLower(config.BatchDirection()),
		RecreateMode:   strings.ToLower(config.RecreateMode()),
		RepositoryURL:  "https://github.com/Azure/ARO-HCP",
		LocalRepoPath:  "./ignore/aro-hcp-repo",
		GitHubToken:    "",
	}

	if parsed := parseDate(config.IngestionStartDate()); parsed != nil {
		cfg.IngestionStartDate = parsed
	}

	return cfg, nil
}

func parseDate(value string) *time.Time {
	if value == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return &t
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return &t
	}
	return nil
}
