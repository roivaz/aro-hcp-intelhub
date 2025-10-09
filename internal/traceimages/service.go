package traceimages

import (
	"context"
	"fmt"

	"github.com/roivaz/aro-hcp-intelhub/internal/db"
	"github.com/roivaz/aro-hcp-intelhub/internal/logging"
	tooltypes "github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools/types"
)

// Service provides cache-aware access to trace image results.
type Service struct {
	tracer *Tracer
	repo   *db.SearchRepository
	log    logging.Logger
}

// New constructs a new Service.
func New(tracer *Tracer, repo *db.SearchRepository, log logging.Logger) *Service {
	return &Service{tracer: tracer, repo: repo, log: log.WithName("traceimages.service")}
}

// TraceImages returns the trace information for a commit/environment pair, serving cached results when possible.
func (s *Service) TraceImages(ctx context.Context, commitSHA, environment string) (tooltypes.TraceImagesResponse, error) {
	if commitSHA == "" || environment == "" {
		return tooltypes.TraceImagesResponse{}, fmt.Errorf("commit and environment are required")
	}

	if s.repo == nil {
		s.log.Debug("no cache repository configured; invoking tracer")
		return s.traceAndBuild(ctx, commitSHA, environment)
	}

	s.log.Debug("checking trace cache", "commit", commitSHA, "environment", environment)
	cached, err := s.repo.TraceImageCacheGet(ctx, commitSHA, environment)
	if err != nil {
		s.log.Error(err, "trace cache lookup failed", "commit", commitSHA, "environment", environment)
		return tooltypes.TraceImagesResponse{}, err
	}
	if cached != nil {
		s.log.Debug("cache hit", "commit", commitSHA, "environment", environment)
		return cached.Response, nil
	}

	s.log.Debug("cache miss", "commit", commitSHA, "environment", environment)
	resp, err := s.traceAndBuild(ctx, commitSHA, environment)
	if err != nil {
		return tooltypes.TraceImagesResponse{}, err
	}

	if hasErrors(resp) {
		s.log.Debug("skipping cache due to errors", "commit", commitSHA, "environment", environment, "errors", resp.Errors)
		return resp, nil
	}

	if err := s.repo.TraceImageCacheUpsert(ctx, commitSHA, environment, resp); err != nil {
		s.log.Error(err, "trace cache upsert failed", "commit", commitSHA, "environment", environment)
		return tooltypes.TraceImagesResponse{}, err
	}

	return resp, nil
}

func (s *Service) traceAndBuild(ctx context.Context, commitSHA, environment string) (tooltypes.TraceImagesResponse, error) {
	result, err := s.tracer.Trace(ctx, commitSHA, environment)
	if err != nil {
		s.log.Error(err, "trace execution failed", "commit", commitSHA, "environment", environment)
		return tooltypes.TraceImagesResponse{}, err
	}

	components := make([]tooltypes.ComponentTraceInfo, len(result.Components))
	for i, comp := range result.Components {
		components[i] = tooltypes.ComponentTraceInfo{
			Name:          comp.Name,
			Registry:      comp.Registry,
			Repository:    comp.Repository,
			Digest:        comp.Digest,
			SourceSHA:     comp.SourceSHA,
			SourceRepoURL: comp.SourceRepoURL,
			Error:         comp.Error,
		}
	}

	return tooltypes.TraceImagesResponse{
		CommitSHA:   result.CommitSHA,
		Environment: result.Environment,
		Components:  components,
		Errors:      result.Errors,
	}, nil
}

func hasErrors(resp tooltypes.TraceImagesResponse) bool {
	if len(resp.Errors) > 0 {
		return true
	}
	for _, comp := range resp.Components {
		if comp.Error != nil && *comp.Error != "" {
			return true
		}
	}
	return false
}
