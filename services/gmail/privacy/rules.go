// Package privacy provides privacy filtering and PII detection for Gmail content.
// It supports configurable rules for detecting and redacting sensitive information
// before messages are stored.
package privacy

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// PIILocation represents the position of detected PII in text.
type PIILocation struct {
	// Start is the byte offset where the PII begins.
	Start int `json:"start"`

	// End is the byte offset where the PII ends (exclusive).
	End int `json:"end"`

	// Type describes the kind of PII detected (e.g., "email", "phone", "ssn").
	Type string `json:"type"`

	// Value is the actual PII value detected (for logging/audit purposes).
	Value string `json:"value,omitempty"`

	// Confidence is the detection confidence (0.0 to 1.0).
	Confidence float64 `json:"confidence"`
}

// Rule defines the interface for privacy detection rules.
// Each rule can detect specific patterns of sensitive information.
type Rule interface {
	// Name returns a unique identifier for the rule.
	Name() string

	// Description returns a human-readable description of what the rule detects.
	Description() string

	// Detect scans the input text and returns all detected PII locations.
	Detect(text string) []PIILocation

	// Redact replaces detected PII at the given locations with a placeholder.
	// The placeholder format is configurable.
	Redact(text string, locations []PIILocation, placeholder string) string

	// Enabled returns whether the rule is currently active.
	Enabled() bool

	// SetEnabled enables or disables the rule.
	SetEnabled(enabled bool)
}

// BaseRule provides common functionality for pattern-based rules.
type BaseRule struct {
	name        string
	description string
	pattern     *regexp.Regexp
	piiType     string
	confidence  float64
	enabled     bool
}

// NewBaseRule creates a new base rule with the given parameters.
func NewBaseRule(name, description, pattern, piiType string, confidence float64) (*BaseRule, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compiling pattern for rule %s: %w", name, err)
	}

	return &BaseRule{
		name:        name,
		description: description,
		pattern:     re,
		piiType:     piiType,
		confidence:  confidence,
		enabled:     true,
	}, nil
}

// Name returns the rule's identifier.
func (r *BaseRule) Name() string {
	return r.name
}

// Description returns the rule's description.
func (r *BaseRule) Description() string {
	return r.description
}

// Detect finds all matches of the rule's pattern in the text.
func (r *BaseRule) Detect(text string) []PIILocation {
	if !r.enabled {
		return nil
	}

	matches := r.pattern.FindAllStringIndex(text, -1)
	locations := make([]PIILocation, 0, len(matches))

	for _, match := range matches {
		locations = append(locations, PIILocation{
			Start:      match[0],
			End:        match[1],
			Type:       r.piiType,
			Value:      text[match[0]:match[1]],
			Confidence: r.confidence,
		})
	}

	return locations
}

// Redact replaces PII at the specified locations with a placeholder.
func (r *BaseRule) Redact(text string, locations []PIILocation, placeholder string) string {
	if len(locations) == 0 {
		return text
	}

	// Process locations from end to start to preserve offsets.
	// First, filter and sort locations by start position in descending order.
	sortedLocs := make([]PIILocation, len(locations))
	copy(sortedLocs, locations)

	// Simple bubble sort (sufficient for small location counts).
	for i := 0; i < len(sortedLocs)-1; i++ {
		for j := 0; j < len(sortedLocs)-i-1; j++ {
			if sortedLocs[j].Start < sortedLocs[j+1].Start {
				sortedLocs[j], sortedLocs[j+1] = sortedLocs[j+1], sortedLocs[j]
			}
		}
	}

	result := text
	for _, loc := range sortedLocs {
		if loc.Start >= 0 && loc.End <= len(result) && loc.Start < loc.End {
			result = result[:loc.Start] + placeholder + result[loc.End:]
		}
	}

	return result
}

// Enabled returns whether the rule is active.
func (r *BaseRule) Enabled() bool {
	return r.enabled
}

