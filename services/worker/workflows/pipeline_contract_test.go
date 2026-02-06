package workflows

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Activity type definitions (duplicated here to avoid import cycle with activities package)
// These mirror the types in services/worker/activities/*.go

type activityParseEmailInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	BodyText string `json:"body_text"`
	BodyHTML string `json:"body_html"`
}

type activityParseEmailOutput struct {
	CleanBody     string `json:"clean_body"`
	NewContent    string `json:"new_content"`
	QuotedContent string `json:"quoted_content"`
	IsReply       bool   `json:"is_reply"`
}

type activityParseTranscriptInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Content  string `json:"content"`
	Format   string `json:"format,omitempty"`
}

type activityParseTranscriptOutput struct {
	CleanText  string   `json:"clean_text"`
	Speakers   []string `json:"speakers"`
	DurationMs int      `json:"duration_ms"`
	Format     string   `json:"format"`
}

type activityTriageInput struct {
	TenantID    string `json:"tenant_id"`
	SourceID    int64  `json:"source_id"`
	ContentID   string `json:"content_id,omitempty"`
	JobID       string `json:"job_id"`
	Content     string `json:"content"`
	Subject     string `json:"subject,omitempty"`
	SenderEmail string `json:"sender_email,omitempty"`
	ContentType string `json:"content_type"`
}

type activityTriageOutput struct {
	Category   string  `json:"category"`
	Importance string  `json:"importance"`
	Reason     string  `json:"reason"`
	Confidence float32 `json:"confidence"`
	ModelUsed  string  `json:"model_used"`
	SkipDeep   bool    `json:"skip_deep"`
}

type activityExtractEntitiesInput struct {
	TenantID       string `json:"tenant_id"`
	SourceID       int64  `json:"source_id"`
	ContentID      string `json:"content_id,omitempty"`
	JobID          string `json:"job_id"`
	Content        string `json:"content"`
	TriageCategory string `json:"triage_category,omitempty"`
}

type activityPersonResult struct {
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type activityDateResult struct {
	Date    string `json:"date"`
	Context string `json:"context,omitempty"`
}

type activityActionItemResult struct {
	Assignee string `json:"assignee,omitempty"`
	Action   string `json:"action"`
	Due      string `json:"due,omitempty"`
}

type activityDetailedRisk struct {
	Description  string `json:"description"`
	SeverityHint string `json:"severity_hint,omitempty"`
	OwnerHint    string `json:"owner_hint,omitempty"`
}

type activityExtractEntitiesOutput struct {
	People               []activityPersonResult     `json:"people"`
	Dates                []activityDateResult       `json:"dates"`
	Projects             []string                   `json:"projects"`
	Organisations        []string                   `json:"organisations"`
	ActionItems          []activityActionItemResult `json:"action_items"`
	Decisions            []string                   `json:"decisions"`
	Risks                []string                   `json:"risks"`
	DetailedRisks        []activityDetailedRisk     `json:"detailed_risks,omitempty"`
	QualityGateTriggered bool                       `json:"quality_gate_triggered"`
	ModelUsed            string                     `json:"model_used"`
}

type activityResolvedPerson struct {
	Name       string  `json:"name"`
	PersonID   *int64  `json:"person_id,omitempty"`
	Confidence float32 `json:"confidence"`
	Source     string  `json:"source"`
	Role       string  `json:"role,omitempty"`
	Title      string  `json:"title,omitempty"`
	Department string  `json:"department,omitempty"`
	IsInternal bool    `json:"is_internal"`
}

type activityResolvedProject struct {
	Name      string `json:"name"`
	ProjectID *int64 `json:"project_id,omitempty"`
	Expansion string `json:"expansion,omitempty"`
	Source    string `json:"source"`
}

type activityContextPackage struct {
	TotalTokensUsed int `json:"total_tokens_used"`
	TokenBudget     int `json:"token_budget"`
}

type activityBuildContextInput struct {
	TenantID    string                          `json:"tenant_id"`
	SourceID    int64                           `json:"source_id"`
	ContentID   string                          `json:"content_id,omitempty"`
	JobID       string                          `json:"job_id"`
	ContentType string                          `json:"content_type"`
	Extraction  *activityExtractEntitiesOutput  `json:"extraction"`
	SenderEmail string                          `json:"sender_email,omitempty"`
	SenderName  string                          `json:"sender_name,omitempty"`
	Subject     string                          `json:"subject,omitempty"`
	ThreadID    string                          `json:"thread_id,omitempty"`
}

type activityBuildContextOutput struct {
	ResolvedPeople     []activityResolvedPerson  `json:"resolved_people"`
	ResolvedProjects   []activityResolvedProject `json:"resolved_projects"`
	UnresolvedTerms    []string                  `json:"unresolved_terms"`
	ContextPackage     *activityContextPackage   `json:"context_package"`
	TokensUsed         int                       `json:"tokens_used"`
	TokenBudget        int                       `json:"token_budget"`
	EntitiesResolved   int                       `json:"entities_resolved"`
	EntitiesUnresolved int                       `json:"entities_unresolved"`
}

type activitySentimentOutput struct {
	Score       float32  `json:"score"`
	Label       string   `json:"label"`
	Confidence  float32  `json:"confidence"`
	Indicators  []string `json:"indicators"`
	Explanation string   `json:"explanation"`
}

type activityTopicMappingOutput struct {
	Topic          string  `json:"topic"`
	RelatedProject string  `json:"related_project"`
	Relationship   string  `json:"relationship"`
	Confidence     float32 `json:"confidence"`
}

type activityVerifiedActionOutput struct {
	Description    string `json:"description"`
	Assignee       string `json:"assignee"`
	Due            string `json:"due"`
	Priority       string `json:"priority"`
	ContextExcerpt string `json:"context_excerpt"`
	Status         string `json:"status"`
}

type activityVerifiedDecisionOutput struct {
	Description    string `json:"description"`
	ContextExcerpt string `json:"context_excerpt"`
	Status         string `json:"status"`
}

type activityRiskReferenceOutput struct {
	RootID          *int64  `json:"root_id,omitempty"`
	Description     string  `json:"description"`
	LifecycleChange *string `json:"lifecycle_change,omitempty"`
	Significance    string  `json:"significance"`
	ContextExcerpt  string  `json:"context_excerpt"`
	SeverityChange  *string `json:"severity_change,omitempty"`
	OwnerChange     *string `json:"owner_change,omitempty"`
	IsNew           bool    `json:"is_new"`
}

type activityImplicitActionOutput struct {
	Description    string `json:"description"`
	Reasoning      string `json:"reasoning"`
	ContextExcerpt string `json:"context_excerpt"`
}

type activityDeepAnalyzeInput struct {
	TenantID          string                          `json:"tenant_id"`
	SourceID          int64                           `json:"source_id"`
	ContentID         string                          `json:"content_id,omitempty"`
	JobID             string                          `json:"job_id"`
	Content           string                          `json:"content"`
	ContentType       string                          `json:"content_type"`
	TriageCategory    string                          `json:"triage_category"`
	TriageImportance  string                          `json:"triage_importance"`
	ExtractionResult  *activityExtractEntitiesOutput  `json:"extraction_result"`
	BackgroundContext string                          `json:"background_context,omitempty"`
}

type activityDeepAnalyzeOutput struct {
	Summary           string                           `json:"summary"`
	Sentiment         *activitySentimentOutput         `json:"sentiment"`
	TopicMappings     []activityTopicMappingOutput     `json:"topic_mappings"`
	VerifiedActions   []activityVerifiedActionOutput   `json:"verified_action_items"`
	VerifiedDecisions []activityVerifiedDecisionOutput `json:"verified_decisions"`
	RiskReferences    []activityRiskReferenceOutput    `json:"risk_references"`
	Insights          []string                         `json:"strategic_insights"`
	ImplicitActions   []activityImplicitActionOutput   `json:"implicit_action_items"`
	ModelUsed         string                           `json:"model_used"`
}

type activityPersistFindingsActivityInput struct {
	TenantID       string                     `json:"tenant_id"`
	SourceID       int64                      `json:"source_id"`
	ThreadID       *int64                     `json:"thread_id,omitempty"`
	ProjectID      *int64                     `json:"project_id,omitempty"`
	Analysis       *activityDeepAnalyzeOutput `json:"analysis"`
	ResolvedPeople map[string]int64           `json:"resolved_people,omitempty"`
}

type activityPersistFindingsActivityOutput struct {
	AssertionsCreated    int `json:"assertions_created"`
	AssertionsSuperseded int `json:"assertions_superseded"`
	ReferencesCreated    int `json:"references_created"`
	ReviewItemsCreated   int `json:"review_items_created"`
	AffinityUpdates      int `json:"affinity_updates"`
}

// TestContract_ParseEmail_Input tests JSON round-trip between workflow and activity ParseEmail input types.
func TestContract_ParseEmail_Input(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		// Populate workflow type with all fields
		workflow := pipelineParseEmailInput{
			TenantID: "tenant-123",
			SourceID: 42,
			BodyText: "Hello world",
			BodyHTML: "<p>Hello world</p>",
		}

		// Marshal workflow type
		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		// Unmarshal into activity type
		var activity activityParseEmailInput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		// Assert all fields survived round-trip
		assert.Equal(t, workflow.TenantID, activity.TenantID)
		assert.Equal(t, workflow.SourceID, activity.SourceID)
		assert.Equal(t, workflow.BodyText, activity.BodyText)
		assert.Equal(t, workflow.BodyHTML, activity.BodyHTML)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		// Populate activity type with all fields
		activity := activityParseEmailInput{
			TenantID: "tenant-456",
			SourceID: 99,
			BodyText: "Test body",
			BodyHTML: "<div>Test body</div>",
		}

		// Marshal activity type
		data, err := json.Marshal(activity)
		require.NoError(t, err)

		// Unmarshal into workflow type
		var workflow pipelineParseEmailInput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		// Assert all fields survived round-trip
		assert.Equal(t, activity.TenantID, workflow.TenantID)
		assert.Equal(t, activity.SourceID, workflow.SourceID)
		assert.Equal(t, activity.BodyText, workflow.BodyText)
		assert.Equal(t, activity.BodyHTML, workflow.BodyHTML)
	})
}

