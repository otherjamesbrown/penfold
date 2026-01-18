package entity

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PatternMatcher defines the interface for pattern-based entity extraction.
type PatternMatcher interface {
	// Match finds all matches of the pattern in the text.
	Match(text string) []PatternMatch

	// EntityType returns the type of entity this pattern matches.
	EntityType() EntityType

	// Name returns a descriptive name for this pattern.
	Name() string
}

// PatternMatch represents a single pattern match in text.
type PatternMatch struct {
	Value    string
	Start    int
	End      int
	Metadata map[string]string
}

// CompiledPattern is a regex-based pattern matcher.
type CompiledPattern struct {
	pattern    *regexp.Regexp
	entityType EntityType
	name       string
	confidence float32
}

// NewCompiledPattern creates a new compiled pattern matcher.
func NewCompiledPattern(pattern string, entityType EntityType, name string, confidence float32) (*CompiledPattern, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &CompiledPattern{
		pattern:    re,
		entityType: entityType,
		name:       name,
		confidence: confidence,
	}, nil
}

// Match finds all matches in the text.
func (p *CompiledPattern) Match(text string) []PatternMatch {
	matches := p.pattern.FindAllStringIndex(text, -1)
	result := make([]PatternMatch, 0, len(matches))
	for _, m := range matches {
		result = append(result, PatternMatch{
			Value: text[m[0]:m[1]],
			Start: m[0],
			End:   m[1],
		})
	}
	return result
}

// EntityType returns the entity type for this pattern.
func (p *CompiledPattern) EntityType() EntityType {
	return p.entityType
}

// Name returns the pattern name.
func (p *CompiledPattern) Name() string {
	return p.name
}

// Confidence returns the confidence score for matches.
func (p *CompiledPattern) Confidence() float32 {
	return p.confidence
}

// ===== Email Patterns =====

// EmailPattern matches email addresses.
var EmailPattern = mustCompilePattern(
	`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`,
	EntityTypeEmail,
	"email",
	0.95,
)

// ===== URL Patterns =====

// URLPattern matches HTTP/HTTPS URLs.
var URLPattern = mustCompilePattern(
	`https?://[^\s<>"'\)\]\}]+`,
	EntityTypeURL,
	"url",
	0.95,
)

// ===== Phone Patterns =====

// PhonePatternUS matches US phone numbers in various formats.
var PhonePatternUS = mustCompilePattern(
	`(?:\+1[-.\s]?)?(?:\(?[0-9]{3}\)?[-.\s]?)?[0-9]{3}[-.\s]?[0-9]{4}`,
	EntityTypePhone,
	"phone_us",
	0.85,
)

// PhonePatternInternational matches international phone numbers.
var PhonePatternInternational = mustCompilePattern(
	`\+[1-9][0-9]{0,2}[-.\s]?(?:[0-9][-.\s]?){6,14}[0-9]`,
	EntityTypePhone,
	"phone_international",
	0.80,
)

// ===== Date Patterns =====

// DatePatternISO matches ISO8601 dates (YYYY-MM-DD).
var DatePatternISO = mustCompilePattern(
	`\b[0-9]{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12][0-9]|3[01])\b`,
	EntityTypeDate,
	"date_iso",
	0.95,
)

// DatePatternUSSlash matches US dates with slashes (MM/DD/YYYY or M/D/YYYY).
var DatePatternUSSlash = mustCompilePattern(
	`\b(?:0?[1-9]|1[0-2])/(?:0?[1-9]|[12][0-9]|3[01])/(?:[0-9]{2}|[0-9]{4})\b`,
	EntityTypeDate,
	"date_us_slash",
	0.85,
)

// DatePatternEUSlash matches EU dates with slashes (DD/MM/YYYY).
var DatePatternEUSlash = mustCompilePattern(
	`\b(?:0?[1-9]|[12][0-9]|3[01])/(?:0?[1-9]|1[0-2])/(?:[0-9]{2}|[0-9]{4})\b`,
	EntityTypeDate,
	"date_eu_slash",
	0.80,
)

// DatePatternWritten matches written dates like "January 15, 2024" or "Jan 15 2024".
var DatePatternWritten = mustCompilePattern(
	`\b(?:January|February|March|April|May|June|July|August|September|October|November|December|`+
		`Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sep|Sept|Oct|Nov|Dec)\.?\s+[0-9]{1,2}(?:st|nd|rd|th)?(?:,?\s+[0-9]{4})?\b`,
	EntityTypeDate,
	"date_written",
	0.90,
)

