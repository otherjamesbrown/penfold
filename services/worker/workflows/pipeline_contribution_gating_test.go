package workflows

// Tests for per-stage contribution gating (pf-adabbc / pf-bb24a8).
//
// Background: global skipExtract/skipAnalyze was removed (pf-620f23) and replaced with
// per-stage skip_when_low from pipeline_definitions (pf-5f1a20). Each stage independently
// decides whether to run based on its own skip_when_low flag and the content contribution.
//
// Key rule: if skip_when_low=true AND contribution is NONE or LOW → skip that stage.
//           if skip_when_low=false → always run (regardless of contribution).
//
// These tests verify:
//   - Standard pipeline + MEDIUM: all stages run (skip_when_low=true stages ignored)
//   - Newsletter pipeline + LOW: newsletter_extract (skip_when_low=false) still runs
//   - Skip_when_low=false: always runs even when contribution is NONE
//   - Runtime config change: different FetchPipelineDefinition returns produce different behaviour

import (
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestSLMPipeline_ContentContribution_Standard_MEDIUM_RunsAll verifies that MEDIUM
// contribution runs all stages regardless of skip_when_low settings.
// Per-stage gating only gates stages when contribution is NONE or LOW.
func (s *SLMPipelineTestSuite) TestSLMPipeline_ContentContribution_Standard_MEDIUM_RunsAll() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    3001,
		ContentID:   "em-medium-contrib",
		JobID:       "job-3001",
		ContentType: "email",
		BodyText:    "Q3 planning update: budget approved, roadmap confirmed.",
		Subject:     "Q3 Update",
		SenderEmail: "pm@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 3001
	})).Return(&ParseEmailOutput{
		CleanBody:  "Q3 planning update: budget approved, roadmap confirmed.",
		NewContent: "Q3 planning update: budget approved, roadmap confirmed.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)

	// Stage 1: Triage - returns MEDIUM contribution
	// standardTestPipelineDef has extract_ner, analyze with skip_when_low=true.
	// MEDIUM contribution must NOT gate any of them.
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 3001
	})).Return(&TriageOutput{
		Category:            "PROJECT_UPDATE",
		Importance:          "MEDIUM",
		Reason:              "Budget and roadmap update",
		ModelUsed:           "llama-3.2-1b",
		SkipDeep:            false,
		ContentContribution: "MEDIUM",
		ContributionReason:  "Useful project context",
	}, nil)

	// Stage 2: Extract - MUST run for MEDIUM contribution (skip_when_low=true is irrelevant for MEDIUM)
	personID := int64(3001)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.MatchedBy(func(in SLMPipelineExtractEntitiesInput) bool {
		return in.SourceID == 3001
	})).Return(&SLMPipelineExtractEntitiesOutput{
		People: []PersonResult{{Name: "PM", Role: "project_manager"}},
	}, nil)

	// Stage 3: Context - MUST run for MEDIUM contribution
	s.activities.On("BuildContextPackage", mock.Anything, mock.MatchedBy(func(in BuildContextInput) bool {
		return in.SourceID == 3001
	})).Return(&BuildContextOutput{
		ResolvedPeople:   []ResolvedPerson{{Name: "PM", PersonID: &personID, Confidence: 0.8, Source: "fuzzy_match"}},
		EntitiesResolved: 1,
	}, nil)

	// Stage 4: Deep Analysis - MUST run for MEDIUM (skip_when_low=true only gates NONE/LOW)
	s.activities.On("DeepAnalyze", mock.Anything, mock.MatchedBy(func(in DeepAnalyzeInput) bool {
		return in.SourceID == 3001
	})).Return(&DeepAnalyzeOutput{
		Summary:   "Q3 planning update",
		ModelUsed: "gemini-2.0-flash",
	}, nil)

	// Stage 4.5: Persist - MUST run for MEDIUM
	s.activities.On("PersistFindings", mock.Anything, mock.MatchedBy(func(in PersistFindingsInput) bool {
		return in.SourceID == 3001 && in.Analysis != nil
	})).Return(&PersistFindingsOutput{
		AssertionsCreated: 2,
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 3001
	})).Return(int64(3001), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "completed", result.Status)

	// KEY ASSERTION: all stages must have been called for MEDIUM contribution.
	s.activities.AssertCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)
	s.activities.AssertCalled(s.T(), "DeepAnalyze", mock.Anything, mock.Anything)
	s.activities.AssertCalled(s.T(), "PersistFindings", mock.Anything, mock.Anything)
}