// SetEnabled enables or disables the rule.
func (r *BaseRule) SetEnabled(enabled bool) {
	r.enabled = enabled
}

// EmailPatternRule detects email addresses in text.
type EmailPatternRule struct {
	*BaseRule
}

// NewEmailPatternRule creates a new email detection rule.
func NewEmailPatternRule() *EmailPatternRule {
	// RFC 5322 compliant email pattern (simplified but practical).
	pattern := `[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`
	base, _ := NewBaseRule(
		"email",
		"Detects email addresses",
		pattern,
		"email",
		0.95,
	)
	return &EmailPatternRule{BaseRule: base}
}

// PhonePatternRule detects phone numbers in text.
type PhonePatternRule struct {
	*BaseRule
	patterns []*regexp.Regexp
}

// NewPhonePatternRule creates a new phone number detection rule.
func NewPhonePatternRule() *PhonePatternRule {
	// Multiple patterns for different phone formats.
	patterns := []*regexp.Regexp{
		// Format: (XXX) XXX-XXXX
		regexp.MustCompile(`\(\d{3}\)\s?\d{3}[-.]?\d{4}`),
		// Format: XXX-XXX-XXXX or XXX.XXX.XXXX
		regexp.MustCompile(`\d{3}[-.]?\d{3}[-.]?\d{4}`),
		// Format: +1 XXX XXX XXXX or +1-XXX-XXX-XXXX
		regexp.MustCompile(`\+1[-.\s]?\d{3}[-.\s]?\d{3}[-.\s]?\d{4}`),
	}

	// Use a basic pattern for the base rule (not used, we override Detect).
	base, _ := NewBaseRule(
		"phone",
		"Detects US phone numbers",
		`\d{3}[-.]?\d{3}[-.]?\d{4}`,
		"phone",
		0.85,
	)
	return &PhonePatternRule{
		BaseRule: base,
		patterns: patterns,
	}
}

// Detect overrides BaseRule.Detect to handle multiple phone patterns.
func (r *PhonePatternRule) Detect(text string) []PIILocation {
	if !r.enabled {
		return nil
	}

	seen := make(map[string]bool)
	var locations []PIILocation

	for _, pattern := range r.patterns {
		matches := pattern.FindAllStringIndex(text, -1)
		for _, match := range matches {
			value := text[match[0]:match[1]]
			// Normalize to check for duplicates.
			normalized := normalizePhone(value)

			if !seen[normalized] {
				seen[normalized] = true
				locations = append(locations, PIILocation{
					Start:      match[0],
					End:        match[1],
					Type:       "phone",
					Value:      value,
					Confidence: 0.85,
				})
			}
		}
	}

	return locations
}

// normalizePhone removes formatting characters from a phone number.
func normalizePhone(phone string) string {
	var result strings.Builder
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			result.WriteRune(ch)
		}
	}
	return result.String()
}

// SSNPatternRule detects Social Security Numbers in text.
type SSNPatternRule struct {
	*BaseRule
}

// NewSSNPatternRule creates a new SSN detection rule.
func NewSSNPatternRule() *SSNPatternRule {
	// Matches SSN format: XXX-XX-XXXX with optional dashes or spaces.
	// Excludes patterns starting with 000, 666, or 9XX (invalid SSNs).
	pattern := `(?:^|[^0-9])([0-8][0-9]{2}|0[1-9][0-9]|00[1-9])[-\s]?([0-9]{2})[-\s]?([0-9]{4})(?:[^0-9]|$)`
	base, _ := NewBaseRule(
		"ssn",
		"Detects US Social Security Numbers",
		pattern,
		"ssn",
		0.90,
	)
	return &SSNPatternRule{BaseRule: base}
}

