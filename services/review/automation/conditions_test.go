package automation

import (
	"testing"
	"time"
)

func TestConditionMatcherPattern(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:      "item-1",
		Subject: "Important: Project Update",
		Body:    "This is the email body",
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "pattern matches subject",
			condition: PatternCondition("Important", "subject", false),
			expected:  true,
		},
		{
			name:      "pattern does not match",
			condition: PatternCondition("Urgent", "subject", false),
			expected:  false,
		},
		{
			name:      "pattern matches combined content",
			condition: PatternCondition("email body", "", false),
			expected:  true,
		},
		{
			name:      "case insensitive match",
			condition: PatternCondition("important", "subject", false),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherSender(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:     "item-1",
		Sender: "john.doe@example.com",
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "exact match",
			condition: SenderCondition("eq", "john.doe@example.com"),
			expected:  true,
		},
		{
			name:      "not equal",
			condition: SenderCondition("neq", "jane@example.com"),
			expected:  true,
		},
		{
			name:      "contains domain",
			condition: SenderCondition("contains", "example.com"),
			expected:  true,
		},
		{
			name:      "starts with",
			condition: SenderCondition("starts_with", "john"),
			expected:  true,
		},
		{
			name:      "ends with",
			condition: SenderCondition("ends_with", "@example.com"),
			expected:  true,
		},
		{
			name:      "regex match",
			condition: SenderCondition("matches", `^[a-z]+\.[a-z]+@`),
			expected:  true,
		},
		{
			name:      "no match",
			condition: SenderCondition("eq", "other@example.com"),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherCategory(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:       "item-1",
		Category: "communications",
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "exact match",
			condition: CategoryCondition("eq", "communications"),
			expected:  true,
		},
		{
			name:      "not equal",
			condition: CategoryCondition("neq", "tasks"),
			expected:  true,
		},
		{
			name:      "contains",
			condition: CategoryCondition("contains", "comm"),
			expected:  true,
		},
		{
			name:      "no match",
			condition: CategoryCondition("eq", "tasks"),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherKeywords(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:       "item-1",
		Subject:  "Important meeting tomorrow",
		Body:     "Please review the agenda",
		Keywords: []string{"meeting", "agenda"},
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "any keyword match",
			condition: KeywordsCondition([]string{"meeting", "lunch"}, false),
			expected:  true,
		},
		{
			name:      "all keywords match",
			condition: KeywordsCondition([]string{"meeting", "agenda"}, true),
			expected:  true,
		},
		{
			name:      "all keywords not present",
			condition: KeywordsCondition([]string{"meeting", "dinner"}, true),
			expected:  false,
		},
		{
			name:      "no keywords match",
			condition: KeywordsCondition([]string{"vacation", "holiday"}, false),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherConfidence(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:         "item-1",
		Confidence: 0.85,
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "above minimum",
			condition: ConfidenceCondition(0.8, 0),
			expected:  true,
		},
		{
			name:      "below minimum",
			condition: ConfidenceCondition(0.9, 0),
			expected:  false,
		},
		{
			name:      "within range",
			condition: ConfidenceCondition(0.8, 0.9),
			expected:  true,
		},
		{
			name:      "below range",
			condition: ConfidenceCondition(0.9, 1.0),
			expected:  false,
		},
		{
			name: "threshold gte",
			condition: Condition{
				Type:      ConditionConfidence,
				Threshold: 0.8,
				Operator:  "gte",
			},
			expected: true,
		},
		{
			name: "threshold lt",
			condition: Condition{
				Type:      ConditionConfidence,
				Threshold: 0.9,
				Operator:  "lt",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherPriority(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:       "item-1",
		Priority: 3, // High
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "equal",
			condition: PriorityCondition("eq", 3),
			expected:  true,
		},
		{
			name:      "not equal",
			condition: PriorityCondition("neq", 2),
			expected:  true,
		},
		{
			name:      "greater than",
			condition: PriorityCondition("gt", 2),
			expected:  true,
		},
		{
			name:      "greater or equal",
			condition: PriorityCondition("gte", 3),
			expected:  true,
		},
		{
			name:      "less than",
			condition: PriorityCondition("lt", 4),
			expected:  true,
		},
		{
			name:      "not greater",
			condition: PriorityCondition("gt", 3),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherAge(t *testing.T) {
	matcher := NewConditionMatcher()

	// Item created 2 hours ago
	item := &ReviewItem{
		ID:        "item-1",
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "older than 1 hour",
			condition: AgeCondition("gte", 1),
			expected:  true,
		},
		{
			name:      "older than 3 hours",
			condition: AgeCondition("gte", 3),
			expected:  false,
		},
		{
			name:      "younger than 3 hours",
			condition: AgeCondition("lt", 3),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherContentType(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:          "item-1",
		ContentType: "email",
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "exact match",
			condition: ContentTypeCondition("email"),
			expected:  true,
		},
		{
			name:      "no match",
			condition: ContentTypeCondition("document"),
			expected:  false,
		},
		{
			name: "in list",
			condition: Condition{
				Type:     ConditionContentType,
				Operator: "in",
				Value:    "email,document,meeting",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherSource(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:     "item-1",
		Source: "gmail",
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "exact match",
			condition: SourceCondition("gmail"),
			expected:  true,
		},
		{
			name:      "no match",
			condition: SourceCondition("slack"),
			expected:  false,
		},
		{
			name: "in list",
			condition: Condition{
				Type:     ConditionSource,
				Operator: "in",
				Value:    "gmail,outlook,manual",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherThreshold(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:         "item-1",
		Confidence: 0.85,
		Priority:   3,
		Keywords:   []string{"a", "b", "c"},
		CreatedAt:  time.Now().Add(-2 * time.Hour),
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "confidence threshold",
			condition: ThresholdCondition("confidence", "gte", 0.8),
			expected:  true,
		},
		{
			name:      "priority threshold",
			condition: ThresholdCondition("priority", "eq", 3),
			expected:  true,
		},
		{
			name:      "keyword count",
			condition: ThresholdCondition("keyword_count", "gte", 3),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherAnd(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:       "item-1",
		Sender:   "john@example.com",
		Category: "communications",
		Priority: 3,
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name: "all conditions met",
			condition: AndCondition(
				SenderCondition("contains", "example.com"),
				CategoryCondition("eq", "communications"),
				PriorityCondition("gte", 2),
			),
			expected: true,
		},
		{
			name: "one condition not met",
			condition: AndCondition(
				SenderCondition("contains", "example.com"),
				CategoryCondition("eq", "tasks"), // Not matching
			),
			expected: false,
		},
		{
			name:      "empty and",
			condition: AndCondition(),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherOr(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:       "item-1",
		Sender:   "john@example.com",
		Category: "communications",
		Priority: 3,
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name: "one condition met",
			condition: OrCondition(
				SenderCondition("eq", "other@example.com"),
				CategoryCondition("eq", "communications"), // Matching
			),
			expected: true,
		},
		{
			name: "no conditions met",
			condition: OrCondition(
				SenderCondition("eq", "other@example.com"),
				CategoryCondition("eq", "tasks"),
			),
			expected: false,
		},
		{
			name:      "empty or",
			condition: OrCondition(),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherNot(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:     "item-1",
		Sender: "john@example.com",
	}

	tests := []struct {
		name      string
		condition Condition
		expected  bool
	}{
		{
			name:      "negate non-matching",
			condition: NotCondition(SenderCondition("eq", "other@example.com")),
			expected:  true,
		},
		{
			name:      "negate matching",
			condition: NotCondition(SenderCondition("eq", "john@example.com")),
			expected:  false,
		},
		{
			name:      "empty not",
			condition: NotCondition(Condition{}),
			expected:  true, // Negating empty returns true
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(item, tt.condition)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestConditionMatcherComplex(t *testing.T) {
	matcher := NewConditionMatcher()

	item := &ReviewItem{
		ID:          "item-1",
		Sender:      "boss@company.com",
		Category:    "communications",
		ContentType: "email",
		Priority:    4,
		Confidence:  0.95,
		Keywords:    []string{"urgent", "review", "deadline"},
	}

	// Complex condition: (sender contains company.com AND priority >= 3) OR (confidence > 0.9 AND keywords contain urgent)
	complexCondition := OrCondition(
		AndCondition(
			SenderCondition("contains", "company.com"),
			PriorityCondition("gte", 3),
		),
		AndCondition(
			ConfidenceCondition(0.9, 0),
			KeywordsCondition([]string{"urgent"}, false),
		),
	)

	result := matcher.Match(item, complexCondition)
	if !result {
		t.Error("expected complex condition to match")
	}

	// Item that doesn't match
	item2 := &ReviewItem{
		ID:          "item-2",
		Sender:      "other@external.com",
		Category:    "spam",
		ContentType: "email",
		Priority:    1,
		Confidence:  0.5,
		Keywords:    []string{"newsletter"},
	}

	result2 := matcher.Match(item2, complexCondition)
	if result2 {
		t.Error("expected complex condition to not match")
	}
}

func TestConditionMatcherNilItem(t *testing.T) {
	matcher := NewConditionMatcher()

	condition := SenderCondition("eq", "test@example.com")

	result := matcher.Match(nil, condition)
	if result {
		t.Error("expected false for nil item")
	}
}

func TestReviewItemHelpers(t *testing.T) {
	item := &ReviewItem{
		ID:        "item-1",
		Tags:      []string{"inbox", "important"},
		Keywords:  []string{"meeting", "agenda"},
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}

	// Test HasTag
	if !item.HasTag("inbox") {
		t.Error("expected HasTag to find 'inbox'")
	}
	if item.HasTag("spam") {
		t.Error("expected HasTag to not find 'spam'")
	}

	// Test HasKeyword
	if !item.HasKeyword("meeting") {
		t.Error("expected HasKeyword to find 'meeting'")
	}
	if item.HasKeyword("vacation") {
		t.Error("expected HasKeyword to not find 'vacation'")
	}

	// Test Age
	age := item.Age()
	if age < 1*time.Hour || age > 3*time.Hour {
		t.Errorf("unexpected age: %v", age)
	}
}