// TestContract_ParseEmail_Output tests JSON round-trip between workflow and activity ParseEmail output types.
func TestContract_ParseEmail_Output(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		workflow := pipelineParseEmailOutput{
			CleanBody:     "Clean text",
			NewContent:    "New content",
			QuotedContent: "Quoted reply",
			IsReply:       true,
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityParseEmailOutput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		assert.Equal(t, workflow.CleanBody, activity.CleanBody)
		assert.Equal(t, workflow.NewContent, activity.NewContent)
		assert.Equal(t, workflow.QuotedContent, activity.QuotedContent)
		assert.Equal(t, workflow.IsReply, activity.IsReply)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		activity := activityParseEmailOutput{
			CleanBody:     "Activity clean",
			NewContent:    "Activity new",
			QuotedContent: "Activity quoted",
			IsReply:       false,
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineParseEmailOutput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		assert.Equal(t, activity.CleanBody, workflow.CleanBody)
		assert.Equal(t, activity.NewContent, workflow.NewContent)
		assert.Equal(t, activity.QuotedContent, workflow.QuotedContent)
		assert.Equal(t, activity.IsReply, workflow.IsReply)
	})
}

// TestContract_ParseTranscript_Input tests JSON round-trip for ParseTranscript input types.
func TestContract_ParseTranscript_Input(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		workflow := pipelineParseTranscriptInput{
			TenantID: "tenant-789",
			SourceID: 101,
			Content:  "00:00:01.000 --> 00:00:05.000\nSpeaker: Hello",
			Format:   "vtt",
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityParseTranscriptInput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		assert.Equal(t, workflow.TenantID, activity.TenantID)
		assert.Equal(t, workflow.SourceID, activity.SourceID)
		assert.Equal(t, workflow.Content, activity.Content)
		assert.Equal(t, workflow.Format, activity.Format)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		activity := activityParseTranscriptInput{
			TenantID: "tenant-321",
			SourceID: 202,
			Content:  "Speaker A: Test transcript",
			Format:   "srt",
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineParseTranscriptInput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		assert.Equal(t, activity.TenantID, workflow.TenantID)
		assert.Equal(t, activity.SourceID, workflow.SourceID)
		assert.Equal(t, activity.Content, workflow.Content)
		assert.Equal(t, activity.Format, workflow.Format)
	})
}

