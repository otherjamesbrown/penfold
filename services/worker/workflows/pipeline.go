// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
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
	TenantName  string `json:"tenant_name,omitempty"` // Human-readable tenant name for Langfuse environment; falls back to TenantID if empty
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

	// Pipeline routing
	Pipeline string `json:"pipeline,omitempty"` // Pipeline name from routing (e.g., "standard", "transcript"); empty = determine at runtime

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
	ParsedContent       string `json:"parsed_content,omitempty"`
	Category            string `json:"category,omitempty"`
	Importance          string `json:"importance,omitempty"`
	EmbeddingID         *int64 `json:"embedding_id,omitempty"`
	AssertionsCreated   int    `json:"assertions_created,omitempty"`
	ContentContribution string `json:"content_contribution,omitempty"`
	ContributionReason  string `json:"contribution_reason,omitempty"`
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
	TenantID         string            `json:"tenant_id"`
	SourceID         int64             `json:"source_id"`
	ContentID        string            `json:"content_id,omitempty"`
	JobID            string            `json:"job_id"`
	Content          string            `json:"content"`
	Subject          string            `json:"subject,omitempty"`
	SenderEmail      string            `json:"sender_email,omitempty"`
	ContentType   string            `json:"content_type"`
	Headers       map[string]string `json:"headers,omitempty"`        // Email headers for subtype classification
	ModelOverride  string            `json:"model_override,omitempty"` // Optional model override for reprocessing
	PromptOverride int32             `json:"prompt_override,omitempty"` // Optional prompt version override from pipeline definition
	// Langfuse tracing: passed via gRPC metadata to AI coordinator.
	LangfuseTraceID string `json:"langfuse_trace_id,omitempty"`
	LangfusePhaseID string `json:"langfuse_phase_id,omitempty"`
}

// TriageOutput is the output from the Triage activity.
type TriageOutput struct {
	Category            string  `json:"category"`
	Importance          string  `json:"importance"`
	Reason              string  `json:"reason"`
	Confidence          float32 `json:"confidence"`
	ModelUsed           string  `json:"model_used"`
	SkipDeep            bool    `json:"skip_deep"`
	ContentSubtype      string  `json:"content_subtype,omitempty"`
	ContentContribution string  `json:"content_contribution,omitempty"`
	ContributionReason  string  `json:"contribution_reason,omitempty"`
	SourceSystem        string  `json:"source_system,omitempty"` // classified by rule engine in Triage activity

	// Pipeline routing: classification keys and resolved pipelines.
	// RoutingContentType and RoutingSubtype are the uppercase keys used for route lookup.
	// Pipelines contains the matched pipeline names from the routing table.
	RoutingContentType string   `json:"routing_content_type,omitempty"`
	RoutingSubtype     string   `json:"routing_subtype,omitempty"`
	Pipelines          []string `json:"pipelines,omitempty"` // matched pipeline names; empty = skip all
}

// ExtractHeaderMentionsInput is the input for the ExtractHeaderMentions activity.
type ExtractHeaderMentionsInput struct {
	TenantID     string        `json:"tenant_id"`
	SourceID     int64         `json:"source_id"`
	SenderEmail  string        `json:"sender_email"`
	SenderName   string        `json:"sender_name"`
	Participants []Participant `json:"participants"`
}

// ExtractHeaderMentionsOutput is the output from the ExtractHeaderMentions activity.
type ExtractHeaderMentionsOutput struct {
	MentionsCreated int `json:"mentions_created"`
	FromMentions    int `json:"from_mentions"`
	ToMentions      int `json:"to_mentions"`
	CcMentions      int `json:"cc_mentions"`
	GroupExpanded   int `json:"group_expanded"`
}

// SLMPipelineExtractEntitiesInput is the input for the ExtractEntities activity (pipeline version with TriageCategory).
type SLMPipelineExtractEntitiesInput struct {
	TenantID        string `json:"tenant_id"`
	SourceID        int64  `json:"source_id"`
	ContentID       string `json:"content_id,omitempty"`
	JobID           string `json:"job_id"`
	Content         string `json:"content"`
	TriageCategory        string `json:"triage_category,omitempty"`
	ModelOverride         string `json:"model_override,omitempty"` // Optional model override for reprocessing
	NERPromptOverride     int32  `json:"ner_prompt_override,omitempty"`      // Optional NER prompt version override
	SemanticPromptOverride int32 `json:"semantic_prompt_override,omitempty"` // Optional semantic prompt version override

	// Email header metadata for NER prompt enrichment (pf-de2b09).
	// Only populated for email content type.
	ContentType   string        `json:"content_type,omitempty"`
	Subject       string        `json:"subject,omitempty"`
	SenderName    string        `json:"sender_name,omitempty"`
	SenderEmail   string        `json:"sender_email,omitempty"`
	Participants  []Participant `json:"participants,omitempty"`

	// Background context (glossary + topics) for extraction grounding (pf-2f8c70).
	// Prepended to content as a "Background Context" section.
	BackgroundContext string `json:"background_context,omitempty"`

	// PipelineStages is the list of stage names defined in the active pipeline definition.
	// Used by the extraction activity to record only the stages present in the pipeline.
	// Nil or empty means record all stages (backward compat).
	PipelineStages []string `json:"pipeline_stages,omitempty"`

	// Langfuse tracing: passed via gRPC metadata to AI coordinator.
	LangfuseTraceID string `json:"langfuse_trace_id,omitempty"`
	LangfusePhaseID string `json:"langfuse_phase_id,omitempty"`
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
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
	Source string `json:"source,omitempty"` // "header" or "body" — set from PersonEntity.Source (pf-2e6663)
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

// CreateEnrichmentRecordInput contains the data needed to create a content_enrichment record.
type CreateEnrichmentRecordInput struct {
	SourceID                 int64                         `json:"source_id"`
	TenantID                 string                        `json:"tenant_id"`
	ContentType              enrichment.ContentType        `json:"content_type"`
	ContentSubtype           enrichment.ContentSubtype     `json:"content_subtype"`
	SourceSystem             enrichment.SourceSystem       `json:"source_system"`
	ProcessingProfile        enrichment.ProcessingProfile  `json:"processing_profile"`
	ClassificationConfidence float32                       `json:"classification_confidence"`
	ClassificationReason     string                        `json:"classification_reason"`
	ContentStructure         enrichment.ContentStructure   `json:"content_structure,omitempty"` // pf-43acf2
}

// CreateEnrichmentRecordOutput contains the result of creating a content_enrichment record.
type CreateEnrichmentRecordOutput struct {
	EnrichmentID int64 `json:"enrichment_id"`
}

// GroupEmailThreadInput is the input for the GroupEmailThread activity.
type GroupEmailThreadInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
}

// GroupEmailThreadOutput is the output from the GroupEmailThread activity.
type GroupEmailThreadOutput struct {
	ThreadID      *string `json:"thread_id,omitempty"`       // Root message ID (nil if not threaded)
	EmailThreadID *int64  `json:"email_thread_id,omitempty"` // Numeric email_threads.id for assertion dedup
}

// LinkConversationInput is the input for the LinkConversation activity.
type LinkConversationInput struct {
	TenantID  string `json:"tenant_id"`
	SourceID  int64  `json:"source_id"`
	ThreadID  string `json:"thread_id"`  // Root message ID from threading
	ContentID string `json:"content_id"` // Content item ID to link
}

// LinkConversationOutput is the output from the LinkConversation activity.
type LinkConversationOutput struct {
	ConversationID string `json:"conversation_id,omitempty"` // Empty if skipped or failed
}

// BuildExtractionContextInput is the input for the lightweight pre-extraction
// context builder. Unlike BuildContextPackage, this doesn't need extraction
// results — it scans raw text for acronyms and topic candidates.
type BuildExtractionContextInput struct {
	TenantID string `json:"tenant_id"`
	Subject  string `json:"subject,omitempty"`
	Content  string `json:"content"`
}

// BuildExtractionContextOutput returns a formatted context string containing
// glossary terms and topic descriptions (no actions/decisions/risks).
type BuildExtractionContextOutput struct {
	BackgroundContext string `json:"background_context"`
	GlossaryCount    int    `json:"glossary_count"`
	TopicCount       int    `json:"topic_count"`
}

// BuildNewsletterContextInput is the input for the newsletter-specific context builder.
// It enriches the extraction context with user, glossary, project, and product sections.
type BuildNewsletterContextInput struct {
	TenantID string `json:"tenant_id"`
	Subject  string `json:"subject,omitempty"`
	Content  string `json:"content"`
}

// BuildNewsletterContextOutput is the output of the newsletter context builder.
type BuildNewsletterContextOutput struct {
	BackgroundContext string `json:"background_context"`
	UserContextFound  bool   `json:"user_context_found"`
	GlossaryCount     int    `json:"glossary_count"`
	ProjectCount      int    `json:"project_count"`
	ProductCount      int    `json:"product_count"`
}

// BuildContextInput is the input for the BuildContextPackage activity.
type BuildContextInput struct {
	TenantID          string                            `json:"tenant_id"`
	SourceID          int64                             `json:"source_id"`
	ContentID         string                            `json:"content_id,omitempty"`
	JobID             string                            `json:"job_id"`
	ContentType       string                            `json:"content_type"`
	ContentSubtype    string                            `json:"content_subtype,omitempty"` // pf-bcb565: used for context scaling
	Extraction        *SLMPipelineExtractEntitiesOutput `json:"extraction"`
	SenderEmail       string                            `json:"sender_email,omitempty"`
	SenderName        string                            `json:"sender_name,omitempty"`
	Subject           string                            `json:"subject,omitempty"`
	ThreadID          string                            `json:"thread_id,omitempty"`
	ParticipantEmails []Participant                     `json:"participant_emails,omitempty"`
	ConversationID    string                            `json:"conversation_id,omitempty"`
	Content           string                            `json:"content,omitempty"`
}

// BuildContextOutput is the output from the BuildContextPackage activity.
type BuildContextOutput struct {
	ResolvedPeople       []ResolvedPerson                  `json:"resolved_people"`
	ResolvedProjects     []ResolvedProject                 `json:"resolved_projects"`
	UnresolvedTerms      []string                          `json:"unresolved_terms"`
	ContextPackage       *ContextPackage                   `json:"context_package"`
	TokensUsed           int                               `json:"tokens_used"`
	TokenBudget          int                               `json:"token_budget"`
	EntitiesResolved     int                               `json:"entities_resolved"`
	EntitiesUnresolved   int                               `json:"entities_unresolved"`
	// CorrectedExtraction carries the extraction after Stage 3 post-processing
	// (e.g. org→project reclassification). Temporal serialises activity I/O so
	// in-place mutations to the input don't propagate back to the workflow.
	CorrectedExtraction  *SLMPipelineExtractEntitiesOutput `json:"corrected_extraction,omitempty"`
	// BackgroundContext is the assembled markdown context string for Stage 4 (deep_analyze).
	// Populated by BuildContextPackage via BuildStageContext; replaces formatContextPackage(output).
	BackgroundContext     string                            `json:"background_context,omitempty"`
}

// BuildStageContextInput is the input for the BuildStageContext activity.
// Pipeline and Stage identify which row in pipeline_definitions to read context_providers from.
type BuildStageContextInput struct {
	TenantID       string `json:"tenant_id"`
	Pipeline       string `json:"pipeline"`
	Stage          string `json:"stage"`
	// Provider fields — all optional; providers use what they need.
	SourceID          int64                             `json:"source_id,omitempty"`
	ContentID         string                            `json:"content_id,omitempty"`
	JobID             string                            `json:"job_id,omitempty"`
	ContentType       string                            `json:"content_type,omitempty"`
	ContentSubtype    string                            `json:"content_subtype,omitempty"`
	Content           string                            `json:"content,omitempty"`
	Subject           string                            `json:"subject,omitempty"`
	SenderEmail       string                            `json:"sender_email,omitempty"`
	SenderName        string                            `json:"sender_name,omitempty"`
	ThreadID          string                            `json:"thread_id,omitempty"`
	ParticipantEmails []Participant                     `json:"participant_emails,omitempty"`
	ConversationID    string                            `json:"conversation_id,omitempty"`
	Extraction        *SLMPipelineExtractEntitiesOutput `json:"extraction,omitempty"`
	ResolvedPeople    []ResolvedPerson                  `json:"resolved_people,omitempty"`
	ResolvedProjects  []ResolvedProject                 `json:"resolved_projects,omitempty"`
	// Optional Langfuse trace/phase IDs for observability.
	LangfuseTraceID string `json:"langfuse_trace_id,omitempty"`
	LangfusePhaseID string `json:"langfuse_phase_id,omitempty"`
}

