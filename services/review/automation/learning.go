// Package automation provides rule learning capabilities for the automation engine.
package automation

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Decision represents a user's decision on a review item.
type Decision struct {
	// ID is the unique identifier for this decision.
	ID string `json:"id"`

	// TenantID is the tenant this decision belongs to.
	TenantID string `json:"tenant_id"`

	// UserID is the user who made the decision.
	UserID string `json:"user_id"`

	// ItemID is the review item this decision was made on.
	ItemID string `json:"item_id"`

	// Item is a snapshot of the review item at decision time.
	Item *ReviewItem `json:"item"`

	// Action is the action the user took.
	Action ActionType `json:"action"`

	// Feedback is optional user feedback.
	Feedback string `json:"feedback,omitempty"`

	// TimeSpentMs is how long the user spent on this decision.
	TimeSpentMs int64 `json:"time_spent_ms"`

	// AutomatedRuleID is set if an automation rule suggested this action.
	AutomatedRuleID string `json:"automated_rule_id,omitempty"`

	// OverrodeAutomation indicates if the user overrode an automated action.
	OverrodeAutomation bool `json:"overrode_automation"`

	// CreatedAt is when this decision was made.
	CreatedAt time.Time `json:"created_at"`

	// Metadata holds additional decision context.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewDecision creates a new Decision.
func NewDecision(tenantID, userID string, item *ReviewItem, action ActionType) *Decision {
	return &Decision{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		ItemID:    item.ID,
		Item:      item,
		Action:    action,
		CreatedAt: time.Now().UTC(),
		Metadata:  make(map[string]interface{}),
	}
}

// Example represents a training example for rule learning.
type Example struct {
	// Item is the review item.
	Item *ReviewItem `json:"item"`

	// Action is the expected action.
	Action ActionType `json:"action"`

	// Weight is the importance of this example (default 1.0).
	Weight float64 `json:"weight"`

	// Source indicates where this example came from.
	Source string `json:"source"`
}

// RuleSuggestion represents a suggested automation rule learned from decisions.
type RuleSuggestion struct {
	// ID is a unique identifier for this suggestion.
	ID string `json:"id"`

	// TenantID is the tenant this suggestion is for.
	TenantID string `json:"tenant_id"`

	// Rule is the suggested rule.
	Rule *Rule `json:"rule"`

	// Confidence is the confidence score for this suggestion (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// SupportCount is the number of decisions supporting this rule.
	SupportCount int `json:"support_count"`

	// ExampleDecisions contains example decisions that led to this suggestion.
	ExampleDecisions []*Decision `json:"example_decisions,omitempty"`

	// Explanation describes why this rule was suggested.
	Explanation string `json:"explanation"`

	// Status is the current status (pending, approved, rejected, deployed).
	Status string `json:"status"`

	// CreatedAt is when this suggestion was created.
	CreatedAt time.Time `json:"created_at"`

	// ApprovedAt is when this suggestion was approved.
	ApprovedAt *time.Time `json:"approved_at,omitempty"`

	// ApprovedBy is who approved this suggestion.
	ApprovedBy string `json:"approved_by,omitempty"`
}

// NewRuleSuggestion creates a new RuleSuggestion.
func NewRuleSuggestion(tenantID string, rule *Rule, confidence float64, supportCount int) *RuleSuggestion {
	return &RuleSuggestion{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		Rule:             rule,
		Confidence:       confidence,
		SupportCount:     supportCount,
		ExampleDecisions: make([]*Decision, 0),
		Status:           "pending",
		CreatedAt:        time.Now().UTC(),
	}
}

// Approve marks the suggestion as approved.
func (s *RuleSuggestion) Approve(approvedBy string) {
	s.Status = "approved"
	now := time.Now().UTC()
	s.ApprovedAt = &now
	s.ApprovedBy = approvedBy
}

// Reject marks the suggestion as rejected.
func (s *RuleSuggestion) Reject() {
	s.Status = "rejected"
}

// Feedback represents user feedback on an automated action.
type Feedback struct {
	// ID is the unique identifier for this feedback.
	ID string `json:"id"`

	// TenantID is the tenant this feedback belongs to.
	TenantID string `json:"tenant_id"`

	// UserID is the user who provided the feedback.
	UserID string `json:"user_id"`

	// RuleID is the rule that triggered the automated action.
	RuleID string `json:"rule_id"`

	// ActionID is the action this feedback is for.
	ActionID string `json:"action_id"`

	// ItemID is the review item.
	ItemID string `json:"item_id"`

	// Positive indicates if the feedback is positive or negative.
	Positive bool `json:"positive"`

	// Comment is an optional comment from the user.
	Comment string `json:"comment,omitempty"`

	// CreatedAt is when this feedback was provided.
	CreatedAt time.Time `json:"created_at"`
}

// NewFeedback creates a new Feedback entry.
func NewFeedback(tenantID, userID, ruleID, actionID, itemID string, positive bool) *Feedback {
	return &Feedback{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		RuleID:    ruleID,
		ActionID:  actionID,
		ItemID:    itemID,
		Positive:  positive,
		CreatedAt: time.Now().UTC(),
	}
}

// LearningConfig holds configuration for the rule learner.
type LearningConfig struct {
	// MinSupportCount is the minimum number of decisions needed to suggest a rule.
	MinSupportCount int

	// MinConfidence is the minimum confidence score to suggest a rule.
	MinConfidence float64

	// MaxSuggestions is the maximum number of suggestions to return.
	MaxSuggestions int

	// LookbackDays is how far back to look for decisions.
	LookbackDays int

	// EnableSenderRules enables learning sender-based rules.
	EnableSenderRules bool

	// EnableCategoryRules enables learning category-based rules.
	EnableCategoryRules bool

	// EnableKeywordRules enables learning keyword-based rules.
	EnableKeywordRules bool

	// EnablePatternRules enables learning pattern-based rules.
	EnablePatternRules bool
}

// DefaultLearningConfig returns sensible defaults for learning configuration.
func DefaultLearningConfig() *LearningConfig {
	return &LearningConfig{
		MinSupportCount:     5,
		MinConfidence:       0.8,
		MaxSuggestions:      10,
		LookbackDays:        30,
		EnableSenderRules:   true,
		EnableCategoryRules: true,
		EnableKeywordRules:  true,
		EnablePatternRules:  false, // More complex, disabled by default
	}
}

// RuleLearner learns automation rules from user decisions.
type RuleLearner struct {
	config    *LearningConfig
	decisions []*Decision
	mu        sync.RWMutex
}

// NewRuleLearner creates a new RuleLearner with the given configuration.
func NewRuleLearner(config *LearningConfig) *RuleLearner {
	if config == nil {
		config = DefaultLearningConfig()
	}
	return &RuleLearner{
		config:    config,
		decisions: make([]*Decision, 0),
	}
}

// LearnFromDecision adds a decision to the learning dataset.
func (l *RuleLearner) LearnFromDecision(decision *Decision) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.decisions = append(l.decisions, decision)
}

