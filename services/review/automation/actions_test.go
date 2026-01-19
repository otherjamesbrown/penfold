package automation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestActionRegistry(t *testing.T) {
	registry := NewActionRegistry()

	// All built-in executors should be registered
	actionTypes := []ActionType{
		ActionAccept,
		ActionReject,
		ActionDefer,
		ActionEscalate,
		ActionTag,
		ActionNotify,
		ActionRoute,
	}

	for _, at := range actionTypes {
		executor, err := registry.GetExecutor(at)
		if err != nil {
			t.Errorf("expected executor for %s, got error: %v", at, err)
		}
		if executor == nil {
			t.Errorf("expected non-nil executor for %s", at)
		}
	}

	// Unknown action type
	_, err := registry.GetExecutor("unknown")
	if err == nil {
		t.Error("expected error for unknown action type")
	}
}

func TestAcceptExecutor(t *testing.T) {
	ctx := context.Background()

	t.Run("without handler", func(t *testing.T) {
		executor := &AcceptExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionAccept, "rule-1", item.ID)

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !action.Executed {
			t.Error("expected action to be marked as executed")
		}
		if action.ExecutedAt == nil {
			t.Error("expected executed_at to be set")
		}
	})

	t.Run("with handler success", func(t *testing.T) {
		handlerCalled := false
		handler := func(ctx context.Context, itemID string) error {
			handlerCalled = true
			return nil
		}

		executor := NewAcceptExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionAccept, "rule-1", item.ID)

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !handlerCalled {
			t.Error("expected handler to be called")
		}
	})

	t.Run("with handler error", func(t *testing.T) {
		handler := func(ctx context.Context, itemID string) error {
			return errors.New("handler error")
		}

		executor := NewAcceptExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionAccept, "rule-1", item.ID)

		err := executor.Execute(ctx, action, item)
		if err == nil {
			t.Error("expected error")
		}

		if action.Error == "" {
			t.Error("expected action error to be set")
		}
	})
}

func TestRejectExecutor(t *testing.T) {
	ctx := context.Background()

	t.Run("without handler", func(t *testing.T) {
		executor := &RejectExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionReject, "rule-1", item.ID)
		action.Parameters["reason"] = "spam"

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !action.Executed {
			t.Error("expected action to be marked as executed")
		}
	})

	t.Run("with handler success", func(t *testing.T) {
		var capturedReason string
		handler := func(ctx context.Context, itemID string, reason string) error {
			capturedReason = reason
			return nil
		}

		executor := NewRejectExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionReject, "rule-1", item.ID)
		action.Parameters["reason"] = "spam"

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedReason != "spam" {
			t.Errorf("expected reason 'spam', got '%s'", capturedReason)
		}
	})
}

func TestDeferExecutor(t *testing.T) {
	ctx := context.Background()

	t.Run("without handler", func(t *testing.T) {
		executor := &DeferExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionDefer, "rule-1", item.ID)
		action.DeferDuration = 2 * time.Hour

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !action.Executed {
			t.Error("expected action to be marked as executed")
		}
	})

	t.Run("with handler using parameters", func(t *testing.T) {
		var capturedUntil time.Time
		handler := func(ctx context.Context, itemID string, until time.Time) error {
			capturedUntil = until
			return nil
		}

		executor := NewDeferExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionDefer, "rule-1", item.ID)
		action.Parameters["hours"] = 3.0

		beforeExec := time.Now()
		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check that until is approximately 3 hours from now
		expected := beforeExec.Add(3 * time.Hour)
		if capturedUntil.Before(expected.Add(-time.Minute)) || capturedUntil.After(expected.Add(time.Minute)) {
			t.Errorf("expected defer until ~%v, got %v", expected, capturedUntil)
		}
	})
}

func TestEscalateExecutor(t *testing.T) {
	ctx := context.Background()

	t.Run("without handler", func(t *testing.T) {
		executor := &EscalateExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionEscalate, "rule-1", item.ID)
		action.EscalationLevel = 2

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !action.Executed {
			t.Error("expected action to be marked as executed")
		}
	})

	t.Run("with handler", func(t *testing.T) {
		var capturedLevel int
		handler := func(ctx context.Context, itemID string, level int) error {
			capturedLevel = level
			return nil
		}

		executor := NewEscalateExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionEscalate, "rule-1", item.ID)
		action.EscalationLevel = 3

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedLevel != 3 {
			t.Errorf("expected level 3, got %d", capturedLevel)
		}
	})
}

