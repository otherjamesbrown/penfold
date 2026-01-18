package automation

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

// testItem creates a test ReviewItem.
func testItem(id, tenantID string) *ReviewItem {
	return &ReviewItem{
		ID:          id,
		TenantID:    tenantID,
		ContentID:   "content-" + id,
		ContentType: "email",
		Category:    "communications",
		Source:      "gmail",
		Sender:      "test@example.com",
		Subject:     "Test Subject",
		Body:        "This is a test email body",
		Summary:     "Test summary",
		Priority:    2,
		Confidence:  0.85,
		Keywords:    []string{"test", "email"},
		Tags:        []string{"inbox"},
		CreatedAt:   time.Now().UTC(),
		Metadata:    make(map[string]interface{}),
	}
}

func TestNewEngine(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)

	if engine == nil {
		t.Fatal("expected engine to be created")
	}
	if engine.config == nil {
		t.Error("expected default config to be set")
	}
	if engine.matcher == nil {
		t.Error("expected matcher to be initialized")
	}
	if engine.actionRegistry == nil {
		t.Error("expected action registry to be initialized")
	}
}

func TestEngineAddRule(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()
	_ = ctx // Used in later tests

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)

	err := engine.AddRule(rule)
	if err != nil {
		t.Fatalf("unexpected error adding rule: %v", err)
	}

	ruleSet := engine.GetRuleSet("tenant-1")
	if ruleSet == nil {
		t.Fatal("expected rule set to be created")
	}

	if len(ruleSet.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(ruleSet.Rules))
	}
}

func TestEngineRemoveRule(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	_ = engine.AddRule(rule)

	removed := engine.RemoveRule("tenant-1", rule.ID)
	if !removed {
		t.Error("expected rule to be removed")
	}

	ruleSet := engine.GetRuleSet("tenant-1")
	if len(ruleSet.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(ruleSet.Rules))
	}
}

func TestEngineProcessRulesNoMatch(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	// Add rule that won't match
	rule := NewRule("tenant-1", "No Match Rule", SenderCondition("eq", "other@example.com"), ActionAccept)
	_ = engine.AddRule(rule)

	item := testItem("item-1", "tenant-1")
	item.Sender = "test@example.com" // Different sender

	actions, err := engine.ProcessRules(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}
}

func TestEngineProcessRulesMatch(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	// Add rule that will match
	rule := NewRule("tenant-1", "Match Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	_ = engine.AddRule(rule)

	item := testItem("item-1", "tenant-1")
	item.Sender = "test@example.com"

	actions, err := engine.ProcessRules(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}

	if actions[0].Type != ActionAccept {
		t.Errorf("expected action type %s, got %s", ActionAccept, actions[0].Type)
	}
}

func TestEngineProcessRulesPriority(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	// Add rules with different priorities
	rule1 := NewRule("tenant-1", "Low Priority", SenderCondition("eq", "test@example.com"), ActionDefer)
	rule1.Priority = 1
	rule1.StopOnMatch = true

	rule2 := NewRule("tenant-1", "High Priority", SenderCondition("eq", "test@example.com"), ActionAccept)
	rule2.Priority = 10
	rule2.StopOnMatch = true

	_ = engine.AddRule(rule1)
	_ = engine.AddRule(rule2)

	item := testItem("item-1", "tenant-1")

	actions, err := engine.ProcessRules(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only get the high priority action due to StopOnMatch
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}

	if actions[0].Type != ActionAccept {
		t.Errorf("expected high priority action (accept), got %s", actions[0].Type)
	}
}

func TestEngineExecuteAction(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	// Set up no-op handlers
	engine.SetActionHandlers(NoOpHandlers())

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	_ = engine.AddRule(rule)

	item := testItem("item-1", "tenant-1")
	action := NewAction(ActionAccept, rule.ID, item.ID)

	err := engine.ExecuteAction(ctx, action, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !action.Executed {
		t.Error("expected action to be marked as executed")
	}
	if action.ExecutedAt == nil {
		t.Error("expected executed_at to be set")
	}
}

func TestEngineExecuteActionDryRun(t *testing.T) {
	logger := testLogger()
	config := DefaultEngineConfig()
	config.DefaultDryRun = true
	engine := NewEngine(config, logger)
	ctx := context.Background()

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	_ = engine.AddRule(rule)

	item := testItem("item-1", "tenant-1")

	actions, err := engine.ProcessRules(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}

	if !actions[0].DryRun {
		t.Error("expected action to be in dry-run mode")
	}
}

func TestEngineProcessAndExecute(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	engine.SetActionHandlers(NoOpHandlers())

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionTag)
	rule.ActionParameters = map[string]interface{}{
		"tags": []string{"automated", "test"},
	}
	_ = engine.AddRule(rule)

	item := testItem("item-1", "tenant-1")

	actions, errors := engine.ProcessAndExecute(ctx, item)

	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}

	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
}