// TestSLMPipeline_Newsletter_LOW_RunsExtract verifies that the newsletter pipeline runs
// newsletter_extract even when content contribution is LOW.
// The newsletter pipeline sets skip_when_low=false on newsletter_extract, which means
// per-stage gating never fires for that stage — regardless of contribution level.
// This is the primary use case enabled by per-stage gating (pf-bb24a8 design goal).
func (s *SLMPipelineTestSuite) TestSLMPipeline_Newsletter_LOW_RunsExtract() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    3002,
		ContentID:   "em-nl-low",
		JobID:       "job-3002",
		ContentType: "email",
		BodyText:    "Weekly digest: a few industry links, nothing actionable.",
		Subject:     "Weekly Digest",
		SenderEmail: "digest@newsletter.example.com",
		Pipeline:    "newsletter",
	}

	// Override the default FetchPipelineDefinition mock to return the newsletter pipeline.
	// newsletter_extract has skip_when_low=false — it always runs.
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Unset()
	newsletterDef := &FetchPipelineDefinitionOutput{
		Found:       true,
		ContentType: "email",
		Stages: []PipelineStageConfig{
			{Stage: "parse", StageOrder: 0, Enabled: true, SkipWhenLow: false},
			{Stage: "triage", StageOrder: 1, Enabled: true, SkipWhenLow: false},
			{
				Stage:       "newsletter_extract",
				StageKind:   "structured_extract",
				PersistKey:  "newsletter",
				StageOrder:  2,
				Enabled:     true,
				SkipWhenLow: false, // always runs — even on LOW contribution
			},
			{Stage: "embed", StageOrder: 3, Enabled: true, SkipWhenLow: false},
		},
	}
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Return(newsletterDef, nil)

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 3002
	})).Return(&ParseEmailOutput{
		CleanBody:  "Weekly digest: a few industry links, nothing actionable.",
		NewContent: "Weekly digest: a few industry links, nothing actionable.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)

	// Stage 1: Triage - returns LOW contribution but routes to newsletter pipeline
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 3002
	})).Return(&TriageOutput{
		Category:            "NEWSLETTER",
		Importance:          "LOW",
		Reason:              "Weekly digest, generic content",
		ModelUsed:           "llama-3.2-1b",
		SkipDeep:            false,
		ContentContribution: "LOW",
		ContributionReason:  "Digest with no actionable items",
		RoutingContentType:  "EMAIL",
		RoutingSubtype:      "NEWSLETTER",
		Pipelines:           []string{"newsletter"},
	}, nil)

	// BuildExtractionContext and BuildNewsletterContext run before StructuredExtract.
	s.activities.On("BuildExtractionContext", mock.Anything, mock.Anything).Maybe().Return(&BuildExtractionContextOutput{
		BackgroundContext: "email context",
	}, nil)
	s.activities.On("BuildNewsletterContext", mock.Anything, mock.Anything).Maybe().Return(&BuildNewsletterContextOutput{
		BackgroundContext: "newsletter context",
		UserContextFound:  true,
	}, nil)

	// KEY: StructuredExtract MUST be called — skip_when_low=false means LOW contribution
	// does NOT gate newsletter_extract.
	s.activities.On("StructuredExtract", mock.Anything, mock.MatchedBy(func(in StructuredExtractInput) bool {
		return in.SourceID == 3002 && in.StageName == "newsletter_extract"
	})).Return(&StructuredExtractOutput{
		ModelUsed: "gemini-2.0-flash",
		StageName: "newsletter_extract",
	}, nil)

	s.activities.On("PersistExtractedData", mock.Anything, mock.MatchedBy(func(in PersistExtractedDataInput) bool {
		return in.SourceID == 3002 && in.Key == "newsletter"
	})).Return(&PersistExtractedDataOutput{Updated: true}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 3002
	})).Return(int64(3002), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "completed", result.Status)

	// KEY ASSERTION: newsletter_extract must have been called despite LOW contribution.
	s.activities.AssertCalled(s.T(), "StructuredExtract", mock.Anything, mock.MatchedBy(func(in StructuredExtractInput) bool {
		return in.StageName == "newsletter_extract"
	}))
}

