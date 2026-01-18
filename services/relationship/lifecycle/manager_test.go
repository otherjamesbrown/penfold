package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockStore is an in-memory implementation of RelationshipStore for testing.
type mockStore struct {
	mu            sync.RWMutex
	relationships map[string]*relationshipv1.Relationship // key: tenantID:id
}

func newMockStore() *mockStore {
	return &mockStore{
		relationships: make(map[string]*relationshipv1.Relationship),
	}
}

func (s *mockStore) key(tenantID, id string) string {
	return fmt.Sprintf("%s:%s", tenantID, id)
}

func (s *mockStore) Get(ctx context.Context, tenantID, id string) (*relationshipv1.Relationship, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rel, ok := s.relationships[s.key(tenantID, id)]
	if !ok {
		return nil, fmt.Errorf("relationship not found: %s", id)
	}
	return rel, nil
}

func (s *mockStore) Save(ctx context.Context, rel *relationshipv1.Relationship) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.relationships[s.key(rel.TenantId, rel.Id)] = rel
	return nil
}

func (s *mockStore) Delete(ctx context.Context, tenantID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.relationships, s.key(tenantID, id))
	return nil
}

func (s *mockStore) List(ctx context.Context, tenantID string, filter *ListFilter) ([]*relationshipv1.Relationship, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*relationshipv1.Relationship
	for _, rel := range s.relationships {
		if rel.TenantId != tenantID {
			continue
		}
		if filter != nil {
			if filter.Status != nil && rel.Status != *filter.Status {
				continue
			}
			if filter.MinConfidence != nil && rel.Confidence < *filter.MinConfidence {
				continue
			}
		}
		result = append(result, rel)
	}
	return result, nil
}

// testLogger is a no-op logger for testing.
type testLogger struct{}

func (l *testLogger) Debug(msg string, fields ...logging.Field) {}
func (l *testLogger) Info(msg string, fields ...logging.Field)  {}
func (l *testLogger) Warn(msg string, fields ...logging.Field)  {}
func (l *testLogger) Error(msg string, fields ...logging.Field) {}
func (l *testLogger) With(fields ...logging.Field) logging.Logger {
	return l
}
func (l *testLogger) WithContext(ctx context.Context) logging.Logger {
	return l
}

func newTestLogger() logging.Logger {
	return &testLogger{}
}

