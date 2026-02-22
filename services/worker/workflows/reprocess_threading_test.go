package workflows

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// ReprocessMockActivities provides mock implementations for activities needed in reprocess test.
type ReprocessMockActivities struct {
	mock.Mock
}

func (m *ReprocessMockActivities) FetchSource(ctx context.Context, input FetchSourceInput) (*FetchSourceOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FetchSourceOutput), args.Error(1)
}

func (m *ReprocessMockActivities) ParseEmail(ctx context.Context, input ParseEmailInput) (*ParseEmailOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ParseEmailOutput), args.Error(1)
}

func (m *ReprocessMockActivities) ParseTranscript(ctx context.Context, input ParseTranscriptInput) (*ParseTranscriptOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ParseTranscriptOutput), args.Error(1)
}

func (m *ReprocessMockActivities) Triage(ctx context.Context, input TriageInput) (*TriageOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*TriageOutput), args.Error(1)
}

func (m *ReprocessMockActivities) GroupEmailThread(ctx context.Context, input GroupEmailThreadInput) (*GroupEmailThreadOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*GroupEmailThreadOutput), args.Error(1)
}

func (m *ReprocessMockActivities) LinkConversation(ctx context.Context, input LinkConversationInput) (*LinkConversationOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LinkConversationOutput), args.Error(1)
}

func (m *ReprocessMockActivities) GenerateContentEmbedding(ctx context.Context, input GenerateEmbeddingInput) (int64, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int64), args.Error(1)
}

func (m *ReprocessMockActivities) UpdateContentStatus(ctx context.Context, input UpdateContentStatusInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func (m *ReprocessMockActivities) CreateEnrichmentRecord(ctx context.Context, input CreateEnrichmentRecordInput) (*CreateEnrichmentRecordOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*CreateEnrichmentRecordOutput), args.Error(1)
}

func (m *ReprocessMockActivities) KickNextPending(ctx context.Context, input KickNextPendingInput) (*KickNextPendingOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*KickNextPendingOutput), args.Error(1)
}

