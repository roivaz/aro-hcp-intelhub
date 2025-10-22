package docs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pgvector/pgvector-go"

	"github.com/roivaz/aro-hcp-intelhub/internal/db"
	"github.com/roivaz/aro-hcp-intelhub/internal/gitrepo"
)

type EmbeddingClient interface {
	EmbedTexts(ctx context.Context, inputs []string) ([][]float32, error)
}

type Chunker interface {
	Split(text string) []string
}

type RepoSpec struct {
	Name      string // e.g., Azure/ARO-HCP
	Path      string // local path
	Component string // optional
	Ref       string // optional ref (default HEAD)
}

type Ingester struct {
	Repo      *db.SearchRepository
	Client    EmbeddingClient
	Chunker   Chunker
	Include   []string
	Exclude   []string
	MaxFiles  int
	MaxChunks int
	ModelName string
}

func (i *Ingester) Run(ctx context.Context, repos []RepoSpec) error {
	for _, r := range repos {
		if err := i.ingestRepoAtomic(ctx, r); err != nil {
			return fmt.Errorf("failed to ingest %s: %w", r.Name, err)
		}
	}
	return nil
}

func (i *Ingester) ingestRepoAtomic(ctx context.Context, r RepoSpec) error {
	// Create batch writer (handles transaction, temp table internally)
	writer, err := i.Repo.NewDocumentBatchWriter(ctx, r.Name)
	if err != nil {
		return fmt.Errorf("create batch writer: %w", err)
	}
	defer writer.Rollback() // Safe to call even after commit

	// Get repo reference
	repo := gitrepo.New(gitrepo.RepoConfig{Path: r.Path})
	ref := r.Ref
	if ref == "" {
		head, err := repo.HeadSHA(ctx)
		if err != nil {
			return fmt.Errorf("get HEAD: %w", err)
		}
		ref = head
	}

	// List and filter files
	files, err := repo.ListFiles(ctx, ref)
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}

	includeRx := globsToRegexp(i.Include)
	excludeRx := globsToRegexp(i.Exclude)
	selected := filterFiles(files, includeRx, excludeRx, i.MaxFiles)

	// Process files and add to batch
	for _, p := range selected {
		if i.MaxChunks > 0 && writer.Count() >= i.MaxChunks {
			break
		}

		content, err := repo.ShowFile(ctx, ref, p)
		if err != nil {
			continue
		}

		parts := i.Chunker.Split(string(content))
		for idx, part := range parts {
			if strings.TrimSpace(part) == "" {
				continue
			}
			if i.MaxChunks > 0 && writer.Count() >= i.MaxChunks {
				break
			}

			// Embed the chunk
			vecs, err := i.Client.EmbedTexts(ctx, []string{part})
			if err != nil {
				continue
			}

			// Create document
			id := sha256Hex(r.Name + ":" + p + ":" + ref + ":" + itoa(idx) + ":" + part)
			doc := db.DocumentChunk{
				ID:             id,
				Repo:           r.Name,
				Path:           p,
				CommitSHA:      ref,
				DocType:        classifyDocType(p),
				ChunkIndex:     idx,
				ChunkText:      part,
				Embedding:      pgvector.NewVector(vecs[0]),
				EmbeddingModel: i.ModelName,
				SourceURL:      strptr(guessURL(r.Name, p, ref)),
			}

			// Add to batch
			if err := writer.Add(ctx, &doc); err != nil {
				continue
			}
		}
	}

	// Commit atomic swap
	if err := writer.Commit(ctx); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}

	return nil
}

func globsToRegexp(globs []string) *regexp.Regexp {
	if len(globs) == 0 {
		return nil
	}
	var parts []string
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		// Convert glob to regex:
		// **/ → (.*/)? (zero or more directories)
		// ** → .* (any characters)
		// * → [^/]* (any characters except /)
		r := regexp.QuoteMeta(g)
		// Handle **/ as a special case (must come before ** replacement)
		r = strings.ReplaceAll(r, "\\*\\*/", "(.*/)?")
		// Handle remaining ** (not followed by /)
		r = strings.ReplaceAll(r, "\\*\\*", ".*")
		// Handle single *
		r = strings.ReplaceAll(r, "\\*", "[^/]*")
		parts = append(parts, "^"+r+"$")
	}
	if len(parts) == 0 {
		return nil
	}
	return regexp.MustCompile(strings.Join(parts, "|"))
}

func filterFiles(files []string, include, exclude *regexp.Regexp, max int) []string {
	var out []string
	for _, f := range files {
		if include != nil && !include.MatchString(f) {
			continue
		}
		if exclude != nil && exclude.MatchString(f) {
			continue
		}
		out = append(out, f)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func itoa(i int) string { return fmtInt(i) }

func fmtInt(i int) string { return strings.TrimPrefix(fmt.Sprintf("%d", i), "+") }

func classifyDocType(path string) string {
	base := filepath.Base(path)
	if strings.EqualFold(base, "README.md") {
		return "readme"
	}
	if strings.Contains(strings.ToLower(path), "/docs/") {
		return "docs"
	}
	if strings.Contains(strings.ToLower(path), "/adr/") {
		return "adr"
	}
	return "other"
}

func guessURL(repoName, path, ref string) string {
	// assume github for public repos
	if strings.Contains(repoName, "/") {
		return "https://github.com/" + repoName + "/blob/" + ref + "/" + path
	}
	return ""
}

func strptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
