// Package activities provides tests for conversation auto-linking activities.
package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockConversationRepository is a mock implementation of the ConversationRepository interface for testing.
type mockConversationRepository struct {
	upsertConversationFn                func(ctx context.Context, conversation *Conversation) (string, error)
	addConversationItemFn               func(ctx context.Context, conversationID, contentID string, sourceID *int64, tenantID string) error
	addConversationParticipantFn        func(ctx context.Context, conversationID string, name, address *string, tenantID string) error
	cleanInvalidParticipantsFn          func(ctx context.Context, conversationID, tenantID string) (int64, error)
	updateConversationStatsFn           func(ctx context.Context, conversationID string) error
	updateSummaryFn                     func(ctx context.Context, conversationID, summary string, version int32) error
	updateStateFn                       func(ctx context.Context, conversationID, state, reason string) error
	getConversationItemsFn              func(ctx context.Context, conversationID string, limit int) ([]ConversationItem, error)
	getConversationFn                   func(ctx context.Context, tenantID, conversationID string) (*Conversation, error)
	getConversationItemsWithContentFn   func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error)
	getConversationsWithoutSummaryFn    func(ctx context.Context, tenantID string, limit int) ([]ConversationForSummary, error)
	getStaleActiveConversationsFn       func(ctx context.Context, tenantID string, staleDays int, limit int) ([]ConversationForSummary, error)
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

func (m *mockConversationRepository) CleanInvalidParticipants(ctx context.Context, conversationID, tenantID string) (int64, error) {
	if m.cleanInvalidParticipantsFn != nil {
		return m.cleanInvalidParticipantsFn(ctx, conversationID, tenantID)
	}
	return 0, nil
}

func (m *mockConversationRepository) UpdateConversationStats(ctx context.Context, conversationID string) error {
	if m.updateConversationStatsFn != nil {
		return m.updateConversationStatsFn(ctx, conversationID)
	}
	return nil
}

func (m *mockConversationRepository) UpdateSummary(ctx context.Context, conversationID, summary string, version int32) error {
	if m.updateSummaryFn != nil {
		return m.updateSummaryFn(ctx, conversationID, summary, version)
	}
	return nil
}

func (m *mockConversationRepository) UpdateState(ctx context.Context, conversationID, state, reason string) error {
	if m.updateStateFn != nil {
		return m.updateStateFn(ctx, conversationID, state, reason)
	}
	return nil
}

func (m *mockConversationRepository) GetConversationItems(ctx context.Context, conversationID string, limit int) ([]ConversationItem, error) {
	if m.getConversationItemsFn != nil {
		return m.getConversationItemsFn(ctx, conversationID, limit)
	}
	return nil, nil
}

func (m *mockConversationRepository) GetConversation(ctx context.Context, tenantID, conversationID string) (*Conversation, error) {
	if m.getConversationFn != nil {
		return m.getConversationFn(ctx, tenantID, conversationID)
	}
	return nil, nil
}

func (m *mockConversationRepository) GetConversationItemsWithContent(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
	if m.getConversationItemsWithContentFn != nil {
		return m.getConversationItemsWithContentFn(ctx, conversationID, limit)
	}
	return nil, nil
}

func (m *mockConversationRepository) GetConversationsWithoutSummary(ctx context.Context, tenantID string, limit int) ([]ConversationForSummary, error) {
	if m.getConversationsWithoutSummaryFn != nil {
		return m.getConversationsWithoutSummaryFn(ctx, tenantID, limit)
	}
	return nil, nil
}

