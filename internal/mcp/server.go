package mcp

import (
	"context"
	"log"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/roivaz/aro-hcp-intelhub/internal/db"
)

type ToolAdapter interface {
	ToolAdapter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

type Server struct {
	MCP     *server.MCPServer
	HTTP    *server.StreamableHTTPServer
	Handler http.Handler
	DB      *db.Database
}

func New(cfg Config) *Server {
	mcpServer := server.NewMCPServer(
		"aro-hcp-server",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register tools with their proper schemas using mcp-go builder pattern
	toolDefinitions := map[string]mcp.Tool{
		"search_docs": mcp.NewTool("search_docs",
			mcp.WithDescription("Semantic search across documentation using embeddings. Returns relevant documentation chunks with similarity scores from the ARO-HCP repository."),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Natural language search query (e.g., 'How does cluster creation work?')"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of results to return (default: 10)"),
			),
			mcp.WithString("component",
				mcp.Description("Optional: Filter results by component name (e.g., 'cluster-service', 'maestro')"),
			),
			mcp.WithString("repo",
				mcp.Description("Optional: Filter results by repository URL"),
			),
			mcp.WithBoolean("include_full_file",
				mcp.Description("Include full file content in results (default: false)"),
			),
		),
		"search_prs": mcp.NewTool("search_prs",
			mcp.WithDescription("Semantic search across pull requests using embeddings. Returns relevant PRs with similarity scores, titles, descriptions, and metadata."),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Natural language search query (e.g., 'PRs related to authentication')"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of results to return (default: 10)"),
			),
		),
		"get_pr_details": mcp.NewTool("get_pr_details",
			mcp.WithDescription("Retrieve detailed information about a specific pull request by its number, including title, body, status, and metadata."),
			mcp.WithNumber("pr_number",
				mcp.Required(),
				mcp.Description("The pull request number (e.g., 1234)"),
			),
		),
		"trace_images": mcp.NewTool("trace_images",
			mcp.WithDescription("Trace container images used in deployments for a specific commit and environment. Returns image references, tags, and deployment manifests."),
			mcp.WithString("commit_sha",
				mcp.Required(),
				mcp.Description("Git commit SHA to trace images from (full 40-character SHA)"),
			),
			mcp.WithString("environment",
				mcp.Required(),
				mcp.Description("Deployment environment"),
				mcp.Enum("dev", "staging", "production"),
			),
		),
	}

	for name, adapter := range cfg.ToolAdapters {
		tool := toolDefinitions[name]
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
