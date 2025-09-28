package mcp

import (
	"context"
	"log"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/db"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion/embeddings"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/mcp/tools"
)

type ToolAdapter interface {
	ToolAdapter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

type Config struct {
	ToolAdapters map[string]ToolAdapter
	Options      []server.StreamableHTTPOption
	Database     *db.Database
}

type Server struct {
	MCP     *server.MCPServer
	HTTP    *server.StreamableHTTPServer
	Handler http.Handler
	DB      *db.Database
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

	repo := db.NewSearchRepository(database)
	embedClient := embeddings.NewClient(ingestionCfg.OllamaURL, ingestionCfg.EmbeddingModel)
	searchService := tools.NewDBSearchService(repo, embedClient)
	detailsService := tools.NewDBDetailsService(repo)

	return Config{
		ToolAdapters: map[string]ToolAdapter{
			"search_prs":     &tools.SearchPRsHandler{Service: searchService},
			"get_pr_details": &tools.GetPRDetailsHandler{Service: detailsService},
		},
		Options: []server.StreamableHTTPOption{
			server.WithEndpointPath("/mcp/jsonrpc"),
			server.WithStateLess(true),
		},
		Database: database,
	}
}

func New(cfg Config) *Server {
	mcpServer := server.NewMCPServer(
		"aro-hcp-server",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	for name, adapter := range cfg.ToolAdapters {
		tool := mcp.NewTool(name)
		mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return adapter.ToolAdapter(ctx, req)
		})
	}

	httpServer := server.NewStreamableHTTPServer(mcpServer, cfg.Options...)

	return &Server{
		MCP:     mcpServer,
		HTTP:    httpServer,
		Handler: httpServer,
		DB:      cfg.Database,
	}
}

func (s *Server) Close() {
	if s.DB != nil {
		if err := s.DB.Close(); err != nil {
			log.Printf("error closing database: %v", err)
		}
	}
}
