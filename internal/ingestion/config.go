package ingestion

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/roivaz/aro-hcp-intelhub/internal/config"
	"github.com/roivaz/aro-hcp-intelhub/internal/ingestion/diff"
)

type Config struct {
	PostgresURL     string
	OllamaURL       string
	EmbeddingModel  string
	GitHubFetchMax  int    // Maximum PRs to fetch from GitHub per run
	ExecutionMode   string // FULL, CACHE, or PROCESS
	MaxProcessBatch int    // Maximum PRs to process from DB per run
	DiffAnalyzer    diff.Config
	RepositoryURL   string
	LocalRepoPath   string
	GitHubToken     string
	AutoMigrate     bool
	LLMCallTimeout  time.Duration
	RetryFailed     bool // Retry diff analysis on previously failed PRs
}

func LoadConfig() (Config, error) {
	cfg := Config{
		PostgresURL:     config.PostgresURL(),
		OllamaURL:       config.OllamaURL(),
		EmbeddingModel:  config.EmbeddingModel(),
		GitHubFetchMax:  config.GitHubFetchMax(),
		ExecutionMode:   strings.ToUpper(config.ExecutionMode()),
		MaxProcessBatch: config.MaxProcessBatch(),
		DiffAnalyzer: diff.Config{
			Enabled:          config.DiffAnalysisEnabled(),
			ModelName:        config.DiffAnalysisModel(),
			OllamaURL:        config.DiffAnalysisOllamaURL(),
			RepoPath:         filepath.Join(config.CacheDir(), "aro-hcp-repo"),
			MaxContextTokens: config.DiffAnalysisContextTokens(),
			Logger:           logr.Logger{},
		},
		RepositoryURL: "https://github.com/Azure/ARO-HCP",
		LocalRepoPath: filepath.Join(config.CacheDir(), "aro-hcp-repo"),
		GitHubToken:   "",
		AutoMigrate:   config.AutoMigrate(),
	}

	timeout, err := parseDuration(config.LLMCallTimeout(), 2*time.Minute)
	if err != nil {
		return Config{}, fmt.Errorf("invalid llm_call_timeout: %w", err)
	}
	cfg.LLMCallTimeout = timeout
	cfg.DiffAnalyzer.CallTimeout = timeout

	return cfg, nil
}

func parseDuration(value string, fallback time.Duration) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, err
	}
	return d, nil
}