func createTestRelationship(tenantID, id string, confidence float32) *relationshipv1.Relationship {
	now := timestamppb.Now()
	return &relationshipv1.Relationship{
		Id:       id,
		TenantId: tenantID,
		SourceEntity: &relationshipv1.Entity{
			Id:   "entity-1",
			Name: "John Doe",
			Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
		},
		TargetEntity: &relationshipv1.Entity{
			Id:   "entity-2",
			Name: "Acme Corp",
			Type: relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION,
		},
		RelationshipType: relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT,
		Confidence:       confidence,
		Status:           relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func TestManager_Create(t *testing.T) {
	store := newMockStore()
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	manager := NewManager(&ManagerConfig{
		Logger:                 logger,
		Store:                  store,
		EventPublisher:         publisher,
		AutoActivateConfidence: 0.8,
	})

	ctx := context.Background()

	t.Run("creates relationship in proposed state for low confidence", func(t *testing.T) {
		rel := createTestRelationship("tenant-1", "", 0.5)
		err := manager.Create(ctx, rel)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if rel.Id == "" {
			t.Error("Expected relationship ID to be set")
		}

		if rel.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED {
			t.Errorf("Expected status DISCOVERED, got %v", rel.Status)
		}

		// Check event was published
		events := publisher.GetEventLog()
		if len(events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(events))
		}
		if events[0].Type != EventRelationshipCreated {
			t.Errorf("Expected event type %s, got %s", EventRelationshipCreated, events[0].Type)
		}
	})

	t.Run("auto-activates high confidence relationship", func(t *testing.T) {
		rel := createTestRelationship("tenant-1", "", 0.9)
		err := manager.Create(ctx, rel)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if rel.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED {
			t.Errorf("Expected status CONFIRMED, got %v", rel.Status)
		}
	})

	t.Run("validates required fields", func(t *testing.T) {
		rel := &relationshipv1.Relationship{
			TenantId: "tenant-1",
			// Missing source and target entities
		}
		err := manager.Create(ctx, rel)
		if err == nil {
			t.Error("Expected validation error")
		}
	})

	t.Run("rejects nil relationship", func(t *testing.T) {
		err := manager.Create(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil relationship")
		}
	})
}

func TestManager_Update(t *testing.T) {
	store := newMockStore()
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	manager := NewManager(&ManagerConfig{
		Logger:         logger,
		Store:          store,
		EventPublisher: publisher,
	})

	ctx := context.Background()

	// Create a relationship first
	rel := createTestRelationship("tenant-1", "rel-1", 0.5)
	if err := store.Save(ctx, rel); err != nil {
		t.Fatalf("Failed to save test relationship: %v", err)
	}

	t.Run("updates confidence", func(t *testing.T) {
		newConfidence := float32(0.7)
		update := &RelationshipUpdate{
			Confidence: &newConfidence,
		}

		err := manager.Update(ctx, "tenant-1", "rel-1", update)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		updated, _ := store.Get(ctx, "tenant-1", "rel-1")
		if updated.Confidence != 0.7 {
			t.Errorf("Expected confidence 0.7, got %v", updated.Confidence)
		}
	})

	t.Run("adds evidence", func(t *testing.T) {
		update := &RelationshipUpdate{
			AddEvidence: []*relationshipv1.Evidence{
				{
					SourceId:   "source-1",
					SourceType: "email",
					Excerpt:    "Test evidence",
				},
			},
		}

		err := manager.Update(ctx, "tenant-1", "rel-1", update)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		updated, _ := store.Get(ctx, "tenant-1", "rel-1")
		if len(updated.Evidence) != 1 {
			t.Errorf("Expected 1 evidence item, got %d", len(updated.Evidence))
		}
	})

	t.Run("rejects invalid confidence", func(t *testing.T) {
		invalidConfidence := float32(1.5)
		update := &RelationshipUpdate{
			Confidence: &invalidConfidence,
		}

		err := manager.Update(ctx, "tenant-1", "rel-1", update)
		if err == nil {
			t.Error("Expected validation error for invalid confidence")
		}
	})
}

func TestManager_Archive(t *testing.T) {
	store := newMockStore()
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	manager := NewManager(&ManagerConfig{
		Logger:         logger,
		Store:          store,
		EventPublisher: publisher,
	})

	ctx := context.Background()

	// Create an active relationship
	rel := createTestRelationship("tenant-1", "rel-1", 0.9)
	rel.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED
	if err := store.Save(ctx, rel); err != nil {
		t.Fatalf("Failed to save test relationship: %v", err)
	}

	t.Run("archives relationship", func(t *testing.T) {
		err := manager.Archive(ctx, "tenant-1", "rel-1", "No longer relevant")
		if err != nil {
			t.Fatalf("Archive failed: %v", err)
		}

		archived, _ := store.Get(ctx, "tenant-1", "rel-1")
		if archived.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED {
			t.Errorf("Expected status ARCHIVED, got %v", archived.Status)
		}
	})
}

func TestManager_Restore(t *testing.T) {
	store := newMockStore()
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	manager := NewManager(&ManagerConfig{
		Logger:                 logger,
		Store:                  store,
		EventPublisher:         publisher,
		AutoActivateConfidence: 0.8,
	})

	ctx := context.Background()

	// Create an archived relationship
	rel := createTestRelationship("tenant-1", "rel-1", 0.9)
	rel.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED
	if err := store.Save(ctx, rel); err != nil {
		t.Fatalf("Failed to save test relationship: %v", err)
	}

	t.Run("restores high confidence relationship to active", func(t *testing.T) {
		err := manager.Restore(ctx, "tenant-1", "rel-1")
		if err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		restored, _ := store.Get(ctx, "tenant-1", "rel-1")
		if restored.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED {
			t.Errorf("Expected status CONFIRMED, got %v", restored.Status)
		}
	})

	t.Run("restores low confidence relationship to proposed", func(t *testing.T) {
		// Create a low confidence archived relationship
		rel2 := createTestRelationship("tenant-1", "rel-2", 0.5)
		rel2.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED
		store.Save(ctx, rel2)

		err := manager.Restore(ctx, "tenant-1", "rel-2")
		if err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		restored, _ := store.Get(ctx, "tenant-1", "rel-2")
		if restored.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED {
			t.Errorf("Expected status DISCOVERED, got %v", restored.Status)
		}
	})

	t.Run("rejects restore of non-archived relationship", func(t *testing.T) {
		// Create an active relationship
		rel3 := createTestRelationship("tenant-1", "rel-3", 0.9)
		rel3.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED
		store.Save(ctx, rel3)

		err := manager.Restore(ctx, "tenant-1", "rel-3")
		if err == nil {
			t.Error("Expected error for restoring non-archived relationship")
		}
	})
}

func TestManager_Merge(t *testing.T) {
	store := newMockStore()
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	manager := NewManager(&ManagerConfig{
		Logger:         logger,
		Store:          store,
		EventPublisher: publisher,
	})

	ctx := context.Background()

	t.Run("merges relationships successfully", func(t *testing.T) {
		// Create source relationship with evidence
		source := createTestRelationship("tenant-1", "source-1", 0.6)
		source.Evidence = []*relationshipv1.Evidence{
			{SourceId: "ev-1", SourceType: "email", Excerpt: "Evidence 1"},
		}
		store.Save(ctx, source)

		// Create target relationship
		target := createTestRelationship("tenant-1", "target-1", 0.7)
		target.Evidence = []*relationshipv1.Evidence{
			{SourceId: "ev-2", SourceType: "meeting", Excerpt: "Evidence 2"},
		}
		store.Save(ctx, target)

		err := manager.Merge(ctx, "tenant-1", "source-1", "target-1")
		if err != nil {
			t.Fatalf("Merge failed: %v", err)
		}

		// Check source is marked as merged
		mergedSource, _ := store.Get(ctx, "tenant-1", "source-1")
		if mergedSource.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED {
			t.Errorf("Expected source status ARCHIVED, got %v", mergedSource.Status)
		}

		// Check target has combined evidence
		mergedTarget, _ := store.Get(ctx, "tenant-1", "target-1")
		if len(mergedTarget.Evidence) != 2 {
			t.Errorf("Expected 2 evidence items, got %d", len(mergedTarget.Evidence))
		}
	})

	t.Run("rejects self-merge", func(t *testing.T) {
		rel := createTestRelationship("tenant-1", "self-1", 0.5)
		store.Save(ctx, rel)

		err := manager.Merge(ctx, "tenant-1", "self-1", "self-1")
		if err == nil {
			t.Error("Expected error for self-merge")
		}
	})
}

func TestManager_Confirm(t *testing.T) {
	store := newMockStore()
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	manager := NewManager(&ManagerConfig{
		Logger:         logger,
		Store:          store,
		EventPublisher: publisher,
	})

	ctx := context.Background()

	// Create a proposed relationship
	rel := createTestRelationship("tenant-1", "rel-1", 0.5)
	rel.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED
	store.Save(ctx, rel)

	t.Run("confirms relationship", func(t *testing.T) {
		err := manager.Confirm(ctx, "tenant-1", "rel-1", "user-123")
		if err != nil {
			t.Fatalf("Confirm failed: %v", err)
		}

		confirmed, _ := store.Get(ctx, "tenant-1", "rel-1")
		if confirmed.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED {
			t.Errorf("Expected status CONFIRMED, got %v", confirmed.Status)
		}
	})
}

func TestManager_Reject(t *testing.T) {
	store := newMockStore()
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	manager := NewManager(&ManagerConfig{
		Logger:         logger,
		Store:          store,
		EventPublisher: publisher,
	})

	ctx := context.Background()

	// Create a proposed relationship
	rel := createTestRelationship("tenant-1", "rel-1", 0.5)
	rel.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED
	store.Save(ctx, rel)

	t.Run("rejects relationship", func(t *testing.T) {
		err := manager.Reject(ctx, "tenant-1", "rel-1", "Not a valid relationship", "user-123")
		if err != nil {
			t.Fatalf("Reject failed: %v", err)
		}

		rejected, _ := store.Get(ctx, "tenant-1", "rel-1")
		if rejected.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED {
			t.Errorf("Expected status ARCHIVED, got %v", rejected.Status)
		}
	})
}

func TestStateMachine_CanTransition(t *testing.T) {
	sm := NewStateMachine(&StateMachineConfig{Logger: newTestLogger()})

	testCases := []struct {
		name     string
		from     State
		to       State
		expected bool
	}{
		{"proposed to active", StateProposed, StateActive, true},
		{"proposed to archived", StateProposed, StateArchived, true},
		{"proposed to merged", StateProposed, StateMerged, true},
		{"proposed to inactive", StateProposed, StateInactive, false},
		{"active to inactive", StateActive, StateInactive, true},
		{"active to archived", StateActive, StateArchived, true},
		{"active to merged", StateActive, StateMerged, true},
		{"active to proposed", StateActive, StateProposed, false},
		{"inactive to active", StateInactive, StateActive, true},
		{"inactive to archived", StateInactive, StateArchived, true},
		{"archived to active", StateArchived, StateActive, true},
		{"archived to proposed", StateArchived, StateProposed, true},
		{"merged to anything", StateMerged, StateActive, false},
		{"same state", StateActive, StateActive, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sm.CanTransition(tc.from, tc.to)
			if result != tc.expected {
				t.Errorf("CanTransition(%s, %s) = %v, expected %v", tc.from, tc.to, result, tc.expected)
			}
		})
	}
}

