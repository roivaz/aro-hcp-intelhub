package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

type Adapter interface {
	ToolAdapter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
}
