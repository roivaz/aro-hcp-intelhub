package ingestion

import (
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/config"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion/diff"
)

type Config struct {
	PostgresURL          string
	OllamaURL            string
	EmbeddingModel       string
	GitHubFetchMax       int        // Maximum PRs to fetch from GitHub per run
	GitHubFetchStartDate *time.Time // Start date for fetching PRs from GitHub (used when DB is empty)
	RecreateMode         string
	ExecutionMode        string // FULL, CACHE, or PROCESS
	MaxProcessBatch      int    // Maximum PRs to process from DB per run
	DiffAnalyzer         diff.Config
	RepositoryURL        string
	LocalRepoPath        string
	GitHubToken          string
}

func LoadConfig() (Config, error) {
	cfg := Config{
		PostgresURL:     config.PostgresURL(),
		OllamaURL:       config.OllamaURL(),
		EmbeddingModel:  config.EmbeddingModel(),
		GitHubFetchMax:  config.GitHubFetchMax(),
		RecreateMode:    strings.ToLower(config.RecreateMode()),
		ExecutionMode:   strings.ToUpper(config.ExecutionMode()),
		MaxProcessBatch: config.MaxProcessBatch(),
		DiffAnalyzer: diff.Config{
			Enabled:          config.DiffAnalysisEnabled(),
			ModelName:        config.DiffAnalysisModel(),
			OllamaURL:        config.DiffAnalysisOllamaURL(),
			RepoPath:         config.RepoPath(),
			MaxContextTokens: config.DiffAnalysisContextTokens(),
			Logger:           logr.Logger{},
		},
		RepositoryURL: "https://github.com/Azure/ARO-HCP",
		LocalRepoPath: config.RepoPath(),
		GitHubToken:   "",
	}

	if parsed := parseDate(config.GitHubFetchStartDate()); parsed != nil {
		cfg.GitHubFetchStartDate = parsed
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
