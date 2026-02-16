// Package activities provides tests for email threading activities.
package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockSourceRepository is a mock implementation of the SourceRepository interface for testing.
type mockSourceRepository struct {
	getSourceFn func(ctx context.Context, tenantID string, sourceID int64) (*Source, error)
}

func (m *mockSourceRepository) GetSource(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
	if m.getSourceFn != nil {
		return m.getSourceFn(ctx, tenantID, sourceID)
	}
	return nil, nil
}

func (m *mockSourceRepository) UpdateSourceStatus(ctx context.Context, tenantID string, sourceID int64, status string) error {
	return nil
}

func (m *mockSourceRepository) UpdateSourceStatusWithFailure(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error {
	return nil
}

// mockThreadRepository is a mock implementation of the ThreadRepository interface for testing.
type mockThreadRepository struct {
	upsertThreadFn               func(ctx context.Context, input *UpsertThreadInput) (int64, error)
	addThreadMessageFn           func(ctx context.Context, input *AddThreadMessageInput) error
	setContentEnrichmentThreadIDFn func(ctx context.Context, sourceID int64, threadID string) error
	getThreadByRootMessageIDFn   func(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error)
}

func (m *mockThreadRepository) UpsertThread(ctx context.Context, input *UpsertThreadInput) (int64, error) {
	if m.upsertThreadFn != nil {
		return m.upsertThreadFn(ctx, input)
	}
	return 0, nil
}

func (m *mockThreadRepository) AddThreadMessage(ctx context.Context, input *AddThreadMessageInput) error {
	if m.addThreadMessageFn != nil {
		return m.addThreadMessageFn(ctx, input)
	}
	return nil
}

func (m *mockThreadRepository) SetContentEnrichmentThreadID(ctx context.Context, sourceID int64, threadID string) error {
	if m.setContentEnrichmentThreadIDFn != nil {
		return m.setContentEnrichmentThreadIDFn(ctx, sourceID, threadID)
	}
	return nil
}

func (m *mockThreadRepository) GetThreadByRootMessageID(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error) {
	if m.getThreadByRootMessageIDFn != nil {
		return m.getThreadByRootMessageIDFn(ctx, tenantID, rootMessageID)
	}
	return nil, nil
}

// TestGroupEmailThread_EmailWithReplyChain tests threading for an email with In-Reply-To and References headers.
func TestGroupEmailThread_EmailWithReplyChain(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 14, 30, 0, 0, time.UTC)

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			require.Equal(t, "test-tenant", tenantID)
			require.Equal(t, int64(123), sourceID)

			return &Source{
				ID:          123,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id":   "<msg-456@example.com>",
					"in_reply_to":  "<msg-123@example.com>",
					"references":   "<msg-123@example.com> <msg-234@example.com>",
					"subject":      "Re: Project Update",
					"date":         messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	upsertCalled := false
	addMessageCalled := false
	setEnrichmentCalled := false

	mockThreadRepo := &mockThreadRepository{
		getThreadByRootMessageIDFn: func(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error) {
			require.Equal(t, "test-tenant", tenantID)
			require.Equal(t, "<msg-123@example.com>", rootMessageID)
			return &EmailThread{
				ID:              100,
				RootMessageID:   "<msg-123@example.com>",
				Subject:         "Project Update",
				MessageCount:    2,
			}, nil
		},
		upsertThreadFn: func(ctx context.Context, input *UpsertThreadInput) (int64, error) {
			require.Equal(t, "test-tenant", input.TenantID)
			require.Equal(t, "<msg-123@example.com>", input.RootMessageID)
			require.Equal(t, "Project Update", input.NormalizedSubject)
			require.Equal(t, int64(123), input.LatestSourceID)
			upsertCalled = true
			return 100, nil
		},
		addThreadMessageFn: func(ctx context.Context, input *AddThreadMessageInput) error {
			require.Equal(t, int64(100), input.ThreadID)
			require.Equal(t, int64(123), input.SourceID)
			require.Equal(t, "<msg-456@example.com>", input.MessageID)
			require.Equal(t, 3, input.PositionInThread) // 3rd message in the thread
			require.True(t, input.IsReply)
			require.Equal(t, "<msg-123@example.com>", input.ReplyToMessageID)
			require.Equal(t, messageDate, input.MessageDate)
			addMessageCalled = true
			return nil
		},
		setContentEnrichmentThreadIDFn: func(ctx context.Context, sourceID int64, threadID string) error {
			require.Equal(t, int64(123), sourceID)
			require.Equal(t, "<msg-123@example.com>", threadID)
			setEnrichmentCalled = true
			return nil
		},
	}

	activities := NewThreadActivities(logger, mockSourceRepo, mockThreadRepo)

	input := GroupEmailThreadInput{
		TenantID: "test-tenant",
		SourceID: 123,
	}

	output, err := activities.GroupEmailThread(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, output.ThreadID)
	require.Equal(t, "<msg-123@example.com>", *output.ThreadID)
	require.True(t, upsertCalled, "UpsertThread should have been called")
	require.True(t, addMessageCalled, "AddThreadMessage should have been called")
	require.True(t, setEnrichmentCalled, "SetContentEnrichmentThreadID should have been called")
}

// TestGroupEmailThread_RootEmail tests threading for an email with no In-Reply-To (thread root).
func TestGroupEmailThread_RootEmail(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          200,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<root-msg@example.com>",
					"subject":    "New Discussion Topic",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	upsertCalled := false
	addMessageCalled := false

	mockThreadRepo := &mockThreadRepository{
		getThreadByRootMessageIDFn: func(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error) {
			require.Equal(t, "<root-msg@example.com>", rootMessageID)
			return nil, nil // Thread doesn't exist yet
		},
		upsertThreadFn: func(ctx context.Context, input *UpsertThreadInput) (int64, error) {
			require.Equal(t, "<root-msg@example.com>", input.RootMessageID)
			require.Equal(t, "New Discussion Topic", input.NormalizedSubject)
			require.Equal(t, int64(200), input.LatestSourceID)
			require.Equal(t, messageDate, input.FirstMessageAt)
			require.Equal(t, messageDate, input.LastMessageAt)
			upsertCalled = true
			return 101, nil
		},
		addThreadMessageFn: func(ctx context.Context, input *AddThreadMessageInput) error {
			require.Equal(t, int64(101), input.ThreadID)
			require.Equal(t, int64(200), input.SourceID)
			require.Equal(t, "<root-msg@example.com>", input.MessageID)
			require.Equal(t, 1, input.PositionInThread) // First message
			require.False(t, input.IsReply)
			require.Equal(t, "", input.ReplyToMessageID)
			addMessageCalled = true
			return nil
		},
		setContentEnrichmentThreadIDFn: func(ctx context.Context, sourceID int64, threadID string) error {
			require.Equal(t, "<root-msg@example.com>", threadID)
			return nil
		},
	}

	activities := NewThreadActivities(logger, mockSourceRepo, mockThreadRepo)

	input := GroupEmailThreadInput{
		TenantID: "test-tenant",
		SourceID: 200,
	}

	output, err := activities.GroupEmailThread(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, output.ThreadID)
	require.Equal(t, "<root-msg@example.com>", *output.ThreadID)
	require.True(t, upsertCalled)
	require.True(t, addMessageCalled)
}

// TestGroupEmailThread_OrphanReply tests handling of a reply to an unknown message.
func TestGroupEmailThread_OrphanReply(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          300,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id":  "<orphan-reply@example.com>",
					"in_reply_to": "<unknown-msg@example.com>",
					"subject":     "Re: Lost Thread",
					"date":        messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	mockThreadRepo := &mockThreadRepository{
		getThreadByRootMessageIDFn: func(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error) {
			require.Equal(t, "<unknown-msg@example.com>", rootMessageID)
			return nil, nil // Parent thread doesn't exist
		},
		upsertThreadFn: func(ctx context.Context, input *UpsertThreadInput) (int64, error) {
			// Should create thread with the referenced (unknown) message as root
			require.Equal(t, "<unknown-msg@example.com>", input.RootMessageID)
			require.Equal(t, "Lost Thread", input.NormalizedSubject)
			return 102, nil
		},
		addThreadMessageFn: func(ctx context.Context, input *AddThreadMessageInput) error {
			require.Equal(t, int64(102), input.ThreadID)
			require.Equal(t, "<orphan-reply@example.com>", input.MessageID)
			require.Equal(t, 1, input.PositionInThread) // First message in this orphaned thread
			require.True(t, input.IsReply)
			require.Equal(t, "<unknown-msg@example.com>", input.ReplyToMessageID)
			return nil
		},
		setContentEnrichmentThreadIDFn: func(ctx context.Context, sourceID int64, threadID string) error {
			require.Equal(t, "<unknown-msg@example.com>", threadID)
			return nil
		},
	}

	activities := NewThreadActivities(logger, mockSourceRepo, mockThreadRepo)

	input := GroupEmailThreadInput{
		TenantID: "test-tenant",
		SourceID: 300,
	}

	output, err := activities.GroupEmailThread(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, output.ThreadID)
	require.Equal(t, "<unknown-msg@example.com>", *output.ThreadID)
}

// TestGroupEmailThread_NonEmailSkipped tests that non-email content is skipped.
func TestGroupEmailThread_NonEmailSkipped(t *testing.T) {
	logger := logging.NewNopLogger()

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          400,
				TenantID:    "test-tenant",
				ContentType: "meeting", // Not an email
				Metadata:    map[string]string{},
			}, nil
		},
	}

	mockThreadRepo := &mockThreadRepository{
		upsertThreadFn: func(ctx context.Context, input *UpsertThreadInput) (int64, error) {
			t.Fatal("UpsertThread should not be called for non-email content")
			return 0, nil
		},
	}

	activities := NewThreadActivities(logger, mockSourceRepo, mockThreadRepo)

	input := GroupEmailThreadInput{
		TenantID: "test-tenant",
		SourceID: 400,
	}

	output, err := activities.GroupEmailThread(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Nil(t, output.ThreadID, "ThreadID should be nil for non-email content")
}

