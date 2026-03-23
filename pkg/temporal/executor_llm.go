package temporal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"

	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// LLMAIClient is the AI coordinator interface needed by LLMStageExecutor.
// It mirrors the subset of AIClient used for template-driven LLM generation.
type LLMAIClient interface {
	GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error)
}

// LLMPromptRepo loads versioned prompt templates by stage name.
// Version 0 requests the currently active version.
type LLMPromptRepo interface {
	GetPromptByStage(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error)
}

// LLMStageExecutor implements StageExecutor for stages with stage_kind = "llm".
//
// It handles the five LLM pipeline stages (triage, extract_semantic, extract_ner,
// analyze, summarize) using a template-driven approach: each stage's prompt
// template is loaded from the database and rendered against the document content
// and any previous stage outputs. The rendered prompt is sent to the AI coordinator
// via GenerateSummary in JSON mode.
//
// Context assembly is data-driven, not stage-name-driven. The prompt template
// references previous outputs by their stage name (e.g. {{.triage}}) — the executor
// exposes all successful PreviousOutputs to the template without branching on stage
// identity. This makes the executor generic across all LLM stage_kinds.
//
// Register it under "llm" in the ExecutorRegistry:
//
//	registry.Register("llm", NewLLMStageExecutor(aiClient, promptRepo))
type LLMStageExecutor struct {
	aiClient   LLMAIClient
	promptRepo LLMPromptRepo
}

// NewLLMStageExecutor creates an LLMStageExecutor with the required dependencies.
// Panics on nil dependencies to catch configuration errors early.
func NewLLMStageExecutor(aiClient LLMAIClient, promptRepo LLMPromptRepo) *LLMStageExecutor {
	if aiClient == nil {
		panic("NewLLMStageExecutor: aiClient is required")
	}
	if promptRepo == nil {
		panic("NewLLMStageExecutor: promptRepo is required")
	}
	return &LLMStageExecutor{
		aiClient:   aiClient,
		promptRepo: promptRepo,
	}
}

// Execute implements StageExecutor for stage_kind = "llm".
//
// It:
//  1. Skips the stage when stage.SkipWhenLow is set and input.SkipDeep is true
//  2. Validates that stage name and content are non-empty
//  3. Builds template data from input.Content and all successful PreviousOutputs
//  4. Loads the prompt template (PromptOverride=0 → active version)
//  5. Renders the template against the assembled data
//  6. Calls the AI coordinator in JSON mode (ModelOverride applied when set)
//  7. Returns StageOutput with the LLM response in RawData
//
// The stage name is used only to load the prompt — there is no branching on
// stage identity inside Execute. Stage-specific behaviour lives in the template.
func (e *LLMStageExecutor) Execute(ctx context.Context, stage PipelineStageConfig, input StageInput) (StageOutput, error) {
	// Skip when triage signalled a low-signal document and this stage is opt-out.
	if stage.SkipWhenLow && input.SkipDeep {
		return StageOutput{
			Success:    true,
			Skipped:    true,
			SkipReason: "skip_deep",
			StageName:  stage.Stage,
			StageKind:  stage.StageKind,
		}, nil
	}

	if stage.Stage == "" {
		return StageOutput{
			Success:   false,
			StageKind: stage.StageKind,
			Error:     "stage name is required",
		}, fmt.Errorf("llm executor: stage name is required")
	}
	if input.Content == "" {
		return StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     "content is empty",
		}, fmt.Errorf("llm %s: content is empty", stage.Stage)
	}

	// Build template data: Content + all successful previous stage outputs.
	// Previous outputs are unmarshalled to interface{} so templates can traverse
	// them with dot notation (e.g. {{.triage}}, {{.extract_ner}}).
	templateData := buildLLMTemplateData(input)

	// Load the prompt template. PromptOverride=0 loads the active version.
	promptVersion := int(stage.PromptOverride)
	tmplRecord, err := e.promptRepo.GetPromptByStage(ctx, stage.Stage, promptVersion)
	if err != nil {
		return StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     fmt.Sprintf("load prompt (version %d): %v", promptVersion, err),
		}, fmt.Errorf("llm %s: load prompt (version %d): %w", stage.Stage, promptVersion, err)
	}

	tmpl, err := template.New(stage.Stage).Parse(tmplRecord.Content)
	if err != nil {
		return StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     fmt.Sprintf("parse prompt template: %v", err),
		}, fmt.Errorf("llm %s: parse prompt template: %w", stage.Stage, err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, templateData); err != nil {
		return StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     fmt.Sprintf("render prompt template: %v", err),
		}, fmt.Errorf("llm %s: render prompt template: %w", stage.Stage, err)
	}

	// Attach Langfuse tracing metadata to the gRPC call context when available.
	callCtx := ctx
	if input.LangfuseTraceID != "" {
		md := metadata.Pairs("x-langfuse-trace-id", input.LangfuseTraceID)
		callCtx = metadata.NewOutgoingContext(ctx, md)
	}

	// Build the AI request. JSON mode ensures structured output for downstream
	// stage consumers that unmarshal RawData into stage-specific types.
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
		out := StageOutput{
			Success:   false,
			StageName: stage.Stage,
			StageKind: stage.StageKind,
			Error:     fmt.Sprintf("LLM call: %v", err),
		}
		if stage.Optional {
			return out, nil
		}
		return out, fmt.Errorf("llm %s: LLM call: %w", stage.Stage, err)
	}

	return StageOutput{
		Success:   true,
		StageName: stage.Stage,
		StageKind: stage.StageKind,
		RawData:   json.RawMessage(resp.Summary),
	}, nil
}

// buildLLMTemplateData constructs the map passed to the prompt template.
// It always includes "Content". Each successful previous output is decoded and
// added under its stage name so templates can reference them directly:
//
//	{{.triage}}            — the triage stage output (as a map)
//	{{.extract_ner}}       — the NER extraction output
//	{{.Content}}           — the raw document content
//
// Failed or skipped previous outputs are excluded to prevent templates from
// consuming partial data.
func buildLLMTemplateData(input StageInput) map[string]interface{} {
	data := map[string]interface{}{
		"Content": input.Content,
	}
	for stageName, output := range input.PreviousOutputs {
		if !output.Success || len(output.RawData) == 0 {
			continue
		}
		var decoded interface{}
		if err := json.Unmarshal(output.RawData, &decoded); err == nil {
			data[stageName] = decoded
		}
	}
	return data
}

// Compile-time interface check.
var _ StageExecutor = (*LLMStageExecutor)(nil)
