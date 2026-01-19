// Package pattern provides pattern learning capabilities for the review service.
package pattern

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/services/review/automation"
)

// LearnerConfig holds configuration for the PatternLearner.
type LearnerConfig struct {
	// MinSupportCount is the minimum number of decisions needed to learn a pattern.
	MinSupportCount int

	// MinConfidence is the minimum confidence to suggest a pattern.
	MinConfidence float64

	// MaxSuggestions limits the number of pattern suggestions.
	MaxSuggestions int

	// LookbackDays is how far back to look for decisions.
	LookbackDays int

	// EnableSenderPatterns enables learning sender-based patterns.
	EnableSenderPatterns bool

	// EnableTopicPatterns enables learning topic/category-based patterns.
	EnableTopicPatterns bool

	// EnableTimePatterns enables learning time-based patterns.
	EnableTimePatterns bool

	// EnableContentPatterns enables learning content-based patterns.
	EnableContentPatterns bool

	// MinTimePeakRatio is the minimum ratio for time-based pattern peaks.
	MinTimePeakRatio float64
}

// DefaultLearnerConfig returns sensible defaults.
func DefaultLearnerConfig() *LearnerConfig {
	return &LearnerConfig{
		MinSupportCount:       5,
		MinConfidence:         0.75,
		MaxSuggestions:        10,
		LookbackDays:          30,
		EnableSenderPatterns:  true,
		EnableTopicPatterns:   true,
		EnableTimePatterns:    true,
		EnableContentPatterns: true,
		MinTimePeakRatio:      1.5, // 50% more than average
	}
}

// DecisionRecord stores a decision with additional context for learning.
type DecisionRecord struct {
	// Decision is the original decision.
	Decision *automation.Decision

	// Features holds extracted features for pattern learning.
	Features map[string]interface{}

	// RecordedAt is when this record was created.
	RecordedAt time.Time
}

// PatternLearner learns patterns from user decisions.
type PatternLearner struct {
	config      *LearnerConfig
	detector    *PatternDetector
	decisions   map[string][]*DecisionRecord // tenantID -> decisions
	suggestions map[string][]*PatternSuggestion // tenantID -> suggestions
	mu          sync.RWMutex
}

// NewPatternLearner creates a new PatternLearner.
func NewPatternLearner(config *LearnerConfig, detector *PatternDetector) *PatternLearner {
	if config == nil {
		config = DefaultLearnerConfig()
	}
	return &PatternLearner{
		config:      config,
		detector:    detector,
		decisions:   make(map[string][]*DecisionRecord),
		suggestions: make(map[string][]*PatternSuggestion),
	}
}

// RecordDecision adds a decision to the learning dataset.
func (l *PatternLearner) RecordDecision(decision *automation.Decision) {
	l.mu.Lock()
	defer l.mu.Unlock()

	record := &DecisionRecord{
		Decision:   decision,
		Features:   l.extractFeatures(decision),
		RecordedAt: time.Now().UTC(),
	}

	l.decisions[decision.TenantID] = append(l.decisions[decision.TenantID], record)
}

// extractFeatures extracts features from a decision for pattern learning.
func (l *PatternLearner) extractFeatures(decision *automation.Decision) map[string]interface{} {
	features := make(map[string]interface{})

	if decision.Item == nil {
		return features
	}

	item := decision.Item

	// Basic features
	features["sender"] = item.Sender
	features["sender_domain"] = extractDomain(item.Sender)
	features["category"] = item.Category
	features["source"] = item.Source
	features["content_type"] = item.ContentType
	features["priority"] = item.Priority
	features["confidence"] = item.Confidence

	// Time features
	features["hour_of_day"] = decision.CreatedAt.Hour()
	features["day_of_week"] = int(decision.CreatedAt.Weekday())
	features["is_weekend"] = decision.CreatedAt.Weekday() == time.Saturday || decision.CreatedAt.Weekday() == time.Sunday

	// Content features
	features["subject_words"] = extractWords(item.Subject)
	features["keywords"] = item.Keywords
	features["has_attachment"] = hasAttachment(item)
	features["content_length"] = len(item.Body)

	// Action features
	features["action"] = string(decision.Action)
	features["time_spent_ms"] = decision.TimeSpentMs
	features["overrode_automation"] = decision.OverrodeAutomation

	return features
}

// extractDomain extracts the domain from an email address.
func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// extractWords extracts significant words from text.
func extractWords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true, "as": true,
		"re": true, "fwd": true, "fw": true,
	}

	result := make([]string, 0)
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:\"'()[]{}")
		if len(word) > 2 && !stopWords[word] {
			result = append(result, word)
		}
	}
	return result
}

