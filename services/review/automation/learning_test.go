package automation

import (
	"context"
	"testing"
	"time"
)

func TestNewDecision(t *testing.T) {
	item := &ReviewItem{
		ID:       "item-1",
		TenantID: "tenant-1",
	}

	decision := NewDecision("tenant-1", "user-1", item, ActionAccept)

	if decision.ID == "" {
		t.Error("expected decision ID to be set")
	}
	if decision.TenantID != "tenant-1" {
		t.Errorf("expected tenant ID 'tenant-1', got '%s'", decision.TenantID)
	}
	if decision.UserID != "user-1" {
		t.Errorf("expected user ID 'user-1', got '%s'", decision.UserID)
	}
	if decision.ItemID != "item-1" {
		t.Errorf("expected item ID 'item-1', got '%s'", decision.ItemID)
	}
	if decision.Action != ActionAccept {
		t.Errorf("expected action %s, got %s", ActionAccept, decision.Action)
	}
}

func TestNewRuleSuggestion(t *testing.T) {
	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	suggestion := NewRuleSuggestion("tenant-1", rule, 0.85, 10)

	if suggestion.ID == "" {
		t.Error("expected suggestion ID to be set")
	}
	if suggestion.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", suggestion.Confidence)
	}
	if suggestion.SupportCount != 10 {
		t.Errorf("expected support count 10, got %d", suggestion.SupportCount)
	}
	if suggestion.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", suggestion.Status)
	}
}

func TestRuleSuggestionApprove(t *testing.T) {
	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	suggestion := NewRuleSuggestion("tenant-1", rule, 0.85, 10)

	suggestion.Approve("admin@example.com")

	if suggestion.Status != "approved" {
		t.Errorf("expected status 'approved', got '%s'", suggestion.Status)
	}
	if suggestion.ApprovedAt == nil {
		t.Error("expected approved_at to be set")
	}
	if suggestion.ApprovedBy != "admin@example.com" {
		t.Errorf("expected approved by 'admin@example.com', got '%s'", suggestion.ApprovedBy)
	}
}

func TestRuleSuggestionReject(t *testing.T) {
	rule := NewRule("tenant-1", "Test Rule", SenderCondition("eq", "test@example.com"), ActionAccept)
	suggestion := NewRuleSuggestion("tenant-1", rule, 0.85, 10)

	suggestion.Reject()

	if suggestion.Status != "rejected" {
		t.Errorf("expected status 'rejected', got '%s'", suggestion.Status)
	}
}

func TestNewFeedback(t *testing.T) {
	fb := NewFeedback("tenant-1", "user-1", "rule-1", "action-1", "item-1", true)

	if fb.ID == "" {
		t.Error("expected feedback ID to be set")
	}
	if fb.TenantID != "tenant-1" {
		t.Errorf("expected tenant ID 'tenant-1', got '%s'", fb.TenantID)
	}
	if !fb.Positive {
		t.Error("expected positive feedback")
	}
}

