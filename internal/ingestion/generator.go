package ingestion

import (
	"context"
	"fmt"
	"log"
	"strings"

	pgvector "github.com/pgvector/pgvector-go"

	"github.com/roivaz/aro-hcp-intelhub/internal/db"
	dbmigrate "github.com/roivaz/aro-hcp-intelhub/internal/db/migrate"
	diffanalyzer "github.com/roivaz/aro-hcp-intelhub/internal/ingestion/diff"
	"github.com/roivaz/aro-hcp-intelhub/internal/ingestion/embeddings"
)

type Generator struct {
	cfg         Config
	db          *db.Database
	repo        *db.SearchRepository
	embedClient *embeddings.Client
	fetcher     *GitHubFetcher
}

func NewGenerator(cfg Config, database *db.Database, repo *db.SearchRepository, embed *embeddings.Client, fetcher *GitHubFetcher) *Generator {
	return &Generator{cfg: cfg, db: database, repo: repo, embedClient: embed, fetcher: fetcher}
}

func (g *Generator) Run(ctx context.Context) error {
	if err := dbmigrate.EnsureCurrent(ctx, g.db.Bun(), "", g.cfg.AutoMigrate); err != nil {
		return err
	}

	switch strings.ToUpper(g.cfg.ExecutionMode) {
	case "CACHE":
		return g.RunCache(ctx)
	case "PROCESS":
		return g.RunProcess(ctx)
	case "FULL", "":
		return g.RunFull(ctx)
	default:
		return fmt.Errorf("invalid execution mode: %s (must be FULL, CACHE, or PROCESS)", g.cfg.ExecutionMode)
	}
}

func (g *Generator) RunFull(ctx context.Context) error {
	log.Printf("full mode: caching PRs from GitHub, then processing them")

	// First, cache new PRs from GitHub
	if err := g.RunCache(ctx); err != nil {
		return fmt.Errorf("cache phase: %w", err)
	}

	// Then, process the cached PRs
	if err := g.RunProcess(ctx); err != nil {
		return fmt.Errorf("process phase: %w", err)
	}

	return nil
}

func (g *Generator) RunProcess(ctx context.Context) error {
	limit := g.cfg.MaxProcessBatch
	if limit <= 0 {
		limit = g.cfg.GitHubFetchMax
	}

	unprocessedCount, err := g.repo.CountUnprocessedPRs(ctx)
	if err != nil {
		return fmt.Errorf("count unprocessed PRs: %w", err)
	}

	log.Printf("process mode: found %d unprocessed PRs, will process up to %d", unprocessedCount, limit)

	if unprocessedCount == 0 {
		log.Printf("process: no unprocessed PRs found")
		return nil
	}

	prs, err := g.repo.GetUnprocessedPRs(ctx, limit)
	if err != nil {
		return fmt.Errorf("get unprocessed PRs: %w", err)
	}

	log.Printf("process: processing %d PRs sequentially", len(prs))

	var analyzer *diffanalyzer.Analyzer
	if g.cfg.DiffAnalyzer.Enabled {
		a, err := diffanalyzer.NewAnalyzer(g.cfg.DiffAnalyzer)
		if err != nil {
			return fmt.Errorf("init diff analyzer: %w", err)
		}
		analyzer = a
	}

	// Process PRs sequentially
	processed := 0
	for _, pr := range prs {
		if err := g.processSinglePR(ctx, pr, analyzer); err != nil {
			log.Printf("process: error processing PR #%d: %v", pr.PRNumber, err)
			continue
		}
		processed++
	}

	log.Printf("process: processed %d PR(s)", processed)
	return nil
}

func (g *Generator) RunCache(ctx context.Context) error {
	log.Printf("cache mode: fetching and storing PR metadata only (no embeddings/analysis)")

	newPRs, err := g.fetchNewPRs(ctx)
	if err != nil {
		return err
	}

	if len(newPRs) == 0 {
		log.Printf("cache: no new PRs to store")
		return nil
	}

	return g.cachePRs(ctx, newPRs)
}

// fetchNewPRs fetches new PRs from GitHub that aren't already in the database
func (g *Generator) fetchNewPRs(ctx context.Context) ([]PRChange, error) {
	var newPRs []PRChange
	currentPage := 1
	totalFetched := 0
	reachedCached := false

	for len(newPRs) < g.cfg.GitHubFetchMax {
		result, err := g.fetcher.FetchBatch(ctx, currentPage)
		if err != nil {
			return nil, fmt.Errorf("fetch batch prs (page %d): %w", currentPage, err)
		}

		if result.PageCount == 0 {
			break
		}

		totalFetched += result.PageCount

		for _, pr := range result.PRs {
			if len(newPRs) >= g.cfg.GitHubFetchMax {
				break
			}

			exists, err := g.repo.HasPR(ctx, pr.Number)
			if err != nil {
				return nil, fmt.Errorf("check PR existence: %w", err)
			}
			if exists {
				log.Printf("PR #%d already stored, stopping at first cached PR", pr.Number)
				reachedCached = true
				break
			}
			newPRs = append(newPRs, pr)
		}

		if len(newPRs) >= g.cfg.GitHubFetchMax || reachedCached || !result.HasMore {
			break
		}

		currentPage = result.NextPage
	}

	log.Printf("cache: scanned %d PRs from GitHub total, found %d new", totalFetched, len(newPRs))
	return newPRs, nil
}