func TestStateMachine_ExecuteTransition(t *testing.T) {
	historyStore := NewInMemoryHistoryStore()
	sm := NewStateMachine(&StateMachineConfig{
		Logger:       newTestLogger(),
		HistoryStore: historyStore,
	})

	ctx := context.Background()

	t.Run("executes valid transition", func(t *testing.T) {
		rel := createTestRelationship("tenant-1", "rel-1", 0.5)
		rel.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED

		transition, err := sm.ExecuteTransition(ctx, rel, StateActive, "test reason", "test-actor")
		if err != nil {
			t.Fatalf("ExecuteTransition failed: %v", err)
		}

		if transition.From != StateProposed {
			t.Errorf("Expected from state %s, got %s", StateProposed, transition.From)
		}
		if transition.To != StateActive {
			t.Errorf("Expected to state %s, got %s", StateActive, transition.To)
		}
		if rel.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED {
			t.Errorf("Expected status CONFIRMED, got %v", rel.Status)
		}
	})

	t.Run("rejects invalid transition", func(t *testing.T) {
		rel := createTestRelationship("tenant-1", "rel-2", 0.5)
		rel.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED

		_, err := sm.ExecuteTransition(ctx, rel, StateInactive, "test reason", "test-actor")
		if err == nil {
			t.Error("Expected error for invalid transition")
		}
	})
}

