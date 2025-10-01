package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/go-github/v66/github"
	"github.com/spf13/cobra"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/config"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/db"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/docs"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion/embeddings"
)

var rootCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Ingestion CLI (PRs, Docs)",
}

var prsCmd = &cobra.Command{
	Use:   "prs",
	Short: "Ingest merged PRs (cache/process)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ingestion.LoadConfig()
		if err != nil {
			return err
		}

		database, err := db.NewDatabase(db.Config{DSN: cfg.PostgresURL})
		if err != nil {
			return err
		}
		defer database.Close()

		ctx := context.Background()
		if err := database.Bootstrap(ctx, cfg.RecreateMode); err != nil {
			return err
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
		go func() { <-sigs; cancel() }()

		return generator.Run(ctx)
	},
}

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Ingest documentation (Markdown) into vector store",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ingestion.LoadConfig()
		if err != nil {
			return err
		}
		database, err := db.NewDatabase(db.Config{DSN: cfg.PostgresURL})
		if err != nil {
			return err
		}
		defer database.Close()

		// Markdown-aware chunker via langchaingo
		chunker := docs.NewMDChunker(1000, 100)

		ing := docs.Ingester{
			DB:        database.Bun(),
			Client:    embeddings.NewClient(cfg.OllamaURL, cfg.EmbeddingModel),
			Chunker:   chunker,
			Include:   []string{"**/*.md", "**/*.mdx", "README.md"},
			Exclude:   []string{"**/.git/**"},
			MaxFiles:  200,
			MaxChunks: 1500,
			ModelName: cfg.EmbeddingModel,
		}

		// Single repo from ARO_HCP_REPO_PATH for now; can extend to CSV later
		repoName := "Azure/ARO-HCP"
		repos := []docs.RepoSpec{{Name: repoName, Path: cfg.LocalRepoPath}}
		ctx := context.Background()
		return ing.Run(ctx, repos)
	},
}

func main() {
	// Bind config/env for all subcommands
	config.Init(rootCmd)
	rootCmd.AddCommand(prsCmd)
	rootCmd.AddCommand(docsCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("ingest: %v", err)
	}
}