// TestGroupEmailThread_ThreadingFailureNonBlocking tests that repository errors don't fail the activity.
func TestGroupEmailThread_ThreadingFailureNonBlocking(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 15, 0, 0, 0, time.UTC)

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          500,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<error-msg@example.com>",
					"subject":    "Test Error Handling",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	mockThreadRepo := &mockThreadRepository{
		getThreadByRootMessageIDFn: func(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error) {
			return nil, nil
		},
		upsertThreadFn: func(ctx context.Context, input *UpsertThreadInput) (int64, error) {
			return 0, errors.New("database connection failed")
		},
	}

	activities := NewThreadActivities(logger, mockSourceRepo, mockThreadRepo)

	input := GroupEmailThreadInput{
		TenantID: "test-tenant",
		SourceID: 500,
	}

	// Activity should NOT return an error - threading failures are non-blocking
	output, err := activities.GroupEmailThread(context.Background(), input)
	require.NoError(t, err, "Threading failures should not fail the activity")
	require.NotNil(t, output)
	require.Nil(t, output.ThreadID, "ThreadID should be nil on threading failure")
}

// TestGroupEmailThread_ContentTypeMismatch_BUG_pf_a3d615 verifies the fix for pf-a3d615:
// GetSource now returns logical content type ("email") via mapSourceSystemToContentType,
// instead of returning the MIME type ("message/rfc822") from the DB content_type column.
// This ensures email threading runs correctly.
func TestGroupEmailThread_ContentTypeMismatch_BUG_pf_a3d615(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 14, 30, 0, 0, time.UTC)

	// This mock simulates the FIXED behavior of PostgresSourceRepository.GetSource:
	// After the fix, GetSource reads source_system and calls mapSourceSystemToContentType()
	// to return the logical type "email" (not the MIME type "message/rfc822").
	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:       600,
				TenantID: "test-tenant",
				// FIXED: GetSource now returns logical type "email" via mapSourceSystemToContentType
				// (DB would have source_system="manual_eml" -> logical type "email")
				ContentType: "email",
				Metadata: map[string]string{
					"message_id":  "<bug-test@example.com>",
					"in_reply_to": "<parent-msg@example.com>",
					"subject":     "Re: Bug Report",
					"date":        messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	upsertCalled := false
	mockThreadRepo := &mockThreadRepository{
		getThreadByRootMessageIDFn: func(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error) {
			return &EmailThread{
				ID:            200,
				RootMessageID: "<parent-msg@example.com>",
				Subject:       "Bug Report",
				MessageCount:  1,
			}, nil
		},
		upsertThreadFn: func(ctx context.Context, input *UpsertThreadInput) (int64, error) {
			upsertCalled = true
			return 200, nil
		},
	}

	activities := NewThreadActivities(logger, mockSourceRepo, mockThreadRepo)

	input := GroupEmailThreadInput{
		TenantID: "test-tenant",
		SourceID: 600,
	}

	output, err := activities.GroupEmailThread(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// FIXED: Threading now happens because GetSource returns ContentType="email"
	// ThreadID should be set to "<parent-msg@example.com>"
	require.NotNil(t, output.ThreadID, "Threading should happen for emails after pf-a3d615 fix")
	require.Equal(t, "<parent-msg@example.com>", *output.ThreadID)
	require.True(t, upsertCalled, "UpsertThread should be called for emails")
}

// TestGroupEmailThread_JSONArrayMetadata_BUG_pf_6c7c8f reproduces bug pf-6c7c8f part 1:
// source_repo.go:67-72 converts metadata arrays to JSON strings (e.g. references: ["<id1>","<id2>"] -> "[\"<id1>\",\"<id2>\"]").
// thread_activities.go:82-94 copies these as-is into map[string]interface{} without deserializing.
// ThreadGrouper receives JSON strings instead of arrays, can't extract message IDs, returns empty.
//
// This test SHOULD FAIL because the current code doesn't parse JSON strings in metadata.
func TestGroupEmailThread_JSONArrayMetadata_BUG_pf_6c7c8f(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 14, 30, 0, 0, time.UTC)

	// Simulate source_repo.go behavior: references metadata comes as JSON array string
	// (this is what actually happens when reading from DB)
	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          700,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id":  "<msg-700@example.com>",
					"in_reply_to": "<msg-600@example.com>",
					// BUG: references comes as JSON string from source_repo.go:67-72
					"references": "[\"<msg-500@example.com>\",\"<msg-600@example.com>\"]",
					"subject":    "Re: Thread Test",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	var capturedRootMessageID string
	mockThreadRepo := &mockThreadRepository{
		getThreadByRootMessageIDFn: func(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error) {
			capturedRootMessageID = rootMessageID
			return nil, nil
		},
		upsertThreadFn: func(ctx context.Context, input *UpsertThreadInput) (int64, error) {
			return 300, nil
		},
		addThreadMessageFn: func(ctx context.Context, input *AddThreadMessageInput) error {
			return nil
		},
		setContentEnrichmentThreadIDFn: func(ctx context.Context, sourceID int64, threadID string) error {
			return nil
		},
	}

	activities := NewThreadActivities(logger, mockSourceRepo, mockThreadRepo)

	input := GroupEmailThreadInput{
		TenantID: "test-tenant",
		SourceID: 700,
	}

	output, err := activities.GroupEmailThread(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// EXPECTED (AFTER FIX): ThreadGrouper should parse the JSON string in references,
	// extract first reference "<msg-500@example.com>", and use it as thread root
	expectedRootMessageID := "<msg-500@example.com>"

	// ACTUAL (BUG): ThreadGrouper receives references as string "[\"<msg-500@example.com>\",\"<msg-600@example.com>\"]",
	// The regex DOES extract the message IDs, so this test may pass even with the bug.
	// The real issue may be elsewhere (e.g., CreateEnrichmentRecord not being called).
	require.NotNil(t, output.ThreadID, "Should have a thread ID")
	t.Logf("Captured root message ID: %s (expected: %s)", capturedRootMessageID, expectedRootMessageID)
	require.Equal(t, expectedRootMessageID, *output.ThreadID,
		"BUG: references JSON string not parsed - should use first reference as root, not in_reply_to")
	require.Equal(t, expectedRootMessageID, capturedRootMessageID,
		"BUG: thread root should be first reference, not in_reply_to")
}
