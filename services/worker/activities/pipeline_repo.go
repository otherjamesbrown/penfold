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
		SourceID:        input.SourceID,
		Stage:           input.Stage,
		ModelID:         input.ModelID,
		PromptVersion:   input.PromptVersion,
		ConfigHash:      input.ConfigHash,
		Status:          input.Status,
		DurationMS:      input.DurationMS,
		InputData:       input.InputData,
		OutputData:      input.OutputData,
		ParsedData:      input.ParsedData,
		InputTokens:     input.InputTokens,
		OutputTokens:    input.OutputTokens,
		SkipReason:      input.SkipReason,
		LangfuseTraceID: input.LangfuseTraceID,
	})
}

// RecordOverrides records override parameters for a pipeline run.
func (a *pipelineRepositoryAdapter) RecordOverrides(ctx context.Context, runID int64, overrides map[string]string) error {
	return a.repo.RecordOverrides(ctx, runID, overrides)
}
