package diff

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

var configureFetchSpecOnce sync.Once

const prFetchSpec = "+refs/pull/*/head:refs/remotes/origin/pr/*"

func fetchConsolidatedDiff(ctx context.Context, meta PRMetadata, repoPath string, log logger) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("diff analyzer requires repo path")
	}
	if meta.Number == 0 {
		return "", fmt.Errorf("missing PR number")
	}

	if err := ensurePRFetchSpec(ctx, repoPath, log); err != nil {
		return "", fmt.Errorf("configure fetch spec: %w", err)
	}

	if _, err := gitCommand(ctx, repoPath, "fetch", "origin", "--prune"); err != nil {
		return "", fmt.Errorf("git fetch origin: %w", err)
	}

	// merged PR with merge commit available -> show merge diff
	if meta.MergeCommitSHA != "" {
		parent := fmt.Sprintf("%s^1", meta.MergeCommitSHA)
		rangeSpec := fmt.Sprintf("%s..%s", parent, meta.MergeCommitSHA)
		log.Debug("generating diff", "range", rangeSpec)
		diff, err := gitCommand(ctx, repoPath, "diff", "--unified=3", parent, meta.MergeCommitSHA)
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

func ensurePRFetchSpec(ctx context.Context, repoPath string, log logger) error {
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

func gitCommand(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %v: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}