// Detect overrides BaseRule.Detect to handle SSN-specific matching.
func (r *SSNPatternRule) Detect(text string) []PIILocation {
	if !r.enabled {
		return nil
	}

	// Pattern matches any 9-digit number with optional separators.
	ssnPattern := regexp.MustCompile(`\b(\d{3})[-\s]?(\d{2})[-\s]?(\d{4})\b`)
	matches := ssnPattern.FindAllStringSubmatchIndex(text, -1)

	locations := make([]PIILocation, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			ssn := text[match[0]:match[1]]
			normalized := strings.ReplaceAll(strings.ReplaceAll(ssn, "-", ""), " ", "")
			if len(normalized) == 9 {
				// Validate SSN per SSA rules:
				// - Area number (first 3 digits) cannot be 000, 666, or 900-999
				// - Group number (middle 2 digits) cannot be 00
				// - Serial number (last 4 digits) cannot be 0000
				areaNumber := normalized[0:3]
				groupNumber := normalized[3:5]
				serialNumber := normalized[5:9]

				// Check area number constraints.
				if areaNumber == "000" || areaNumber == "666" {
					continue
				}
				if areaNumber[0] == '9' {
					continue
				}

				// Check group and serial number constraints.
				if groupNumber == "00" || serialNumber == "0000" {
					continue
				}

				locations = append(locations, PIILocation{
					Start:      match[0],
					End:        match[1],
					Type:       "ssn",
					Value:      ssn,
					Confidence: 0.90,
				})
			}
		}
	}

	return locations
}

// CreditCardRule detects credit card numbers in text.
type CreditCardRule struct {
	*BaseRule
}

// NewCreditCardRule creates a new credit card detection rule.
func NewCreditCardRule() *CreditCardRule {
	// Matches major credit card formats with optional separators.
	// Visa: 4XXX, Mastercard: 5XXX or 2XXX, Amex: 34XX or 37XX, Discover: 6XXX.
	pattern := `\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|2[2-7][0-9]{14}|3[47][0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\b`
	base, _ := NewBaseRule(
		"credit_card",
		"Detects credit card numbers",
		pattern,
		"credit_card",
		0.85,
	)
	return &CreditCardRule{BaseRule: base}
}