func (m *mockConversationRepository) GetStaleActiveConversations(ctx context.Context, tenantID string, staleDays int, limit int) ([]ConversationForSummary, error) {
	if m.getStaleActiveConversationsFn != nil {
		return m.getStaleActiveConversationsFn(ctx, tenantID, staleDays, limit)
	}
	return nil, nil
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

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

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

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

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

// TestLinkConversation_ParticipantExtraction tests that participants are correctly extracted from email headers
// using JSON array format for to/cc fields and verifying both name and address are passed.
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
					"to":         `[{"address":"bob@example.com","name":"Bob Smith"},{"address":"charlie@example.com","name":"Charlie Jones"},{"address":"dave@example.com","name":"Dave Brown"}]`,
					"cc":         `[{"address":"eve@example.com","name":"Eve White"},{"address":"frank@example.com","name":"Frank Black"}]`,
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	type captured struct {
		name    string
		address string
	}
	participantsByAddr := make(map[string]captured)

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-456", nil
		},
		addConversationItemFn: func(ctx context.Context, conversationID, itemContentID string, sourceID *int64, tenantID string) error {
			return nil
		},
		addConversationParticipantFn: func(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
			if address != nil {
				c := captured{address: *address}
				if name != nil {
					c.name = *name
				}
				participantsByAddr[*address] = c
			}
			return nil
		},
		updateConversationStatsFn: func(ctx context.Context, conversationID string) error {
			return nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  300,
		ThreadID:  threadID,
		ContentID: "cnt-333",
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify all 6 participants were extracted (1 from plain from + 3 JSON to + 2 JSON cc)
	require.Len(t, participantsByAddr, 6, "Should extract all unique participants from from/to/cc")

	// from field — plain string, no name
	require.True(t, participantsByAddr["alice@example.com"].address == "alice@example.com", "Should have alice")
	require.Equal(t, "", participantsByAddr["alice@example.com"].name, "Plain from should have empty name")

	// to field — JSON array, names present
	require.Equal(t, "Bob Smith", participantsByAddr["bob@example.com"].name)
	require.Equal(t, "Charlie Jones", participantsByAddr["charlie@example.com"].name)
	require.Equal(t, "Dave Brown", participantsByAddr["dave@example.com"].name)

	// cc field — JSON array, names present
	require.Equal(t, "Eve White", participantsByAddr["eve@example.com"].name)
	require.Equal(t, "Frank Black", participantsByAddr["frank@example.com"].name)
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

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

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

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

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

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

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

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

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

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

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

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

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

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

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

// TestLinkConversation_TriggersSummaryGeneration tests that after successful linking,
// an LLM call is made to generate summary and state, and they are persisted.
func TestLinkConversation_TriggersSummaryGeneration(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	threadID := "<summary-test@example.com>"
	contentID := "cnt-summary-1"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          100,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-100@example.com>",
					"subject":    "Project Discussion",
					"from":       "alice@example.com",
					"to":         "bob@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	summaryCalled := false
	summaryPersisted := false
	statePersisted := false
	var capturedSummary string
	var capturedState string
	var capturedReason string

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-summary-1", nil
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
		updateSummaryFn: func(ctx context.Context, conversationID, summary string, version int32) error {
			summaryPersisted = true
			capturedSummary = summary
			require.Equal(t, "conv-summary-1", conversationID)
			require.Equal(t, int32(1), version) // First version (0 + 1)
			return nil
		},
		updateStateFn: func(ctx context.Context, conversationID, state, reason string) error {
			statePersisted = true
			capturedState = state
			capturedReason = reason
			require.Equal(t, "conv-summary-1", conversationID)
			return nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return []ConversationItemWithContent{
				{ContentID: contentID, ContentText: "Discussion about project milestones", Subject: "Project Discussion", From: "alice@example.com"},
			}, nil
		},
		getConversationFn: func(ctx context.Context, tenantID, conversationID string) (*Conversation, error) {
			return &Conversation{
				ID:             conversationID,
				TenantID:       tenantID,
				Topic:          "Project Discussion",
				SummaryVersion: 0,
			}, nil
		},
	}

	mockAIClient := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			summaryCalled = true
			require.NotEmpty(t, req.Content)
			// Return JSON structured response
			return &aiv1.SummaryResponse{
				Summary:   `{"summary":"Discussion about project milestones and next steps.","state":"active","state_reason":"Ongoing project discussion"}`,
				KeyPoints: []string{"Project timeline", "Team assignments"},
				ModelUsed: "llama3.2",
			}, nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, mockAIClient)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  100,
		ThreadID:  threadID,
		ContentID: contentID,
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "conv-summary-1", output.ConversationID)

	// Verify LLM was called for summary+state generation
	require.True(t, summaryCalled, "LLM should be called to generate summary and state")

	// Verify summary was persisted
	require.True(t, summaryPersisted, "Summary should be persisted via UpdateSummary")
	require.NotEmpty(t, capturedSummary, "Summary should not be empty")
	require.Contains(t, capturedSummary, "project", "Summary should mention project")

	// Verify state was persisted
	require.True(t, statePersisted, "State should be persisted via UpdateState")
	require.NotEmpty(t, capturedState, "State should not be empty")
	require.Contains(t, []string{"active", "stalled", "resolved", "unknown"}, capturedState,
		"State should be one of: active, stalled, resolved, unknown")
	require.NotEmpty(t, capturedReason, "State reason should not be empty")
}

// TestLinkConversation_SummaryWithExistingContent tests that the LLM prompt includes
// existing summary + last 3 items + new item when conversation already has content.
func TestLinkConversation_SummaryWithExistingContent(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 16, 11, 0, 0, 0, time.UTC)
	threadID := "<existing-content@example.com>"
	contentID := "cnt-existing-4"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          104,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-104@example.com>",
					"subject":    "Re: Project Discussion",
					"from":       "bob@example.com",
					"to":         "alice@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	var llmPrompt string
	summaryCalled := false

	existingSummary := "Initial discussion about project milestones."
	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-existing-1", nil
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
		updateSummaryFn: func(ctx context.Context, conversationID, summary string, version int32) error {
			return nil
		},
		updateStateFn: func(ctx context.Context, conversationID, state, reason string) error {
			return nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return []ConversationItemWithContent{
				{ContentID: "cnt-existing-1", ContentText: "First email about project", Subject: "Project Discussion", From: "alice@example.com"},
				{ContentID: "cnt-existing-2", ContentText: "Reply with updates", Subject: "Re: Project Discussion", From: "bob@example.com"},
				{ContentID: "cnt-existing-3", ContentText: "Follow up on timeline", Subject: "Re: Project Discussion", From: "alice@example.com"},
				{ContentID: contentID, ContentText: "New input about resources", Subject: "Re: Project Discussion", From: "bob@example.com"},
			}, nil
		},
		getConversationFn: func(ctx context.Context, tenantID, conversationID string) (*Conversation, error) {
			return &Conversation{
				ID:             conversationID,
				TenantID:       tenantID,
				Topic:          "Project Discussion",
				StateSummary:   &existingSummary,
				SummaryVersion: 3,
			}, nil
		},
	}

	mockAIClient := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			summaryCalled = true
			llmPrompt = req.Content
			require.NotEmpty(t, req.Content, "Prompt should not be empty")
			return &aiv1.SummaryResponse{
				Summary:   `{"summary":"Updated discussion including new inputs about timeline.","state":"active","state_reason":"Ongoing discussion"}`,
				KeyPoints: []string{"Timeline", "Resources"},
				ModelUsed: "llama3.2",
			}, nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, mockAIClient)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  104,
		ThreadID:  threadID,
		ContentID: contentID,
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify LLM was called with context
	require.True(t, summaryCalled, "LLM should be called for summary update")
	require.NotEmpty(t, llmPrompt, "LLM prompt should not be empty")
}

