package automation

import (
	"testing"
	"time"
)

func TestNewRule(t *testing.T) {
	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)

	if rule.ID == "" {
		t.Error("expected rule ID to be set")
	}
	if rule.TenantID != "tenant-1" {
		t.Errorf("expected tenant ID 'tenant-1', got '%s'", rule.TenantID)
	}
	if rule.Name != "Test Rule" {
		t.Errorf("expected name 'Test Rule', got '%s'", rule.Name)
	}
	if rule.ActionType != ActionAccept {
		t.Errorf("expected action type %s, got %s", ActionAccept, rule.ActionType)
	}
	if !rule.Enabled {
		t.Error("expected rule to be enabled by default")
	}
	if rule.Source != "manual" {
		t.Errorf("expected source 'manual', got '%s'", rule.Source)
	}
}

func TestRuleValidate(t *testing.T) {
	tests := []struct {
		name    string
		rule    *Rule
		wantErr bool
	}{
		{
			name:    "valid rule",
			rule:    NewRule("tenant-1", "Test", SenderCondition("eq", "test@example.com"), ActionAccept),
			wantErr: false,
		},
		{
			name: "missing ID",
			rule: &Rule{
				TenantID:   "tenant-1",
				Name:       "Test",
				ActionType: ActionAccept,
			},
			wantErr: true,
		},
		{
			name: "missing tenant ID",
			rule: &Rule{
				ID:         "rule-1",
				Name:       "Test",
				ActionType: ActionAccept,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			rule: &Rule{
				ID:         "rule-1",
				TenantID:   "tenant-1",
				ActionType: ActionAccept,
			},
			wantErr: true,
		},
		{
			name: "invalid action type",
			rule: &Rule{
				ID:         "rule-1",
				TenantID:   "tenant-1",
				Name:       "Test",
				ActionType: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRuleClone(t *testing.T) {
	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	rule.ActionParameters["key"] = "value"
	rule.Metadata["meta"] = "data"
	rule.Tags = []string{"tag1", "tag2"}

	clone := rule.Clone()

	// Verify clone is independent
	if clone.ID != rule.ID {
		t.Errorf("expected same ID, got different")
	}

	// Modify original
	rule.ActionParameters["key"] = "modified"
	rule.Metadata["meta"] = "modified"
	rule.Tags[0] = "modified"

	// Clone should be unchanged
	if clone.ActionParameters["key"] != "value" {
		t.Error("clone action parameters were modified")
	}
	if clone.Metadata["meta"] != "data" {
		t.Error("clone metadata was modified")
	}
	if clone.Tags[0] != "tag1" {
		t.Error("clone tags were modified")
	}
}

func TestRuleJSON(t *testing.T) {
	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	rule.Description = "Test description"
	rule.Priority = 10

	// Serialize
	data, err := rule.ToJSON()
	if err != nil {
		t.Fatalf("unexpected error serializing: %v", err)
	}

	// Deserialize
	restored, err := RuleFromJSON(data)
	if err != nil {
		t.Fatalf("unexpected error deserializing: %v", err)
	}

	if restored.ID != rule.ID {
		t.Errorf("ID mismatch: got %s, want %s", restored.ID, rule.ID)
	}
	if restored.Name != rule.Name {
		t.Errorf("Name mismatch: got %s, want %s", restored.Name, rule.Name)
	}
	if restored.Priority != rule.Priority {
		t.Errorf("Priority mismatch: got %d, want %d", restored.Priority, rule.Priority)
	}
}

func TestActionTypeValid(t *testing.T) {
	validTypes := []ActionType{
		ActionAccept,
		ActionReject,
		ActionDefer,
		ActionEscalate,
		ActionTag,
		ActionNotify,
		ActionRoute,
	}

	for _, at := range validTypes {
		if !at.IsValid() {
			t.Errorf("expected %s to be valid", at)
		}
	}

	invalidType := ActionType("invalid")
	if invalidType.IsValid() {
		t.Error("expected 'invalid' to not be valid")
	}
}

func TestNewAction(t *testing.T) {
	action := NewAction(ActionAccept, "rule-1", "item-1")

	if action.ID == "" {
		t.Error("expected action ID to be set")
	}
	if action.Type != ActionAccept {
		t.Errorf("expected type %s, got %s", ActionAccept, action.Type)
	}
	if action.RuleID != "rule-1" {
		t.Errorf("expected rule ID 'rule-1', got '%s'", action.RuleID)
	}
	if action.ItemID != "item-1" {
		t.Errorf("expected item ID 'item-1', got '%s'", action.ItemID)
	}
	if action.Executed {
		t.Error("expected action to not be executed")
	}
}

func TestRuleMetrics(t *testing.T) {
	metrics := NewRuleMetrics("rule-1", "tenant-1")

	// Record evaluations
	metrics.RecordEvaluation(true, 10)
	metrics.RecordEvaluation(true, 15)
	metrics.RecordEvaluation(false, 5)

	if metrics.EvaluationCount != 3 {
		t.Errorf("expected 3 evaluations, got %d", metrics.EvaluationCount)
	}
	if metrics.MatchCount != 2 {
		t.Errorf("expected 2 matches, got %d", metrics.MatchCount)
	}
	if metrics.TotalEvaluationTimeMs != 30 {
		t.Errorf("expected 30ms total time, got %d", metrics.TotalEvaluationTimeMs)
	}
	if metrics.AverageEvaluationTimeMs != 10 {
		t.Errorf("expected 10ms average time, got %f", metrics.AverageEvaluationTimeMs)
	}

	// Record action executions
	metrics.RecordActionExecution(true)
	metrics.RecordActionExecution(true)
	metrics.RecordActionExecution(false)

	if metrics.ActionExecutedCount != 3 {
		t.Errorf("expected 3 action executions, got %d", metrics.ActionExecutedCount)
	}
	if metrics.ActionErrorCount != 1 {
		t.Errorf("expected 1 action error, got %d", metrics.ActionErrorCount)
	}

	// Check success rate (2 success / 3 total = 66.67%)
	expectedSuccessRate := (2.0 / 3.0) * 100
	if metrics.SuccessRate < expectedSuccessRate-1 || metrics.SuccessRate > expectedSuccessRate+1 {
		t.Errorf("expected success rate ~%.2f%%, got %.2f%%", expectedSuccessRate, metrics.SuccessRate)
	}

	// Record feedback
	metrics.RecordFeedback(true)
	metrics.RecordFeedback(true)
	metrics.RecordFeedback(false)

	if metrics.FeedbackPositive != 2 {
		t.Errorf("expected 2 positive feedback, got %d", metrics.FeedbackPositive)
	}
	if metrics.FeedbackNegative != 1 {
		t.Errorf("expected 1 negative feedback, got %d", metrics.FeedbackNegative)
	}

	// Record override
	metrics.RecordOverride()
	if metrics.OverrideCount != 1 {
		t.Errorf("expected 1 override, got %d", metrics.OverrideCount)
	}
}

func TestRuleSet(t *testing.T) {
	rs := NewRuleSet("tenant-1")

	if rs.TenantID != "tenant-1" {
		t.Errorf("expected tenant ID 'tenant-1', got '%s'", rs.TenantID)
	}
	if !rs.Enabled {
		t.Error("expected rule set to be enabled by default")
	}

	// Add rules with different priorities
	rule1 := NewRule("tenant-1", "Rule 1", SenderCondition("eq", "test@example.com"), ActionAccept)
	rule1.Priority = 5

	rule2 := NewRule("tenant-1", "Rule 2", SenderCondition("eq", "test@example.com"), ActionReject)
	rule2.Priority = 10

	rule3 := NewRule("tenant-1", "Rule 3", SenderCondition("eq", "test@example.com"), ActionDefer)
	rule3.Priority = 1

	rs.AddRule(rule1)
	rs.AddRule(rule2)
	rs.AddRule(rule3)

	// Rules should be sorted by priority (highest first)
	if rs.Rules[0].ID != rule2.ID {
		t.Error("expected rule2 (priority 10) to be first")
	}
	if rs.Rules[1].ID != rule1.ID {
		t.Error("expected rule1 (priority 5) to be second")
	}
	if rs.Rules[2].ID != rule3.ID {
		t.Error("expected rule3 (priority 1) to be third")
	}
}

func TestRuleSetGetRule(t *testing.T) {
	rs := NewRuleSet("tenant-1")

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	rs.AddRule(rule)

	found := rs.GetRule(rule.ID)
	if found == nil {
		t.Fatal("expected to find rule")
	}
	if found.ID != rule.ID {
		t.Errorf("expected rule ID %s, got %s", rule.ID, found.ID)
	}

	notFound := rs.GetRule("nonexistent")
	if notFound != nil {
		t.Error("expected nil for nonexistent rule")
	}
}

func TestRuleSetRemoveRule(t *testing.T) {
	rs := NewRuleSet("tenant-1")

	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	rs.AddRule(rule)

	removed := rs.RemoveRule(rule.ID)
	if !removed {
		t.Error("expected rule to be removed")
	}
	if len(rs.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rs.Rules))
	}

	// Try to remove again
	removed = rs.RemoveRule(rule.ID)
	if removed {
		t.Error("expected false for already removed rule")
	}
}

func TestRuleSetGetEnabledRules(t *testing.T) {
	rs := NewRuleSet("tenant-1")

	rule1 := NewRule("tenant-1", "Rule 1", SenderCondition("eq", "test@example.com"), ActionAccept)
	rule1.Enabled = true

	rule2 := NewRule("tenant-1", "Rule 2", SenderCondition("eq", "test@example.com"), ActionReject)
	rule2.Enabled = false

	rule3 := NewRule("tenant-1", "Rule 3", SenderCondition("eq", "test@example.com"), ActionDefer)
	rule3.Enabled = true

	rs.AddRule(rule1)
	rs.AddRule(rule2)
	rs.AddRule(rule3)

	enabled := rs.GetEnabledRules()
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled rules, got %d", len(enabled))
	}

	for _, r := range enabled {
		if !r.Enabled {
			t.Error("expected all returned rules to be enabled")
		}
	}
}

func TestConditionBuilders(t *testing.T) {
	// Test all condition builder functions

	// PatternCondition
	pc := PatternCondition("test.*", "subject", true)
	if pc.Type != ConditionPattern {
		t.Errorf("expected type %s, got %s", ConditionPattern, pc.Type)
	}
	if pc.Pattern != "test.*" {
		t.Errorf("expected pattern 'test.*', got '%s'", pc.Pattern)
	}

	// SenderCondition
	sc := SenderCondition("eq", "test@example.com")
	if sc.Type != ConditionSender {
		t.Errorf("expected type %s, got %s", ConditionSender, sc.Type)
	}

	// CategoryCondition
	cc := CategoryCondition("contains", "comm")
	if cc.Type != ConditionCategory {
		t.Errorf("expected type %s, got %s", ConditionCategory, cc.Type)
	}

	// KeywordsCondition
	kc := KeywordsCondition([]string{"urgent", "important"}, true)
	if kc.Type != ConditionKeywords {
		t.Errorf("expected type %s, got %s", ConditionKeywords, kc.Type)
	}
	if kc.Operator != "all" {
		t.Errorf("expected operator 'all', got '%s'", kc.Operator)
	}

	// ConfidenceCondition
	conf := ConfidenceCondition(0.8, 0.95)
	if conf.Type != ConditionConfidence {
		t.Errorf("expected type %s, got %s", ConditionConfidence, conf.Type)
	}

	// ThresholdCondition
	tc := ThresholdCondition("priority", "gte", 3)
	if tc.Type != ConditionThreshold {
		t.Errorf("expected type %s, got %s", ConditionThreshold, tc.Type)
	}

	// ContentTypeCondition
	ctc := ContentTypeCondition("email")
	if ctc.Type != ConditionContentType {
		t.Errorf("expected type %s, got %s", ConditionContentType, ctc.Type)
	}

	// SourceCondition
	src := SourceCondition("gmail")
	if src.Type != ConditionSource {
		t.Errorf("expected type %s, got %s", ConditionSource, src.Type)
	}

	// PriorityCondition
	pri := PriorityCondition("gte", 3)
	if pri.Type != ConditionPriority {
		t.Errorf("expected type %s, got %s", ConditionPriority, pri.Type)
	}

	// AgeCondition
	ac := AgeCondition("gt", 24)
	if ac.Type != ConditionAge {
		t.Errorf("expected type %s, got %s", ConditionAge, ac.Type)
	}

	// AndCondition
	and := AndCondition(sc, cc)
	if and.Type != ConditionAnd {
		t.Errorf("expected type %s, got %s", ConditionAnd, and.Type)
	}
	if len(and.SubConditions) != 2 {
		t.Errorf("expected 2 sub-conditions, got %d", len(and.SubConditions))
	}

	// OrCondition
	or := OrCondition(sc, cc)
	if or.Type != ConditionOr {
		t.Errorf("expected type %s, got %s", ConditionOr, or.Type)
	}

	// NotCondition
	not := NotCondition(sc)
	if not.Type != ConditionNot {
		t.Errorf("expected type %s, got %s", ConditionNot, not.Type)
	}
	if len(not.SubConditions) != 1 {
		t.Errorf("expected 1 sub-condition, got %d", len(not.SubConditions))
	}
}

func TestRuleMetricsTimestamps(t *testing.T) {
	metrics := NewRuleMetrics("rule-1", "tenant-1")

	// Initially nil
	if metrics.LastEvaluatedAt != nil {
		t.Error("expected LastEvaluatedAt to be nil initially")
	}
	if metrics.LastMatchedAt != nil {
		t.Error("expected LastMatchedAt to be nil initially")
	}

	// Record evaluation without match
	metrics.RecordEvaluation(false, 10)
	if metrics.LastEvaluatedAt == nil {
		t.Error("expected LastEvaluatedAt to be set")
	}
	if metrics.LastMatchedAt != nil {
		t.Error("expected LastMatchedAt to still be nil")
	}

	// Record evaluation with match
	time.Sleep(1 * time.Millisecond) // Ensure different timestamp
	metrics.RecordEvaluation(true, 10)
	if metrics.LastMatchedAt == nil {
		t.Error("expected LastMatchedAt to be set")
	}
	if !metrics.LastEvaluatedAt.After(metrics.PeriodStart) {
		t.Error("expected LastEvaluatedAt to be after PeriodStart")
	}
}