// TestReprocessThreadingEmailPath verifies that ThreadGrouper (GroupEmailThread activity)
// is called during reprocessing for email content.
//
// Bug: pf-745f70 reported that ThreadGrouper is not running during reprocess.
//
// This test simulates the reprocess path:
// 1. Workflow starts with minimal SLMPipelineInput (only SourceID, TenantID, JobID)
// 2. FetchContent activity populates ContentType="email" from database
// 3. ThreadGrouper (GroupEmailThread) SHOULD run at Stage 2.5 (lines 1101-1123)
// 4. ThreadGrouper runs BEFORE the skipExtract gate (independent of SkipDeep)
//
// Expected: GroupEmailThread activity is called for email content during reprocess
// Actual (bug): Test FAILS if GroupEmailThread is NOT called
func TestReprocessThreadingEmailPath(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	activities := &ReprocessMockActivities{}

	// Register activities
	env.RegisterActivityWithOptions(activities.FetchSource, activity.RegisterOptions{Name: pkgtemporal.ActivityFetchContent})
	env.RegisterActivityWithOptions(activities.ParseEmail, activity.RegisterOptions{Name: pkgtemporal.ActivityParseEmail})
	env.RegisterActivityWithOptions(activities.Triage, activity.RegisterOptions{Name: pkgtemporal.ActivityTriage})
	env.RegisterActivityWithOptions(activities.GroupEmailThread, activity.RegisterOptions{Name: pkgtemporal.ActivityGroupEmailThread})
	env.RegisterActivityWithOptions(activities.LinkConversation, activity.RegisterOptions{Name: pkgtemporal.ActivityLinkConversation})
	env.RegisterActivityWithOptions(activities.GenerateContentEmbedding, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateContentEmbedding})
	env.RegisterActivityWithOptions(activities.UpdateContentStatus, activity.RegisterOptions{Name: pkgtemporal.ActivityUpdateContentStatus})
	env.RegisterActivityWithOptions(activities.CreateEnrichmentRecord, activity.RegisterOptions{Name: pkgtemporal.ActivityCreateEnrichmentRecord})
	env.RegisterActivityWithOptions(activities.KickNextPending, activity.RegisterOptions{Name: pkgtemporal.ActivityKickNextPending})

	// Stage -1: FetchContent (minimal input path — simulates reprocess)
	activities.On("FetchSource", mock.Anything, mock.Anything).
		Return(&FetchSourceOutput{
			ContentType:       "email",
			ContentText:       "Test email body",
			Subject:           "Test Subject",
			SenderEmail:       "sender@example.com",
			SenderName:        "Test Sender",
			ParticipantEmails: []Participant{},
		}, nil)

	// Stage 0: ParseEmail
	activities.On("ParseEmail", mock.Anything, mock.Anything).
		Return(&ParseEmailOutput{
			CleanBody:     "Clean test email body",
			NewContent:    "New content",
			QuotedContent: "",
			IsReply:       false,
		}, nil)

	// Stage 0.6: UpdateContentStatus (parsed)
	activities.On("UpdateContentStatus", mock.Anything, mock.Anything).
		Return(nil)

	// Stage 1: Triage
	activities.On("Triage", mock.Anything, mock.Anything).
		Return(&TriageOutput{
			Category:            "INTERNAL_COMMS",
			Importance:          "MEDIUM",
			Reason:              "Test triage",
			Confidence:          0.95,
			ModelUsed:           "test-model",
			SkipDeep:            true, // Skip deep processing (realistic reprocess scenario)
			ContentSubtype:      "email_standalone",
			ContentContribution: "NONE", // Low contribution — skip extract/analyze
			ContributionReason:  "Test",
		}, nil)

	// Stage 1.5: CreateEnrichmentRecord
	activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).
		Return(&CreateEnrichmentRecordOutput{
			EnrichmentID: 123,
		}, nil)

	// Stage 2.5: GroupEmailThread (THE CRITICAL ACTIVITY FOR THIS BUG)
	// This SHOULD be called for ALL emails, regardless of SkipDeep or contribution
	activities.On("GroupEmailThread", mock.Anything, mock.Anything).
		Return(&GroupEmailThreadOutput{
			ThreadID: stringPtr("thread-123"),
		}, nil)

	// Stage 2.6: LinkConversation (runs if ThreadID exists)
	activities.On("LinkConversation", mock.Anything, mock.Anything).
		Return(&LinkConversationOutput{
			ConversationID: "conv-456",
		}, nil)

	// Stage 5: GenerateEmbedding
	activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).
		Return(int64(789), nil)

	// Stage 6: KickNextPending (auto-drain)
	activities.On("KickNextPending", mock.Anything, mock.Anything).
		Return(&KickNextPendingOutput{
			QueuedCount: 0,
			Message:     "No pending items",
		}, nil)

	// Execute workflow with minimal input (simulates reprocess path)
	// The workflow will call FetchContent to populate ContentType from database
	input := PipelineInput{
		TenantID:    "test-tenant",
		SourceID:    100,
		ContentID:   "em-test123",
		JobID:       "job-reprocess-test",
		ContentType: "", // Empty — triggers FetchContent (reprocess path)
	}

	env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(t, env.IsWorkflowCompleted(), "workflow should complete")
	require.NoError(t, env.GetWorkflowError(), "workflow should not error")

	// Verify stage progression
	activities.AssertCalled(t, "FetchSource", mock.Anything, mock.Anything)
	activities.AssertCalled(t, "ParseEmail", mock.Anything, mock.Anything)
	activities.AssertCalled(t, "Triage", mock.Anything, mock.Anything)
	activities.AssertCalled(t, "GenerateContentEmbedding", mock.Anything, mock.Anything)

	// THE CRITICAL ASSERTION FOR BUG pf-745f70
	// GroupEmailThread MUST be called for emails during reprocess
	// This runs at Stage 2.5 (lines 1101-1123 in pipeline.go)
	// It is gated ONLY by `if input.ContentType == "email"` — NOT by SkipDeep or contribution
	activities.AssertCalled(t, "GroupEmailThread", mock.Anything, mock.Anything)

	// Verify result
	var result PipelineResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status, "pipeline should complete successfully")
}