func TestDecayDetector(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	detector := &DecayDetector{
		InactivePeriod: 180 * 24 * time.Hour, // 6 months
		DecayThreshold: 0.5,
		Now:            func() time.Time { return fixedTime },
	}

	t.Run("detects decayed relationship", func(t *testing.T) {
		// Last updated 200 days ago
		oldTime := fixedTime.Add(-200 * 24 * time.Hour)
		rel := createTestRelationship("tenant-1", "rel-1", 0.8)
		rel.UpdatedAt = timestamppb.New(oldTime)

		info := detector.CheckDecay(rel)

		if !info.ShouldTransition {
			t.Error("Expected relationship to be marked for transition")
		}
		if info.DaysSinceActivity != 200 {
			t.Errorf("Expected 200 days since activity, got %d", info.DaysSinceActivity)
		}
	})

	t.Run("does not decay recent relationship", func(t *testing.T) {
		// Last updated 30 days ago
		recentTime := fixedTime.Add(-30 * 24 * time.Hour)
		rel := createTestRelationship("tenant-1", "rel-2", 0.8)
		rel.UpdatedAt = timestamppb.New(recentTime)

		info := detector.CheckDecay(rel)

		if info.ShouldTransition {
			t.Error("Expected relationship to NOT be marked for transition")
		}
	})
}

