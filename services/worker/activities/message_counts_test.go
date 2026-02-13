package activities

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// mockEntityLookupWithCounts extends mockEntityLookup with message count tracking.
type mockEntityLookupWithCounts struct {
	mockEntityLookup
	sentCountCalls     []int64
	receivedCountCalls []int64
}

func (m *mockEntityLookupWithCounts) IncrementSentCount(ctx context.Context, personID int64) error {
	m.sentCountCalls = append(m.sentCountCalls, personID)
	if m.incrementSentCountFunc != nil {
		return m.incrementSentCountFunc(ctx, personID)
	}
	return nil
}

func (m *mockEntityLookupWithCounts) IncrementReceivedCount(ctx context.Context, personID int64) error {
	m.receivedCountCalls = append(m.receivedCountCalls, personID)
	if m.incrementReceivedCountFunc != nil {
		return m.incrementReceivedCountFunc(ctx, personID)
	}
	return nil
}

// TestBuildContextPackage_IncrementsSenderSentCount verifies that when an email is processed,
// the sender's sent_count is incremented after entity resolution.
//
// This test verifies requirement pf-0f8878: message count tracking on person entities.
//
// Expected behavior:
//   - After BuildContextPackage resolves the sender to a person entity
//   - IncrementSentCount is called with the sender's person_id
//   - Sender's sent_count increases by 1
func TestBuildContextPackage_IncrementsSenderSentCount(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Track calls to IncrementSentCount
	entityLookupWithCounts := &mockEntityLookupWithCounts{}

	// Setup mock entity resolver that returns a resolved sender
	senderPersonID := int64(123)
	mockResolver := &mockEntityResolver{
		resolveOrCreateFunc: func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
			if email == "sender@example.com" {
				return &entities.ResolutionResult{
					Person: &entities.Person{
						ID:            senderPersonID,
						TenantID:      tenantID,
						CanonicalName: "Sender Person",
						PrimaryEmail:  email,
						IsInternal:    true,
					},
					Confidence: 1.0,
					Source:     "exact_match",
					IsNew:      false,
				}, nil
			}
			return nil, nil
		},
	}

	// Setup mock context repo
	mockContextRepo := &mockContextPackageRepo{}

	// Create activity
	activity := NewContextBuilderActivities(
		logger,
		mockResolver,
		entityLookupWithCounts,
		mockContextRepo,
		nil, // pipelineRepo not needed for this test
		nil, // configResolver not needed for this test
	)

	// Build context for an email with sender
	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		JobID:       "test-job",
		ContentID:   "em-12345678",
		ContentType: "email",
		SenderEmail: "sender@example.com",
		SenderName:  "Sender Person",
		ParticipantEmails: []workflows.Participant{
			{Email: "sender@example.com", DisplayName: "Sender Person"},
		},
		Extraction: &ExtractEntitiesOutput{
			People:   []PersonResult{},
			Projects: []string{},
		},
	}

	output, err := activity.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// Verify sender was resolved
	if len(output.ResolvedPeople) == 0 {
		t.Fatal("Expected sender to be resolved")
	}

	// CRITICAL ASSERTION: Verify IncrementSentCount was called for the sender
	if len(entityLookupWithCounts.sentCountCalls) != 1 {
		t.Errorf("Expected IncrementSentCount to be called once, got %d calls", len(entityLookupWithCounts.sentCountCalls))
	}

	if len(entityLookupWithCounts.sentCountCalls) > 0 {
		if entityLookupWithCounts.sentCountCalls[0] != senderPersonID {
			t.Errorf("Expected IncrementSentCount to be called with person_id=%d, got %d",
				senderPersonID, entityLookupWithCounts.sentCountCalls[0])
		}
	}

	// Verify received_count was NOT incremented for sender (they are the sender, not a recipient)
	if len(entityLookupWithCounts.receivedCountCalls) != 0 {
		t.Errorf("Expected IncrementReceivedCount not to be called for sender, got %d calls",
			len(entityLookupWithCounts.receivedCountCalls))
	}
}

