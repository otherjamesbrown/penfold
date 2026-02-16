// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/temporal"

	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// EnrichmentActivities holds dependencies for enrichment record creation activities.
type EnrichmentActivities struct {
	logger           logging.Logger
	enrichmentRepo   *enrichment.Repository
}

// NewEnrichmentActivities creates a new EnrichmentActivities instance.
func NewEnrichmentActivities(
	logger logging.Logger,
	enrichmentRepo *enrichment.Repository,
) *EnrichmentActivities {
	if logger == nil {
		panic("NewEnrichmentActivities: logger is required")
	}
	if enrichmentRepo == nil {
		panic("NewEnrichmentActivities: enrichmentRepo is required")
	}
	return &EnrichmentActivities{
		logger:         logger.With(logging.F("component", "enrichment_activities")),
		enrichmentRepo: enrichmentRepo,
	}
}

// CreateEnrichmentRecordInput contains the data needed to create a content_enrichment record.
type CreateEnrichmentRecordInput struct {
	SourceID                   int64                       `json:"source_id"`
	TenantID                   string                      `json:"tenant_id"`
	ContentType                enrichment.ContentType      `json:"content_type"`
	ContentSubtype             enrichment.ContentSubtype   `json:"content_subtype"`
	SourceSystem               enrichment.SourceSystem     `json:"source_system"`
	ProcessingProfile          enrichment.ProcessingProfile `json:"processing_profile"`
	ClassificationConfidence   float32                     `json:"classification_confidence"`
	ClassificationReason       string                      `json:"classification_reason"`
}

// CreateEnrichmentRecordOutput contains the result of creating a content_enrichment record.
type CreateEnrichmentRecordOutput struct {
	EnrichmentID int64 `json:"enrichment_id"`
}

// CreateEnrichmentRecord creates a content_enrichment record for a source after triage.
// This activity should be called after the Triage stage completes.
func (a *EnrichmentActivities) CreateEnrichmentRecord(ctx context.Context, input CreateEnrichmentRecordInput) (*CreateEnrichmentRecordOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "CreateEnrichmentRecord"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting create enrichment record")

	logger.Info("Creating enrichment record",
		logging.F("content_type", input.ContentType),
		logging.F("content_subtype", input.ContentSubtype),
		logging.F("source_system", input.SourceSystem),
		logging.F("processing_profile", input.ProcessingProfile),
	)

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
	if input.TenantID == "" {
		return nil, temporal.NewApplicationError(
			"tenant_id is required",
			"ValidationError",
		)
	}
	if input.ContentType == "" {
		return nil, temporal.NewApplicationError(
			"content_type is required",
			"ValidationError",
		)
	}
	if input.ContentSubtype == "" {
		return nil, temporal.NewApplicationError(
			"content_subtype is required",
			"ValidationError",
		)
	}
	if input.ProcessingProfile == "" {
		return nil, temporal.NewApplicationError(
			"processing_profile is required",
			"ValidationError",
		)
	}

	// Create enrichment struct
	e := &enrichment.Enrichment{
		SourceID: input.SourceID,
		TenantID: input.TenantID,
		Classification: enrichment.Classification{
			ContentType: input.ContentType,
			Subtype:     input.ContentSubtype,
			Profile:     input.ProcessingProfile,
			Confidence:  input.ClassificationConfidence,
			Reason:      input.ClassificationReason,
		},
		SourceSystem:         input.SourceSystem,
		Status:               enrichment.StatusPending,
		CurrentStage:         "triage_complete",
		RetryCount:           0,
		Participants:         []enrichment.Participant{},
		ResolvedParticipants: []enrichment.ResolvedParticipant{},
		ExtractedLinks:       []enrichment.ExtractedLink{},
		ExtractedData:        make(map[string]interface{}),
		AIProcessed:          false,
	}

	// Call repository Create method
	err := a.enrichmentRepo.Create(ctx, e)
	if err != nil {
		logger.Error("Failed to create enrichment record", logging.Err(err))
		return nil, temporal.NewApplicationErrorWithCause(
			fmt.Sprintf("failed to create enrichment record for source %d", input.SourceID),
			"RepositoryError",
			err,
		)
	}

	// Record heartbeat after processing
	recordHeartbeat(ctx, "create enrichment record complete")

	logger.Info("Enrichment record created successfully",
		logging.F("enrichment_id", e.ID),
	)

	return &CreateEnrichmentRecordOutput{
		EnrichmentID: e.ID,
	}, nil
}

// Ensure EnrichmentActivities implements required interfaces at compile time.
var _ interface {
	CreateEnrichmentRecord(ctx context.Context, input CreateEnrichmentRecordInput) (*CreateEnrichmentRecordOutput, error)
} = (*EnrichmentActivities)(nil)