func TestValidation(t *testing.T) {
	validator := DefaultValidator()
	ctx := context.Background()

	t.Run("validates complete relationship", func(t *testing.T) {
		rel := createTestRelationship("tenant-1", "rel-1", 0.5)
		result := validator.ValidateCreate(ctx, rel)

		if !result.Valid {
			t.Errorf("Expected valid result, got errors: %v", result.Errors)
		}
	})

	t.Run("rejects missing tenant ID", func(t *testing.T) {
		rel := createTestRelationship("", "rel-1", 0.5)
		result := validator.ValidateCreate(ctx, rel)

		if result.Valid {
			t.Error("Expected invalid result for missing tenant ID")
		}
	})

	t.Run("rejects self-referential relationship", func(t *testing.T) {
		rel := createTestRelationship("tenant-1", "rel-1", 0.5)
		rel.SourceEntity.Id = "same-id"
		rel.TargetEntity.Id = "same-id"
		result := validator.ValidateCreate(ctx, rel)

		if result.Valid {
			t.Error("Expected invalid result for self-referential relationship")
		}
	})

	t.Run("rejects invalid confidence", func(t *testing.T) {
		rel := createTestRelationship("tenant-1", "rel-1", 1.5) // > 1.0
		result := validator.ValidateCreate(ctx, rel)

		if result.Valid {
			t.Error("Expected invalid result for confidence > 1.0")
		}
	})

	t.Run("validates merge operation", func(t *testing.T) {
		source := createTestRelationship("tenant-1", "source-1", 0.5)
		target := createTestRelationship("tenant-1", "target-1", 0.6)

		result := validator.ValidateMerge(ctx, source, target)

		if !result.Valid {
			t.Errorf("Expected valid merge, got errors: %v", result.Errors)
		}
	})

	t.Run("rejects merge with different tenants", func(t *testing.T) {
		source := createTestRelationship("tenant-1", "source-1", 0.5)
		target := createTestRelationship("tenant-2", "target-1", 0.6)

		result := validator.ValidateMerge(ctx, source, target)

		if result.Valid {
			t.Error("Expected invalid result for different tenants")
		}
	})
}

func TestEventPublisher(t *testing.T) {
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)
	ctx := context.Background()

	t.Run("publishes and logs events", func(t *testing.T) {
		event := NewEventBuilder(EventRelationshipCreated).
			WithRelationshipID("rel-1").
			WithTenantID("tenant-1").
			WithActor("test-actor").
			Build()

		err := publisher.Publish(ctx, event)
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}

		events := publisher.GetEventLog()
		if len(events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(events))
		}
	})

	t.Run("calls subscribed handlers", func(t *testing.T) {
		handlerCalled := false
		publisher.Subscribe(EventRelationshipUpdated, func(ctx context.Context, event *LifecycleEvent) error {
			handlerCalled = true
			return nil
		})

		event := NewEventBuilder(EventRelationshipUpdated).
			WithRelationshipID("rel-1").
			WithTenantID("tenant-1").
			Build()

		publisher.Publish(ctx, event)

		if !handlerCalled {
			t.Error("Expected handler to be called")
		}
	})

	t.Run("calls global handlers", func(t *testing.T) {
		globalCalled := false
		publisher.SubscribeAll(func(ctx context.Context, event *LifecycleEvent) error {
			globalCalled = true
			return nil
		})

		event := NewEventBuilder(EventRelationshipArchived).
			WithRelationshipID("rel-1").
			WithTenantID("tenant-1").
			Build()

		publisher.Publish(ctx, event)

		if !globalCalled {
			t.Error("Expected global handler to be called")
		}
	})
}

