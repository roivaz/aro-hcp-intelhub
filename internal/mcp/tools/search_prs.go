package tools

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/mcp/tools/types"
)

type SearchService interface {
	SearchPRs(ctx context.Context, query string, limit int) ([]types.PRResult, error)
}

type SearchPRsHandler struct {
	Service SearchService
}

type SearchPRsParams struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (h *SearchPRsHandler) ToolAdapter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	query, _ := args["query"].(string)
	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}
	limit := 10
	if rawLimit, ok := args["limit"].(float64); ok {
		parsed := int(rawLimit)
		if parsed > 0 {
			limit = parsed
		}
	}
	results, err := h.Service.SearchPRs(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(results)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultJSON(payload)
}