// TestContract_ParseTranscript_Output tests JSON round-trip for ParseTranscript output types.
func TestContract_ParseTranscript_Output(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		workflow := pipelineParseTranscriptOutput{
			CleanText:  "Speaker A: Hello world",
			Speakers:   []string{"Speaker A", "Speaker B"},
			DurationMs: 30000,
			Format:     "vtt",
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityParseTranscriptOutput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		assert.Equal(t, workflow.CleanText, activity.CleanText)
		assert.Equal(t, workflow.Speakers, activity.Speakers)
		assert.Equal(t, workflow.DurationMs, activity.DurationMs)
		assert.Equal(t, workflow.Format, activity.Format)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		activity := activityParseTranscriptOutput{
			CleanText:  "Activity transcript text",
			Speakers:   []string{"Alice", "Bob", "Carol"},
			DurationMs: 45000,
			Format:     "srt",
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineParseTranscriptOutput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		assert.Equal(t, activity.CleanText, workflow.CleanText)
		assert.Equal(t, activity.Speakers, workflow.Speakers)
		assert.Equal(t, activity.DurationMs, workflow.DurationMs)
		assert.Equal(t, activity.Format, workflow.Format)
	})
}

// TestContract_Triage_Input tests JSON round-trip for Triage input types.
func TestContract_Triage_Input(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		workflow := pipelineTriageInput{
			TenantID:    "tenant-111",
			SourceID:    303,
			ContentID:   "em-abc123",
			JobID:       "job-555",
			Content:     "Important project update",
			Subject:     "Project Alpha Status",
			SenderEmail: "pm@example.com",
			ContentType: "email",
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityTriageInput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		assert.Equal(t, workflow.TenantID, activity.TenantID)
		assert.Equal(t, workflow.SourceID, activity.SourceID)
		assert.Equal(t, workflow.ContentID, activity.ContentID)
		assert.Equal(t, workflow.JobID, activity.JobID)
		assert.Equal(t, workflow.Content, activity.Content)
		assert.Equal(t, workflow.Subject, activity.Subject)
		assert.Equal(t, workflow.SenderEmail, activity.SenderEmail)
		assert.Equal(t, workflow.ContentType, activity.ContentType)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		activity := activityTriageInput{
			TenantID:    "tenant-222",
			SourceID:    404,
			ContentID:   "mt-xyz789",
			JobID:       "job-666",
			Content:     "Meeting transcript",
			Subject:     "Weekly Sync",
			SenderEmail: "organizer@example.com",
			ContentType: "meeting",
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineTriageInput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		assert.Equal(t, activity.TenantID, workflow.TenantID)
		assert.Equal(t, activity.SourceID, workflow.SourceID)
		assert.Equal(t, activity.ContentID, workflow.ContentID)
		assert.Equal(t, activity.JobID, workflow.JobID)
		assert.Equal(t, activity.Content, workflow.Content)
		assert.Equal(t, activity.Subject, workflow.Subject)
		assert.Equal(t, activity.SenderEmail, workflow.SenderEmail)
		assert.Equal(t, activity.ContentType, workflow.ContentType)
	})
}

// TestContract_Triage_Output tests JSON round-trip for Triage output types.
func TestContract_Triage_Output(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		workflow := pipelineTriageOutput{
			Category:   "RISK_ISSUE",
			Importance: "HIGH",
			Reason:     "Critical risk identified",
			Confidence: 0.92,
			ModelUsed:  "llama-3.2-1b",
			SkipDeep:   false,
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityTriageOutput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		assert.Equal(t, workflow.Category, activity.Category)
		assert.Equal(t, workflow.Importance, activity.Importance)
		assert.Equal(t, workflow.Reason, activity.Reason)
		assert.Equal(t, workflow.Confidence, activity.Confidence)
		assert.Equal(t, workflow.ModelUsed, activity.ModelUsed)
		assert.Equal(t, workflow.SkipDeep, activity.SkipDeep)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		activity := activityTriageOutput{
			Category:   "PERSONAL",
			Importance: "LOW",
			Reason:     "Personal communication",
			Confidence: 0.88,
			ModelUsed:  "llama-3.2-1b",
			SkipDeep:   true,
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineTriageOutput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		assert.Equal(t, activity.Category, workflow.Category)
		assert.Equal(t, activity.Importance, workflow.Importance)
		assert.Equal(t, activity.Reason, workflow.Reason)
		assert.Equal(t, activity.Confidence, workflow.Confidence)
		assert.Equal(t, activity.ModelUsed, workflow.ModelUsed)
		assert.Equal(t, activity.SkipDeep, workflow.SkipDeep)
	})
}

// TestContract_ExtractEntities_Input tests JSON round-trip for ExtractEntities input types.
func TestContract_ExtractEntities_Input(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		workflow := pipelineExtractInput{
			TenantID:  "tenant-333",
			SourceID:  505,
			ContentID: "em-def456",
			JobID:     "job-777",
			Content:   "Project Alpha risk review",
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityExtractEntitiesInput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		// Top-level fields match
		assert.Equal(t, workflow.TenantID, activity.TenantID)
		assert.Equal(t, workflow.SourceID, activity.SourceID)
		assert.Equal(t, workflow.ContentID, activity.ContentID)
		assert.Equal(t, workflow.JobID, activity.JobID)
		assert.Equal(t, workflow.Content, activity.Content)

		// TODO: fix mismatch - activity has extra TriageCategory field not in workflow type
		// This means activity can't receive triage_category from workflow JSON
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		activity := activityExtractEntitiesInput{
			TenantID:       "tenant-444",
			SourceID:       606,
			ContentID:      "mt-ghi789",
			JobID:          "job-888",
			Content:        "Meeting notes with action items",
			TriageCategory: "PROJECT_UPDATE",
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineExtractInput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		// Top-level fields that exist in both types
		assert.Equal(t, activity.TenantID, workflow.TenantID)
		assert.Equal(t, activity.SourceID, workflow.SourceID)
		assert.Equal(t, activity.ContentID, workflow.ContentID)
		assert.Equal(t, activity.JobID, workflow.JobID)
		assert.Equal(t, activity.Content, workflow.Content)

		// TODO: fix mismatch - workflow type is missing TriageCategory field
		// activity.TriageCategory is lost in this direction
	})
}

