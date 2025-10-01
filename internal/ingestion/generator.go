package ingestion

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	pgvector "github.com/pgvector/pgvector-go"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/db"
	diffanalyzer "github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion/diff"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/ingestion/embeddings"
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
	if strings.EqualFold(g.cfg.IngestionMode, "BATCH") {
		return g.runBatch(ctx)
	}
	return g.runIncremental(ctx)
}

func (g *Generator) runIncremental(ctx context.Context) error {
	latestMergedAt, latestNumber, err := g.repo.LatestMergedPR(ctx)
	if err != nil {
		return fmt.Errorf("latest merged pr: %w", err)
	}
	var watermark time.Time
	var lastNumber int
	if !latestMergedAt.IsZero() {
		watermark = latestMergedAt
		lastNumber = latestNumber
	} else if g.cfg.IngestionStartDate != nil {
		watermark = *g.cfg.IngestionStartDate
	}

	prs, err := g.fetcher.FetchSince(ctx, watermark, lastNumber, g.cfg.IngestionLimit)
	if err != nil {
		return fmt.Errorf("fetch incremental prs: %w", err)
	}

	if len(prs) == 0 {
		log.Printf("incremental: no new PRs after %s", watermark.Format(time.RFC3339))
		return nil
	}

	return g.ingestPRs(ctx, prs)
}

func (g *Generator) runBatch(ctx context.Context) error {
	if g.cfg.BatchDirection != "backwards" && g.cfg.BatchDirection != "onwards" {
		return fmt.Errorf("invalid batch direction: %s", g.cfg.BatchDirection)
	}
	var start time.Time
	if g.cfg.IngestionStartDate != nil {
		start = *g.cfg.IngestionStartDate
	}

	prs, err := g.fetcher.FetchBatch(ctx, start, g.cfg.BatchDirection, g.cfg.IngestionLimit)
	if err != nil {
		return fmt.Errorf("fetch batch prs: %w", err)
	}
	if len(prs) == 0 {
		log.Printf("batch: no PRs fetched starting from %s direction %s", start.Format(time.RFC3339), g.cfg.BatchDirection)
		return nil
	}
	return g.ingestPRs(ctx, prs)
}

func (g *Generator) ingestPRs(ctx context.Context, prs []PRChange) error {
	processed := 0

	var analyzer *diffanalyzer.Analyzer
	if g.cfg.DiffAnalyzer.Enabled {
		a, err := diffanalyzer.NewAnalyzer(g.cfg.DiffAnalyzer)
		if err != nil {
			return fmt.Errorf("init diff analyzer: %w", err)
		}
		analyzer = a
	}

	for _, pr := range prs {
		exists, err := g.repo.HasPR(ctx, pr.Number)
		if err != nil {
			return fmt.Errorf("check PR existence: %w", err)
		}
		if exists {
			log.Printf("ingestion: PR #%d already stored, skipping", pr.Number)
			continue
		}

		document := embeddings.BuildDocument(pr.Title, pr.Body, "")
		log.Printf("ingestion: embedding PR #%d", pr.Number)
		vectors, err := g.embedClient.EmbedTexts(ctx, []string{document})
		if err != nil {
			return fmt.Errorf("embed PR #%d: %w", pr.Number, err)
		}
		if len(vectors) == 0 {
			return fmt.Errorf("ollama returned no vectors for PR #%d", pr.Number)
		}

		var richDescription *string
		analysisSuccessful := false
		var failureReason *string

		if analyzer != nil {
			metadata := diffanalyzer.PRMetadata{
				Number:         pr.Number,
				Title:          pr.Title,
				Body:           pr.Body,
				Author:         pr.Author,
				BaseRef:        pr.BaseRef,
				HeadCommitSHA:  pr.HeadCommitSHA,
				MergeCommitSHA: pr.MergeCommitSHA,
				CreatedAt:      pr.CreatedAt,
				MergedAt:       pr.MergedAt,
			}
			analysis, err := analyzer.Analyze(ctx, metadata)
			if err != nil {
				reason := err.Error()
				failureReason = &reason
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
			}
		}

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
			Embedding:          pgvector.NewVector(vectors[0]),
			RichDescription:    richDescription,
			AnalysisSuccessful: analysisSuccessful,
			FailureReason:      failureReason,
		}

		if err := g.repo.StorePR(ctx, record); err != nil {
			return fmt.Errorf("store PR #%d: %w", pr.Number, err)
		}
		log.Printf("ingestion: stored embedding for PR #%d", pr.Number)

		processed++
	}

	log.Printf("processed %d new PR embeddings", processed)
	return nil
}

func nullableString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
