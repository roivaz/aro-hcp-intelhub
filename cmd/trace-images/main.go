package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/roivaz/aro-hcp-intelhub/internal/config"
	"github.com/roivaz/aro-hcp-intelhub/internal/db"
	"github.com/roivaz/aro-hcp-intelhub/internal/logging"
	"github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools/types"
	"github.com/roivaz/aro-hcp-intelhub/internal/traceimages"
)

func main() {
	root := &cobra.Command{Use: "trace-images"}

	var commit string
	var environment string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Trace container images for a commit/environment pair",
		RunE: func(cmd *cobra.Command, args []string) error {
			if commit == "" {
				return fmt.Errorf("--commit-sha is required")
			}
			if environment == "" {
				return fmt.Errorf("--environment is required")
			}

			cfg := tracingConfig()

			dbConfig := db.Config{DSN: config.PostgresURL()}
			database, err := db.NewDatabase(dbConfig)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			repo := db.NewSearchRepository(database, db.WithTraceCacheMax(config.TraceCacheMaxEntries()))
			tclog := logging.New(logging.DefaultLogger())

			tracer, err := traceimages.NewTracer(cfg)
			if err != nil {
				return fmt.Errorf("init tracer: %w", err)
			}

			service := traceimages.New(tracer, repo, tclog)

			ctx := context.Background()
			resp, err := service.TraceImages(ctx, commit, environment)
			if err != nil {
				return err
			}

			return outputResponse(resp)
		},
	}

	cmd.Flags().StringVar(&commit, "commit-sha", "", "Git commit SHA to trace")
	cmd.Flags().StringVar(&environment, "environment", "", "Deployment environment")

	root.AddCommand(cmd)

	config.Init(root)

	if err := root.Execute(); err != nil {
		log.Fatalf("trace-images: %v", err)
	}
}

func tracingConfig() traceimages.Config {
	return traceimages.Config{
		RepoPath:   filepath.Join(config.CacheDir(), "aro-hcp-repo"),
		SkopeoPath: config.TraceSkopeoPath(),
		PullSecret: config.TracePullSecret(),
		Logger:     logging.New(logging.DefaultLogger().WithName("trace-images")),
	}
}

func outputResponse(resp types.TraceImagesResponse) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}