// TestContract_ExtractEntities_Output tests JSON round-trip for ExtractEntities output types.
func TestContract_ExtractEntities_Output(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		workflow := pipelineExtractOutput{
			People: []pipelinePersonResult{
				{Name: "Alice Smith", Role: "engineer"},
				{Name: "Bob Jones", Role: "manager"},
			},
			Dates: []pipelineDateResult{
				{Date: "2026-03-15", Context: "delivery deadline", IsDeadline: true},
				{Date: "2026-04-01", Context: "review date", IsDeadline: false},
			},
			Projects:      []string{"Project Alpha", "Project Beta"},
			Organisations: []string{"Acme Corp", "Beta Inc"},
			ActionItems: []pipelineActionItemResult{
				{Description: "Fix bug #123", Assignee: "Alice", Due: "2026-02-10", Priority: "high"},
				{Description: "Update docs", Assignee: "Bob", Due: "2026-02-15", Priority: "medium"},
			},
			Decisions: []string{"Decision A", "Decision B"},
			Risks:     []string{"Risk 1", "Risk 2"},
			DetailedRisks: []pipelineDetailedRisk{
				{Description: "Database latency", Severity: "high", Owner: "Alice", Impact: "performance degradation"},
				{Description: "API rate limits", Severity: "medium", Owner: "Bob", Impact: "service disruption"},
			},
			QualityGateTriggered: true,
			ModelUsed:            "llama-3.2-3b",
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityExtractEntitiesOutput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		// People: Name and Role match
		assert.Equal(t, len(workflow.People), len(activity.People))
		for i := range workflow.People {
			assert.Equal(t, workflow.People[i].Name, activity.People[i].Name)
			assert.Equal(t, workflow.People[i].Role, activity.People[i].Role)
		}

		// Dates: Date and Context match
		assert.Equal(t, len(workflow.Dates), len(activity.Dates))
		for i := range workflow.Dates {
			assert.Equal(t, workflow.Dates[i].Date, activity.Dates[i].Date)
			assert.Equal(t, workflow.Dates[i].Context, activity.Dates[i].Context)
			// TODO: fix mismatch - pipelineDateResult.IsDeadline (json:"is_deadline") is MISSING from activity DateResult
			// workflow.Dates[i].IsDeadline is lost
		}

		// Projects and Organisations: exact match
		assert.Equal(t, workflow.Projects, activity.Projects)
		assert.Equal(t, workflow.Organisations, activity.Organisations)

		// ActionItems: field name mismatch
		assert.Equal(t, len(workflow.ActionItems), len(activity.ActionItems))
		for i := range workflow.ActionItems {
			// TODO: fix mismatch - pipelineActionItemResult.Description (json:"description") vs ActionItemResult.Action (json:"action")
			// These are DIFFERENT field names! Description doesn't map to Action.
			// workflow.ActionItems[i].Description is lost, and activity.Action is empty from workflow JSON

			assert.Equal(t, workflow.ActionItems[i].Assignee, activity.ActionItems[i].Assignee)
			assert.Equal(t, workflow.ActionItems[i].Due, activity.ActionItems[i].Due)
			// TODO: fix mismatch - pipelineActionItemResult.Priority is MISSING from activity ActionItemResult
			// workflow.ActionItems[i].Priority is lost
		}

		// Decisions and Risks: exact match
		assert.Equal(t, workflow.Decisions, activity.Decisions)
		assert.Equal(t, workflow.Risks, activity.Risks)

		// DetailedRisks: field name mismatches
		assert.Equal(t, len(workflow.DetailedRisks), len(activity.DetailedRisks))
		for i := range workflow.DetailedRisks {
			assert.Equal(t, workflow.DetailedRisks[i].Description, activity.DetailedRisks[i].Description)
			// TODO: fix mismatch - pipelineDetailedRisk.Severity (json:"severity") vs DetailedRisk.SeverityHint (json:"severity_hint")
			// workflow.Severity doesn't map to activity.SeverityHint
			// TODO: fix mismatch - pipelineDetailedRisk.Owner (json:"owner") vs DetailedRisk.OwnerHint (json:"owner_hint")
			// workflow.Owner doesn't map to activity.OwnerHint
			// TODO: fix mismatch - pipelineDetailedRisk.Impact is MISSING from activity DetailedRisk
			// workflow.DetailedRisks[i].Impact is lost
		}

		// Top-level fields
		assert.Equal(t, workflow.QualityGateTriggered, activity.QualityGateTriggered)
		assert.Equal(t, workflow.ModelUsed, activity.ModelUsed)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		activity := activityExtractEntitiesOutput{
			People: []activityPersonResult{
				{Name: "Carol White", Role: "designer"},
			},
			Dates: []activityDateResult{
				{Date: "2026-05-20", Context: "launch date"},
			},
			Projects:      []string{"Gamma Project"},
			Organisations: []string{"Gamma LLC"},
			ActionItems: []activityActionItemResult{
				{Action: "Review PR #45", Assignee: "Carol", Due: "2026-02-20"},
			},
			Decisions: []string{"Decision X"},
			Risks:     []string{"Risk Y"},
			DetailedRisks: []activityDetailedRisk{
				{Description: "Security vulnerability", SeverityHint: "critical", OwnerHint: "Carol"},
			},
			QualityGateTriggered: false,
			ModelUsed:            "llama-3.2-3b",
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineExtractOutput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		// People: Name and Role match
		assert.Equal(t, len(activity.People), len(workflow.People))
		for i := range activity.People {
			assert.Equal(t, activity.People[i].Name, workflow.People[i].Name)
			assert.Equal(t, activity.People[i].Role, workflow.People[i].Role)
		}

		// Dates: Date and Context match
		assert.Equal(t, len(activity.Dates), len(workflow.Dates))
		for i := range activity.Dates {
			assert.Equal(t, activity.Dates[i].Date, workflow.Dates[i].Date)
			assert.Equal(t, activity.Dates[i].Context, workflow.Dates[i].Context)
			// TODO: fix mismatch - workflow expects IsDeadline but activity doesn't have it
			// workflow.Dates[i].IsDeadline will be zero-value (false)
		}

		// Projects and Organisations: exact match
		assert.Equal(t, activity.Projects, workflow.Projects)
		assert.Equal(t, activity.Organisations, workflow.Organisations)

		// ActionItems: field name mismatch
		assert.Equal(t, len(activity.ActionItems), len(workflow.ActionItems))
		for i := range activity.ActionItems {
			// TODO: fix mismatch - activity.Action (json:"action") doesn't map to workflow.Description (json:"description")
			// activity.ActionItems[i].Action is lost, workflow.Description is empty

			assert.Equal(t, activity.ActionItems[i].Assignee, workflow.ActionItems[i].Assignee)
			assert.Equal(t, activity.ActionItems[i].Due, workflow.ActionItems[i].Due)
			// TODO: fix mismatch - workflow expects Priority but activity doesn't have it
			// workflow.ActionItems[i].Priority will be empty string
		}

		// Decisions and Risks: exact match
		assert.Equal(t, activity.Decisions, workflow.Decisions)
		assert.Equal(t, activity.Risks, workflow.Risks)

		// DetailedRisks: field name mismatches
		assert.Equal(t, len(activity.DetailedRisks), len(workflow.DetailedRisks))
		for i := range activity.DetailedRisks {
			assert.Equal(t, activity.DetailedRisks[i].Description, workflow.DetailedRisks[i].Description)
			// TODO: fix mismatch - activity.SeverityHint (json:"severity_hint") doesn't map to workflow.Severity (json:"severity")
			// activity.SeverityHint is lost, workflow.Severity is empty
			// TODO: fix mismatch - activity.OwnerHint (json:"owner_hint") doesn't map to workflow.Owner (json:"owner")
			// activity.OwnerHint is lost, workflow.Owner is empty
			// TODO: fix mismatch - workflow expects Impact but activity doesn't have it
			// workflow.DetailedRisks[i].Impact will be empty string
		}

		// Top-level fields
		assert.Equal(t, activity.QualityGateTriggered, workflow.QualityGateTriggered)
		assert.Equal(t, activity.ModelUsed, workflow.ModelUsed)
	})
}

