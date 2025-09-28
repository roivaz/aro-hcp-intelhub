package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/go-github/v66/github"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/config"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/db"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion/embeddings"
)

func main() {
	config.Init(nil)

	cfg, err := ingestion.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	database, err := db.NewDatabase(db.Config{DSN: cfg.PostgresURL})
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	if err := database.Bootstrap(ctx, cfg.RecreateMode); err != nil {
		log.Fatalf("bootstrap database: %v", err)
	}

	repo := db.NewSearchRepository(database)
	embedClient := embeddings.NewClient(cfg.OllamaURL, cfg.EmbeddingModel)
	ghClient := github.NewClient(nil)
	fetcher := ingestion.NewGitHubFetcher(ghClient, "Azure", "ARO-HCP")

	generator := ingestion.NewGenerator(cfg, database, repo, embedClient, fetcher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

	if err := generator.Run(ctx); err != nil {
		log.Fatalf("run ingestion: %v", err)
	}
}