// DatePatternRelative matches relative dates like "today", "tomorrow", "next week".
var DatePatternRelative = mustCompilePattern(
	`\b(?:today|tomorrow|yesterday|next\s+(?:week|month|year)|last\s+(?:week|month|year)|`+
		`this\s+(?:week|month|year)|in\s+[0-9]+\s+(?:days?|weeks?|months?|years?))\b`,
	EntityTypeDate,
	"date_relative",
	0.85,
)

// ===== Money Patterns =====

// MoneyPatternUSD matches USD amounts like "$1,234.56" or "$100".
var MoneyPatternUSD = mustCompilePattern(
	`\$[0-9]{1,3}(?:,[0-9]{3})*(?:\.[0-9]{2})?\b`,
	EntityTypeMoney,
	"money_usd",
	0.90,
)

// MoneyPatternEUR matches EUR amounts like "EUR 1,234.56" or "1234.56 euros".
var MoneyPatternEUR = mustCompilePattern(
	`(?:EUR\s*)?[0-9]{1,3}(?:[.,][0-9]{3})*(?:[.,][0-9]{2})?\s*(?:EUR|euros?|EUROS?)\b|`+
		`\bEUR\s*[0-9]{1,3}(?:[.,][0-9]{3})*(?:[.,][0-9]{2})?`,
	EntityTypeMoney,
	"money_eur",
	0.85,
)

// MoneyPatternGBP matches GBP amounts like "GBP 1,234" or "100 pounds".
var MoneyPatternGBP = mustCompilePattern(
	`(?:GBP\s*)?[0-9]{1,3}(?:,[0-9]{3})*(?:\.[0-9]{2})?\s*(?:GBP|pounds?|POUNDS?)\b|`+
		`\bGBP\s*[0-9]{1,3}(?:,[0-9]{3})*(?:\.[0-9]{2})?`,
	EntityTypeMoney,
	"money_gbp",
	0.85,
)

// MoneyPatternGeneric matches generic money amounts with currency codes.
var MoneyPatternGeneric = mustCompilePattern(
	`\b[A-Z]{3}\s*[0-9]{1,3}(?:[.,][0-9]{3})*(?:[.,][0-9]{2})?\b`,
	EntityTypeMoney,
	"money_generic",
	0.75,
)

// ===== Address Patterns =====

// AddressPatternUSStreet matches US street addresses.
var AddressPatternUSStreet = mustCompilePattern(
	`\b[0-9]+\s+[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*\s+`+
		`(?:Street|St|Avenue|Ave|Boulevard|Blvd|Drive|Dr|Road|Rd|Lane|Ln|Way|Court|Ct|Circle|Cir|Place|Pl)\.?\b`,
	EntityTypeLocation,
	"address_us_street",
	0.80,
)

// AddressPatternUSZip matches US ZIP codes.
var AddressPatternUSZip = mustCompilePattern(
	`\b[0-9]{5}(?:-[0-9]{4})?\b`,
	EntityTypeLocation,
	"address_us_zip",
	0.70,
)

// AddressPatternUKPostcode matches UK postcodes.
var AddressPatternUKPostcode = mustCompilePattern(
	`\b[A-Z]{1,2}[0-9][0-9A-Z]?\s*[0-9][A-Z]{2}\b`,
	EntityTypeLocation,
	"address_uk_postcode",
	0.85,
)

// ===== Default Pattern Registry =====

// DefaultPatterns returns the default set of pattern matchers.
func DefaultPatterns() []*CompiledPattern {
	return []*CompiledPattern{
		// Email
		EmailPattern,
		// URL
		URLPattern,
		// Phone
		PhonePatternUS,
		PhonePatternInternational,
		// Date
		DatePatternISO,
		DatePatternUSSlash,
		DatePatternWritten,
		DatePatternRelative,
		// Money
		MoneyPatternUSD,
		MoneyPatternEUR,
		MoneyPatternGBP,
		// Address
		AddressPatternUSStreet,
		AddressPatternUSZip,
		AddressPatternUKPostcode,
	}
}