// TestContract_BuildContext_Input tests JSON round-trip for BuildContextPackage input types.
func TestContract_BuildContext_Input(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		extractOutput := &pipelineExtractOutput{
			People:    []pipelinePersonResult{{Name: "Dave", Role: "lead"}},
			Projects:  []string{"Delta"},
			ModelUsed: "test-model",
		}

		workflow := pipelineContextInput{
			TenantID:    "tenant-555",
			SourceID:    707,
			ContentID:   "em-jkl012",
			JobID:       "job-999",
			ContentType: "email",
			Extraction:  extractOutput,
			SenderEmail: "dave@example.com",
			SenderName:  "Dave Smith",
			Subject:     "Delta Update",
			ThreadID:    "thread-123",
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityBuildContextInput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		// Top-level fields match
		assert.Equal(t, workflow.TenantID, activity.TenantID)
		assert.Equal(t, workflow.SourceID, activity.SourceID)
		assert.Equal(t, workflow.ContentID, activity.ContentID)
		assert.Equal(t, workflow.JobID, activity.JobID)
		assert.Equal(t, workflow.ContentType, activity.ContentType)
		assert.Equal(t, workflow.SenderEmail, activity.SenderEmail)
		assert.Equal(t, workflow.SenderName, activity.SenderName)
		assert.Equal(t, workflow.Subject, activity.Subject)
		assert.Equal(t, workflow.ThreadID, activity.ThreadID)

		// Extraction field: workflow uses *pipelineExtractOutput, activity uses *ExtractEntitiesOutput
		// JSON round-trip works for shared fields, but mismatched fields are lost (see ExtractEntities_Output tests)
		assert.NotNil(t, activity.Extraction)
		assert.Equal(t, len(workflow.Extraction.People), len(activity.Extraction.People))
		assert.Equal(t, len(workflow.Extraction.Projects), len(activity.Extraction.Projects))
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		extractOutput := &activityExtractEntitiesOutput{
			People:    []activityPersonResult{{Name: "Eve", Role: "analyst"}},
			Projects:  []string{"Epsilon"},
			ModelUsed: "test-model",
		}

		activity := activityBuildContextInput{
			TenantID:    "tenant-666",
			SourceID:    808,
			ContentID:   "mt-mno345",
			JobID:       "job-1111",
			ContentType: "meeting",
			Extraction:  extractOutput,
			SenderEmail: "eve@example.com",
			SenderName:  "Eve Johnson",
			Subject:     "Epsilon Review",
			ThreadID:    "thread-456",
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineContextInput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		// Top-level fields match
		assert.Equal(t, activity.TenantID, workflow.TenantID)
		assert.Equal(t, activity.SourceID, workflow.SourceID)
		assert.Equal(t, activity.ContentID, workflow.ContentID)
		assert.Equal(t, activity.JobID, workflow.JobID)
		assert.Equal(t, activity.ContentType, workflow.ContentType)
		assert.Equal(t, activity.SenderEmail, workflow.SenderEmail)
		assert.Equal(t, activity.SenderName, workflow.SenderName)
		assert.Equal(t, activity.Subject, workflow.Subject)
		assert.Equal(t, activity.ThreadID, workflow.ThreadID)

		// Extraction field: same mismatch as above
		assert.NotNil(t, workflow.Extraction)
		assert.Equal(t, len(activity.Extraction.People), len(workflow.Extraction.People))
		assert.Equal(t, len(activity.Extraction.Projects), len(workflow.Extraction.Projects))
	})
}