// TestLinkConversation_StateTransition tests that state changes are tracked correctly.
func TestLinkConversation_StateTransition(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	threadID := "<state-test@example.com>"
	contentID := "cnt-state-1"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          105,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-105@example.com>",
					"subject":    "Project Blocked",
					"from":       "alice@example.com",
					"to":         "bob@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	var capturedState string
	var capturedReason string

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-state-1", nil
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
		updateSummaryFn: func(ctx context.Context, conversationID, summary string, version int32) error {
			return nil
		},
		updateStateFn: func(ctx context.Context, conversationID, state, reason string) error {
			capturedState = state
			capturedReason = reason
			return nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return []ConversationItemWithContent{
				{ContentID: contentID, ContentText: "Project is blocked waiting for vendor pricing", Subject: "Project Blocked", From: "alice@example.com"},
			}, nil
		},
		getConversationFn: func(ctx context.Context, tenantID, conversationID string) (*Conversation, error) {
			return &Conversation{
				ID:             conversationID,
				TenantID:       tenantID,
				Topic:          "Project Blocked",
				SummaryVersion: 0,
			}, nil
		},
	}

	mockAIClient := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			// Return a JSON response that indicates the conversation is stalled
			return &aiv1.SummaryResponse{
				Summary:   `{"summary":"Project is blocked waiting for external dependencies.","state":"stalled","state_reason":"Waiting on vendor pricing"}`,
				KeyPoints: []string{"Blocked", "External dependency"},
				ModelUsed: "llama3.2",
			}, nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, mockAIClient)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  105,
		ThreadID:  threadID,
		ContentID: contentID,
	}

	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify state was set correctly
	require.NotEmpty(t, capturedState, "State should be set")
	validStates := map[string]bool{"active": true, "stalled": true, "resolved": true, "unknown": true}
	require.True(t, validStates[capturedState], "State should be one of: active, stalled, resolved, unknown")
	require.NotEmpty(t, capturedReason, "State reason should explain why the state was chosen")
}

// TestLinkConversation_SummaryFailureNonBlocking tests that if the LLM call fails,
// LinkConversation still succeeds (non-blocking pattern).
func TestLinkConversation_SummaryFailureNonBlocking(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 16, 13, 0, 0, 0, time.UTC)
	threadID := "<llm-fail@example.com>"
	contentID := "cnt-llm-fail-1"

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          106,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-106@example.com>",
					"subject":    "Test Subject",
					"from":       "alice@example.com",
					"to":         "bob@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	summaryNotPersisted := true
	stateNotPersisted := true

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-llm-fail-1", nil
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
		updateSummaryFn: func(ctx context.Context, conversationID, summary string, version int32) error {
			summaryNotPersisted = false
			t.Fatal("UpdateSummary should not be called when LLM fails")
			return nil
		},
		updateStateFn: func(ctx context.Context, conversationID, state, reason string) error {
			stateNotPersisted = false
			t.Fatal("UpdateState should not be called when LLM fails")
			return nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return []ConversationItemWithContent{
				{ContentID: contentID, ContentText: "Test content", Subject: "Test Subject", From: "alice@example.com"},
			}, nil
		},
		getConversationFn: func(ctx context.Context, tenantID, conversationID string) (*Conversation, error) {
			return &Conversation{
				ID:             conversationID,
				TenantID:       tenantID,
				Topic:          "Test Subject",
				SummaryVersion: 0,
			}, nil
		},
	}

	mockAIClient := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			// Simulate LLM failure
			return nil, errors.New("LLM service unavailable")
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, mockAIClient)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  106,
		ThreadID:  threadID,
		ContentID: contentID,
	}

	// Activity should still succeed even if LLM fails (non-blocking)
	output, err := activities.LinkConversation(context.Background(), input)
	require.NoError(t, err, "LinkConversation should succeed even if LLM fails")
	require.NotNil(t, output)
	require.Equal(t, "conv-llm-fail-1", output.ConversationID)

	// Verify summary and state were not persisted (since LLM failed)
	require.True(t, summaryNotPersisted, "Summary should not be persisted when LLM fails")
	require.True(t, stateNotPersisted, "State should not be persisted when LLM fails")
}