func (g *Generator) cachePRs(ctx context.Context, prs []PRChange) error {
	for _, pr := range prs {
		record := &db.PREmbedding{
			PRNumber:           pr.Number,
			PRTitle:            pr.Title,
			PRBody:             pr.Body,
			Author:             pr.Author,
			CreatedAt:          pr.CreatedAt,
			MergedAt:           pr.MergedAt,
			State:              pr.State,
			BaseRef:            pr.BaseRef,
			GithubBaseSHA:      nullableString(pr.BaseSHA),
			HeadCommitSHA:      nullableString(pr.HeadCommitSHA),
			MergeCommitSHA:     nullableString(pr.MergeCommitSHA),
			Embedding:          nil, // Not processed yet
			RichDescription:    nil,
			AnalysisSuccessful: false,
			FailureReason:      nil,
			ProcessedAt:        nil, // Mark as unprocessed
		}

		if err := g.repo.StorePR(ctx, record); err != nil {
			return fmt.Errorf("store PR #%d: %w", pr.Number, err)
		}
		log.Printf("cache: stored PR #%d (unprocessed)", pr.Number)
	}

	log.Printf("cached %d new PRs without processing", len(prs))
	return nil
}

func (g *Generator) processSinglePR(ctx context.Context, pr *db.PREmbedding, analyzer *diffanalyzer.Analyzer) error {
	log.Printf("process: generating embedding for PR #%d", pr.PRNumber)

	document := embeddings.BuildDocument(pr.PRTitle, pr.PRBody, "")
	vectors, err := g.embedClient.EmbedTexts(ctx, []string{document})
	if err != nil {
		reason, category := diffanalyzer.GetFailureDetails(err)
		log.Printf("process: embedding failed for PR #%d: %v", pr.PRNumber, err)
		if updateErr := g.repo.UpdatePRProcessing(ctx, pr.PRNumber, nil, nil, false, strPtr(reason), strPtr(string(category))); updateErr != nil {
			return fmt.Errorf("update PR #%d after embedding failure: %w", pr.PRNumber, updateErr)
		}
		return nil
	}
	if len(vectors) == 0 {
		reason := "embedding returned no vectors"
		if updateErr := g.repo.UpdatePRProcessing(ctx, pr.PRNumber, nil, nil, false, strPtr(reason), strPtr("empty_embedding")); updateErr != nil {
			return fmt.Errorf("update PR #%d after empty embedding: %w", pr.PRNumber, updateErr)
		}
		return nil
	}

	embedding := pgvector.NewVector(vectors[0])
	var richDescription *string
	analysisSuccessful := false
	var failureReason *string
	var failureCategory *string

	if analyzer != nil {
		log.Printf("process: analyzing diff for PR #%d", pr.PRNumber)
		metadata := diffanalyzer.PRMetadata{
			Number:         pr.PRNumber,
			Title:          pr.PRTitle,
			Body:           pr.PRBody,
			Author:         pr.Author,
			BaseRef:        pr.BaseRef,
			HeadCommitSHA:  stringValue(pr.HeadCommitSHA),
			MergeCommitSHA: stringValue(pr.MergeCommitSHA),
			CreatedAt:      pr.CreatedAt,
			MergedAt:       pr.MergedAt,
		}
		analysis, err := analyzer.Analyze(ctx, metadata)
		if err != nil {
			reason, category := diffanalyzer.GetFailureDetails(err)
			failureReason = strPtr(reason)
			failureCategory = strPtr(string(category))
		} else {
			analysisSuccessful = analysis.AnalysisSuccessful
			if analysis.RichDescription != "" {
				desc := analysis.RichDescription
				richDescription = &desc
			}
			if analysis.FailureReason != "" {
				reason := analysis.FailureReason
				failureReason = &reason
			}
			if analysis.FailureCategory != "" {
				category := analysis.FailureCategory
				failureCategory = strPtr(string(category))
			}
		}
	}

	if err := g.repo.UpdatePRProcessing(ctx, pr.PRNumber, &embedding, richDescription, analysisSuccessful, failureReason, failureCategory); err != nil {
		return fmt.Errorf("update PR #%d: %w", pr.PRNumber, err)
	}

	log.Printf("process: completed PR #%d (analysis_successful=%v)", pr.PRNumber, analysisSuccessful)
	return nil
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nullableString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