func TestTransitionHistory(t *testing.T) {
	historyStore := NewInMemoryHistoryStore()
	ctx := context.Background()

	t.Run("saves and retrieves history", func(t *testing.T) {
		transition := &StateTransition{
			ID:             "trans-1",
			RelationshipID: "rel-1",
			TenantID:       "tenant-1",
			From:           StateProposed,
			To:             StateActive,
			Timestamp:      time.Now(),
			Reason:         "test",
			Actor:          "test-actor",
		}

		err := historyStore.SaveTransition(ctx, transition)
		if err != nil {
			t.Fatalf("SaveTransition failed: %v", err)
		}

		history, err := historyStore.GetHistory(ctx, "tenant-1", "rel-1")
		if err != nil {
			t.Fatalf("GetHistory failed: %v", err)
		}

		if history.CurrentState != StateActive {
			t.Errorf("Expected current state %s, got %s", StateActive, history.CurrentState)
		}
		if len(history.Transitions) != 1 {
			t.Errorf("Expected 1 transition, got %d", len(history.Transitions))
		}
	})
}

func TestState_IsValid(t *testing.T) {
	testCases := []struct {
		state    State
		expected bool
	}{
		{StateProposed, true},
		{StateActive, true},
		{StateInactive, true},
		{StateArchived, true},
		{StateMerged, true},
		{State("invalid"), false},
		{State(""), false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.state), func(t *testing.T) {
			if tc.state.IsValid() != tc.expected {
				t.Errorf("IsValid(%s) = %v, expected %v", tc.state, tc.state.IsValid(), tc.expected)
			}
		})
	}
}

func TestState_IsTerminal(t *testing.T) {
	testCases := []struct {
		state    State
		expected bool
	}{
		{StateProposed, false},
		{StateActive, false},
		{StateInactive, false},
		{StateArchived, false},
		{StateMerged, true},
	}

	for _, tc := range testCases {
		t.Run(string(tc.state), func(t *testing.T) {
			if tc.state.IsTerminal() != tc.expected {
				t.Errorf("IsTerminal(%s) = %v, expected %v", tc.state, tc.state.IsTerminal(), tc.expected)
			}
		})
	}
}

// ============================================================================
// Additional Tests for Coverage Improvement
// ============================================================================

func TestManager_CreateWithNoStore(t *testing.T) {
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	manager := NewManager(&ManagerConfig{
		Logger:         logger,
		Store:          nil,
		EventPublisher: publisher,
	})

	ctx := context.Background()
	rel := createTestRelationship("tenant-1", "", 0.5)

	err := manager.Create(ctx, rel)
	if err != nil {
		t.Fatalf("Expected no error without store, got: %v", err)
	}
}

func TestManager_UpdateWithNilUpdate(t *testing.T) {
	store := newMockStore()
	logger := newTestLogger()

	manager := NewManager(&ManagerConfig{
		Logger: logger,
		Store:  store,
	})

	ctx := context.Background()

	err := manager.Update(ctx, "tenant-1", "rel-1", nil)
	if err == nil {
		t.Error("Expected error for nil update")
	}
}

func TestManager_UpdateNotes(t *testing.T) {
	store := newMockStore()
	logger := newTestLogger()

	manager := NewManager(&ManagerConfig{
		Logger: logger,
		Store:  store,
	})

	ctx := context.Background()

	rel := createTestRelationship("tenant-1", "rel-notes", 0.5)
	store.Save(ctx, rel)

	notes := "Updated notes"
	update := &RelationshipUpdate{
		Notes: &notes,
	}

	err := manager.Update(ctx, "tenant-1", "rel-notes", update)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, _ := store.Get(ctx, "tenant-1", "rel-notes")
	if updated.Notes == nil || *updated.Notes != "Updated notes" {
		t.Error("Notes not updated correctly")
	}
}

func TestStateMachine_GetValidTransitions(t *testing.T) {
	sm := NewStateMachine(&StateMachineConfig{Logger: newTestLogger()})

	// Test valid transitions from proposed state
	validTargets := sm.GetValidTransitions(StateProposed)
	if len(validTargets) == 0 {
		t.Error("Expected valid transitions from proposed state")
	}

	// Check that active is a valid target
	hasActive := false
	for _, target := range validTargets {
		if target == StateActive {
			hasActive = true
			break
		}
	}
	if !hasActive {
		t.Error("Expected StateActive as valid target from StateProposed")
	}

	// Test merged state (terminal, no valid transitions)
	mergedTargets := sm.GetValidTransitions(StateMerged)
	if len(mergedTargets) != 0 {
		t.Error("Expected no valid transitions from merged state")
	}
}