// hasAttachment checks if an item has attachments.
func hasAttachment(item *automation.ReviewItem) bool {
	if item.Metadata == nil {
		return false
	}
	if v, ok := item.Metadata["has_attachment"]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// ExtractPattern extracts a pattern from a set of decisions.
func (l *PatternLearner) ExtractPattern(decisions []*automation.Decision) *Pattern {
	if len(decisions) < l.config.MinSupportCount {
		return nil
	}

	// Analyze decisions to find common characteristics
	senderCounts := make(map[string]int)
	categoryCounts := make(map[string]int)
	sourceCounts := make(map[string]int)
	actionCounts := make(map[automation.ActionType]int)
	hourCounts := make(map[int]int)

	for _, d := range decisions {
		if d.Item != nil {
			senderCounts[d.Item.Sender]++
			categoryCounts[d.Item.Category]++
			sourceCounts[d.Item.Source]++
		}
		actionCounts[d.Action]++
		hourCounts[d.CreatedAt.Hour()]++
	}

	// Find dominant action
	var dominantAction automation.ActionType
	maxActionCount := 0
	for action, count := range actionCounts {
		if count > maxActionCount {
			maxActionCount = count
			dominantAction = action
		}
	}

	confidence := float64(maxActionCount) / float64(len(decisions))
	if confidence < l.config.MinConfidence {
		return nil
	}

	// Build conditions based on common characteristics
	conditions := make([]PatternCondition, 0)

	// Check for sender pattern
	for sender, count := range senderCounts {
		if sender != "" && float64(count)/float64(len(decisions)) > 0.8 {
			conditions = append(conditions, NewPatternCondition("sender", OpEquals, sender))
			break
		}
	}

	// Check for category pattern
	for category, count := range categoryCounts {
		if category != "" && float64(count)/float64(len(decisions)) > 0.8 {
			conditions = append(conditions, NewPatternCondition("category", OpEquals, category))
			break
		}
	}

	// Check for source pattern
	for source, count := range sourceCounts {
		if source != "" && float64(count)/float64(len(decisions)) > 0.8 {
			conditions = append(conditions, NewPatternCondition("source", OpEquals, source))
			break
		}
	}

	if len(conditions) == 0 {
		return nil
	}

	// Create pattern
	tenantID := ""
	if len(decisions) > 0 {
		tenantID = decisions[0].TenantID
	}

	patternType := determinePatternType(conditions)
	pattern := NewPattern(tenantID, generatePatternName(conditions, dominantAction), patternType, conditions)
	pattern.Confidence = confidence
	pattern.SuggestedAction = string(dominantAction)
	pattern.AutoGenerated = true
	pattern.Source = "learned"

	// Add examples
	maxExamples := 5
	if len(decisions) < maxExamples {
		maxExamples = len(decisions)
	}
	for i := 0; i < maxExamples; i++ {
		pattern.AddExample(PatternExample{
			ItemID:    decisions[i].ItemID,
			Timestamp: decisions[i].CreatedAt,
			Action:    string(decisions[i].Action),
		})
	}

	return pattern
}

// determinePatternType determines the pattern type from conditions.
func determinePatternType(conditions []PatternCondition) PatternType {
	if len(conditions) == 0 {
		return PatternContent
	}

	// Check first condition's field
	for _, c := range conditions {
		switch c.Field {
		case "sender", "sender_domain":
			return PatternSender
		case "category", "topic":
			return PatternTopic
		case "hour_of_day", "day_of_week":
			return PatternTime
		case "source":
			return PatternSource
		case "priority":
			return PatternPriority
		}
	}

	if len(conditions) > 1 {
		return PatternComposite
	}

	return PatternContent
}

// generatePatternName generates a human-readable name for a pattern.
func generatePatternName(conditions []PatternCondition, action automation.ActionType) string {
	parts := make([]string, 0)

	for _, c := range conditions {
		if s, ok := c.Value.(string); ok {
			parts = append(parts, s)
		}
	}

	actionStr := string(action)
	if len(parts) > 0 {
		return "Auto-" + actionStr + ": " + strings.Join(parts, ", ")
	}
	return "Auto-" + actionStr + " pattern"
}

// SuggestPatterns returns all current pattern suggestions.
func (l *PatternLearner) SuggestPatterns() []*PatternSuggestion {
	l.mu.RLock()
	defer l.mu.RUnlock()

	all := make([]*PatternSuggestion, 0)
	for _, suggestions := range l.suggestions {
		all = append(all, suggestions...)
	}
	return all
}

// SuggestPatternsForTenant analyzes decisions and suggests patterns for a tenant.
func (l *PatternLearner) SuggestPatternsForTenant(ctx context.Context, tenantID string) ([]*PatternSuggestion, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Get decisions within lookback period
	cutoff := time.Now().AddDate(0, 0, -l.config.LookbackDays)
	tenantDecisions := make([]*DecisionRecord, 0)
	for _, d := range l.decisions[tenantID] {
		if d.RecordedAt.After(cutoff) {
			tenantDecisions = append(tenantDecisions, d)
		}
	}

	if len(tenantDecisions) < l.config.MinSupportCount {
		return []*PatternSuggestion{}, nil
	}

	suggestions := make([]*PatternSuggestion, 0)

	// Learn sender patterns
	if l.config.EnableSenderPatterns {
		senderSuggestions := l.learnSenderPatterns(tenantID, tenantDecisions)
		suggestions = append(suggestions, senderSuggestions...)
	}

	// Learn topic/category patterns
	if l.config.EnableTopicPatterns {
		topicSuggestions := l.learnTopicPatterns(tenantID, tenantDecisions)
		suggestions = append(suggestions, topicSuggestions...)
	}

	// Learn time patterns
	if l.config.EnableTimePatterns {
		timeSuggestions := l.learnTimePatterns(tenantID, tenantDecisions)
		suggestions = append(suggestions, timeSuggestions...)
	}

	// Sort by confidence and limit
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	if len(suggestions) > l.config.MaxSuggestions {
		suggestions = suggestions[:l.config.MaxSuggestions]
	}

	// Store suggestions
	l.suggestions[tenantID] = suggestions

	return suggestions, nil
}

// learnSenderPatterns learns patterns based on sender.
func (l *PatternLearner) learnSenderPatterns(tenantID string, records []*DecisionRecord) []*PatternSuggestion {
	suggestions := make([]*PatternSuggestion, 0)

	// Group by sender and action
	senderActions := make(map[string]map[automation.ActionType][]*DecisionRecord)

	for _, r := range records {
		sender, ok := r.Features["sender"].(string)
		if !ok || sender == "" {
			continue
		}

		if _, exists := senderActions[sender]; !exists {
			senderActions[sender] = make(map[automation.ActionType][]*DecisionRecord)
		}
		senderActions[sender][r.Decision.Action] = append(senderActions[sender][r.Decision.Action], r)
	}

	// Find senders with consistent actions
	for sender, actions := range senderActions {
		for action, actionRecords := range actions {
			if len(actionRecords) < l.config.MinSupportCount {
				continue
			}

			// Calculate total for this sender
			total := 0
			for _, recs := range actions {
				total += len(recs)
			}

			confidence := float64(len(actionRecords)) / float64(total)
			if confidence < l.config.MinConfidence {
				continue
			}

			// Create pattern
			conditions := []PatternCondition{
				NewPatternCondition("sender", OpEquals, sender),
			}
			pattern := NewPattern(tenantID, "Auto-"+string(action)+" from "+sender, PatternSender, conditions)
			pattern.Confidence = confidence
			pattern.SuggestedAction = string(action)
			pattern.AutoGenerated = true
			pattern.Source = "learned"

			suggestion := NewPatternSuggestion(tenantID, pattern, confidence, len(actionRecords))
			suggestion.Explanation = formatSuggestionExplanation(
				"Users consistently %s items from '%s' (%d of %d, %.0f%% confidence)",
				action, sender, len(actionRecords), total, confidence*100,
			)
			suggestion.PotentialImpact = estimateImpact(len(actionRecords), l.config.LookbackDays)

			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions
}

// learnTopicPatterns learns patterns based on category/topic.
func (l *PatternLearner) learnTopicPatterns(tenantID string, records []*DecisionRecord) []*PatternSuggestion {
	suggestions := make([]*PatternSuggestion, 0)

	// Group by category and action
	categoryActions := make(map[string]map[automation.ActionType][]*DecisionRecord)

	for _, r := range records {
		category, ok := r.Features["category"].(string)
		if !ok || category == "" {
			continue
		}

		if _, exists := categoryActions[category]; !exists {
			categoryActions[category] = make(map[automation.ActionType][]*DecisionRecord)
		}
		categoryActions[category][r.Decision.Action] = append(categoryActions[category][r.Decision.Action], r)
	}

	// Find categories with consistent actions
	for category, actions := range categoryActions {
		for action, actionRecords := range actions {
			if len(actionRecords) < l.config.MinSupportCount {
				continue
			}

			total := 0
			for _, recs := range actions {
				total += len(recs)
			}

			confidence := float64(len(actionRecords)) / float64(total)
			if confidence < l.config.MinConfidence {
				continue
			}

			conditions := []PatternCondition{
				NewPatternCondition("category", OpEquals, category),
			}
			pattern := NewPattern(tenantID, "Auto-"+string(action)+" category: "+category, PatternTopic, conditions)
			pattern.Confidence = confidence
			pattern.SuggestedAction = string(action)
			pattern.AutoGenerated = true
			pattern.Source = "learned"

			suggestion := NewPatternSuggestion(tenantID, pattern, confidence, len(actionRecords))
			suggestion.Explanation = formatSuggestionExplanation(
				"Users consistently %s items in category '%s' (%d of %d, %.0f%% confidence)",
				action, category, len(actionRecords), total, confidence*100,
			)
			suggestion.PotentialImpact = estimateImpact(len(actionRecords), l.config.LookbackDays)

			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions
}

// learnTimePatterns learns patterns based on time of day/week.
func (l *PatternLearner) learnTimePatterns(tenantID string, records []*DecisionRecord) []*PatternSuggestion {
	suggestions := make([]*PatternSuggestion, 0)

	// Analyze hour distribution by action
	hourActions := make(map[int]map[automation.ActionType]int)
	actionTotals := make(map[automation.ActionType]int)

	for _, r := range records {
		hour, ok := r.Features["hour_of_day"].(int)
		if !ok {
			continue
		}

		if _, exists := hourActions[hour]; !exists {
			hourActions[hour] = make(map[automation.ActionType]int)
		}
		hourActions[hour][r.Decision.Action]++
		actionTotals[r.Decision.Action]++
	}

	// Find hours with significantly higher action rates
	for action, total := range actionTotals {
		if total < l.config.MinSupportCount {
			continue
		}

		averagePerHour := float64(total) / 24.0

		for hour, actions := range hourActions {
			count := actions[action]
			if count == 0 {
				continue
			}

			// Check if this hour has significantly more than average
			ratio := float64(count) / averagePerHour
			if ratio >= l.config.MinTimePeakRatio && count >= l.config.MinSupportCount {
				confidence := float64(count) / float64(total) * ratio / 2 // Normalize

				if confidence >= l.config.MinConfidence {
					conditions := []PatternCondition{
						NewPatternCondition("hour_of_day", OpEquals, hour),
					}
					pattern := NewPattern(
						tenantID,
						formatTimeName(hour, action),
						PatternTime,
						conditions,
					)
					pattern.Confidence = confidence
					pattern.SuggestedAction = string(action)
					pattern.AutoGenerated = true
					pattern.Source = "learned"

					suggestion := NewPatternSuggestion(tenantID, pattern, confidence, count)
					suggestion.Explanation = formatSuggestionExplanation(
						"Items are %s %.1fx more often at %02d:00 (%d occurrences)",
						action, ratio, hour, count,
					)
					suggestion.PotentialImpact = estimateImpact(count, l.config.LookbackDays)

					suggestions = append(suggestions, suggestion)
				}
			}
		}
	}

	return suggestions
}

// formatTimeName generates a readable name for time-based patterns.
func formatTimeName(hour int, action automation.ActionType) string {
	timeOfDay := "morning"
	if hour >= 12 && hour < 17 {
		timeOfDay = "afternoon"
	} else if hour >= 17 && hour < 21 {
		timeOfDay = "evening"
	} else if hour >= 21 || hour < 6 {
		timeOfDay = "night"
	}
	return "Auto-" + string(action) + " " + timeOfDay + " items"
}

// formatSuggestionExplanation formats an explanation string.
func formatSuggestionExplanation(format string, args ...interface{}) string {
	// Simple implementation that works for our use case
	result := format
	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			result = strings.Replace(result, "%s", v, 1)
		case automation.ActionType:
			result = strings.Replace(result, "%s", string(v), 1)
		case int:
			result = strings.Replace(result, "%d", toString(v), 1)
		case float64:
			if strings.Contains(result, "%.0f") {
				result = strings.Replace(result, "%.0f", toString(int(v)), 1)
			} else if strings.Contains(result, "%.1f") {
				result = strings.Replace(result, "%.1f", toStringFloat(v), 1)
			}
		}
	}
	return result
}

// toString converts int to string.
func toString(v int) string {
	if v == 0 {
		return "0"
	}
	if v < 0 {
		return "-" + toString(-v)
	}
	result := ""
	for v > 0 {
		result = string(rune('0'+v%10)) + result
		v /= 10
	}
	return result
}

// toStringFloat converts float64 to string with 1 decimal place.
func toStringFloat(v float64) string {
	intPart := int(v)
	decPart := int((v - float64(intPart)) * 10)
	if decPart < 0 {
		decPart = -decPart
	}
	return toString(intPart) + "." + toString(decPart)
}

// estimateImpact estimates how many items would be affected by a pattern.
func estimateImpact(observedCount, lookbackDays int) int {
	if lookbackDays == 0 {
		return observedCount
	}
	// Project to next 30 days based on observed rate
	dailyRate := float64(observedCount) / float64(lookbackDays)
	return int(dailyRate * 30)
}

// DecisionTracker records and manages user decisions for learning.
type DecisionTracker struct {
	decisions  []*automation.Decision
	maxSize    int
	mu         sync.RWMutex
}

// NewDecisionTracker creates a new DecisionTracker.
func NewDecisionTracker(maxSize int) *DecisionTracker {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &DecisionTracker{
		decisions: make([]*automation.Decision, 0),
		maxSize:   maxSize,
	}
}

// Record adds a decision to the tracker.
func (t *DecisionTracker) Record(decision *automation.Decision) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.decisions = append(t.decisions, decision)

	// Trim if over max size (FIFO)
	if len(t.decisions) > t.maxSize {
		t.decisions = t.decisions[len(t.decisions)-t.maxSize:]
	}
}

// GetDecisions returns all tracked decisions.
func (t *DecisionTracker) GetDecisions() []*automation.Decision {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*automation.Decision, len(t.decisions))
	copy(result, t.decisions)
	return result
}

// GetDecisionsByTenant returns decisions for a specific tenant.
func (t *DecisionTracker) GetDecisionsByTenant(tenantID string) []*automation.Decision {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*automation.Decision, 0)
	for _, d := range t.decisions {
		if d.TenantID == tenantID {
			result = append(result, d)
		}
	}
	return result
}

