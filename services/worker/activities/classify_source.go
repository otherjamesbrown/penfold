// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"

	"github.com/otherjamesbrown/penfold/pkg/classify"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// LLMClassifier defines the interface for LLM-based source classification.
type LLMClassifier interface {
	ClassifySource(ctx context.Context, headers map[string]string, bodyPreview string) (*ClassifyResult, error)
}

// ClassifyResult represents the LLM classification result.
type ClassifyResult struct {
	SourceSystem string
	Pattern      string
	Confidence   float64
}

// ClassifySourceInput is the input for the ClassifySource activity.
type ClassifySourceInput struct {
	TenantID     string
	SourceID     int64
	ContentID    string
	SourceSystem string            // Current source_system value
	Headers      map[string]string // Email headers
	BodyPreview  string            // First 500 chars of body
	AutoApply    bool              // Whether to auto-apply high-confidence suggestions
}

// ClassifySourceOutput is the output from the ClassifySource activity.
type ClassifySourceOutput struct {
	SourceSystem         string
	Applied              bool
	SuggestionID         *int
	SuggestionConfidence *float64
}

// SuggestionRepository defines the interface for suggestion storage.
type SuggestionRepository interface {
	CreateSuggestion(ctx context.Context, s *classify.ClassificationSuggestion) error
}

// ClassifySourceActivities holds dependencies for source classification.
type ClassifySourceActivities struct {
	logger               logging.Logger
	classifier           LLMClassifier
	suggestionRepository SuggestionRepository
}

// NewClassifySourceActivities creates a new ClassifySourceActivities.
func NewClassifySourceActivities(
	logger logging.Logger,
	classifier LLMClassifier,
	suggestionRepository SuggestionRepository,
) *ClassifySourceActivities {
	return &ClassifySourceActivities{
		logger:               logger.With(logging.F("component", "classify_source_activities")),
		classifier:           classifier,
		suggestionRepository: suggestionRepository,
	}
}

// ClassifySource performs LLM-based source classification when rule-based classification fails.
// This activity:
// - Calls LLM with email headers + first 500 chars of body
// - Handles confidence gates (below 0.7: skip, 0.7-0.9: store as pending, >0.9: optionally auto-apply)
// - Stores suggestions via the suggestion repository
// - Is non-blocking on LLM errors (logs warning, returns empty result)
func (a *ClassifySourceActivities) ClassifySource(ctx context.Context, input ClassifySourceInput) (*ClassifySourceOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ClassifySource"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("content_id", input.ContentID),
		logging.F("current_source_system", input.SourceSystem),
	)

	// Record heartbeat for monitoring (safe — no-op outside Temporal context)
	recordHeartbeat(ctx, "starting LLM classification")

	logger.Info("Starting LLM-based source classification")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Call LLM classifier
	result, err := a.classifier.ClassifySource(ctx, input.Headers, input.BodyPreview)
	if err != nil {
		// Non-blocking: log warning and return empty output
		logger.Warn("LLM classification failed, continuing without suggestion",
			logging.Err(err),
		)
		return &ClassifySourceOutput{
			SourceSystem: input.SourceSystem,
			Applied:      false,
		}, nil
	}

	logger.Debug("LLM classification completed",
		logging.F("suggested_source_system", result.SourceSystem),
		logging.F("pattern", result.Pattern),
		logging.F("confidence", result.Confidence),
	)

	// Confidence gate: below 0.7, don't store suggestion
	if result.Confidence < 0.7 {
		logger.Info("LLM confidence below threshold, not storing suggestion",
			logging.F("confidence", result.Confidence),
			logging.F("threshold", 0.7),
		)
		return &ClassifySourceOutput{
			SourceSystem: input.SourceSystem,
			Applied:      false,
		}, nil
	}

	// Determine suggestion status based on confidence and auto-apply setting
	var status string
	var shouldApply bool

	if result.Confidence > 0.9 && input.AutoApply {
		// High confidence + auto-apply enabled: mark as applied
		status = "applied"
		shouldApply = true
	} else {
		// Medium confidence (0.7-0.9) OR high confidence with auto-apply off: pending
		status = "pending"
		shouldApply = false
	}

	logger.Info("Storing classification suggestion",
		logging.F("status", status),
		logging.F("will_apply", shouldApply),
	)

	// Create suggestion record
	suggestion := &classify.ClassificationSuggestion{
		ContentID:    input.ContentID,
		SourceSystem: result.SourceSystem,
		Pattern:      result.Pattern,
		Confidence:   result.Confidence,
		Status:       status,
	}

	// Store suggestion
	err = a.suggestionRepository.CreateSuggestion(ctx, suggestion)
	if err != nil {
		logger.Error("Failed to store classification suggestion",
			logging.Err(err),
		)
		return nil, fmt.Errorf("failed to store classification suggestion: %w", err)
	}

	logger.Info("Classification suggestion stored successfully",
		logging.F("suggestion_id", suggestion.ID),
		logging.F("status", status),
	)

	// Build output
	output := &ClassifySourceOutput{
		Applied:              shouldApply,
		SuggestionID:         &suggestion.ID,
		SuggestionConfidence: &result.Confidence,
	}

	// Set SourceSystem based on whether we applied the suggestion
	if shouldApply {
		output.SourceSystem = result.SourceSystem
	} else {
		output.SourceSystem = input.SourceSystem
	}

	return output, nil
}

// Ensure ClassifySourceActivities implements required interfaces at compile time.
var _ interface {
	ClassifySource(ctx context.Context, input ClassifySourceInput) (*ClassifySourceOutput, error)
} = (*ClassifySourceActivities)(nil)