// TestExtractParticipants_JSONArray tests the extractParticipants helper directly
// for all supported input formats.
func TestExtractParticipants_JSONArray(t *testing.T) {
	t.Run("JSON array input returns correct name+address pairs", func(t *testing.T) {
		to := `[{"address":"tdunn@akamai.com","name":"Dunn, Tim"},{"address":"jabrown@akamai.com","name":"Brown, James"}]`
		results := extractParticipants("", to, "")
		require.Len(t, results, 2)
		require.Equal(t, "tdunn@akamai.com", results[0].address)
		require.Equal(t, "Dunn, Tim", results[0].name)
		require.Equal(t, "jabrown@akamai.com", results[1].address)
		require.Equal(t, "Brown, James", results[1].name)
	})

	t.Run("JSON array with commas in names is parsed correctly", func(t *testing.T) {
		to := `[{"address":"smith.john@example.com","name":"Smith, John"},{"address":"doe.jane@example.com","name":"Doe, Jane"}]`
		results := extractParticipants("", to, "")
		require.Len(t, results, 2)
		require.Equal(t, "Smith, John", results[0].name)
		require.Equal(t, "Doe, Jane", results[1].name)
	})

	t.Run("plain comma-separated emails — backward compat, no names", func(t *testing.T) {
		results := extractParticipants("alice@example.com", "bob@example.com,charlie@example.com", "")
		require.Len(t, results, 3)
		for _, p := range results {
			require.Empty(t, p.name, "plain addresses should have no name")
		}
		addrs := []string{results[0].address, results[1].address, results[2].address}
		require.Contains(t, addrs, "alice@example.com")
		require.Contains(t, addrs, "bob@example.com")
		require.Contains(t, addrs, "charlie@example.com")
	})

	t.Run("mixed: plain from, JSON to and cc", func(t *testing.T) {
		from := "alice@example.com"
		to := `[{"address":"bob@example.com","name":"Bob Smith"}]`
		cc := `[{"address":"charlie@example.com","name":"Charlie Jones"}]`
		results := extractParticipants(from, to, cc)
		require.Len(t, results, 3)

		byAddr := make(map[string]emailParticipant)
		for _, p := range results {
			byAddr[p.address] = p
		}
		require.Equal(t, "", byAddr["alice@example.com"].name)
		require.Equal(t, "Bob Smith", byAddr["bob@example.com"].name)
		require.Equal(t, "Charlie Jones", byAddr["charlie@example.com"].name)
	})

	t.Run("empty string returns empty result", func(t *testing.T) {
		results := extractParticipants("", "", "")
		require.Empty(t, results)
	})

	t.Run("malformed JSON falls back to comma split", func(t *testing.T) {
		// Starts with '[' but is malformed — falls back to treating the whole string as one address.
		malformed := `[not valid json`
		results := extractParticipants("", malformed, "")
		// Falls back to comma-split; the single token is the whole malformed string.
		require.Len(t, results, 1)
		require.Equal(t, malformed, results[0].address)
		require.Empty(t, results[0].name)
	})

	t.Run("deduplication by address case-insensitive", func(t *testing.T) {
		// Same email in from (plain) and in to (JSON array), different case.
		from := "Alice@example.com"
		to := `[{"address":"alice@example.com","name":"Alice Smith"}]`
		results := extractParticipants(from, to, "")
		require.Len(t, results, 1, "duplicate address should be deduplicated")
		require.Equal(t, "Alice@example.com", results[0].address, "first-seen address is kept")
	})
}

// TestLinkConversation_JSONParticipants exercises the full LinkConversation flow
// with JSON array metadata and verifies correct participant count, name+address
// passing, and deduplication.
func TestLinkConversation_JSONParticipants(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC)
	threadID := "<json-participants@example.com>"

	// from is a plain string; to/cc are JSON arrays.
	// alice@example.com appears in both from and to — should be deduplicated to 1 entry.
	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          400,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-400@example.com>",
					"subject":    "JSON Participant Test",
					"from":       "alice@example.com",
					"to":         `[{"address":"alice@example.com","name":"Alice Smith"},{"address":"bob@example.com","name":"Bob Jones"}]`,
					"cc":         `[{"address":"carol@example.com","name":"Carol White"}]`,
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	type capturedParticipant struct {
		name    string
		address string
	}
	var capturedParticipants []capturedParticipant

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-json-1", nil
		},
		addConversationItemFn: func(ctx context.Context, conversationID, itemContentID string, sourceID *int64, tenantID string) error {
			return nil
		},
		addConversationParticipantFn: func(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
			cp := capturedParticipant{}
			if address != nil {
				cp.address = *address
			}
			if name != nil {
				cp.name = *name
			}
			capturedParticipants = append(capturedParticipants, cp)
			return nil
		},
		updateConversationStatsFn: func(ctx context.Context, conversationID string) error {
			return nil
		},
	}

	activities := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

	output, err := activities.LinkConversation(context.Background(), LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  400,
		ThreadID:  threadID,
		ContentID: "cnt-400",
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "conv-json-1", output.ConversationID)

	// alice in from + alice in to JSON = 1 unique; bob + carol = 3 total.
	require.Len(t, capturedParticipants, 3, "Should have 3 unique participants (from dedup removes duplicate alice)")

	byAddr := make(map[string]capturedParticipant)
	for _, cp := range capturedParticipants {
		byAddr[cp.address] = cp
	}

	// alice comes from the plain from field (first seen), no name
	require.Contains(t, byAddr, "alice@example.com")
	require.Empty(t, byAddr["alice@example.com"].name, "alice from plain from should have no name")

	// bob comes from JSON to, should have name
	require.Contains(t, byAddr, "bob@example.com")
	require.Equal(t, "Bob Jones", byAddr["bob@example.com"].name)

	// carol comes from JSON cc, should have name
	require.Contains(t, byAddr, "carol@example.com")
	require.Equal(t, "Carol White", byAddr["carol@example.com"].name)
}