// GetDecisionsSince returns decisions since a given time.
func (t *DecisionTracker) GetDecisionsSince(since time.Time) []*automation.Decision {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*automation.Decision, 0)
	for _, d := range t.decisions {
		if d.CreatedAt.After(since) {
			result = append(result, d)
		}
	}
	return result
}

// Clear removes all tracked decisions.
func (t *DecisionTracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.decisions = make([]*automation.Decision, 0)
}

// FeedbackLoop manages pattern accuracy improvement through feedback.
type FeedbackLoop struct {
	// positive tracks positive feedback per pattern
	positive map[string]int
	// negative tracks negative feedback per pattern
	negative map[string]int
	// overrides tracks when users override pattern suggestions
	overrides map[string]int
	mu        sync.RWMutex
}

// NewFeedbackLoop creates a new FeedbackLoop.
func NewFeedbackLoop() *FeedbackLoop {
	return &FeedbackLoop{
		positive:  make(map[string]int),
		negative:  make(map[string]int),
		overrides: make(map[string]int),
	}
}

// RecordFeedback records user feedback on a pattern.
func (f *FeedbackLoop) RecordFeedback(patternID string, positive bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if positive {
		f.positive[patternID]++
	} else {
		f.negative[patternID]++
	}
}

// RecordOverride records when a user overrides a pattern suggestion.
func (f *FeedbackLoop) RecordOverride(patternID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.overrides[patternID]++
}

