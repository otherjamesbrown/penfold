// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// Signal and query names for SLMPipelineWorkflow.
const (
	PipelineStatusQuery    = "pipeline_status"
	PipelinePrioritySignal = "pipeline_priority"
	PipelineCancelSignal   = "pipeline_cancel"
)

// PipelineInput is the input for the SLM pipeline workflow.
type PipelineInput struct {
	TenantID    string `json:"tenant_id"`
	SourceID    int64  `json:"source_id"`
	ContentID   string `json:"content_id,omitempty"`
	JobID       string `json:"job_id"`
	ContentType string `json:"content_type"` // email, meeting, slack
	ContentHash string `json:"content_hash,omitempty"`

	// Email-specific fields
	BodyText    string `json:"body_text,omitempty"`
	BodyHTML    string `json:"body_html,omitempty"`
	Subject     string `json:"subject,omitempty"`
	SenderEmail string `json:"sender_email,omitempty"`
	SenderName  string `json:"sender_name,omitempty"`

	// Meeting-specific fields
	TranscriptContent string `json:"transcript_content,omitempty"`
	TranscriptFormat  string `json:"transcript_format,omitempty"`
}

// PipelineResult is the output from the SLM pipeline workflow.
type PipelineResult struct {
	SourceID  int64  `json:"source_id"`
	Status    string `json:"status"` // completed, failed, cancelled
	Error     string `json:"error,omitempty"`
	SkipDeep  bool   `json:"skip_deep"`
	ModelUsed string `json:"model_used,omitempty"`

	// Stage outputs
	ParsedContent     string `json:"parsed_content,omitempty"`
	Category          string `json:"category,omitempty"`
	Importance        string `json:"importance,omitempty"`
	EmbeddingID       *int64 `json:"embedding_id,omitempty"`
	AssertionsCreated int    `json:"assertions_created,omitempty"`
}

// PipelineStatus tracks the status of the pipeline workflow.
type PipelineStatus struct {
	Stage           string    `json:"stage"`
	StepsCompleted  int       `json:"steps_completed"`
	TotalSteps      int       `json:"total_steps"` // 7 for full, 3 for skip
	LastActivity    string    `json:"last_activity"`
	StartedAt       time.Time `json:"started_at"`
	LastUpdated     time.Time `json:"last_updated"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	CompensationRan bool      `json:"compensation_ran,omitempty"`
}

// Pipeline activity input/output types (JSON-compatible with activities package types).

type pipelineParseEmailInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	BodyText string `json:"body_text"`
	BodyHTML string `json:"body_html"`
}

type pipelineParseEmailOutput struct {
	CleanBody     string `json:"clean_body"`
	NewContent    string `json:"new_content"`
	QuotedContent string `json:"quoted_content"`
	IsReply       bool   `json:"is_reply"`
}

type pipelineParseTranscriptInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Content  string `json:"content"`
	Format   string `json:"format,omitempty"`
}

type pipelineParseTranscriptOutput struct {
	CleanText  string   `json:"clean_text"`
	Speakers   []string `json:"speakers"`
	DurationMs int      `json:"duration_ms"`
	Format     string   `json:"format"`
}

type pipelineTriageInput struct {
	TenantID    string `json:"tenant_id"`
	SourceID    int64  `json:"source_id"`
	ContentID   string `json:"content_id,omitempty"`
	JobID       string `json:"job_id"`
	Content     string `json:"content"`
	Subject     string `json:"subject,omitempty"`
	SenderEmail string `json:"sender_email,omitempty"`
	ContentType string `json:"content_type"`
}

type pipelineTriageOutput struct {
	Category   string  `json:"category"`
	Importance string  `json:"importance"`
	Reason     string  `json:"reason"`
	Confidence float32 `json:"confidence"`
	ModelUsed  string  `json:"model_used"`
	SkipDeep   bool    `json:"skip_deep"`
}

type pipelineExtractInput struct {
	TenantID  string `json:"tenant_id"`
	SourceID  int64  `json:"source_id"`
	ContentID string `json:"content_id,omitempty"`
	JobID     string `json:"job_id"`
	Content   string `json:"content"`
}

// pipelineExtractOutput mirrors activities.ExtractEntitiesOutput.
type pipelineExtractOutput struct {
	People               []pipelinePersonResult     `json:"people"`
	Dates                []pipelineDateResult       `json:"dates"`
	Projects             []string                   `json:"projects"`
	Organisations        []string                   `json:"organisations"`
	ActionItems          []pipelineActionItemResult `json:"action_items"`
	Decisions            []string                   `json:"decisions"`
	Risks                []string                   `json:"risks"`
	DetailedRisks        []pipelineDetailedRisk     `json:"detailed_risks,omitempty"`
	QualityGateTriggered bool                       `json:"quality_gate_triggered"`
	ModelUsed            string                     `json:"model_used"`
}

type pipelinePersonResult struct {
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type pipelineDateResult struct {
	Date        string `json:"date"`
	Context     string `json:"context,omitempty"`
	IsDeadline  bool   `json:"is_deadline,omitempty"`
}

type pipelineActionItemResult struct {
	Description string `json:"description"`
	Assignee    string `json:"assignee,omitempty"`
	Due         string `json:"due,omitempty"`
	Priority    string `json:"priority,omitempty"`
}

type pipelineDetailedRisk struct {
	Description string `json:"description"`
	Severity    string `json:"severity,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Impact      string `json:"impact,omitempty"`
}