// LearnFromDecisions adds multiple decisions to the learning dataset.
func (l *RuleLearner) LearnFromDecisions(decisions []*Decision) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.decisions = append(l.decisions, decisions...)
}

// ClearDecisions clears all stored decisions.
func (l *RuleLearner) ClearDecisions() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.decisions = make([]*Decision, 0)
}

// SuggestRules analyzes decisions and suggests new automation rules.
func (l *RuleLearner) SuggestRules(ctx context.Context, tenantID string) ([]*RuleSuggestion, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Filter decisions for this tenant and within lookback period
	cutoff := time.Now().AddDate(0, 0, -l.config.LookbackDays)
	tenantDecisions := make([]*Decision, 0)
	for _, d := range l.decisions {
		if d.TenantID == tenantID && d.CreatedAt.After(cutoff) {
			tenantDecisions = append(tenantDecisions, d)
		}
	}

	if len(tenantDecisions) < l.config.MinSupportCount {
		return []*RuleSuggestion{}, nil
	}

	suggestions := make([]*RuleSuggestion, 0)

	// Learn sender-based rules
	if l.config.EnableSenderRules {
		senderSuggestions := l.learnSenderRules(tenantID, tenantDecisions)
		suggestions = append(suggestions, senderSuggestions...)
	}

	// Learn category-based rules
	if l.config.EnableCategoryRules {
		categorySuggestions := l.learnCategoryRules(tenantID, tenantDecisions)
		suggestions = append(suggestions, categorySuggestions...)
	}

	// Learn keyword-based rules
	if l.config.EnableKeywordRules {
		keywordSuggestions := l.learnKeywordRules(tenantID, tenantDecisions)
		suggestions = append(suggestions, keywordSuggestions...)
	}

	// Sort by confidence and limit
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	if len(suggestions) > l.config.MaxSuggestions {
		suggestions = suggestions[:l.config.MaxSuggestions]
	}

	return suggestions, nil
}