// GetAccuracy returns the accuracy rate for a pattern.
func (f *FeedbackLoop) GetAccuracy(patternID string) float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	pos := f.positive[patternID]
	neg := f.negative[patternID]
	total := pos + neg

	if total == 0 {
		return 1.0 // Default to high accuracy when no feedback
	}

	return float64(pos) / float64(total)
}

// GetOverrideRate returns the override rate for a pattern.
func (f *FeedbackLoop) GetOverrideRate(patternID string) float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	overrides := f.overrides[patternID]
	pos := f.positive[patternID]
	total := pos + overrides

	if total == 0 {
		return 0.0
	}

	return float64(overrides) / float64(total)
}

// ShouldDisable returns true if a pattern should be disabled due to poor performance.
func (f *FeedbackLoop) ShouldDisable(patternID string, minFeedback int, maxOverrideRate float64) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	pos := f.positive[patternID]
	neg := f.negative[patternID]
	total := pos + neg

	if total < minFeedback {
		return false
	}

	accuracy := float64(pos) / float64(total)
	return accuracy < 0.5 || f.GetOverrideRate(patternID) > maxOverrideRate
}

// GetPatternStats returns feedback statistics for a pattern.
func (f *FeedbackLoop) GetPatternStats(patternID string) (positive, negative, overrides int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.positive[patternID], f.negative[patternID], f.overrides[patternID]
}
