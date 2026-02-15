// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
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
	BodyText          string        `json:"body_text,omitempty"`
	BodyHTML          string        `json:"body_html,omitempty"`
	Subject           string        `json:"subject,omitempty"`
	SenderEmail       string        `json:"sender_email,omitempty"`
	SenderName        string        `json:"sender_name,omitempty"`
	ParticipantEmails []Participant `json:"participant_emails,omitempty"`

	// Meeting-specific fields
	TranscriptContent string `json:"transcript_content,omitempty"`
	TranscriptFormat  string `json:"transcript_format,omitempty"`

	// Reprocessing overrides
	ModelOverride   string        `json:"model_override,omitempty"`   // If set, use this model instead of default
	TimeoutOverride time.Duration `json:"timeout_override,omitempty"` // If set, use this timeout for activities
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

// Pipeline activity input/output types (canonical types shared with activities package).

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

// ParseTranscriptInput is the input for the ParseTranscript activity.
type ParseTranscriptInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Content  string `json:"content"`
	Format   string `json:"format,omitempty"`
}

// ParseTranscriptOutput is the output from the ParseTranscript activity.
type ParseTranscriptOutput struct {
	CleanText  string   `json:"clean_text"`
	Speakers   []string `json:"speakers"`
	DurationMs int      `json:"duration_ms"`
	Format     string   `json:"format"`
}

// TriageInput is the input for the Triage activity.
type TriageInput struct {
	TenantID      string            `json:"tenant_id"`
	SourceID      int64             `json:"source_id"`
	ContentID     string            `json:"content_id,omitempty"`
	JobID         string            `json:"job_id"`
	Content       string            `json:"content"`
	Subject       string            `json:"subject,omitempty"`
	SenderEmail   string            `json:"sender_email,omitempty"`
	ContentType   string            `json:"content_type"`
	Headers       map[string]string `json:"headers,omitempty"`       // Email headers for subtype classification
	ModelOverride string            `json:"model_override,omitempty"` // Optional model override for reprocessing
}

// TriageOutput is the output from the Triage activity.
type TriageOutput struct {
	Category       string  `json:"category"`
	Importance     string  `json:"importance"`
	Reason         string  `json:"reason"`
	Confidence     float32 `json:"confidence"`
	ModelUsed      string  `json:"model_used"`
	SkipDeep       bool    `json:"skip_deep"`
	ContentSubtype string  `json:"content_subtype,omitempty"`
}

// SLMPipelineExtractEntitiesInput is the input for the ExtractEntities activity (pipeline version with TriageCategory).
type SLMPipelineExtractEntitiesInput struct {
	TenantID       string `json:"tenant_id"`
	SourceID       int64  `json:"source_id"`
	ContentID      string `json:"content_id,omitempty"`
	JobID          string `json:"job_id"`
	Content        string `json:"content"`
	TriageCategory string `json:"triage_category,omitempty"`
	ModelOverride  string `json:"model_override,omitempty"` // Optional model override for reprocessing
}

// SLMPipelineExtractEntitiesOutput is the output from the ExtractEntities activity (pipeline version with DetailedRisks).
type SLMPipelineExtractEntitiesOutput struct {
	People               []PersonResult     `json:"people"`
	Dates                []DateResult       `json:"dates"`
	Projects             []string           `json:"projects"`
	Organisations        []string           `json:"organisations"`
	ActionItems          []ActionItemResult `json:"action_items"`
	Decisions            []string           `json:"decisions"`
	Risks                []string           `json:"risks"`
	DetailedRisks        []DetailedRisk     `json:"detailed_risks,omitempty"`
	QualityGateTriggered bool               `json:"quality_gate_triggered"`
	ModelUsed            string             `json:"model_used"`
}