func TestEngineConflictResolutionFirst(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	ruleSet := NewRuleSet("tenant-1")
	ruleSet.ConflictResolution = "first"

	rule1 := NewRule("tenant-1", "Rule 1", SenderCondition("eq", "test@example.com"), ActionAccept)
	rule1.Priority = 10
	rule2 := NewRule("tenant-1", "Rule 2", SenderCondition("eq", "test@example.com"), ActionReject)
	rule2.Priority = 5

	ruleSet.AddRule(rule1)
	ruleSet.AddRule(rule2)
	engine.SetRuleSet(ruleSet)

	item := testItem("item-1", "tenant-1")

	actions, err := engine.ProcessRules(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only get first (highest priority) action
	if len(actions) != 1 {
		t.Errorf("expected 1 action after conflict resolution, got %d", len(actions))
	}

	if actions[0].Type != ActionAccept {
		t.Errorf("expected accept (higher priority), got %s", actions[0].Type)
	}
}

func TestEngineConflictResolutionMerge(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	ruleSet := NewRuleSet("tenant-1")
	ruleSet.ConflictResolution = "merge"

	// Accept rule
	rule1 := NewRule("tenant-1", "Rule 1", SenderCondition("eq", "test@example.com"), ActionAccept)
	rule1.Priority = 10
	// Tag rule - should coexist with accept
	rule2 := NewRule("tenant-1", "Rule 2", SenderCondition("eq", "test@example.com"), ActionTag)
	rule2.Priority = 5
	// Another accept - should be skipped (conflict)
	rule3 := NewRule("tenant-1", "Rule 3", SenderCondition("eq", "test@example.com"), ActionReject)
	rule3.Priority = 1

	ruleSet.AddRule(rule1)
	ruleSet.AddRule(rule2)
	ruleSet.AddRule(rule3)
	engine.SetRuleSet(ruleSet)

	item := testItem("item-1", "tenant-1")

	actions, err := engine.ProcessRules(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get accept and tag (reject conflicts with accept)
	if len(actions) != 2 {
		t.Errorf("expected 2 actions after merge, got %d", len(actions))
	}

	hasAccept := false
	hasTag := false
	for _, a := range actions {
		if a.Type == ActionAccept {
			hasAccept = true
		}
		if a.Type == ActionTag {
			hasTag = true
		}
	}

	if !hasAccept {
		t.Error("expected accept action")
	}
	if !hasTag {
		t.Error("expected tag action")
	}
}

func TestEngineEnableDisableRule(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	_ = engine.AddRule(rule)

	// Disable rule
	err := engine.DisableRule("tenant-1", rule.ID)
	if err != nil {
		t.Fatalf("unexpected error disabling rule: %v", err)
	}

	item := testItem("item-1", "tenant-1")
	actions, _ := engine.ProcessRules(ctx, item)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions with disabled rule, got %d", len(actions))
	}

	// Re-enable rule
	err = engine.EnableRule("tenant-1", rule.ID)
	if err != nil {
		t.Fatalf("unexpected error enabling rule: %v", err)
	}

	actions, _ = engine.ProcessRules(ctx, item)
	if len(actions) != 1 {
		t.Errorf("expected 1 action with enabled rule, got %d", len(actions))
	}
}

func TestEngineUpdateRulePriority(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)

	rule1 := NewRule("tenant-1", "Rule 1", SenderCondition("eq", "test@example.com"), ActionAccept)
	rule1.Priority = 5
	rule1.StopOnMatch = true

	rule2 := NewRule("tenant-1", "Rule 2", SenderCondition("eq", "test@example.com"), ActionReject)
	rule2.Priority = 10
	rule2.StopOnMatch = true

	_ = engine.AddRule(rule1)
	_ = engine.AddRule(rule2)

	// Update priority to make rule1 higher
	err := engine.UpdateRulePriority("tenant-1", rule1.ID, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	item := testItem("item-1", "tenant-1")

	actions, _ := engine.ProcessRules(ctx, item)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}

	if actions[0].Type != ActionAccept {
		t.Errorf("expected accept (now higher priority), got %s", actions[0].Type)
	}
}