type pipelineContextInput struct {
	TenantID    string                 `json:"tenant_id"`
	SourceID    int64                  `json:"source_id"`
	ContentID   string                 `json:"content_id,omitempty"`
	JobID       string                 `json:"job_id"`
	ContentType string                 `json:"content_type"`
	Extraction  *pipelineExtractOutput `json:"extraction"`
	SenderEmail string                 `json:"sender_email,omitempty"`
	SenderName  string                 `json:"sender_name,omitempty"`
	Subject     string                 `json:"subject,omitempty"`
	ThreadID    string                 `json:"thread_id,omitempty"`
}

type pipelineContextOutput struct {
	ResolvedPeople     []pipelineResolvedPerson  `json:"resolved_people"`
	ResolvedProjects   []pipelineResolvedProject `json:"resolved_projects"`
	UnresolvedTerms    []string                  `json:"unresolved_terms"`
	ContextPackage     interface{}               `json:"context_package"` // Opaque for workflow
	TokensUsed         int                       `json:"tokens_used"`
	TokenBudget        int                       `json:"token_budget"`
	EntitiesResolved   int                       `json:"entities_resolved"`
	EntitiesUnresolved int                       `json:"entities_unresolved"`
}

type pipelineResolvedPerson struct {
	Name       string  `json:"name"`
	PersonID   *int64  `json:"person_id,omitempty"`
	Confidence float32 `json:"confidence"`
	Source     string  `json:"source"`
}

type pipelineResolvedProject struct {
	Name      string `json:"name"`
	ProjectID *int64 `json:"project_id,omitempty"`
	Source    string  `json:"source"`
}

type pipelineAnalyzeInput struct {
	TenantID          string                 `json:"tenant_id"`
	SourceID          int64                  `json:"source_id"`
	ContentID         string                 `json:"content_id,omitempty"`
	JobID             string                 `json:"job_id"`
	Content           string                 `json:"content"`
	ContentType       string                 `json:"content_type"`
	TriageCategory    string                 `json:"triage_category"`
	TriageImportance  string                 `json:"triage_importance"`
	ExtractionResult  *pipelineExtractOutput `json:"extraction_result"`
	BackgroundContext string                 `json:"background_context,omitempty"`
}

type pipelineAnalyzeOutput struct {
	Summary           string      `json:"summary"`
	Sentiment         interface{} `json:"sentiment"`
	TopicMappings     interface{} `json:"topic_mappings"`
	VerifiedActions   interface{} `json:"verified_action_items"`
	VerifiedDecisions interface{} `json:"verified_decisions"`
	RiskReferences    interface{} `json:"risk_references"`
	Insights          []string    `json:"strategic_insights"`
	ImplicitActions   interface{} `json:"implicit_action_items"`
	ModelUsed         string      `json:"model_used"`
}

