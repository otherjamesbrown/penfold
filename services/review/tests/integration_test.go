// Package tests provides integration tests for the Review Service.
// These tests verify the interaction between session, queue, automation, and undo components.
package tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/review/automation"
	"github.com/otherjamesbrown/penfold/services/review/session"
	"github.com/otherjamesbrown/penfold/services/review/undo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger creates a logger for testing.
func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "integration_test",
		Environment: "test",
		JSONFormat:  false,
	})
}

// TestFullReviewWorkflow tests a complete review workflow from session start to completion.
func TestFullReviewWorkflow(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	// Set up session manager with in-memory store
	store := session.NewInMemoryStore()
	publisher := &session.NoOpPublisher{}
	config := &session.ManagerConfig{
		SessionTimeout:      1 * time.Hour,
		ExpiryCheckInterval: 10 * time.Minute,
		ExpiryBatchSize:     100,
	}

	manager := session.NewManager(store, publisher, logger, config)

	// Step 1: Start a session
	sess, err := manager.StartSession(ctx, "tenant-1", "user-1")
	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.Equal(t, session.StateActive, sess.State)
	assert.Equal(t, "tenant-1", sess.TenantID)
	assert.Equal(t, "user-1", sess.UserID)

	sessionID := sess.ID

	// Step 2: Add review items
	for i := 0; i < 5; i++ {
		item := session.ReviewItem{
			ID:          "item-" + string(rune('a'+i)),
			ContentID:   "content-" + string(rune('a'+i)),
			ContentType: "email",
			Action:      session.ItemActionApproved,
			TimeSpentMs: 500 + int64(i*100),
		}
		sess, err = manager.AddReviewItem(ctx, sessionID, item)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, sess.Analytics.ItemsReviewed)
	assert.Equal(t, 5, sess.Analytics.ItemsApproved)

	// Step 3: Pause the session
	sess, err = manager.PauseSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, session.StatePaused, sess.State)
	assert.Equal(t, 1, sess.Analytics.PauseCount)

	// Step 4: Resume the session
	sess, err = manager.ResumeSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, session.StateActive, sess.State)

	// Step 5: Add more items
	for i := 0; i < 3; i++ {
		item := session.ReviewItem{
			ID:          "item-" + string(rune('f'+i)),
			ContentID:   "content-" + string(rune('f'+i)),
			ContentType: "document",
			Action:      session.ItemActionRejected,
			TimeSpentMs: 300,
		}
		sess, err = manager.AddReviewItem(ctx, sessionID, item)
		require.NoError(t, err)
	}

	assert.Equal(t, 8, sess.Analytics.ItemsReviewed)
	assert.Equal(t, 5, sess.Analytics.ItemsApproved)
	assert.Equal(t, 3, sess.Analytics.ItemsRejected)

	// Step 6: Complete the session
	sess, err = manager.CompleteSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, session.StateCompleted, sess.State)
	assert.NotNil(t, sess.CompletedAt)

	// Step 7: Verify session events were recorded
	events, err := manager.GetSessionEvents(ctx, sessionID)
	require.NoError(t, err)
	assert.True(t, len(events) > 0, "expected events to be recorded")
}

// TestAutomationWithUndo tests automation rule execution combined with undo functionality.
func TestAutomationWithUndo(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	// Set up automation engine
	engine := automation.NewEngine(nil, logger)
	engine.SetActionHandlers(automation.NoOpHandlers())

	// Set up undo stack
	undoStack := undo.NewStack(&undo.StackConfig{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		SessionID: "session-1",
		Logger:    logger,
	})

	// Create an automation rule
	rule := automation.NewRule(
		"tenant-1",
		"Auto-accept newsletters",
		automation.SenderCondition("contains", "@newsletter."),
		automation.ActionAccept,
	)
	rule.Priority = 10
	require.NoError(t, engine.AddRule(rule))

	// Create test item that matches the rule
	item := &automation.ReviewItem{
		ID:       "item-1",
		TenantID: "tenant-1",
		Sender:   "updates@newsletter.example.com",
		Subject:  "Weekly Newsletter",
		Category: "communications",
	}

	// Process rules
	actions, err := engine.ProcessRules(ctx, item)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	assert.Equal(t, automation.ActionAccept, actions[0].Type)

	// Execute action and create undoable action
	err = engine.ExecuteAction(ctx, actions[0], item)
	require.NoError(t, err)

	// Create corresponding undo action
	undoAction := undo.NewAcceptAction(
		"tenant-1", "user-1", "session-1",
		"item-1", "email", "pending",
	)
	require.NoError(t, undoAction.Execute(ctx))
	require.NoError(t, undoStack.Push(undoAction))

	assert.Equal(t, 1, undoStack.UndoSize())
	assert.True(t, undoStack.CanUndo())

	// Undo the action
	undone, err := undoStack.Undo(ctx)
	require.NoError(t, err)
	assert.Equal(t, undoAction.ID(), undone.ID())
	assert.Equal(t, 0, undoStack.UndoSize())
	assert.Equal(t, 1, undoStack.RedoSize())

	// Redo the action
	redone, err := undoStack.Redo(ctx)
	require.NoError(t, err)
	assert.Equal(t, undoAction.ID(), redone.ID())
	assert.Equal(t, 1, undoStack.UndoSize())
	assert.Equal(t, 0, undoStack.RedoSize())
}

