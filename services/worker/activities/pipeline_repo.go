// Package activities provides repository wrappers for activity implementations.
package activities

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// pipelineRepositoryAdapter adapts pkg/pipeline.Repository to activities.PipelineRepository.
type pipelineRepositoryAdapter struct {
	repo *pipeline.Repository
}

// NewPipelineRepository creates a new pipeline repository for recording pipeline runs.
func NewPipelineRepository(db *pgxpool.Pool) PipelineRepository {
	return &pipelineRepositoryAdapter{
		repo: pipeline.NewRepository(db),
	}
}

// CreateRun records a pipeline run for provenance tracking.
func (a *pipelineRepositoryAdapter) CreateRun(ctx context.Context, input PipelineRunInput) error {
	return a.repo.CreateRun(ctx, pipeline.PipelineRunInput{
		SourceID:      input.SourceID,
		Stage:         input.Stage,
		ModelID:       input.ModelID,
		PromptVersion: input.PromptVersion,
		ConfigHash:    input.ConfigHash,
		Status:        input.Status,
		DurationMS:    input.DurationMS,
	})
}
