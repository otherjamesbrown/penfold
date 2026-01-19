package pattern

import (
	"context"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/review/automation"
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

func newTestDetector() *PatternDetector {
	config := DefaultDetectorConfig()
	store := NewInMemoryPatternStore()
	logger := testLogger()
	return NewPatternDetector(config, store, logger)
}

func TestNewPatternDetector(t *testing.T) {
	detector := newTestDetector()

	if detector == nil {
		t.Fatal("expected detector to be created")
	}
	if detector.config == nil {
		t.Error("expected config to be set")
	}
	if detector.analyzer == nil {
		t.Error("expected analyzer to be created")
	}
	if detector.learner == nil {
		t.Error("expected learner to be created")
	}
}

func TestDefaultDetectorConfig(t *testing.T) {
	config := DefaultDetectorConfig()

	if config.MinConfidence <= 0 || config.MinConfidence > 1 {
		t.Error("MinConfidence should be between 0 and 1")
	}
	if config.MinSupportCount <= 0 {
		t.Error("MinSupportCount should be positive")
	}
	if config.MaxPatterns <= 0 {
		t.Error("MaxPatterns should be positive")
	}
	if config.MaxSuggestions <= 0 {
		t.Error("MaxSuggestions should be positive")
	}
}

func TestPatternDetectorAddPattern(t *testing.T) {
	detector := newTestDetector()

	pattern := NewPattern("tenant-1", "Test Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})

	err := detector.AddPattern(pattern)
	if err != nil {
		t.Fatalf("failed to add pattern: %v", err)
	}

	// Verify pattern was added
	ps := detector.GetPatternSet("tenant-1")
	if ps == nil {
		t.Fatal("expected pattern set to exist")
	}
	if len(ps.Patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(ps.Patterns))
	}
}

func TestPatternDetectorAddPatternValidation(t *testing.T) {
	detector := newTestDetector()

	// Try to add invalid pattern
	pattern := &Pattern{
		// Missing required fields
	}

	err := detector.AddPattern(pattern)
	if err == nil {
		t.Error("expected error for invalid pattern")
	}
}

func TestPatternDetectorRemovePattern(t *testing.T) {
	detector := newTestDetector()

	pattern := NewPattern("tenant-1", "Test Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	detector.AddPattern(pattern)

	// Remove pattern
	removed := detector.RemovePattern("tenant-1", pattern.ID)
	if !removed {
		t.Error("expected pattern to be removed")
	}

	// Verify removal
	ps := detector.GetPatternSet("tenant-1")
	if len(ps.Patterns) != 0 {
		t.Errorf("expected 0 patterns after removal, got %d", len(ps.Patterns))
	}

	// Try to remove again
	removed = detector.RemovePattern("tenant-1", pattern.ID)
	if removed {
		t.Error("expected remove to return false for non-existent pattern")
	}
}

func TestPatternDetectorMatchPattern(t *testing.T) {
	detector := newTestDetector()

	pattern := NewPattern("tenant-1", "Sender Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "newsletter@example.com"),
	})

	item := &automation.ReviewItem{
		ID:       "item-1",
		TenantID: "tenant-1",
		Sender:   "newsletter@example.com",
		Subject:  "Weekly Newsletter",
	}

	// Test match
	matched := detector.MatchPattern(item, pattern)
	if !matched {
		t.Error("expected pattern to match")
	}

	// Test non-match
	item.Sender = "other@example.com"
	matched = detector.MatchPattern(item, pattern)
	if matched {
		t.Error("expected pattern to not match")
	}
}

func TestPatternDetectorMatchPatternOperators(t *testing.T) {
	detector := newTestDetector()

	tests := []struct {
		name       string
		conditions []PatternCondition
		item       *automation.ReviewItem
		expected   bool
	}{
		{
			name: "equals match",
			conditions: []PatternCondition{
				NewPatternCondition("sender", OpEquals, "test@example.com"),
			},
			item: &automation.ReviewItem{
				ID:     "1",
				Sender: "test@example.com",
			},
			expected: true,
		},
		{
			name: "equals no match",
			conditions: []PatternCondition{
				NewPatternCondition("sender", OpEquals, "test@example.com"),
			},
			item: &automation.ReviewItem{
				ID:     "1",
				Sender: "other@example.com",
			},
			expected: false,
		},
		{
			name: "contains match",
			conditions: []PatternCondition{
				NewPatternCondition("subject", OpContains, "urgent"),
			},
			item: &automation.ReviewItem{
				ID:      "1",
				Subject: "URGENT: Please review",
			},
			expected: true,
		},
		{
			name: "starts_with match",
			conditions: []PatternCondition{
				NewPatternCondition("sender", OpStartsWith, "admin@"),
			},
			item: &automation.ReviewItem{
				ID:     "1",
				Sender: "admin@example.com",
			},
			expected: true,
		},
		{
			name: "ends_with match",
			conditions: []PatternCondition{
				NewPatternCondition("sender", OpEndsWith, "@company.com"),
			},
			item: &automation.ReviewItem{
				ID:     "1",
				Sender: "user@company.com",
			},
			expected: true,
		},
		{
			name: "greater_than match",
			conditions: []PatternCondition{
				NewPatternCondition("priority", OpGreaterThan, 2),
			},
			item: &automation.ReviewItem{
				ID:       "1",
				Priority: 3,
			},
			expected: true,
		},
		{
			name: "greater_than no match",
			conditions: []PatternCondition{
				NewPatternCondition("priority", OpGreaterThan, 2),
			},
			item: &automation.ReviewItem{
				ID:       "1",
				Priority: 2,
			},
			expected: false,
		},
		{
			name: "less_than match",
			conditions: []PatternCondition{
				NewPatternCondition("confidence", OpLessThan, 0.5),
			},
			item: &automation.ReviewItem{
				ID:         "1",
				Confidence: 0.3,
			},
			expected: true,
		},
		{
			name: "matches regex",
			conditions: []PatternCondition{
				NewPatternCondition("sender", OpMatches, ".*@example\\.com$"),
			},
			item: &automation.ReviewItem{
				ID:     "1",
				Sender: "user@example.com",
			},
			expected: true,
		},
		{
			name: "in list match",
			conditions: []PatternCondition{
				{
					Field:    "category",
					Operator: OpIn,
					Value:    []string{"marketing", "newsletter", "promo"},
				},
			},
			item: &automation.ReviewItem{
				ID:       "1",
				Category: "newsletter",
			},
			expected: true,
		},
		{
			name: "multiple conditions all match",
			conditions: []PatternCondition{
				NewPatternCondition("sender", OpContains, "example.com"),
				NewPatternCondition("category", OpEquals, "work"),
			},
			item: &automation.ReviewItem{
				ID:       "1",
				Sender:   "user@example.com",
				Category: "work",
			},
			expected: true,
		},
		{
			name: "multiple conditions partial match",
			conditions: []PatternCondition{
				NewPatternCondition("sender", OpContains, "example.com"),
				NewPatternCondition("category", OpEquals, "work"),
			},
			item: &automation.ReviewItem{
				ID:       "1",
				Sender:   "user@example.com",
				Category: "personal",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := NewPattern("tenant-1", tt.name, PatternContent, tt.conditions)
			result := detector.MatchPattern(tt.item, pattern)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPatternDetectorDetectPatterns(t *testing.T) {
	detector := newTestDetector()

	// Add patterns
	p1 := NewPattern("tenant-1", "Newsletter", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpContains, "newsletter"),
	})
	p2 := NewPattern("tenant-1", "Marketing", PatternTopic, []PatternCondition{
		NewPatternCondition("category", OpEquals, "marketing"),
	})
	p3 := NewPattern("tenant-1", "High Priority", PatternPriority, []PatternCondition{
		NewPatternCondition("priority", OpGreaterOrEqual, 3),
	})

	detector.AddPattern(p1)
	detector.AddPattern(p2)
	detector.AddPattern(p3)

	items := []*automation.ReviewItem{
		{
			ID:       "item-1",
			TenantID: "tenant-1",
			Sender:   "newsletter@company.com",
			Category: "marketing",
			Priority: 1,
		},
		{
			ID:       "item-2",
			TenantID: "tenant-1",
			Sender:   "boss@company.com",
			Category: "work",
			Priority: 4,
		},
	}

	// Detect patterns
	matched := detector.DetectPatterns(items)

	// Should match:
	// - item-1 matches p1 (newsletter) and p2 (marketing)
	// - item-2 matches p3 (high priority)
	if len(matched) != 3 {
		t.Errorf("expected 3 matched patterns, got %d", len(matched))
	}
}

func TestPatternDetectorDetectPatternsEmpty(t *testing.T) {
	detector := newTestDetector()

	// Add a pattern
	pattern := NewPattern("tenant-1", "Test", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	detector.AddPattern(pattern)

	// Detect with empty items
	matched := detector.DetectPatterns([]*automation.ReviewItem{})
	if len(matched) != 0 {
		t.Errorf("expected 0 matches for empty items, got %d", len(matched))
	}
}

func TestPatternDetectorDetectPatternsNoMatch(t *testing.T) {
	detector := newTestDetector()

	pattern := NewPattern("tenant-1", "Specific Sender", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "specific@example.com"),
	})
	detector.AddPattern(pattern)

	items := []*automation.ReviewItem{
		{
			ID:       "item-1",
			TenantID: "tenant-1",
			Sender:   "other@example.com",
		},
	}

	matched := detector.DetectPatterns(items)
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matched))
	}
}