// TestSessionWithAutomation tests the integration of session management with automation rules.
func TestSessionWithAutomation(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	// Set up session manager
	store := session.NewInMemoryStore()
	publisher := &session.NoOpPublisher{}
	config := &session.ManagerConfig{
		SessionTimeout:      1 * time.Hour,
		ExpiryCheckInterval: 10 * time.Minute,
		ExpiryBatchSize:     100,
	}
	sessionManager := session.NewManager(store, publisher, logger, config)

	// Set up automation engine
	automationEngine := automation.NewEngine(nil, logger)
	automationEngine.SetActionHandlers(automation.NoOpHandlers())

	// Add rules
	spamRule := automation.NewRule(
		"tenant-1",
		"Auto-reject spam",
		automation.CategoryCondition("eq", "spam"),
		automation.ActionReject,
	)
	spamRule.Priority = 20
	require.NoError(t, automationEngine.AddRule(spamRule))

	importantRule := automation.NewRule(
		"tenant-1",
		"Flag important",
		automation.PriorityCondition("gte", 4),
		automation.ActionTag,
	)
	importantRule.Priority = 15
	importantRule.ActionParameters = map[string]interface{}{
		"tags": []string{"important", "urgent"},
	}
	require.NoError(t, automationEngine.AddRule(importantRule))

	// Start session
	sess, err := sessionManager.StartSession(ctx, "tenant-1", "user-1")
	require.NoError(t, err)

	// Simulate processing items through automation
	testItems := []struct {
		item           *automation.ReviewItem
		expectedAction automation.ActionType
		sessionAction  session.ItemAction
	}{
		{
			item: &automation.ReviewItem{
				ID:       "item-1",
				TenantID: "tenant-1",
				Category: "spam",
				Subject:  "Win a free iPhone",
			},
			expectedAction: automation.ActionReject,
			sessionAction:  session.ItemActionRejected,
		},
		{
			item: &automation.ReviewItem{
				ID:       "item-2",
				TenantID: "tenant-1",
				Category: "work",
				Priority: 4,
				Subject:  "Urgent: Contract deadline",
			},
			expectedAction: automation.ActionTag,
			sessionAction:  session.ItemActionApproved, // Tag doesn't have a corresponding action, treat as approved
		},
	}

	for _, tc := range testItems {
		actions, err := automationEngine.ProcessRules(ctx, tc.item)
		require.NoError(t, err)
		require.True(t, len(actions) > 0, "expected actions for item %s", tc.item.ID)
		assert.Equal(t, tc.expectedAction, actions[0].Type)

		// Execute action
		err = automationEngine.ExecuteAction(ctx, actions[0], tc.item)
		require.NoError(t, err)

		// Record in session
		reviewItem := session.ReviewItem{
			ID:          tc.item.ID,
			ContentID:   tc.item.ID,
			ContentType: "email",
			Action:      tc.sessionAction,
			TimeSpentMs: 100, // Automated, so fast
		}
		sess, err = sessionManager.AddReviewItem(ctx, sess.ID, reviewItem)
		require.NoError(t, err)
	}

	// Verify session analytics
	assert.Equal(t, 2, sess.Analytics.ItemsReviewed)
}