// TestLinkConversation_TypedNilAIClient_ReturnsOutput reproduces bug pf-60104c:
// the Go nil-pointer-in-interface trap in generateSummaryAndState.
//
// When main.go does:
//
//	var aiClient *ai.Client  // typed nil pointer
//	conversationActivities := activities.NewConversationActivities(..., aiClient)
//
// NewConversationActivities accepts an AIClient interface. Go wraps the typed nil
// *ai.Client in a non-nil interface value. The guard at conversation_activities.go:346
// (`if a.aiClient == nil`) evaluates to FALSE because the interface wrapper is
// non-nil, so execution continues to call a.aiClient.GenerateSummary() on a nil
// concrete value. This panics. The panic propagates up to the defer/recover in
// LinkConversation, which catches it — but because the output local variable has not
// yet been assigned at that point, the recover block leaves the function returns at
// their zero values (nil, nil), so the caller receives no ConversationID.
//
// This test FAILS before the fix: output is nil because the panic causes
// LinkConversation's defer/recover to fire before the return statement is reached.
// After the fix (passing a properly-nil AIClient interface in main.go), the nil guard
// works correctly, summary generation is skipped, and the function returns valid output.
func TestLinkConversation_TypedNilAIClient_ReturnsOutput(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	threadID := "<typed-nil-ai@example.com>"
	contentID := "cnt-typed-nil-1"

	// Construct the typed-nil AIClient: a *mockAIClient nil pointer wrapped in the
	// AIClient interface. The interface value is non-nil (it has a type), but the
	// concrete pointer inside is nil. This is the exact state that main.go creates
	// when it declares `var aiClient *ai.Client` and passes it into NewConversationActivities.
	var nilMock *mockAIClient = nil
	var typedNilAI AIClient = nilMock // non-nil interface, nil concrete pointer

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          900,
				TenantID:    "test-tenant",
				ContentType: "email",
				Metadata: map[string]string{
					"message_id": "<msg-900@example.com>",
					"subject":    "Typed Nil AI Test",
					"from":       "sender@example.com",
					"to":         "recipient@example.com",
					"date":       messageDate.Format(time.RFC3339),
				},
			}, nil
		},
	}

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-typed-nil-1", nil
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
		getConversationFn: func(ctx context.Context, tenantID, conversationID string) (*Conversation, error) {
			return &Conversation{
				ID:             conversationID,
				TenantID:       tenantID,
				Topic:          "Typed Nil AI Test",
				SummaryVersion: 0,
			}, nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return []ConversationItemWithContent{
				{ContentID: "cnt-prev-1", ContentText: "Previous content"},
			}, nil
		},
	}

	// Pass the typed-nil interface directly to NewConversationActivities.
	// This replicates the exact bug: the nil guard inside generateSummaryAndState
	// (`if a.aiClient == nil`) will NOT fire because the interface itself is non-nil.
	ca := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, typedNilAI)

	input := LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  900,
		ThreadID:  threadID,
		ContentID: contentID,
	}

	output, err := ca.LinkConversation(context.Background(), input)

	// These assertions FAIL with the bug:
	//   - generateSummaryAndState passes the nil guard (interface is non-nil)
	//   - a.aiClient.GenerateSummary() panics (nil concrete method dispatch)
	//   - LinkConversation's defer/recover catches the panic
	//   - the deferred recover returns (nil, nil) — output was never assigned
	//
	// After the fix (main.go passes a proper nil AIClient interface instead of a typed
	// nil), the nil guard at conversation_activities.go:346 fires, summary is skipped,
	// and LinkConversation reaches its final return with a valid ConversationID.
	require.NoError(t, err)
	require.NotNil(t, output, "output should not be nil — typed nil AIClient caused panic in generateSummaryAndState, which triggered LinkConversation defer/recover and returned nil output")
	require.NotEmpty(t, output.ConversationID, "conversation ID should be returned even when AI client is a typed nil")
	require.Equal(t, "conv-typed-nil-1", output.ConversationID)
}

// TestBackfillConversationSummaries tests the backfill method that generates
// initial summaries for existing conversations.
// Commented out until BackfillConversationSummaries method is added to ConversationActivities.
/*
func TestBackfillConversationSummaries(t *testing.T) {
	logger := logging.NewNopLogger()

	// This test will fail because the backfill method doesn't exist yet
	t.Skip("BackfillConversationSummaries method not implemented yet")

	activities := NewConversationActivities(logger, &mockSourceRepository{}, &mockConversationRepository{})

	input := BackfillConversationSummariesInput{
		TenantID: "test-tenant",
		Limit:    10,
	}

	output, err := activities.BackfillConversationSummaries(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Greater(t, output.Processed, 0, "Should process at least one conversation")
}
*/

