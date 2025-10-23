package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools/types"
)

type TraceService interface {
	TraceImages(ctx context.Context, commitSHA, environment string) (types.TraceImagesResponse, error)
}

type TraceImagesHandler struct {
	Service TraceService
}

func (h *TraceImagesHandler) ToolAdapter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	commit, _ := args["commit_sha"].(string)
	env, _ := args["environment"].(string)
	if commit == "" {
		return mcp.NewToolResultError("commit_sha is required"), nil
	}
	if env == "" {
		return mcp.NewToolResultError("environment is required"), nil
	}
	resp, err := h.Service.TraceImages(ctx, commit, env)
	if err != nil {
		return nil, err
	}

	response := struct {
		CommitSHA   string                    `json:"commit_sha"`
		Environment string                    `json:"environment"`
		Results     types.TraceImagesResponse `json:"results"`
	}{
		CommitSHA:   commit,
		Environment: env,
		Results:     resp,
	}

	return mcp.NewToolResultText(string(mustMarshal(response))), nil
}