// Detect overrides BaseRule.Detect to handle credit card numbers with separators.
func (r *CreditCardRule) Detect(text string) []PIILocation {
	if !r.enabled {
		return nil
	}

	// Pattern that matches card numbers with optional dashes or spaces.
	patterns := []*regexp.Regexp{
		// Continuous digits.
		regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|2[2-7][0-9]{14}|3[47][0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\b`),
		// With dashes or spaces (4 groups of 4 digits).
		regexp.MustCompile(`\b(?:4[0-9]{3}[-\s]?[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4})\b`),
		regexp.MustCompile(`\b(?:5[1-5][0-9]{2}[-\s]?[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4})\b`),
		// Amex (4-6-5 grouping).
		regexp.MustCompile(`\b(?:3[47][0-9]{2}[-\s]?[0-9]{6}[-\s]?[0-9]{5})\b`),
	}

	seen := make(map[string]bool)
	var locations []PIILocation

	for _, pattern := range patterns {
		matches := pattern.FindAllStringIndex(text, -1)
		for _, match := range matches {
			value := text[match[0]:match[1]]
			// Normalize to check for duplicates.
			normalized := strings.ReplaceAll(strings.ReplaceAll(value, "-", ""), " ", "")

			if !seen[normalized] && r.luhnCheck(normalized) {
				seen[normalized] = true
				locations = append(locations, PIILocation{
					Start:      match[0],
					End:        match[1],
					Type:       "credit_card",
					Value:      value,
					Confidence: 0.95, // Higher confidence with Luhn validation.
				})
			}
		}
	}

	return locations
}

// luhnCheck validates a credit card number using the Luhn algorithm.
func (r *CreditCardRule) luhnCheck(number string) bool {
	// Remove any non-digit characters.
	var digits []int
	for _, ch := range number {
		if ch >= '0' && ch <= '9' {
			digits = append(digits, int(ch-'0'))
		}
	}

	if len(digits) < 13 || len(digits) > 19 {
		return false
	}

	// Luhn algorithm.
	sum := 0
	double := false

	for i := len(digits) - 1; i >= 0; i-- {
		digit := digits[i]
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}

	return sum%10 == 0
}

// ConfigurableRule allows defining rules from configuration files.
type ConfigurableRule struct {
	*BaseRule
}

// RuleConfig defines the structure for loading rules from YAML/JSON.
type RuleConfig struct {
	Name        string  `json:"name" yaml:"name"`
	Description string  `json:"description" yaml:"description"`
	Pattern     string  `json:"pattern" yaml:"pattern"`
	Type        string  `json:"type" yaml:"type"`
	Confidence  float64 `json:"confidence" yaml:"confidence"`
	Enabled     bool    `json:"enabled" yaml:"enabled"`
}

// NewConfigurableRule creates a rule from a configuration.
func NewConfigurableRule(cfg *RuleConfig) (*ConfigurableRule, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rule config is nil")
	}

	if cfg.Name == "" {
		return nil, fmt.Errorf("rule name is required")
	}

	if cfg.Pattern == "" {
		return nil, fmt.Errorf("rule pattern is required")
	}

	if cfg.Confidence <= 0 || cfg.Confidence > 1.0 {
		cfg.Confidence = 0.80 // Default confidence.
	}

	base, err := NewBaseRule(cfg.Name, cfg.Description, cfg.Pattern, cfg.Type, cfg.Confidence)
	if err != nil {
		return nil, err
	}

	base.enabled = cfg.Enabled

	return &ConfigurableRule{BaseRule: base}, nil
}

// RulesConfig holds a collection of rule configurations.
type RulesConfig struct {
	Rules []RuleConfig `json:"rules" yaml:"rules"`
}

// LoadRulesFromJSON loads rules from a JSON configuration file.
func LoadRulesFromJSON(path string) ([]Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading rules config: %w", err)
	}

	var config RulesConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing rules config: %w", err)
	}

	rules := make([]Rule, 0, len(config.Rules))
	for i, rc := range config.Rules {
		rule, err := NewConfigurableRule(&rc)
		if err != nil {
			return nil, fmt.Errorf("creating rule %d (%s): %w", i, rc.Name, err)
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// DefaultRules returns the standard set of PII detection rules.
func DefaultRules() []Rule {
	return []Rule{
		NewEmailPatternRule(),
		NewPhonePatternRule(),
		NewSSNPatternRule(),
		NewCreditCardRule(),
	}
}

// MergeLocations combines PII locations from multiple rules, removing duplicates
// and handling overlapping regions.
func MergeLocations(allLocations [][]PIILocation) []PIILocation {
	// Flatten all locations.
	var flat []PIILocation
	for _, locs := range allLocations {
		flat = append(flat, locs...)
	}

	if len(flat) == 0 {
		return nil
	}

	// Sort by start position (ascending).
	for i := 0; i < len(flat)-1; i++ {
		for j := 0; j < len(flat)-i-1; j++ {
			if flat[j].Start > flat[j+1].Start {
				flat[j], flat[j+1] = flat[j+1], flat[j]
			}
		}
	}

	// Merge overlapping locations, keeping higher confidence.
	merged := []PIILocation{flat[0]}

	for i := 1; i < len(flat); i++ {
		current := flat[i]
		last := &merged[len(merged)-1]

		if current.Start < last.End {
			// Overlap detected - keep the one with higher confidence.
			if current.Confidence > last.Confidence {
				last.Type = current.Type
				last.Value = current.Value
				last.Confidence = current.Confidence
			}
			// Extend the end if needed.
			if current.End > last.End {
				last.End = current.End
			}
		} else {
			merged = append(merged, current)
		}
	}

	return merged
}