// TestContract_BuildContext_Output tests JSON round-trip for BuildContextPackage output types.
func TestContract_BuildContext_Output(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		personID := int64(100)
		projectID := int64(200)

		workflow := pipelineContextOutput{
			ResolvedPeople: []pipelineResolvedPerson{
				{Name: "Frank", PersonID: &personID, Confidence: 0.95, Source: "exact_match"},
			},
			ResolvedProjects: []pipelineResolvedProject{
				{Name: "Zeta", ProjectID: &projectID, Source: "keyword"},
			},
			UnresolvedTerms:    []string{"TLA", "FLA"},
			ContextPackage:     map[string]interface{}{"key": "value"}, // interface{} allows any JSON
			TokensUsed:         500,
			TokenBudget:        2000,
			EntitiesResolved:   1,
			EntitiesUnresolved: 2,
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityBuildContextOutput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		// ResolvedPeople: partial match
		assert.Equal(t, len(workflow.ResolvedPeople), len(activity.ResolvedPeople))
		for i := range workflow.ResolvedPeople {
			assert.Equal(t, workflow.ResolvedPeople[i].Name, activity.ResolvedPeople[i].Name)
			assert.Equal(t, workflow.ResolvedPeople[i].PersonID, activity.ResolvedPeople[i].PersonID)
			assert.Equal(t, workflow.ResolvedPeople[i].Confidence, activity.ResolvedPeople[i].Confidence)
			assert.Equal(t, workflow.ResolvedPeople[i].Source, activity.ResolvedPeople[i].Source)
			// TODO: fix mismatch - pipelineResolvedPerson missing: Role, Title, Department, IsInternal
			// These fields exist in activity.ResolvedPerson but not in workflow type
		}

		// ResolvedProjects: partial match
		assert.Equal(t, len(workflow.ResolvedProjects), len(activity.ResolvedProjects))
		for i := range workflow.ResolvedProjects {
			assert.Equal(t, workflow.ResolvedProjects[i].Name, activity.ResolvedProjects[i].Name)
			assert.Equal(t, workflow.ResolvedProjects[i].ProjectID, activity.ResolvedProjects[i].ProjectID)
			assert.Equal(t, workflow.ResolvedProjects[i].Source, activity.ResolvedProjects[i].Source)
			// TODO: fix mismatch - pipelineResolvedProject missing: Expansion
			// This field exists in activity.ResolvedProject but not in workflow type
		}

		// Other fields
		assert.Equal(t, workflow.UnresolvedTerms, activity.UnresolvedTerms)
		assert.Equal(t, workflow.TokensUsed, activity.TokensUsed)
		assert.Equal(t, workflow.TokenBudget, activity.TokenBudget)
		assert.Equal(t, workflow.EntitiesResolved, activity.EntitiesResolved)
		assert.Equal(t, workflow.EntitiesUnresolved, activity.EntitiesUnresolved)

		// ContextPackage: workflow uses interface{}, activity uses *ContextPackage
		// JSON round-trip works because interface{} can hold any unmarshaled JSON
		assert.NotNil(t, activity.ContextPackage)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		personID := int64(300)
		projectID := int64(400)

		activity := activityBuildContextOutput{
			ResolvedPeople: []activityResolvedPerson{
				{
					Name:       "Grace",
					PersonID:   &personID,
					Confidence: 0.88,
					Source:     "fuzzy",
					Role:       "architect",
					Title:      "Senior Architect",
					Department: "Engineering",
					IsInternal: true,
				},
			},
			ResolvedProjects: []activityResolvedProject{
				{Name: "Eta", ProjectID: &projectID, Expansion: "Extended Test Architecture", Source: "glossary"},
			},
			UnresolvedTerms: []string{"UNK1"},
			ContextPackage: &activityContextPackage{
				TotalTokensUsed: 300,
				TokenBudget:     1000,
			},
			TokensUsed:         300,
			TokenBudget:        1000,
			EntitiesResolved:   1,
			EntitiesUnresolved: 1,
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineContextOutput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		// ResolvedPeople: workflow receives subset of fields
		assert.Equal(t, len(activity.ResolvedPeople), len(workflow.ResolvedPeople))
		for i := range activity.ResolvedPeople {
			assert.Equal(t, activity.ResolvedPeople[i].Name, workflow.ResolvedPeople[i].Name)
			assert.Equal(t, activity.ResolvedPeople[i].PersonID, workflow.ResolvedPeople[i].PersonID)
			assert.Equal(t, activity.ResolvedPeople[i].Confidence, workflow.ResolvedPeople[i].Confidence)
			assert.Equal(t, activity.ResolvedPeople[i].Source, workflow.ResolvedPeople[i].Source)
			// TODO: fix mismatch - activity fields Role, Title, Department, IsInternal are lost
			// workflow.ResolvedPeople[i] doesn't have these fields
		}

		// ResolvedProjects: workflow receives subset of fields
		assert.Equal(t, len(activity.ResolvedProjects), len(workflow.ResolvedProjects))
		for i := range activity.ResolvedProjects {
			assert.Equal(t, activity.ResolvedProjects[i].Name, workflow.ResolvedProjects[i].Name)
			assert.Equal(t, activity.ResolvedProjects[i].ProjectID, workflow.ResolvedProjects[i].ProjectID)
			assert.Equal(t, activity.ResolvedProjects[i].Source, workflow.ResolvedProjects[i].Source)
			// TODO: fix mismatch - activity.Expansion is lost
			// workflow.ResolvedProjects[i] doesn't have Expansion field
		}

		// Other fields
		assert.Equal(t, activity.UnresolvedTerms, workflow.UnresolvedTerms)
		assert.Equal(t, activity.TokensUsed, workflow.TokensUsed)
		assert.Equal(t, activity.TokenBudget, workflow.TokenBudget)
		assert.Equal(t, activity.EntitiesResolved, workflow.EntitiesResolved)
		assert.Equal(t, activity.EntitiesUnresolved, workflow.EntitiesUnresolved)

		// ContextPackage: workflow uses interface{}, so unmarshaling works
		assert.NotNil(t, workflow.ContextPackage)
	})
}

