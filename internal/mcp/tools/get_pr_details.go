package tools

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/db"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/mcp/tools/types"
)

type DetailsService interface {
	GetPRByNumber(ctx context.Context, prNumber int) (types.PRResult, error)
}

type GetPRDetailsHandler struct {
	Service DetailsService
}

type dbDetailsService struct {
	repo *db.SearchRepository
}

func NewDBDetailsService(repo *db.SearchRepository) DetailsService {
	return &dbDetailsService{repo: repo}
}

func (s *dbDetailsService) GetPRByNumber(ctx context.Context, prNumber int) (types.PRResult, error) {
	entity, err := s.repo.GetPRByNumber(ctx, prNumber)
	if err != nil {
		return types.PRResult{}, err
	}
	if entity == nil {
		return types.PRResult{}, nil
	}
	result := db.ToPRResult(*entity, nil)
	return result, nil
}

func (h *GetPRDetailsHandler) ToolAdapter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idValue := req.GetArguments()["pr_number"]
	number, err := parseIntArgument(idValue)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	pr, err := h.Service.GetPRByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(pr)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultJSON(payload)
}