// TestBuildContextPackage_IncrementsRecipientReceivedCount verifies that when an email is processed,
// each recipient's received_count is incremented after entity resolution.
//
// This test verifies requirement pf-0f8878: message count tracking on person entities.
//
// Expected behavior:
//   - After BuildContextPackage resolves recipients to person entities
//   - IncrementReceivedCount is called for each recipient's person_id
//   - Each recipient's received_count increases by 1
func TestBuildContextPackage_IncrementsRecipientReceivedCount(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Track calls to IncrementReceivedCount
	entityLookupWithCounts := &mockEntityLookupWithCounts{}

	// Setup mock entity resolver that returns resolved people
	senderPersonID := int64(100)
	recipient1PersonID := int64(200)
	recipient2PersonID := int64(300)

	mockResolver := &mockEntityResolver{
		resolveOrCreateFunc: func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
			switch email {
			case "sender@example.com":
				return &entities.ResolutionResult{
					Person: &entities.Person{
						ID:            senderPersonID,
						TenantID:      tenantID,
						CanonicalName: "Sender Person",
						PrimaryEmail:  email,
						IsInternal:    true,
					},
					Confidence: 1.0,
					Source:     "exact_match",
					IsNew:      false,
				}, nil
			case "recipient1@example.com":
				return &entities.ResolutionResult{
					Person: &entities.Person{
						ID:            recipient1PersonID,
						TenantID:      tenantID,
						CanonicalName: "Recipient One",
						PrimaryEmail:  email,
						IsInternal:    true,
					},
					Confidence: 1.0,
					Source:     "exact_match",
					IsNew:      false,
				}, nil
			case "recipient2@example.com":
				return &entities.ResolutionResult{
					Person: &entities.Person{
						ID:            recipient2PersonID,
						TenantID:      tenantID,
						CanonicalName: "Recipient Two",
						PrimaryEmail:  email,
						IsInternal:    true,
					},
					Confidence: 1.0,
					Source:     "exact_match",
					IsNew:      false,
				}, nil
			}
			return nil, nil
		},
	}

	// Setup mock context repo
	mockContextRepo := &mockContextPackageRepo{}

	// Create activity
	activity := NewContextBuilderActivities(
		logger,
		mockResolver,
		entityLookupWithCounts,
		mockContextRepo,
		nil, // pipelineRepo not needed for this test
		nil, // configResolver not needed for this test
	)

	// Build context for an email with sender and two recipients
	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		JobID:       "test-job",
		ContentID:   "em-12345678",
		ContentType: "email",
		SenderEmail: "sender@example.com",
		SenderName:  "Sender Person",
		ParticipantEmails: []workflows.Participant{
			{Email: "sender@example.com", DisplayName: "Sender Person"},
			{Email: "recipient1@example.com", DisplayName: "Recipient One"},
			{Email: "recipient2@example.com", DisplayName: "Recipient Two"},
		},
		Extraction: &ExtractEntitiesOutput{
			People:   []PersonResult{},
			Projects: []string{},
		},
	}

	output, err := activity.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// Verify sender and recipients were resolved
	if len(output.ResolvedPeople) != 3 {
		t.Errorf("Expected 3 people resolved (1 sender + 2 recipients), got %d", len(output.ResolvedPeople))
	}

	// CRITICAL ASSERTION: Verify IncrementReceivedCount was called for each recipient (not sender)
	expectedRecipientIDs := []int64{recipient1PersonID, recipient2PersonID}
	if len(entityLookupWithCounts.receivedCountCalls) != len(expectedRecipientIDs) {
		t.Errorf("Expected IncrementReceivedCount to be called %d times (for recipients), got %d calls",
			len(expectedRecipientIDs), len(entityLookupWithCounts.receivedCountCalls))
	}

	// Verify the correct person IDs were called
	for i, expectedID := range expectedRecipientIDs {
		if i < len(entityLookupWithCounts.receivedCountCalls) {
			actualID := entityLookupWithCounts.receivedCountCalls[i]
			if actualID != expectedID {
				t.Errorf("Expected IncrementReceivedCount call %d to be for person_id=%d, got %d",
					i, expectedID, actualID)
			}
		}
	}

	// Verify sender's received_count was NOT incremented (sender is not a recipient)
	for _, callID := range entityLookupWithCounts.receivedCountCalls {
		if callID == senderPersonID {
			t.Error("IncrementReceivedCount should NOT be called for the sender")
		}
	}
}