// PatternsByType returns patterns filtered by entity type.
func PatternsByType(patterns []*CompiledPattern, entityType EntityType) []*CompiledPattern {
	var result []*CompiledPattern
	for _, p := range patterns {
		if p.entityType == entityType {
			result = append(result, p)
		}
	}
	return result
}

// mustCompilePattern creates a CompiledPattern or panics.
func mustCompilePattern(pattern string, entityType EntityType, name string, confidence float32) *CompiledPattern {
	p, err := NewCompiledPattern(pattern, entityType, name, confidence)
	if err != nil {
		panic("invalid pattern " + name + ": " + err.Error())
	}
	return p
}

// PatternExtractor extracts entities using regex patterns.
type PatternExtractor struct {
	patterns []*CompiledPattern
}

// NewPatternExtractor creates a new pattern-based extractor.
func NewPatternExtractor(patterns []*CompiledPattern) *PatternExtractor {
	if patterns == nil {
		patterns = DefaultPatterns()
	}
	return &PatternExtractor{
		patterns: patterns,
	}
}

// Extract extracts entities from text using patterns.
func (e *PatternExtractor) Extract(text string, opts ExtractionOptions) []Entity {
	if text == "" {
		return nil
	}

	var entities []Entity
	seen := make(map[string]bool) // Track seen values to avoid exact duplicates within same run

	for _, pattern := range e.patterns {
		// Check if we should extract this entity type
		if !opts.ShouldExtractType(pattern.EntityType()) {
			continue
		}

		matches := pattern.Match(text)
		for _, match := range matches {
			// Skip if we've already seen this exact value
			key := string(pattern.EntityType()) + ":" + match.Value
			if seen[key] {
				continue
			}
			seen[key] = true

			// Check confidence threshold
			if pattern.Confidence() < opts.MinConfidence {
				continue
			}

			entity := Entity{
				ID:         uuid.New().String(),
				Type:       pattern.EntityType(),
				Value:      match.Value,
				Confidence: pattern.Confidence(),
				Source:     ExtractionSourcePattern,
				Metadata: map[string]string{
					"pattern_name": pattern.Name(),
				},
				CreatedAt: time.Now(),
			}

			if opts.TrackPositions {
				entity.Position = TextPosition{
					Start: match.Start,
					End:   match.End,
				}
			}

			entities = append(entities, entity)
		}
	}

	// Limit number of entities if specified
	if opts.MaxEntities > 0 && len(entities) > opts.MaxEntities {
		entities = entities[:opts.MaxEntities]
	}

	return entities
}

// AddPattern adds a pattern to the extractor.
func (e *PatternExtractor) AddPattern(pattern *CompiledPattern) {
	e.patterns = append(e.patterns, pattern)
}

// RemovePatternByName removes a pattern by name.
func (e *PatternExtractor) RemovePatternByName(name string) {
	var filtered []*CompiledPattern
	for _, p := range e.patterns {
		if p.Name() != name {
			filtered = append(filtered, p)
		}
	}
	e.patterns = filtered
}

// GetPatterns returns all registered patterns.
func (e *PatternExtractor) GetPatterns() []*CompiledPattern {
	return e.patterns
}

// ===== Helper Functions =====

// CleanMatchValue removes common noise from matched values.
func CleanMatchValue(value string) string {
	// Trim whitespace
	value = strings.TrimSpace(value)
	// Remove trailing punctuation
	value = strings.TrimRight(value, ".,;:!?")
	return value
}

// IsLikelyFalsePositive checks if a match is likely a false positive.
func IsLikelyFalsePositive(match PatternMatch, entityType EntityType, context string) bool {
	value := strings.ToLower(match.Value)

	switch entityType {
	case EntityTypePhone:
		// Phone numbers that are too short
		digitsOnly := regexp.MustCompile(`[0-9]`).FindAllString(value, -1)
		if len(digitsOnly) < 7 {
			return true
		}
	case EntityTypeEmail:
		// Common test/placeholder emails
		if strings.Contains(value, "example.com") ||
			strings.Contains(value, "test.com") ||
			strings.Contains(value, "localhost") {
			return true
		}
	case EntityTypeURL:
		// URLs that are just protocols
		if value == "http://" || value == "https://" {
			return true
		}
	case EntityTypeDate:
		// Check for obviously invalid dates
		if strings.HasPrefix(value, "00") || strings.HasSuffix(value, "/00") {
			return true
		}
	}

	return false
}