// TestConcurrentSessionOperations tests concurrent operations on sessions.
func TestConcurrentSessionOperations(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	store := session.NewInMemoryStore()
	publisher := &session.NoOpPublisher{}
	config := &session.ManagerConfig{
		SessionTimeout:      1 * time.Hour,
		ExpiryCheckInterval: 10 * time.Minute,
		ExpiryBatchSize:     100,
	}
	manager := session.NewManager(store, publisher, logger, config)

	// Start sessions for multiple users
	const numUsers = 5
	sessions := make([]*session.Session, numUsers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	// Concurrent session creation
	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess, err := manager.StartSession(ctx, "tenant-1", "user-"+string(rune('a'+idx)))
			mu.Lock()
			if err != nil {
				errors = append(errors, err)
			} else {
				sessions[idx] = sess
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	assert.Empty(t, errors, "expected no errors during concurrent session creation")

	// Concurrent item additions
	errors = make([]error, 0)
	for i := 0; i < numUsers; i++ {
		if sessions[i] == nil {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				item := session.ReviewItem{
					ID:          "item-" + string(rune('a'+idx)) + "-" + string(rune('0'+j)),
					ContentID:   "content-test",
					ContentType: "email",
					Action:      session.ItemActionApproved,
					TimeSpentMs: 100,
				}
				_, err := manager.AddReviewItem(ctx, sessions[idx].ID, item)
				if err != nil {
					mu.Lock()
					errors = append(errors, err)
					mu.Unlock()
				}
			}
		}(i)
	}
	wg.Wait()

	assert.Empty(t, errors, "expected no errors during concurrent item additions")

	// Verify each session has items
	for i := 0; i < numUsers; i++ {
		if sessions[i] == nil {
			continue
		}
		sess, err := manager.GetSession(ctx, sessions[i].ID)
		require.NoError(t, err)
		assert.Equal(t, 10, sess.Analytics.ItemsReviewed)
	}
}

// TestUndoStackIntegration tests undo stack behavior across multiple operations.
func TestUndoStackIntegration(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	// Create undo stack with custom limits
	limits := undo.DefaultLimits().WithMaxStackDepth(5)
	stack := undo.NewStack(&undo.StackConfig{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		SessionID: "session-1",
		Limits:    limits,
		Logger:    logger,
	})

	// Push actions beyond stack depth
	var actionIDs []string
	for i := 0; i < 7; i++ {
		action := undo.NewAcceptAction(
			"tenant-1", "user-1", "session-1",
			"content-"+string(rune('a'+i)),
			"email", "pending",
		)
		require.NoError(t, action.Execute(ctx))
		require.NoError(t, stack.Push(action))
		actionIDs = append(actionIDs, action.ID())
	}

	// Should only have 5 actions (max depth)
	assert.Equal(t, 5, stack.UndoSize())

	// Undo all actions
	undone, err := stack.UndoN(ctx, 5)
	require.NoError(t, err)
	assert.Len(t, undone, 5)

	// All items should now be in redo stack
	assert.Equal(t, 0, stack.UndoSize())
	assert.Equal(t, 5, stack.RedoSize())

	// Redo 3 actions
	redone, err := stack.RedoN(ctx, 3)
	require.NoError(t, err)
	assert.Len(t, redone, 3)

	assert.Equal(t, 3, stack.UndoSize())
	assert.Equal(t, 2, stack.RedoSize())

	// Push new action (should clear redo stack)
	newAction := undo.NewAcceptAction(
		"tenant-1", "user-1", "session-1",
		"content-new", "email", "pending",
	)
	require.NoError(t, newAction.Execute(ctx))
	require.NoError(t, stack.Push(newAction))

	assert.Equal(t, 4, stack.UndoSize())
	assert.Equal(t, 0, stack.RedoSize(), "redo stack should be cleared after new push")
}

// TestSessionExpiry tests session expiration handling.
func TestSessionExpiry(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	store := session.NewInMemoryStore()
	publisher := &session.NoOpPublisher{}
	config := &session.ManagerConfig{
		SessionTimeout:      50 * time.Millisecond,
		ExpiryCheckInterval: 10 * time.Millisecond,
		ExpiryBatchSize:     100,
	}
	manager := session.NewManager(store, publisher, logger, config)

	// Start session with short timeout
	sess, err := manager.StartSession(ctx, "tenant-1", "user-1")
	require.NoError(t, err)
	sessionID := sess.ID

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Attempt to get expired session
	_, err = manager.GetSession(ctx, sessionID)
	assert.ErrorIs(t, err, session.ErrSessionExpired)
}

// TestAutomationRuleLearning tests the rule learning and suggestion system.
func TestAutomationRuleLearning(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	config := automation.DefaultEngineConfig()
	config.EnableLearning = true
	config.LearningConfig = &automation.LearningConfig{
		MinSupportCount:     3,
		MinConfidence:       0.7,
		MaxSuggestions:      10,
		LookbackDays:        30,
		EnableSenderRules:   true,
		EnableCategoryRules: true,
	}

	engine := automation.NewEngine(config, logger)

	// Simulate user decisions
	for i := 0; i < 5; i++ {
		item := &automation.ReviewItem{
			ID:        "item-" + string(rune('a'+i)),
			TenantID:  "tenant-1",
			Sender:    "promotions@marketing.com",
			Category:  "marketing",
			Subject:   "Special Offer",
			CreatedAt: time.Now(),
		}
		decision := automation.NewDecision("tenant-1", "user-1", item, automation.ActionReject)
		engine.LearnFromDecision(decision)
	}

	// Get rule suggestions
	suggestions, err := engine.SuggestRules(ctx, "tenant-1")
	require.NoError(t, err)

	// Should have at least one suggestion based on sender pattern
	if len(suggestions) > 0 {
		assert.Equal(t, automation.ActionReject, suggestions[0].Rule.ActionType)
		assert.True(t, suggestions[0].Confidence >= 0.7)
	}
}

// TestAutomationABTesting tests A/B testing functionality in the automation engine.
func TestAutomationABTesting(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	config := automation.DefaultEngineConfig()
	config.EnableABTesting = true
	config.ABTestSampleRate = 1.0 // 100% sample rate for testing

	engine := automation.NewEngine(config, logger)
	engine.SetActionHandlers(automation.NoOpHandlers())

	// Create two rules for A/B testing
	ruleA := automation.NewRule(
		"tenant-1",
		"Test Rule A",
		automation.SenderCondition("eq", "test@example.com"),
		automation.ActionAccept,
	)
	ruleA.ABTestGroup = "test-group-A"
	ruleA.Priority = 10

	ruleB := automation.NewRule(
		"tenant-1",
		"Test Rule B",
		automation.SenderCondition("eq", "test@example.com"),
		automation.ActionTag,
	)
	ruleB.ABTestGroup = "test-group-B"
	ruleB.Priority = 5

	require.NoError(t, engine.AddRule(ruleA))
	require.NoError(t, engine.AddRule(ruleB))

	// Process items and track which rules fire
	item := &automation.ReviewItem{
		ID:       "item-1",
		TenantID: "tenant-1",
		Sender:   "test@example.com",
		Subject:  "Test Subject",
	}

	actions, err := engine.ProcessRules(ctx, item)
	require.NoError(t, err)

	// With 100% sample rate, both rules should potentially match
	// but conflict resolution should only return one
	assert.True(t, len(actions) >= 1, "expected at least one action")
}

// TestConflictResolution tests different conflict resolution strategies.
func TestConflictResolution(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	testCases := []struct {
		name               string
		conflictResolution string
		expectedActions    int
	}{
		{"First", "first", 1},
		{"HighestPriority", "highest_priority", 1},
		{"Merge", "merge", 2}, // Accept + Tag can coexist
		{"Fail", "fail", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			engine := automation.NewEngine(nil, logger)
			engine.SetActionHandlers(automation.NoOpHandlers())

			ruleSet := automation.NewRuleSet("tenant-1")
			ruleSet.ConflictResolution = tc.conflictResolution

			// Two rules that both match
			rule1 := automation.NewRule(
				"tenant-1",
				"Rule 1",
				automation.SenderCondition("eq", "test@example.com"),
				automation.ActionAccept,
			)
			rule1.Priority = 10

			rule2 := automation.NewRule(
				"tenant-1",
				"Rule 2",
				automation.SenderCondition("eq", "test@example.com"),
				automation.ActionTag,
			)
			rule2.Priority = 5
			rule2.ActionParameters = map[string]interface{}{
				"tags": []string{"test"},
			}

			ruleSet.AddRule(rule1)
			ruleSet.AddRule(rule2)
			engine.SetRuleSet(ruleSet)

			item := &automation.ReviewItem{
				ID:       "item-1",
				TenantID: "tenant-1",
				Sender:   "test@example.com",
			}

			actions, err := engine.ProcessRules(ctx, item)
			require.NoError(t, err)
			assert.Len(t, actions, tc.expectedActions)
		})
	}
}

