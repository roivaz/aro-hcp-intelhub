package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

type EchoHandler struct{}

func (h *EchoHandler) ToolAdapter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	message, _ := req.GetArguments()["message"].(string)
	if message == "" {
		message = "(empty)"
	}
	return mcp.NewToolResultText("Echo: " + message), nil
}