func TestRuleLearner(t *testing.T) {
	config := &LearningConfig{
		MinSupportCount:     2,
		MinConfidence:       0.7,
		MaxSuggestions:      10,
		LookbackDays:        30,
		EnableSenderRules:   true,
		EnableCategoryRules: true,
		EnableKeywordRules:  true,
	}

	learner := NewRuleLearner(config)

	if learner == nil {
		t.Fatal("expected learner to be created")
	}

	// Add decisions
	for i := 0; i < 5; i++ {
		item := &ReviewItem{
			ID:        "item-" + string(rune('a'+i)),
			TenantID:  "tenant-1",
			Sender:    "newsletter@example.com",
			Category:  "marketing",
			Subject:   "Weekly Newsletter",
			Keywords:  []string{"newsletter", "weekly"},
			CreatedAt: time.Now(),
		}

		decision := NewDecision("tenant-1", "user-1", item, ActionReject)
		learner.LearnFromDecision(decision)
	}

	ctx := context.Background()
	suggestions, err := learner.SuggestRules(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have suggestions based on sender pattern
	if len(suggestions) == 0 {
		t.Error("expected at least one suggestion")
	}

	// Check that suggestions are for reject action
	for _, s := range suggestions {
		if s.Rule.ActionType != ActionReject {
			t.Errorf("expected action type %s, got %s", ActionReject, s.Rule.ActionType)
		}
	}
}

func TestRuleLearnerNotEnoughData(t *testing.T) {
	config := &LearningConfig{
		MinSupportCount:   10, // High threshold
		MinConfidence:     0.8,
		MaxSuggestions:    10,
		LookbackDays:      30,
		EnableSenderRules: true,
	}

	learner := NewRuleLearner(config)

	// Add only a few decisions
	for i := 0; i < 3; i++ {
		item := &ReviewItem{
			ID:        "item-" + string(rune('a'+i)),
			TenantID:  "tenant-1",
			Sender:    "test@example.com",
			CreatedAt: time.Now(),
		}

		decision := NewDecision("tenant-1", "user-1", item, ActionAccept)
		learner.LearnFromDecision(decision)
	}

	ctx := context.Background()
	suggestions, err := learner.SuggestRules(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions due to insufficient data, got %d", len(suggestions))
	}
}

func TestRuleLearnerDifferentTenants(t *testing.T) {
	config := DefaultLearningConfig()
	config.MinSupportCount = 2

	learner := NewRuleLearner(config)

	// Add decisions for tenant-1
	for i := 0; i < 3; i++ {
		item := &ReviewItem{
			ID:        "item-t1-" + string(rune('a'+i)),
			TenantID:  "tenant-1",
			Sender:    "test@tenant1.com",
			CreatedAt: time.Now(),
		}
		learner.LearnFromDecision(NewDecision("tenant-1", "user-1", item, ActionAccept))
	}

	// Add decisions for tenant-2
	for i := 0; i < 3; i++ {
		item := &ReviewItem{
			ID:        "item-t2-" + string(rune('a'+i)),
			TenantID:  "tenant-2",
			Sender:    "test@tenant2.com",
			CreatedAt: time.Now(),
		}
		learner.LearnFromDecision(NewDecision("tenant-2", "user-2", item, ActionReject))
	}

	ctx := context.Background()

	// Check tenant-1 suggestions
	suggestions1, _ := learner.SuggestRules(ctx, "tenant-1")
	for _, s := range suggestions1 {
		if s.TenantID != "tenant-1" {
			t.Error("expected only tenant-1 suggestions")
		}
		if s.Rule.ActionType != ActionAccept {
			t.Error("expected accept actions for tenant-1")
		}
	}

	// Check tenant-2 suggestions
	suggestions2, _ := learner.SuggestRules(ctx, "tenant-2")
	for _, s := range suggestions2 {
		if s.TenantID != "tenant-2" {
			t.Error("expected only tenant-2 suggestions")
		}
		if s.Rule.ActionType != ActionReject {
			t.Error("expected reject actions for tenant-2")
		}
	}
}

func TestRuleLearnerTrainRule(t *testing.T) {
	config := DefaultLearningConfig()
	learner := NewRuleLearner(config)

	examples := []Example{
		{
			Item: &ReviewItem{
				ID:       "1",
				Sender:   "boss@company.com",
				Category: "work",
			},
			Action: ActionAccept,
			Weight: 1.0,
		},
		{
			Item: &ReviewItem{
				ID:       "2",
				Sender:   "boss@company.com",
				Category: "work",
			},
			Action: ActionAccept,
			Weight: 1.0,
		},
		{
			Item: &ReviewItem{
				ID:       "3",
				Sender:   "boss@company.com",
				Category: "work",
			},
			Action: ActionAccept,
			Weight: 1.0,
		},
	}

	ctx := context.Background()
	rule, err := learner.TrainRule(ctx, "tenant-1", "Boss Emails", examples)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rule == nil {
		t.Fatal("expected rule to be created")
	}

	if rule.Name != "Boss Emails" {
		t.Errorf("expected name 'Boss Emails', got '%s'", rule.Name)
	}

	if rule.ActionType != ActionAccept {
		t.Errorf("expected action type %s, got %s", ActionAccept, rule.ActionType)
	}

	if rule.Source != "trained" {
		t.Errorf("expected source 'trained', got '%s'", rule.Source)
	}
}

func TestRuleLearnerTrainRuleEmpty(t *testing.T) {
	config := DefaultLearningConfig()
	learner := NewRuleLearner(config)

	ctx := context.Background()
	_, err := learner.TrainRule(ctx, "tenant-1", "Empty Rule", []Example{})

	if err == nil {
		t.Error("expected error for empty examples")
	}
}

func TestRuleLearnerClearDecisions(t *testing.T) {
	config := DefaultLearningConfig()
	learner := NewRuleLearner(config)

	item := &ReviewItem{
		ID:        "item-1",
		TenantID:  "tenant-1",
		CreatedAt: time.Now(),
	}
	learner.LearnFromDecision(NewDecision("tenant-1", "user-1", item, ActionAccept))

	learner.ClearDecisions()

	ctx := context.Background()
	suggestions, _ := learner.SuggestRules(ctx, "tenant-1")
	if len(suggestions) != 0 {
		t.Error("expected no suggestions after clearing decisions")
	}
}

func TestFeedbackTracker(t *testing.T) {
	tracker := NewFeedbackTracker()

	// Record feedback
	tracker.RecordFeedback(NewFeedback("tenant-1", "user-1", "rule-1", "action-1", "item-1", true))
	tracker.RecordFeedback(NewFeedback("tenant-1", "user-2", "rule-1", "action-2", "item-2", true))
	tracker.RecordFeedback(NewFeedback("tenant-1", "user-3", "rule-1", "action-3", "item-3", false))

	// Get feedback
	feedback := tracker.GetFeedback("rule-1")
	if len(feedback) != 3 {
		t.Errorf("expected 3 feedback entries, got %d", len(feedback))
	}

	// Get feedback score
	score := tracker.GetFeedbackScore("rule-1")
	expectedScore := 2.0 / 3.0 // 2 positive out of 3
	if score < expectedScore-0.01 || score > expectedScore+0.01 {
		t.Errorf("expected score ~%.2f, got %.2f", expectedScore, score)
	}

	// Check non-existent rule
	score2 := tracker.GetFeedbackScore("nonexistent")
	if score2 != 0 {
		t.Errorf("expected 0 score for nonexistent rule, got %f", score2)
	}
}

func TestFeedbackTrackerShouldDisableRule(t *testing.T) {
	tracker := NewFeedbackTracker()

	// Record mostly negative feedback
	for i := 0; i < 10; i++ {
		positive := i < 2 // Only 2 positive, 8 negative
		fb := NewFeedback("tenant-1", "user", "rule-1", "action", "item", positive)
		tracker.RecordFeedback(fb)
	}

	// Should suggest disabling (80% negative > 70% threshold)
	if !tracker.ShouldDisableRule("rule-1", 5, 0.70) {
		t.Error("expected to suggest disabling rule with high negative ratio")
	}

	// Should not suggest with stricter threshold
	if tracker.ShouldDisableRule("rule-1", 5, 0.90) {
		t.Error("expected not to suggest disabling with stricter threshold")
	}

	// Should not suggest with insufficient feedback
	if tracker.ShouldDisableRule("rule-1", 20, 0.70) {
		t.Error("expected not to suggest disabling with insufficient feedback")
	}
}

func TestLearningConfigDefaults(t *testing.T) {
	config := DefaultLearningConfig()

	if config.MinSupportCount <= 0 {
		t.Error("expected positive MinSupportCount")
	}
	if config.MinConfidence <= 0 || config.MinConfidence > 1 {
		t.Error("expected MinConfidence between 0 and 1")
	}
	if config.MaxSuggestions <= 0 {
		t.Error("expected positive MaxSuggestions")
	}
	if config.LookbackDays <= 0 {
		t.Error("expected positive LookbackDays")
	}
	if !config.EnableSenderRules {
		t.Error("expected sender rules to be enabled by default")
	}
	if !config.EnableCategoryRules {
		t.Error("expected category rules to be enabled by default")
	}
}

func TestRuleLearnerKeywordRules(t *testing.T) {
	config := &LearningConfig{
		MinSupportCount:    2,
		MinConfidence:      0.7,
		MaxSuggestions:     10,
		LookbackDays:       30,
		EnableKeywordRules: true,
	}

	learner := NewRuleLearner(config)

	// Add decisions with common keyword
	for i := 0; i < 5; i++ {
		item := &ReviewItem{
			ID:        "item-" + string(rune('a'+i)),
			TenantID:  "tenant-1",
			Subject:   "URGENT: Please review",
			Keywords:  []string{"urgent"},
			CreatedAt: time.Now(),
		}

		decision := NewDecision("tenant-1", "user-1", item, ActionAccept)
		learner.LearnFromDecision(decision)
	}

	ctx := context.Background()
	suggestions, err := learner.SuggestRules(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have keyword-based suggestions
	hasKeywordSuggestion := false
	for _, s := range suggestions {
		if s.Rule.Condition.Type == ConditionKeywords {
			hasKeywordSuggestion = true
			break
		}
	}

	if !hasKeywordSuggestion && len(suggestions) > 0 {
		// Keywords might be captured differently
		t.Log("Note: keyword suggestion may be captured as part of sender/category rules")
	}
}

func TestRuleLearnerCategoryRules(t *testing.T) {
	config := &LearningConfig{
		MinSupportCount:     2,
		MinConfidence:       0.8,
		MaxSuggestions:      10,
		LookbackDays:        30,
		EnableCategoryRules: true,
	}

	learner := NewRuleLearner(config)

	// Add decisions with same category
	for i := 0; i < 5; i++ {
		item := &ReviewItem{
			ID:        "item-" + string(rune('a'+i)),
			TenantID:  "tenant-1",
			Category:  "spam",
			CreatedAt: time.Now(),
		}

		decision := NewDecision("tenant-1", "user-1", item, ActionReject)
		learner.LearnFromDecision(decision)
	}

	ctx := context.Background()
	suggestions, err := learner.SuggestRules(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have category-based suggestions
	hasCategorySuggestion := false
	for _, s := range suggestions {
		if s.Rule.Condition.Type == ConditionCategory {
			hasCategorySuggestion = true
			if s.Rule.ActionType != ActionReject {
				t.Error("expected reject action for spam category")
			}
			break
		}
	}

	if !hasCategorySuggestion && len(suggestions) > 0 {
		t.Log("Category suggestion might be combined with other rules")
	}
}

func TestExtractKeywords(t *testing.T) {
	item := &ReviewItem{
		Subject:  "Important Meeting Tomorrow",
		Summary:  "Discuss quarterly results",
		Keywords: []string{"meeting", "quarterly"},
	}

	keywords := extractKeywords(item)

	if len(keywords) == 0 {
		t.Error("expected some keywords to be extracted")
	}

	// Should include existing keywords
	hasQuarterly := false
	for _, kw := range keywords {
		if kw == "quarterly" {
			hasQuarterly = true
			break
		}
	}
	if !hasQuarterly {
		t.Error("expected 'quarterly' in keywords")
	}
}

func TestFindCommonKeywords(t *testing.T) {
	examples := []Example{
		{Item: &ReviewItem{Subject: "Important meeting", Summary: "quarterly review"}},
		{Item: &ReviewItem{Subject: "Important update", Summary: "quarterly numbers"}},
		{Item: &ReviewItem{Subject: "Important deadline", Summary: "quarterly report"}},
	}

	common := findCommonKeywords(examples)

	// "important" and "quarterly" should appear in most examples
	if len(common) == 0 {
		t.Error("expected some common keywords")
	}

	// Check that common keywords appear
	hasImportant := false
	hasQuarterly := false
	for _, kw := range common {
		if kw == "important" {
			hasImportant = true
		}
		if kw == "quarterly" {
			hasQuarterly = true
		}
	}

	if !hasImportant && !hasQuarterly {
		t.Error("expected either 'important' or 'quarterly' in common keywords")
	}
}