// ResolvedPerson represents a person resolved from extraction.
type ResolvedPerson struct {
	Name          string  `json:"name"`
	PersonID      *int64  `json:"person_id,omitempty"`
	Confidence    float32 `json:"confidence"`
	Source        string  `json:"source"`
	Role          string  `json:"role,omitempty"`
	Title         string  `json:"title,omitempty"`
	Department    string  `json:"department,omitempty"`
	IsInternal    bool    `json:"is_internal"`
	IsPrimaryUser bool    `json:"is_primary_user,omitempty"`
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
	GlossaryTerms      []ContextGlossaryTerm      `json:"glossary_terms,omitempty"`
	TopicDescriptions  []ContextTopicDescription  `json:"topic_descriptions,omitempty"`
	ParticipantContext []ResolvedPerson           `json:"participant_context,omitempty"`
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

// ContextTopicDescription represents a topic in the context package.
// Topics provide paragraph-level context (richer than glossary) without actions/risks.
type ContextTopicDescription struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FormatForPrompt serializes the ContextPackage into a human-readable string
// for inclusion in the LLM analyze prompt's {background_context} variable.
func (cp *ContextPackage) FormatForPrompt() string {
	if cp == nil {
		return ""
	}

	var sections []string

	if len(cp.GlossaryTerms) > 0 {
		var lines []string
		for _, t := range cp.GlossaryTerms {
			lines = append(lines, fmt.Sprintf("- **%s**: %s", t.Term, t.Definition))
		}
		sections = append(sections, "### Glossary\n"+strings.Join(lines, "\n"))
	}

	if len(cp.TopicDescriptions) > 0 {
		var lines []string
		for _, t := range cp.TopicDescriptions {
			lines = append(lines, fmt.Sprintf("- **%s**: %s", t.Name, t.Description))
		}
		sections = append(sections, "### Topic Context\n"+strings.Join(lines, "\n"))
	}

	// Note: Participant Context is no longer rendered here. People context is now
	// provided via the enriched Entities section in the analyze prompt (pf-9c1485).

	if len(cp.ActiveRisks) > 0 {
		var lines []string
		for _, a := range cp.ActiveRisks {
			lines = append(lines, fmt.Sprintf("- %s %s %s", a.Subject, a.Predicate, a.Object))
		}
		sections = append(sections, "### Active Risks\n"+strings.Join(lines, "\n"))
	}

	if len(cp.OpenActions) > 0 {
		var lines []string
		for _, a := range cp.OpenActions {
			lines = append(lines, fmt.Sprintf("- %s %s %s", a.Subject, a.Predicate, a.Object))
		}
		sections = append(sections, "### Open Actions\n"+strings.Join(lines, "\n"))
	}

	if len(cp.RecentDecisions) > 0 {
		var lines []string
		for _, a := range cp.RecentDecisions {
			lines = append(lines, fmt.Sprintf("- %s %s %s", a.Subject, a.Predicate, a.Object))
		}
		sections = append(sections, "### Recent Decisions\n"+strings.Join(lines, "\n"))
	}

	if len(cp.ProductEvents) > 0 {
		var lines []string
		for _, e := range cp.ProductEvents {
			line := fmt.Sprintf("- [%s] %s", e.EventType, e.Description)
			if e.Timestamp != "" {
				line += " (" + e.Timestamp + ")"
			}
			lines = append(lines, line)
		}
		sections = append(sections, "### Product Events\n"+strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

// selectConfirmedProjectID cross-references resolved projects with the deep
// analysis topic mappings. Returns the first project ID that the analysis
// confirms is actually related to the content. Returns nil if no project
// is confirmed with sufficient confidence, preventing mis-tagging of
// assertions from unrelated content (pf-de3670).
func selectConfirmedProjectID(resolved []ResolvedProject, topicMappings []TopicMappingOutput) *int64 {
	if len(resolved) == 0 {
		return nil
	}

	// If no topic mappings, the analysis found no project connections — don't tag.
	if len(topicMappings) == 0 {
		return nil
	}

	// Build a set of confirmed project names from topic mappings with confidence >= 0.5
	confirmed := make(map[string]bool)
	for _, tm := range topicMappings {
		if tm.Confidence >= 0.5 && tm.RelatedProject != "" {
			confirmed[strings.ToLower(tm.RelatedProject)] = true
		}
	}

	// Find first resolved project that is confirmed by the analysis
	for _, proj := range resolved {
		if proj.ProjectID != nil && confirmed[strings.ToLower(proj.Name)] {
			return proj.ProjectID
		}
	}

	return nil
}

// DeepAnalyzeInput is the input for the DeepAnalyze activity.
type DeepAnalyzeInput struct {
	TenantID          string                            `json:"tenant_id"`
	SourceID          int64                             `json:"source_id"`
	ContentID         string                            `json:"content_id,omitempty"`
	JobID             string                            `json:"job_id"`
	Content           string                            `json:"content"`
	Subject           string                            `json:"subject,omitempty"` // Email subject — topic framing for deep analysis (pf-e219c1)
	ContentType       string                            `json:"content_type"`
	TriageCategory    string                            `json:"triage_category"`
	TriageImportance  string                            `json:"triage_importance"`
	ExtractionResult  *SLMPipelineExtractEntitiesOutput `json:"extraction_result"`
	ResolvedPeople    []ResolvedPerson                  `json:"resolved_people,omitempty"`
	BackgroundContext string `json:"background_context,omitempty"`
	ModelOverride    string `json:"model_override,omitempty"` // Optional model override for reprocessing
	PromptOverride   int32  `json:"prompt_override,omitempty"` // Optional prompt version override from pipeline definition
	// Langfuse tracing: passed via gRPC metadata to AI coordinator.
	LangfuseTraceID string `json:"langfuse_trace_id,omitempty"`
	LangfusePhaseID string `json:"langfuse_phase_id,omitempty"`
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
	AssertionsCreated      int `json:"assertions_created"`
	AssertionsSuperseded   int `json:"assertions_superseded"`
	AssertionsDeduplicated int `json:"assertions_deduplicated"`
	ReferencesCreated      int `json:"references_created"`
	ReviewItemsCreated     int `json:"review_items_created"`
	AffinityUpdates        int `json:"affinity_updates"`
}

// NewsletterExtractInput is the input for the NewsletterExtract activity.
type NewsletterExtractInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Content  string `json:"content"`
}

// NewsletterExtractOutput is the output from the NewsletterExtract activity.
type NewsletterExtractOutput struct {
	RawJSON          json.RawMessage `json:"raw_json"`
	ModelUsed        string          `json:"model_used"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// NotificationExtractInput is the input for the NotificationExtract activity.
type NotificationExtractInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Content  string `json:"content"`
}

// NotificationExtractOutput is the output from the NotificationExtract activity.
type NotificationExtractOutput struct {
	RawJSON          json.RawMessage `json:"raw_json"`
	ModelUsed        string          `json:"model_used"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// StructuredExtractInput is the input for the generic StructuredExtract activity.
// JSON field names must match activities.StructuredExtractInput for Temporal serialization.
type StructuredExtractInput struct {
	TenantID          string `json:"tenant_id"`
	SourceID          int64  `json:"source_id"`
	Content           string `json:"content"`
	StageName         string `json:"stage_name"`
	PromptOverride    int32  `json:"prompt_override,omitempty"`
	BackgroundContext string `json:"background_context,omitempty"`
	LangfuseTraceID   string            `json:"langfuse_trace_id,omitempty"`
	LangfusePhaseID   string            `json:"langfuse_phase_id,omitempty"`
	ContextMeta       map[string]string `json:"context_meta,omitempty"`
}

// StructuredExtractOutput is the output from the generic StructuredExtract activity.
// JSON field names must match activities.StructuredExtractOutput for Temporal serialization.
type StructuredExtractOutput struct {
	RawJSON          json.RawMessage `json:"raw_json"`
	ModelUsed        string          `json:"model_used"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
	StageName        string          `json:"stage_name"`
}

// PersistExtractedDataInput is the input for the PersistExtractedData activity.
type PersistExtractedDataInput struct {
	TenantID string          `json:"tenant_id"`
	SourceID int64           `json:"source_id"`
	Key      string          `json:"key"`
	Data     json.RawMessage `json:"data"`
}

// PersistExtractedDataOutput is the output from the PersistExtractedData activity.
type PersistExtractedDataOutput struct {
	Updated bool `json:"updated"`
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

// EnrichEntitiesInput is the input for the EnrichEntities activity.
// This stage runs after resolve to compute communication patterns and expertise areas.
type EnrichEntitiesInput struct {
	TenantID          string        `json:"tenant_id"`
	SourceID          int64         `json:"source_id"`
	ContentID         string        `json:"content_id,omitempty"`
	Content           string        `json:"content"`
	ResolvedPeople    []ResolvedPerson `json:"resolved_people"`
	ParticipantEmails []Participant `json:"participant_emails,omitempty"`
}

// EnrichEntitiesOutput is the output from the EnrichEntities activity.
type EnrichEntitiesOutput struct {
	PatternsUpdated  int `json:"patterns_updated"`
	ExpertiseUpdated int `json:"expertise_updated"`
}

// RecordOverridesInput is the input for recording override parameters in pipeline_runs.
type RecordOverridesInput struct {
	TenantID  string            `json:"tenant_id"`
	SourceID  int64             `json:"source_id"`
	Overrides map[string]string `json:"overrides"`
}

// KickNextPendingInput is the input for the KickNextPending activity.
// This activity calls the gateway's KickProcessing RPC to automatically
// start the next pending pipeline after the current one completes.
type KickNextPendingInput struct {
	TenantID string `json:"tenant_id"`
	Limit    int32  `json:"limit"` // Max items to kick (typically 1)
}

// EvaluateEventTriggersInput is the input for the EvaluateEventTriggers activity.
// Fields are populated from pipeline workflow state at completion.
type EvaluateEventTriggersInput struct {
	TenantID       string `json:"tenant_id"`
	SourceID       int64  `json:"source_id"`
	ContentType    string `json:"content_type,omitempty"`
	ContentSubtype string `json:"content_subtype,omitempty"`
	SourceSystem   string `json:"source_system,omitempty"`
	SenderEmail    string `json:"sender_email,omitempty"` // matched against "from" field
	Subject        string `json:"subject,omitempty"`      // matched against "subject" field (regex)
	Urgency        string `json:"urgency,omitempty"`      // from triage Importance, lowercased
}

// EvaluateEventTriggersOutput is the output from the EvaluateEventTriggers activity.
type EvaluateEventTriggersOutput struct {
	RulesEvaluated   int `json:"rules_evaluated"`
	RulesMatched     int `json:"rules_matched"`
	WorkflowsStarted int `json:"workflows_started"`
}

// KickNextPendingOutput is the output from the KickNextPending activity.
type KickNextPendingOutput struct {
	QueuedCount int64  `json:"queued_count"` // Number of items successfully queued
	Message     string `json:"message"`      // Human-readable result
}

// SkippedStage describes a pipeline stage that was skipped due to gating logic.
type SkippedStage struct {
	Stage      string `json:"stage"`       // Stage name, e.g. "extract_ner", "analyze"
	SkipReason string `json:"skip_reason"` // e.g. "contribution_gating:NONE", "category_skip:PERSONAL/LOW"
}

// DeleteAssertionsInput is the input for the DeleteAssertions activity.
// It requests deletion of all assertions for a source, used when reprocessing
// with skipExtract=true (e.g. after auto-reply reclassification).
type DeleteAssertionsInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
}

// DeleteAssertionsOutput is the output from the DeleteAssertions activity.
type DeleteAssertionsOutput struct {
	Deleted int `json:"deleted"` // Number of assertions removed
}

// FetchPipelineDefinitionInput is the input for the FetchPipelineDefinition activity.
type FetchPipelineDefinitionInput struct {
	TenantID string `json:"tenant_id"`
	Pipeline string `json:"pipeline"`
}

// FetchPipelineDefinitionOutput is the output from the FetchPipelineDefinition activity.
type FetchPipelineDefinitionOutput struct {
	Found               bool                  `json:"found"`                  // True if definition was found in DB
	ContentType         string                `json:"content_type,omitempty"` // Content type (email, meeting, etc.)
	Stages              []PipelineStageConfig `json:"stages"`                 // Ordered stage configurations
}

// PipelineStageConfig describes a stage's configuration from pipeline_definitions.
type PipelineStageConfig struct {
	Stage          string   `json:"stage"`
	StageKind      string   `json:"stage_kind"`
	PersistKey     string   `json:"persist_key,omitempty"`
	StageOrder     int      `json:"stage_order"`
	Enabled        bool     `json:"enabled"`
	SkipWhenLow    bool     `json:"skip_when_low"`
	Optional       bool     `json:"optional"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	ModelOverride  string   `json:"model_override,omitempty"`
	PromptOverride int32    `json:"prompt_override,omitempty"`
	DependsOn      []string `json:"depends_on,omitempty"` // Stage names that must complete successfully before this stage runs
}

// RecordSkippedStageInput is the input for the RecordSkippedStage activity.
// It records pipeline_runs rows with status "skipped" for all skipped stages in a single call.
type RecordSkippedStageInput struct {
	SourceID        int64          `json:"source_id"`
	Stages          []SkippedStage `json:"stages"`
	LangfuseTraceID string         `json:"langfuse_trace_id,omitempty"`
}

// PreClassifyInput is the input for the PreClassify activity (pf-b375ad).
// Runs the DB-backed rule engine against sender metadata before triage,
// in shadow mode alongside looksLikeNotificationSender().
type PreClassifyInput struct {
	TenantID    string            `json:"tenant_id"`
	ContentType string            `json:"content_type"`
	SenderEmail string            `json:"sender_email"`
	Subject     string            `json:"subject,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// PreClassifyOutput is the output from the PreClassify activity.
type PreClassifyOutput struct {
	Matched        bool   `json:"matched"`                   // True if a rule matched
	ContentSubtype string `json:"content_subtype,omitempty"` // e.g. "NOTIFICATION", "NEWSLETTER", "HUMAN"
	RuleName       string `json:"rule_name,omitempty"`       // Name of the matched rule
	PipelineName   string `json:"pipeline_name,omitempty"`   // Resolved pipeline name from routing table (empty = no route)
	Error          string `json:"error,omitempty"`           // Non-empty if rule loading/evaluation failed
}

// pipelineState maintains the internal state of the pipeline workflow.
type pipelineState struct {
	status          PipelineStatus
	result          *PipelineResult
	cancelRequested bool
	cancelReason    string
}

// pipelineTraceName converts a pipeline name to a Langfuse trace name.
// Examples: "standard" → "email-processing", "transcript" → "transcript-processing",
// "attendees_only" → "attendees-only-processing".
func pipelineTraceName(pipeline string) string {
	if pipeline == "" || pipeline == "standard" {
		return "email-processing"
	}
	return strings.ReplaceAll(pipeline, "_", "-") + "-processing"
}

// formatParticipants builds a comma-separated string of participant emails for Langfuse display.
func formatParticipants(participants []Participant) string {
	if len(participants) == 0 {
		return ""
	}
	parts := make([]string, 0, len(participants))
	for _, p := range participants {
		if p.Email != "" {
			parts = append(parts, p.Email)
		}
	}
	return strings.Join(parts, ", ")
}

// langfuseBodyPreviewMaxRunes is the maximum number of runes to include in the
// Langfuse root-span body preview.
const langfuseBodyPreviewMaxRunes = 500

// truncateBody returns the first maxLen runes of body text, appending "…" if truncated.
func truncateBody(body string, maxLen int) string {
	runes := []rune(body)
	if len(runes) <= maxLen {
		return body
	}
	return string(runes[:maxLen]) + "…"
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
			TotalSteps:     0, // Updated after pipeline definition loads
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
				RetryPolicy: &temporal.RetryPolicy{
					InitialInterval:        time.Second,
					BackoffCoefficient:     2.0,
					MaximumAttempts:        3,
					NonRetryableErrorTypes: pkgtemporal.NonRetryableErrors(),
				},
			})

			// Set failure fields based on workflow state
			failureCategory := "processing_error"
			failureReason := "Workflow terminated abnormally"
			if state.result.Error != "" {
				failureReason = state.result.Error
			}

			if err := workflow.ExecuteActivity(cleanupCtx, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID:        input.TenantID,
				SourceID:        input.SourceID,
				Status:          "failed",
				FailureCategory: failureCategory,
				FailureReason:   failureReason,
			}).Get(cleanupCtx, nil); err != nil {
				logger.Error("Failed to update content status",
					"source_id", input.SourceID,
					"target_status", "failed",
					"error", err,
				)
			}

			// Auto-drain: kick next pending item even on failure to maintain concurrency window
			// This is best-effort — errors are logged but don't affect cleanup
			var kickOutput KickNextPendingOutput
			kickErr := workflow.ExecuteActivity(cleanupCtx, pkgtemporal.ActivityKickNextPending, KickNextPendingInput{
				TenantID: input.TenantID,
				Limit:    0, // limit read from pipeline.kick_next_limit config by the activity
			}).Get(cleanupCtx, &kickOutput)
			if kickErr != nil {
				logger.Warn("Auto-drain kick failed during cleanup (non-blocking)",
					"source_id", input.SourceID,
					"error", kickErr,
				)
			} else {
				logger.Info("Auto-drain kick completed during cleanup",
					"source_id", input.SourceID,
					"queued_count", kickOutput.QueuedCount,
				)
			}
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
	llmOpts := pkgtemporal.LLMActivityOptions()

	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	llmOpts = pkgtemporal.WithNonRetryableErrors(llmOpts, pkgtemporal.NonRetryableErrors()...)

	// stageConfigMap is populated after FetchPipelineDefinition completes.
	// It's declared here so the stageOpts closure can capture it.
	var stageConfigMap map[string]PipelineStageConfig
	// stageConfigSlice is the deterministically-ordered companion to stageConfigMap.
	// Use this for any iteration in the workflow body — map iteration is non-deterministic.
	var stageConfigSlice []PipelineStageConfig

	// Per-stage timeout helper: returns activity options using per-stage config if available,
	// falling back to the provided category defaults.
	stageOpts := func(stage string, fallback workflow.ActivityOptions) workflow.ActivityOptions {
		opts := fallback
		// Override with per-pipeline timeouts from pipeline_definitions (via stageConfigMap).
		// stageConfigMap is populated after FetchPipelineDefinition and contains
		// pipeline-specific timeouts (e.g. transcript pipeline has extract_ner=600s).
		if stageConfigMap != nil {
			if cfg, ok := stageConfigMap[stage]; ok && cfg.TimeoutSeconds > 0 {
				opts.StartToCloseTimeout = time.Duration(cfg.TimeoutSeconds) * time.Second
				opts.HeartbeatTimeout = time.Duration(cfg.TimeoutSeconds/2) * time.Second
			}
		}
		return opts
	}

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

	// originalSourceSystem holds the raw source_system value from the DB (e.g. "teams", "zoom").
	// Populated only when FetchSource is called (i.e. ContentType was empty on entry).
	// Used after triage to restore the correct source_system for non-email content (pf-e494df).
	var originalSourceSystem string

	// fetchedHeaders holds the email MIME headers from FetchSource for triage classification.
	// Populated only when FetchSource is called (i.e. ContentType was empty on entry).
	var fetchedHeaders map[string]string

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
			if err := workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID: input.TenantID, SourceID: input.SourceID,
				Status: "failed", FailureCategory: string(pe.Code), FailureReason: err.Error(),
			}).Get(ctx, nil); err != nil {
				logger.Error("Failed to update content status",
					"source_id", input.SourceID,
					"target_status", "failed",
					"error", err,
				)
			}
			return state.result, nil
		}
		input.ContentType = fetchOut.ContentType
		input.BodyText = fetchOut.ContentText
		input.Subject = fetchOut.Subject
		input.SenderEmail = fetchOut.SenderEmail
		input.SenderName = fetchOut.SenderName
		input.ParticipantEmails = fetchOut.ParticipantEmails
		// pf-e494df: remember original source_system from DB for non-email content
		if fetchOut.SourceSystem != "" {
			originalSourceSystem = fetchOut.SourceSystem
		}
		// pf-3418d4: populate ContentID from DB if not already set (needed for conversation linking)
		if input.ContentID == "" && fetchOut.ContentID != "" {
			input.ContentID = fetchOut.ContentID
		}
		// For meeting content, also populate TranscriptContent so ParseTranscript has data (pf-0065d5)
		if input.ContentType == "meeting" {
			input.TranscriptContent = fetchOut.ContentText
		}
		// For email content, also populate BodyHTML from FetchSource for HTML-only emails (pf-dfbc24)
		if input.ContentType == "email" {
			input.BodyHTML = fetchOut.BodyHTML
		}
		// Store headers for triage classification (e.g., Content-Type for calendar detection)
		if len(fetchOut.Headers) > 0 {
			fetchedHeaders = fetchOut.Headers
		}
	}

	// Generate Langfuse trace ID for the direct Langfuse ingestion API.
	langfuseTraceID := sideEffectUUID(ctx)

	// Create a Langfuse trace for this pipeline run (best-effort, non-blocking).
	// Tenant is identified via the Environment field (TenantName), not a tag.
	langfuseTraceTags := []string{"cont:" + input.ContentID}
	ctxLangfuse := workflow.WithActivityOptions(ctx, fastOpts)
	var langfuseTraceOut *CreateLangfuseTraceOutput
	langfuseErr := workflow.ExecuteActivity(ctxLangfuse, pkgtemporal.ActivityCreateLangfuseTrace, CreateLangfuseTraceInput{
		TraceID:      langfuseTraceID,
		Name:         pipelineTraceName(input.Pipeline),
		ContentID:    input.ContentID,
		TenantID:     input.TenantID,
		Tags:         langfuseTraceTags,
		TenantName:   input.TenantName, // Human-readable name for Langfuse environment; falls back to TenantID if empty
		SourceSystem: input.ContentType,
		Subject:      input.Subject,
		ContentType:  input.ContentType,
		SenderEmail:  input.SenderEmail,
		SenderName:   input.SenderName,
		Recipients:   formatParticipants(input.ParticipantEmails),
		Date:         fetchedHeaders["Date"],
		BodyPreview:  truncateBody(input.BodyText, langfuseBodyPreviewMaxRunes),
	}).Get(ctx, &langfuseTraceOut)
	// Root span ID for nesting phase spans and closing with real duration (pf-1bfbaf).
	var rootSpanID string
	if langfuseTraceOut != nil {
		rootSpanID = langfuseTraceOut.RootSpanID
	}
	if langfuseErr != nil {
		logger.Warn("Failed to create Langfuse trace (non-fatal)", "error", langfuseErr)
	}

	// Persist the Langfuse trace ID back to the sources table (best-effort, non-blocking).
	if langfuseErr == nil {
		_ = workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityPersistLangfuseTraceID,
			PersistLangfuseTraceIDInput{
				SourceID: fmt.Sprintf("%d", input.SourceID),
				TraceID:  langfuseTraceID,
			},
		).Get(ctx, nil)
	}

	// reportLangfuseSkip emits a Langfuse span for a skipped pipeline phase.
	reportLangfuseSkip := func(phaseName, reason string) {
		start := workflow.Now(ctx)
		_ = workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityReportLangfusePhase,
			ReportLangfusePhaseInput{
				PhaseID:       sideEffectUUID(ctx),
				TraceID:       langfuseTraceID,
				PhaseName:     phaseName,
				StartTime:     start,
				EndTime:       workflow.Now(ctx),
				ParentSpanID:  rootSpanID,
				Level:         "DEFAULT",
				StatusMessage: reason,
			},
		).Get(ctx, nil)
	}

	var parsedContent string
	var quotedContent string // parent email body for reference resolution (pf-90b749)

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
			if err := workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID: input.TenantID, SourceID: input.SourceID,
				Status: "failed", FailureCategory: string(pe.Code), FailureReason: err.Error(),
			}).Get(ctx, nil); err != nil {
				logger.Error("Failed to update content status",
					"source_id", input.SourceID,
					"target_status", "failed",
					"error", err,
				)
			}
			return state.result, nil
		}
		if parseOutput.NewContent != "" {
			// For forward emails, don't discard the forwarded body when the
			// sender's comment is minimal. Outlook-style forwards have the
			// forwarded message's From:/Date: headers that look like quoted
			// reply markers, causing separateQuotedReply to keep only the
			// brief sender comment (e.g. "FYI") (pf-100e09).
			subjectLower := strings.ToLower(input.Subject)
			isForward := strings.HasPrefix(subjectLower, "fw:") ||
				strings.HasPrefix(subjectLower, "fwd:")
			if isForward && len(strings.Fields(parseOutput.NewContent)) < 10 {
				parsedContent = parseOutput.CleanBody
			} else {
				parsedContent = parseOutput.NewContent
			}
		} else {
			parsedContent = parseOutput.CleanBody
		}
		input.BodyText = parsedContent
		quotedContent = parseOutput.QuotedContent // for assertion reference resolution (pf-90b749)
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
			if err := workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID: input.TenantID, SourceID: input.SourceID,
				Status: "failed", FailureCategory: string(pe.Code), FailureReason: err.Error(),
			}).Get(ctx, nil); err != nil {
				logger.Error("Failed to update content status",
					"source_id", input.SourceID,
					"target_status", "failed",
					"error", err,
				)
			}
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
		if err := workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
			TenantID: input.TenantID, SourceID: input.SourceID,
			Status: "failed", FailureCategory: "processing_error",
			FailureReason: state.result.Error,
		}).Get(ctx, nil); err != nil {
			logger.Error("Failed to update content status",
				"source_id", input.SourceID,
				"target_status", "failed",
				"error", err,
			)
		}
		return state.result, nil
	}

	state.result.ParsedContent = parsedContent
	state.status.StepsCompleted = 1

	// Progressive availability: mark as "parsed" (keyword-searchable)
	ctxStatus := workflow.WithActivityOptions(ctx, fastOpts)
	if err := workflow.ExecuteActivity(ctxStatus, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		Status:   "parsed",
	}).Get(ctx, nil); err != nil {
		logger.Error("Failed to update content status",
			"source_id", input.SourceID,
			"target_status", "parsed",
			"error", err,
		)
	}

	if checkCancellation() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		return state.result, nil
	}

	// Pre-classification for first-time ingestion (pf-1c083d, pf-7640c8).
	// When input.Pipeline is empty, triage hasn't run yet so the pipeline name is
	// unknown. Run the DB-backed rule engine (PreClassifyContent) to detect notification
	// and newsletter senders before triage. If a rule matches, the pipeline name is known
	// early so the early pipeline definition fetch (below) loads prompt_override before
	// triage runs. This avoids the chicken-and-egg where prompt_override=0 is passed to
	// triage because the pipeline wasn't known until triage completed.
	//
	// If the rule engine returns no match, fall back to hardcoded notification sender
	// detection for tenants that haven't seeded classification rules.
	// If triage later routes to a different pipeline (edge case), the post-triage
	// pipeline re-fetch corrects it.
	var preclassifiedPipeline string
	if input.Pipeline == "" && strings.EqualFold(input.ContentType, "email") && input.SenderEmail != "" {
		ctxPreClassify := workflow.WithActivityOptions(ctx, fastOpts)
		var preClassifyContentOut PreClassifyContentOutput
		preClassifyContentErr := workflow.ExecuteActivity(ctxPreClassify, pkgtemporal.ActivityPreClassifyContent, PreClassifyContentInput{
			TenantID:    input.TenantID,
			ContentType: input.ContentType,
			SenderEmail: input.SenderEmail,
			Subject:     input.Subject,
			Headers:     fetchedHeaders,
		}).Get(ctx, &preClassifyContentOut)

		if preClassifyContentErr != nil {
			logger.Warn("Pre-classify activity failed, falling back to hardcoded detection",
				"error", preClassifyContentErr,
				"sender_email", input.SenderEmail,
			)
		} else if preClassifyContentOut.Pipeline != "" {
			preclassifiedPipeline = preClassifyContentOut.Pipeline
			logger.Info("Pre-classified pipeline from rule engine",
				"sender_email", input.SenderEmail,
				"pipeline", preclassifiedPipeline,
				"content_subtype", preClassifyContentOut.ContentSubtype,
				"rule_name", preClassifyContentOut.RuleName,
			)
		}

		// Fallback: hardcoded notification sender detection for tenants without seeded rules.
		if preclassifiedPipeline == "" && looksLikeNotificationSender(input.SenderEmail) {
			preclassifiedPipeline = "notification"
			logger.Info("Pre-classified as notification pipeline from sender address (hardcoded fallback)",
				"sender_email", input.SenderEmail,
				"pipeline", preclassifiedPipeline,
			)
		}
	}

	// Early pipeline definition fetch: when the pipeline is known before triage
	// (e.g. notification, newsletter), load the definition so prompt_override
	// reaches the triage stage via stageConfigMap.
	// If triage routes to a different pipeline, we re-fetch after triage.
	var earlyPipelineName string
	var earlyPipelineDef *FetchPipelineDefinitionOutput
	resolvedEarlyPipeline := input.Pipeline
	if resolvedEarlyPipeline == "" {
		resolvedEarlyPipeline = preclassifiedPipeline
	}
	if resolvedEarlyPipeline != "" {
		earlyPipelineName = resolvedEarlyPipeline
		ctxDef := workflow.WithActivityOptions(ctx, fastOpts)
		var defOut FetchPipelineDefinitionOutput
		defErr := workflow.ExecuteActivity(ctxDef, pkgtemporal.ActivityFetchPipelineDefinition, FetchPipelineDefinitionInput{
			TenantID: input.TenantID,
			Pipeline: earlyPipelineName,
		}).Get(ctx, &defOut)
		if defErr != nil {
			logger.Warn("Early pipeline definition fetch failed, prompt overrides won't apply to triage",
				"pipeline", earlyPipelineName,
				"error", defErr,
			)
		} else if defOut.Found {
			earlyPipelineDef = &defOut
			stageConfigMap = buildStageConfigMap(earlyPipelineDef)
			stageConfigSlice = buildStageConfigOrdered(earlyPipelineDef)
			logger.Info("Early pipeline definition loaded for triage prompt overrides",
				"pipeline", earlyPipelineName,
				"stage_count", len(defOut.Stages),
			)
		}
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
	triagePhaseID := sideEffectUUID(ctx)

	var triageOutput TriageOutput
	triageOpts := stageOpts("triage", llmOpts)
	if input.TimeoutOverride > 0 {
		triageOpts.StartToCloseTimeout = input.TimeoutOverride
	}
	ctxTriage := workflow.WithActivityOptions(ctx, triageOpts)
	err := workflow.ExecuteActivity(ctxTriage, pkgtemporal.ActivityTriage, TriageInput{
		TenantID:        input.TenantID,
		SourceID:        input.SourceID,
		ContentID:       input.ContentID,
		JobID:           input.JobID,
		Content:         parsedContent,
		Subject:         input.Subject,
		SenderEmail:     input.SenderEmail,
		ContentType:     input.ContentType,
		Headers:         fetchedHeaders,
		ModelOverride:   input.ModelOverride,
		PromptOverride:  promptOverrideForStage(stageConfigMap, "triage"),
		LangfuseTraceID: langfuseTraceID,
		LangfusePhaseID: triagePhaseID,
	}).Get(ctx, &triageOutput)
	if err != nil {
		// Update status to "rejected" with failure info
		pe := perrors.ClassifyError(err, "triage")
		logger.Warn("pipeline stage span error",
			"stage.name", "triage",
			"error.type", classifyTemporalError(err),
			"error.detail", err.Error(),
			"stage.duration_ms", workflow.Now(ctx).Sub(triageStart).Milliseconds(),
		)
		logger.Info("Triage failed, marking as rejected",
			"error", err,
			"error_code", pe.Code,
		)
		ctxTriageUpdate := workflow.WithActivityOptions(ctx, fastOpts)
		if err := workflow.ExecuteActivity(ctxTriageUpdate, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
			TenantID:        input.TenantID,
			SourceID:        input.SourceID,
			Status:          "rejected",
			FailureCategory: string(pe.Code),
			FailureReason:   err.Error(),
		}).Get(ctx, nil); err != nil {
			logger.Error("Failed to update content status",
				"source_id", input.SourceID,
				"target_status", "rejected",
				"error", err,
			)
		}
		state.result.Status = "rejected"
		state.result.Error = fmt.Sprintf("%s: %s", pe.Code, err.Error())
		state.status.ErrorMessage = state.result.Error

		// Auto-drain: kick next pending item even on triage rejection to maintain concurrency window
		kickCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 15 * time.Second,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:    time.Second,
				BackoffCoefficient: 2.0,
				MaximumAttempts:    2,
			},
		})
		var kickOutput KickNextPendingOutput
		kickErr := workflow.ExecuteActivity(kickCtx, pkgtemporal.ActivityKickNextPending, KickNextPendingInput{
			TenantID: input.TenantID,
			Limit:    0, // limit read from pipeline.kick_next_limit config by the activity
		}).Get(kickCtx, &kickOutput)
		if kickErr != nil {
			logger.Warn("Auto-drain kick failed after triage rejection (non-blocking)",
				"source_id", input.SourceID,
				"error", kickErr,
			)
		} else {
			logger.Info("Auto-drain kick completed after triage rejection",
				"source_id", input.SourceID,
				"queued_count", kickOutput.QueuedCount,
			)
		}

		return state.result, nil
	}

	logger.Info("pipeline stage span completed",
		"stage.name", "triage",
		"stage.duration_ms", workflow.Now(ctx).Sub(triageStart).Milliseconds(),
		"stage.timeout_start_to_close_ms", triageOpts.StartToCloseTimeout.Milliseconds(),
	)
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

	// Langfuse: report Triage phase span (best-effort, non-blocking).
	// Generation is now reported by the AI coordinator via gRPC metadata.
	triageEnd := workflow.Now(ctx)
	_ = workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, fastOpts),
		pkgtemporal.ActivityReportLangfusePhase,
		ReportLangfusePhaseInput{
			PhaseID:      triagePhaseID,
			TraceID:      langfuseTraceID,
			PhaseName:    "Triage",
			StartTime:    triageStart,
			EndTime:      triageEnd,
			ParentSpanID: rootSpanID,
		},
	).Get(ctx, nil)

	state.result.Category = triageOutput.Category
	state.result.Importance = triageOutput.Importance
	state.result.SkipDeep = triageOutput.SkipDeep
	state.result.ModelUsed = triageOutput.ModelUsed
	state.result.ContentContribution = triageOutput.ContentContribution
	state.result.ContributionReason = triageOutput.ContributionReason

	// Override contribution gating for forwarded emails (pf-100e09).
	// The 500-char triage truncation means the LLM only sees "FYI" + forwarded
	// email headers, causing it to misclassify as NONE. The actual forwarded
	// content (selected as CleanBody above) is substantial.
	{
		subjectLower := strings.ToLower(input.Subject)
		isForward := strings.HasPrefix(subjectLower, "fw:") ||
			strings.HasPrefix(subjectLower, "fwd:")
		if isForward && triageOutput.ContentContribution == "NONE" && len(parsedContent) > 500 {
			triageOutput.ContentContribution = "MEDIUM"
			triageOutput.ContributionReason = "Forward email override: triage truncation hides forwarded content"
			state.result.ContentContribution = triageOutput.ContentContribution
			state.result.ContributionReason = triageOutput.ContributionReason
			logger.Info("Forward email contribution override",
				"source_id", input.SourceID,
				"original_contribution", "NONE",
				"overridden_to", "MEDIUM",
				"content_length", len(parsedContent),
			)
		}
	}

	state.status.StepsCompleted = 2

	if checkCancellation() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		return state.result, nil
	}

	// ==================== Pipeline Routing ====================
	// If routing was resolved (Pipelines field populated), check if any pipelines match.
	// No matched pipelines = item is skipped entirely (no further processing).
	if triageOutput.Pipelines != nil && len(triageOutput.Pipelines) == 0 {
		logger.Info("Pipeline routing: no pipelines matched, skipping all stages",
			"source_id", input.SourceID,
			"routing_content_type", triageOutput.RoutingContentType,
			"routing_subtype", triageOutput.RoutingSubtype,
		)

		// Record all stages as skipped
		skipReason := fmt.Sprintf("routing:no_pipeline:%s/%s", triageOutput.RoutingContentType, triageOutput.RoutingSubtype)
		ctxSkip := workflow.WithActivityOptions(ctx, fastOpts)
		_ = workflow.ExecuteActivity(ctxSkip, pkgtemporal.ActivityRecordSkippedStage, RecordSkippedStageInput{
			SourceID: input.SourceID,
			Stages: []SkippedStage{
				{Stage: "summarize", SkipReason: skipReason},
				{Stage: "extract_ner", SkipReason: skipReason},
				{Stage: "extract_semantic", SkipReason: skipReason},
				{Stage: "resolve", SkipReason: skipReason},
				{Stage: "analyze", SkipReason: skipReason},
				{Stage: "persist", SkipReason: skipReason},
				{Stage: "embed", SkipReason: skipReason},
			},
			LangfuseTraceID: langfuseTraceID,
		}).Get(ctx, nil)

		reportLangfuseSkip("PipelineSkip", skipReason)

		// Mark as completed (processed with no pipeline)
		ctxStatus := workflow.WithActivityOptions(ctx, fastOpts)
		_ = workflow.ExecuteActivity(ctxStatus, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			Status:   "completed",
		}).Get(ctx, nil)

		state.result.Status = "completed"
		state.result.SkipDeep = true

		// Langfuse: finish trace (close root span + flush)
		_ = workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityFinishLangfuseTrace,
			FinishLangfuseTraceInput{
				TraceID:    langfuseTraceID,
				RootSpanID: rootSpanID,
			},
		).Get(ctx, nil)

		return state.result, nil
	}
	// Log routing decision for items that enter pipelines
	if len(triageOutput.Pipelines) > 0 {
		logger.Info("Pipeline routing: item enters pipelines",
			"source_id", input.SourceID,
			"pipelines", triageOutput.Pipelines,
			"routing_content_type", triageOutput.RoutingContentType,
			"routing_subtype", triageOutput.RoutingSubtype,
		)
	}

	// ==================== Pipeline Definition Lookup ====================
	// Determine the pipeline name and fetch its definition from the database.
	// If not provided in input, use the first routed pipeline (or "standard" as default).
	// A missing or failed definition causes the workflow to fail — no silent fallback.
	pipelineName := ""
	if len(triageOutput.Pipelines) > 0 {
		pipelineName = triageOutput.Pipelines[0]
	} else if input.Pipeline != "" {
		pipelineName = input.Pipeline
	}
	if pipelineName == "" {
		pipelineName = "standard"
	}

	// Update the Langfuse trace name now that the pipeline is resolved.
	// The initial trace was created before triage with the input.Pipeline value
	// (often empty → "email-processing"). If triage routed to a different pipeline,
	// update the trace name to reflect the actual pipeline.
	resolvedTraceName := pipelineTraceName(pipelineName)
	langfuseTraceTags = append(langfuseTraceTags, "pipeline:"+pipelineName)
	if langfuseErr == nil && resolvedTraceName != pipelineTraceName(input.Pipeline) {
		_ = workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityUpdateLangfuseTraceTags,
			UpdateLangfuseTraceTagsInput{
				TraceID: langfuseTraceID,
				Tags:    langfuseTraceTags,
				Name:    resolvedTraceName,
			},
		).Get(ctx, nil)
	}

	// Fetch pipeline definition if not already loaded (or pipeline changed after triage routing).
	// When the early fetch loaded the definition for the same pipeline, reuse it to avoid a duplicate DB call.
	var pipelineDef *FetchPipelineDefinitionOutput
	if pipelineName == earlyPipelineName && earlyPipelineDef != nil {
		// Pipeline didn't change — reuse the early fetch result.
		pipelineDef = earlyPipelineDef
		logger.Info("Pipeline definition reused from early fetch",
			"pipeline", pipelineName,
		)
	} else {
		ctxDef := workflow.WithActivityOptions(ctx, fastOpts)
		var defOut FetchPipelineDefinitionOutput
		defErr := workflow.ExecuteActivity(ctxDef, pkgtemporal.ActivityFetchPipelineDefinition, FetchPipelineDefinitionInput{
			TenantID: input.TenantID,
			Pipeline: pipelineName,
		}).Get(ctx, &defOut)
		if defErr != nil {
			logger.Error("Failed to fetch pipeline definition — failing workflow",
				"pipeline", pipelineName,
				"error", defErr,
			)
			state.result.Status = "failed"
			state.result.Error = fmt.Sprintf("pipeline definition not found: %v", defErr)
			state.status.ErrorMessage = state.result.Error
			ctxFail := workflow.WithActivityOptions(ctx, fastOpts)
			if err := workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
				TenantID: input.TenantID, SourceID: input.SourceID,
				Status: "failed", FailureCategory: "configuration_error", FailureReason: defErr.Error(),
			}).Get(ctx, nil); err != nil {
				logger.Error("Failed to update content status after pipeline definition error",
					"source_id", input.SourceID,
					"error", err,
				)
			}
			if langfuseTraceID != "" {
				_ = workflow.ExecuteActivity(
					workflow.WithActivityOptions(ctx, fastOpts),
					pkgtemporal.ActivityUpdateLangfuseTraceMetadata,
					UpdateLangfuseTraceMetadataInput{
						TraceID: langfuseTraceID,
						Metadata: map[string]any{
							"error":            defErr.Error(),
							"failure_category": "configuration_error",
							"pipeline":         pipelineName,
						},
					},
				).Get(ctx, nil)
			}
			return state.result, nil
		} else if defOut.Found {
			pipelineDef = &defOut
			logger.Info("Pipeline definition loaded from DB",
				"pipeline", pipelineName,
				"stage_count", len(defOut.Stages),
			)
		} else {
			logger.Info("No pipeline definition found in DB",
				"pipeline", pipelineName,
			)
		}
		if !defOut.Found {
			return state.result, temporal.NewNonRetryableApplicationError(
				fmt.Sprintf("pipeline definition not found: pipeline=%s tenant=%s", pipelineName, input.TenantID),
				"configuration_error",
				nil,
			)
		}
		pipelineDef = &defOut
		logger.Info("Pipeline definition loaded from DB",
			"pipeline", pipelineName,
			"stage_count", len(defOut.Stages),
		)
		// Rebuild stageConfigMap and companion slice with the resolved pipeline definition.
		stageConfigMap = buildStageConfigMap(pipelineDef)
		stageConfigSlice = buildStageConfigOrdered(pipelineDef)
	}

	// Fail if the definition has no enabled stages — a misconfigured pipeline
	// would silently produce no output otherwise.
	enabledStageCount := 0
	for _, s := range pipelineDef.Stages {
		if s.Enabled {
			enabledStageCount++
		}
	}
	if enabledStageCount == 0 {
		return state.result, temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("Pipeline %s has no enabled stages", pipelineName),
			"configuration_error",
			nil,
		)
	}

	// Update total steps now that the pipeline definition is loaded.
	state.status.TotalSteps = enabledStageCount

	// Enrich Langfuse trace with pipeline definition metadata (best-effort).
	if pipelineDef != nil && langfuseTraceID != "" {
		metadata := buildPipelineDefinitionMetadata(pipelineName, pipelineDef)
		_ = workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityUpdateLangfuseTraceMetadata,
			UpdateLangfuseTraceMetadataInput{
				TraceID:  langfuseTraceID,
				Metadata: metadata,
			},
		).Get(ctx, nil)
	}

	// Compute content contribution for gating (used by summarize + extract/analyze gates).
	contribution := triageOutput.ContentContribution
	// Default to MEDIUM if field is missing — a missing value should not trigger
	// the most expensive processing path. MEDIUM is a safe middle ground that runs
	// extraction but skips nothing, without silently enabling full HIGH processing.
	if contribution == "" {
		contribution = "MEDIUM"
	}

	// ==================== Stage 1.5: Summarize (non-blocking) ====================
	// Generate a concise summary for the content. Failure does NOT block the pipeline.
	// Gate: skip if content too brief, contribution is NONE, or stage disabled (pf-c42209).
	skipSummarize := false
	var summarizeSkipReason string

	if !stageInPipeline(stageConfigMap, "summarize") {
		skipSummarize = true
		summarizeSkipReason = "stage_not_in_pipeline"
	}
	if !skipSummarize && contribution == "NONE" {
		skipSummarize = true
		summarizeSkipReason = fmt.Sprintf("contribution_gating:%s", contribution)
	}
	if !skipSummarize {
		wordCount := len(strings.Fields(parsedContent))
		if wordCount < 20 {
			skipSummarize = true
			summarizeSkipReason = fmt.Sprintf("insufficient_content:%d_words", wordCount)
		}
	}

	if skipSummarize {
		logger.Info("Summarize stage skipped",
			"source_id", input.SourceID,
			"reason", summarizeSkipReason,
		)
		if stageInPipeline(stageConfigMap, "summarize") {
			ctxSkip := workflow.WithActivityOptions(ctx, fastOpts)
			_ = workflow.ExecuteActivity(ctxSkip, pkgtemporal.ActivityRecordSkippedStage, RecordSkippedStageInput{
				SourceID:        input.SourceID,
				Stages:          []SkippedStage{{Stage: "summarize", SkipReason: summarizeSkipReason}},
				LangfuseTraceID: langfuseTraceID,
			}).Get(ctx, nil)
		}
		reportLangfuseSkip("Summarize", summarizeSkipReason)
	} else {
		summarizeStart := workflow.Now(ctx)
		summarizePhaseID := sideEffectUUID(ctx)
		summarizeOpts := stageOpts("summarize", llmOpts)
		ctxSummarize := workflow.WithActivityOptions(ctx, summarizeOpts)
		var summaryID int64
		summarizeErr := workflow.ExecuteActivity(ctxSummarize, pkgtemporal.ActivityGenerateContentSummary, GenerateSummaryInput{
			TenantID:        input.TenantID,
			SourceID:        input.SourceID,
			ContentID:       input.ContentID,
			JobID:           input.JobID,
			Content:         parsedContent,
			PromptOverride:  promptOverrideForStage(stageConfigMap, "summarize"),
			LangfuseTraceID: langfuseTraceID,
			LangfusePhaseID: summarizePhaseID,
		}).Get(ctx, &summaryID)
		if summarizeErr != nil {
			logger.Error("Stage 1.5 GenerateSummary failed (non-blocking)", "error", summarizeErr)
		} else {
			logger.Debug("Summary generated", "summary_id", summaryID)
		}
		// Langfuse: report Summarize phase span (best-effort, non-blocking).
		// Generation is reported by the AI coordinator via gRPC metadata.
		summarizeEnd := workflow.Now(ctx)
		summarizePhaseInput := ReportLangfusePhaseInput{
			PhaseID:      summarizePhaseID,
			TraceID:      langfuseTraceID,
			PhaseName:    "Summarize",
			StartTime:    summarizeStart,
			EndTime:      summarizeEnd,
			ParentSpanID: rootSpanID,
		}
		if summarizeErr != nil {
			summarizePhaseInput.Level = "ERROR"
			summarizePhaseInput.StatusMessage = summarizeErr.Error()
		}
		_ = workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityReportLangfusePhase,
			summarizePhaseInput,
		).Get(ctx, nil)
	}

	// Per-stage contribution gating: each stage's skip decision reads skip_when_low from
	// pipeline_definitions. A stage is skipped when skip_when_low=true AND the content has
	// low contribution (NONE or LOW) OR triage marked it for reduced processing (SkipDeep).
	// This replaces the former global skipExtract/skipAnalyze switch statement.
	shouldGateStage := func(stageName string) bool {
		cfg, ok := stageConfigMap[stageName]
		if !ok || !cfg.Enabled {
			return false
		}
		if !cfg.SkipWhenLow {
			return false
		}
		return contribution == "NONE" || contribution == "LOW" || triageOutput.SkipDeep
	}

	// skipExtract: skip the extraction block when all primary extract stages are gated.
	// If extract_ner or extract_semantic has skip_when_low=false, the block runs.
	extractShouldRun := (stageInPipeline(stageConfigMap, "extract_ner") && !shouldGateStage("extract_ner")) ||
		(stageInPipeline(stageConfigMap, "extract_semantic") && !shouldGateStage("extract_semantic"))
	skipExtract := !extractShouldRun

	// skipAnalyze: skip the analyze block when the analyze stage is gated.
	skipAnalyze := shouldGateStage("analyze")

	// When any stages are gated: log each skip, record provenance, clean up stale data,
	// and update TotalSteps to reflect only the stages that will actually run.
	if skipExtract || skipAnalyze {
		skipReason := fmt.Sprintf("contribution_gating:%s", contribution)
		if triageOutput.SkipDeep {
			skipReason = fmt.Sprintf("category_skip:%s/%s", triageOutput.Category, triageOutput.Importance)
		}

		var skippedStages []SkippedStage
		for _, sc := range orderedStages(stageConfigSlice) {
			if shouldGateStage(sc.Stage) {
				logger.Info("pipeline stage skipped",
					"source_id", input.SourceID,
					"stage", sc.Stage,
					"stage_order", sc.StageOrder,
					"pipeline", pipelineName,
					"reason", skipReason,
					"contribution", contribution,
				)
				skippedStages = append(skippedStages, SkippedStage{Stage: sc.Stage, SkipReason: skipReason})
			}
		}

		if len(skippedStages) > 0 {
			ctxSkip := workflow.WithActivityOptions(ctx, fastOpts)
			_ = workflow.ExecuteActivity(ctxSkip, pkgtemporal.ActivityRecordSkippedStage, RecordSkippedStageInput{
				SourceID:        input.SourceID,
				Stages:          skippedStages,
				LangfuseTraceID: langfuseTraceID,
			}).Get(ctx, nil)
			reportLangfuseSkip("ContributionGatingSkip", skipReason)
		}

		// pf-91b00d: When extraction is skipped entirely, delete any assertions left over
		// from a prior run. Ensures reprocessing a reclassified item clears stale data.
		// Best-effort — failure does not block the workflow.
		if skipExtract {
			ctxCleanup := workflow.WithActivityOptions(ctx, fastOpts)
			_ = workflow.ExecuteActivity(ctxCleanup, pkgtemporal.ActivityDeleteAssertions, DeleteAssertionsInput{
				TenantID: input.TenantID,
				SourceID: input.SourceID,
			}).Get(ctx, nil)
		}

		// Update TotalSteps to count only stages that will run.
		runningSteps := 0
		for _, sc := range stageConfigMap {
			if sc.Enabled && !shouldGateStage(sc.Stage) {
				runningSteps++
			}
		}
		state.status.TotalSteps = runningSteps
	}

	// Source system now classified in Triage activity via rule engine.
	// Fall back to human_email if the field is empty (e.g. early return paths).
	sourceSystem := enrichment.SourceSystem(triageOutput.SourceSystem)
	if sourceSystem == "" {
		sourceSystem = enrichment.SourceSystemHumanEmail
	}
	// pf-e494df: For non-email content (meeting, calendar, etc.) the triage rule engine
	// has no matching rules and defaults to human_email. Restore the original source_system
	// from the DB when we have it.
	if input.ContentType != "email" && originalSourceSystem != "" {
		sourceSystem = enrichment.SourceSystem(originalSourceSystem)
	}
	// pf-83a646: Ensure meeting content has a meaningful source tag.
	// If no specific source system was stored (e.g. "teams", "zoom"), use "meeting".
	if input.ContentType == "meeting" && sourceSystem == enrichment.SourceSystemHumanEmail {
		sourceSystem = enrichment.SourceSystem("meeting")
	}

	// Persist triage results and source_system to source metadata (fires for all items)
	skipDeep := triageOutput.SkipDeep
	ctxTriageMeta := workflow.WithActivityOptions(ctx, fastOpts)
	if err := workflow.ExecuteActivity(ctxTriageMeta, "UpdateContentStatus", UpdateContentStatusInput{
		TenantID:         input.TenantID,
		SourceID:         input.SourceID,
		Status:           "parsed",
		TriageCategory:   triageOutput.Category,
		TriageImportance: triageOutput.Importance,
		SkipDeep:         &skipDeep,
		ContentSubtype:   triageOutput.ContentSubtype,
		SourceSystem:     string(sourceSystem),
	}).Get(ctx, nil); err != nil {
		logger.Error("Failed to update content status",
			"source_id", input.SourceID,
			"target_status", "parsed",
			"error", err,
		)
	}

	// Add source_system tag to Langfuse trace now that triage has classified it (pf-100b37).
	langfuseTraceTags = append(langfuseTraceTags, "src:"+string(sourceSystem))
	if langfuseErr == nil {
		_ = workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityUpdateLangfuseTraceTags,
			UpdateLangfuseTraceTagsInput{
				TraceID: langfuseTraceID,
				Tags:    langfuseTraceTags,
			},
		).Get(ctx, nil)
	}

	// Create enrichment record (runs for all items after triage)
	// Map input ContentType to enrichment.ContentType
	var enrichmentContentType enrichment.ContentType
	switch input.ContentType {
	case "email":
		enrichmentContentType = enrichment.ContentTypeEmail
	case "calendar":
		enrichmentContentType = enrichment.ContentTypeCalendar
	case "meeting":
		// pf-e494df: meeting is distinct from calendar; use ContentTypeMeeting
		enrichmentContentType = enrichment.ContentTypeMeeting
	case "document":
		enrichmentContentType = enrichment.ContentTypeDocument
	case "attachment":
		enrichmentContentType = enrichment.ContentTypeAttachment
	default:
		// Default to email for unknown types
		enrichmentContentType = enrichment.ContentTypeEmail
	}

	// Map ContentSubtype string to enrichment.ContentSubtype
	var enrichmentSubtype enrichment.ContentSubtype
	if triageOutput.ContentSubtype != "" {
		enrichmentSubtype = enrichment.ContentSubtype(triageOutput.ContentSubtype)
	} else {
		// Default subtype based on content type
		switch enrichmentContentType {
		case enrichment.ContentTypeEmail:
			enrichmentSubtype = enrichment.SubtypeEmailStandalone
		case enrichment.ContentTypeCalendar:
			enrichmentSubtype = enrichment.SubtypeCalendarInvite
		case enrichment.ContentTypeMeeting:
			// pf-e494df: default subtype for meeting content is transcript
			enrichmentSubtype = enrichment.SubtypeMeetingTranscript
		default:
			enrichmentSubtype = enrichment.ContentSubtype("unknown")
		}
	}

	// Map SkipDeep to ProcessingProfile
	var processingProfile enrichment.ProcessingProfile
	if triageOutput.SkipDeep {
		processingProfile = enrichment.ProfileMetadataOnly
	} else {
		processingProfile = enrichment.ProfileFullAI
	}

	// pf-43acf2: Classify content structure (standalone, reply, forward).
	// Subject-based detection (FW:/Fwd: → forward, Re: → reply) covers the majority of cases.
	// In-Reply-To header detection is limited here because raw email headers are not currently
	// propagated through the pipeline input; thread membership via email_threads provides
	// the REPLY signal in that case.
	contentStructure := enrichment.ClassifyContentStructure(nil, input.Subject, input.ContentType)

	ctxEnrichment := workflow.WithActivityOptions(ctx, fastOpts)
	var enrichmentOutput CreateEnrichmentRecordOutput
	err = workflow.ExecuteActivity(ctxEnrichment, pkgtemporal.ActivityCreateEnrichmentRecord, CreateEnrichmentRecordInput{
		SourceID:                 input.SourceID,
		TenantID:                 input.TenantID,
		ContentType:              enrichmentContentType,
		ContentSubtype:           enrichmentSubtype,
		SourceSystem:             sourceSystem,
		ProcessingProfile:        processingProfile,
		ClassificationConfidence: triageOutput.Confidence,
		ClassificationReason:     triageOutput.Reason,
		ContentStructure:         contentStructure,
	}).Get(ctx, &enrichmentOutput)

	if err != nil {
		return nil, fmt.Errorf("failed to create enrichment record: %w", err)
	}

	logger.Info("enrichment record created",
		"source_id", input.SourceID,
		"enrichment_id", enrichmentOutput.EnrichmentID,
	)

	// ==================== Stage 1.5: Header Mention Extraction ====================
	// Extract From/To/CC from email headers into content_mentions with participation roles.
	// Runs for ALL emails (independent of SkipDeep gate). Deterministic — no LLM needed.
	if input.ContentType == "email" && (input.SenderEmail != "" || len(input.ParticipantEmails) > 0) {
		var headerMentionsOutput ExtractHeaderMentionsOutput
		ctxHeaderMentions := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctxHeaderMentions, pkgtemporal.ActivityExtractHeaderMentions, ExtractHeaderMentionsInput{
			TenantID:     input.TenantID,
			SourceID:     input.SourceID,
			SenderEmail:  input.SenderEmail,
			SenderName:   input.SenderName,
			Participants: input.ParticipantEmails,
		}).Get(ctx, &headerMentionsOutput)
		if err != nil {
			logger.Warn("header mention extraction failed (non-blocking)",
				"source_id", input.SourceID,
				"error", err.Error(),
			)
		} else {
			logger.Info("header mention extraction completed",
				"source_id", input.SourceID,
				"mentions_created", headerMentionsOutput.MentionsCreated,
				"from_mentions", headerMentionsOutput.FromMentions,
				"to_mentions", headerMentionsOutput.ToMentions,
				"cc_mentions", headerMentionsOutput.CcMentions,
				"group_expanded", headerMentionsOutput.GroupExpanded,
			)
		}
	}

	// ==================== Stage 2.5: Email Threading ====================
	// Threading runs for ALL emails (independent of SkipDeep gate)
	// Reads email headers directly from source metadata — does not depend on extraction output
	var threadID *string
	var emailThreadID *int64 // Numeric email_threads.id for assertion dedup
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
			emailThreadID = threadOutput.EmailThreadID
			logger.Info("email threading completed",
				"source_id", input.SourceID,
				"thread_id", *threadID,
			)
		}
	}

	// ==================== Stage 2.6: Conversation Auto-Linking ====================
	// After threading, auto-link conversation if a thread_id exists
	// Runs for ALL emails (independent of SkipDeep gate)
	convOutput := &LinkConversationOutput{}
	if threadID != nil && input.ContentID != "" {
		logger.Debug("starting conversation linking",
			"source_id", input.SourceID,
			"thread_id", *threadID,
			"content_id", input.ContentID,
		)
		ctxConv := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctxConv, pkgtemporal.ActivityLinkConversation, LinkConversationInput{
			TenantID:  input.TenantID,
			SourceID:  input.SourceID,
			ThreadID:  *threadID,
			ContentID: input.ContentID,
		}).Get(ctx, convOutput)
		if err != nil {
			logger.Warn("conversation linking failed (non-blocking)",
				"source_id", input.SourceID,
				"thread_id", *threadID,
				"error", err.Error(),
			)
		} else if convOutput != nil && convOutput.ConversationID != "" {
			logger.Info("conversation linking completed",
				"source_id", input.SourceID,
				"thread_id", *threadID,
				"conversation_id", convOutput.ConversationID,
			)

			// Update Langfuse trace tags with conversation_id (best-effort, non-blocking).
			langfuseTraceTags = append(langfuseTraceTags, "conv:"+convOutput.ConversationID)
			_ = workflow.ExecuteActivity(
				workflow.WithActivityOptions(ctx, fastOpts),
				pkgtemporal.ActivityUpdateLangfuseTraceTags,
				UpdateLangfuseTraceTagsInput{
					TraceID: langfuseTraceID,
					Tags:    langfuseTraceTags,
				},
			).Get(ctx, nil)
		}
	}

	// ==================== Stages 2-4.5: Deep Processing ====================
	var extractOutput *SLMPipelineExtractEntitiesOutput
	var contextOutput *BuildContextOutput

	// Pre-extraction context: fetch glossary + topics for extraction grounding (pf-2f8c70).
	// Runs before Stage 2 (NER/semantic) and any structured_extract stages.
	var extractionContext string
	hasActiveStructuredExtract := false
	for _, sc := range stageConfigSlice {
		if sc.StageKind == "structured_extract" && sc.Enabled && !shouldGateStage(sc.Stage) {
			hasActiveStructuredExtract = true
			break
		}
	}
	if !skipExtract || hasActiveStructuredExtract {
		ctxPreExtract := workflow.WithActivityOptions(ctx, fastOpts)
		var preCtxOutput BuildExtractionContextOutput
		err := workflow.ExecuteActivity(ctxPreExtract, pkgtemporal.ActivityBuildExtractionContext, BuildExtractionContextInput{
			TenantID: input.TenantID,
			Subject:  input.Subject,
			Content:  parsedContent,
		}).Get(ctx, &preCtxOutput)
		if err != nil {
			logger.Warn("pre-extraction context build failed (non-blocking)", "error", err.Error())
		} else {
			extractionContext = preCtxOutput.BackgroundContext
		}
	}

	if !skipExtract && (stageInPipeline(stageConfigMap, "extract_ner") || stageInPipeline(stageConfigMap, "extract_semantic")) {
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
		extractPhaseID := sideEffectUUID(ctx)

		extractOutput = &SLMPipelineExtractEntitiesOutput{}
		extractOpts := stageOpts("extract_ner", llmOpts)
		if input.TimeoutOverride > 0 {
			extractOpts.StartToCloseTimeout = input.TimeoutOverride
		}
		ctxExtract := workflow.WithActivityOptions(ctx, extractOpts)
		err = workflow.ExecuteActivity(ctxExtract, pkgtemporal.ActivityExtractEntitiesActivity, SLMPipelineExtractEntitiesInput{
			TenantID:               input.TenantID,
			SourceID:               input.SourceID,
			ContentID:              input.ContentID,
			JobID:                  input.JobID,
			Content:                parsedContent,
			ModelOverride:          input.ModelOverride,
			NERPromptOverride:      promptOverrideForStage(stageConfigMap, "extract_ner"),
			SemanticPromptOverride: promptOverrideForStage(stageConfigMap, "extract_semantic"),
			ContentType:            input.ContentType,
			Subject:                input.Subject,
			SenderName:             input.SenderName,
			SenderEmail:            input.SenderEmail,
			Participants:           input.ParticipantEmails,
			BackgroundContext:      extractionContext,
			PipelineStages:         stageConfigMapKeys(stageConfigMap),
			LangfuseTraceID:        langfuseTraceID,
			LangfusePhaseID:        extractPhaseID,
		}).Get(ctx, extractOutput)

		// Stage 2b: Extract Assertions (failure does NOT block pipeline)
		var assertionCount int
		if stageInPipeline(stageConfigMap, "extract_assertions") {
			assertionOpts := stageOpts("extract_assertions", llmOpts)
			ctxAssertions := workflow.WithActivityOptions(ctx, assertionOpts)
			err2 := workflow.ExecuteActivity(ctxAssertions, pkgtemporal.ActivityExtractAssertions, ExtractAssertionsInput{
				TenantID:          input.TenantID,
				SourceID:          input.SourceID,
				ContentID:         input.ContentID,
				JobID:             input.JobID,
				Content:           parsedContent,
				Subject:           input.Subject,          // Topic framing for assertion extraction (pf-e219c1)
				ContentType:       input.ContentType,
				SenderEmail:       input.SenderEmail,      // Pass sender for owner attribution
				BackgroundContext: extractionContext,       // Glossary + topics for reference resolution (pf-90b749)
				QuotedContent:     quotedContent,           // Parent email for forward-reference resolution (pf-90b749)
				LangfuseTraceID:   langfuseTraceID,
				LangfusePhaseID:   extractPhaseID,
			}).Get(ctx, &assertionCount)
			if err2 != nil {
				logger.Error("pipeline stage span error",
					"stage.name", "extract_assertions",
					"error.type", classifyTemporalError(err2),
					"error.detail", err2.Error(),
				)
				logger.Error("Stage 2b ExtractAssertions failed, continuing", "error", err2)
				assertionCount = 0
			} else {
				logger.Info("pipeline stage span completed",
					"stage.name", "extract_assertions",
					"stage.timeout_start_to_close_ms", assertionOpts.StartToCloseTimeout.Milliseconds(),
				)
			}
		}
		state.result.AssertionsCreated = assertionCount

		if err != nil {
			logger.Warn("pipeline stage span error",
				"stage.name", "extract_ner",
				"error.type", classifyTemporalError(err),
				"error.detail", err.Error(),
				"stage.duration_ms", workflow.Now(ctx).Sub(extractStart).Milliseconds(),
			)
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
			logger.Info("pipeline stage span completed",
				"stage.name", "extract_ner",
				"stage.duration_ms", workflow.Now(ctx).Sub(extractStart).Milliseconds(),
				"stage.timeout_start_to_close_ms", extractOpts.StartToCloseTimeout.Milliseconds(),
			)
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

		// Langfuse: report Extract phase span (covers both entity and assertion extraction).
		// One phase span groups both sub-activities under a single "Extract" phase.
		// Generation is now reported by the AI coordinator via gRPC metadata.
		extractPhaseEnd := workflow.Now(ctx)
		_ = workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityReportLangfusePhase,
			ReportLangfusePhaseInput{
				PhaseID:   extractPhaseID,
				TraceID:   langfuseTraceID,
				PhaseName: "Extract",
				StartTime: extractStart,
				EndTime:      extractPhaseEnd,
				ParentSpanID: rootSpanID,
			},
		).Get(ctx, nil)

		state.status.StepsCompleted = 3

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
		if err := workflow.ExecuteActivity(ctxStatus2, pkgtemporal.ActivityUpdateContentStatus, updateInput).Get(ctx, nil); err != nil {
			logger.Error("Failed to update content status",
				"source_id", input.SourceID,
				"target_status", "extracted",
				"error", err,
			)
		}

		if checkCancellation() {
			state.result.Status = "cancelled"
			state.result.Error = state.cancelReason
			return state.result, nil
		}

		// Stage 3: Context
		if stageInPipeline(stageConfigMap, "resolve") {
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

			// Extract conversation ID for context scoping (pf-26c835).
			convID := ""
			if convOutput != nil && convOutput.ConversationID != "" {
				convID = convOutput.ConversationID
			}

			err = workflow.ExecuteActivity(ctxContext, pkgtemporal.ActivityBuildContextPackage, BuildContextInput{
				TenantID:          input.TenantID,
				SourceID:          input.SourceID,
				ContentID:         input.ContentID,
				JobID:             input.JobID,
				ContentType:       input.ContentType,
				ContentSubtype:    triageOutput.ContentSubtype,
				Extraction:        extractOutput,
				SenderEmail:       input.SenderEmail,
				SenderName:        input.SenderName,
				Subject:           input.Subject,
				ParticipantEmails: input.ParticipantEmails,
				ConversationID:    convID,
				Content:           parsedContent,
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

			// Stage 3.6: Entity Enrichment (enrich_entities — optional, non-blocking)
			// Computes communication patterns and infers expertise areas for resolved entities.
			if stageInPipeline(stageConfigMap, "enrich_entities") && contextOutput != nil && len(contextOutput.ResolvedPeople) > 0 {
				enrichEntitiesCtx := workflow.WithActivityOptions(ctx, stageOpts("enrich_entities", fastOpts))
				enrichEntitiesOutput := &EnrichEntitiesOutput{}

				err = workflow.ExecuteActivity(enrichEntitiesCtx, pkgtemporal.ActivityEnrichEntities, EnrichEntitiesInput{
					TenantID:          input.TenantID,
					SourceID:          input.SourceID,
					ContentID:         input.ContentID,
					Content:           parsedContent,
					ResolvedPeople:    contextOutput.ResolvedPeople,
					ParticipantEmails: input.ParticipantEmails,
				}).Get(ctx, enrichEntitiesOutput)
				if err != nil {
					logger.Warn("entity enrichment failed (non-blocking)",
						"source_id", input.SourceID,
						"error", err.Error(),
					)
				} else {
					logger.Info("entity enrichment completed",
						"source_id", input.SourceID,
						"patterns_updated", enrichEntitiesOutput.PatternsUpdated,
						"expertise_updated", enrichEntitiesOutput.ExpertiseUpdated,
					)
				}
			}
		} // End of resolve stage block (stages 3-3.6)
	} // End of skipExtract block (stages 2-3.6)

	// Stages 2.5+: Structured Extract (generic loop)
	// Runs for any pipeline stage where stage_kind == "structured_extract".
	// Replaces bespoke newsletter_extract and notification_extract dispatch.
	if stageConfigSlice != nil {
		for _, stage := range orderedStages(stageConfigSlice) {
			if stage.StageKind != "structured_extract" || !stage.Enabled {
				continue
			}
			if shouldGateStage(stage.Stage) {
				skipReason := fmt.Sprintf("contribution_gating:%s", contribution)
				if triageOutput.SkipDeep {
					skipReason = fmt.Sprintf("category_skip:%s/%s", triageOutput.Category, triageOutput.Importance)
				}
				logger.Info("pipeline stage skipped",
					"source_id", input.SourceID,
					"stage", stage.Stage,
					"pipeline", pipelineName,
					"reason", skipReason,
					"contribution", contribution,
				)
				continue
			}

			seStart := workflow.Now(ctx)
			sePhaseID := sideEffectUUID(ctx)
			logger.Info("pipeline stage starting",
				"source_id", input.SourceID,
				"stage", stage.Stage,
				"stage_kind", "structured_extract",
				"persist_key", stage.PersistKey,
			)

			// For newsletter_extract stages, build an enriched context via BuildStageContext,
			// which reads config-driven providers from pipeline_definitions.
			// Fall back to the generic extractionContext on any error.
			bgContext := extractionContext
			ctxMeta := map[string]string{
				"stage_name": stage.Stage,
			}
			if stage.Stage == "newsletter_extract" {
				ctxNLCtx := workflow.WithActivityOptions(ctx, fastOpts)
				var stageCtx string
				nlErr := workflow.ExecuteActivity(ctxNLCtx, pkgtemporal.ActivityBuildStageContext, BuildStageContextInput{
					TenantID:    input.TenantID,
					Pipeline:    pipelineName,
					Stage:       stage.Stage,
					SourceID:    input.SourceID,
					ContentID:   input.ContentID,
					Subject:     input.Subject,
					Content:     parsedContent,
					SenderEmail: input.SenderEmail,
					SenderName:  input.SenderName,
				}).Get(ctx, &stageCtx)
				if nlErr != nil {
					logger.Warn("newsletter context build failed, falling back to generic context",
						"source_id", input.SourceID,
						"error", nlErr.Error(),
					)
				} else {
					bgContext = stageCtx
					ctxMeta["context_length"] = fmt.Sprintf("%d", len(stageCtx))
					logger.Info("stage context built via BuildStageContext",
						"source_id", input.SourceID,
						"pipeline", pipelineName,
						"stage", stage.Stage,
						"context_length", len(stageCtx),
					)
				}
			}
			ctxMeta["context_length"] = fmt.Sprintf("%d", len(bgContext))

			var seOutput StructuredExtractOutput
			seOpts := stageOpts(stage.Stage, llmOpts)
			ctxSE := workflow.WithActivityOptions(ctx, seOpts)

			seErr := workflow.ExecuteActivity(ctxSE, pkgtemporal.ActivityStructuredExtract, StructuredExtractInput{
				TenantID:          input.TenantID,
				SourceID:          input.SourceID,
				Content:           parsedContent,
				StageName:         stage.Stage,
				PromptOverride:    stage.PromptOverride,
				BackgroundContext: bgContext,
				LangfuseTraceID:  langfuseTraceID,
				LangfusePhaseID:  sePhaseID,
				ContextMeta:      ctxMeta,
			}).Get(ctx, &seOutput)

			if seErr != nil {
				logger.Warn("pipeline stage failed (non-blocking)",
					"source_id", input.SourceID,
					"stage", stage.Stage,
					"duration_ms", workflow.Now(ctx).Sub(seStart).Milliseconds(),
					"error", seErr.Error(),
				)
			} else {
				logger.Info("pipeline stage completed",
					"source_id", input.SourceID,
					"stage", stage.Stage,
					"duration_ms", workflow.Now(ctx).Sub(seStart).Milliseconds(),
					"model_used", seOutput.ModelUsed,
					"output_length", len(seOutput.RawJSON),
				)

				// Persist the structured JSON to content_enrichment.extracted_data[persist_key].
				persistKey := stage.PersistKey
				if persistKey == "" {
					// Fallback: strip _extract suffix for compatibility
					persistKey = strings.TrimSuffix(stage.Stage, "_extract")
				}
				ctxPersistJSON := workflow.WithActivityOptions(ctx, fastOpts)
				var persistJSONOutput PersistExtractedDataOutput
				persistJSONErr := workflow.ExecuteActivity(ctxPersistJSON, pkgtemporal.ActivityPersistExtractedData, PersistExtractedDataInput{
					TenantID: input.TenantID,
					SourceID: input.SourceID,
					Key:      persistKey,
					Data:     seOutput.RawJSON,
				}).Get(ctx, &persistJSONOutput)

				if persistJSONErr != nil {
					logger.Warn("persist extracted_data failed (non-blocking)",
						"source_id", input.SourceID,
						"key", persistKey,
						"error", persistJSONErr.Error(),
					)
				} else {
					logger.Info("structured extract data persisted",
						"source_id", input.SourceID,
						"key", persistKey,
						"updated", persistJSONOutput.Updated,
					)
				}
			}

			// Langfuse: report structured extract phase span
			if langfuseTraceID != "" {
				sePhaseEnd := workflow.Now(ctx)
				phaseInput := ReportLangfusePhaseInput{
					PhaseID:      sePhaseID,
					TraceID:      langfuseTraceID,
					PhaseName:    stage.Stage,
					StartTime:    seStart,
					EndTime:      sePhaseEnd,
					ParentSpanID: rootSpanID,
				}
				if seErr != nil {
					phaseInput.StatusMessage = seErr.Error()
					phaseInput.Level = "ERROR"
				}
				_ = workflow.ExecuteActivity(
					workflow.WithActivityOptions(ctx, fastOpts),
					pkgtemporal.ActivityReportLangfusePhase,
					phaseInput,
				).Get(ctx, nil)
			}

			state.status.StepsCompleted++

			if checkCancellation() {
				state.result.Status = "cancelled"
				state.result.Error = state.cancelReason
				return state.result, nil
			}
		}
	}

	if checkCancellation() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		return state.result, nil
	}

	// Stages 4-4.6: Deep Analysis and Findings (gated by skipAnalyze and pipeline definition)
	var analyzeOutput *DeepAnalyzeOutput
	analyzeFailed := false

	if !skipAnalyze && stageInPipeline(stageConfigMap, "analyze") {
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
		analyzePhaseID := sideEffectUUID(ctx)

		analyzeOpts := stageOpts("analyze", llmOpts)
		if input.TimeoutOverride > 0 {
			analyzeOpts.StartToCloseTimeout = input.TimeoutOverride
		}
		ctxAnalyze := workflow.WithActivityOptions(ctx, analyzeOpts)
		// Prefer the corrected extraction from Stage 3 (includes org→project
		// reclassification). Falls back to the original Stage 2 output.
		analyzeExtraction := extractOutput
		if contextOutput != nil && contextOutput.CorrectedExtraction != nil {
			analyzeExtraction = contextOutput.CorrectedExtraction
		}

		// Pass resolved people from context output to enrich the entities section
		var analyzeResolvedPeople []ResolvedPerson
		if contextOutput != nil {
			analyzeResolvedPeople = contextOutput.ResolvedPeople
		}

		analyzeOutput = &DeepAnalyzeOutput{}
		err = workflow.ExecuteActivity(ctxAnalyze, pkgtemporal.ActivityDeepAnalyze, DeepAnalyzeInput{
			TenantID:          input.TenantID,
			SourceID:          input.SourceID,
			ContentID:         input.ContentID,
			JobID:             input.JobID,
			Content:           parsedContent,
			Subject:           input.Subject, // Topic framing for deep analysis (pf-e219c1)
			ContentType:       input.ContentType,
			TriageCategory:    triageOutput.Category,
			TriageImportance:  triageOutput.Importance,
			ExtractionResult:  analyzeExtraction,
			ResolvedPeople:    analyzeResolvedPeople,
			BackgroundContext: contextOutput.BackgroundContext,
			ModelOverride:     input.ModelOverride,
			PromptOverride:    promptOverrideForStage(stageConfigMap, "analyze"),
			LangfuseTraceID:   langfuseTraceID,
			LangfusePhaseID:   analyzePhaseID,
		}).Get(ctx, analyzeOutput)
		if err != nil {
			durationMs := workflow.Now(ctx).Sub(analyzeStart).Milliseconds()
			logger.Error("pipeline stage span error",
				"stage.name", "analyze",
				"error.type", classifyTemporalError(err),
				"error.detail", err.Error(),
				"stage.duration_ms", durationMs,
			)
			logger.Error("pipeline stage failed (non-blocking)",
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
			analyzeFailed = true
		} else {
			logger.Info("pipeline stage span completed",
				"stage.name", "analyze",
				"stage.duration_ms", workflow.Now(ctx).Sub(analyzeStart).Milliseconds(),
				"stage.timeout_start_to_close_ms", analyzeOpts.StartToCloseTimeout.Milliseconds(),
			)
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

		// Langfuse: report Analyze phase span (always, even on failure, best-effort).
		// Generation is now reported by the AI coordinator via gRPC metadata.
		analyzeEnd := workflow.Now(ctx)
		analyzePhaseInput := ReportLangfusePhaseInput{
			PhaseID:      analyzePhaseID,
			TraceID:      langfuseTraceID,
			PhaseName:    "Analyze",
			StartTime:    analyzeStart,
			EndTime:      analyzeEnd,
			ParentSpanID: rootSpanID,
		}
		if analyzeFailed && err != nil {
			analyzePhaseInput.Level = "ERROR"
			analyzePhaseInput.StatusMessage = err.Error()
		}
		_ = workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityReportLangfusePhase,
			analyzePhaseInput,
		).Get(ctx, nil)

		state.status.StepsCompleted = 5

		if checkCancellation() {
			state.result.Status = "cancelled"
			state.result.Error = state.cancelReason
			return state.result, nil
		}
	}

	// Stage 4.5: Persist Findings (independently gated by pipeline definition)
	// Runs for any pipeline with "persist" in its definition, even without "analyze"
	// (e.g. notification/newsletter pipelines). Skipped if analyze ran but failed.
	persistShouldRun := stageInPipeline(stageConfigMap, "persist") &&
		!shouldGateStage("persist") &&
		!analyzeFailed &&
		(!stageInPipeline(stageConfigMap, "analyze") || !skipAnalyze)
	if persistShouldRun {
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

		// Find confirmed project ID: cross-reference resolved projects with
		// deep analysis topic mappings. Only tag assertions with a project when
		// the analysis confirms the content actually relates to that project.
		// This prevents mis-tagging assertions from unrelated content (e.g., IT
		// notifications) with the user's primary project (pf-de3670).
		var projectID *int64
		if contextOutput != nil && analyzeOutput != nil {
			projectID = selectConfirmedProjectID(contextOutput.ResolvedProjects, analyzeOutput.TopicMappings)
		}

		var persistOutput PersistFindingsOutput
		ctxPersist := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctxPersist, pkgtemporal.ActivityPersistFindings, PersistFindingsInput{
			TenantID:       input.TenantID,
			SourceID:       input.SourceID,
			ThreadID:       emailThreadID,
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
				if err := workflow.ExecuteActivity(ctxAssertionUpdate, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
					TenantID:       input.TenantID,
					SourceID:       input.SourceID,
					AssertionCount: &totalCount,
				}).Get(ctx, nil); err != nil {
					logger.Error("Failed to update content status",
						"source_id", input.SourceID,
						"target_status", "assertion_count",
						"error", err,
					)
				}
			}
		}
	}

	if !skipAnalyze && stageInPipeline(stageConfigMap, "analyze") {
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

	// Stage 4.7: Attribute Project (assertion-level project attribution)
	if stageInPipeline(stageConfigMap, "attribute_project") {
		updateStatus("attributing_projects", "AttributeProject")
		attrStage := stageByStatus("attributing_projects")
		logger.Info("pipeline stage starting",
			"source_id", input.SourceID,
			"stage", attrStage.Name,
			"stage_number", "4.7",
			"total_steps", state.status.TotalSteps,
		)
		attrStart := workflow.Now(ctx)

		var attrOutput AttributeProjectOutput
		ctxAttr := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctxAttr, pkgtemporal.ActivityAttributeProject, AttributeProjectInput{
			TenantID: input.TenantID,
			SourceID: input.SourceID,
			Subject:  input.Subject,
			BodyText: input.BodyText,
		}).Get(ctx, &attrOutput)
		if err != nil {
			logger.Warn("pipeline stage failed (non-blocking)",
				"source_id", input.SourceID,
				"stage", attrStage.Name,
				"stage_number", "4.7",
				"duration_ms", workflow.Now(ctx).Sub(attrStart).Milliseconds(),
				"status", "failed",
				"error", err.Error(),
			)
		} else {
			logger.Info("pipeline stage completed",
				"source_id", input.SourceID,
				"stage", attrStage.Name,
				"stage_number", "4.7",
				"duration_ms", workflow.Now(ctx).Sub(attrStart).Milliseconds(),
				"status", "completed",
				"assertions_attributed", attrOutput.AssertionsAttributed,
				"projects_matched", attrOutput.ProjectsMatched,
				"attribution_source", attrOutput.AttributionSource,
			)
		}
	}

	// Stage 4.8: Instruction Evaluation (watch instruction matching)
	if stageInPipeline(stageConfigMap, "instruction_evaluate") {
		updateStatus("evaluating_instructions", "InstructionEvaluate")
		instrStage := stageByStatus("evaluating_instructions")
		logger.Info("pipeline stage starting",
			"source_id", input.SourceID,
			"stage", instrStage.Name,
			"stage_number", "4.8",
			"total_steps", state.status.TotalSteps,
		)
		instrStart := workflow.Now(ctx)

		var instrOutput InstructionEvaluationOutput
		ctxInstr := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctxInstr, pkgtemporal.ActivityInstructionEvaluate, InstructionEvaluationInput{
			TenantID:  input.TenantID,
			SourceID:  input.SourceID,
			ContentID: input.ContentID,
		}).Get(ctx, &instrOutput)
		if err != nil {
			logger.Warn("pipeline stage failed (non-blocking)",
				"source_id", input.SourceID,
				"stage", instrStage.Name,
				"stage_number", "4.8",
				"duration_ms", workflow.Now(ctx).Sub(instrStart).Milliseconds(),
				"status", "failed",
				"error", err.Error(),
			)
		} else {
			logger.Info("pipeline stage completed",
				"source_id", input.SourceID,
				"stage", instrStage.Name,
				"stage_number", "4.8",
				"duration_ms", workflow.Now(ctx).Sub(instrStart).Milliseconds(),
				"status", "completed",
				"instructions_evaluated", instrOutput.InstructionsEvaluated,
				"matches_found", instrOutput.MatchesFound,
				"skipped", instrOutput.Skipped,
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
	embedPhaseID := sideEffectUUID(ctx)

	var embeddingID int64

	// Build []pkgtemporal.PipelineStageConfig filtered to embedding stage_kind.
	// Iterate stageConfigSlice (sorted) so the result is deterministic — map iteration is not.
	var embeddingStages []pkgtemporal.PipelineStageConfig
	for _, sc := range stageConfigSlice {
		if sc.StageKind == "embedding" {
			embeddingStages = append(embeddingStages, pkgtemporal.PipelineStageConfig{
				Stage:          sc.Stage,
				StageKind:      sc.StageKind,
				StageOrder:     sc.StageOrder,
				Enabled:        sc.Enabled,
				SkipWhenLow:    sc.SkipWhenLow,
				Optional:       sc.Optional,
				TimeoutSeconds: sc.TimeoutSeconds,
				ModelOverride:  sc.ModelOverride,
				PromptOverride: sc.PromptOverride,
				PersistKey:     sc.PersistKey,
				DependsOn:      sc.DependsOn,
			})
		}
	}

	dispatchReg := pkgtemporal.NewExecutorRegistry(&pkgtemporal.NoOpLangfuseReporter{})
	dispatchReg.Register("embedding", &pkgtemporal.EmbeddingExecutor{})

	stageInput := pkgtemporal.StageInput{
		TenantID:        input.TenantID,
		SourceID:        input.SourceID,
		ContentID:       input.ContentID,
		Content:         parsedContent,
		LangfuseTraceID: langfuseTraceID,
	}

	// Bridge workflow.Context into context.Context for the dispatch loop.
	dispatchCtx := pkgtemporal.WithWorkflowContext(context.Background(), ctx)

	dispatchOutputs, dispatchErr := pkgtemporal.ExecutePipeline(
		dispatchCtx,
		dispatchReg,
		embeddingStages,
		stageInput,
		logging.NewNopLogger(),
	)
	if dispatchErr != nil {
		err = dispatchErr
	} else {
		// Clear any stale error from prior non-blocking stages (e.g. TagProjects).
		// In the old sequential path, `err = workflow.ExecuteActivity(...)` always
		// overwrote err on success; we replicate that here explicitly.
		err = nil
		// Extract embeddingID from the output of any succeeded embedding stage.
		// Iterate embeddingStages (sorted slice) and look up in dispatchOutputs —
		// ranging over the map directly would be non-deterministic.
		// EmbeddingExecutor sets StageOutput.EmbeddingID directly to avoid json.Unmarshal
		// in the workflow body (which workflowcheck flags as non-deterministic).
		for _, stage := range embeddingStages {
			if out, ok := dispatchOutputs[stage.Stage]; ok && out.Success && out.EmbeddingID != 0 {
				embeddingID = out.EmbeddingID
			}
		}
	}

	if err != nil {
		runCompensation(ctx)
		pe := perrors.ClassifyError(err, "embed")
		logger.Warn("pipeline stage span error",
			"stage.name", "embed",
			"error.type", classifyTemporalError(err),
			"error.detail", err.Error(),
			"stage.duration_ms", workflow.Now(ctx).Sub(embedStart).Milliseconds(),
		)
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("embedding_failed: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Stage 5 Embedding failed — pipeline failed", "error", err, "error_code", pe.Code)

		// Update status to failed
		ctxFail := workflow.WithActivityOptions(ctx, fastOpts)
		if err := workflow.ExecuteActivity(ctxFail, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
			TenantID:        input.TenantID,
			SourceID:        input.SourceID,
			Status:          "failed",
			FailureCategory: string(pe.Code),
			FailureReason:   err.Error(),
		}).Get(ctx, nil); err != nil {
			logger.Error("Failed to update content status",
				"source_id", input.SourceID,
				"target_status", "failed",
				"error", err,
			)
		}

		return state.result, nil
	}

	logger.Info("pipeline stage span completed",
		"stage.name", "embed",
		"stage.duration_ms", workflow.Now(ctx).Sub(embedStart).Milliseconds(),
	)
	logger.Info("pipeline stage completed",
		"source_id", input.SourceID,
		"stage", embedStage.Name,
		"stage_number", embedStage.Number,
		"duration_ms", workflow.Now(ctx).Sub(embedStart).Milliseconds(),
		"status", "completed",
		"embedding_id", embeddingID,
	)

	// Langfuse: report Embeddings phase span (best-effort, non-blocking).
	// Generation is now reported by the AI coordinator via gRPC metadata.
	embedEnd := workflow.Now(ctx)
	_ = workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, fastOpts),
		pkgtemporal.ActivityReportLangfusePhase,
		ReportLangfusePhaseInput{
			PhaseID:      embedPhaseID,
			TraceID:      langfuseTraceID,
			PhaseName:    "Embeddings",
			StartTime:    embedStart,
			EndTime:      embedEnd,
			ParentSpanID: rootSpanID,
		},
	).Get(ctx, nil)

	state.result.EmbeddingID = &embeddingID

	// Add compensation to delete embedding on downstream failure
	compensations = append(compensations, func(ctx workflow.Context) error {
		return workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, fastOpts),
			pkgtemporal.ActivityDeleteEmbedding,
			embeddingID,
		).Get(ctx, nil)
	})

	completedSteps := 0
	for _, s := range stageConfigSlice {
		if s.Enabled && !shouldGateStage(s.Stage) {
			completedSteps++
		}
	}
	state.status.StepsCompleted = completedSteps

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
	if err := workflow.ExecuteActivity(ctxComplete, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		Status:   "completed",
	}).Get(ctx, nil); err != nil {
		logger.Error("Failed to update content status",
			"source_id", input.SourceID,
			"target_status", "completed",
			"error", err,
		)
	}

	state.result.Status = "completed"
	updateStatus("completed", "")

	logger.Info("SLM pipeline completed",
		"source_id", input.SourceID,
		"category", triageOutput.Category,
		"importance", triageOutput.Importance,
		"skip_deep", triageOutput.SkipDeep,
		"embedding_id", embeddingID,
	)

	// Finish Langfuse trace: close root span with real duration + flush (pf-1bfbaf).
	_ = workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, fastOpts),
		pkgtemporal.ActivityFinishLangfuseTrace,
		FinishLangfuseTraceInput{
			TraceID:    langfuseTraceID,
			RootSpanID: rootSpanID,
		},
	).Get(ctx, nil)

	// Event trigger evaluation: evaluate automation rules for this completed content item.
	// Best-effort — failures are logged but do not fail the pipeline.
	eventCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    2,
		},
	})
	var eventOutput EvaluateEventTriggersOutput
	eventErr := workflow.ExecuteActivity(eventCtx, pkgtemporal.ActivityEvaluateEventTriggers, EvaluateEventTriggersInput{
		TenantID:       input.TenantID,
		SourceID:       input.SourceID,
		ContentType:    input.ContentType,
		ContentSubtype: triageOutput.ContentSubtype,
		SourceSystem:   triageOutput.SourceSystem,
		SenderEmail:    input.SenderEmail,
		Subject:        input.Subject,
		Urgency:        strings.ToLower(triageOutput.Importance),
	}).Get(eventCtx, &eventOutput)
	if eventErr != nil {
		logger.Warn("Event trigger evaluation failed (non-blocking)",
			"source_id", input.SourceID,
			"error", eventErr,
		)
	} else if eventOutput.RulesMatched > 0 {
		logger.Info("Event trigger evaluation completed",
			"source_id", input.SourceID,
			"rules_evaluated", eventOutput.RulesEvaluated,
			"rules_matched", eventOutput.RulesMatched,
			"workflows_started", eventOutput.WorkflowsStarted,
		)
	}

	// Auto-drain: kick next pending item to maintain concurrency window
	// This is best-effort — if it fails, the pipeline still succeeds
	kickCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    2, // Minimal retries - this is best-effort
		},
	})
	var kickOutput KickNextPendingOutput
	kickErr := workflow.ExecuteActivity(kickCtx, pkgtemporal.ActivityKickNextPending, KickNextPendingInput{
		TenantID: input.TenantID,
		Limit:    0, // limit read from pipeline.kick_next_limit config by the activity
	}).Get(kickCtx, &kickOutput)
	if kickErr != nil {
		logger.Warn("Auto-drain kick failed (non-blocking)",
			"source_id", input.SourceID,
			"error", kickErr,
		)
	} else {
		logger.Info("Auto-drain kick completed",
			"source_id", input.SourceID,
			"queued_count", kickOutput.QueuedCount,
			"message", kickOutput.Message,
		)
	}

	return state.result, nil
}

// stageByStatus returns a minimal Stage with the given status name for logging.
func stageByStatus(statusName string) pkgtemporal.Stage {
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

// sideEffectUUID generates a deterministic UUID via workflow.SideEffect.
// This is safe to call from workflow code because workflow.SideEffect memoises
// the result across replays, preserving determinism.
func sideEffectUUID(ctx workflow.Context) string {
	var id string
	_ = workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		return uuid.New().String()
	}).Get(&id)
	return id
}

// buildPipelineDefinitionMetadata builds the Langfuse trace metadata from the pipeline definition.
// This includes pipeline_name, pipeline_stages, and model_overrides for stages with non-default models.
func buildPipelineDefinitionMetadata(pipelineName string, def *FetchPipelineDefinitionOutput) map[string]any {
	stages := make([]string, 0, len(def.Stages))
	modelOverrides := make(map[string]string)
	promptOverrides := make(map[string]int32)

	for _, s := range def.Stages {
		if s.Enabled {
			stages = append(stages, s.Stage)
		}
		if s.ModelOverride != "" {
			modelOverrides[s.Stage] = s.ModelOverride
		}
		if s.PromptOverride > 0 {
			promptOverrides[s.Stage] = s.PromptOverride
		}
	}

	metadata := map[string]any{
		"pipeline_name":   pipelineName,
		"pipeline_stages": stages,
	}
	if len(modelOverrides) > 0 {
		metadata["model_overrides"] = modelOverrides
	}
	if len(promptOverrides) > 0 {
		metadata["prompt_overrides"] = promptOverrides
	}

	return metadata
}

// buildStageConfigMap creates a lookup map from stage name to its config.
// Returns nil if pipelineDef is nil, not found, or has no stages.
func buildStageConfigMap(def *FetchPipelineDefinitionOutput) map[string]PipelineStageConfig {
	if def == nil || !def.Found || len(def.Stages) == 0 {
		return nil
	}
	m := make(map[string]PipelineStageConfig, len(def.Stages))
	for _, s := range def.Stages {
		m[s.Stage] = s
	}
	return m
}

// stageConfigMapKeys returns the stage names from a stageConfigMap as a sorted slice.
// Returns nil when the map is nil (no pipeline definition loaded), which signals
// the extraction activity to record all stages for backward compatibility.
// Keys are sorted for deterministic output — map iteration order is not stable.
func stageConfigMapKeys(m map[string]PipelineStageConfig) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stageInPipeline returns true if a stage should run based on the pipeline definition.
// If no definition is loaded (nil map), returns true (backward compat — fallback runs everything).
// If a definition exists, the stage must be present AND enabled to run.
func stageInPipeline(stageConfigMap map[string]PipelineStageConfig, stageName string) bool {
	if stageConfigMap == nil {
		return true
	}
	cfg, ok := stageConfigMap[stageName]
	if !ok {
		return false // Stage not in pipeline definition
	}
	return cfg.Enabled
}

// promptOverrideForStage returns the prompt version override for a stage from the pipeline definition.
// Returns 0 when the map is nil or the stage has no override, meaning "use active version".
func promptOverrideForStage(stageConfigMap map[string]PipelineStageConfig, stageName string) int32 {
	if stageConfigMap == nil {
		return 0
	}
	cfg, ok := stageConfigMap[stageName]
	if !ok {
		return 0
	}
	return cfg.PromptOverride
}

// orderedStages returns a copy of stages sorted by StageOrder ascending, with Stage name
// as a tiebreaker for full determinism. Accepts a slice (not a map) so the workflow body
// can call it without triggering a workflowcheck map-iteration violation.
func orderedStages(stages []PipelineStageConfig) []PipelineStageConfig {
	sorted := make([]PipelineStageConfig, len(stages))
	copy(sorted, stages)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].StageOrder != sorted[j].StageOrder {
			return sorted[i].StageOrder < sorted[j].StageOrder
		}
		return sorted[i].Stage < sorted[j].Stage
	})
	return sorted
}

// buildStageConfigOrdered returns pipeline stages sorted deterministically by (StageOrder, Stage).
// Use this alongside buildStageConfigMap so the workflow has a stable slice for iteration.
func buildStageConfigOrdered(def *FetchPipelineDefinitionOutput) []PipelineStageConfig {
	if def == nil || !def.Found || len(def.Stages) == 0 {
		return nil
	}
	stages := make([]PipelineStageConfig, len(def.Stages))
	copy(stages, def.Stages)
	sort.Slice(stages, func(i, j int) bool {
		if stages[i].StageOrder != stages[j].StageOrder {
			return stages[i].StageOrder < stages[j].StageOrder
		}
		return stages[i].Stage < stages[j].Stage
	})
	return stages
}

// looksLikeNotificationSender returns true when the sender address is a known
// automated notification domain. This is a deterministic pre-classification check
// used before triage to enable the early pipeline definition fetch (pf-1c083d).
//
// The patterns mirror the classification rule engine's seed rules (see
// migrations/seed_classification_rules.sql) but are evaluated without a DB call,
// so they can fire before triage. If this function returns true and triage later
// routes to a different pipeline, the post-triage re-fetch corrects it. False
// negatives are safe — the prompt override is a tuning hint, not a hard requirement.
func looksLikeNotificationSender(senderEmail string) bool {
	lower := strings.ToLower(senderEmail)
	notificationDomainPatterns := []string{
		// Google Docs / Sheets / Slides comment notifications
		"noreply@docs.google.com",
		"-noreply@docs.google.com",
		// GitHub (issues, PRs, reviews)
		"@github.com",
		// Jira / Atlassian
		"jira@",
		"@atlassian.net",
		// Aha!
		"@mailer.aha.io",
		// Slack
		"@slack.com",
		// Confluence
		"confluence@",
		// Generic noreply patterns typical of notification services
		"noreply@",
		"no-reply@",
		"notifications@",
		"notification@",
		"do-not-reply@",
		"donotreply@",
	}
	for _, pattern := range notificationDomainPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// PreClassifyContentInput is the input for the PreClassifyContent activity.
type PreClassifyContentInput struct {
	TenantID    string            `json:"tenant_id"`
	SenderEmail string            `json:"sender_email"`
	Subject     string            `json:"subject,omitempty"`
	ContentType string            `json:"content_type"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// PreClassifyContentOutput is the output from the PreClassifyContent activity.
type PreClassifyContentOutput struct {
	Pipeline       string `json:"pipeline,omitempty"`
	ContentSubtype string `json:"content_subtype,omitempty"`
	RuleName       string `json:"rule_name,omitempty"`
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