// TestContract_DeepAnalyze_Input tests JSON round-trip for DeepAnalyze input types.
func TestContract_DeepAnalyze_Input(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		extractResult := &pipelineExtractOutput{
			People:    []pipelinePersonResult{{Name: "Henry"}},
			Projects:  []string{"Theta"},
			ModelUsed: "test-model",
		}

		workflow := pipelineAnalyzeInput{
			TenantID:          "tenant-777",
			SourceID:          909,
			ContentID:         "em-pqr678",
			JobID:             "job-1212",
			Content:           "Strategic analysis content",
			ContentType:       "email",
			TriageCategory:    "STRATEGIC",
			TriageImportance:  "HIGH",
			ExtractionResult:  extractResult,
			BackgroundContext: "Context from Stage 3",
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityDeepAnalyzeInput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		// Top-level fields match
		assert.Equal(t, workflow.TenantID, activity.TenantID)
		assert.Equal(t, workflow.SourceID, activity.SourceID)
		assert.Equal(t, workflow.ContentID, activity.ContentID)
		assert.Equal(t, workflow.JobID, activity.JobID)
		assert.Equal(t, workflow.Content, activity.Content)
		assert.Equal(t, workflow.ContentType, activity.ContentType)
		assert.Equal(t, workflow.TriageCategory, activity.TriageCategory)
		assert.Equal(t, workflow.TriageImportance, activity.TriageImportance)
		assert.Equal(t, workflow.BackgroundContext, activity.BackgroundContext)

		// ExtractionResult: workflow uses *pipelineExtractOutput, activity uses *ExtractEntitiesOutput
		// Same mismatch as BuildContext input (see ExtractEntities_Output tests)
		assert.NotNil(t, activity.ExtractionResult)
		assert.Equal(t, len(workflow.ExtractionResult.People), len(activity.ExtractionResult.People))
		assert.Equal(t, len(workflow.ExtractionResult.Projects), len(activity.ExtractionResult.Projects))
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		extractResult := &activityExtractEntitiesOutput{
			People:    []activityPersonResult{{Name: "Iris"}},
			Projects:  []string{"Iota"},
			ModelUsed: "test-model",
		}

		activity := activityDeepAnalyzeInput{
			TenantID:          "tenant-888",
			SourceID:          1010,
			ContentID:         "mt-stu901",
			JobID:             "job-1313",
			Content:           "Meeting analysis content",
			ContentType:       "meeting",
			TriageCategory:    "CUSTOMER",
			TriageImportance:  "MEDIUM",
			ExtractionResult:  extractResult,
			BackgroundContext: "Activity context",
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineAnalyzeInput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		// Top-level fields match
		assert.Equal(t, activity.TenantID, workflow.TenantID)
		assert.Equal(t, activity.SourceID, workflow.SourceID)
		assert.Equal(t, activity.ContentID, workflow.ContentID)
		assert.Equal(t, activity.JobID, workflow.JobID)
		assert.Equal(t, activity.Content, workflow.Content)
		assert.Equal(t, activity.ContentType, workflow.ContentType)
		assert.Equal(t, activity.TriageCategory, workflow.TriageCategory)
		assert.Equal(t, activity.TriageImportance, workflow.TriageImportance)
		assert.Equal(t, activity.BackgroundContext, workflow.BackgroundContext)

		// ExtractionResult: same mismatch
		assert.NotNil(t, workflow.ExtractionResult)
		assert.Equal(t, len(activity.ExtractionResult.People), len(workflow.ExtractionResult.People))
		assert.Equal(t, len(activity.ExtractionResult.Projects), len(workflow.ExtractionResult.Projects))
	})
}

// TestContract_DeepAnalyze_Output tests JSON round-trip for DeepAnalyze output types.
func TestContract_DeepAnalyze_Output(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		// Workflow uses interface{} for most fields - can hold any JSON value
		workflow := pipelineAnalyzeOutput{
			Summary:           "Test summary",
			Sentiment:         map[string]interface{}{"score": 0.5, "label": "neutral"},
			TopicMappings:     []interface{}{map[string]interface{}{"topic": "A"}},
			VerifiedActions:   []interface{}{map[string]interface{}{"description": "Action 1"}},
			VerifiedDecisions: []interface{}{map[string]interface{}{"description": "Decision 1"}},
			RiskReferences:    []interface{}{map[string]interface{}{"description": "Risk 1"}},
			Insights:          []string{"Insight 1", "Insight 2"},
			ImplicitActions:   []interface{}{map[string]interface{}{"description": "Implicit 1"}},
			ModelUsed:         "gemini-2.0-flash",
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityDeepAnalyzeOutput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		// Summary, Insights, ModelUsed: exact match
		assert.Equal(t, workflow.Summary, activity.Summary)
		assert.Equal(t, workflow.Insights, activity.Insights)
		assert.Equal(t, workflow.ModelUsed, activity.ModelUsed)

		// TODO: fix mismatch - workflow uses interface{} types, activity uses concrete types
		// Unmarshaling from interface{} (which holds map[string]interface{}) into concrete types works
		// but the types don't match structurally
		// Workflow → Activity direction: interface{} → concrete type (works in JSON)
		assert.NotNil(t, activity.Sentiment)
		assert.NotEmpty(t, activity.TopicMappings)
		assert.NotEmpty(t, activity.VerifiedActions)
		assert.NotEmpty(t, activity.VerifiedDecisions)
		assert.NotEmpty(t, activity.RiskReferences)
		assert.NotEmpty(t, activity.ImplicitActions)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		rootID := int64(500)
		lifecycleChange := "escalated"
		severityChange := "high"
		ownerChange := "Jack"

		activity := activityDeepAnalyzeOutput{
			Summary: "Activity summary",
			Sentiment: &activitySentimentOutput{
				Score:       0.75,
				Label:       "positive",
				Confidence:  0.9,
				Indicators:  []string{"good", "great"},
				Explanation: "Positive feedback",
			},
			TopicMappings: []activityTopicMappingOutput{
				{Topic: "Topic A", RelatedProject: "Kappa", Relationship: "primary", Confidence: 0.88},
			},
			VerifiedActions: []activityVerifiedActionOutput{
				{Description: "Action A", Assignee: "Jack", Due: "2026-03-01", Priority: "high", ContextExcerpt: "excerpt", Status: "confirmed"},
			},
			VerifiedDecisions: []activityVerifiedDecisionOutput{
				{Description: "Decision A", ContextExcerpt: "excerpt", Status: "confirmed"},
			},
			RiskReferences: []activityRiskReferenceOutput{
				{
					RootID:          &rootID,
					Description:     "Risk A",
					LifecycleChange: &lifecycleChange,
					Significance:    "primary",
					ContextExcerpt:  "excerpt",
					SeverityChange:  &severityChange,
					OwnerChange:     &ownerChange,
					IsNew:           false,
				},
			},
			Insights: []string{"Insight A", "Insight B"},
			ImplicitActions: []activityImplicitActionOutput{
				{Description: "Implicit A", Reasoning: "Inferred", ContextExcerpt: "excerpt"},
			},
			ModelUsed: "gemini-2.0-flash",
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelineAnalyzeOutput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		// Summary, Insights, ModelUsed: exact match
		assert.Equal(t, activity.Summary, workflow.Summary)
		assert.Equal(t, activity.Insights, workflow.Insights)
		assert.Equal(t, activity.ModelUsed, workflow.ModelUsed)

		// TODO: fix mismatch - activity uses concrete types, workflow uses interface{}
		// Activity → Workflow direction: concrete types serialize to JSON, interface{} accepts anything
		// This direction works (concrete → interface{}), but workflow can't access typed fields
		assert.NotNil(t, workflow.Sentiment)
		assert.NotNil(t, workflow.TopicMappings)
		assert.NotNil(t, workflow.VerifiedActions)
		assert.NotNil(t, workflow.VerifiedDecisions)
		assert.NotNil(t, workflow.RiskReferences)
		assert.NotNil(t, workflow.ImplicitActions)

		// Workflow receives JSON but stores as interface{} - can't access fields without type assertion
	})
}

