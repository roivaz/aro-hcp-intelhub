package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/roivaz/aro-hcp-intelhub/internal/db"
	"github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools/types"
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

	response := struct {
		Result types.PRResult `json:"result"`
	}{Result: pr}

	return mcp.NewToolResultText(string(mustMarshal(response))), nil
}
