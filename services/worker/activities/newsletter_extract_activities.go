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

// NewsletterExtractActivities holds dependencies for newsletter structured extraction.
type NewsletterExtractActivities struct {
	db         *pgxpool.Pool
	aiClient   AIClient
	promptRepo *pipeline.Repository
	logger     logging.Logger
}

// NewNewsletterExtractActivities creates a new NewsletterExtractActivities instance.
func NewNewsletterExtractActivities(
	db *pgxpool.Pool,
	aiClient AIClient,
	promptRepo *pipeline.Repository,
	logger logging.Logger,
) *NewsletterExtractActivities {
	if db == nil {
		panic("NewNewsletterExtractActivities: db is required")
	}
	if aiClient == nil {
		panic("NewNewsletterExtractActivities: aiClient is required")
	}
	if promptRepo == nil {
		panic("NewNewsletterExtractActivities: promptRepo is required")
	}
	if logger == nil {
		panic("NewNewsletterExtractActivities: logger is required")
	}
	return &NewsletterExtractActivities{
		db:         db,
		aiClient:   aiClient,
		promptRepo: promptRepo,
		logger:     logger.With(logging.F("component", "newsletter_extract_activities")),
	}
}

// NewsletterExtractInput is the input for the NewsletterExtract activity.
type NewsletterExtractInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Content  string `json:"content"`
}

// NewsletterExtractOutput is the output from the NewsletterExtract activity.
// RawJSON contains the structured JSON from the LLM (newsletter schema).
type NewsletterExtractOutput struct {
	RawJSON          json.RawMessage `json:"raw_json"`
	ModelUsed        string          `json:"model_used"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// NewsletterExtract calls the LLM with the newsletter_extract prompt and returns
// structured JSON. The result is stored in content_enrichment.extracted_data['newsletter']
// by the subsequent PersistExtractedData activity.
func (a *NewsletterExtractActivities) NewsletterExtract(ctx context.Context, input NewsletterExtractInput) (*NewsletterExtractOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "NewsletterExtract"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("content_length", len(input.Content)),
	)

	activity.RecordHeartbeat(ctx, "starting newsletter extraction")

	logger.Info("Starting newsletter structured extraction")

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if input.Content == "" {
		return nil, temporal.NewApplicationError("content is empty", "ValidationError")
	}

	// Load newsletter_extract prompt template from DB (version 0 = active).
	promptTemplate, err := a.promptRepo.GetPromptByStage(ctx, "newsletter_extract", 0)
	if err != nil {
		return nil, fmt.Errorf("load newsletter_extract prompt: %w", err)
	}

	// Render the prompt template with content.
	tmpl, err := template.New("newsletter_extract").Parse(promptTemplate.Content)
	if err != nil {
		return nil, fmt.Errorf("parse newsletter_extract prompt template: %w", err)
	}

	var renderedPrompt bytes.Buffer
	if err := tmpl.Execute(&renderedPrompt, map[string]string{
		"Content": input.Content,
	}); err != nil {
		return nil, fmt.Errorf("render newsletter_extract prompt: %w", err)
	}

	activity.RecordHeartbeat(ctx, "calling LLM for newsletter extraction")

	// Call the AI coordinator with JSON mode enabled.
	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  renderedPrompt.String(),
		JsonMode: &jsonMode,
	})
	if err != nil {
		return nil, fmt.Errorf("newsletter_extract LLM call: %w", err)
	}

	activity.RecordHeartbeat(ctx, "newsletter extraction complete")

	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	logger.Info("Newsletter extraction completed",
		logging.F("model_used", resp.ModelUsed),
		logging.F("input_tokens", inputTokens),
		logging.F("output_tokens", outputTokens),
		logging.F("response_length", len(resp.Summary)),
	)

	return &NewsletterExtractOutput{
		RawJSON:          json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
	}, nil
}

// PersistExtractedDataInput is the input for the PersistExtractedData activity.
type PersistExtractedDataInput struct {
	TenantID string          `json:"tenant_id"`
	SourceID int64           `json:"source_id"`
	Key      string          `json:"key"`      // JSONB key, e.g. "newsletter"
	Data     json.RawMessage `json:"data"`     // JSON value to store under the key
}

// PersistExtractedDataOutput is the output from the PersistExtractedData activity.
type PersistExtractedDataOutput struct {
	Updated bool `json:"updated"`
}

// PersistExtractedData writes structured JSON data to content_enrichment.extracted_data
// under the given key via a JSONB merge. Creates the enrichment record if it does not exist.
func (a *NewsletterExtractActivities) PersistExtractedData(ctx context.Context, input PersistExtractedDataInput) (*PersistExtractedDataOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "PersistExtractedData"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("key", input.Key),
	)

	activity.RecordHeartbeat(ctx, "persisting extracted data")

	logger.Info("Persisting extracted data to content_enrichment")

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if input.SourceID <= 0 {
		return nil, temporal.NewApplicationError("source_id must be positive", "ValidationError")
	}
	if input.Key == "" {
		return nil, temporal.NewApplicationError("key must not be empty", "ValidationError")
	}
	if len(input.Data) == 0 {
		return nil, temporal.NewApplicationError("data must not be empty", "ValidationError")
	}

	// Merge the new key into extracted_data using JSONB concatenation.
	// This preserves any existing keys and adds/overwrites the specified key.
	query := `
		UPDATE content_enrichment
		SET extracted_data = COALESCE(extracted_data, '{}')::jsonb || jsonb_build_object($1, $2::jsonb),
		    updated_at = NOW()
		WHERE source_id = $3 AND tenant_id = $4
	`

	result, err := a.db.Exec(ctx, query, input.Key, string(input.Data), input.SourceID, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("persist extracted_data[%s]: %w", input.Key, err)
	}

	updated := result.RowsAffected() > 0
	if !updated {
		logger.Warn("No content_enrichment row found for source",
			logging.F("source_id", input.SourceID),
			logging.F("tenant_id", input.TenantID),
		)
		return &PersistExtractedDataOutput{Updated: false}, nil
	}

	logger.Info("Extracted data persisted successfully",
		logging.F("key", input.Key),
		logging.F("data_length", len(input.Data)),
	)

	return &PersistExtractedDataOutput{Updated: true}, nil
}

// Ensure NewsletterExtractActivities implements required interfaces at compile time.
var _ interface {
	NewsletterExtract(ctx context.Context, input NewsletterExtractInput) (*NewsletterExtractOutput, error)
	PersistExtractedData(ctx context.Context, input PersistExtractedDataInput) (*PersistExtractedDataOutput, error)
} = (*NewsletterExtractActivities)(nil)

// Ensure we have time imported.
var _ = time.Now
