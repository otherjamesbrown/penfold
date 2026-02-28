// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// PersistActivities holds dependencies for Stage 4.5 persistence activities.
type PersistActivities struct {
	logger       logging.Logger
	repository   PersistRepository
	pipelineRepo PipelineRepository
}

// NewPersistActivities creates a new PersistActivities instance.
func NewPersistActivities(
	logger logging.Logger,
	repository PersistRepository,
	pipelineRepo PipelineRepository,
) *PersistActivities {
	if logger == nil {
		panic("NewPersistActivities: logger is required")
	}
	if repository == nil {
		panic("NewPersistActivities: repository is required")
	}
	// pipelineRepo is optional (provenance recording)
	return &PersistActivities{
		logger:       logger.With(logging.F("component", "persist_activities")),
		repository:   repository,
		pipelineRepo: pipelineRepo,
	}
}

// PersistFindings persists findings from Stage 4 deep analysis to the database.
// It validates the input, calls the repository to persist the data in a transaction,
// and returns statistics about the persisted records.
func (a *PersistActivities) PersistFindings(ctx context.Context, input workflows.PersistFindingsInput) (*workflows.PersistFindingsOutput, error) {
	// Set trace_id in context for log correlation
	// Note: PersistFindingsActivityInput doesn't have ContentID, so we use source_id
	ctx = context.WithValue(ctx, logging.TraceIDKey, fmt.Sprintf("source-%d", input.SourceID))
	startTime := time.Now()
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "PersistFindings"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting persist findings")

	logger.Info("Starting persist findings")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.SourceID <= 0 {
		return nil, temporal.NewApplicationError(
			"source_id must be positive",
			"ValidationError",
		)
	}

	// Lightweight persist: no analysis output (notification/newsletter pipelines).
	// Record the pipeline_run as completed and return zero counts.
	if input.Analysis == nil {
		logger.Info("Lightweight persist (no analysis output)")
		output := &workflows.PersistFindingsOutput{}
		if a.pipelineRepo != nil {
			durationMS := int(time.Since(startTime).Milliseconds())
			inputJSON, _ := json.Marshal(map[string]interface{}{
				"source_id":    input.SourceID,
				"has_analysis": false,
				"lightweight":  true,
			})
			outputJSON, _ := json.Marshal(output)
			runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
				SourceID:   input.SourceID,
				Stage:      "persist",
				Status:     "completed",
				DurationMS: durationMS,
				InputData:  inputJSON,
				OutputData: outputJSON,
			})
			if runErr != nil {
				logger.Warn("Failed to record pipeline run for lightweight persist", logging.Err(runErr))
			}
		}
		return output, nil
	}

	// Check if repository is available
	if a.repository == nil {
		logger.Warn("Persist repository not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"persist repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Build repository input from activity input
	repoInput := &PersistFindingsInput{
		TenantID:       input.TenantID,
		SourceID:       input.SourceID,
		ThreadID:       input.ThreadID,
		ProjectID:      input.ProjectID,
		Analysis:       input.Analysis,
		ResolvedPeople: input.ResolvedPeople,
		BodyText:       input.BodyText,
		Subject:        input.Subject,
	}

	// Call repository to persist findings
	repoOutput, err := a.repository.PersistFindings(ctx, repoInput)
	if err != nil {
		pe := perrors.ClassifyError(err, "persist")
		logger.Error("Failed to persist findings", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	// Build activity output
	output := &workflows.PersistFindingsOutput{
		AssertionsCreated:      repoOutput.AssertionsCreated,
		AssertionsSuperseded:   repoOutput.AssertionsSuperseded,
		AssertionsDeduplicated: repoOutput.AssertionsDeduplicated,
		ReferencesCreated:      repoOutput.ReferencesCreated,
		ReviewItemsCreated:     repoOutput.ReviewItemsCreated,
		AffinityUpdates:        repoOutput.AffinityUpdates,
	}

	// Record heartbeat after processing
	recordHeartbeat(ctx, "persist findings complete")

	logger.Info("Persist findings completed successfully",
		logging.F("assertions_created", output.AssertionsCreated),
		logging.F("assertions_superseded", output.AssertionsSuperseded),
		logging.F("assertions_deduplicated", output.AssertionsDeduplicated),
		logging.F("references_created", output.ReferencesCreated),
		logging.F("review_items_created", output.ReviewItemsCreated),
		logging.F("affinity_updates", output.AffinityUpdates),
	)

	// Record pipeline run for provenance tracking (Stage 4.5: persist)
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())

		// Capture IO data for code-only stage
		inputJSON, _ := json.Marshal(map[string]interface{}{
			"source_id": input.SourceID,
			"has_analysis": input.Analysis != nil,
			"has_thread_id": input.ThreadID != nil,
			"has_project_id": input.ProjectID != nil,
			"resolved_people_count": len(input.ResolvedPeople),
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"assertions_created": repoOutput.AssertionsCreated,
			"assertions_superseded": repoOutput.AssertionsSuperseded,
			"assertions_deduplicated": repoOutput.AssertionsDeduplicated,
			"references_created": repoOutput.ReferencesCreated,
			"review_items_created": repoOutput.ReviewItemsCreated,
			"affinity_updates": repoOutput.AffinityUpdates,
		})
		parsedJSON, _ := json.Marshal(output)

		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "persist",
			ModelID:    "", // code-only stage
			Status:     "completed",
			DurationMS: durationMS,
			InputData:  inputJSON,
			OutputData: outputJSON,
			ParsedData: parsedJSON,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run", logging.Err(runErr))
		}
	}

	return output, nil
}

// Ensure PersistActivities implements required interfaces at compile time.
var _ interface {
	PersistFindings(ctx context.Context, input workflows.PersistFindingsInput) (*workflows.PersistFindingsOutput, error)
} = (*PersistActivities)(nil)