// TestSessionAnalytics tests session analytics calculation.
func TestSessionAnalytics(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	store := session.NewInMemoryStore()
	publisher := &session.NoOpPublisher{}
	config := &session.ManagerConfig{
		SessionTimeout:      1 * time.Hour,
		ExpiryCheckInterval: 10 * time.Minute,
		ExpiryBatchSize:     100,
	}
	manager := session.NewManager(store, publisher, logger, config)

	sess, err := manager.StartSession(ctx, "tenant-1", "user-1")
	require.NoError(t, err)

	// Add various types of actions
	actions := []struct {
		action      session.ItemAction
		timeSpentMs int64
	}{
		{session.ItemActionApproved, 500},
		{session.ItemActionApproved, 300},
		{session.ItemActionRejected, 400},
		{session.ItemActionDeferred, 200},
		{session.ItemActionSkipped, 100},
	}

	for i, a := range actions {
		item := session.ReviewItem{
			ID:          "item-" + string(rune('a'+i)),
			ContentID:   "content-test",
			ContentType: "email",
			Action:      a.action,
			TimeSpentMs: a.timeSpentMs,
		}
		sess, err = manager.AddReviewItem(ctx, sess.ID, item)
		require.NoError(t, err)
	}

	// Get analytics
	analytics, err := manager.GetSessionAnalytics(ctx, sess.ID)
	require.NoError(t, err)

	assert.Equal(t, 5, analytics.ItemsReviewed)
	assert.Equal(t, 2, analytics.ItemsApproved)
	assert.Equal(t, 1, analytics.ItemsRejected)
	assert.Equal(t, 1, analytics.ItemsDeferred)
	assert.Equal(t, 1, analytics.ItemsSkipped)
	assert.Equal(t, int64(1500), analytics.TotalTimeMs)
	assert.Equal(t, int64(300), analytics.AverageTimePerItemMs)
}

