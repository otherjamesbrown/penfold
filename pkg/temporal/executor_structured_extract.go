package temporal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// StructuredExtractAIClient is the AI coordinator interface needed by StructuredExtractExecutor.
type StructuredExtractAIClient interface {
	GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error)
}

// StructuredExtractPromptRepo loads versioned prompt templates by stage name.
type StructuredExtractPromptRepo interface {
	GetPromptByStage(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error)
}

// StructuredExtractPersister writes structured JSON to content_enrichment.extracted_data.
// Returns true if an existing row was updated, false if no matching row was found.
type StructuredExtractPersister interface {
	PersistExtractedData(ctx context.Context, tenantID string, sourceID int64, key string, data json.RawMessage) (bool, error)
}

// StructuredExtractExecutor implements StageExecutor for stages with stage_kind = "structured_extract".
// It follows the pattern established in services/worker/workflows/pipeline.go:2521-2655:
// build context → call LLM → persist to extracted_data[persist_key].
type StructuredExtractExecutor struct {
	aiClient   StructuredExtractAIClient
	promptRepo StructuredExtractPromptRepo
	persister  StructuredExtractPersister
}

// NewStructuredExtractExecutor creates a StructuredExtractExecutor with all required dependencies.
func NewStructuredExtractExecutor(
	aiClient StructuredExtractAIClient,
	promptRepo StructuredExtractPromptRepo,
	persister StructuredExtractPersister,
) *StructuredExtractExecutor {
	if aiClient == nil {
		panic("NewStructuredExtractExecutor: aiClient is required")
	}
	if promptRepo == nil {
		panic("NewStructuredExtractExecutor: promptRepo is required")
	}
	if persister == nil {
		panic("NewStructuredExtractExecutor: persister is required")
	}
	return &StructuredExtractExecutor{
		aiClient:   aiClient,
		promptRepo: promptRepo,
		persister:  persister,
	}
}

// backgroundContextCarrier is used to unmarshal a background_context field from a
// previous stage's RawData. Stages that assemble context (e.g. build_extraction_context)
// embed this field so StructuredExtractExecutor can find it.
type backgroundContextCarrier struct {
	BackgroundContext string `json:"background_context"`
}

// Execute implements StageExecutor for stage_kind = "structured_extract".
//
// It:
//  1. Validates inputs (content, stage name)
//  2. Extracts any pre-assembled background context from previous stage outputs
//  3. Loads and renders the prompt template (PromptOverride=0 → active version)
//  4. Calls the AI coordinator in JSON mode (ModelOverride applied when set)
//  5. Persists the result to extracted_data[PersistKey] (non-fatal on failure)
//  6. Returns StageOutput with the extracted JSON in RawData
func (e *StructuredExtractExecutor) Execute(ctx context.Context, stage PipelineStageConfig, input StageInput) (StageOutput, error) {
	if input.Content == "" {
		return StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     "content is empty",
		}, fmt.Errorf("structured_extract %s: content is empty", stage.Stage)
	}
	if stage.Stage == "" {
		return StageOutput{
			Success:   false,
			StageKind: stage.StageKind,
			Error:     "stage name is required",
		}, fmt.Errorf("structured_extract: stage name is required")
	}

	// Extract background context from previous stage outputs. A context-building stage
	// (e.g. build_extraction_context) places a "background_context" field in its RawData.
	// Falls back to empty string if no such output is present.
	bgContext := extractBackgroundContext(input.PreviousOutputs)

	// Load the prompt template. PromptOverride=0 loads the active version.
	promptVersion := int(stage.PromptOverride)
	tmplRecord, err := e.promptRepo.GetPromptByStage(ctx, stage.Stage, promptVersion)
	if err != nil {
		return StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     fmt.Sprintf("load prompt (version %d): %v", promptVersion, err),
		}, fmt.Errorf("structured_extract %s: load prompt (version %d): %w", stage.Stage, promptVersion, err)
	}

	// Render the prompt template with document content and optional background context.
	tmpl, err := template.New(stage.Stage).Parse(tmplRecord.Content)
	if err != nil {
		return StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     fmt.Sprintf("parse prompt template: %v", err),
		}, fmt.Errorf("structured_extract %s: parse prompt template: %w", stage.Stage, err)
	}

	templateData := map[string]string{
		"Content": input.Content,
	}
	if bgContext != "" {
		templateData["BackgroundContext"] = bgContext
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, templateData); err != nil {
		return StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     fmt.Sprintf("render prompt template: %v", err),
		}, fmt.Errorf("structured_extract %s: render prompt template: %w", stage.Stage, err)
	}

	// Attach Langfuse tracing metadata to the gRPC call context when available.
	callCtx := ctx
	if input.LangfuseTraceID != "" {
		md := metadata.Pairs("x-langfuse-trace-id", input.LangfuseTraceID)
		callCtx = metadata.NewOutgoingContext(ctx, md)
	}

	// Build the AI request. Apply ModelOverride when set.
	jsonMode := true
	req := &aiv1.SummaryRequest{
		Content:  rendered.String(),
		JsonMode: &jsonMode,
	}
	if stage.ModelOverride != "" {
		req.Model = &stage.ModelOverride
	}

	resp, err := e.aiClient.GenerateSummary(callCtx, req)
	if err != nil {
		return StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     fmt.Sprintf("LLM call: %v", err),
		}, fmt.Errorf("structured_extract %s: LLM call: %w", stage.Stage, err)
	}

	rawJSON := json.RawMessage(resp.Summary)

	// Determine persist key: PersistKey from config, or strip the _extract suffix as fallback.
	persistKey := stage.PersistKey
	if persistKey == "" {
		persistKey = strings.TrimSuffix(stage.Stage, "_extract")
	}

	// Persist result to extracted_data[persistKey]. This is non-blocking — a persist failure
	// does not fail the stage, matching the pipeline.go:2608-2613 non-blocking pattern.
	_, _ = e.persister.PersistExtractedData(ctx, input.TenantID, input.SourceID, persistKey, rawJSON)

	return StageOutput{
		Success:   true,
		StageName: stage.Stage,
		StageKind: stage.StageKind,
		RawData:   rawJSON,
	}, nil
}

// extractBackgroundContext searches previous stage outputs for a pre-assembled background
// context string. Returns the first non-empty value found, or empty string if none.
func extractBackgroundContext(outputs map[string]StageOutput) string {
	for _, output := range outputs {
		if !output.Success || len(output.RawData) == 0 {
			continue
		}
		var carrier backgroundContextCarrier
		if err := json.Unmarshal(output.RawData, &carrier); err == nil && carrier.BackgroundContext != "" {
			return carrier.BackgroundContext
		}
	}
	return ""
}

// Compile-time interface check.
var _ StageExecutor = (*StructuredExtractExecutor)(nil)
