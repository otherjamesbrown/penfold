package pipeline

import (
	"context"
	"fmt"
	"strings"
)

// PipelineLister can list all pipeline definitions for a tenant.
type PipelineLister interface {
	ListPipelines(ctx context.Context, tenantID string) ([]PipelineDefinition, error)
}

// ValidateAllDefinitions checks that every pipeline definition for tenantID has
// at least one enabled stage. Returns an error naming every invalid pipeline.
//
// Called at worker startup to fail fast if DB configuration is broken or incomplete.
func ValidateAllDefinitions(ctx context.Context, lister PipelineLister, tenantID string) error {
	defs, err := lister.ListPipelines(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("loading pipeline definitions for tenant %s: %w", tenantID, err)
	}

	var invalid []string
	for _, def := range defs {
		hasEnabled := false
		for _, stage := range def.Stages {
			if stage.Enabled {
				hasEnabled = true
				break
			}
		}
		if !hasEnabled {
			invalid = append(invalid, fmt.Sprintf("pipeline=%s tenant=%s", def.Pipeline, tenantID))
		}
	}

	if len(invalid) > 0 {
		return fmt.Errorf("pipeline definitions have no enabled stages: %s", strings.Join(invalid, "; "))
	}
	return nil
}
