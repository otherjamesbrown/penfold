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
	ParsedContent string `json:"parsed_content,omitempty"`
	Category      string `json:"category,omitempty"`
	Importance    string `json:"importance,omitempty"`
	EmbeddingID   *int64 `json:"embedding_id,omitempty"`
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
			TotalSteps:     7, // Full pipeline: parse, triage, extract, context, analyze, persist, embed
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
	var parsedContent string

	switch input.ContentType {
	case "email":
		var parseOutput pipelineParseEmailOutput
		ctxParse := workflow.WithActivityOptions(ctx, fastOpts)
		err := workflow.ExecuteActivity(ctxParse, "ParseEmail", pipelineParseEmailInput{
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

	case "meeting":
		var parseOutput pipelineParseTranscriptOutput
		ctxParse := workflow.WithActivityOptions(ctx, fastOpts)
		err := workflow.ExecuteActivity(ctxParse, "ParseTranscript", pipelineParseTranscriptInput{
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

	default:
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("unsupported content_type: %s", input.ContentType)
		state.status.ErrorMessage = state.result.Error
		return state.result, nil
	}

	state.result.ParsedContent = parsedContent
	state.status.StepsCompleted = 1

	// Progressive availability: mark as "parsed" (keyword-searchable)
	ctxStatus := workflow.WithActivityOptions(ctx, fastOpts)
	_ = workflow.ExecuteActivity(ctxStatus, "UpdateContentStatus", UpdateContentStatusInput{
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
	var triageOutput pipelineTriageOutput
	ctxTriage := workflow.WithActivityOptions(ctx, embeddingOpts)
	err := workflow.ExecuteActivity(ctxTriage, "Triage", pipelineTriageInput{
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
		logger.Info("Triage gate: skipping deep processing",
			"category", triageOutput.Category,
			"importance", triageOutput.Importance,
		)
		state.status.TotalSteps = 3 // parse, triage, embed
	}

	// ==================== Stages 2-4.5: Deep Processing ====================
	var extractOutput *pipelineExtractOutput
	var contextOutput *pipelineContextOutput

	if !triageOutput.SkipDeep {
		// Stage 2: Extract
		updateStatus("extracting", "ExtractEntities")
		extractOutput = &pipelineExtractOutput{}
		ctxExtract := workflow.WithActivityOptions(ctx, embeddingOpts)
		err = workflow.ExecuteActivity(ctxExtract, "ExtractEntitiesActivity", pipelineExtractInput{
			TenantID:  input.TenantID,
			SourceID:  input.SourceID,
			ContentID: input.ContentID,
			JobID:     input.JobID,
			Content:   parsedContent,
		}).Get(ctx, extractOutput)
		if err != nil {
			logger.Warn("Stage 2 Extract failed, continuing with empty extraction", "error", err)
			extractOutput = &pipelineExtractOutput{}
		}
		state.status.StepsCompleted = 3

		// Progressive availability: mark as "extracted" (entity-searchable)
		ctxStatus2 := workflow.WithActivityOptions(ctx, fastOpts)
		_ = workflow.ExecuteActivity(ctxStatus2, "UpdateContentStatus", UpdateContentStatusInput{
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
		contextOutput = &pipelineContextOutput{}
		ctxContext := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctxContext, "BuildContextPackage", pipelineContextInput{
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
			logger.Warn("Stage 3 Context failed, continuing without context", "error", err)
			contextOutput = &pipelineContextOutput{}
		}
		state.status.StepsCompleted = 4

		if checkCancellation() {
			state.result.Status = "cancelled"
			state.result.Error = state.cancelReason
			return state.result, nil
		}

		// Stage 4: Deep Analysis (optional — failure does NOT block pipeline)
		updateStatus("analyzing", "DeepAnalyze")
		var analyzeOutput *pipelineAnalyzeOutput
		ctxAnalyze := workflow.WithActivityOptions(ctx, llmOpts)
		analyzeOutput = &pipelineAnalyzeOutput{}
		err = workflow.ExecuteActivity(ctxAnalyze, "DeepAnalyze", pipelineAnalyzeInput{
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
			logger.Warn("Stage 4 DeepAnalyze failed, continuing to embedding", "error", err)
			analyzeOutput = nil // Skip persist if analysis failed
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
			err = workflow.ExecuteActivity(ctxPersist, "PersistFindings", pipelinePersistInput{
				TenantID:       input.TenantID,
				SourceID:       input.SourceID,
				ProjectID:      projectID,
				Analysis:       analyzeOutput,
				ResolvedPeople: resolvedPeople,
			}).Get(ctx, &persistOutput)
			if err != nil {
				logger.Warn("Stage 4.5 PersistFindings failed", "error", err)
			} else {
				logger.Info("Findings persisted",
					"assertions_created", persistOutput.AssertionsCreated,
					"references_created", persistOutput.ReferencesCreated,
				)
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
	var embeddingID int64
	ctxEmbed := workflow.WithActivityOptions(ctx, embeddingOpts)
	err = workflow.ExecuteActivity(ctxEmbed, "GenerateContentEmbedding", GenerateEmbeddingInput{
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
		_ = workflow.ExecuteActivity(ctxFail, "UpdateContentStatus", UpdateContentStatusInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			Status:   "failed",
		}).Get(ctx, nil)

		return state.result, nil
	}

	state.result.EmbeddingID = &embeddingID

	// Add compensation to delete embedding on downstream failure
	compensations = append(compensations, func(ctx workflow.Context) error {
		return workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			"DeleteEmbedding",
			embeddingID,
		).Get(ctx, nil)
	})

	if triageOutput.SkipDeep {
		state.status.StepsCompleted = 3
	} else {
		state.status.StepsCompleted = 7
	}

	// Final status update
	ctxComplete := workflow.WithActivityOptions(ctx, fastOpts)
	_ = workflow.ExecuteActivity(ctxComplete, "UpdateContentStatus", UpdateContentStatusInput{
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

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