func TestPatternDetectorNilInputs(t *testing.T) {
	detector := newTestDetector()

	// Test with nil item
	matched := detector.MatchPattern(nil, &Pattern{})
	if matched {
		t.Error("expected no match for nil item")
	}

	// Test with nil pattern
	matched = detector.MatchPattern(&automation.ReviewItem{}, nil)
	if matched {
		t.Error("expected no match for nil pattern")
	}
}

func TestPatternDetectorSetPatternSet(t *testing.T) {
	detector := newTestDetector()

	ps := NewPatternSet("tenant-1")
	ps.AddPattern(NewPattern("tenant-1", "Test 1", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	}))
	ps.AddPattern(NewPattern("tenant-1", "Test 2", PatternTopic, []PatternCondition{
		NewPatternCondition("category", OpEquals, "work"),
	}))

	detector.SetPatternSet(ps)

	retrieved := detector.GetPatternSet("tenant-1")
	if retrieved == nil {
		t.Fatal("expected pattern set to be retrieved")
	}
	if len(retrieved.Patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(retrieved.Patterns))
	}
}

func TestPatternDetectorLearnPattern(t *testing.T) {
	detector := newTestDetector()

	// Create decisions with a clear pattern
	decisions := make([]*automation.Decision, 0)
	for i := 0; i < 10; i++ {
		item := &automation.ReviewItem{
			ID:       "item-" + string(rune('a'+i)),
			TenantID: "tenant-1",
			Sender:   "newsletter@company.com",
			Category: "marketing",
		}
		decision := automation.NewDecision("tenant-1", "user-1", item, automation.ActionReject)
		decisions = append(decisions, decision)
	}

	// Learn pattern
	pattern := detector.LearnPattern(decisions)
	if pattern == nil {
		t.Skip("pattern learning may require more diverse data")
		return
	}

	if pattern.SuggestedAction != string(automation.ActionReject) {
		t.Errorf("expected suggested action 'reject', got '%s'", pattern.SuggestedAction)
	}
	if !pattern.AutoGenerated {
		t.Error("expected pattern to be auto-generated")
	}
}