// TestSLMPipeline_PerStageGating_SkipWhenLowFalse_NONE_StillRuns verifies that a stage
// with skip_when_low=false runs even when content contribution is NONE.
// This is the fundamental property: skip_when_low=false is an unconditional opt-out from gating.
func (s *SLMPipelineTestSuite) TestSLMPipeline_PerStageGating_SkipWhenLowFalse_NONE_StillRuns() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    3003,
		ContentID:   "em-none-skip-false",
		JobID:       "job-3003",
		ContentType: "email",
		BodyText:    "Thanks",
		Subject:     "Re: meeting",
		SenderEmail: "sender@example.com",
	}

	// Custom pipeline: extract_ner has skip_when_low=false. Even NONE contribution must run it.
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Unset()
	customDef := &FetchPipelineDefinitionOutput{
		Found:       true,
		ContentType: "email",
		Stages: []PipelineStageConfig{
			{Stage: "parse", StageOrder: 0, Enabled: true},
			{Stage: "triage", StageOrder: 1, Enabled: true},
			{Stage: "extract_ner", StageOrder: 2, Enabled: true, SkipWhenLow: false}, // never skip
			{Stage: "persist", StageOrder: 3, Enabled: true, SkipWhenLow: false},
			{Stage: "embed", StageOrder: 4, Enabled: true},
		},
	}
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Return(customDef, nil)

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 3003
	})).Return(&ParseEmailOutput{
		CleanBody:  "Thanks",
		NewContent: "Thanks",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)

	// Stage 1: Triage - returns NONE contribution
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 3003
	})).Return(&TriageOutput{
		Category:            "PERSONAL",
		Importance:          "LOW",
		Reason:              "Simple acknowledgement",
		ModelUsed:           "llama-3.2-1b",
		SkipDeep:            false,
		ContentContribution: "NONE",
		ContributionReason:  "No actionable content",
	}, nil)

	// Stage 2: Extract - MUST run because skip_when_low=false (even with NONE contribution)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.MatchedBy(func(in SLMPipelineExtractEntitiesInput) bool {
		return in.SourceID == 3003
	})).Return(&SLMPipelineExtractEntitiesOutput{
		People: []PersonResult{},
	}, nil)

	// Persist (skip_when_low=false) must also run
	s.activities.On("PersistFindings", mock.Anything, mock.MatchedBy(func(in PersistFindingsInput) bool {
		return in.SourceID == 3003
	})).Return(&PersistFindingsOutput{
		AssertionsCreated: 0,
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 3003
	})).Return(int64(3003), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "completed", result.Status)

	// KEY ASSERTION: extract_ner must have run despite NONE contribution, because skip_when_low=false.
	s.activities.AssertCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)

	// Analyze was not in the pipeline, so it must not be called.
	s.activities.AssertNotCalled(s.T(), "DeepAnalyze", mock.Anything, mock.Anything)
}