func TestEngineEvaluateRule(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	engine.SetActionHandlers(NoOpHandlers())

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	_ = engine.AddRule(rule)

	item := testItem("item-1", "tenant-1")

	// Process a few times to generate metrics
	for i := 0; i < 5; i++ {
		engine.ProcessAndExecute(ctx, item)
	}

	metrics, err := engine.EvaluateRule(rule.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metrics.EvaluationCount != 5 {
		t.Errorf("expected 5 evaluations, got %d", metrics.EvaluationCount)
	}

	if metrics.MatchCount != 5 {
		t.Errorf("expected 5 matches, got %d", metrics.MatchCount)
	}

	if metrics.ActionExecutedCount != 5 {
		t.Errorf("expected 5 executed actions, got %d", metrics.ActionExecutedCount)
	}
}

func TestEngineLearnFromDecision(t *testing.T) {
	logger := testLogger()
	config := DefaultEngineConfig()
	config.EnableLearning = true
	engine := NewEngine(config, logger)

	item := testItem("item-1", "tenant-1")
	decision := NewDecision("tenant-1", "user-1", item, ActionAccept)

	// Should not panic
	engine.LearnFromDecision(decision)
}

func TestEngineRecordFeedback(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	_ = engine.AddRule(rule)

	feedback := NewFeedback("tenant-1", "user-1", rule.ID, "action-1", "item-1", true)
	engine.RecordFeedback(feedback)

	// Should update metrics
	metrics, _ := engine.EvaluateRule(rule.ID)
	if metrics.FeedbackPositive != 1 {
		t.Errorf("expected 1 positive feedback, got %d", metrics.FeedbackPositive)
	}
}

func TestEngineAuditLog(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	engine.SetActionHandlers(NoOpHandlers())

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	_ = engine.AddRule(rule)

	item := testItem("item-1", "tenant-1")
	engine.ProcessAndExecute(ctx, item)

	auditLog, err := engine.GetAuditLog(ctx, "tenant-1", "item-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(auditLog) != 1 {
		t.Errorf("expected 1 audit entry, got %d", len(auditLog))
	}

	if auditLog[0].Result != "success" {
		t.Errorf("expected result 'success', got '%s'", auditLog[0].Result)
	}
}

func TestEngineMaxActionsPerItem(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	ruleSet := NewRuleSet("tenant-1")
	ruleSet.MaxActionsPerItem = 2

	// Add 5 rules that all match
	for i := 0; i < 5; i++ {
		rule := NewRule("tenant-1", "Rule", SenderCondition("eq", "test@example.com"), ActionTag)
		rule.Priority = 10 - i
		ruleSet.AddRule(rule)
	}

	engine.SetRuleSet(ruleSet)

	item := testItem("item-1", "tenant-1")

	actions, err := engine.ProcessRules(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(actions) > 2 {
		t.Errorf("expected at most 2 actions, got %d", len(actions))
	}
}

func TestEngineDisabledRuleSet(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	ruleSet := NewRuleSet("tenant-1")
	ruleSet.Enabled = false

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	ruleSet.AddRule(rule)

	engine.SetRuleSet(ruleSet)

	item := testItem("item-1", "tenant-1")

	actions, err := engine.ProcessRules(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(actions) != 0 {
		t.Errorf("expected 0 actions with disabled rule set, got %d", len(actions))
	}
}

func TestEngineNoRuleSet(t *testing.T) {
	logger := testLogger()
	engine := NewEngine(nil, logger)
	ctx := context.Background()

	item := testItem("item-1", "unknown-tenant")

	actions, err := engine.ProcessRules(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(actions) != 0 {
		t.Errorf("expected 0 actions for unknown tenant, got %d", len(actions))
	}
}
