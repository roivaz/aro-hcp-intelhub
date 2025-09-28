package mcp

import (
	"context"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/mcp/tools"
)

type ToolAdapter interface {
	ToolAdapter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

type Config struct {
	ToolAdapters map[string]ToolAdapter
	Options      []server.StreamableHTTPOption
}

type Server struct {
	MCP     *server.MCPServer
	HTTP    *server.StreamableHTTPServer
	Handler http.Handler
}

func DefaultConfig() Config {
	return Config{
		ToolAdapters: map[string]ToolAdapter{
			"dummy_echo": &tools.EchoHandler{},
		},
		Options: []server.StreamableHTTPOption{
			server.WithEndpointPath("/mcp/jsonrpc"),
			server.WithStateLess(true),
		},
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
	}
}