// TestContract_PersistFindings_Input tests JSON round-trip for PersistFindings input types.
func TestContract_PersistFindings_Input(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		threadID := int64(1000)
		projectID := int64(2000)

		analysis := &pipelineAnalyzeOutput{
			Summary:   "Workflow analysis",
			Insights:  []string{"Insight W"},
			ModelUsed: "test-model",
		}

		workflow := pipelinePersistInput{
			TenantID:       "tenant-999",
			SourceID:       1111,
			ThreadID:       &threadID,
			ProjectID:      &projectID,
			Analysis:       analysis,
			ResolvedPeople: map[string]int64{"Kate": 3000, "Leo": 4000},
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityPersistFindingsActivityInput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		// Top-level fields match
		assert.Equal(t, workflow.TenantID, activity.TenantID)
		assert.Equal(t, workflow.SourceID, activity.SourceID)
		assert.Equal(t, workflow.ThreadID, activity.ThreadID)
		assert.Equal(t, workflow.ProjectID, activity.ProjectID)
		assert.Equal(t, workflow.ResolvedPeople, activity.ResolvedPeople)

		// Analysis: workflow uses *pipelineAnalyzeOutput, activity uses *DeepAnalyzeOutput
		// Same mismatch as DeepAnalyze output (workflow uses interface{}, activity uses concrete types)
		assert.NotNil(t, activity.Analysis)
		assert.Equal(t, workflow.Analysis.Summary, activity.Analysis.Summary)
		assert.Equal(t, workflow.Analysis.ModelUsed, activity.Analysis.ModelUsed)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		threadID := int64(5000)
		projectID := int64(6000)

		analysis := &activityDeepAnalyzeOutput{
			Summary:   "Activity analysis",
			Insights:  []string{"Insight A"},
			ModelUsed: "test-model",
		}

		activity := activityPersistFindingsActivityInput{
			TenantID:       "tenant-1000",
			SourceID:       1212,
			ThreadID:       &threadID,
			ProjectID:      &projectID,
			Analysis:       analysis,
			ResolvedPeople: map[string]int64{"Mia": 7000, "Noah": 8000},
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelinePersistInput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		// Top-level fields match
		assert.Equal(t, activity.TenantID, workflow.TenantID)
		assert.Equal(t, activity.SourceID, workflow.SourceID)
		assert.Equal(t, activity.ThreadID, workflow.ThreadID)
		assert.Equal(t, activity.ProjectID, workflow.ProjectID)
		assert.Equal(t, activity.ResolvedPeople, workflow.ResolvedPeople)

		// Analysis: same mismatch
		assert.NotNil(t, workflow.Analysis)
		assert.Equal(t, activity.Analysis.Summary, workflow.Analysis.Summary)
		assert.Equal(t, activity.Analysis.ModelUsed, workflow.Analysis.ModelUsed)
	})
}

// TestContract_PersistFindings_Output tests JSON round-trip for PersistFindings output types.
func TestContract_PersistFindings_Output(t *testing.T) {
	t.Run("WorkflowToActivity", func(t *testing.T) {
		workflow := pipelinePersistOutput{
			AssertionsCreated:    10,
			AssertionsSuperseded: 2,
			ReferencesCreated:    5,
			ReviewItemsCreated:   3,
			AffinityUpdates:      7,
		}

		data, err := json.Marshal(workflow)
		require.NoError(t, err)

		var activity activityPersistFindingsActivityOutput
		err = json.Unmarshal(data, &activity)
		require.NoError(t, err)

		// All fields match exactly
		assert.Equal(t, workflow.AssertionsCreated, activity.AssertionsCreated)
		assert.Equal(t, workflow.AssertionsSuperseded, activity.AssertionsSuperseded)
		assert.Equal(t, workflow.ReferencesCreated, activity.ReferencesCreated)
		assert.Equal(t, workflow.ReviewItemsCreated, activity.ReviewItemsCreated)
		assert.Equal(t, workflow.AffinityUpdates, activity.AffinityUpdates)
	})

	t.Run("ActivityToWorkflow", func(t *testing.T) {
		activity := activityPersistFindingsActivityOutput{
			AssertionsCreated:    15,
			AssertionsSuperseded: 3,
			ReferencesCreated:    8,
			ReviewItemsCreated:   4,
			AffinityUpdates:      9,
		}

		data, err := json.Marshal(activity)
		require.NoError(t, err)

		var workflow pipelinePersistOutput
		err = json.Unmarshal(data, &workflow)
		require.NoError(t, err)

		// All fields match exactly
		assert.Equal(t, activity.AssertionsCreated, workflow.AssertionsCreated)
		assert.Equal(t, activity.AssertionsSuperseded, workflow.AssertionsSuperseded)
		assert.Equal(t, activity.ReferencesCreated, workflow.ReferencesCreated)
		assert.Equal(t, activity.ReviewItemsCreated, workflow.ReviewItemsCreated)
		assert.Equal(t, activity.AffinityUpdates, workflow.AffinityUpdates)
	})
}
