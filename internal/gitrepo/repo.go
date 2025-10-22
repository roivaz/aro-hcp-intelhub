package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type RepoConfig struct {
	URL    string
	Path   string
	Remote string // default: origin
}

type Repo struct {
	cfg    RepoConfig
	runner Runner
}

func New(cfg RepoConfig) *Repo {
	if cfg.Remote == "" {
		cfg.Remote = "origin"
	}
	return &Repo{cfg: cfg, runner: Runner{Timeout: 2 * time.Minute}}
}

type Runner struct {
	Timeout time.Duration
}

func (r Runner) Git(ctx context.Context, dir string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "git", args...)
	c.Dir = dir
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Start(); err != nil {
		return "", formatGitError(args, err, stderr.String())
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return "", formatGitError(args, err, stderr.String())
		}
		return stdout.String(), nil
	case <-time.After(r.Timeout):
		_ = c.Process.Kill()
		<-done
		return "", formatGitTimeoutError(args, r.Timeout, stderr.String())
	case <-ctx.Done():
		_ = c.Process.Kill()
		<-done
		return "", formatGitContextError(args, ctx.Err(), stderr.String())
	}
}

func formatGitError(args []string, cause error, stderr string) error {
	cmd := strings.Join(args, " ")
	stderr = strings.TrimSpace(stderr)
	if stderr != "" {
		return fmt.Errorf("git %s: %w: %s", cmd, cause, stderr)
	}
	return fmt.Errorf("git %s: %w", cmd, cause)
}

func formatGitTimeoutError(args []string, timeout time.Duration, stderr string) error {
	return formatGitError(args, fmt.Errorf("command timed out after %s", timeout), stderr)
}

func formatGitContextError(args []string, cause error, stderr string) error {
	if cause == nil {
		cause = errors.New("context canceled")
	}
	return formatGitError(args, cause, stderr)
}

// Run is a helper to execute arbitrary git subcommands in the repo path.
func (r *Repo) Run(ctx context.Context, args ...string) (string, error) {
	return r.runner.Git(ctx, r.cfg.Path, args...)
}

// Ensure clones the repo if missing; otherwise fetches.
func (r *Repo) Ensure(ctx context.Context) (string, error) {
	abs, err := filepath.Abs(r.cfg.Path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		if _, err := r.runner.Git(ctx, "", "clone", "--filter=blob:none", "--no-tags", r.cfg.URL, abs); err != nil {
			return "", err
		}
		return abs, nil
	}
	if err := r.Fetch(ctx); err != nil {
		return "", err
	}
	return abs, nil
}

func (r *Repo) Fetch(ctx context.Context, extraArgs ...string) error {
	args := append([]string{"fetch", "--prune", r.cfg.Remote}, extraArgs...)
	_, err := r.runner.Git(ctx, r.cfg.Path, args...)
	return err
}

func (r *Repo) CheckoutDetach(ctx context.Context, ref string) error {
	// Fast path: already at ref
	if head, _ := r.HeadSHA(ctx); head == ref {
		return nil
	}
	_, err := r.runner.Git(ctx, r.cfg.Path, "checkout", "--detach", ref)
	return err
}

func (r *Repo) HeadSHA(ctx context.Context) (string, error) {
	out, err := r.runner.Git(ctx, r.cfg.Path, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// MergeDiff returns a unified diff for merge^1..merge range.
func (r *Repo) MergeDiff(ctx context.Context, mergeSHA string) (string, error) {
	rangeSpec := fmt.Sprintf("%s^1..%s", mergeSHA, mergeSHA)
	out, err := r.runner.Git(ctx, r.cfg.Path, "show", "--no-color", "--no-ext-diff", "--format=", "--find-renames", rangeSpec)
	if err != nil {
		return "", err
	}
	return out, nil
}

// ListFiles returns repo-relative paths at the given ref.
func (r *Repo) ListFiles(ctx context.Context, ref string) ([]string, error) {
	out, err := r.runner.Git(ctx, r.cfg.Path, "ls-tree", "-r", "--name-only", ref)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var files []string
	for _, l := range lines {
		if l != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

// ShowFile reads a file blob at ref:path.
func (r *Repo) ShowFile(ctx context.Context, ref, path string) ([]byte, error) {
	spec := fmt.Sprintf("%s:%s", ref, path)
	out, err := r.runner.Git(ctx, r.cfg.Path, "show", spec)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// WorktreeAddDetach creates a detached worktree at dir for the given ref.
func (r *Repo) WorktreeAddDetach(ctx context.Context, dir, ref string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	_, err := r.runner.Git(ctx, r.cfg.Path, "worktree", "add", "--detach", dir, ref)
	return err
}

// WorktreeRemove removes the worktree at dir.
func (r *Repo) WorktreeRemove(ctx context.Context, dir string) error {
	_, err := r.runner.Git(ctx, r.cfg.Path, "worktree", "remove", dir, "--force")
	return err
}

// ConfigHasLocal checks if `git config --local --get-all <key>` contains value.
func (r *Repo) ConfigHasLocal(ctx context.Context, key, value string) (bool, error) {
	out, err := r.runner.Git(ctx, r.cfg.Path, "config", "--local", "--get-all", key)
	if err != nil {
		return false, nil
	}
	return strings.Contains(out, value), nil
}

// ConfigAddLocal appends a value to a multivalue local config key.
func (r *Repo) ConfigAddLocal(ctx context.Context, key, value string) error {
	_, err := r.runner.Git(ctx, r.cfg.Path, "config", "--local", "--add", key, value)
	return err
}