func TestStateMachine_GetHistoryNoStore(t *testing.T) {
	sm := NewStateMachine(&StateMachineConfig{
		Logger:       newTestLogger(),
		HistoryStore: nil,
	})

	ctx := context.Background()
	_, err := sm.GetHistory(ctx, "tenant-1", "rel-1")
	if err == nil {
		t.Error("Expected error when history store not configured")
	}
}

func TestTransitionHistory_Methods(t *testing.T) {
	history := &TransitionHistory{
		RelationshipID: "rel-1",
		TenantID:       "tenant-1",
		CurrentState:   StateProposed,
	}

	// Test empty history
	if history.GetLatestTransition() != nil {
		t.Error("Expected nil for empty history")
	}

	if history.GetTransitionCount() != 0 {
		t.Error("Expected 0 transitions")
	}

	// Add transitions
	t1 := StateTransition{
		ID:   "trans-1",
		From: StateProposed,
		To:   StateActive,
	}
	history.AddTransition(t1)

	t2 := StateTransition{
		ID:   "trans-2",
		From: StateActive,
		To:   StateArchived,
	}
	history.AddTransition(t2)

	if history.GetTransitionCount() != 2 {
		t.Error("Expected 2 transitions")
	}

	latest := history.GetLatestTransition()
	if latest == nil || latest.ID != "trans-2" {
		t.Error("Expected latest transition to be trans-2")
	}

	// Test GetTransitionsByState
	archivedTrans := history.GetTransitionsByState(StateArchived)
	if len(archivedTrans) != 1 {
		t.Errorf("Expected 1 transition to archived, got %d", len(archivedTrans))
	}
}

func TestEventBuilder_Methods(t *testing.T) {
	event := NewEventBuilder(EventRelationshipCreated).
		WithRelationshipID("rel-1").
		WithTenantID("tenant-1").
		WithActor("test-actor").
		WithData("key1", "value1").
		WithMetadata("meta1", "metavalue1").
		WithTimestamp(time.Now()).
		Build()

	if event.RelationshipID != "rel-1" {
		t.Error("RelationshipID not set correctly")
	}
	if event.TenantID != "tenant-1" {
		t.Error("TenantID not set correctly")
	}
	if event.Actor != "test-actor" {
		t.Error("Actor not set correctly")
	}
	if event.Data["key1"] != "value1" {
		t.Error("Data not set correctly")
	}
	if event.Metadata["meta1"] != "metavalue1" {
		t.Error("Metadata not set correctly")
	}
}

func TestEventBuilder_WithRelationship(t *testing.T) {
	rel := createTestRelationship("tenant-1", "rel-1", 0.5)

	event := NewEventBuilder(EventRelationshipCreated).
		WithRelationship(rel).
		Build()

	if event.RelationshipID != rel.Id {
		t.Error("RelationshipID not set from relationship")
	}
	if event.TenantID != rel.TenantId {
		t.Error("TenantID not set from relationship")
	}
}

func TestEventBuilder_WithStateChange(t *testing.T) {
	event := NewEventBuilder(EventRelationshipStateChanged).
		WithStateChange(StateProposed, StateActive).
		Build()

	if event.PreviousState == nil || *event.PreviousState != StateProposed {
		t.Error("PreviousState not set correctly")
	}
	if event.NewState == nil || *event.NewState != StateActive {
		t.Error("NewState not set correctly")
	}
}

