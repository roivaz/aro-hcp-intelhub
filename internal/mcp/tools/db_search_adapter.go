package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/db"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion/embeddings"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/mcp/tools/types"
)

type DBSearchService struct {
	Repository  *db.SearchRepository
	EmbedClient *embeddings.Client
}

func NewDBSearchService(repo *db.SearchRepository, embed *embeddings.Client) *DBSearchService {
	return &DBSearchService{Repository: repo, EmbedClient: embed}
}

func (s *DBSearchService) SearchPRs(ctx context.Context, query string, limit int) ([]types.PRResult, error) {
	if strings.TrimSpace(query) == "" {
		return []types.PRResult{}, nil
	}

	vectors, err := s.EmbedClient.EmbedTexts(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return []types.PRResult{}, nil
	}

	rows, err := s.Repository.SearchPRs(ctx, vectors[0], limit)
	if err != nil {
		return nil, fmt.Errorf("search embeddings: %w", err)
	}

	results := make([]types.PRResult, 0, len(rows))
	for _, row := range rows {
		similarity := 1 - (row.Distance / 2.0)
		result := db.ToPRResult(row.PREmbedding, &similarity)
		results = append(results, result)
	}
	return results, nil
}