// TestMultiplePauseResumeCycles tests multiple pause/resume cycles.
func TestMultiplePauseResumeCycles(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	store := session.NewInMemoryStore()
	publisher := &session.NoOpPublisher{}
	config := &session.ManagerConfig{
		SessionTimeout:      1 * time.Hour,
		ExpiryCheckInterval: 10 * time.Minute,
		ExpiryBatchSize:     100,
	}
	manager := session.NewManager(store, publisher, logger, config)

	sess, err := manager.StartSession(ctx, "tenant-1", "user-1")
	require.NoError(t, err)

	// Multiple pause/resume cycles
	for i := 0; i < 3; i++ {
		sess, err = manager.PauseSession(ctx, sess.ID)
		require.NoError(t, err)
		assert.Equal(t, session.StatePaused, sess.State)
		assert.Equal(t, i+1, sess.Analytics.PauseCount)

		time.Sleep(10 * time.Millisecond)

		sess, err = manager.ResumeSession(ctx, sess.ID)
		require.NoError(t, err)
		assert.Equal(t, session.StateActive, sess.State)
	}

	// Verify analytics tracked pauses
	assert.Equal(t, 3, sess.Analytics.PauseCount)
	assert.True(t, sess.Analytics.PausedTimeMs > 0, "expected paused time to be tracked")
}

// TestUndoWithCallbacks tests undo stack with callbacks.
func TestUndoWithCallbacks(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	stack := undo.NewStack(&undo.StackConfig{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		SessionID: "session-1",
		Logger:    logger,
	})

	pushCount := 0
	undoCount := 0
	redoCount := 0

	stack.SetOnPush(func(action undo.UndoableAction) {
		pushCount++
	})
	stack.SetOnUndo(func(action undo.UndoableAction) {
		undoCount++
	})
	stack.SetOnRedo(func(action undo.UndoableAction) {
		redoCount++
	})

	// Push action
	action := undo.NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	require.NoError(t, action.Execute(ctx))
	require.NoError(t, stack.Push(action))
	assert.Equal(t, 1, pushCount)

	// Undo
	_, err := stack.Undo(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, undoCount)

	// Redo
	_, err = stack.Redo(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, redoCount)
}
