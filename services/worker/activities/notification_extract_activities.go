// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationExtractActivities holds dependencies for notification structured extraction.
type NotificationExtractActivities struct {
	db         *pgxpool.Pool
	aiClient   AIClient
	promptRepo *pipeline.Repository
	logger     logging.Logger
}

// NewNotificationExtractActivities creates a new NotificationExtractActivities instance.
func NewNotificationExtractActivities(
	db *pgxpool.Pool,
	aiClient AIClient,
	promptRepo *pipeline.Repository,
	logger logging.Logger,
) *NotificationExtractActivities {
	if db == nil {
		panic("NewNotificationExtractActivities: db is required")
	}
	if aiClient == nil {
		panic("NewNotificationExtractActivities: aiClient is required")
	}
	if promptRepo == nil {
		panic("NewNotificationExtractActivities: promptRepo is required")
	}
	if logger == nil {
		panic("NewNotificationExtractActivities: logger is required")
	}
	return &NotificationExtractActivities{
		db:         db,
		aiClient:   aiClient,
		promptRepo: promptRepo,
		logger:     logger.With(logging.F("component", "notification_extract_activities")),
	}
}

// NotificationExtractInput is the input for the NotificationExtract activity.
type NotificationExtractInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Content  string `json:"content"`
}

// NotificationExtractOutput is the output from the NotificationExtract activity.
// RawJSON contains the structured JSON from the LLM (notification schema).
type NotificationExtractOutput struct {
	RawJSON          json.RawMessage `json:"raw_json"`
	ModelUsed        string          `json:"model_used"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// NotificationExtract calls the LLM with the notification_extract prompt and returns
// structured JSON. The result is stored in content_enrichment.extracted_data['notification']
// by the subsequent PersistExtractedData activity.
func (a *NotificationExtractActivities) NotificationExtract(ctx context.Context, input NotificationExtractInput) (*NotificationExtractOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "NotificationExtract"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("content_length", len(input.Content)),
	)

	activity.RecordHeartbeat(ctx, "starting notification extraction")

	logger.Info("Starting notification structured extraction")

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if input.Content == "" {
		return nil, temporal.NewApplicationError("content is empty", "ValidationError")
	}

	// Load notification_extract prompt template from DB (version 0 = active).
	promptTemplate, err := a.promptRepo.GetPromptByStage(ctx, "notification_extract", 0)
	if err != nil {
		return nil, fmt.Errorf("load notification_extract prompt: %w", err)
	}

	// Render the prompt template with content.
	tmpl, err := template.New("notification_extract").Parse(promptTemplate.Content)
	if err != nil {
		return nil, fmt.Errorf("parse notification_extract prompt template: %w", err)
	}

	var renderedPrompt bytes.Buffer
	if err := tmpl.Execute(&renderedPrompt, map[string]string{
		"Content": input.Content,
	}); err != nil {
		return nil, fmt.Errorf("render notification_extract prompt: %w", err)
	}

	activity.RecordHeartbeat(ctx, "calling LLM for notification extraction")

	// Call the AI coordinator with JSON mode enabled.
	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  renderedPrompt.String(),
		JsonMode: &jsonMode,
	})
	if err != nil {
		return nil, fmt.Errorf("notification_extract LLM call: %w", err)
	}

	activity.RecordHeartbeat(ctx, "notification extraction complete")

	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	logger.Info("Notification extraction completed",
		logging.F("model_used", resp.ModelUsed),
		logging.F("input_tokens", inputTokens),
		logging.F("output_tokens", outputTokens),
		logging.F("response_length", len(resp.Summary)),
	)

	return &NotificationExtractOutput{
		RawJSON:          json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
	}, nil
}

// Ensure NotificationExtractActivities implements required interfaces at compile time.
var _ interface {
	NotificationExtract(ctx context.Context, input NotificationExtractInput) (*NotificationExtractOutput, error)
} = (*NotificationExtractActivities)(nil)

// Ensure we have time imported.
var _ = time.Now