// learnSenderRules learns rules based on sender patterns.
func (l *RuleLearner) learnSenderRules(tenantID string, decisions []*Decision) []*RuleSuggestion {
	suggestions := make([]*RuleSuggestion, 0)

	// Group decisions by sender and action
	senderActions := make(map[string]map[ActionType][]*Decision)

	for _, d := range decisions {
		if d.Item == nil || d.Item.Sender == "" {
			continue
		}

		sender := d.Item.Sender
		if _, ok := senderActions[sender]; !ok {
			senderActions[sender] = make(map[ActionType][]*Decision)
		}
		senderActions[sender][d.Action] = append(senderActions[sender][d.Action], d)
	}

	// Find senders with consistent actions
	for sender, actions := range senderActions {
		for action, senderDecisions := range actions {
			if len(senderDecisions) < l.config.MinSupportCount {
				continue
			}

			// Calculate total decisions for this sender
			totalForSender := 0
			for _, decs := range actions {
				totalForSender += len(decs)
			}

			// Calculate confidence
			confidence := float64(len(senderDecisions)) / float64(totalForSender)

			if confidence >= l.config.MinConfidence {
				// Create rule suggestion
				condition := SenderCondition("eq", sender)
				rule := NewRule(tenantID, "Auto-"+string(action)+" from "+sender, condition, action)
				rule.Source = "learned"
				rule.Description = "Automatically learned from user decisions"

				suggestion := NewRuleSuggestion(tenantID, rule, confidence, len(senderDecisions))
				suggestion.Explanation = formatExplanation(
					"Users consistently %s items from sender '%s' (%d/%d = %.0f%% of the time)",
					action, sender, len(senderDecisions), totalForSender, confidence*100,
				)

				// Add up to 3 example decisions
				maxExamples := 3
				if len(senderDecisions) < maxExamples {
					maxExamples = len(senderDecisions)
				}
				suggestion.ExampleDecisions = senderDecisions[:maxExamples]

				suggestions = append(suggestions, suggestion)
			}
		}
	}

	return suggestions
}

// learnCategoryRules learns rules based on category patterns.
func (l *RuleLearner) learnCategoryRules(tenantID string, decisions []*Decision) []*RuleSuggestion {
	suggestions := make([]*RuleSuggestion, 0)

	// Group decisions by category and action
	categoryActions := make(map[string]map[ActionType][]*Decision)

	for _, d := range decisions {
		if d.Item == nil || d.Item.Category == "" {
			continue
		}

		category := d.Item.Category
		if _, ok := categoryActions[category]; !ok {
			categoryActions[category] = make(map[ActionType][]*Decision)
		}
		categoryActions[category][d.Action] = append(categoryActions[category][d.Action], d)
	}

	// Find categories with consistent actions
	for category, actions := range categoryActions {
		for action, categoryDecisions := range actions {
			if len(categoryDecisions) < l.config.MinSupportCount {
				continue
			}

			// Calculate total decisions for this category
			totalForCategory := 0
			for _, decs := range actions {
				totalForCategory += len(decs)
			}

			// Calculate confidence
			confidence := float64(len(categoryDecisions)) / float64(totalForCategory)

			if confidence >= l.config.MinConfidence {
				// Create rule suggestion
				condition := CategoryCondition("eq", category)
				rule := NewRule(tenantID, "Auto-"+string(action)+" category: "+category, condition, action)
				rule.Source = "learned"
				rule.Description = "Automatically learned from user decisions"

				suggestion := NewRuleSuggestion(tenantID, rule, confidence, len(categoryDecisions))
				suggestion.Explanation = formatExplanation(
					"Users consistently %s items in category '%s' (%d/%d = %.0f%% of the time)",
					action, category, len(categoryDecisions), totalForCategory, confidence*100,
				)

				maxExamples := 3
				if len(categoryDecisions) < maxExamples {
					maxExamples = len(categoryDecisions)
				}
				suggestion.ExampleDecisions = categoryDecisions[:maxExamples]

				suggestions = append(suggestions, suggestion)
			}
		}
	}

	return suggestions
}

