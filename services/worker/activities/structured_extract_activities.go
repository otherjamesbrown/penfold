// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"

	"go.temporal.io/sdk/temporal"
	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StructuredExtractActivities holds dependencies for the generic structured extraction activity.
type StructuredExtractActivities struct {
	db         *pgxpool.Pool
	aiClient   AIClient
	promptRepo PromptRepository
	logger     logging.Logger
}

// NewStructuredExtractActivities creates a new StructuredExtractActivities instance.
func NewStructuredExtractActivities(
	db *pgxpool.Pool,
	aiClient AIClient,
	promptRepo PromptRepository,
	logger logging.Logger,
) *StructuredExtractActivities {
	if db == nil {
		panic("NewStructuredExtractActivities: db is required")
	}
	if aiClient == nil {
		panic("NewStructuredExtractActivities: aiClient is required")
	}
	if promptRepo == nil {
		panic("NewStructuredExtractActivities: promptRepo is required")
	}
	if logger == nil {
		panic("NewStructuredExtractActivities: logger is required")
	}
	return &StructuredExtractActivities{
		db:         db,
		aiClient:   aiClient,
		promptRepo: promptRepo,
		logger:     logger.With(logging.F("component", "structured_extract_activities")),
	}
}

// StructuredExtractInput is the input for the StructuredExtract activity.
type StructuredExtractInput struct {
	TenantID          string `json:"tenant_id"`
	SourceID          int64  `json:"source_id"`
	Content           string `json:"content"`
	StageName         string `json:"stage_name"`
	PromptOverride    int32  `json:"prompt_override,omitempty"`
	BackgroundContext string `json:"background_context,omitempty"`
	LangfuseTraceID   string            `json:"langfuse_trace_id,omitempty"`
	LangfusePhaseID   string            `json:"langfuse_phase_id,omitempty"`
	ContextMeta       map[string]string `json:"context_meta,omitempty"` // metadata to attach to Langfuse generation
}

// StructuredExtractOutput is the output from the StructuredExtract activity.
type StructuredExtractOutput struct {
	RawJSON          json.RawMessage `json:"raw_json"`
	ModelUsed        string          `json:"model_used"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
	StageName        string          `json:"stage_name"`
}

// StructuredExtract calls the LLM with the prompt for the given stage and returns
// structured JSON. This is a generic replacement for NewsletterExtract and NotificationExtract.
func (a *StructuredExtractActivities) StructuredExtract(ctx context.Context, input StructuredExtractInput) (*StructuredExtractOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "StructuredExtract"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("stage_name", input.StageName),
		logging.F("content_length", len(input.Content)),
	)

	safeRecordHeartbeat(ctx, fmt.Sprintf("starting structured extraction for %s", input.StageName))

	logger.Info("Starting structured extraction")

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if input.Content == "" {
		return nil, temporal.NewApplicationError("content is empty", "ValidationError")
	}
	if input.StageName == "" {
		return nil, temporal.NewApplicationError("stage_name is required", "ValidationError")
	}

	// Load prompt template from DB. PromptOverride > 0 loads that specific version;
	// 0 (default) loads the active version.
	promptVersion := int(input.PromptOverride)
	promptTemplate, err := a.promptRepo.GetPromptByStage(ctx, input.StageName, promptVersion)
	if err != nil {
		return nil, fmt.Errorf("load %s prompt (version %d): %w", input.StageName, promptVersion, err)
	}

	// Render the prompt template with content and optional background context.
	tmpl, err := template.New(input.StageName).Parse(promptTemplate.Content)
	if err != nil {
		return nil, fmt.Errorf("parse %s prompt template: %w", input.StageName, err)
	}

	templateData := map[string]string{
		"Content": input.Content,
	}
	if input.BackgroundContext != "" {
		templateData["BackgroundContext"] = input.BackgroundContext
	}

	var renderedPrompt bytes.Buffer
	if err := tmpl.Execute(&renderedPrompt, templateData); err != nil {
		return nil, fmt.Errorf("render %s prompt: %w", input.StageName, err)
	}

	safeRecordHeartbeat(ctx, fmt.Sprintf("calling LLM for %s extraction", input.StageName))

	// Attach metadata for AI coordinator generation — always send stage name and context meta,
	// even when LangfuseTraceID is empty (the AI coordinator creates its own trace).
	callCtx := ctx
	{
		pairs := []string{
			"x-langfuse-stage-name", input.StageName,
		}
		if input.LangfuseTraceID != "" {
			pairs = append(pairs, "x-langfuse-trace-id", input.LangfuseTraceID)
			pairs = append(pairs, "x-langfuse-phase-id", input.LangfusePhaseID)
		}
		if input.ContextMeta != nil {
			for k, v := range input.ContextMeta {
				pairs = append(pairs, "x-langfuse-meta-"+k, v)
			}
		}
		md := metadata.Pairs(pairs...)
		callCtx = metadata.NewOutgoingContext(ctx, md)
	}

	// Call the AI coordinator with JSON mode enabled.
	jsonMode := true
	req := &aiv1.SummaryRequest{
		Content:  renderedPrompt.String(),
		JsonMode: &jsonMode,
	}

	resp, err := a.aiClient.GenerateSummary(callCtx, req)
	if err != nil {
		return nil, fmt.Errorf("%s LLM call: %w", input.StageName, err)
	}

	safeRecordHeartbeat(ctx, fmt.Sprintf("%s extraction complete", input.StageName))

	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	logger.Info("Structured extraction completed",
		logging.F("model_used", resp.ModelUsed),
		logging.F("input_tokens", inputTokens),
		logging.F("output_tokens", outputTokens),
		logging.F("response_length", len(resp.Summary)),
	)

	return &StructuredExtractOutput{
		RawJSON:          json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
		StageName:        input.StageName,
	}, nil
}

// Compile-time interface check
var _ interface {
	StructuredExtract(ctx context.Context, input StructuredExtractInput) (*StructuredExtractOutput, error)
} = (*StructuredExtractActivities)(nil)