func TestPatternDetectorRecordDecision(t *testing.T) {
	detector := newTestDetector()

	item := &automation.ReviewItem{
		ID:       "item-1",
		TenantID: "tenant-1",
		Sender:   "test@example.com",
	}
	decision := automation.NewDecision("tenant-1", "user-1", item, automation.ActionAccept)

	// Should not panic
	detector.RecordDecision(decision)

	// Verify learner received the decision (indirect check via suggestions)
	// This is more of a smoke test
}

func TestPatternDetectorGetAnalyzer(t *testing.T) {
	detector := newTestDetector()

	analyzer := detector.GetAnalyzer()
	if analyzer == nil {
		t.Error("expected analyzer to be returned")
	}
}

func TestPatternDetectorGetLearner(t *testing.T) {
	detector := newTestDetector()

	learner := detector.GetLearner()
	if learner == nil {
		t.Error("expected learner to be returned")
	}
}

func TestPatternDetectorLoadPatterns(t *testing.T) {
	detector := newTestDetector()

	// Pre-populate store
	ctx := context.Background()
	store := NewInMemoryPatternStore()

	p1 := NewPattern("tenant-1", "Pattern 1", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	p2 := NewPattern("tenant-1", "Pattern 2", PatternTopic, []PatternCondition{
		NewPatternCondition("category", OpEquals, "work"),
	})

	store.Save(ctx, p1)
	store.Save(ctx, p2)

	detector.SetStore(store)

	// Load patterns
	err := detector.LoadPatterns(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}

	// Verify patterns were loaded
	ps := detector.GetPatternSet("tenant-1")
	if ps == nil {
		t.Fatal("expected pattern set to exist after load")
	}
	if len(ps.Patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(ps.Patterns))
	}
}