// learnKeywordRules learns rules based on keyword patterns.
func (l *RuleLearner) learnKeywordRules(tenantID string, decisions []*Decision) []*RuleSuggestion {
	suggestions := make([]*RuleSuggestion, 0)

	// Build keyword frequency maps per action
	keywordsByAction := make(map[ActionType]map[string]int)
	totalByAction := make(map[ActionType]int)

	for _, d := range decisions {
		if d.Item == nil {
			continue
		}

		totalByAction[d.Action]++

		// Extract keywords from content
		keywords := extractKeywords(d.Item)
		if _, ok := keywordsByAction[d.Action]; !ok {
			keywordsByAction[d.Action] = make(map[string]int)
		}
		for _, kw := range keywords {
			keywordsByAction[d.Action][kw]++
		}
	}

	// Find keywords that strongly correlate with specific actions
	for action, keywords := range keywordsByAction {
		total := totalByAction[action]
		if total < l.config.MinSupportCount {
			continue
		}

		for keyword, count := range keywords {
			// Check if this keyword appears in a significant portion of decisions
			actionRatio := float64(count) / float64(total)
			if actionRatio < 0.5 {
				continue
			}

			// Check how often this keyword appears with other actions
			keywordTotalCount := count
			for otherAction, otherKeywords := range keywordsByAction {
				if otherAction != action {
					keywordTotalCount += otherKeywords[keyword]
				}
			}

			confidence := float64(count) / float64(keywordTotalCount)

			if confidence >= l.config.MinConfidence && count >= l.config.MinSupportCount {
				condition := KeywordsCondition([]string{keyword}, false)
				rule := NewRule(tenantID, "Auto-"+string(action)+" keyword: "+keyword, condition, action)
				rule.Source = "learned"
				rule.Description = "Automatically learned from user decisions"

				suggestion := NewRuleSuggestion(tenantID, rule, confidence, count)
				suggestion.Explanation = formatExplanation(
					"Items containing '%s' are consistently %s (%d/%d = %.0f%% of the time)",
					keyword, action, count, keywordTotalCount, confidence*100,
				)

				suggestions = append(suggestions, suggestion)
			}
		}
	}

	return suggestions
}

// TrainRule creates or updates a rule based on provided examples.
func (l *RuleLearner) TrainRule(ctx context.Context, tenantID, name string, examples []Example) (*Rule, error) {
	if len(examples) == 0 {
		return nil, ErrInvalidRule
	}

	// Determine the dominant action
	actionCounts := make(map[ActionType]float64)
	for _, ex := range examples {
		weight := ex.Weight
		if weight == 0 {
			weight = 1.0
		}
		actionCounts[ex.Action] += weight
	}

	var dominantAction ActionType
	maxCount := 0.0
	for action, count := range actionCounts {
		if count > maxCount {
			maxCount = count
			dominantAction = action
		}
	}

	// Build a condition that matches the examples
	condition := l.buildConditionFromExamples(examples, dominantAction)

	rule := NewRule(tenantID, name, condition, dominantAction)
	rule.Source = "trained"

	return rule, nil
}

// buildConditionFromExamples creates a condition from training examples.
func (l *RuleLearner) buildConditionFromExamples(examples []Example, targetAction ActionType) Condition {
	// Filter examples for the target action
	relevantExamples := make([]Example, 0)
	for _, ex := range examples {
		if ex.Action == targetAction {
			relevantExamples = append(relevantExamples, ex)
		}
	}

	if len(relevantExamples) == 0 {
		// Return a condition that matches nothing
		return Condition{Type: ConditionPattern, Pattern: "^$"}
	}

	// Find common attributes
	conditions := make([]Condition, 0)

	// Check for common sender
	senders := make(map[string]int)
	for _, ex := range relevantExamples {
		if ex.Item != nil && ex.Item.Sender != "" {
			senders[ex.Item.Sender]++
		}
	}
	for sender, count := range senders {
		if float64(count)/float64(len(relevantExamples)) > 0.8 {
			conditions = append(conditions, SenderCondition("eq", sender))
			break
		}
	}

	// Check for common category
	categories := make(map[string]int)
	for _, ex := range relevantExamples {
		if ex.Item != nil && ex.Item.Category != "" {
			categories[ex.Item.Category]++
		}
	}
	for category, count := range categories {
		if float64(count)/float64(len(relevantExamples)) > 0.8 {
			conditions = append(conditions, CategoryCondition("eq", category))
			break
		}
	}

	// Check for common source
	sources := make(map[string]int)
	for _, ex := range relevantExamples {
		if ex.Item != nil && ex.Item.Source != "" {
			sources[ex.Item.Source]++
		}
	}
	for source, count := range sources {
		if float64(count)/float64(len(relevantExamples)) > 0.8 {
			conditions = append(conditions, SourceCondition(source))
			break
		}
	}

	// If no specific conditions found, use a catch-all
	if len(conditions) == 0 {
		// Try to find common keywords
		commonKeywords := findCommonKeywords(relevantExamples)
		if len(commonKeywords) > 0 {
			return KeywordsCondition(commonKeywords, false)
		}
		// Return always-false condition
		return Condition{Type: ConditionPattern, Pattern: "^$"}
	}

	// Combine conditions with AND
	if len(conditions) == 1 {
		return conditions[0]
	}
	return AndCondition(conditions...)
}

