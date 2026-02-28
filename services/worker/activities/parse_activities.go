// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/parse"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// ParseActivities holds dependencies for content parsing activities.
// These are Stage 0 activities — deterministic, no AI calls.
type ParseActivities struct {
	logger       logging.Logger
	pipelineRepo PipelineRepository
}

// NewParseActivities creates a new ParseActivities instance.
// pipelineRepo is optional; if nil, pipeline run recording is skipped.
func NewParseActivities(logger logging.Logger, pipelineRepo ...PipelineRepository) *ParseActivities {
	if logger == nil {
		panic("NewParseActivities: logger is required")
	}
	var repo PipelineRepository
	if len(pipelineRepo) > 0 {
		repo = pipelineRepo[0]
	}
	return &ParseActivities{
		logger:       logger.With(logging.F("component", "parse_activities")),
		pipelineRepo: repo,
	}
}

// ParseEmail parses email body content, stripping HTML and separating quoted replies.
// This is a deterministic Stage 0 activity with no AI calls.
func (a *ParseActivities) ParseEmail(ctx context.Context, input workflows.ParseEmailInput) (*workflows.ParseEmailOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ParseEmail"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	logger.Info("Parsing email body")
	startTime := time.Now()

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}

	// Handle empty body - return empty output, not error
	if input.BodyText == "" && input.BodyHTML == "" {
		logger.Info("Empty email body, returning empty output")
		return &workflows.ParseEmailOutput{
			CleanBody:     "",
			NewContent:    "",
			QuotedContent: "",
			IsReply:       false,
		}, nil
	}

	// Call parse utility
	result := parse.ParseEmailBody(input.BodyText, input.BodyHTML)

	output := &workflows.ParseEmailOutput{
		CleanBody:     result.CleanBody,
		NewContent:    result.NewContent,
		QuotedContent: result.QuotedContent,
		IsReply:       result.QuotedReplyDetected,
	}

	logger.Info("Email parsing completed",
		logging.F("duration", time.Since(startTime)),
		logging.F("is_reply", output.IsReply),
		logging.F("clean_body_length", len(output.CleanBody)),
		logging.F("new_content_length", len(output.NewContent)),
	)

	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())
		inputJSON, _ := json.Marshal(map[string]interface{}{
			"body_text_length": len(input.BodyText),
			"body_html_length": len(input.BodyHTML),
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"clean_body_length": len(output.CleanBody),
			"is_reply":          output.IsReply,
		})
		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "parse",
			Status:     "completed",
			DurationMS: durationMS,
			InputData:  inputJSON,
			OutputData: outputJSON,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run for parse", logging.Err(runErr))
		}
	}

	return output, nil
}

// ParseTranscript parses transcript content, detecting format and extracting speaker-attributed text.
// This is a deterministic Stage 0 activity with no AI calls.
func (a *ParseActivities) ParseTranscript(ctx context.Context, input workflows.ParseTranscriptInput) (*workflows.ParseTranscriptOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ParseTranscript"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("format_hint", input.Format),
	)

	logger.Info("Parsing transcript content")
	startTime := time.Now()

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}

	// Handle empty content - return empty output, not error
	if input.Content == "" {
		logger.Info("Empty transcript content, returning empty output")
		return &workflows.ParseTranscriptOutput{
			CleanText:  "",
			Speakers:   []string{},
			DurationMs: 0,
			Format:     "",
		}, nil
	}

	// Call parse utility
	result, err := parse.ParseTranscript(input.Content)
	if err != nil {
		logger.Error("Failed to parse transcript", logging.Err(err))
		return nil, err
	}

	output := &workflows.ParseTranscriptOutput{
		CleanText:  result.CleanText,
		Speakers:   result.Speakers,
		DurationMs: result.DurationMs,
		Format:     result.Format,
	}

	logger.Info("Transcript parsing completed",
		logging.F("duration", time.Since(startTime)),
		logging.F("format", output.Format),
		logging.F("speaker_count", len(output.Speakers)),
		logging.F("duration_ms", output.DurationMs),
		logging.F("clean_text_length", len(output.CleanText)),
	)

	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())
		inputJSON, _ := json.Marshal(map[string]interface{}{
			"content_length": len(input.Content),
			"format_hint":    input.Format,
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"clean_text_length": len(output.CleanText),
			"speaker_count":     len(output.Speakers),
			"format":            output.Format,
		})
		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "parse",
			Status:     "completed",
			DurationMS: durationMS,
			InputData:  inputJSON,
			OutputData: outputJSON,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run for parse", logging.Err(runErr))
		}
	}

	return output, nil
}

// Ensure ParseActivities implements required interfaces at compile time.
var _ interface {
	ParseEmail(ctx context.Context, input workflows.ParseEmailInput) (*workflows.ParseEmailOutput, error)
	ParseTranscript(ctx context.Context, input workflows.ParseTranscriptInput) (*workflows.ParseTranscriptOutput, error)
} = (*ParseActivities)(nil)