// TestSLMPipeline_PerStageGating_RuntimeConfigChange verifies that changing skip_when_low
// in pipeline_definitions changes gating behaviour without any code change.
//
// The test uses two sub-scenarios sharing the same contribution level (LOW) but different
// pipeline definitions — one with skip_when_low=true on extract_ner (gates it) and one
// with skip_when_low=false (passes it through). This simulates a DB-level config change.
//
// Note: each sub-scenario runs in its own workflow execution via a fresh test environment.
// The test structure demonstrates that FetchPipelineDefinition's return value is the sole
// driver of per-stage gating — no code change is needed.
func (s *SLMPipelineTestSuite) TestSLMPipeline_PerStageGating_RuntimeConfigChange() {
	baseInput := PipelineInput{
		TenantID:    "tenant-cfg",
		SourceID:    3004,
		ContentID:   "em-cfg-change",
		JobID:       "job-3004",
		ContentType: "email",
		BodyText:    "Routine status update, nothing critical.",
		Subject:     "Status Update",
		SenderEmail: "ops@example.com",
	}

	triageResult := &TriageOutput{
		Category:            "INTERNAL_COMMS",
		Importance:          "LOW",
		Reason:              "Routine update",
		ModelUsed:           "llama-3.2-1b",
		SkipDeep:            false,
		ContentContribution: "LOW",
		ContributionReason:  "Generic status update",
	}

	// Scenario A: extract_ner has skip_when_low=true → gated on LOW contribution.
	s.Run("SkipWhenLow_True_GatesExtractOnLow", func() {
		s.SetupTest() // fresh environment for each sub-test

		defSkipEnabled := &FetchPipelineDefinitionOutput{
			Found:       true,
			ContentType: "email",
			Stages: []PipelineStageConfig{
				{Stage: "parse", StageOrder: 0, Enabled: true},
				{Stage: "triage", StageOrder: 1, Enabled: true},
				{Stage: "extract_ner", StageOrder: 2, Enabled: true, SkipWhenLow: true}, // DB config: gate on LOW
				{Stage: "persist", StageOrder: 3, Enabled: true, SkipWhenLow: true},
				{Stage: "embed", StageOrder: 4, Enabled: true},
			},
		}
		s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Unset()
		s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Return(defSkipEnabled, nil)

		s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
			CleanBody: "Routine status update, nothing critical.",
		}, nil)
		s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)
		s.activities.On("Triage", mock.Anything, mock.Anything).Return(triageResult, nil)
		s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(3004), nil)

		s.env.ExecuteWorkflow(SLMPipelineWorkflow, baseInput)

		require.True(s.T(), s.env.IsWorkflowCompleted())
		require.NoError(s.T(), s.env.GetWorkflowError())

		// extract_ner must NOT have been called (skip_when_low=true, contribution=LOW)
		s.activities.AssertNotCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)
	})

	// Scenario B: same content, same contribution — but skip_when_low=false in DB.
	// Simulates a DB update that changes the gating policy without a code change.
	s.Run("SkipWhenLow_False_RunsExtractOnLow", func() {
		s.SetupTest() // fresh environment

		defSkipDisabled := &FetchPipelineDefinitionOutput{
			Found:       true,
			ContentType: "email",
			Stages: []PipelineStageConfig{
				{Stage: "parse", StageOrder: 0, Enabled: true},
				{Stage: "triage", StageOrder: 1, Enabled: true},
				{Stage: "extract_ner", StageOrder: 2, Enabled: true, SkipWhenLow: false}, // DB config changed: always run
				{Stage: "persist", StageOrder: 3, Enabled: true, SkipWhenLow: false},
				{Stage: "embed", StageOrder: 4, Enabled: true},
			},
		}
		s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Unset()
		s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Return(defSkipDisabled, nil)

		s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
			CleanBody: "Routine status update, nothing critical.",
		}, nil)
		s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)
		s.activities.On("Triage", mock.Anything, mock.Anything).Return(triageResult, nil)

		// extract_ner must now run (skip_when_low=false — DB config changed)
		s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
			People: []PersonResult{},
		}, nil)
		s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)
		s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(3004), nil)

		s.env.ExecuteWorkflow(SLMPipelineWorkflow, baseInput)

		require.True(s.T(), s.env.IsWorkflowCompleted())
		require.NoError(s.T(), s.env.GetWorkflowError())

		// KEY ASSERTION: extract_ner must have run (skip_when_low=false overrides contribution gate)
		s.activities.AssertCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)
	})
}
