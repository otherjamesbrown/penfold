// Package automation provides condition matching for automation rules.
package automation

import (
	"regexp"
	"strings"
	"time"
)

// ReviewItem represents an item being evaluated by the automation engine.
// This is the input to rule evaluation and condition matching.
type ReviewItem struct {
	// ID is the unique identifier for the review item.
	ID string `json:"id"`

	// TenantID is the tenant this item belongs to.
	TenantID string `json:"tenant_id"`

	// ContentID references the underlying content entity.
	ContentID string `json:"content_id"`

	// ContentType is the type of content (email, document, relationship, etc.).
	ContentType string `json:"content_type"`

	// Category groups items for organization.
	Category string `json:"category"`

	// Source identifies where the item came from.
	Source string `json:"source"`

	// Sender is the sender identity (email address, user ID, etc.).
	Sender string `json:"sender"`

	// Subject is the subject or title of the content.
	Subject string `json:"subject"`

	// Body is the main content body.
	Body string `json:"body"`

	// Summary is an AI-generated summary.
	Summary string `json:"summary"`

	// Priority is the priority level (0=unspecified, 1=low, 2=medium, 3=high, 4=urgent).
	Priority int `json:"priority"`

	// Confidence is the AI confidence score (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// Keywords are extracted keywords from the content.
	Keywords []string `json:"keywords"`

	// Tags are user or system-applied tags.
	Tags []string `json:"tags"`

	// CreatedAt is when the item was created.
	CreatedAt time.Time `json:"created_at"`

	// Metadata holds additional key-value pairs.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Age returns the duration since the item was created.
func (r *ReviewItem) Age() time.Duration {
	return time.Since(r.CreatedAt)
}

// HasTag returns true if the item has the given tag.
func (r *ReviewItem) HasTag(tag string) bool {
	for _, t := range r.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// HasKeyword returns true if the item has the given keyword.
func (r *ReviewItem) HasKeyword(keyword string) bool {
	for _, k := range r.Keywords {
		if strings.EqualFold(k, keyword) {
			return true
		}
	}
	return false
}

// ConditionMatcher evaluates conditions against review items.
type ConditionMatcher struct {
	// compiledPatterns caches compiled regex patterns.
	compiledPatterns map[string]*regexp.Regexp
}

// NewConditionMatcher creates a new ConditionMatcher.
func NewConditionMatcher() *ConditionMatcher {
	return &ConditionMatcher{
		compiledPatterns: make(map[string]*regexp.Regexp),
	}
}

// Match evaluates if the condition matches the review item.
func (m *ConditionMatcher) Match(item *ReviewItem, condition Condition) bool {
	if item == nil {
		return false
	}

	switch condition.Type {
	case ConditionPattern:
		return m.matchPattern(item, condition)
	case ConditionSender:
		return m.matchSender(item, condition)
	case ConditionCategory:
		return m.matchCategory(item, condition)
	case ConditionKeywords:
		return m.matchKeywords(item, condition)
	case ConditionConfidence:
		return m.matchConfidence(item, condition)
	case ConditionSource:
		return m.matchSource(item, condition)
	case ConditionContentType:
		return m.matchContentType(item, condition)
	case ConditionPriority:
		return m.matchPriority(item, condition)
	case ConditionAge:
		return m.matchAge(item, condition)
	case ConditionAnd:
		return m.matchAnd(item, condition)
	case ConditionOr:
		return m.matchOr(item, condition)
	case ConditionNot:
		return m.matchNot(item, condition)
	case ConditionThreshold:
		return m.matchThreshold(item, condition)
	default:
		return false
	}
}

// matchPattern matches content against a regex pattern.
func (m *ConditionMatcher) matchPattern(item *ReviewItem, condition Condition) bool {
	if condition.Pattern == "" {
		return false
	}

	// Build the pattern key for caching (include case sensitivity)
	patternKey := condition.Pattern
	if !condition.CaseSensitive {
		patternKey = "(?i)" + condition.Pattern
	}

	// Get or compile the regex
	re, ok := m.compiledPatterns[patternKey]
	if !ok {
		var err error
		re, err = regexp.Compile(patternKey)
		if err != nil {
			return false
		}
		m.compiledPatterns[patternKey] = re
	}

	// Determine which field to match
	var target string
	switch condition.Field {
	case "subject":
		target = item.Subject
	case "body":
		target = item.Body
	case "summary":
		target = item.Summary
	case "sender":
		target = item.Sender
	default:
		// Match against combined content
		target = item.Subject + " " + item.Body + " " + item.Summary
	}

	return re.MatchString(target)
}

// matchSender matches based on sender identity.
func (m *ConditionMatcher) matchSender(item *ReviewItem, condition Condition) bool {
	value, ok := condition.Value.(string)
	if !ok {
		return false
	}

	sender := item.Sender
	if !condition.CaseSensitive {
		sender = strings.ToLower(sender)
		value = strings.ToLower(value)
	}

	switch condition.Operator {
	case "eq", "equals", "":
		return sender == value
	case "neq", "not_equals":
		return sender != value
	case "contains":
		return strings.Contains(sender, value)
	case "starts_with":
		return strings.HasPrefix(sender, value)
	case "ends_with":
		return strings.HasSuffix(sender, value)
	case "matches":
		re, err := regexp.Compile(value)
		if err != nil {
			return false
		}
		return re.MatchString(sender)
	default:
		return sender == value
	}
}

// matchCategory matches based on item category.
func (m *ConditionMatcher) matchCategory(item *ReviewItem, condition Condition) bool {
	value, ok := condition.Value.(string)
	if !ok {
		return false
	}

	category := item.Category
	if !condition.CaseSensitive {
		category = strings.ToLower(category)
		value = strings.ToLower(value)
	}

	switch condition.Operator {
	case "eq", "equals", "":
		return category == value
	case "neq", "not_equals":
		return category != value
	case "contains":
		return strings.Contains(category, value)
	default:
		return category == value
	}
}

// matchKeywords matches based on presence of keywords.
func (m *ConditionMatcher) matchKeywords(item *ReviewItem, condition Condition) bool {
	if len(condition.Keywords) == 0 {
		return false
	}

	// Build a combined text to search
	searchText := strings.ToLower(item.Subject + " " + item.Body + " " + item.Summary)

	// Determine match mode from operator
	matchAll := condition.Operator == "all" || condition.Operator == "and"

	matchedCount := 0
	for _, keyword := range condition.Keywords {
		kw := keyword
		if !condition.CaseSensitive {
			kw = strings.ToLower(keyword)
		}

		if strings.Contains(searchText, kw) || item.HasKeyword(keyword) {
			matchedCount++
			if !matchAll {
				// Any match is sufficient
				return true
			}
		}
	}

	if matchAll {
		return matchedCount == len(condition.Keywords)
	}
	return matchedCount > 0
}

// matchConfidence matches based on AI confidence score.
func (m *ConditionMatcher) matchConfidence(item *ReviewItem, condition Condition) bool {
	confidence := item.Confidence

	// Check range if both min and max are set
	if condition.MinConfidence > 0 && condition.MaxConfidence > 0 {
		return confidence >= condition.MinConfidence && confidence <= condition.MaxConfidence
	}

	// Check minimum
	if condition.MinConfidence > 0 {
		return confidence >= condition.MinConfidence
	}

	// Check maximum
	if condition.MaxConfidence > 0 {
		return confidence <= condition.MaxConfidence
	}

	// Check threshold
	if condition.Threshold > 0 {
		switch condition.Operator {
		case "gte", ">=", "":
			return confidence >= condition.Threshold
		case "gt", ">":
			return confidence > condition.Threshold
		case "lte", "<=":
			return confidence <= condition.Threshold
		case "lt", "<":
			return confidence < condition.Threshold
		case "eq", "==":
			return confidence == condition.Threshold
		default:
			return confidence >= condition.Threshold
		}
	}

	return false
}

// matchSource matches based on item source.
func (m *ConditionMatcher) matchSource(item *ReviewItem, condition Condition) bool {
	value, ok := condition.Value.(string)
	if !ok {
		return false
	}

	source := item.Source
	if !condition.CaseSensitive {
		source = strings.ToLower(source)
		value = strings.ToLower(value)
	}

	switch condition.Operator {
	case "eq", "equals", "":
		return source == value
	case "neq", "not_equals":
		return source != value
	case "in":
		// Value should be comma-separated list
		values := strings.Split(value, ",")
		for _, v := range values {
			if strings.TrimSpace(v) == source {
				return true
			}
		}
		return false
	default:
		return source == value
	}
}

// matchContentType matches based on content type.
func (m *ConditionMatcher) matchContentType(item *ReviewItem, condition Condition) bool {
	value, ok := condition.Value.(string)
	if !ok {
		return false
	}

	contentType := item.ContentType
	if !condition.CaseSensitive {
		contentType = strings.ToLower(contentType)
		value = strings.ToLower(value)
	}

	switch condition.Operator {
	case "eq", "equals", "":
		return contentType == value
	case "neq", "not_equals":
		return contentType != value
	case "in":
		values := strings.Split(value, ",")
		for _, v := range values {
			if strings.TrimSpace(v) == contentType {
				return true
			}
		}
		return false
	default:
		return contentType == value
	}
}

// matchPriority matches based on item priority level.
func (m *ConditionMatcher) matchPriority(item *ReviewItem, condition Condition) bool {
	var value int

	switch v := condition.Value.(type) {
	case int:
		value = v
	case int64:
		value = int(v)
	case float64:
		value = int(v)
	default:
		return false
	}

	priority := item.Priority

	switch condition.Operator {
	case "eq", "equals", "==", "":
		return priority == value
	case "neq", "not_equals", "!=":
		return priority != value
	case "gt", ">":
		return priority > value
	case "gte", ">=":
		return priority >= value
	case "lt", "<":
		return priority < value
	case "lte", "<=":
		return priority <= value
	default:
		return priority == value
	}
}

// matchAge matches based on item age.
func (m *ConditionMatcher) matchAge(item *ReviewItem, condition Condition) bool {
	age := item.Age()

	// Threshold is expected to be in hours
	thresholdHours := condition.Threshold
	if thresholdHours == 0 {
		if v, ok := condition.Value.(float64); ok {
			thresholdHours = v
		}
	}

	thresholdDuration := time.Duration(thresholdHours * float64(time.Hour))

	switch condition.Operator {
	case "gt", ">":
		return age > thresholdDuration
	case "gte", ">=", "":
		return age >= thresholdDuration
	case "lt", "<":
		return age < thresholdDuration
	case "lte", "<=":
		return age <= thresholdDuration
	default:
		return age >= thresholdDuration
	}
}

// matchAnd combines multiple conditions with AND logic.
func (m *ConditionMatcher) matchAnd(item *ReviewItem, condition Condition) bool {
	if len(condition.SubConditions) == 0 {
		return true
	}

	for _, sub := range condition.SubConditions {
		if !m.Match(item, sub) {
			return false
		}
	}
	return true
}

// matchOr combines multiple conditions with OR logic.
func (m *ConditionMatcher) matchOr(item *ReviewItem, condition Condition) bool {
	if len(condition.SubConditions) == 0 {
		return false
	}

	for _, sub := range condition.SubConditions {
		if m.Match(item, sub) {
			return true
		}
	}
	return false
}

// matchNot negates a condition.
func (m *ConditionMatcher) matchNot(item *ReviewItem, condition Condition) bool {
	if len(condition.SubConditions) == 0 {
		return true
	}

	// Negate the first sub-condition
	return !m.Match(item, condition.SubConditions[0])
}

// matchThreshold matches based on numeric thresholds.
func (m *ConditionMatcher) matchThreshold(item *ReviewItem, condition Condition) bool {
	// Get the field value
	var fieldValue float64

	switch condition.Field {
	case "confidence":
		fieldValue = item.Confidence
	case "priority":
		fieldValue = float64(item.Priority)
	case "age_hours":
		fieldValue = item.Age().Hours()
	case "keyword_count":
		fieldValue = float64(len(item.Keywords))
	case "tag_count":
		fieldValue = float64(len(item.Tags))
	default:
		// Try to get from metadata
		if item.Metadata != nil {
			if v, ok := item.Metadata[condition.Field]; ok {
				switch val := v.(type) {
				case float64:
					fieldValue = val
				case int:
					fieldValue = float64(val)
				case int64:
					fieldValue = float64(val)
				default:
					return false
				}
			}
		}
	}

	threshold := condition.Threshold

	switch condition.Operator {
	case "eq", "==":
		return fieldValue == threshold
	case "neq", "!=":
		return fieldValue != threshold
	case "gt", ">":
		return fieldValue > threshold
	case "gte", ">=", "":
		return fieldValue >= threshold
	case "lt", "<":
		return fieldValue < threshold
	case "lte", "<=":
		return fieldValue <= threshold
	default:
		return fieldValue >= threshold
	}
}

// MatchResult contains the result of condition matching.
type MatchResult struct {
	// Matched indicates if the condition was matched.
	Matched bool `json:"matched"`

	// Rule is the rule that was evaluated.
	Rule *Rule `json:"rule,omitempty"`

	// Item is the item that was evaluated.
	Item *ReviewItem `json:"item,omitempty"`

	// EvaluationTimeMs is how long evaluation took in milliseconds.
	EvaluationTimeMs int64 `json:"evaluation_time_ms"`

	// Details contains additional matching details.
	Details map[string]interface{} `json:"details,omitempty"`
}

// AndCondition creates an AND condition from multiple conditions.
func AndCondition(conditions ...Condition) Condition {
	return Condition{
		Type:          ConditionAnd,
		SubConditions: conditions,
	}
}

// OrCondition creates an OR condition from multiple conditions.
func OrCondition(conditions ...Condition) Condition {
	return Condition{
		Type:          ConditionOr,
		SubConditions: conditions,
	}
}

// NotCondition creates a NOT condition that negates the given condition.
func NotCondition(condition Condition) Condition {
	return Condition{
		Type:          ConditionNot,
		SubConditions: []Condition{condition},
	}
}

// PatternCondition creates a pattern matching condition.
func PatternCondition(pattern string, field string, caseSensitive bool) Condition {
	return Condition{
		Type:          ConditionPattern,
		Pattern:       pattern,
		Field:         field,
		CaseSensitive: caseSensitive,
	}
}

// SenderCondition creates a sender matching condition.
func SenderCondition(operator, value string) Condition {
	return Condition{
		Type:     ConditionSender,
		Operator: operator,
		Value:    value,
	}
}

// CategoryCondition creates a category matching condition.
func CategoryCondition(operator, value string) Condition {
	return Condition{
		Type:     ConditionCategory,
		Operator: operator,
		Value:    value,
	}
}

// KeywordsCondition creates a keywords matching condition.
func KeywordsCondition(keywords []string, matchAll bool) Condition {
	operator := "any"
	if matchAll {
		operator = "all"
	}
	return Condition{
		Type:     ConditionKeywords,
		Keywords: keywords,
		Operator: operator,
	}
}

// ConfidenceCondition creates a confidence threshold condition.
func ConfidenceCondition(minConfidence, maxConfidence float64) Condition {
	return Condition{
		Type:          ConditionConfidence,
		MinConfidence: minConfidence,
		MaxConfidence: maxConfidence,
	}
}

// ThresholdCondition creates a numeric threshold condition.
func ThresholdCondition(field string, operator string, threshold float64) Condition {
	return Condition{
		Type:      ConditionThreshold,
		Field:     field,
		Operator:  operator,
		Threshold: threshold,
	}
}

// ContentTypeCondition creates a content type matching condition.
func ContentTypeCondition(contentType string) Condition {
	return Condition{
		Type:     ConditionContentType,
		Operator: "eq",
		Value:    contentType,
	}
}

// SourceCondition creates a source matching condition.
func SourceCondition(source string) Condition {
	return Condition{
		Type:     ConditionSource,
		Operator: "eq",
		Value:    source,
	}
}

// PriorityCondition creates a priority matching condition.
func PriorityCondition(operator string, priority int) Condition {
	return Condition{
		Type:     ConditionPriority,
		Operator: operator,
		Value:    priority,
	}
}

// AgeCondition creates an age-based condition.
func AgeCondition(operator string, hours float64) Condition {
	return Condition{
		Type:      ConditionAge,
		Operator:  operator,
		Threshold: hours,
	}
}
