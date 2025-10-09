package diff

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/roivaz/aro-hcp-intelhub/internal/gitrepo"
	"github.com/roivaz/aro-hcp-intelhub/internal/logging"
)

var configureFetchSpecOnce sync.Once

const prFetchSpec = "+refs/pull/*/head:refs/remotes/origin/pr/*"

func fetchConsolidatedDiff(ctx context.Context, meta PRMetadata, repoPath string, log logging.Logger) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("diff analyzer requires repo path")
	}
	if meta.Number == 0 {
		return "", fmt.Errorf("missing PR number")
	}

	if err := ensurePRFetchSpec(ctx, repoPath, log); err != nil {
		return "", fmt.Errorf("configure fetch spec: %w", err)
	}

	repo := gitrepo.New(gitrepo.RepoConfig{Path: repoPath})
	if err := repo.Fetch(ctx); err != nil {
		return "", fmt.Errorf("git fetch origin: %w", err)
	}

	// merged PR with merge commit available -> show merge diff
	if meta.MergeCommitSHA != "" {
		parent := fmt.Sprintf("%s^1", meta.MergeCommitSHA)
		rangeSpec := fmt.Sprintf("%s..%s", parent, meta.MergeCommitSHA)
		log.Debug("generating diff", "range", rangeSpec)
		// Prefer 'show' with range to match tracer semantics; keep unified context 3
		r := gitrepo.New(gitrepo.RepoConfig{Path: repoPath})
		diff, err := r.MergeDiff(ctx, meta.MergeCommitSHA)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(diff) == "" {
			return "", fmt.Errorf("empty diff")
		}
		return diff, nil
	}

	return "", fmt.Errorf("merged PR with no merge commit available")
}

func ensurePRFetchSpec(ctx context.Context, repoPath string, log logging.Logger) error {
	var returnErr error
	configureFetchSpecOnce.Do(func() {
		output, err := gitCommand(ctx, repoPath, "config", "--local", "--get-all", "remote.origin.fetch")
		if err == nil && strings.Contains(output, prFetchSpec) {
			return
		}
		if _, err := gitCommand(ctx, repoPath, "config", "--local", "--add", "remote.origin.fetch", prFetchSpec); err != nil {
			returnErr = err
			return
		}
		log.Info("added PR fetch-spec to origin remote")
	})
	return returnErr
}

func normalizeBaseRef(base string) string {
	switch {
	case strings.HasPrefix(base, "refs/heads/"):
		base = strings.TrimPrefix(base, "refs/heads/")
	case strings.HasPrefix(base, "origin/"):
		return base
	}
	return fmt.Sprintf("origin/%s", base)
}

// gitCommand retained only for config probing; consider replacing with gitrepo if needed elsewhere.
func gitCommand(ctx context.Context, repoPath string, args ...string) (string, error) {
	r := gitrepo.New(gitrepo.RepoConfig{Path: repoPath})
	return r.Run(ctx, args...)
}
