// Package activities provides tests for conversation auto-linking activities.
package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockConversationRepository is a mock implementation of the ConversationRepository interface for testing.
type mockConversationRepository struct {
	upsertConversationFn         func(ctx context.Context, conversation *Conversation) (string, error)
	addConversationItemFn        func(ctx context.Context, conversationID, contentID string, sourceID *int64, tenantID string) error
	addConversationParticipantFn func(ctx context.Context, conversationID string, name, address *string, tenantID string) error
	updateConversationStatsFn    func(ctx context.Context, conversationID string) error
}

func (m *mockConversationRepository) UpsertConversation(ctx context.Context, conversation *Conversation) (string, error) {
	if m.upsertConversationFn != nil {
		return m.upsertConversationFn(ctx, conversation)
	}
	return "", nil
}

func (m *mockConversationRepository) AddConversationItem(ctx context.Context, conversationID, contentID string, sourceID *int64, tenantID string) error {
	if m.addConversationItemFn != nil {
		return m.addConversationItemFn(ctx, conversationID, contentID, sourceID, tenantID)
	}
	return nil
}

func (m *mockConversationRepository) AddConversationParticipant(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
	if m.addConversationParticipantFn != nil {
		return m.addConversationParticipantFn(ctx, conversationID, name, address, tenantID)
	}
	return nil
}

func (m *mockConversationRepository) UpdateConversationStats(ctx context.Context, conversationID string) error {
	if m.updateConversationStatsFn != nil {
		return m.updateConversationStatsFn(ctx, conversationID)
	}
	return nil
}

// TestLinkConversation_CreateNewConversation tests creating a new conversation from thread data.
func TestLinkConversation_CreateNewConversation(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 14, 30, 0, 0, time.UTC)
	threadID := "<thread-root@example.com>"
	contentID := "cnt-12345"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			require.Equal(t, "test-tenant", tenantID)
			require.Equal(t, int64(100), sourceID)

			return &Source{
				ID:          100,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-100@example.com>",
					"subject":    "Project Update Q1",
					"from":       "alice@example.com",
					"to":         "bob@example.com,charlie@example.com",
					"cc":         "dave@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	upsertCalled := false
	addItemCalled := false
	participantsCalled := 0
	statsUpdated := false

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			require.Equal(t, "test-tenant", conversation.TenantID)
			require.Equal(t, "Project Update Q1", conversation.Topic)
			require.NotNil(t, conversation.ThreadKey)
			require.Equal(t, threadID, *conversation.ThreadKey)
			require.NotNil(t, conversation.FirstSeen)
			require.Equal(t, messageDate, *conversation.FirstSeen)
			require.NotNil(t, conversation.LastSeen)
			require.Equal(t, messageDate, *conversation.LastSeen)
			upsertCalled = true
			return "conv-123", nil
		},
		addConversationItemFn: func(ctx context.Context, conversationID, itemContentID string, sourceID *int64, tenantID string) error {
			require.Equal(t, "conv-123", conversationID)
			require.Equal(t, contentID, itemContentID)
			require.NotNil(t, sourceID)
			require.Equal(t, int64(100), *sourceID)
			require.Equal(t, "test-tenant", tenantID)
			addItemCalled = true
			return nil
		},
		addConversationParticipantFn: func(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
			require.Equal(t, "conv-123", conversationID)
			require.Equal(t, "test-tenant", tenantID)
			participantsCalled++
			return nil
		},
		updateConversationStatsFn: func(ctx context.Context, conversationID string) error {
			require.Equal(t, "conv-123", conversationID)
			statsUpdated = true
			return nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  100,
		ThreadID:  threadID,
		ContentID: contentID,
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "conv-123", output.ConversationID)
	require.True(t, upsertCalled, "UpsertConversation should have been called")
	require.True(t, addItemCalled, "AddConversationItem should have been called")
	require.Equal(t, 4, participantsCalled, "Should add 4 participants: from + 2 to + 1 cc")
	require.True(t, statsUpdated, "UpdateConversationStats should have been called")
}