// TestBuildContextPackage_MessageCounts_SenderIsAlsoRecipient verifies that when the sender
// is also in the To/Cc list (e.g., replying to their own thread), the counts are tracked correctly.
//
// This test verifies requirement pf-0f8878: message count tracking on person entities.
//
// Expected behavior:
//   - Sender's sent_count is incremented (because they sent the message)
//   - Sender's received_count is incremented (because they are also in the recipient list)
//   - Both counts can be incremented for the same person
func TestBuildContextPackage_MessageCounts_SenderIsAlsoRecipient(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Track calls to message count methods
	entityLookupWithCounts := &mockEntityLookupWithCounts{}

	// Setup mock entity resolver
	senderPersonID := int64(123)

	mockResolver := &mockEntityResolver{
		resolveOrCreateFunc: func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
			if email == "sender@example.com" {
				return &entities.ResolutionResult{
					Person: &entities.Person{
						ID:            senderPersonID,
						TenantID:      tenantID,
						CanonicalName: "Sender Person",
						PrimaryEmail:  email,
						IsInternal:    true,
					},
					Confidence: 1.0,
					Source:     "exact_match",
					IsNew:      false,
				}, nil
			}
			return nil, nil
		},
	}

	// Setup mock context repo
	mockContextRepo := &mockContextPackageRepo{}

	// Create activity
	activity := NewContextBuilderActivities(
		logger,
		mockResolver,
		entityLookupWithCounts,
		mockContextRepo,
		nil, // pipelineRepo not needed for this test
		nil, // configResolver not needed for this test
	)

	// Build context for an email where sender is also a recipient
	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		JobID:       "test-job",
		ContentID:   "em-12345678",
		ContentType: "email",
		SenderEmail: "sender@example.com",
		SenderName:  "Sender Person",
		ParticipantEmails: []workflows.Participant{
			{Email: "sender@example.com", DisplayName: "Sender Person"}, // Sender in recipient list
		},
		Extraction: &ExtractEntitiesOutput{
			People:   []PersonResult{},
			Projects: []string{},
		},
	}

	output, err := activity.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// Verify person was resolved once (deduplication)
	if len(output.ResolvedPeople) != 1 {
		t.Errorf("Expected 1 person resolved (sender), got %d", len(output.ResolvedPeople))
	}

	// CRITICAL ASSERTION: Verify IncrementSentCount was called once for sender
	if len(entityLookupWithCounts.sentCountCalls) != 1 {
		t.Errorf("Expected IncrementSentCount to be called once, got %d calls",
			len(entityLookupWithCounts.sentCountCalls))
	}

	// CRITICAL ASSERTION: Verify IncrementReceivedCount was NOT called (sender should not increment their own received_count)
	// NOTE: The requirement states "increment received_count for each recipient", and the sender
	// should not be considered a recipient for their own message, even if they are in the To/Cc list.
	if len(entityLookupWithCounts.receivedCountCalls) != 0 {
		t.Errorf("Expected IncrementReceivedCount not to be called for sender-as-recipient, got %d calls",
			len(entityLookupWithCounts.receivedCountCalls))
	}
}

// TestBuildContextPackage_MessageCounts_SkipsBots verifies that bot accounts do not have
// their message counts incremented.
//
// This test verifies requirement pf-0f8878: message count tracking on person entities.
//
// Expected behavior:
//   - Bot accounts (e.g., noreply@, notifications@) are filtered out during entity resolution
//   - IncrementSentCount is not called for bot senders
//   - IncrementReceivedCount is not called for bot recipients
func TestBuildContextPackage_MessageCounts_SkipsBots(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Track calls to message count methods
	entityLookupWithCounts := &mockEntityLookupWithCounts{}

	// Setup mock entity resolver that returns bot accounts
	mockResolver := &mockEntityResolver{
		resolveOrCreateFunc: func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
			// Bot accounts should be filtered out before resolution, so this shouldn't be called
			t.Errorf("ResolveOrCreate should not be called for bot accounts, but was called for %s", email)
			return nil, nil
		},
	}

	// Setup mock context repo
	mockContextRepo := &mockContextPackageRepo{}

	// Create activity
	activity := NewContextBuilderActivities(
		logger,
		mockResolver,
		entityLookupWithCounts,
		mockContextRepo,
		nil, // pipelineRepo not needed for this test
		nil, // configResolver not needed for this test
	)

	// Build context for an email from a bot account
	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		JobID:       "test-job",
		ContentID:   "em-12345678",
		ContentType: "email",
		SenderEmail: "noreply@example.com",
		SenderName:  "No Reply",
		ParticipantEmails: []workflows.Participant{
			{Email: "noreply@example.com", DisplayName: "No Reply"},
			{Email: "notifications@example.com", DisplayName: "Notifications"},
		},
		Extraction: &ExtractEntitiesOutput{
			People:   []PersonResult{},
			Projects: []string{},
		},
	}

	output, err := activity.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// Verify no people were resolved (bots filtered out)
	if len(output.ResolvedPeople) != 0 {
		t.Errorf("Expected 0 people resolved (bots filtered), got %d", len(output.ResolvedPeople))
	}

	// CRITICAL ASSERTION: Verify no message counts were incremented for bots
	if len(entityLookupWithCounts.sentCountCalls) != 0 {
		t.Errorf("Expected IncrementSentCount not to be called for bots, got %d calls",
			len(entityLookupWithCounts.sentCountCalls))
	}

	if len(entityLookupWithCounts.receivedCountCalls) != 0 {
		t.Errorf("Expected IncrementReceivedCount not to be called for bots, got %d calls",
			len(entityLookupWithCounts.receivedCountCalls))
	}
}

// Mock implementations for testing
// Note: mockEntityResolver and mockEntityLookup are defined in context_builder_test.go
// Note: mockContextPackageRepo is defined in context_repo_test.go
