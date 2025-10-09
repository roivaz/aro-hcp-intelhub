package tools

import (
	"context"
	"fmt"

	"github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools/types"
	"github.com/roivaz/aro-hcp-intelhub/internal/traceimages"
)

// TraceImagesServiceAdapter bridges the MCP handler to the traceimages service.
type TraceImagesServiceAdapter struct {
	Service *traceimages.Service
}

func NewTraceImagesServiceAdapter(svc *traceimages.Service) *TraceImagesServiceAdapter {
	return &TraceImagesServiceAdapter{Service: svc}
}

func (a *TraceImagesServiceAdapter) TraceImages(ctx context.Context, commitSHA, environment string) (types.TraceImagesResponse, error) {
	if a.Service == nil {
		return types.TraceImagesResponse{}, fmt.Errorf("trace service not configured")
	}
	return a.Service.TraceImages(ctx, commitSHA, environment)
}