// TestReprocessThreadingMeetingPath verifies that ThreadGrouper is NOT called for meeting content.
// This is a control test to ensure ThreadGrouper is email-specific.
func TestReprocessThreadingMeetingPath(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	activities := &ReprocessMockActivities{}

	// Register activities
	env.RegisterActivityWithOptions(activities.FetchSource, activity.RegisterOptions{Name: pkgtemporal.ActivityFetchContent})
	env.RegisterActivityWithOptions(activities.ParseTranscript, activity.RegisterOptions{Name: pkgtemporal.ActivityParseTranscript})
	env.RegisterActivityWithOptions(activities.Triage, activity.RegisterOptions{Name: pkgtemporal.ActivityTriage})
	env.RegisterActivityWithOptions(activities.GroupEmailThread, activity.RegisterOptions{Name: pkgtemporal.ActivityGroupEmailThread})
	env.RegisterActivityWithOptions(activities.GenerateContentEmbedding, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateContentEmbedding})
	env.RegisterActivityWithOptions(activities.UpdateContentStatus, activity.RegisterOptions{Name: pkgtemporal.ActivityUpdateContentStatus})
	env.RegisterActivityWithOptions(activities.CreateEnrichmentRecord, activity.RegisterOptions{Name: pkgtemporal.ActivityCreateEnrichmentRecord})
	env.RegisterActivityWithOptions(activities.KickNextPending, activity.RegisterOptions{Name: pkgtemporal.ActivityKickNextPending})

	// Stage -1: FetchContent returns meeting content
	activities.On("FetchSource", mock.Anything, mock.Anything).
		Return(&FetchSourceOutput{
			ContentType: "meeting", // Not email
			ContentText: "Meeting transcript",
		}, nil)

	// Stage 0: ParseTranscript
	activities.On("ParseTranscript", mock.Anything, mock.Anything).
		Return(&ParseTranscriptOutput{
			CleanText:  "Clean transcript",
			Speakers:   []string{"Speaker1"},
			DurationMs: 3600000,
			Format:     "text",
		}, nil)
	activities.On("UpdateContentStatus", mock.Anything, mock.Anything).
		Return(nil)

	// Stage 1: Triage
	activities.On("Triage", mock.Anything, mock.Anything).
		Return(&TriageOutput{
			Category:            "INTERNAL_COMMS",
			Importance:          "MEDIUM",
			Reason:              "Test",
			Confidence:          0.95,
			ModelUsed:           "test-model",
			SkipDeep:            true,
			ContentContribution: "NONE",
			ContributionReason:  "Test",
		}, nil)

	activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).
		Return(&CreateEnrichmentRecordOutput{EnrichmentID: 123}, nil)

	// Stage 2.5: GroupEmailThread should NOT be called for meetings
	activities.On("GroupEmailThread", mock.Anything, mock.Anything).
		Return(&GroupEmailThreadOutput{}, nil)

	// Stage 5: Embedding
	activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).
		Return(int64(789), nil)

	activities.On("KickNextPending", mock.Anything, mock.Anything).
		Return(&KickNextPendingOutput{}, nil)

	input := PipelineInput{
		TenantID:    "test-tenant",
		SourceID:    200,
		ContentID:   "mt-test456",
		JobID:       "job-meeting-test",
		ContentType: "", // Empty — triggers FetchContent
	}

	env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// GroupEmailThread should NOT be called for meeting content
	activities.AssertNotCalled(t, "GroupEmailThread", mock.Anything, mock.Anything)
}

