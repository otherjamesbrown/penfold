// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/parse"
)

// ParseActivities holds dependencies for content parsing activities.
// These are Stage 0 activities — deterministic, no AI calls.
type ParseActivities struct {
	logger logging.Logger
}

// NewParseActivities creates a new ParseActivities instance.
func NewParseActivities(logger logging.Logger) *ParseActivities {
	if logger == nil {
		panic("NewParseActivities: logger is required")
	}
	return &ParseActivities{
		logger: logger.With(logging.F("component", "parse_activities")),
	}
}

// ParseEmailInput is the input for the ParseEmail activity.
type ParseEmailInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	BodyText string `json:"body_text"`
	BodyHTML string `json:"body_html"`
}

// ParseEmailOutput is the output from the ParseEmail activity.
type ParseEmailOutput struct {
	CleanBody     string `json:"clean_body"`
	NewContent    string `json:"new_content"`
	QuotedContent string `json:"quoted_content"`
	IsReply       bool   `json:"is_reply"`
}

// ParseEmail parses email body content, stripping HTML and separating quoted replies.
// This is a deterministic Stage 0 activity with no AI calls.
func (a *ParseActivities) ParseEmail(ctx context.Context, input ParseEmailInput) (*ParseEmailOutput, error) {
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
		return &ParseEmailOutput{
			CleanBody:     "",
			NewContent:    "",
			QuotedContent: "",
			IsReply:       false,
		}, nil
	}

	// Call parse utility
	result := parse.ParseEmailBody(input.BodyText, input.BodyHTML)

	output := &ParseEmailOutput{
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

	return output, nil
}

// ParseTranscriptInput is the input for the ParseTranscript activity.
type ParseTranscriptInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Content  string `json:"content"`
	Format   string `json:"format,omitempty"` // hint: vtt, srt, txt (auto-detect if empty)
}

// ParseTranscriptOutput is the output from the ParseTranscript activity.
type ParseTranscriptOutput struct {
	CleanText  string   `json:"clean_text"`
	Speakers   []string `json:"speakers"`
	DurationMs int      `json:"duration_ms"`
	Format     string   `json:"format"`
}

// ParseTranscript parses transcript content, detecting format and extracting speaker-attributed text.
// This is a deterministic Stage 0 activity with no AI calls.
func (a *ParseActivities) ParseTranscript(ctx context.Context, input ParseTranscriptInput) (*ParseTranscriptOutput, error) {
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
		return &ParseTranscriptOutput{
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

	output := &ParseTranscriptOutput{
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

	return output, nil
}

// Ensure ParseActivities implements required interfaces at compile time.
var _ interface {
	ParseEmail(ctx context.Context, input ParseEmailInput) (*ParseEmailOutput, error)
	ParseTranscript(ctx context.Context, input ParseTranscriptInput) (*ParseTranscriptOutput, error)
} = (*ParseActivities)(nil)
