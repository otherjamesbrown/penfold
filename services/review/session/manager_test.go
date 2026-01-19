package session

import (
	"context"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// testLogger creates a logger for testing.
func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  false,
	})
}

func TestNewSession(t *testing.T) {
	tenantID := "tenant-1"
	userID := "user-1"
	timeout := 1 * time.Hour

	session := NewSession(tenantID, userID, timeout)

	if session.ID == "" {
		t.Error("expected session ID to be set")
	}
	if session.TenantID != tenantID {
		t.Errorf("expected tenant ID %s, got %s", tenantID, session.TenantID)
	}
	if session.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, session.UserID)
	}
	if session.State != StateActive {
		t.Errorf("expected state %s, got %s", StateActive, session.State)
	}
	if len(session.Items) != 0 {
		t.Errorf("expected empty items, got %d items", len(session.Items))
	}
}

func TestSessionStateTransitions(t *testing.T) {
	tests := []struct {
		name      string
		from      State
		to        State
		canChange bool
	}{
		{"active to paused", StateActive, StatePaused, true},
		{"active to completed", StateActive, StateCompleted, true},
		{"active to expired", StateActive, StateExpired, true},
		{"paused to active", StatePaused, StateActive, true},
		{"paused to completed", StatePaused, StateCompleted, true},
		{"paused to expired", StatePaused, StateExpired, true},
		{"completed to active", StateCompleted, StateActive, false},
		{"completed to paused", StateCompleted, StatePaused, false},
		{"completed to expired", StateCompleted, StateExpired, false},
		{"expired to active", StateExpired, StateActive, false},
		{"expired to paused", StateExpired, StatePaused, false},
		{"expired to completed", StateExpired, StateCompleted, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.from.CanTransitionTo(tt.to)
			if got != tt.canChange {
				t.Errorf("CanTransitionTo(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.canChange)
			}
		})
	}
}

func TestSessionPause(t *testing.T) {
	session := NewSession("tenant-1", "user-1", 1*time.Hour)

	err := session.Pause()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if session.State != StatePaused {
		t.Errorf("expected state %s, got %s", StatePaused, session.State)
	}
	if session.PausedAt == nil {
		t.Error("expected paused_at to be set")
	}
	if session.Analytics.PauseCount != 1 {
		t.Errorf("expected pause count 1, got %d", session.Analytics.PauseCount)
	}
}

func TestSessionResume(t *testing.T) {
	session := NewSession("tenant-1", "user-1", 1*time.Hour)

	if err := session.Pause(); err != nil {
		t.Fatalf("unexpected error pausing: %v", err)
	}

	// Wait a bit to accumulate paused time
	time.Sleep(10 * time.Millisecond)

	if err := session.Resume(); err != nil {
		t.Fatalf("unexpected error resuming: %v", err)
	}

	if session.State != StateActive {
		t.Errorf("expected state %s, got %s", StateActive, session.State)
	}
	if session.PausedAt != nil {
		t.Error("expected paused_at to be nil")
	}
	if session.Analytics.PausedTimeMs < 10 {
		t.Errorf("expected paused time >= 10ms, got %d", session.Analytics.PausedTimeMs)
	}
}

func TestSessionComplete(t *testing.T) {
	session := NewSession("tenant-1", "user-1", 1*time.Hour)

	err := session.Complete()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if session.State != StateCompleted {
		t.Errorf("expected state %s, got %s", StateCompleted, session.State)
	}
	if session.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestSessionAddItem(t *testing.T) {
	session := NewSession("tenant-1", "user-1", 1*time.Hour)

	item := ReviewItem{
		ID:          "item-1",
		ContentID:   "content-1",
		ContentType: "email",
		Action:      ItemActionApproved,
		ReviewedAt:  time.Now().UTC(),
		TimeSpentMs: 5000,
	}

	err := session.AddItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(session.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(session.Items))
	}
	if session.Analytics.ItemsReviewed != 1 {
		t.Errorf("expected items_reviewed 1, got %d", session.Analytics.ItemsReviewed)
	}
	if session.Analytics.ItemsApproved != 1 {
		t.Errorf("expected items_approved 1, got %d", session.Analytics.ItemsApproved)
	}
	if session.Analytics.TotalTimeMs != 5000 {
		t.Errorf("expected total_time_ms 5000, got %d", session.Analytics.TotalTimeMs)
	}
	if session.StartedAt == nil {
		t.Error("expected started_at to be set on first item")
	}
}