// TestLinkConversation_UpdateExistingConversation tests updating an existing conversation (idempotent via thread_key).
func TestLinkConversation_UpdateExistingConversation(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 16, 0, 0, 0, time.UTC)
	threadID := "<thread-root@example.com>"
	contentID := "cnt-67890"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          200,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-200@example.com>",
					"subject":    "Re: Project Update Q1",
					"from":       "bob@example.com",
					"to":         "alice@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	upsertCalled := false

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			require.Equal(t, threadID, *conversation.ThreadKey)
			upsertCalled = true
			// Return existing conversation ID
			return "conv-123", nil
		},
		addConversationItemFn: func(ctx context.Context, conversationID, itemContentID string, sourceID *int64, tenantID string) error {
			require.Equal(t, "conv-123", conversationID)
			require.Equal(t, contentID, itemContentID)
			return nil
		},
		addConversationParticipantFn: func(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
			require.Equal(t, "conv-123", conversationID)
			return nil
		},
		updateConversationStatsFn: func(ctx context.Context, conversationID string) error {
			require.Equal(t, "conv-123", conversationID)
			return nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  200,
		ThreadID:  threadID,
		ContentID: contentID,
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "conv-123", output.ConversationID)
	require.True(t, upsertCalled, "UpsertConversation should update existing conversation")
}

// TestLinkConversation_ParticipantExtraction tests that participants are correctly extracted from email headers.
func TestLinkConversation_ParticipantExtraction(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	threadID := "<meeting-thread@example.com>"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          300,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-300@example.com>",
					"subject":    "Team Meeting Notes",
					"from":       "alice@example.com",
					"to":         "bob@example.com,charlie@example.com,dave@example.com",
					"cc":         "eve@example.com,frank@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	participantEmails := make(map[string]bool)

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-456", nil
		},
		addConversationItemFn: func(ctx context.Context, conversationID, itemContentID string, sourceID *int64, tenantID string) error {
			return nil
		},
		addConversationParticipantFn: func(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
			if address != nil {
				participantEmails[*address] = true
			}
			return nil
		},
		updateConversationStatsFn: func(ctx context.Context, conversationID string) error {
			return nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  300,
		ThreadID:  threadID,
		ContentID: "cnt-333",
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify all participants were extracted
	expectedParticipants := []string{
		"alice@example.com",
		"bob@example.com",
		"charlie@example.com",
		"dave@example.com",
		"eve@example.com",
		"frank@example.com",
	}

	require.Len(t, participantEmails, len(expectedParticipants), "Should extract all unique participants from from/to/cc")

	for _, email := range expectedParticipants {
		require.True(t, participantEmails[email], "Should have participant %s", email)
	}
}

// TestLinkConversation_NoThreadIDSkipped tests that emails without a thread_id are skipped.
func TestLinkConversation_NoThreadIDSkipped(t *testing.T) {
	logger := logging.NewNopLogger()

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			t.Fatal("GetSource should not be called when thread_id is empty")
			return nil, nil
		},
	}

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			t.Fatal("UpsertConversation should not be called when thread_id is empty")
			return "", nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  400,
		ThreadID:  "", // Empty thread ID
		ContentID: "cnt-444",
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Empty(t, output.ConversationID, "ConversationID should be empty when thread_id is missing")
}

// TestLinkConversation_NonEmailSkipped tests that non-email content is skipped.
func TestLinkConversation_NonEmailSkipped(t *testing.T) {
	logger := logging.NewNopLogger()

	threadID := "<thread-root@example.com>"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          500,
				TenantID:    "test-tenant",
				ContentType: "meeting", // Not an email
				Metadata:    map[string]string{},
			}, nil
		},
	}

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			t.Fatal("UpsertConversation should not be called for non-email content")
			return "", nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  500,
		ThreadID:  threadID,
		ContentID: "cnt-555",
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Empty(t, output.ConversationID, "ConversationID should be empty for non-email content")
}

// TestLinkConversation_NonBlockingErrors tests that repository errors don't fail the activity.
func TestLinkConversation_NonBlockingErrors(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 18, 0, 0, 0, time.UTC)
	threadID := "<error-thread@example.com>"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          600,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-600@example.com>",
					"subject":    "Error Test",
					"from":       "test@example.com",
					"to":         "user@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "", errors.New("database connection failed")
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  600,
		ThreadID:  threadID,
		ContentID: "cnt-666",
	}

	// Activity should NOT return an error - conversation linking failures are non-blocking
	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err, "Conversation linking failures should not fail the activity")
	require.NotNil(t, output)
	require.Empty(t, output.ConversationID, "ConversationID should be empty on linking failure")
}