type pipelinePersistInput struct {
	TenantID       string               `json:"tenant_id"`
	SourceID       int64                `json:"source_id"`
	ThreadID       *int64               `json:"thread_id,omitempty"`
	ProjectID      *int64               `json:"project_id,omitempty"`
	Analysis       *pipelineAnalyzeOutput `json:"analysis"`
	ResolvedPeople map[string]int64     `json:"resolved_people,omitempty"`
}

type pipelinePersistOutput struct {
	AssertionsCreated    int `json:"assertions_created"`
	AssertionsSuperseded int `json:"assertions_superseded"`
	ReferencesCreated    int `json:"references_created"`
	ReviewItemsCreated   int `json:"review_items_created"`
	AffinityUpdates      int `json:"affinity_updates"`
}

// pipelineState maintains the internal state of the pipeline workflow.
type pipelineState struct {
	status          PipelineStatus
	result          *PipelineResult
	cancelRequested bool
	cancelReason    string
}

// SLMPipelineWorkflow orchestrates the SLM/LLM content processing pipeline.
//
// Stages:
//
//	0. Parse — deterministic text extraction (email HTML strip, transcript format)
//	1. Triage — SLM classification into category + importance
//	2. Extract — SLM entity extraction (NER + semantic)
//	3. Context — code-based entity resolution + context assembly
//	4. Analyze — LLM deep analysis (optional, failure doesn't block pipeline)
//	4.5 Persist — write findings to database
//	5. Embed — generate search embeddings (critical, failure = pipeline failure)
//
// Triage gates: PERSONAL (any importance) or INTERNAL_COMMS+LOW skip Stages 2-4.5.
// Progressive availability: status updated to "parsed" after Stage 0, "extracted" after Stage 2.
func SLMPipelineWorkflow(ctx workflow.Context, input PipelineInput) (*PipelineResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting SLM pipeline workflow",
		"source_id", input.SourceID,
		"tenant_id", input.TenantID,
		"content_type", input.ContentType,
		"job_id", input.JobID,
	)

	// Initialize state
	state := &pipelineState{
		status: PipelineStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     pkgtemporal.FullPipelineTotalSteps(),
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &PipelineResult{
			SourceID: input.SourceID,
		},
	}

	// Register query handler
	if err := workflow.SetQueryHandler(ctx, PipelineStatusQuery, func() (PipelineStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Signal channels
	prioritySignalChan := workflow.GetSignalChannel(ctx, PipelinePrioritySignal)
	cancelSignalChan := workflow.GetSignalChannel(ctx, PipelineCancelSignal)

	selector := workflow.NewSelector(ctx)

	selector.AddReceive(prioritySignalChan, func(c workflow.ReceiveChannel, more bool) {
		var signal pkgtemporal.PriorityUpdateSignal
		c.Receive(ctx, &signal)
		logger.Info("Received priority update", "new_priority", signal.NewPriority)
	})

	selector.AddReceive(cancelSignalChan, func(c workflow.ReceiveChannel, more bool) {
		var signal pkgtemporal.CancelWithCompensationSignal
		c.Receive(ctx, &signal)
		logger.Info("Received cancellation signal", "reason", signal.Reason)
		state.cancelRequested = true
		state.cancelReason = signal.Reason
	})

	// Activity option presets
	fastOpts := pkgtemporal.FastActivityOptions()
	embeddingOpts := pkgtemporal.EmbeddingActivityOptions()
	llmOpts := pkgtemporal.LLMActivityOptions()

	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	embeddingOpts = pkgtemporal.WithNonRetryableErrors(embeddingOpts, pkgtemporal.NonRetryableErrors()...)
	llmOpts = pkgtemporal.WithNonRetryableErrors(llmOpts, pkgtemporal.NonRetryableErrors()...)

	// Saga compensation stack
	var compensations []func(workflow.Context) error

	checkCancellation := func() bool {
		for selector.HasPending() {
			selector.Select(ctx)
		}
		return state.cancelRequested
	}

	runCompensation := func(ctx workflow.Context) {
		if len(compensations) == 0 {
			return
		}
		logger.Info("Running saga compensation", "compensation_count", len(compensations))
		state.status.CompensationRan = true
		for i := len(compensations) - 1; i >= 0; i-- {
			if err := compensations[i](ctx); err != nil {
				logger.Warn("Compensation failed", "index", i, "error", err)
			}
		}
	}

	updateStatus := func(stage, activity string) {
		state.status.Stage = stage
		state.status.LastActivity = activity
		state.status.LastUpdated = workflow.Now(ctx)
	}

	// ==================== Stage 0: Parse ====================
	updateStatus("parsing", "Parse")
	parseStage := stageByStatus("parsing")
	logger.Info("pipeline stage starting",
		"source_id", input.SourceID,
		"stage", parseStage.Name,
		"stage_number", parseStage.Number,
		"total_steps", state.status.TotalSteps,
	)
	parseStart := workflow.Now(ctx)

	// If ContentType is empty, the workflow was started with SLMPipelineInput
	// (minimal contract). Fetch content and metadata from the database.
	if input.ContentType == "" {
		var fetchOut FetchSourceOutput
		ctxFetch := workflow.WithActivityOptions(ctx, fastOpts)
		err := workflow.ExecuteActivity(ctxFetch, pkgtemporal.ActivityFetchContent, FetchSourceInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
		}).Get(ctx, &fetchOut)
		if err != nil {
			state.result.Status = "failed"
			state.result.Error = fmt.Sprintf("fetch_source: %v", err)
			state.status.ErrorMessage = state.result.Error
			logger.Error("FetchSource failed", "error", err)
			ctxFail := workflow.WithActivityOptions(ctx, fastOpts)
			_ = workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID: input.TenantID, SourceID: input.SourceID,
				Status: "failed", FailureCategory: "fetch", FailureReason: err.Error(),
			}).Get(ctx, nil)
			return state.result, nil
		}
		input.ContentType = fetchOut.ContentType
		input.BodyText = fetchOut.ContentText
		input.Subject = fetchOut.Subject
		input.SenderEmail = fetchOut.SenderEmail
		input.SenderName = fetchOut.SenderName
	}

	var parsedContent string

	switch input.ContentType {
	case "email":
		var parseOutput pipelineParseEmailOutput
		ctxParse := workflow.WithActivityOptions(ctx, fastOpts)
		err := workflow.ExecuteActivity(ctxParse, pkgtemporal.ActivityParseEmail, pipelineParseEmailInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			BodyText: input.BodyText,
			BodyHTML: input.BodyHTML,
		}).Get(ctx, &parseOutput)
		if err != nil {
			state.result.Status = "failed"
			state.result.Error = fmt.Sprintf("parse_email: %v", err)
			state.status.ErrorMessage = state.result.Error
			logger.Error("Stage 0 ParseEmail failed", "error", err)
			return state.result, nil
		}
		if parseOutput.NewContent != "" {
			parsedContent = parseOutput.NewContent
		} else {
			parsedContent = parseOutput.CleanBody
		}
		logger.Info("pipeline stage completed",
			"source_id", input.SourceID,
			"stage", parseStage.Name,
			"stage_number", parseStage.Number,
			"duration_ms", workflow.Now(ctx).Sub(parseStart).Milliseconds(),
			"status", "completed",
			"content_length", len(parsedContent),
			"is_reply", parseOutput.IsReply,
		)

	case "meeting":
		var parseOutput pipelineParseTranscriptOutput
		ctxParse := workflow.WithActivityOptions(ctx, fastOpts)
		err := workflow.ExecuteActivity(ctxParse, pkgtemporal.ActivityParseTranscript, pipelineParseTranscriptInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			Content:  input.TranscriptContent,
			Format:   input.TranscriptFormat,
		}).Get(ctx, &parseOutput)
		if err != nil {
			state.result.Status = "failed"
			state.result.Error = fmt.Sprintf("parse_transcript: %v", err)
			state.status.ErrorMessage = state.result.Error
			logger.Error("Stage 0 ParseTranscript failed", "error", err)
			return state.result, nil
		}
		parsedContent = parseOutput.CleanText
		logger.Info("pipeline stage completed",
			"source_id", input.SourceID,
			"stage", parseStage.Name,
			"stage_number", parseStage.Number,
			"duration_ms", workflow.Now(ctx).Sub(parseStart).Milliseconds(),
			"status", "completed",
			"content_length", len(parsedContent),
			"speaker_count", len(parseOutput.Speakers),
		)

	default:
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("unsupported content_type: %s", input.ContentType)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Unsupported content type", "content_type", input.ContentType)
		ctxFail := workflow.WithActivityOptions(ctx, fastOpts)
		_ = workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
			TenantID: input.TenantID, SourceID: input.SourceID,
			Status: "failed", FailureCategory: "unsupported_type",
			FailureReason: state.result.Error,
		}).Get(ctx, nil)
		return state.result, nil
	}

	state.result.ParsedContent = parsedContent
	state.status.StepsCompleted = 1

	// Progressive availability: mark as "parsed" (keyword-searchable)
	ctxStatus := workflow.WithActivityOptions(ctx, fastOpts)
	_ = workflow.ExecuteActivity(ctxStatus, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		Status:   "parsed",
	}).Get(ctx, nil)

	if checkCancellation() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		return state.result, nil
	}

	// ==================== Stage 1: Triage ====================
	updateStatus("triaging", "Triage")
	triageStage := stageByStatus("triaging")
	logger.Info("pipeline stage starting",
		"source_id", input.SourceID,
		"stage", triageStage.Name,
		"stage_number", triageStage.Number,
		"total_steps", state.status.TotalSteps,
	)
	triageStart := workflow.Now(ctx)

	var triageOutput pipelineTriageOutput
	ctxTriage := workflow.WithActivityOptions(ctx, embeddingOpts)
	err := workflow.ExecuteActivity(ctxTriage, pkgtemporal.ActivityTriage, pipelineTriageInput{
		TenantID:    input.TenantID,
		SourceID:    input.SourceID,
		ContentID:   input.ContentID,
		JobID:       input.JobID,
		Content:     parsedContent,
		Subject:     input.Subject,
		SenderEmail: input.SenderEmail,
		ContentType: input.ContentType,
	}).Get(ctx, &triageOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("triage: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Stage 1 Triage failed", "error", err)
		return state.result, nil
	}

	logger.Info("pipeline stage completed",
		"source_id", input.SourceID,
		"stage", triageStage.Name,
		"stage_number", triageStage.Number,
		"duration_ms", workflow.Now(ctx).Sub(triageStart).Milliseconds(),
		"status", "completed",
		"category", triageOutput.Category,
		"importance", triageOutput.Importance,
		"confidence", triageOutput.Confidence,
		"skip_deep", triageOutput.SkipDeep,
		"model_used", triageOutput.ModelUsed,
	)

	state.result.Category = triageOutput.Category
	state.result.Importance = triageOutput.Importance
	state.result.SkipDeep = triageOutput.SkipDeep
	state.result.ModelUsed = triageOutput.ModelUsed
	state.status.StepsCompleted = 2

	if checkCancellation() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		return state.result, nil
	}

	// Triage gate: skip Stages 2-4.5 for LOW/PERSONAL content
	if triageOutput.SkipDeep {
		// Log skip for each deep processing stage
		for _, s := range pkgtemporal.SLMPipelineStages {
			if s.SkipWhenLow {
				logger.Info("pipeline stage skipped",
					"source_id", input.SourceID,
					"stage", s.Name,
					"stage_number", s.Number,
					"reason", "skip_deep",
				)
			}
		}
		state.status.TotalSteps = pkgtemporal.SkipDeepTotalSteps()
	}

	// ==================== Stages 2-4.5: Deep Processing ====================
	var extractOutput *pipelineExtractOutput
	var contextOutput *pipelineContextOutput

	if !triageOutput.SkipDeep {
		// Stage 2: Extract
		updateStatus("extracting", "ExtractEntities")
		extractStage := stageByStatus("extracting")
		logger.Info("pipeline stage starting",
			"source_id", input.SourceID,
			"stage", extractStage.Name,
			"stage_number", extractStage.Number,
			"total_steps", state.status.TotalSteps,
		)
		extractStart := workflow.Now(ctx)

		extractOutput = &pipelineExtractOutput{}
		ctxExtract := workflow.WithActivityOptions(ctx, embeddingOpts)
		err = workflow.ExecuteActivity(ctxExtract, pkgtemporal.ActivityExtractEntitiesActivity, pipelineExtractInput{
			TenantID:  input.TenantID,
			SourceID:  input.SourceID,
			ContentID: input.ContentID,
			JobID:     input.JobID,
			Content:   parsedContent,
		}).Get(ctx, extractOutput)

		// Stage 2b: Extract Assertions (failure does NOT block pipeline)
		var assertionCount int
		ctxAssertions := workflow.WithActivityOptions(ctx, embeddingOpts)
		err2 := workflow.ExecuteActivity(ctxAssertions, pkgtemporal.ActivityExtractAssertions, ExtractAssertionsInput{
			TenantID:  input.TenantID,
			SourceID:  input.SourceID,
			ContentID: input.ContentID,
			JobID:     input.JobID,
			Content:   parsedContent,
		}).Get(ctx, &assertionCount)
		if err2 != nil {
			logger.Warn("Stage 2b ExtractAssertions failed, continuing", "error", err2)
			assertionCount = 0
		}
		state.result.AssertionsCreated = assertionCount

		if err != nil {
			logger.Warn("pipeline stage failed (non-blocking)",
				"source_id", input.SourceID,
				"stage", extractStage.Name,
				"stage_number", extractStage.Number,
				"duration_ms", workflow.Now(ctx).Sub(extractStart).Milliseconds(),
				"status", "failed",
				"error", err.Error(),
			)
			extractOutput = &pipelineExtractOutput{}
		} else {
			logger.Info("pipeline stage completed",
				"source_id", input.SourceID,
				"stage", extractStage.Name,
				"stage_number", extractStage.Number,
				"duration_ms", workflow.Now(ctx).Sub(extractStart).Milliseconds(),
				"status", "completed",
				"people_count", len(extractOutput.People),
				"action_items_count", len(extractOutput.ActionItems),
				"assertions_created", assertionCount,
				"model_used", extractOutput.ModelUsed,
			)
		}

		state.status.StepsCompleted = 3

		// Progressive availability: mark as "extracted" (entity-searchable)
		ctxStatus2 := workflow.WithActivityOptions(ctx, fastOpts)
		_ = workflow.ExecuteActivity(ctxStatus2, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			Status:   "extracted",
		}).Get(ctx, nil)

		if checkCancellation() {
			state.result.Status = "cancelled"
			state.result.Error = state.cancelReason
			return state.result, nil
		}

		// Stage 3: Context
		updateStatus("building_context", "BuildContextPackage")
		contextStage := stageByStatus("building_context")
		logger.Info("pipeline stage starting",
			"source_id", input.SourceID,
			"stage", contextStage.Name,
			"stage_number", contextStage.Number,
			"total_steps", state.status.TotalSteps,
		)
		contextStart := workflow.Now(ctx)

		contextOutput = &pipelineContextOutput{}
		ctxContext := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctxContext, pkgtemporal.ActivityBuildContextPackage, pipelineContextInput{
			TenantID:    input.TenantID,
			SourceID:    input.SourceID,
			ContentID:   input.ContentID,
			JobID:       input.JobID,
			ContentType: input.ContentType,
			Extraction:  extractOutput,
			SenderEmail: input.SenderEmail,
			SenderName:  input.SenderName,
			Subject:     input.Subject,
		}).Get(ctx, contextOutput)
		if err != nil {
			logger.Warn("pipeline stage failed (non-blocking)",
				"source_id", input.SourceID,
				"stage", contextStage.Name,
				"stage_number", contextStage.Number,
				"duration_ms", workflow.Now(ctx).Sub(contextStart).Milliseconds(),
				"status", "failed",
				"error", err.Error(),
			)
			contextOutput = &pipelineContextOutput{}
		} else {
			logger.Info("pipeline stage completed",
				"source_id", input.SourceID,
				"stage", contextStage.Name,
				"stage_number", contextStage.Number,
				"duration_ms", workflow.Now(ctx).Sub(contextStart).Milliseconds(),
				"status", "completed",
				"entities_resolved", contextOutput.EntitiesResolved,
				"entities_unresolved", contextOutput.EntitiesUnresolved,
				"tokens_used", contextOutput.TokensUsed,
			)
		}
		state.status.StepsCompleted = 4

		if checkCancellation() {
			state.result.Status = "cancelled"
			state.result.Error = state.cancelReason
			return state.result, nil
		}

		// Stage 4: Deep Analysis (optional — failure does NOT block pipeline)
		updateStatus("analyzing", "DeepAnalyze")
		analyzeStage := stageByStatus("analyzing")
		logger.Info("pipeline stage starting",
			"source_id", input.SourceID,
			"stage", analyzeStage.Name,
			"stage_number", analyzeStage.Number,
			"total_steps", state.status.TotalSteps,
		)
		analyzeStart := workflow.Now(ctx)

		var analyzeOutput *pipelineAnalyzeOutput
		ctxAnalyze := workflow.WithActivityOptions(ctx, llmOpts)
		analyzeOutput = &pipelineAnalyzeOutput{}
		err = workflow.ExecuteActivity(ctxAnalyze, pkgtemporal.ActivityDeepAnalyze, pipelineAnalyzeInput{
			TenantID:          input.TenantID,
			SourceID:          input.SourceID,
			ContentID:         input.ContentID,
			JobID:             input.JobID,
			Content:           parsedContent,
			ContentType:       input.ContentType,
			TriageCategory:    triageOutput.Category,
			TriageImportance:  triageOutput.Importance,
			ExtractionResult:  extractOutput,
			BackgroundContext: "", // Context package content assembled by activity
		}).Get(ctx, analyzeOutput)
		if err != nil {
			logger.Warn("pipeline stage failed (non-blocking)",
				"source_id", input.SourceID,
				"stage", analyzeStage.Name,
				"stage_number", analyzeStage.Number,
				"duration_ms", workflow.Now(ctx).Sub(analyzeStart).Milliseconds(),
				"status", "failed",
				"error", err.Error(),
			)
			analyzeOutput = nil // Skip persist if analysis failed
		} else {
			logger.Info("pipeline stage completed",
				"source_id", input.SourceID,
				"stage", analyzeStage.Name,
				"stage_number", analyzeStage.Number,
				"duration_ms", workflow.Now(ctx).Sub(analyzeStart).Milliseconds(),
				"status", "completed",
				"model_used", analyzeOutput.ModelUsed,
				"has_summary", analyzeOutput.Summary != "",
			)
		}
		state.status.StepsCompleted = 5

		if checkCancellation() {
			state.result.Status = "cancelled"
			state.result.Error = state.cancelReason
			return state.result, nil
		}

		// Stage 4.5: Persist Findings (only if Stage 4 succeeded)
		if analyzeOutput != nil {
			updateStatus("persisting", "PersistFindings")
			persistStage := stageByStatus("persisting")
			logger.Info("pipeline stage starting",
				"source_id", input.SourceID,
				"stage", persistStage.Name,
				"stage_number", persistStage.Number,
				"total_steps", state.status.TotalSteps,
			)
			persistStart := workflow.Now(ctx)

			// Build resolved people map from context output
			resolvedPeople := make(map[string]int64)
			if contextOutput != nil {
				for _, p := range contextOutput.ResolvedPeople {
					if p.PersonID != nil {
						resolvedPeople[p.Name] = *p.PersonID
					}
				}
			}

			// Find first resolved project ID
			var projectID *int64
			if contextOutput != nil {
				for _, proj := range contextOutput.ResolvedProjects {
					if proj.ProjectID != nil {
						projectID = proj.ProjectID
						break
					}
				}
			}

			var persistOutput pipelinePersistOutput
			ctxPersist := workflow.WithActivityOptions(ctx, fastOpts)
			err = workflow.ExecuteActivity(ctxPersist, pkgtemporal.ActivityPersistFindings, pipelinePersistInput{
				TenantID:       input.TenantID,
				SourceID:       input.SourceID,
				ProjectID:      projectID,
				Analysis:       analyzeOutput,
				ResolvedPeople: resolvedPeople,
			}).Get(ctx, &persistOutput)
			if err != nil {
				logger.Warn("pipeline stage failed (non-blocking)",
					"source_id", input.SourceID,
					"stage", persistStage.Name,
					"stage_number", persistStage.Number,
					"duration_ms", workflow.Now(ctx).Sub(persistStart).Milliseconds(),
					"status", "failed",
					"error", err.Error(),
				)
			} else {
				logger.Info("pipeline stage completed",
					"source_id", input.SourceID,
					"stage", persistStage.Name,
					"stage_number", persistStage.Number,
					"duration_ms", workflow.Now(ctx).Sub(persistStart).Milliseconds(),
					"status", "completed",
					"assertions_created", persistOutput.AssertionsCreated,
					"assertions_superseded", persistOutput.AssertionsSuperseded,
					"references_created", persistOutput.ReferencesCreated,
				)
				state.result.AssertionsCreated += persistOutput.AssertionsCreated
			}
		}
		state.status.StepsCompleted = 6
	}

	if checkCancellation() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		return state.result, nil
	}

	// ==================== Stage 5: Embed ====================
	// Embedding is critical — failure means pipeline failure.
	updateStatus("embedding", "GenerateEmbedding")
	embedStage := stageByStatus("embedding")
	logger.Info("pipeline stage starting",
		"source_id", input.SourceID,
		"stage", embedStage.Name,
		"stage_number", embedStage.Number,
		"total_steps", state.status.TotalSteps,
	)
	embedStart := workflow.Now(ctx)

	var embeddingID int64
	ctxEmbed := workflow.WithActivityOptions(ctx, embeddingOpts)
	err = workflow.ExecuteActivity(ctxEmbed, pkgtemporal.ActivityGenerateContentEmbedding, GenerateEmbeddingInput{
		TenantID:    input.TenantID,
		SourceID:    input.SourceID,
		ContentID:   input.ContentID,
		Content:     parsedContent,
		ContentHash: input.ContentHash,
	}).Get(ctx, &embeddingID)
	if err != nil {
		runCompensation(ctx)
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("embedding_failed: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Stage 5 Embedding failed — pipeline failed", "error", err)

		// Update status to failed
		ctxFail := workflow.WithActivityOptions(ctx, fastOpts)
		_ = workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			Status:   "failed",
		}).Get(ctx, nil)

		return state.result, nil
	}

	logger.Info("pipeline stage completed",
		"source_id", input.SourceID,
		"stage", embedStage.Name,
		"stage_number", embedStage.Number,
		"duration_ms", workflow.Now(ctx).Sub(embedStart).Milliseconds(),
		"status", "completed",
		"embedding_id", embeddingID,
	)

	state.result.EmbeddingID = &embeddingID

	// Add compensation to delete embedding on downstream failure
	compensations = append(compensations, func(ctx workflow.Context) error {
		return workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityDeleteEmbedding,
			embeddingID,
		).Get(ctx, nil)
	})

	if triageOutput.SkipDeep {
		state.status.StepsCompleted = pkgtemporal.SkipDeepTotalSteps()
	} else {
		state.status.StepsCompleted = pkgtemporal.FullPipelineTotalSteps()
	}

	// Final status update
	ctxComplete := workflow.WithActivityOptions(ctx, fastOpts)
	_ = workflow.ExecuteActivity(ctxComplete, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		Status:   "completed",
	}).Get(ctx, nil)

	state.result.Status = "completed"
	updateStatus("completed", "")

	logger.Info("SLM pipeline completed",
		"source_id", input.SourceID,
		"category", triageOutput.Category,
		"importance", triageOutput.Importance,
		"skip_deep", triageOutput.SkipDeep,
		"embedding_id", embeddingID,
	)

	return state.result, nil
}

// stageByStatus returns the Stage metadata for a given StatusName.
// If not found, returns a minimal Stage with the status name.
func stageByStatus(statusName string) pkgtemporal.Stage {
	for _, s := range pkgtemporal.SLMPipelineStages {
		if s.StatusName == statusName {
			return s
		}
	}
	return pkgtemporal.Stage{Name: statusName, StatusName: statusName}
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
