package db

import (
	"fmt"
	"time"

	"github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools/types"
)

func ToPRResult(entity PREmbedding, similarity *float64) types.PRResult {
	var mergedAt *string
	if entity.MergedAt != nil {
		v := entity.MergedAt.Format(time.RFC3339)
		mergedAt = &v
	}
	result := types.PRResult{
		PRNumber:        entity.PRNumber,
		Title:           entity.PRTitle,
		Body:            entity.PRBody,
		Author:          entity.Author,
		State:           entity.State,
		CreatedAt:       entity.CreatedAt.Format(time.RFC3339),
		MergedAt:        mergedAt,
		GithubURL:       githubURL(entity.PRNumber),
		SimilarityScore: similarity,
	}
	return result
}

func githubURL(prNumber int) string {
	return fmt.Sprintf("https://github.com/Azure/ARO-HCP/pull/%d", prNumber)
}