// TestLinkConversation_PartialFailureNonBlocking tests that partial failures (e.g., adding participants) don't fail the activity.
func TestLinkConversation_PartialFailureNonBlocking(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 20, 0, 0, 0, time.UTC)
	threadID := "<partial-error@example.com>"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          700,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-700@example.com>",
					"subject":    "Partial Failure Test",
					"from":       "sender@example.com",
					"to":         "recipient@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	upsertCalled := false
	addItemCalled := false

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			upsertCalled = true
			return "conv-789", nil
		},
		addConversationItemFn: func(ctx context.Context, conversationID, itemContentID string, sourceID *int64, tenantID string) error {
			addItemCalled = true
			return nil
		},
		addConversationParticipantFn: func(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
			// Simulate participant addition failure
			return errors.New("participant constraint violation")
		},
		updateConversationStatsFn: func(ctx context.Context, conversationID string) error {
			return nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  700,
		ThreadID:  threadID,
		ContentID: "cnt-777",
	}

	// Activity should NOT return an error even if participant addition fails
	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err, "Partial failures should not fail the activity")
	require.NotNil(t, output)
	require.Equal(t, "conv-789", output.ConversationID, "Should return conversation ID even with participant failure")
	require.True(t, upsertCalled, "UpsertConversation should have succeeded")
	require.True(t, addItemCalled, "AddConversationItem should have succeeded")
}

// TestLinkConversation_DuplicateItemIdempotent tests that adding duplicate items is idempotent.
func TestLinkConversation_DuplicateItemIdempotent(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 15, 22, 0, 0, 0, time.UTC)
	threadID := "<duplicate-thread@example.com>"
	contentID := "cnt-888"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          800,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-800@example.com>",
					"subject":    "Idempotency Test",
					"from":       "test@example.com",
					"to":         "user@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	addItemCallCount := 0

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-existing", nil
		},
		addConversationItemFn: func(ctx context.Context, conversationID, itemContentID string, sourceID *int64, tenantID string) error {
			addItemCallCount++
			// Simulate idempotent behavior - no error on duplicate
			return nil
		},
		addConversationParticipantFn: func(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
			return nil
		},
		updateConversationStatsFn: func(ctx context.Context, conversationID string) error {
			return nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  800,
		ThreadID:  threadID,
		ContentID: contentID,
	}

	// Call the activity multiple times with the same input
	output1, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output1)
	require.Equal(t, "conv-existing", output1.ConversationID)

	output2, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output2)
	require.Equal(t, "conv-existing", output2.ConversationID)

	require.Equal(t, 2, addItemCallCount, "AddConversationItem should be called each time but handle duplicates")
}

// TestLinkConversation_MissingContentIDSkipped tests that emails without a content_id are skipped.
func TestLinkConversation_MissingContentIDSkipped(t *testing.T) {
	logger := logging.NewNopLogger()

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			t.Fatal("GetSource should not be called when content_id is empty")
			return nil, nil
		},
	}

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			t.Fatal("UpsertConversation should not be called when content_id is empty")
			return "", nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  900,
		ThreadID:  "<thread@example.com>",
		ContentID: "", // Empty content ID
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Empty(t, output.ConversationID, "ConversationID should be empty when content_id is missing")
}

// TestLinkConversation_ExtractSubject tests that the conversation topic is extracted from the email subject.
func TestLinkConversation_ExtractSubject(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 16, 8, 0, 0, 0, time.UTC)
	threadID := "<subject-test@example.com>"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          1000,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-1000@example.com>",
					"subject":    "Re: FW: [URGENT] Critical Bug in Production",
					"from":       "dev@example.com",
					"to":         "ops@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	var capturedTopic string

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			capturedTopic = conversation.Topic
			return "conv-1000", nil
		},
		addConversationItemFn: func(ctx context.Context, conversationID, itemContentID string, sourceID *int64, tenantID string) error {
			return nil
		},
		addConversationParticipantFn: func(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
			return nil
		},
		updateConversationStatsFn: func(ctx context.Context, conversationID string) error {
			return nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  1000,
		ThreadID:  threadID,
		ContentID: "cnt-1000",
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "conv-1000", output.ConversationID)

	// Verify subject normalization: prefixes like "Re:", "FW:" should be stripped
	// Expected: "[URGENT] Critical Bug in Production"
	require.Equal(t, "[URGENT] Critical Bug in Production", capturedTopic,
		"Topic should be normalized subject with Re:/FW: prefixes removed")
}