func TestTagExecutor(t *testing.T) {
	ctx := context.Background()

	t.Run("without handler", func(t *testing.T) {
		executor := &TagExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionTag, "rule-1", item.ID)
		action.Tags = []string{"automated", "urgent"}

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !action.Executed {
			t.Error("expected action to be marked as executed")
		}
	})

	t.Run("no tags specified", func(t *testing.T) {
		executor := &TagExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionTag, "rule-1", item.ID)

		err := executor.Execute(ctx, action, item)
		if err == nil {
			t.Error("expected error for no tags")
		}
	})

	t.Run("with handler", func(t *testing.T) {
		var capturedTags []string
		handler := func(ctx context.Context, itemID string, tags []string) error {
			capturedTags = tags
			return nil
		}

		executor := NewTagExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionTag, "rule-1", item.ID)
		action.Tags = []string{"important", "followup"}

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(capturedTags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(capturedTags))
		}
	})

	t.Run("tags from parameters", func(t *testing.T) {
		var capturedTags []string
		handler := func(ctx context.Context, itemID string, tags []string) error {
			capturedTags = tags
			return nil
		}

		executor := NewTagExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionTag, "rule-1", item.ID)
		action.Parameters["tags"] = []interface{}{"tag1", "tag2"}

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(capturedTags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(capturedTags))
		}
	})
}

func TestNotifyExecutor(t *testing.T) {
	ctx := context.Background()

	t.Run("without handler", func(t *testing.T) {
		executor := &NotifyExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionNotify, "rule-1", item.ID)
		action.NotificationTarget = "user@example.com"
		action.Message = "Item needs attention"

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !action.Executed {
			t.Error("expected action to be marked as executed")
		}
	})

	t.Run("no target specified", func(t *testing.T) {
		executor := &NotifyExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionNotify, "rule-1", item.ID)

		err := executor.Execute(ctx, action, item)
		if err == nil {
			t.Error("expected error for no target")
		}
	})

	t.Run("with handler", func(t *testing.T) {
		var capturedTarget, capturedMessage string
		handler := func(ctx context.Context, itemID string, target, message string) error {
			capturedTarget = target
			capturedMessage = message
			return nil
		}

		executor := NewNotifyExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionNotify, "rule-1", item.ID)
		action.NotificationTarget = "admin@example.com"
		action.Message = "Urgent review needed"

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedTarget != "admin@example.com" {
			t.Errorf("expected target 'admin@example.com', got '%s'", capturedTarget)
		}
		if capturedMessage != "Urgent review needed" {
			t.Errorf("unexpected message: %s", capturedMessage)
		}
	})
}

func TestRouteExecutor(t *testing.T) {
	ctx := context.Background()

	t.Run("without handler", func(t *testing.T) {
		executor := &RouteExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionRoute, "rule-1", item.ID)
		action.RouteTo = "escalation-queue"

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !action.Executed {
			t.Error("expected action to be marked as executed")
		}
	})

	t.Run("no target specified", func(t *testing.T) {
		executor := &RouteExecutor{}

		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionRoute, "rule-1", item.ID)

		err := executor.Execute(ctx, action, item)
		if err == nil {
			t.Error("expected error for no target")
		}
	})

	t.Run("with handler", func(t *testing.T) {
		var capturedTarget string
		handler := func(ctx context.Context, itemID string, target string) error {
			capturedTarget = target
			return nil
		}

		executor := NewRouteExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionRoute, "rule-1", item.ID)
		action.RouteTo = "legal-review"

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedTarget != "legal-review" {
			t.Errorf("expected target 'legal-review', got '%s'", capturedTarget)
		}
	})

	t.Run("target from parameters", func(t *testing.T) {
		var capturedTarget string
		handler := func(ctx context.Context, itemID string, target string) error {
			capturedTarget = target
			return nil
		}

		executor := NewRouteExecutor(handler)
		item := &ReviewItem{ID: "item-1"}
		action := NewAction(ActionRoute, "rule-1", item.ID)
		action.Parameters["target"] = "priority-queue"

		err := executor.Execute(ctx, action, item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedTarget != "priority-queue" {
			t.Errorf("expected target 'priority-queue', got '%s'", capturedTarget)
		}
	})
}

