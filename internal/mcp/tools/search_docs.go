package tools

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	vcsurl "github.com/gitsight/go-vcsurl"
	"github.com/roivaz/aro-hcp-intelhub/internal/config"
	"github.com/roivaz/aro-hcp-intelhub/internal/gitrepo"
	"github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools/types"
)

type DocSearchService interface {
	SearchDocs(ctx context.Context, query string, limit int, component, repo *string, includeFull bool) ([]types.DocResult, error)
}

type SearchDocsHandler struct{ Service DocSearchService }

func (h *SearchDocsHandler) ToolAdapter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}
	limit := 10
	if raw, ok := args["limit"].(float64); ok {
		if int(raw) > 0 {
			limit = int(raw)
		}
	}
	var componentPtr, repoPtr *string
	if v, ok := args["component"].(string); ok && v != "" {
		componentPtr = &v
	}
	if v, ok := args["repo"].(string); ok && v != "" {
		repoPtr = &v
	}
	includeFull := false
	if v, ok := args["include_full_file"].(bool); ok {
		includeFull = v
	}

	results, err := h.Service.SearchDocs(ctx, query, limit, componentPtr, repoPtr, includeFull)
	if err != nil {
		return nil, err
	}

	if includeFull {
		// Enrich with full file content from local cache
		for i := range results {
			r := &results[i]
			if r.Repo == "" || r.CommitSHA == "" || r.Path == "" {
				continue
			}

			info, err := vcsurl.Parse(r.Repo)
			if err != nil {
				continue
			}
			localPath := filepath.Join(config.CacheDir(), info.Name)
			// Ensure repo exists as in ingest command
			_, _ = gitrepo.New(gitrepo.RepoConfig{URL: r.Repo, Path: localPath}).Ensure(ctx)
			gr := gitrepo.New(gitrepo.RepoConfig{Path: localPath})
			if content, err := gr.ShowFile(ctx, r.CommitSHA, r.Path); err == nil {
				s := string(content)
				r.Content = &s
			}
		}
	}

	response := struct {
		Query   string            `json:"query"`
		Results []types.DocResult `json:"results"`
		Total   int               `json:"total_found"`
	}{Query: query, Results: results, Total: len(results)}

	return mcp.NewToolResultText(string(mustMarshal(response))), nil
}