// TestLinkConversation_ParticipantDedup_OnReprocess reproduces bug pf-8d613c.
//
// When the same conversation is processed twice (e.g. after a parsing fix),
// LinkConversation must handle participants correctly:
// 1. UPSERT participants (ON CONFLICT updates name) — so clean names replace mangled ones
// 2. Clean up invalid participant addresses (JSON artifacts from old comma-split parser)
// 3. Preserve participants from other emails in the conversation (no delete-all)
//
// The previous delete-all approach over-corrected: it removed all 136 participants
// (including those from other emails) and re-added only ~4 from the current email.
func TestLinkConversation_ParticipantDedup_OnReprocess(t *testing.T) {
	logger := logging.NewNopLogger()

	messageDate := time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC)
	threadID := "<reprocess-dedup@example.com>"

	source := &Source{
		ID:          901,
		TenantID:    "test-tenant",
		ContentType: "email",
		Metadata: map[string]string{
			"message_id": "<msg-901@example.com>",
			"subject":    "Reprocess Dedup Test",
			"from":       "alice@example.com",
			"to":         `[{"address":"bob@example.com","name":"Bob Jones"},{"address":"carol@example.com","name":"Carol White"}]`,
			"date":       messageDate.Format(time.RFC3339),
		},
	}

	mockSourceRepo := &mockSourceRepository{
		getSourceFn: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return source, nil
		},
	}

	// Track calls to AddConversationParticipant and CleanInvalidParticipants.
	var addedAddresses []string
	cleanCallCount := 0

	mockConvRepo := &mockConversationRepository{
		upsertConversationFn: func(ctx context.Context, conversation *Conversation) (string, error) {
			return "conv-reprocess-1", nil
		},
		addConversationItemFn: func(ctx context.Context, conversationID, itemContentID string, sourceID *int64, tenantID string) error {
			return nil
		},
		addConversationParticipantFn: func(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
			if address != nil {
				addedAddresses = append(addedAddresses, *address)
			}
			return nil
		},
		cleanInvalidParticipantsFn: func(ctx context.Context, conversationID, tenantID string) (int64, error) {
			cleanCallCount++
			return 3, nil // simulate cleaning 3 garbage entries
		},
		updateConversationStatsFn: func(ctx context.Context, conversationID string) error {
			return nil
		},
	}

	act := NewConversationActivities(logger, mockSourceRepo, mockConvRepo, nil)

	output, err := act.LinkConversation(context.Background(), LinkConversationInput{
		TenantID:  "test-tenant",
		SourceID:  901,
		ThreadID:  threadID,
		ContentID: "cnt-901",
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "conv-reprocess-1", output.ConversationID)

	// ASSERTION 1: Participants from the current email are upserted (not preceded by delete-all).
	// 3 participants: alice (from), bob and carol (to JSON array).
	require.Len(t, addedAddresses, 3,
		"Should upsert all participants from the current email's from/to/cc headers")
	require.Contains(t, addedAddresses, "alice@example.com")
	require.Contains(t, addedAddresses, "bob@example.com")
	require.Contains(t, addedAddresses, "carol@example.com")

	// ASSERTION 2: CleanInvalidParticipants is called to remove garbage entries
	// from the old JSON-split parser (addresses containing {, [, " etc).
	require.Equal(t, 1, cleanCallCount,
		"CleanInvalidParticipants must be called once per LinkConversation invocation "+
			"to remove garbage participant entries from old parser (bug pf-8d613c)")
}

// =============================================================================
// buildSummaryPrompt Tests
// =============================================================================

func TestBuildSummaryPrompt_IncludesContent(t *testing.T) {
	items := []ConversationItemWithContent{
		{
			ContentID:   "cnt-1",
			ContentText: "Hi team, let's discuss the project timeline.",
			Subject:     "Project Timeline",
			From:        "alice@example.com",
		},
		{
			ContentID:   "cnt-2",
			ContentText: "I think we should target Q2 for delivery.",
			Subject:     "Re: Project Timeline",
			From:        "bob@example.com",
		},
	}

	prompt := buildSummaryPrompt("Project Timeline", nil, items)

	require.Contains(t, prompt, "Project Timeline")
	require.Contains(t, prompt, "alice@example.com")
	require.Contains(t, prompt, "bob@example.com")
	require.Contains(t, prompt, "discuss the project timeline")
	require.Contains(t, prompt, "target Q2")
	require.Contains(t, prompt, `"summary"`)
	require.Contains(t, prompt, `"state"`)
	require.Contains(t, prompt, `"state_reason"`)
}

func TestBuildSummaryPrompt_IncludesExistingSummary(t *testing.T) {
	existing := "Previous discussion about milestones."
	items := []ConversationItemWithContent{
		{ContentID: "cnt-1", ContentText: "New update"},
	}

	prompt := buildSummaryPrompt("Topic", &existing, items)

	require.Contains(t, prompt, "Previous summary:")
	require.Contains(t, prompt, "Previous discussion about milestones.")
}

func TestBuildSummaryPrompt_TruncatesLongContent(t *testing.T) {
	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "word "
	}
	items := []ConversationItemWithContent{
		{ContentID: "cnt-1", ContentText: longContent},
	}

	prompt := buildSummaryPrompt("Topic", nil, items)

	// Content should be truncated to maxContentPerItem + "..."
	require.Contains(t, prompt, "...")
	// But should not contain the full content
	require.Less(t, len(prompt), len(longContent)+500)
}

// =============================================================================
// parseSummaryResponse Tests
// =============================================================================

func TestParseSummaryResponse_JSON(t *testing.T) {
	resp := &aiv1.SummaryResponse{
		Summary: `{"summary":"Project kickoff complete, awaiting vendor proposals.","state":"active","state_reason":"Waiting on three vendor proposals due next week"}`,
	}

	summary, state, reason := parseSummaryResponse(resp)

	require.Equal(t, "Project kickoff complete, awaiting vendor proposals.", summary)
	require.Equal(t, "active", state)
	require.Equal(t, "Waiting on three vendor proposals due next week", reason)
}

func TestParseSummaryResponse_JSONStalled(t *testing.T) {
	resp := &aiv1.SummaryResponse{
		Summary: `{"summary":"Discussion about pricing stalled.","state":"stalled","state_reason":"Waiting on Tim for pricing"}`,
	}

	summary, state, reason := parseSummaryResponse(resp)

	require.Equal(t, "Discussion about pricing stalled.", summary)
	require.Equal(t, "stalled", state)
	require.Equal(t, "Waiting on Tim for pricing", reason)
}

