package diff

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/roivaz/aro-hcp-intelhub/internal/logging"
)

type Analyzer struct {
	cfg       Config
	log       logging.Logger
	patterns  map[string]*regexp.Regexp
	llmClient *llmClient
}

func NewAnalyzer(cfg Config) (*Analyzer, error) {
	log := logging.New(cfg.Logger)

	patterns := buildIgnorePatterns()

	client, err := newLLMClient(cfg, cfg.Logger)
	if err != nil {
		return nil, err
	}

	return &Analyzer{
		cfg:       cfg,
		log:       log,
		patterns:  patterns,
		llmClient: client,
	}, nil
}

func (a *Analyzer) Analyze(ctx context.Context, meta PRMetadata) (Analysis, error) {
	if !a.cfg.Enabled {
		a.log.Info("diff analyzer disabled", "pr", meta.Number)
		return Analysis{AnalysisSuccessful: false, FailureReason: "diff analyzer disabled", FailureCategory: "disabled"}, nil
	}

	diffText, err := fetchConsolidatedDiff(ctx, meta, a.cfg.RepoPath, a.log)
	if err != nil {
		a.log.Error(err, "fetch diff failed", "pr", meta.Number)
		return Analysis{AnalysisSuccessful: false, FailureReason: err.Error()}, nil
	}

	fileChunks := splitDiffIntoFiles(diffText, a.log)
	if len(fileChunks) == 0 {
		return Analysis{AnalysisSuccessful: false, FailureReason: "no diff content"}, nil
	}

	included, skipped := filterGeneratedFiles(fileChunks, a.patterns)
	if len(included) == 0 {
		return Analysis{AnalysisSuccessful: false, FailureReason: "all files filtered as generated"}, nil
	}

	docs, stats := buildDocuments(included, a.log, a.cfg)
	stats.FilesFiltered = len(skipped)
	stats.FilesTotal = len(fileChunks)

	a.log.Info("diff prep stats",
		"pr", meta.Number,
		"files_total", stats.FilesTotal,
		"files_included", stats.FilesIncluded,
		"files_filtered", stats.FilesFiltered,
		"max_tokens", stats.MaxTokens,
		"median_tokens", stats.MedianTokens,
	)

	if len(docs) > 100 {
		a.log.Error(fmt.Errorf("large diff detected: %d chunks", len(docs)), "large diff", "pr", meta.Number, "files", len(docs))
		return Analysis{AnalysisSuccessful: false, FailureReason: "large diff detected",
			FailureCategory: FailureCategoryLargeDiff}, nil
	}

	mapSummaries := make([]string, 0, len(docs))
	for idx, doc := range docs {
		a.log.Debug(fmt.Sprintf("mapping chunk %d/%d", idx+1, len(docs)), "file", doc.FilePath)
		result, err := a.llmClient.mapChunk(ctx, doc, meta)
		if err != nil {
			a.log.Error(err, "map stage failed", "file", doc.FilePath)
			reason, category := GetFailureDetails(err)
			return Analysis{AnalysisSuccessful: false, FailureReason: reason, FailureCategory: category}, nil
		}
		mapSummaries = append(mapSummaries, result)
	}

	reduceResult, err := a.llmClient.reduceSummary(ctx, mapSummaries, meta)
	if err != nil {
		a.log.Error(err, "reduce stage failed", "pr", meta.Number)
		reason, category := GetFailureDetails(err)
		return Analysis{AnalysisSuccessful: false, FailureReason: reason, FailureCategory: category}, nil
	}
	a.log.Debug("Reduce stage completed", "summary", reduceResult)

	richDescription := fmt.Sprintf("## Pull Request Analysis: %s\n\n%s", meta.Title, strings.TrimSpace(reduceResult))

	return Analysis{
		RichDescription:    richDescription,
		AnalysisSuccessful: true,
	}, nil
}