func TestActionRegistryExecuteDryRun(t *testing.T) {
	ctx := context.Background()
	registry := NewActionRegistry()

	item := &ReviewItem{ID: "item-1"}
	action := NewAction(ActionAccept, "rule-1", item.ID)
	action.DryRun = true

	err := registry.Execute(ctx, action, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !action.Executed {
		t.Error("expected action to be marked as executed")
	}
	if action.ExecutedAt == nil {
		t.Error("expected executed_at to be set")
	}
	if action.Message == "" {
		t.Error("expected dry-run message to be set")
	}
}

func TestNoOpHandlers(t *testing.T) {
	ctx := context.Background()
	handlers := NoOpHandlers()

	// All handlers should execute without error
	if err := handlers.Accept(ctx, "item-1"); err != nil {
		t.Errorf("unexpected error from Accept: %v", err)
	}
	if err := handlers.Reject(ctx, "item-1", "reason"); err != nil {
		t.Errorf("unexpected error from Reject: %v", err)
	}
	if err := handlers.Defer(ctx, "item-1", time.Now()); err != nil {
		t.Errorf("unexpected error from Defer: %v", err)
	}
	if err := handlers.Escalate(ctx, "item-1", 1); err != nil {
		t.Errorf("unexpected error from Escalate: %v", err)
	}
	if err := handlers.Tag(ctx, "item-1", []string{"tag"}); err != nil {
		t.Errorf("unexpected error from Tag: %v", err)
	}
	if err := handlers.Notify(ctx, "item-1", "target", "msg"); err != nil {
		t.Errorf("unexpected error from Notify: %v", err)
	}
	if err := handlers.Route(ctx, "item-1", "queue"); err != nil {
		t.Errorf("unexpected error from Route: %v", err)
	}
}

func TestActionResult(t *testing.T) {
	action := NewAction(ActionAccept, "rule-1", "item-1")
	result := NewActionResult(action)

	if result.Success {
		t.Error("expected success to be false initially")
	}

	// Mark success
	result.MarkSuccess()

	if !result.Success {
		t.Error("expected success to be true after MarkSuccess")
	}
	if !result.Action.Executed {
		t.Error("expected action to be marked as executed")
	}

	// Mark failed
	action2 := NewAction(ActionReject, "rule-1", "item-1")
	result2 := NewActionResult(action2)
	result2.MarkFailed(errors.New("test error"))

	if result2.Success {
		t.Error("expected success to be false after MarkFailed")
	}
	if result2.Error != "test error" {
		t.Errorf("expected error 'test error', got '%s'", result2.Error)
	}
	if result2.Action.Error != "test error" {
		t.Errorf("expected action error 'test error', got '%s'", result2.Action.Error)
	}
}

func TestCompositeExecutor(t *testing.T) {
	ctx := context.Background()

	var callOrder []string
	executor1 := &mockExecutor{
		name:       "first",
		canExecute: func(at ActionType) bool { return at == ActionTag },
		execute: func(ctx context.Context, action *Action, item *ReviewItem) error {
			callOrder = append(callOrder, "first")
			return nil
		},
	}
	executor2 := &mockExecutor{
		name:       "second",
		canExecute: func(at ActionType) bool { return at == ActionTag },
		execute: func(ctx context.Context, action *Action, item *ReviewItem) error {
			callOrder = append(callOrder, "second")
			return nil
		},
	}

	composite := NewCompositeExecutor("composite", executor1, executor2)

	item := &ReviewItem{ID: "item-1"}
	action := NewAction(ActionTag, "rule-1", item.ID)

	err := composite.Execute(ctx, action, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(callOrder) != 2 {
		t.Errorf("expected 2 calls, got %d", len(callOrder))
	}
	if callOrder[0] != "first" || callOrder[1] != "second" {
		t.Errorf("unexpected call order: %v", callOrder)
	}

	if !composite.CanExecute(ActionTag) {
		t.Error("expected composite to handle ActionTag")
	}
	if composite.Name() != "composite" {
		t.Errorf("expected name 'composite', got '%s'", composite.Name())
	}
}

// mockExecutor is a test helper.
type mockExecutor struct {
	name       string
	canExecute func(ActionType) bool
	execute    func(context.Context, *Action, *ReviewItem) error
}

func (m *mockExecutor) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	return m.execute(ctx, action, item)
}

func (m *mockExecutor) CanExecute(at ActionType) bool {
	return m.canExecute(at)
}

func (m *mockExecutor) Name() string {
	return m.name
}