// TestReprocessThreadingContentID_BUG_pf_3418d4 verifies that when the pipeline
// runs via the FetchContent path (ContentType empty, e.g. reprocess), ContentID
// is populated from the DB so that conversation linking can proceed.
//
// Bug: pf-3418d4 — emails with contribution=NONE get a thread_id but skip
// conversation linking because input.ContentID was empty.
func TestReprocessThreadingContentID_BUG_pf_3418d4(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	activities := &ReprocessMockActivities{}

	// Register activities
	env.RegisterActivityWithOptions(activities.FetchSource, activity.RegisterOptions{Name: pkgtemporal.ActivityFetchContent})
	env.RegisterActivityWithOptions(activities.ParseEmail, activity.RegisterOptions{Name: pkgtemporal.ActivityParseEmail})
	env.RegisterActivityWithOptions(activities.Triage, activity.RegisterOptions{Name: pkgtemporal.ActivityTriage})
	env.RegisterActivityWithOptions(activities.GroupEmailThread, activity.RegisterOptions{Name: pkgtemporal.ActivityGroupEmailThread})
	env.RegisterActivityWithOptions(activities.LinkConversation, activity.RegisterOptions{Name: pkgtemporal.ActivityLinkConversation})
	env.RegisterActivityWithOptions(activities.GenerateContentEmbedding, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateContentEmbedding})
	env.RegisterActivityWithOptions(activities.UpdateContentStatus, activity.RegisterOptions{Name: pkgtemporal.ActivityUpdateContentStatus})
	env.RegisterActivityWithOptions(activities.CreateEnrichmentRecord, activity.RegisterOptions{Name: pkgtemporal.ActivityCreateEnrichmentRecord})
	env.RegisterActivityWithOptions(activities.KickNextPending, activity.RegisterOptions{Name: pkgtemporal.ActivityKickNextPending})

	// FetchContent returns ContentID from DB
	activities.On("FetchSource", mock.Anything, mock.Anything).
		Return(&FetchSourceOutput{
			ContentType: "email",
			ContentID:   "em-fromDB123", // pf-3418d4: ContentID now returned by FetchSource
			ContentText: "Test email body",
			Subject:     "Re: GPU requirements",
			SenderEmail: "sender@example.com",
		}, nil)

	activities.On("ParseEmail", mock.Anything, mock.Anything).
		Return(&ParseEmailOutput{
			CleanBody:  "Clean body",
			NewContent: "New content",
		}, nil)

	activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	activities.On("Triage", mock.Anything, mock.Anything).
		Return(&TriageOutput{
			Category:            "INTERNAL_COMMS",
			Importance:          "LOW",
			Reason:              "Test",
			Confidence:          0.9,
			ModelUsed:           "test",
			SkipDeep:            false,
			ContentContribution: "NONE", // NONE contribution — this is the key scenario
			ContributionReason:  "Duplicate content",
		}, nil)

	activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).
		Return(&CreateEnrichmentRecordOutput{EnrichmentID: 456}, nil)

	activities.On("GroupEmailThread", mock.Anything, mock.Anything).
		Return(&GroupEmailThreadOutput{
			ThreadID: stringPtr("thread-gpu-root"),
		}, nil)

	// THE CRITICAL ASSERTION: LinkConversation MUST be called with the ContentID
	// that was fetched from the DB via FetchSource, even though the pipeline
	// was started with empty ContentID.
	linkCalled := false
	activities.On("LinkConversation", mock.Anything, mock.MatchedBy(func(input LinkConversationInput) bool {
		linkCalled = true
		return input.ContentID == "em-fromDB123" && input.ThreadID == "thread-gpu-root"
	})).Return(&LinkConversationOutput{
		ConversationID: "conv-test789",
	}, nil)

	activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).
		Return(int64(101), nil)

	activities.On("KickNextPending", mock.Anything, mock.Anything).
		Return(&KickNextPendingOutput{}, nil)

	// Start pipeline with EMPTY ContentID (simulates reprocess or old dispatch)
	input := PipelineInput{
		TenantID:    "test-tenant",
		SourceID:    3401,
		ContentID:   "", // Empty — this is the bug scenario
		JobID:       "job-pf-3418d4-test",
		ContentType: "", // Triggers FetchContent path
	}

	env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(t, env.IsWorkflowCompleted(), "workflow should complete")
	require.NoError(t, env.GetWorkflowError(), "workflow should not error")

	// Verify LinkConversation was called (not skipped due to empty ContentID)
	require.True(t, linkCalled,
		"BUG pf-3418d4: LinkConversation should be called even when pipeline started with empty ContentID")

	var result PipelineResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status)
}

// stringPtr is a helper to create string pointers for test data
func stringPtr(s string) *string {
	return &s
}
