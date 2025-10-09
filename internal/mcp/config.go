package mcp

import (
	"log"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"

	"github.com/roivaz/aro-hcp-intelhub/internal/config"
	"github.com/roivaz/aro-hcp-intelhub/internal/db"
	"github.com/roivaz/aro-hcp-intelhub/internal/ingestion"
	"github.com/roivaz/aro-hcp-intelhub/internal/ingestion/embeddings"
	"github.com/roivaz/aro-hcp-intelhub/internal/logging"
	"github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools"
	"github.com/roivaz/aro-hcp-intelhub/internal/traceimages"
)

type Config struct {
	ToolAdapters map[string]ToolAdapter
	Options      []server.StreamableHTTPOption
	Database     *db.Database
}

func DefaultConfig() Config {
	ingestionCfg, err := ingestion.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load ingestion config: %v", err)
	}

	database, err := db.NewDatabase(db.Config{DSN: ingestionCfg.PostgresURL})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	repo := db.NewSearchRepository(database, db.WithTraceCacheMax(config.TraceCacheMaxEntries()))
	embedClient := embeddings.NewClient(ingestionCfg.OllamaURL, ingestionCfg.EmbeddingModel, ingestionCfg.LLMCallTimeout)
	searchService := tools.NewDBSearchService(repo, embedClient)
	detailsService := tools.NewDBDetailsService(repo)

	baseLogger := logging.DefaultLogger()
	traceTracer, err := traceimages.NewTracer(traceimages.Config{
		RepoPath:   filepath.Join(config.CacheDir(), "aro-hcp-repo"),
		SkopeoPath: config.TraceSkopeoPath(),
		PullSecret: config.TracePullSecret(),
		Logger:     logging.New(baseLogger.WithName("trace")),
	})
	if err != nil {
		log.Fatalf("failed to init trace tracer: %v", err)
	}

	traceService := traceimages.New(traceTracer, repo, logging.New(baseLogger.WithName("traceimages")))
	traceAdapter := tools.NewTraceImagesServiceAdapter(traceService)

	return Config{
		ToolAdapters: map[string]ToolAdapter{
			"search_prs":     &tools.SearchPRsHandler{Service: searchService},
			"get_pr_details": &tools.GetPRDetailsHandler{Service: detailsService},
			"trace_images":   &tools.TraceImagesHandler{Service: traceAdapter},
			"search_docs":    &tools.SearchDocsHandler{Service: searchService},
		},
		Options: []server.StreamableHTTPOption{
			server.WithEndpointPath("/mcp/jsonrpc"),
			server.WithStateLess(true),
		},
		Database: database,
	}
}
