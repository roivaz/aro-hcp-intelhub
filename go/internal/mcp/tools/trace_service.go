package tools

import (
	"context"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/mcp/tools/types"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/tracing"
)

type TraceImagesService struct {
	Tracer *tracing.Tracer
}

func NewTraceImagesService(tracer *tracing.Tracer) *TraceImagesService {
	return &TraceImagesService{Tracer: tracer}
}

func (s *TraceImagesService) TraceImages(ctx context.Context, commitSHA, environment string) (types.TraceImagesResponse, error) {
	result, err := s.Tracer.Trace(ctx, commitSHA, environment)
	if err != nil {
		return types.TraceImagesResponse{}, err
	}

	components := make([]types.ComponentTraceInfo, len(result.Components))
	for i, comp := range result.Components {
		components[i] = types.ComponentTraceInfo{
			Name:          comp.Name,
			Registry:      comp.Registry,
			Repository:    comp.Repository,
			Digest:        comp.Digest,
			SourceSHA:     comp.SourceSHA,
			SourceRepoURL: comp.SourceRepoURL,
			Error:         comp.Error,
		}
	}

	return types.TraceImagesResponse{
		CommitSHA:   result.CommitSHA,
		Environment: result.Environment,
		Components:  components,
		Errors:      result.Errors,
	}, nil
}