func TestSessionAddItemCompletedSession(t *testing.T) {
	session := NewSession("tenant-1", "user-1", 1*time.Hour)
	_ = session.Complete()

	item := ReviewItem{
		ID:          "item-1",
		ContentID:   "content-1",
		ContentType: "email",
		Action:      ItemActionApproved,
		ReviewedAt:  time.Now().UTC(),
		TimeSpentMs: 5000,
	}

	err := session.AddItem(item)
	if err != ErrSessionAlreadyCompleted {
		t.Errorf("expected ErrSessionAlreadyCompleted, got %v", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	// Create session with very short timeout
	session := NewSession("tenant-1", "user-1", 1*time.Millisecond)

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	if !session.IsExpired() {
		t.Error("expected session to be expired")
	}
}

func TestManagerStartSession(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := &ManagerConfig{
		SessionTimeout:      1 * time.Hour,
		ExpiryCheckInterval: 1 * time.Minute,
		ExpiryBatchSize:     100,
	}

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, err := manager.StartSession(ctx, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if session.ID == "" {
		t.Error("expected session ID to be set")
	}
	if session.State != StateActive {
		t.Errorf("expected state %s, got %s", StateActive, session.State)
	}

	// Verify stored
	stored, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("unexpected error getting session: %v", err)
	}
	if stored.ID != session.ID {
		t.Errorf("stored session ID mismatch: got %s, want %s", stored.ID, session.ID)
	}

	// Verify event published
	events := publisher.Events()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventTypeSessionStarted {
		t.Errorf("expected event type %s, got %s", EventTypeSessionStarted, events[0].Type)
	}
}

func TestManagerStartSessionReturnsExisting(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()

	// Start first session
	session1, err := manager.StartSession(ctx, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Start second session - should return the same one
	session2, err := manager.StartSession(ctx, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if session1.ID != session2.ID {
		t.Errorf("expected same session, got different IDs: %s vs %s", session1.ID, session2.ID)
	}
}

func TestManagerGetSession(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	// Get session
	retrieved, err := manager.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if retrieved.ID != session.ID {
		t.Errorf("expected session ID %s, got %s", session.ID, retrieved.ID)
	}
}

func TestManagerGetSessionNotFound(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	_, err := manager.GetSession(ctx, "nonexistent")

	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestManagerPauseSession(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	// Pause session
	paused, err := manager.PauseSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if paused.State != StatePaused {
		t.Errorf("expected state %s, got %s", StatePaused, paused.State)
	}

	// Verify event
	events := publisher.Events()
	hasEvent := false
	for _, e := range events {
		if e.Type == EventTypeSessionPaused {
			hasEvent = true
			break
		}
	}
	if !hasEvent {
		t.Error("expected session paused event")
	}
}

func TestManagerResumeSession(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	// Pause and resume
	_, _ = manager.PauseSession(ctx, session.ID)
	resumed, err := manager.ResumeSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resumed.State != StateActive {
		t.Errorf("expected state %s, got %s", StateActive, resumed.State)
	}

	// Verify event
	events := publisher.Events()
	hasEvent := false
	for _, e := range events {
		if e.Type == EventTypeSessionResumed {
			hasEvent = true
			break
		}
	}
	if !hasEvent {
		t.Error("expected session resumed event")
	}
}

func TestManagerCompleteSession(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	// Add some items
	_, _ = manager.AddReviewItem(ctx, session.ID, ReviewItem{
		ID:          "item-1",
		ContentID:   "content-1",
		ContentType: "email",
		Action:      ItemActionApproved,
		TimeSpentMs: 1000,
	})

	// Complete session
	completed, err := manager.CompleteSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if completed.State != StateCompleted {
		t.Errorf("expected state %s, got %s", StateCompleted, completed.State)
	}
	if completed.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}

	// Verify event
	events := publisher.Events()
	hasEvent := false
	for _, e := range events {
		if e.Type == EventTypeSessionCompleted {
			hasEvent = true
			break
		}
	}
	if !hasEvent {
		t.Error("expected session completed event")
	}
}

func TestManagerAddReviewItem(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	// Add item
	updated, err := manager.AddReviewItem(ctx, session.ID, ReviewItem{
		ID:          "item-1",
		ContentID:   "content-1",
		ContentType: "email",
		Action:      ItemActionApproved,
		TimeSpentMs: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updated.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(updated.Items))
	}
	if updated.Analytics.ItemsReviewed != 1 {
		t.Errorf("expected items_reviewed 1, got %d", updated.Analytics.ItemsReviewed)
	}

	// Verify event
	events := publisher.Events()
	hasEvent := false
	for _, e := range events {
		if e.Type == EventTypeItemReviewed {
			hasEvent = true
			break
		}
	}
	if !hasEvent {
		t.Error("expected item reviewed event")
	}
}

func TestManagerExtendSession(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := &ManagerConfig{
		SessionTimeout:      1 * time.Hour,
		ExpiryCheckInterval: 1 * time.Minute,
		ExpiryBatchSize:     100,
	}

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	originalExpiry := session.ExpiresAt

	// Extend session
	extended, err := manager.ExtendSession(ctx, session.ID, 2*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !extended.ExpiresAt.After(originalExpiry) {
		t.Error("expected expires_at to be extended")
	}

	// Verify event
	events := publisher.Events()
	hasEvent := false
	for _, e := range events {
		if e.Type == EventTypeSessionExtended {
			hasEvent = true
			break
		}
	}
	if !hasEvent {
		t.Error("expected session extended event")
	}
}

func TestManagerGetSessionAnalytics(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	// Add some items
	items := []ReviewItem{
		{ID: "1", ContentID: "c1", ContentType: "email", Action: ItemActionApproved, TimeSpentMs: 1000},
		{ID: "2", ContentID: "c2", ContentType: "email", Action: ItemActionRejected, TimeSpentMs: 2000},
		{ID: "3", ContentID: "c3", ContentType: "document", Action: ItemActionDeferred, TimeSpentMs: 1500},
	}
	for _, item := range items {
		_, _ = manager.AddReviewItem(ctx, session.ID, item)
	}

	analytics, err := manager.GetSessionAnalytics(ctx, session.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analytics.ItemsReviewed != 3 {
		t.Errorf("expected items_reviewed 3, got %d", analytics.ItemsReviewed)
	}
	if analytics.ItemsApproved != 1 {
		t.Errorf("expected items_approved 1, got %d", analytics.ItemsApproved)
	}
	if analytics.ItemsRejected != 1 {
		t.Errorf("expected items_rejected 1, got %d", analytics.ItemsRejected)
	}
	if analytics.ItemsDeferred != 1 {
		t.Errorf("expected items_deferred 1, got %d", analytics.ItemsDeferred)
	}
	if analytics.TotalTimeMs != 4500 {
		t.Errorf("expected total_time_ms 4500, got %d", analytics.TotalTimeMs)
	}
	if analytics.AverageTimePerItemMs != 1500 {
		t.Errorf("expected average_time_per_item_ms 1500, got %d", analytics.AverageTimePerItemMs)
	}
}

func TestManagerGetSessionEvents(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	// Perform some actions
	_, _ = manager.PauseSession(ctx, session.ID)
	_, _ = manager.ResumeSession(ctx, session.ID)
	_, _ = manager.AddReviewItem(ctx, session.ID, ReviewItem{
		ID:          "item-1",
		ContentID:   "content-1",
		ContentType: "email",
		Action:      ItemActionApproved,
		TimeSpentMs: 1000,
	})

	events, err := manager.GetSessionEvents(ctx, session.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: started, paused, resumed, item_reviewed
	if len(events) < 4 {
		t.Errorf("expected at least 4 events, got %d", len(events))
	}
}

func TestManagerStartStop(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := &ManagerConfig{
		SessionTimeout:      1 * time.Hour,
		ExpiryCheckInterval: 50 * time.Millisecond,
		ExpiryBatchSize:     100,
	}

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()

	// Start manager
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("unexpected error starting: %v", err)
	}

	// Try to start again - should error
	if err := manager.Start(ctx); err == nil {
		t.Error("expected error when starting already running manager")
	}

	// Stop manager
	if err := manager.Stop(); err != nil {
		t.Fatalf("unexpected error stopping: %v", err)
	}
}

func TestManagerExpiryChecker(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := &ManagerConfig{
		SessionTimeout:      10 * time.Millisecond,
		ExpiryCheckInterval: 20 * time.Millisecond,
		ExpiryBatchSize:     100,
	}

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()

	// Create a session
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	// Start manager
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("unexpected error starting: %v", err)
	}

	// Wait for expiry check to run
	time.Sleep(100 * time.Millisecond)

	// Stop manager
	if err := manager.Stop(); err != nil {
		t.Fatalf("unexpected error stopping: %v", err)
	}

	// Session should be expired
	expired, _ := store.Get(ctx, session.ID)
	if expired.State != StateExpired {
		t.Errorf("expected state %s, got %s", StateExpired, expired.State)
	}

	// Should have expired event
	events := publisher.Events()
	hasExpiredEvent := false
	for _, e := range events {
		if e.Type == EventTypeSessionExpired {
			hasExpiredEvent = true
			break
		}
	}
	if !hasExpiredEvent {
		t.Error("expected session expired event")
	}
}

func TestManagerDeleteSession(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()
	session, _ := manager.StartSession(ctx, "tenant-1", "user-1")

	// Delete session
	if err := manager.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not be found
	_, err := manager.GetSession(ctx, session.ID)
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestManagerGetActiveSessions(t *testing.T) {
	store := NewInMemoryStore()
	publisher := NewInMemoryPublisher()
	logger := testLogger()
	config := DefaultManagerConfig()

	manager := NewManager(store, publisher, logger, config)

	ctx := context.Background()

	// Create sessions for different users
	_, _ = manager.StartSession(ctx, "tenant-1", "user-1")
	_, _ = manager.StartSession(ctx, "tenant-1", "user-2")

	// Get active sessions for user-1
	sessions, err := manager.GetActiveSessions(ctx, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

func TestSessionClone(t *testing.T) {
	session := NewSession("tenant-1", "user-1", 1*time.Hour)
	session.Metadata["key"] = "value"
	_ = session.AddItem(ReviewItem{
		ID:          "item-1",
		ContentID:   "content-1",
		ContentType: "email",
		Action:      ItemActionApproved,
		TimeSpentMs: 1000,
	})

	clone := session.Clone()

	// Modify original
	session.Metadata["key"] = "modified"
	session.Items[0].ID = "modified"

	// Clone should be unchanged
	if clone.Metadata["key"] != "value" {
		t.Error("clone metadata was modified")
	}
	if clone.Items[0].ID != "item-1" {
		t.Error("clone items were modified")
	}
}

func TestSessionJSON(t *testing.T) {
	session := NewSession("tenant-1", "user-1", 1*time.Hour)
	_ = session.AddItem(ReviewItem{
		ID:          "item-1",
		ContentID:   "content-1",
		ContentType: "email",
		Action:      ItemActionApproved,
		TimeSpentMs: 1000,
	})

	// Serialize
	data, err := session.ToJSON()
	if err != nil {
		t.Fatalf("unexpected error serializing: %v", err)
	}

	// Deserialize
	restored, err := SessionFromJSON(data)
	if err != nil {
		t.Fatalf("unexpected error deserializing: %v", err)
	}

	if restored.ID != session.ID {
		t.Errorf("ID mismatch: got %s, want %s", restored.ID, session.ID)
	}
	if len(restored.Items) != len(session.Items) {
		t.Errorf("items count mismatch: got %d, want %d", len(restored.Items), len(session.Items))
	}
}

func TestInMemoryStore(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	session := NewSession("tenant-1", "user-1", 1*time.Hour)

	// Save
	if err := store.Save(ctx, session); err != nil {
		t.Fatalf("unexpected error saving: %v", err)
	}

	// Get
	retrieved, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("unexpected error getting: %v", err)
	}
	if retrieved.ID != session.ID {
		t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, session.ID)
	}

	// GetByTenantAndUser
	sessions, err := store.GetByTenantAndUser(ctx, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}

	// Delete
	if err := store.Delete(ctx, session.ID); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	// Should not be found
	_, err = store.Get(ctx, session.ID)
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestInMemoryStoreEvents(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	session := NewSession("tenant-1", "user-1", 1*time.Hour)
	_ = store.Save(ctx, session)

	event := NewEvent(EventTypeSessionStarted, session.ID, "tenant-1", "user-1")

	// Save event
	if err := store.SaveEvent(ctx, event); err != nil {
		t.Fatalf("unexpected error saving event: %v", err)
	}

	// Get events
	events, err := store.GetEvents(ctx, session.ID)
	if err != nil {
		t.Fatalf("unexpected error getting events: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestItemActionAnalytics(t *testing.T) {
	session := NewSession("tenant-1", "user-1", 1*time.Hour)

	actions := []ItemAction{
		ItemActionApproved,
		ItemActionApproved,
		ItemActionRejected,
		ItemActionDeferred,
		ItemActionSkipped,
		ItemActionSkipped,
	}

	for i, action := range actions {
		_ = session.AddItem(ReviewItem{
			ID:          string(rune('a' + i)),
			ContentID:   string(rune('a' + i)),
			ContentType: "email",
			Action:      action,
			TimeSpentMs: 1000,
		})
	}

	if session.Analytics.ItemsApproved != 2 {
		t.Errorf("expected 2 approved, got %d", session.Analytics.ItemsApproved)
	}
	if session.Analytics.ItemsRejected != 1 {
		t.Errorf("expected 1 rejected, got %d", session.Analytics.ItemsRejected)
	}
	if session.Analytics.ItemsDeferred != 1 {
		t.Errorf("expected 1 deferred, got %d", session.Analytics.ItemsDeferred)
	}
	if session.Analytics.ItemsSkipped != 2 {
		t.Errorf("expected 2 skipped, got %d", session.Analytics.ItemsSkipped)
	}
}

func TestSessionDuration(t *testing.T) {
	session := NewSession("tenant-1", "user-1", 1*time.Hour)

	// No duration before first item
	if session.Duration() != 0 {
		t.Errorf("expected 0 duration before first item, got %v", session.Duration())
	}

	// Add item to start session
	now := time.Now().UTC()
	session.StartedAt = &now
	session.UpdatedAt = now.Add(100 * time.Millisecond)

	duration := session.Duration()
	if duration < 100*time.Millisecond || duration > 150*time.Millisecond {
		t.Errorf("unexpected duration: %v", duration)
	}
}