// extractKeywords extracts significant keywords from a review item.
func extractKeywords(item *ReviewItem) []string {
	keywords := make([]string, 0)

	// Start with item's own keywords
	keywords = append(keywords, item.Keywords...)

	// Extract words from subject (simplified)
	words := strings.Fields(strings.ToLower(item.Subject + " " + item.Summary))
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true, "as": true,
		"this": true, "that": true, "it": true, "its": true, "if": true,
	}

	for _, word := range words {
		word = strings.Trim(word, ".,!?;:\"'()[]{}") // Clean punctuation
		if len(word) > 3 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// findCommonKeywords finds keywords that appear in most examples.
func findCommonKeywords(examples []Example) []string {
	if len(examples) == 0 {
		return nil
	}

	// Count keyword occurrences
	keywordCounts := make(map[string]int)
	for _, ex := range examples {
		if ex.Item == nil {
			continue
		}
		seen := make(map[string]bool)
		for _, kw := range extractKeywords(ex.Item) {
			if !seen[kw] {
				keywordCounts[kw]++
				seen[kw] = true
			}
		}
	}

	// Find keywords that appear in at least 50% of examples
	threshold := int(math.Ceil(float64(len(examples)) * 0.5))
	common := make([]string, 0)
	for kw, count := range keywordCounts {
		if count >= threshold {
			common = append(common, kw)
		}
	}

	// Limit to top 5 keywords
	if len(common) > 5 {
		sort.Slice(common, func(i, j int) bool {
			return keywordCounts[common[i]] > keywordCounts[common[j]]
		})
		common = common[:5]
	}

	return common
}

// formatExplanation formats an explanation string.
func formatExplanation(format string, args ...interface{}) string {
	return strings.TrimSpace(strings.ReplaceAll(
		strings.ReplaceAll(
			strings.ReplaceAll(format, "%s", placeholderReplace(args)),
			"%d", placeholderReplace(args),
		),
		"%.0f", placeholderReplace(args),
	))
}

// placeholderReplace is a helper for explanation formatting.
func placeholderReplace(args []interface{}) string {
	if len(args) == 0 {
		return ""
	}
	return strings.Join(toStringSlice(args), " ")
}

// toStringSlice converts interface slice to string slice.
func toStringSlice(args []interface{}) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			result = append(result, v)
		case ActionType:
			result = append(result, string(v))
		default:
			// Skip non-string types for this simple helper
		}
	}
	return result
}

// FeedbackTracker tracks user feedback on automated actions.
type FeedbackTracker struct {
	feedback map[string][]*Feedback // ruleID -> feedback
	mu       sync.RWMutex
}

// NewFeedbackTracker creates a new FeedbackTracker.
func NewFeedbackTracker() *FeedbackTracker {
	return &FeedbackTracker{
		feedback: make(map[string][]*Feedback),
	}
}

// RecordFeedback records feedback for a rule.
func (t *FeedbackTracker) RecordFeedback(fb *Feedback) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.feedback[fb.RuleID] = append(t.feedback[fb.RuleID], fb)
}

// GetFeedback returns all feedback for a rule.
func (t *FeedbackTracker) GetFeedback(ruleID string) []*Feedback {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.feedback[ruleID]
}

// GetFeedbackScore returns the positive feedback ratio for a rule.
func (t *FeedbackTracker) GetFeedbackScore(ruleID string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	fb := t.feedback[ruleID]
	if len(fb) == 0 {
		return 0
	}

	positive := 0
	for _, f := range fb {
		if f.Positive {
			positive++
		}
	}

	return float64(positive) / float64(len(fb))
}

// ShouldDisableRule returns true if feedback suggests the rule should be disabled.
func (t *FeedbackTracker) ShouldDisableRule(ruleID string, minFeedback int, maxNegativeRatio float64) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	fb := t.feedback[ruleID]
	if len(fb) < minFeedback {
		return false
	}

	negativeCount := 0
	for _, f := range fb {
		if !f.Positive {
			negativeCount++
		}
	}

	negativeRatio := float64(negativeCount) / float64(len(fb))
	return negativeRatio > maxNegativeRatio
}