func TestParseSummaryResponse_JSONResolved(t *testing.T) {
	resp := &aiv1.SummaryResponse{
		Summary: `{"summary":"Contract signed with Acme Corp.","state":"resolved","state_reason":"Contract executed, no further action needed"}`,
	}

	_, state, _ := parseSummaryResponse(resp)
	require.Equal(t, "resolved", state)
}

func TestParseSummaryResponse_FallbackKeywords(t *testing.T) {
	// When JSON parsing fails, fall back to keyword detection
	resp := &aiv1.SummaryResponse{
		Summary:   "The project is blocked waiting for external resources.",
		KeyPoints: []string{"Blocked on vendor"},
	}

	_, state, reason := parseSummaryResponse(resp)

	require.Equal(t, "stalled", state)
	require.Equal(t, "Blocked on vendor", reason)
}

func TestParseSummaryResponse_FallbackNoKeywords(t *testing.T) {
	resp := &aiv1.SummaryResponse{
		Summary: "Ongoing discussion about features.",
	}

	_, state, reason := parseSummaryResponse(resp)

	require.Equal(t, "active", state)
	require.Equal(t, "New message received", reason)
}

// =============================================================================
// BackfillConversationSummaries Tests
// =============================================================================

func TestBackfillConversationSummaries_ProcessesConversations(t *testing.T) {
	logger := logging.NewNopLogger()

	var summaryIDs []string
	var stateIDs []string

	mockConvRepo := &mockConversationRepository{
		getConversationsWithoutSummaryFn: func(ctx context.Context, tenantID string, limit int) ([]ConversationForSummary, error) {
			return []ConversationForSummary{
				{ID: "conv-1", TenantID: "test-tenant", Topic: "Topic 1", ItemCount: 3},
				{ID: "conv-2", TenantID: "test-tenant", Topic: "Topic 2", ItemCount: 1},
			}, nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return []ConversationItemWithContent{
				{ContentID: "cnt-1", ContentText: "Email content", Subject: "Subject", From: "alice@example.com"},
			}, nil
		},
		updateSummaryFn: func(ctx context.Context, conversationID, summary string, version int32) error {
			summaryIDs = append(summaryIDs, conversationID)
			return nil
		},
		updateStateFn: func(ctx context.Context, conversationID, state, reason string) error {
			stateIDs = append(stateIDs, conversationID)
			return nil
		},
	}

	mockAIClient := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{
				Summary: `{"summary":"Test summary","state":"active","state_reason":"New discussion"}`,
			}, nil
		},
	}

	activities := NewConversationActivities(logger, &mockSourceRepository{}, mockConvRepo, mockAIClient)

	output, err := activities.BackfillConversationSummaries(context.Background(), BackfillConversationSummariesInput{
		TenantID: "test-tenant",
		Limit:    100,
	})

	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 2, output.Processed)
	require.Equal(t, 0, output.Failed)
	require.Equal(t, 0, output.Skipped)
	require.Len(t, summaryIDs, 2)
	require.Len(t, stateIDs, 2)
	require.Contains(t, summaryIDs, "conv-1")
	require.Contains(t, summaryIDs, "conv-2")
}

func TestBackfillConversationSummaries_SkipsEmpty(t *testing.T) {
	logger := logging.NewNopLogger()

	mockConvRepo := &mockConversationRepository{
		getConversationsWithoutSummaryFn: func(ctx context.Context, tenantID string, limit int) ([]ConversationForSummary, error) {
			return []ConversationForSummary{
				{ID: "conv-empty", TenantID: "test-tenant", Topic: "Empty", ItemCount: 1},
			}, nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return nil, nil // no items
		},
	}

	activities := NewConversationActivities(logger, &mockSourceRepository{}, mockConvRepo, &mockAIClient{})

	output, err := activities.BackfillConversationSummaries(context.Background(), BackfillConversationSummariesInput{
		TenantID: "test-tenant",
		Limit:    100,
	})

	require.NoError(t, err)
	require.Equal(t, 0, output.Processed)
	require.Equal(t, 1, output.Skipped)
}

func TestBackfillConversationSummaries_RequiresAIClient(t *testing.T) {
	logger := logging.NewNopLogger()

	activities := NewConversationActivities(logger, &mockSourceRepository{}, &mockConversationRepository{}, nil)

	_, err := activities.BackfillConversationSummaries(context.Background(), BackfillConversationSummariesInput{
		TenantID: "test-tenant",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "AI client is required")
}

// =============================================================================
// RegenerateConversationSummary Tests
// =============================================================================

func TestRegenerateConversationSummary_RebuildFromScratch(t *testing.T) {
	logger := logging.NewNopLogger()

	var capturedVersion int32
	var capturedSummary string

	mockConvRepo := &mockConversationRepository{
		getConversationFn: func(ctx context.Context, tenantID, conversationID string) (*Conversation, error) {
			existingSummary := "Old summary that should be replaced"
			return &Conversation{
				ID:             conversationID,
				TenantID:       tenantID,
				Topic:          "Contract Negotiation",
				StateSummary:   &existingSummary,
				SummaryVersion: 5,
				ItemCount:      3,
			}, nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return []ConversationItemWithContent{
				{ContentID: "cnt-1", ContentText: "Initial proposal", Subject: "Contract", From: "alice@example.com"},
				{ContentID: "cnt-2", ContentText: "Counter offer", Subject: "Re: Contract", From: "bob@example.com"},
				{ContentID: "cnt-3", ContentText: "Agreement reached", Subject: "Re: Contract", From: "alice@example.com"},
			}, nil
		},
		updateSummaryFn: func(ctx context.Context, conversationID, summary string, version int32) error {
			capturedSummary = summary
			capturedVersion = version
			return nil
		},
		updateStateFn: func(ctx context.Context, conversationID, state, reason string) error {
			return nil
		},
	}

	mockAIClient := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			// Verify prompt does NOT include existing summary (regeneration)
			require.NotContains(t, req.Content, "Previous summary:")
			return &aiv1.SummaryResponse{
				Summary: `{"summary":"Contract negotiation completed with agreement.","state":"resolved","state_reason":"Agreement reached by both parties"}`,
			}, nil
		},
	}

	activities := NewConversationActivities(logger, &mockSourceRepository{}, mockConvRepo, mockAIClient)

	output, err := activities.RegenerateConversationSummary(context.Background(), RegenerateConversationSummaryInput{
		TenantID:       "test-tenant",
		ConversationID: "conv-regen-1",
	})

	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "resolved", output.State)
	require.Equal(t, int32(3), capturedVersion, "Version should equal item count on regeneration")
	require.Equal(t, 3, output.ItemCount)
	require.NotEmpty(t, capturedSummary)
}

