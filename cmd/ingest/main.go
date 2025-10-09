package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/google/go-github/v66/github"
	"github.com/spf13/cobra"

	"github.com/roivaz/aro-hcp-intelhub/internal/config"
	"github.com/roivaz/aro-hcp-intelhub/internal/db"
	"github.com/roivaz/aro-hcp-intelhub/internal/docs"
	"github.com/roivaz/aro-hcp-intelhub/internal/gitrepo"
	"github.com/roivaz/aro-hcp-intelhub/internal/ingestion"
	"github.com/roivaz/aro-hcp-intelhub/internal/ingestion/embeddings"

	vcsurl "github.com/gitsight/go-vcsurl"
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

		repo := db.NewSearchRepository(database, db.WithTraceCacheMax(config.TraceCacheMaxEntries()))
		embedClient := embeddings.NewClient(cfg.OllamaURL, cfg.EmbeddingModel, cfg.LLMCallTimeout)
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

func newDocsCmd() *cobra.Command {

	var repoURLs []string
	var component string
	var ref string

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Ingest documentation (Markdown) into vector store",
	}

	// Build repo list from flags (repeatable --docs-repo full URL, with optional @ref and #component)
	cmd.Flags().StringArrayVar(&repoURLs, "repo-url", nil, "Repo URL to ingest (repeat)")
	cmd.Flags().StringVar(&component, "component", "", "Component name")
	cmd.Flags().StringVar(&ref, "ref", "HEAD", "Reference name")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
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
			Client:    embeddings.NewClient(cfg.OllamaURL, cfg.EmbeddingModel, cfg.LLMCallTimeout),
			Chunker:   chunker,
			Include:   []string{"**/*.md", "**/*.mdx", "README.md"},
			Exclude:   []string{"**/.git/**"},
			MaxFiles:  200,
			MaxChunks: 1500,
			ModelName: cfg.EmbeddingModel,
		}

		var repos []docs.RepoSpec
		for _, url := range repoURLs {
			surl, err := vcsurl.Parse(url)
			if err != nil {
				log.Fatalf("doesn't look like a VCS URL: %s", err)
			}

			localPath := filepath.Join(config.CacheDir(), surl.Name)
			if _, err := gitrepo.New(gitrepo.RepoConfig{URL: url, Path: localPath}).Ensure(cmd.Context()); err != nil {
				log.Printf("ensure clone for %s: %s", url, err)
				continue
			}
			if component == "" {
				component = surl.Name
			}
			repos = append(repos, docs.RepoSpec{Name: url, Path: localPath, Ref: ref, Component: component})
		}
		if len(repos) == 0 {
			// Fallback to local ARO-HCP repo path
			repos = []docs.RepoSpec{{Name: "Azure/ARO-HCP", Path: cfg.LocalRepoPath}}
		}
		ctx := context.Background()
		return ing.Run(ctx, repos)
	}

	return cmd
}

func main() {
	// Bind config/env for all subcommands
	config.Init(rootCmd)
	rootCmd.AddCommand(prsCmd)
	rootCmd.AddCommand(newDocsCmd())

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("ingest: %v", err)
	}
}
