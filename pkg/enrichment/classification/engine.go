package classification

import (
	"sort"
	"strings"
)

// ClassificationRule represents a rule loaded from the database.
type ClassificationRule struct {
	ID                 int
	TenantID           string
	Name               string
	Priority           int
	ContentTypeScope   string // "" means any content type
	ContentType        string
	ContentSubtype     string
	NotificationSource string
	Active             bool
	Conditions         []MatchCondition
}

// MatchCondition represents a single match condition for a rule.
type MatchCondition struct {
	ID            int
	Field         string // "from_address", "subject", "message_id", "header:Content-Type", etc.
	MatchType     string // "contains", "prefix", "suffix", "glob", "exact"
	Value         string
	CaseSensitive bool
}

// ClassificationResult is the output of the rule engine.
type ClassificationResult struct {
	ContentType        string
	ContentSubtype     string
	NotificationSource string
	RuleName           string // which rule matched
	RulePriority       int
}

// Engine evaluates classification rules against ingestion metadata.
type Engine struct {
	rules []ClassificationRule // cached rules per tenant
}

// NewEngine creates a new classification engine from loaded rules.
func NewEngine(rules []ClassificationRule) *Engine {
	return &Engine{rules: rules}
}

// Classify evaluates rules against metadata and returns the classification result.
// contentType is the item's content type (e.g., "EMAIL", "CALENDAR") for scope filtering.
// metadata is a flat key-value map of ingestion metadata.
func (e *Engine) Classify(contentType string, metadata map[string]string) ClassificationResult {
	// Filter rules: only active rules where ContentTypeScope is "" (null/any) or matches contentType
	var applicable []ClassificationRule
	for _, rule := range e.rules {
		if !rule.Active {
			continue
		}
		if rule.ContentTypeScope == "" || strings.EqualFold(rule.ContentTypeScope, contentType) {
			applicable = append(applicable, rule)
		}
	}

	// Sort by Priority ascending (lower = higher priority)
	sort.Slice(applicable, func(i, j int) bool {
		return applicable[i].Priority < applicable[j].Priority
	})

	// For each rule, check conditions with OR logic: if ANY condition matches → rule fires
	for _, rule := range applicable {
		for _, cond := range rule.Conditions {
			fieldValue := resolveField(cond.Field, metadata)
			if matchCondition(cond, fieldValue) {
				return ClassificationResult{
					ContentType:        rule.ContentType,
					ContentSubtype:     rule.ContentSubtype,
					NotificationSource: rule.NotificationSource,
					RuleName:           rule.Name,
					RulePriority:       rule.Priority,
				}
			}
		}
	}

	// No match → default: EMAIL/HUMAN
	return ClassificationResult{
		ContentType:    "EMAIL",
		ContentSubtype: "HUMAN",
	}
}

// resolveField extracts the field value from the metadata map.
// Supports simple fields ("from_address", "subject") and header fields ("header:Content-Type").
func resolveField(field string, metadata map[string]string) string {
	return metadata[field]
}

// matchCondition checks whether a field value satisfies a match condition.
func matchCondition(cond MatchCondition, fieldValue string) bool {
	v := fieldValue
	condValue := cond.Value

	if !cond.CaseSensitive {
		v = strings.ToLower(v)
		condValue = strings.ToLower(condValue)
	}

	switch cond.MatchType {
	case "contains":
		return strings.Contains(v, condValue)
	case "prefix":
		return strings.HasPrefix(v, condValue)
	case "suffix":
		return strings.HasSuffix(v, condValue)
	case "glob":
		return matchesPatternEngine(v, condValue)
	case "exact":
		return v == condValue
	case "exists":
		return fieldValue != ""
	default:
		return false
	}
}

// matchesPatternEngine checks if a string matches a wildcard pattern.
// Supports patterns like "*@domain.com", "prefix@*", "*@*.domain.com".
// Both s and pattern should already be lowercased if case-insensitive matching is desired.
func matchesPatternEngine(s, pattern string) bool {
	// No wildcard - exact match
	if !strings.Contains(pattern, "*") {
		return s == pattern
	}

	// Split pattern by wildcard
	parts := strings.Split(pattern, "*")

	// Pattern like "*@*.domain.com" has 3 parts: ["", "@", ".domain.com"]
	// We need to check that s contains each non-empty part in order
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}

		// For the first non-empty part, it must be a prefix
		if i == 0 {
			if !strings.HasPrefix(s, part) {
				return false
			}
			pos = len(part)
			continue
		}

		// For the last non-empty part, it must be a suffix
		if i == len(parts)-1 {
			if !strings.HasSuffix(s, part) {
				return false
			}
			continue
		}

		// For middle parts, must contain the part after the current position
		idx := strings.Index(s[pos:], part)
		if idx == -1 {
			return false
		}
		pos += idx + len(part)
	}

	return true
}