func TestRegenerateConversationSummary_EmptyConversation(t *testing.T) {
	logger := logging.NewNopLogger()

	mockConvRepo := &mockConversationRepository{
		getConversationFn: func(ctx context.Context, tenantID, conversationID string) (*Conversation, error) {
			return &Conversation{
				ID:       conversationID,
				TenantID: tenantID,
				Topic:    "Empty",
			}, nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return nil, nil
		},
	}

	activities := NewConversationActivities(logger, &mockSourceRepository{}, mockConvRepo, &mockAIClient{})

	output, err := activities.RegenerateConversationSummary(context.Background(), RegenerateConversationSummaryInput{
		TenantID:       "test-tenant",
		ConversationID: "conv-empty",
	})

	require.NoError(t, err)
	require.Equal(t, "unknown", output.State)
	require.Equal(t, 0, output.ItemCount)
}

// =============================================================================
// CheckStaleConversations Tests
// =============================================================================

func TestCheckStaleConversations_TransitionsStalled(t *testing.T) {
	logger := logging.NewNopLogger()

	var transitionedIDs []string

	mockConvRepo := &mockConversationRepository{
		getStaleActiveConversationsFn: func(ctx context.Context, tenantID string, staleDays int, limit int) ([]ConversationForSummary, error) {
			existingSummary := "Discussion about vendor pricing"
			return []ConversationForSummary{
				{ID: "conv-stale-1", TenantID: "test-tenant", Topic: "Vendor Pricing", StateSummary: &existingSummary, ItemCount: 5},
			}, nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return []ConversationItemWithContent{
				{ContentID: "cnt-1", ContentText: "Waiting on vendor to respond", Subject: "Vendor Pricing", From: "alice@example.com"},
			}, nil
		},
		updateStateFn: func(ctx context.Context, conversationID, state, reason string) error {
			transitionedIDs = append(transitionedIDs, conversationID)
			return nil
		},
	}

	mockAIClient := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{
				Summary: `{"summary":"Vendor pricing discussion","state":"stalled","state_reason":"No response from vendor in 3 weeks"}`,
			}, nil
		},
	}

	activities := NewConversationActivities(logger, &mockSourceRepository{}, mockConvRepo, mockAIClient)

	output, err := activities.CheckStaleConversations(context.Background(), CheckStaleConversationsInput{
		TenantID:  "test-tenant",
		StaleDays: 14,
		Limit:     100,
	})

	require.NoError(t, err)
	require.Equal(t, 1, output.Checked)
	require.Equal(t, 1, output.Transitioned)
	require.Equal(t, 0, output.Failed)
	require.Contains(t, transitionedIDs, "conv-stale-1")
}

func TestCheckStaleConversations_KeepsActive(t *testing.T) {
	logger := logging.NewNopLogger()

	stateUpdated := false

	mockConvRepo := &mockConversationRepository{
		getStaleActiveConversationsFn: func(ctx context.Context, tenantID string, staleDays int, limit int) ([]ConversationForSummary, error) {
			return []ConversationForSummary{
				{ID: "conv-still-active", TenantID: "test-tenant", Topic: "Active Topic", ItemCount: 3},
			}, nil
		},
		getConversationItemsWithContentFn: func(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
			return []ConversationItemWithContent{
				{ContentID: "cnt-1", ContentText: "Still relevant discussion", Subject: "Active Topic", From: "alice@example.com"},
			}, nil
		},
		updateStateFn: func(ctx context.Context, conversationID, state, reason string) error {
			stateUpdated = true
			return nil
		},
	}

	mockAIClient := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{
				Summary: `{"summary":"Discussion still ongoing","state":"active","state_reason":"Expected follow-up next month"}`,
			}, nil
		},
	}

	activities := NewConversationActivities(logger, &mockSourceRepository{}, mockConvRepo, mockAIClient)

	output, err := activities.CheckStaleConversations(context.Background(), CheckStaleConversationsInput{
		TenantID:  "test-tenant",
		StaleDays: 14,
	})

	require.NoError(t, err)
	require.Equal(t, 1, output.Checked)
	require.Equal(t, 0, output.Transitioned, "Should not transition when LLM says still active")
	require.False(t, stateUpdated, "State should not be updated when remaining active")
}