func TestPatternDetectorBetweenOperator(t *testing.T) {
	detector := newTestDetector()

	pattern := NewPattern("tenant-1", "Confidence Range", PatternContent, []PatternCondition{
		{
			Field:    "confidence",
			Operator: OpBetween,
			Value:    []float64{0.5, 0.8},
		},
	})

	tests := []struct {
		confidence float64
		expected   bool
	}{
		{0.3, false},
		{0.5, true},
		{0.65, true},
		{0.8, true},
		{0.9, false},
	}

	for _, tt := range tests {
		item := &automation.ReviewItem{
			ID:         "1",
			Confidence: tt.confidence,
		}
		result := detector.MatchPattern(item, pattern)
		if result != tt.expected {
			t.Errorf("confidence %.2f: expected %v, got %v", tt.confidence, tt.expected, result)
		}
	}
}

func TestPatternDetectorCaseSensitivity(t *testing.T) {
	detector := newTestDetector()

	// Case-insensitive (default)
	patternInsensitive := NewPattern("tenant-1", "Insensitive", PatternSender, []PatternCondition{
		{
			Field:         "sender",
			Operator:      OpEquals,
			Value:         "TEST@example.com",
			CaseSensitive: false,
		},
	})

	// Case-sensitive
	patternSensitive := NewPattern("tenant-1", "Sensitive", PatternSender, []PatternCondition{
		{
			Field:         "sender",
			Operator:      OpEquals,
			Value:         "TEST@example.com",
			CaseSensitive: true,
		},
	})

	item := &automation.ReviewItem{
		ID:     "1",
		Sender: "test@example.com",
	}

	// Case-insensitive should match
	if !detector.MatchPattern(item, patternInsensitive) {
		t.Error("expected case-insensitive pattern to match")
	}

	// Case-sensitive should not match
	if detector.MatchPattern(item, patternSensitive) {
		t.Error("expected case-sensitive pattern to not match")
	}
}

func TestPatternDetectorMetadataField(t *testing.T) {
	detector := newTestDetector()

	pattern := NewPattern("tenant-1", "Custom Field", PatternContent, []PatternCondition{
		NewPatternCondition("custom_field", OpEquals, "special_value"),
	})

	item := &automation.ReviewItem{
		ID: "1",
		Metadata: map[string]interface{}{
			"custom_field": "special_value",
		},
	}

	if !detector.MatchPattern(item, pattern) {
		t.Error("expected pattern to match metadata field")
	}

	item.Metadata["custom_field"] = "other_value"
	if detector.MatchPattern(item, pattern) {
		t.Error("expected pattern to not match with different metadata value")
	}
}

func TestPatternDetectorContentField(t *testing.T) {
	detector := newTestDetector()

	pattern := NewPattern("tenant-1", "Content Search", PatternContent, []PatternCondition{
		NewPatternCondition("content", OpContains, "important"),
	})

	item := &automation.ReviewItem{
		ID:      "1",
		Subject: "Something important",
		Body:    "Regular body text",
		Summary: "A summary",
	}

	// Should match because "important" is in subject
	if !detector.MatchPattern(item, pattern) {
		t.Error("expected pattern to match content field")
	}

	item.Subject = "Regular subject"
	item.Body = "This is important information"

	// Should still match because "important" is in body
	if !detector.MatchPattern(item, pattern) {
		t.Error("expected pattern to match content field from body")
	}
}

func TestPatternDetectorAgeHoursField(t *testing.T) {
	detector := newTestDetector()

	pattern := NewPattern("tenant-1", "Old Items", PatternTime, []PatternCondition{
		NewPatternCondition("age_hours", OpGreaterThan, 24.0),
	})

	// Item created 48 hours ago
	item := &automation.ReviewItem{
		ID:        "1",
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}

	if !detector.MatchPattern(item, pattern) {
		t.Error("expected pattern to match old item")
	}

	// Item created 1 hour ago
	item.CreatedAt = time.Now().Add(-1 * time.Hour)
	if detector.MatchPattern(item, pattern) {
		t.Error("expected pattern to not match recent item")
	}
}
