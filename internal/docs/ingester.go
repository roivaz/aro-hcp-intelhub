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
	"github.com/uptrace/bun"

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
	DB        *bun.DB
	Client    EmbeddingClient
	Chunker   Chunker
	Include   []string
	Exclude   []string
	MaxFiles  int
	MaxChunks int
	ModelName string
}

func (i *Ingester) Run(ctx context.Context, repos []RepoSpec) error {
	includeRx := globsToRegexp(i.Include)
	excludeRx := globsToRegexp(i.Exclude)
	totalChunks := 0

	for _, r := range repos {
		repo := gitrepo.New(gitrepo.RepoConfig{Path: r.Path})
		ref := r.Ref
		if ref == "" {
			head, err := repo.HeadSHA(ctx)
			if err != nil {
				return err
			}
			ref = head
		}
		files, err := repo.ListFiles(ctx, ref)
		if err != nil {
			return err
		}

		selected := filterFiles(files, includeRx, excludeRx, i.MaxFiles)
		for _, p := range selected {
			if i.MaxChunks > 0 && totalChunks >= i.MaxChunks {
				return nil
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
				if i.MaxChunks > 0 && totalChunks >= i.MaxChunks {
					return nil
				}
				vecs, err := i.Client.EmbedTexts(ctx, []string{part})
				if err != nil {
					continue
				}
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
				_, _ = i.DB.NewInsert().Model(&doc).On("CONFLICT (id) DO NOTHING").Exec(ctx)
				totalChunks++
			}
		}
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
		// very small glob→regex: ** → .*, * → [^/]*, . escape
		r := regexp.QuoteMeta(g)
		r = strings.ReplaceAll(r, "\\*\\*", ".*")
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