// PersonResult represents a person extracted from content.
type PersonResult struct {
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

// DateResult represents a date or deadline extracted from content.
type DateResult struct {
	Date       string `json:"date"`
	Context    string `json:"context,omitempty"`
	IsDeadline bool   `json:"is_deadline,omitempty"`
}

// ActionItemResult represents an action item extracted from content.
type ActionItemResult struct {
	Assignee string `json:"assignee,omitempty"`
	Action   string `json:"action"`
	Due      string `json:"due,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// DetailedRisk represents a detailed risk from the quality gate re-run.
type DetailedRisk struct {
	Description  string `json:"description"`
	SeverityHint string `json:"severity_hint,omitempty"`
	OwnerHint    string `json:"owner_hint,omitempty"`
	Impact       string `json:"impact,omitempty"`
}

// GroupEmailThreadInput is the input for the GroupEmailThread activity.
type GroupEmailThreadInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
}

// GroupEmailThreadOutput is the output from the GroupEmailThread activity.
type GroupEmailThreadOutput struct {
	ThreadID *string `json:"thread_id,omitempty"` // Root message ID (nil if not threaded)
}

// BuildContextInput is the input for the BuildContextPackage activity.
type BuildContextInput struct {
	TenantID          string                            `json:"tenant_id"`
	SourceID          int64                             `json:"source_id"`
	ContentID         string                            `json:"content_id,omitempty"`
	JobID             string                            `json:"job_id"`
	ContentType       string                            `json:"content_type"`
	Extraction        *SLMPipelineExtractEntitiesOutput `json:"extraction"`
	SenderEmail       string                            `json:"sender_email,omitempty"`
	SenderName        string                            `json:"sender_name,omitempty"`
	Subject           string                            `json:"subject,omitempty"`
	ThreadID          string                            `json:"thread_id,omitempty"`
	ParticipantEmails []Participant                     `json:"participant_emails,omitempty"`
}

// BuildContextOutput is the output from the BuildContextPackage activity.
type BuildContextOutput struct {
	ResolvedPeople     []ResolvedPerson `json:"resolved_people"`
	ResolvedProjects   []ResolvedProject `json:"resolved_projects"`
	UnresolvedTerms    []string         `json:"unresolved_terms"`
	ContextPackage     *ContextPackage  `json:"context_package"`
	TokensUsed         int              `json:"tokens_used"`
	TokenBudget        int              `json:"token_budget"`
	EntitiesResolved   int              `json:"entities_resolved"`
	EntitiesUnresolved int              `json:"entities_unresolved"`
}

// ResolvedPerson represents a person resolved from extraction.
type ResolvedPerson struct {
	Name       string  `json:"name"`
	PersonID   *int64  `json:"person_id,omitempty"`
	Confidence float32 `json:"confidence"`
	Source     string  `json:"source"`
	Role       string  `json:"role,omitempty"`
	Title      string  `json:"title,omitempty"`
	Department string  `json:"department,omitempty"`
	IsInternal bool    `json:"is_internal"`
}

// ResolvedProject represents a project resolved from extraction.
type ResolvedProject struct {
	Name      string `json:"name"`
	ProjectID *int64 `json:"project_id,omitempty"`
	Expansion string `json:"expansion,omitempty"`
	Source    string `json:"source"`
}

// ContextPackage is the assembled context for Stage 4.
type ContextPackage struct {
	ActiveRisks        []ContextAssertion    `json:"active_risks,omitempty"`
	OpenActions        []ContextAssertion    `json:"open_actions,omitempty"`
	RecentDecisions    []ContextAssertion    `json:"recent_decisions,omitempty"`
	ProductEvents      []ContextProductEvent `json:"product_events,omitempty"`
	GlossaryTerms      []ContextGlossaryTerm `json:"glossary_terms,omitempty"`
	ParticipantContext []ResolvedPerson      `json:"participant_context,omitempty"`
	TotalTokensUsed    int                   `json:"total_tokens_used"`
	TokenBudget        int                   `json:"token_budget"`
}

// ContextAssertion represents an assertion in the context package.
type ContextAssertion struct {
	ID         int64   `json:"id"`
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate"`
	Object     string  `json:"object"`
	Confidence float32 `json:"confidence"`
	SourceText string  `json:"source_text,omitempty"`
}

// ContextProductEvent represents a product event in the context package.
type ContextProductEvent struct {
	EventType   string `json:"event_type"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
}

// ContextGlossaryTerm represents a glossary term in the context package.
type ContextGlossaryTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
	Category   string `json:"category,omitempty"`
}

// DeepAnalyzeInput is the input for the DeepAnalyze activity.
type DeepAnalyzeInput struct {
	TenantID          string                            `json:"tenant_id"`
	SourceID          int64                             `json:"source_id"`
	ContentID         string                            `json:"content_id,omitempty"`
	JobID             string                            `json:"job_id"`
	Content           string                            `json:"content"`
	ContentType       string                            `json:"content_type"`
	TriageCategory    string                            `json:"triage_category"`
	TriageImportance  string                            `json:"triage_importance"`
	ExtractionResult  *SLMPipelineExtractEntitiesOutput `json:"extraction_result"`
	BackgroundContext string                            `json:"background_context,omitempty"`
	ModelOverride     string                            `json:"model_override,omitempty"` // Optional model override for reprocessing
}

// DeepAnalyzeOutput is the output from the DeepAnalyze activity.
type DeepAnalyzeOutput struct {
	Summary           string                   `json:"summary"`
	Sentiment         *SentimentOutput         `json:"sentiment"`
	TopicMappings     []TopicMappingOutput     `json:"topic_mappings"`
	VerifiedActions   []VerifiedActionOutput   `json:"verified_action_items"`
	VerifiedDecisions []VerifiedDecisionOutput `json:"verified_decisions"`
	RiskReferences    []RiskReferenceOutput    `json:"risk_references"`
	Insights          []string                 `json:"strategic_insights"`
	ImplicitActions   []ImplicitActionOutput   `json:"implicit_action_items"`
	ModelUsed         string                   `json:"model_used"`
}

// SentimentOutput represents business-context-aware sentiment analysis.
type SentimentOutput struct {
	Score       float32  `json:"score"`
	Label       string   `json:"label"`
	Confidence  float32  `json:"confidence"`
	Indicators  []string `json:"indicators"`
	Explanation string   `json:"explanation"`
}

// TopicMappingOutput connects content to known projects/products.
type TopicMappingOutput struct {
	Topic          string  `json:"topic"`
	RelatedProject string  `json:"related_project"`
	Relationship   string  `json:"relationship"`
	Confidence     float32 `json:"confidence"`
}

// VerifiedActionOutput represents an action item verified/refined by LLM.
type VerifiedActionOutput struct {
	Description    string `json:"description"`
	Assignee       string `json:"assignee"`
	Due            string `json:"due"`
	Priority       string `json:"priority"`
	ContextExcerpt string `json:"context_excerpt"`
	Status         string `json:"status"`
}

// VerifiedDecisionOutput represents a decision verified/refined by LLM.
type VerifiedDecisionOutput struct {
	Description    string `json:"description"`
	ContextExcerpt string `json:"context_excerpt"`
	Status         string `json:"status"`
}

// RiskReferenceOutput connects content to existing or new risks.
type RiskReferenceOutput struct {
	RootID          *int64  `json:"root_id,omitempty"`
	Description     string  `json:"description"`
	LifecycleChange *string `json:"lifecycle_change,omitempty"`
	Significance    string  `json:"significance"`
	ContextExcerpt  string  `json:"context_excerpt"`
	SeverityChange  *string `json:"severity_change,omitempty"`
	OwnerChange     *string `json:"owner_change,omitempty"`
	IsNew           bool    `json:"is_new"`
}

// ImplicitActionOutput represents an inferred action not explicitly stated.
type ImplicitActionOutput struct {
	Description    string `json:"description"`
	Reasoning      string `json:"reasoning"`
	ContextExcerpt string `json:"context_excerpt"`
}

// PersistFindingsInput is the input for the PersistFindings activity.
type PersistFindingsInput struct {
	TenantID       string             `json:"tenant_id"`
	SourceID       int64              `json:"source_id"`
	ThreadID       *int64             `json:"thread_id,omitempty"`
	ProjectID      *int64             `json:"project_id,omitempty"`
	Analysis       *DeepAnalyzeOutput `json:"analysis"`
	ResolvedPeople map[string]int64   `json:"resolved_people,omitempty"`
	BodyText       string             `json:"body_text,omitempty"`
	Subject        string             `json:"subject,omitempty"`
}

// PersistFindingsOutput is the output from the PersistFindings activity.
type PersistFindingsOutput struct {
	AssertionsCreated    int `json:"assertions_created"`
	AssertionsSuperseded int `json:"assertions_superseded"`
	ReferencesCreated    int `json:"references_created"`
	ReviewItemsCreated   int `json:"review_items_created"`
	AffinityUpdates      int `json:"affinity_updates"`
}

// EnrichPersonMetadataInput is the input for the EnrichPersonMetadata activity (Stage 3.5).
type EnrichPersonMetadataInput struct {
	TenantID             string            `json:"tenant_id"`
	ResolvedPeople       []ResolvedPerson  `json:"resolved_people"`
	SignatureText        string            `json:"signature_text,omitempty"`
	SenderEmail          string            `json:"sender_email,omitempty"`          // Email of the message sender, for scoping shared signature
	BodyText             string            `json:"body_text,omitempty"`
	PerSenderSignatures  map[string]string `json:"per_sender_signatures,omitempty"` // Maps person name to their signature
}

// EnrichPersonMetadataOutput is the output from the EnrichPersonMetadata activity.
type EnrichPersonMetadataOutput struct {
	PeopleEnriched int `json:"people_enriched"`
}

// RecordOverridesInput is the input for recording override parameters in pipeline_runs.
type RecordOverridesInput struct {
	TenantID  string            `json:"tenant_id"`
	SourceID  int64             `json:"source_id"`
	Overrides map[string]string `json:"overrides"`
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

	// Cleanup handler: if workflow terminates abnormally (timeout/cancellation),
	// update status to 'failed' so content doesn't stay stuck in 'pending'.
	// Uses disconnected context so cleanup runs even after workflow cancellation.
	// Fix pf-904069: Set failure_category and failure_reason when marking as failed.
	defer func() {
		// Only run cleanup if workflow didn't complete successfully AND didn't already set a terminal status
		// (successful completion is the ONLY case where we're certain the final status was written)
		if state.result.Status != "completed" && state.result.Status != "rejected" && state.result.Status != "cancelled" {
			logger.Info("Workflow did not complete successfully, running cleanup",
				"source_id", input.SourceID,
				"final_status", state.result.Status,
				"error", state.result.Error,
			)
			newCtx, _ := workflow.NewDisconnectedContext(ctx)
			cleanupCtx := workflow.WithActivityOptions(newCtx, workflow.ActivityOptions{
				StartToCloseTimeout: 30 * time.Second,
			})

			// Set failure fields based on workflow state
			failureCategory := "processing_error"
			failureReason := "Workflow terminated abnormally"
			if state.result.Error != "" {
				failureReason = state.result.Error
			}

			_ = workflow.ExecuteActivity(cleanupCtx, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID:        input.TenantID,
				SourceID:        input.SourceID,
				Status:          "failed",
				FailureCategory: failureCategory,
				FailureReason:   failureReason,
			}).Get(cleanupCtx, nil)
		}
	}()

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
			pe := perrors.ClassifyError(err, "fetch")
			state.result.Status = "failed"
			state.result.Error = fmt.Sprintf("fetch_source: %v", err)
			state.status.ErrorMessage = state.result.Error
			logger.Error("FetchSource failed", "error", err, "error_code", pe.Code)
			ctxFail := workflow.WithActivityOptions(ctx, fastOpts)
			_ = workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID: input.TenantID, SourceID: input.SourceID,
				Status: "failed", FailureCategory: string(pe.Code), FailureReason: err.Error(),
			}).Get(ctx, nil)
			return state.result, nil
		}
		input.ContentType = fetchOut.ContentType
		input.BodyText = fetchOut.ContentText
		input.Subject = fetchOut.Subject
		input.SenderEmail = fetchOut.SenderEmail
		input.SenderName = fetchOut.SenderName
		input.ParticipantEmails = fetchOut.ParticipantEmails
		// For meeting content, also populate TranscriptContent so ParseTranscript has data (pf-0065d5)
		if input.ContentType == "meeting" {
			input.TranscriptContent = fetchOut.ContentText
		}
		// For email content, also populate BodyHTML from FetchSource for HTML-only emails (pf-dfbc24)
		if input.ContentType == "email" {
			input.BodyHTML = fetchOut.BodyHTML
		}
	}

	var parsedContent string

	switch input.ContentType {
	case "email":
		var parseOutput ParseEmailOutput
		ctxParse := workflow.WithActivityOptions(ctx, fastOpts)
		err := workflow.ExecuteActivity(ctxParse, pkgtemporal.ActivityParseEmail, ParseEmailInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			BodyText: input.BodyText,
			BodyHTML: input.BodyHTML,
		}).Get(ctx, &parseOutput)
		if err != nil {
			pe := perrors.ClassifyError(err, "parse")
			state.result.Status = "failed"
			state.result.Error = fmt.Sprintf("parse_email: %v", err)
			state.status.ErrorMessage = state.result.Error
			logger.Error("Stage 0 ParseEmail failed", "error", err, "error_code", pe.Code)
			ctxFail := workflow.WithActivityOptions(ctx, fastOpts)
			_ = workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID: input.TenantID, SourceID: input.SourceID,
				Status: "failed", FailureCategory: string(pe.Code), FailureReason: err.Error(),
			}).Get(ctx, nil)
			return state.result, nil
		}
		if parseOutput.NewContent != "" {
			parsedContent = parseOutput.NewContent
		} else {
			parsedContent = parseOutput.CleanBody
		}
		input.BodyText = parsedContent
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
		var parseOutput ParseTranscriptOutput
		ctxParse := workflow.WithActivityOptions(ctx, fastOpts)
		err := workflow.ExecuteActivity(ctxParse, pkgtemporal.ActivityParseTranscript, ParseTranscriptInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			Content:  input.TranscriptContent,
			Format:   input.TranscriptFormat,
		}).Get(ctx, &parseOutput)
		if err != nil {
			pe := perrors.ClassifyError(err, "parse")
			state.result.Status = "failed"
			state.result.Error = fmt.Sprintf("parse_transcript: %v", err)
			state.status.ErrorMessage = state.result.Error
			logger.Error("Stage 0 ParseTranscript failed", "error", err, "error_code", pe.Code)
			ctxFail := workflow.WithActivityOptions(ctx, fastOpts)
			_ = workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID: input.TenantID, SourceID: input.SourceID,
				Status: "failed", FailureCategory: string(pe.Code), FailureReason: err.Error(),
			}).Get(ctx, nil)
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
			Status: "failed", FailureCategory: "processing_error",
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

	var triageOutput TriageOutput
	triageOpts := embeddingOpts
	if input.TimeoutOverride > 0 {
		triageOpts.StartToCloseTimeout = input.TimeoutOverride
	}
	ctxTriage := workflow.WithActivityOptions(ctx, triageOpts)
	err := workflow.ExecuteActivity(ctxTriage, pkgtemporal.ActivityTriage, TriageInput{
		TenantID:      input.TenantID,
		SourceID:      input.SourceID,
		ContentID:     input.ContentID,
		JobID:         input.JobID,
		Content:       parsedContent,
		Subject:       input.Subject,
		SenderEmail:   input.SenderEmail,
		ContentType:   input.ContentType,
		ModelOverride: input.ModelOverride,
	}).Get(ctx, &triageOutput)
	if err != nil {
		// Update status to "rejected" with failure info
		pe := perrors.ClassifyError(err, "triage")
		logger.Info("Triage failed, marking as rejected",
			"error", err,
			"error_code", pe.Code,
		)
		ctxTriageUpdate := workflow.WithActivityOptions(ctx, fastOpts)
		_ = workflow.ExecuteActivity(ctxTriageUpdate, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
			TenantID:        input.TenantID,
			SourceID:        input.SourceID,
			Status:          "rejected",
			FailureCategory: string(pe.Code),
			FailureReason:   err.Error(),
		}).Get(ctx, nil)
		state.result.Status = "rejected"
		state.result.Error = fmt.Sprintf("%s: %s", pe.Code, err.Error())
		state.status.ErrorMessage = state.result.Error
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

	// Classify source_system (deterministic, runs for all items)
	// Note: Currently only using from_address and subject. message_id and headers
	// would require expanding FetchSourceOutput to include them from source metadata.
	sourceSystem := classification.ClassifySourceSystem(
		input.SenderEmail, // from_address
		input.Subject,     // subject
		"",                // message_id (not currently available in pipeline)
		nil,               // headers (not currently available in pipeline)
	)

	// Persist triage results and source_system to source metadata (fires for all items)
	skipDeep := triageOutput.SkipDeep
	ctxTriageMeta := workflow.WithActivityOptions(ctx, fastOpts)
	_ = workflow.ExecuteActivity(ctxTriageMeta, "UpdateContentStatus", UpdateContentStatusInput{
		TenantID:         input.TenantID,
		SourceID:         input.SourceID,
		Status:           "parsed",
		TriageCategory:   triageOutput.Category,
		TriageImportance: triageOutput.Importance,
		SkipDeep:         &skipDeep,
		ContentSubtype:   triageOutput.ContentSubtype,
		SourceSystem:     string(sourceSystem),
	}).Get(ctx, nil)

	// ==================== Stages 2-4.5: Deep Processing ====================
	var extractOutput *SLMPipelineExtractEntitiesOutput
	var contextOutput *BuildContextOutput

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

		extractOutput = &SLMPipelineExtractEntitiesOutput{}
		extractOpts := embeddingOpts
		if input.TimeoutOverride > 0 {
			extractOpts.StartToCloseTimeout = input.TimeoutOverride
		}
		ctxExtract := workflow.WithActivityOptions(ctx, extractOpts)
		err = workflow.ExecuteActivity(ctxExtract, pkgtemporal.ActivityExtractEntitiesActivity, SLMPipelineExtractEntitiesInput{
			TenantID:      input.TenantID,
			SourceID:      input.SourceID,
			ContentID:     input.ContentID,
			JobID:         input.JobID,
			Content:       parsedContent,
			ModelOverride: input.ModelOverride,
		}).Get(ctx, extractOutput)

		// Stage 2b: Extract Assertions (failure does NOT block pipeline)
		var assertionCount int
		ctxAssertions := workflow.WithActivityOptions(ctx, embeddingOpts)
		err2 := workflow.ExecuteActivity(ctxAssertions, pkgtemporal.ActivityExtractAssertions, ExtractAssertionsInput{
			TenantID:    input.TenantID,
			SourceID:    input.SourceID,
			ContentID:   input.ContentID,
			JobID:       input.JobID,
			Content:     parsedContent,
			SenderEmail: input.SenderEmail, // Pass sender for owner attribution
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
			extractOutput = &SLMPipelineExtractEntitiesOutput{}
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

		// Stage 2.5: Email Threading (emails only, non-blocking)
		var threadID *string
		if input.ContentType == "email" {
			logger.Debug("starting email threading",
				"source_id", input.SourceID,
			)
			threadOutput := &GroupEmailThreadOutput{}
			ctxThread := workflow.WithActivityOptions(ctx, fastOpts)
			err = workflow.ExecuteActivity(ctxThread, pkgtemporal.ActivityGroupEmailThread, GroupEmailThreadInput{
				TenantID: input.TenantID,
				SourceID: input.SourceID,
			}).Get(ctx, threadOutput)
			if err != nil {
				logger.Warn("email threading failed (non-blocking)",
					"source_id", input.SourceID,
					"error", err.Error(),
				)
			} else if threadOutput != nil && threadOutput.ThreadID != nil {
				threadID = threadOutput.ThreadID
				logger.Info("email threading completed",
					"source_id", input.SourceID,
					"thread_id", *threadID,
				)
			}
		}

		// Progressive availability: mark as "extracted" (entity-searchable)
		// Also update assertion count if any assertions were extracted
		ctxStatus2 := workflow.WithActivityOptions(ctx, fastOpts)
		updateInput := UpdateContentStatusInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			Status:   "extracted",
		}
		if assertionCount > 0 {
			updateInput.AssertionCount = &assertionCount
		}
		_ = workflow.ExecuteActivity(ctxStatus2, pkgtemporal.ActivityUpdateContentStatus, updateInput).Get(ctx, nil)

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

		contextOutput = &BuildContextOutput{}
		ctxContext := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctxContext, pkgtemporal.ActivityBuildContextPackage, BuildContextInput{
			TenantID:          input.TenantID,
			SourceID:          input.SourceID,
			ContentID:         input.ContentID,
			JobID:             input.JobID,
			ContentType:       input.ContentType,
			Extraction:        extractOutput,
			SenderEmail:       input.SenderEmail,
			SenderName:        input.SenderName,
			Subject:           input.Subject,
			ParticipantEmails: input.ParticipantEmails,
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
			contextOutput = &BuildContextOutput{}
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

		// Stage 3.5: Person Metadata Enrichment (non-blocking)
		// Enriches person entities with title, company, and is_internal flag from email content and domain
		if contextOutput != nil && len(contextOutput.ResolvedPeople) > 0 {
			enrichCtx := workflow.WithActivityOptions(ctx, fastOpts)
			enrichOutput := &EnrichPersonMetadataOutput{}

			// For multi-sender emails (threads), extract per-sender signatures to avoid title cross-contamination
			enrichInput := EnrichPersonMetadataInput{
				TenantID:       input.TenantID,
				ResolvedPeople: contextOutput.ResolvedPeople,
				BodyText:       input.BodyText,
				SenderEmail:    input.SenderEmail,
			}

			// Try per-sender extraction for multiple people
			if len(contextOutput.ResolvedPeople) > 1 {
				perSenderSigs := extractSignaturesPerSender(input.BodyText, contextOutput.ResolvedPeople)
				if len(perSenderSigs) > 0 {
					enrichInput.PerSenderSignatures = perSenderSigs
				} else {
					// Fall back to single signature if no thread separators found.
					// SenderEmail is passed so the activity can scope it to the sender only.
					enrichInput.SignatureText = extractSignature(input.BodyText)
				}
			} else {
				// Single sender: use original extractSignature for backwards compatibility
				enrichInput.SignatureText = extractSignature(input.BodyText)
			}

			err = workflow.ExecuteActivity(enrichCtx, pkgtemporal.ActivityEnrichPersonMetadata, enrichInput).Get(ctx, enrichOutput)
			if err != nil {
				logger.Warn("person metadata enrichment failed (non-blocking)",
					"source_id", input.SourceID,
					"error", err.Error(),
				)
			} else if enrichOutput.PeopleEnriched > 0 {
				logger.Info("person metadata enriched",
					"source_id", input.SourceID,
					"people_enriched", enrichOutput.PeopleEnriched,
				)
			}
		}

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

		var analyzeOutput *DeepAnalyzeOutput
		analyzeOpts := llmOpts
		if input.TimeoutOverride > 0 {
			analyzeOpts.StartToCloseTimeout = input.TimeoutOverride
		}
		ctxAnalyze := workflow.WithActivityOptions(ctx, analyzeOpts)
		analyzeOutput = &DeepAnalyzeOutput{}
		err = workflow.ExecuteActivity(ctxAnalyze, pkgtemporal.ActivityDeepAnalyze, DeepAnalyzeInput{
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
			ModelOverride:     input.ModelOverride,
		}).Get(ctx, analyzeOutput)
		if err != nil {
			durationMs := workflow.Now(ctx).Sub(analyzeStart).Milliseconds()
			logger.Warn("pipeline stage failed (non-blocking)",
				"source_id", input.SourceID,
				"stage", analyzeStage.Name,
				"stage_number", analyzeStage.Number,
				"duration_ms", durationMs,
				"status", "failed",
				"error", err.Error(),
			)
			// Log additional context for timeout errors (bulk reprocessing can overwhelm LLM service)
			if durationMs > 3*60*1000 { // > 3 minutes suggests timeout rather than immediate failure
				logger.Error("Stage 4 DeepAnalyze timeout - LLM service may be overloaded",
					"source_id", input.SourceID,
					"duration_ms", durationMs,
					"heartbeat_timeout_ms", 5*60*1000,
					"error", err.Error(),
				)
			}
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

			var persistOutput PersistFindingsOutput
			ctxPersist := workflow.WithActivityOptions(ctx, fastOpts)
			err = workflow.ExecuteActivity(ctxPersist, pkgtemporal.ActivityPersistFindings, PersistFindingsInput{
				TenantID:       input.TenantID,
				SourceID:       input.SourceID,
				ProjectID:      projectID,
				Analysis:       analyzeOutput,
				ResolvedPeople: resolvedPeople,
				BodyText:       input.BodyText,
				Subject:        input.Subject,
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

				// Update assertion count on source after persist (total from both Stage 2b and Stage 4.5)
				if state.result.AssertionsCreated > 0 {
					totalCount := state.result.AssertionsCreated
					ctxAssertionUpdate := workflow.WithActivityOptions(ctx, fastOpts)
					_ = workflow.ExecuteActivity(ctxAssertionUpdate, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
						TenantID:       input.TenantID,
						SourceID:       input.SourceID,
						Status:         "analyzed",
						AssertionCount: &totalCount,
					}).Get(ctx, nil)
				}
			}
		}
		state.status.StepsCompleted = 6

		// Stage 4.6: Tag Projects (match project keywords)
		updateStatus("tagging_projects", "TagProjects")
		projectTagStage := stageByStatus("tagging_projects")
		logger.Info("pipeline stage starting",
			"source_id", input.SourceID,
			"stage", projectTagStage.Name,
			"stage_number", "4.6",
			"total_steps", state.status.TotalSteps,
		)
		projectTagStart := workflow.Now(ctx)

		var tagProjectsOutput TagProjectsOutput
		ctxTagProjects := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctxTagProjects, pkgtemporal.ActivityTagProjects, TagProjectsInput{
			TenantID:  input.TenantID,
			ContentID: input.SourceID,
		}).Get(ctx, &tagProjectsOutput)
		if err != nil {
			logger.Warn("pipeline stage failed (non-blocking)",
				"source_id", input.SourceID,
				"stage", projectTagStage.Name,
				"stage_number", "4.6",
				"duration_ms", workflow.Now(ctx).Sub(projectTagStart).Milliseconds(),
				"status", "failed",
				"error", err.Error(),
			)
		} else {
			logger.Info("pipeline stage completed",
				"source_id", input.SourceID,
				"stage", projectTagStage.Name,
				"stage_number", "4.6",
				"duration_ms", workflow.Now(ctx).Sub(projectTagStart).Milliseconds(),
				"status", "completed",
				"projects_matched", tagProjectsOutput.ProjectsMatched,
				"mentions_created", tagProjectsOutput.MentionsCreated,
			)
		}
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
		pe := perrors.ClassifyError(err, "embedding")
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("embedding_failed: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Stage 5 Embedding failed — pipeline failed", "error", err, "error_code", pe.Code)

		// Update status to failed
		ctxFail := workflow.WithActivityOptions(ctx, fastOpts)
		_ = workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
			TenantID:        input.TenantID,
			SourceID:        input.SourceID,
			Status:          "failed",
			FailureCategory: string(pe.Code),
			FailureReason:   err.Error(),
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

	// Record overrides if any were provided
	if input.ModelOverride != "" || input.TimeoutOverride > 0 {
		overrides := make(map[string]string)
		if input.ModelOverride != "" {
			overrides["model_override"] = input.ModelOverride
		}
		if input.TimeoutOverride > 0 {
			overrides["timeout_override"] = input.TimeoutOverride.String()
		}
		ctxRecord := workflow.WithActivityOptions(ctx, fastOpts)
		_ = workflow.ExecuteActivity(ctxRecord, pkgtemporal.ActivityRecordOverrides, RecordOverridesInput{
			TenantID:  input.TenantID,
			SourceID:  input.SourceID,
			Overrides: overrides,
		}).Get(ctx, nil)
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

// extractSignature attempts to extract email signature from body text.
// It looks for RFC 3676 "-- " separator or common sign-offs (Regards, Best, etc.)
// and returns everything after the last occurrence. Returns empty string if not found.
func extractSignature(body string) string {
	if body == "" {
		return ""
	}

	// Common signature markers in descending order of specificity
	markers := []string{
		"-- \n",        // RFC 3676 standard
		"\n-- \n",      // RFC 3676 with leading newline
		"\nBest regards,",
		"\nKind regards,",
		"\nWarm regards,",
		"\nRegards,",
		"\nSincerely,",
		"\nCheers,",
		"\nThanks,",
		"\nBest,",
		"\nThank you,",
	}

	lastPos := -1

	// Find the last occurrence of any marker
	for _, marker := range markers {
		if pos := strings.LastIndex(body, marker); pos > lastPos {
			lastPos = pos
		}
	}

	if lastPos == -1 {
		return ""
	}

	// Extract from marker to end, including the marker text
	return strings.TrimSpace(body[lastPos:])
}

// extractSenderFromSeparator tries to extract sender identity from a thread separator line.
// Returns (email, name) where email is preferred if found, otherwise name, or both empty if parse fails.
// Examples:
//   - "On Mon, Feb 10, 2026, John Smith <john@example.com> wrote:" -> ("john@example.com", "John Smith")
//   - "From: Sarah Chen <sarah@example.com>" -> ("sarah@example.com", "Sarah Chen")
//   - "On Mon, Sarah Chen wrote:" -> ("", "Sarah Chen")
func extractSenderFromSeparator(separatorLine string) (email string, name string) {
	// Pattern: email in angle brackets <email@domain.com>
	if idx := strings.Index(separatorLine, "<"); idx != -1 {
		if endIdx := strings.Index(separatorLine[idx:], ">"); endIdx != -1 {
			email = separatorLine[idx+1 : idx+endIdx]
		}
	}

	// Try to extract name from common patterns
	// "On Mon, Feb 10, 2026, John Smith <john@example.com> wrote:"
	// "From: Sarah Chen <sarah@example.com>"
	// "On Mon, Sarah Chen wrote:"

	// For "From:" pattern
	if strings.HasPrefix(strings.TrimSpace(separatorLine), "From:") {
		fromText := strings.TrimPrefix(strings.TrimSpace(separatorLine), "From:")
		fromText = strings.TrimSpace(fromText)
		// Extract name before <email> if present
		if idx := strings.Index(fromText, "<"); idx != -1 {
			name = strings.TrimSpace(fromText[:idx])
		} else {
			name = fromText
		}
		return
	}

	// For "On ... wrote:" pattern
	if strings.Contains(separatorLine, " wrote:") {
		// Extract text between last comma and " wrote:" or between last comma and "<" if email present
		wroteIdx := strings.Index(separatorLine, " wrote:")
		workingText := separatorLine[:wroteIdx]

		// If email present, extract name before it
		if idx := strings.Index(workingText, "<"); idx != -1 {
			// Find the name before the email (after the last comma or "On")
			beforeEmail := workingText[:idx]
			// Look for the last comma
			if lastComma := strings.LastIndex(beforeEmail, ","); lastComma != -1 {
				name = strings.TrimSpace(beforeEmail[lastComma+1:])
			} else if onIdx := strings.Index(beforeEmail, "On "); onIdx != -1 {
				name = strings.TrimSpace(beforeEmail[onIdx+3:])
			}
		} else {
			// No email, try to extract name after last comma
			if lastComma := strings.LastIndex(workingText, ","); lastComma != -1 {
				name = strings.TrimSpace(workingText[lastComma+1:])
			}
		}
	}

	return
}

// extractSignaturesPerSender extracts per-sender signatures from a threaded email body.
// It splits the email body into message blocks using common separators (e.g., "On ... wrote:",
// "-----Original Message-----", "From:") and extracts the signature from each block.
// Returns a map from person name to their signature text.
//
// For single-message emails (no thread separators), returns an empty map and caller should
// fall back to extractSignature() for the single signature.
func extractSignaturesPerSender(bodyText string, people []ResolvedPerson) map[string]string {
	if bodyText == "" || len(people) == 0 {
		return nil
	}

	// Common email thread separators - look for both with and without leading newline
	threadSeparators := []string{
		"\nOn ",                       // "On Mon, Feb 10, 2026, Alice wrote:"
		"\n-----Original Message-----", // Outlook separator
		"\nFrom:",                     // "From: Alice <alice@example.com>"
		"\n> On ",                     // Quoted version
	}

	// Find all separator positions and store the separator line text
	type separatorInfo struct {
		pos  int
		line string
	}
	separators := []separatorInfo{}

	// Check if body starts with a separator pattern (no leading newline)
	for _, sep := range []string{"On ", "-----Original Message-----", "From:", "> On "} {
		if strings.HasPrefix(bodyText, sep) {
			// Extract the full separator line (up to newline or end of text)
			lineEnd := strings.Index(bodyText, "\n")
			if lineEnd == -1 {
				lineEnd = len(bodyText)
			}
			separators = append(separators, separatorInfo{pos: 0, line: bodyText[:lineEnd]})
			break
		}
	}

	// Find all separator positions in the middle of the text
	for _, sep := range threadSeparators {
		idx := 0
		for {
			pos := strings.Index(bodyText[idx:], sep)
			if pos == -1 {
				break
			}
			actualPos := idx + pos
			// Extract the separator line (from newline to next newline)
			lineStart := actualPos
			lineEnd := strings.Index(bodyText[actualPos+1:], "\n")
			if lineEnd == -1 {
				lineEnd = len(bodyText) - actualPos - 1
			}
			separatorLine := bodyText[lineStart : actualPos+1+lineEnd]
			separators = append(separators, separatorInfo{pos: actualPos, line: separatorLine})
			idx = actualPos + 1
		}
	}

	// If no separators found, this is a single-message email, return empty map
	if len(separators) == 0 {
		return nil
	}

	// Sort separators by position
	// Use simple bubble sort since the list is typically small
	for i := 0; i < len(separators); i++ {
		for j := i + 1; j < len(separators); j++ {
			if separators[i].pos > separators[j].pos {
				separators[i], separators[j] = separators[j], separators[i]
			}
		}
	}

	// Split body into message blocks, pairing each block with its preceding separator
	type blockInfo struct {
		text          string
		senderEmail   string
		senderName    string
		hasSeparator  bool
	}
	blocks := []blockInfo{}

	lastPos := 0
	for i, sep := range separators {
		if sep.pos > lastPos {
			// This is the block BEFORE the separator
			var block blockInfo
			block.text = bodyText[lastPos:sep.pos]
			if i > 0 {
				// Extract sender from the previous separator
				email, name := extractSenderFromSeparator(separators[i-1].line)
				block.senderEmail = email
				block.senderName = name
				block.hasSeparator = true
			} else {
				// First block has no preceding separator
				block.hasSeparator = false
			}
			blocks = append(blocks, block)
		}
		lastPos = sep.pos
	}
	// Add final block (after last separator)
	if lastPos < len(bodyText) {
		var block blockInfo
		block.text = bodyText[lastPos:]
		// Extract sender from the last separator
		email, name := extractSenderFromSeparator(separators[len(separators)-1].line)
		block.senderEmail = email
		block.senderName = name
		block.hasSeparator = true
		blocks = append(blocks, block)
	}

	// Extract signature from each block and match to a person
	result := make(map[string]string)
	for _, block := range blocks {
		var matchedPerson *ResolvedPerson

		// Try to match by sender info from separator first
		if block.hasSeparator && (block.senderEmail != "" || block.senderName != "") {
			for i := range people {
				person := &people[i]
				// Match by name from separator (case-insensitive)
				if block.senderName != "" && strings.EqualFold(block.senderName, person.Name) {
					matchedPerson = person
					break
				}
			}
		}

		// Fall back to name-presence matching if separator matching failed
		if matchedPerson == nil {
			for i := range people {
				person := &people[i]
				// Check if person's name appears in the block
				if strings.Contains(block.text, person.Name) {
					matchedPerson = person
					break
				}
			}
		}

		if matchedPerson != nil {
			// Extract signature from this block
			signature := extractSignature(block.text)
			if signature != "" {
				result[matchedPerson.Name] = signature
			}
		}
	}

	return result
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
