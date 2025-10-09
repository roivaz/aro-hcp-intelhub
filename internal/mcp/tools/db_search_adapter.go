package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/roivaz/aro-hcp-intelhub/internal/db"
	"github.com/roivaz/aro-hcp-intelhub/internal/ingestion/embeddings"
	"github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools/types"
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

func (s *DBSearchService) SearchDocs(ctx context.Context, query string, limit int, component, repo *string, includeFull bool) ([]types.DocResult, error) {
	if strings.TrimSpace(query) == "" {
		return []types.DocResult{}, nil
	}
	vectors, err := s.EmbedClient.EmbedTexts(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return []types.DocResult{}, nil
	}
	rows, err := s.Repository.SearchDocs(ctx, vectors[0], limit, component, repo)
	if err != nil {
		return nil, fmt.Errorf("search docs: %w", err)
	}
	results := make([]types.DocResult, 0, len(rows))
	for _, row := range rows {
		sim := 1 - row.Distance
		r := types.DocResult{
			Repo:       row.DocumentChunk.Repo,
			Component:  row.DocumentChunk.Component,
			Path:       row.DocumentChunk.Path,
			CommitSHA:  row.DocumentChunk.CommitSHA,
			SourceURL:  row.DocumentChunk.SourceURL,
			Snippet:    row.Snippet,
			Similarity: sim,
		}
		results = append(results, r)
	}
	return results, nil
}