func TestEventFactory_AllEventTypes(t *testing.T) {
	factory := NewEventFactory("test-actor")
	rel := createTestRelationship("tenant-1", "rel-1", 0.5)

	// Test event types
	created := factory.CreatedEvent(rel, "")
	if created.Type != EventRelationshipCreated {
		t.Error("CreatedEvent type incorrect")
	}

	updated := factory.UpdatedEvent(rel, "", map[string]interface{}{"key": "value"})
	if updated.Type != EventRelationshipUpdated {
		t.Error("UpdatedEvent type incorrect")
	}

	stateChanged := factory.StateChangedEvent(rel, StateProposed, StateActive, "test", "")
	if stateChanged.Type != EventRelationshipStateChanged {
		t.Error("StateChangedEvent type incorrect")
	}

	archived := factory.ArchivedEvent(rel, "reason", "")
	if archived.Type != EventRelationshipArchived {
		t.Error("ArchivedEvent type incorrect")
	}

	restored := factory.RestoredEvent(rel, "")
	if restored.Type != EventRelationshipRestored {
		t.Error("RestoredEvent type incorrect")
	}

	merged := factory.MergedEvent("source-1", "target-1", "tenant-1", "duplicate", "", 5)
	if merged.Type != EventRelationshipMerged {
		t.Error("MergedEvent type incorrect")
	}

	confirmed := factory.ConfirmedEvent(rel, "")
	if confirmed.Type != EventRelationshipConfirmed {
		t.Error("ConfirmedEvent type incorrect")
	}

	rejected := factory.RejectedEvent(rel, "not valid", "")
	if rejected.Type != EventRelationshipRejected {
		t.Error("RejectedEvent type incorrect")
	}

	evidenceAdded := factory.EvidenceAddedEvent(rel, 3, "")
	if evidenceAdded.Type != EventEvidenceAdded {
		t.Error("EvidenceAddedEvent type incorrect")
	}
}

func TestAuditLogger(t *testing.T) {
	logger := newTestLogger()
	auditLogger := NewAuditLogger(logger)

	ctx := context.Background()
	event := NewEventBuilder(EventRelationshipCreated).
		WithRelationshipID("rel-1").
		WithTenantID("tenant-1").
		WithActor("test-actor").
		Build()

	err := auditLogger.LogEvent(ctx, event)
	if err != nil {
		t.Errorf("LogEvent failed: %v", err)
	}

	// Test handler
	handler := auditLogger.CreateAuditHandler()
	err = handler(ctx, event)
	if err != nil {
		t.Errorf("Audit handler failed: %v", err)
	}
}

func TestInMemoryEventPublisher_NilEvent(t *testing.T) {
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	ctx := context.Background()
	err := publisher.Publish(ctx, nil)
	if err == nil {
		t.Error("Expected error for nil event")
	}
}

func TestInMemoryEventPublisher_Close(t *testing.T) {
	logger := newTestLogger()
	publisher := NewInMemoryEventPublisher(logger, 100)

	err := publisher.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestValidation_ValidateRestoreNonArchived(t *testing.T) {
	validator := DefaultValidator()
	ctx := context.Background()

	rel := createTestRelationship("tenant-1", "rel-1", 0.5)
	rel.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED

	result := validator.ValidateRestore(ctx, rel)
	if result.Valid {
		t.Error("Expected invalid result for restoring non-archived relationship")
	}
}

func TestValidation_ValidateMergeIntoArchivedTarget(t *testing.T) {
	validator := DefaultValidator()
	ctx := context.Background()

	source := createTestRelationship("tenant-1", "source-1", 0.5)
	target := createTestRelationship("tenant-1", "target-1", 0.5)
	target.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED

	result := validator.ValidateMerge(ctx, source, target)
	if result.Valid {
		t.Error("Expected invalid result for merging into archived target")
	}
}

func TestValidation_NilEntity(t *testing.T) {
	validator := DefaultValidator()
	ctx := context.Background()

	rel := &relationshipv1.Relationship{
		TenantId: "tenant-1",
		SourceEntity: &relationshipv1.Entity{
			Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
		},
		TargetEntity: &relationshipv1.Entity{
			Name: "Target",
			Type: relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION,
		},
		RelationshipType: relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT,
		Confidence:       0.5,
	}

	result := validator.ValidateCreate(ctx, rel)
	if result.Valid {
		t.Error("Expected invalid result for missing entity name")
	}
}

func TestNewManager_NilConfig(t *testing.T) {
	manager := NewManager(nil)
	if manager == nil {
		t.Error("Expected non-nil manager with nil config")
	}
}

func TestDefaultDecayDetector(t *testing.T) {
	detector := DefaultDecayDetector()
	if detector == nil {
		t.Error("Expected non-nil default decay detector")
	}
	if detector.InactivePeriod != 180*24*time.Hour {
		t.Error("Expected default inactive period of 180 days")
	}
}
