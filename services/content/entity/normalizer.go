package entity

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// EntityNormalizer normalizes entity values to standard formats.
type EntityNormalizer struct {
	// dateFormats are the date formats to try when parsing dates.
	dateFormats []string

	// referenceTime is used for relative date calculations.
	referenceTime time.Time

	// phoneDefaultCountry is the default country code for phone normalization.
	phoneDefaultCountry string
}

// NormalizerConfig holds configuration for the normalizer.
type NormalizerConfig struct {
	// DateFormats are additional date formats to try.
	DateFormats []string

	// ReferenceTime is the reference time for relative dates.
	// If zero, uses current time.
	ReferenceTime time.Time

	// PhoneDefaultCountry is the default country code (e.g., "US", "GB").
	PhoneDefaultCountry string
}

// DefaultNormalizerConfig returns default configuration.
func DefaultNormalizerConfig() *NormalizerConfig {
	return &NormalizerConfig{
		DateFormats:         nil,
		ReferenceTime:       time.Time{},
		PhoneDefaultCountry: "US",
	}
}

// NewEntityNormalizer creates a new normalizer with the given configuration.
func NewEntityNormalizer(cfg *NormalizerConfig) *EntityNormalizer {
	if cfg == nil {
		cfg = DefaultNormalizerConfig()
	}

	dateFormats := []string{
		// ISO8601
		"2006-01-02",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		// US formats
		"01/02/2006",
		"1/2/2006",
		"01/02/06",
		"1/2/06",
		"01-02-2006",
		"1-2-2006",
		// EU formats
		"02/01/2006",
		"2/1/2006",
		"02-01-2006",
		// Written formats
		"January 2, 2006",
		"January 2 2006",
		"Jan 2, 2006",
		"Jan 2 2006",
		"2 January 2006",
		"2 Jan 2006",
	}

	if cfg.DateFormats != nil {
		dateFormats = append(dateFormats, cfg.DateFormats...)
	}

	refTime := cfg.ReferenceTime
	if refTime.IsZero() {
		refTime = time.Now()
	}

	return &EntityNormalizer{
		dateFormats:         dateFormats,
		referenceTime:       refTime,
		phoneDefaultCountry: cfg.PhoneDefaultCountry,
	}
}

// Normalize normalizes an entity's value based on its type.
func (n *EntityNormalizer) Normalize(entity *Entity) {
	if entity == nil {
		return
	}

	switch entity.Type {
	case EntityTypeDate:
		entity.NormalizedValue = n.NormalizeDate(entity.Value)
	case EntityTypePhone:
		entity.NormalizedValue = n.NormalizePhone(entity.Value)
	case EntityTypePerson:
		entity.NormalizedValue = n.NormalizeName(entity.Value)
	case EntityTypeEmail:
		entity.NormalizedValue = n.NormalizeEmail(entity.Value)
	case EntityTypeMoney:
		entity.NormalizedValue = n.NormalizeMoney(entity.Value)
	case EntityTypeURL:
		entity.NormalizedValue = n.NormalizeURL(entity.Value)
	case EntityTypeOrganization:
		entity.NormalizedValue = n.NormalizeOrganization(entity.Value)
	case EntityTypeLocation:
		entity.NormalizedValue = n.NormalizeLocation(entity.Value)
	default:
		// For other types, just clean up the value
		entity.NormalizedValue = strings.TrimSpace(entity.Value)
	}
}

// NormalizeDate normalizes a date string to ISO8601 format (YYYY-MM-DD).
func (n *EntityNormalizer) NormalizeDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	// Handle relative dates first
	if normalized := n.normalizeRelativeDate(value); normalized != "" {
		return normalized
	}

	// Clean up ordinal suffixes (1st, 2nd, 3rd, 4th, etc.)
	ordinalRe := regexp.MustCompile(`(\d+)(?:st|nd|rd|th)`)
	value = ordinalRe.ReplaceAllString(value, "$1")

	// Try parsing with various formats
	for _, format := range n.dateFormats {
		if t, err := time.Parse(format, value); err == nil {
			// Handle 2-digit years
			if t.Year() < 100 {
				if t.Year() < 50 {
					t = t.AddDate(2000, 0, 0)
				} else {
					t = t.AddDate(1900, 0, 0)
				}
			}
			return t.Format("2006-01-02")
		}
	}

	// Return original if parsing fails
	return value
}

// normalizeRelativeDate handles relative date expressions.
func (n *EntityNormalizer) normalizeRelativeDate(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))

	switch value {
	case "today":
		return n.referenceTime.Format("2006-01-02")
	case "tomorrow":
		return n.referenceTime.AddDate(0, 0, 1).Format("2006-01-02")
	case "yesterday":
		return n.referenceTime.AddDate(0, 0, -1).Format("2006-01-02")
	case "next week":
		return n.referenceTime.AddDate(0, 0, 7).Format("2006-01-02")
	case "last week":
		return n.referenceTime.AddDate(0, 0, -7).Format("2006-01-02")
	case "this week":
		return n.referenceTime.Format("2006-01-02")
	case "next month":
		return n.referenceTime.AddDate(0, 1, 0).Format("2006-01-02")
	case "last month":
		return n.referenceTime.AddDate(0, -1, 0).Format("2006-01-02")
	case "this month":
		return n.referenceTime.Format("2006-01-02")
	case "next year":
		return n.referenceTime.AddDate(1, 0, 0).Format("2006-01-02")
	case "last year":
		return n.referenceTime.AddDate(-1, 0, 0).Format("2006-01-02")
	case "this year":
		return n.referenceTime.Format("2006-01-02")
	}

	// Handle "in X days/weeks/months/years"
	inPattern := regexp.MustCompile(`^in\s+(\d+)\s+(days?|weeks?|months?|years?)$`)
	if matches := inPattern.FindStringSubmatch(value); matches != nil {
		num, _ := strconv.Atoi(matches[1])
		unit := matches[2]
		switch {
		case strings.HasPrefix(unit, "day"):
			return n.referenceTime.AddDate(0, 0, num).Format("2006-01-02")
		case strings.HasPrefix(unit, "week"):
			return n.referenceTime.AddDate(0, 0, num*7).Format("2006-01-02")
		case strings.HasPrefix(unit, "month"):
			return n.referenceTime.AddDate(0, num, 0).Format("2006-01-02")
		case strings.HasPrefix(unit, "year"):
			return n.referenceTime.AddDate(num, 0, 0).Format("2006-01-02")
		}
	}

	return ""
}

// NormalizePhone normalizes a phone number to E.164 format.
func (n *EntityNormalizer) NormalizePhone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	// Extract only digits and leading +
	hasPlus := strings.HasPrefix(value, "+")
	digits := extractDigits(value)

	if len(digits) == 0 {
		return value
	}

	// Handle US numbers (default)
	if n.phoneDefaultCountry == "US" && !hasPlus {
		// Remove leading 1 if present for 11-digit numbers
		if len(digits) == 11 && digits[0] == '1' {
			digits = digits[1:]
		}

		// Format as E.164 with US country code
		if len(digits) == 10 {
			return "+1" + digits
		}
	}

	// If already has +, keep it
	if hasPlus {
		return "+" + digits
	}

	return value
}

// NormalizeName normalizes a person's name to "First Last" format.
func (n *EntityNormalizer) NormalizeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	// Split by whitespace
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return value
	}

	// Handle "Last, First" format
	if len(parts) >= 2 && strings.HasSuffix(parts[0], ",") {
		lastName := strings.TrimSuffix(parts[0], ",")
		firstName := strings.Join(parts[1:], " ")
		parts = []string{firstName, lastName}
	}

	// Capitalize each part
	for i, part := range parts {
		parts[i] = capitalizeName(part)
	}

	return strings.Join(parts, " ")
}

// capitalizeName capitalizes a name part, handling special cases.
func capitalizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	// Handle prefixes like "McDonald", "O'Brien", "van", "de", "der"
	lowerPrefixes := map[string]bool{"van": true, "de": true, "der": true, "von": true, "la": true, "du": true, "den": true, "het": true}
	lower := strings.ToLower(name)

	// Keep certain prefixes lowercase (unless it's the first/only word)
	if lowerPrefixes[lower] {
		return lower
	}

	// Handle names with apostrophes (O'Brien, D'Angelo)
	if strings.Contains(name, "'") {
		parts := strings.SplitN(name, "'", 2)
		if len(parts) == 2 {
			return strings.Title(parts[0]) + "'" + strings.Title(parts[1])
		}
	}

	// Handle "Mc" and "Mac" prefixes
	if strings.HasPrefix(lower, "mc") && len(name) > 2 {
		return "Mc" + strings.ToUpper(string(name[2])) + strings.ToLower(name[3:])
	}
	if strings.HasPrefix(lower, "mac") && len(name) > 3 {
		return "Mac" + strings.ToUpper(string(name[3])) + strings.ToLower(name[4:])
	}

	// Default: capitalize first letter, lowercase rest
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}

// NormalizeEmail normalizes an email address to lowercase.
func (n *EntityNormalizer) NormalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// NormalizeMoney normalizes a money value to a standard format.
// Returns format like "USD 1234.56" or "1234.56 USD".
func (n *EntityNormalizer) NormalizeMoney(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	// Detect currency
	currency := detectCurrency(value)

	// Extract numeric value
	numericValue := extractNumericValue(value)

	if numericValue == "" {
		return value
	}

	return currency + " " + numericValue
}

// detectCurrency detects the currency from a money string.
func detectCurrency(value string) string {
	value = strings.ToUpper(value)

	// Check for currency symbols
	if strings.HasPrefix(value, "$") {
		return "USD"
	}

	// Check for currency codes
	currencies := []string{"USD", "EUR", "GBP", "JPY", "CAD", "AUD", "CHF"}
	for _, c := range currencies {
		if strings.Contains(value, c) {
			return c
		}
	}

	// Check for currency words
	if strings.Contains(strings.ToLower(value), "euro") {
		return "EUR"
	}
	if strings.Contains(strings.ToLower(value), "pound") {
		return "GBP"
	}

	return "USD" // Default
}

// extractNumericValue extracts the numeric value from a money string.
func extractNumericValue(value string) string {
	// Remove currency symbols and letters
	re := regexp.MustCompile(`[^0-9.,]`)
	numeric := re.ReplaceAllString(value, "")

	// Normalize decimal separator
	// If there's a comma followed by exactly 2 digits at the end, it's likely a decimal
	if match := regexp.MustCompile(`^([0-9.]+),([0-9]{2})$`).FindStringSubmatch(numeric); match != nil {
		numeric = match[1] + "." + match[2]
	}

	// Remove thousands separators (commas followed by 3 digits)
	numeric = regexp.MustCompile(`([0-9]),([0-9]{3})`).ReplaceAllString(numeric, "$1$2")

	// Parse and format to 2 decimal places
	if f, err := strconv.ParseFloat(numeric, 64); err == nil {
		return strconv.FormatFloat(f, 'f', 2, 64)
	}

	return numeric
}

// NormalizeURL normalizes a URL by ensuring consistent formatting.
func (n *EntityNormalizer) NormalizeURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	// Remove trailing slashes for consistency
	value = strings.TrimRight(value, "/")

	// Remove trailing punctuation that's not part of the URL
	value = strings.TrimRight(value, ".,;:!?")

	// Ensure lowercase scheme
	if strings.HasPrefix(value, "HTTP://") {
		value = "http://" + value[7:]
	} else if strings.HasPrefix(value, "HTTPS://") {
		value = "https://" + value[8:]
	}

	return value
}

// NormalizeOrganization normalizes an organization name.
func (n *EntityNormalizer) NormalizeOrganization(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	// Remove common suffixes for comparison
	suffixes := []string{", Inc.", ", Inc", " Inc.", " Inc", ", LLC", " LLC",
		", Ltd.", ", Ltd", " Ltd.", " Ltd", " Corporation", " Corp.", " Corp"}

	normalized := value
	for _, suffix := range suffixes {
		if strings.HasSuffix(strings.ToLower(normalized), strings.ToLower(suffix)) {
			// Keep the original case but note the suffix
			break
		}
	}

	return normalized
}

// NormalizeLocation normalizes a location/address.
func (n *EntityNormalizer) NormalizeLocation(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	// Standardize common abbreviations
	replacements := map[string]string{
		" St.":   " Street",
		" St ":   " Street ",
		" Ave.":  " Avenue",
		" Ave ":  " Avenue ",
		" Blvd.": " Boulevard",
		" Blvd ": " Boulevard ",
		" Dr.":   " Drive",
		" Dr ":   " Drive ",
		" Rd.":   " Road",
		" Rd ":   " Road ",
		" Ln.":   " Lane",
		" Ln ":   " Lane ",
		" Ct.":   " Court",
		" Ct ":   " Court ",
	}

	for abbrev, full := range replacements {
		if strings.Contains(value, abbrev) {
			value = strings.ReplaceAll(value, abbrev, full)
		}
	}

	return value
}

// extractDigits extracts only digit characters from a string.
func extractDigits(s string) string {
	var digits strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	return digits.String()
}

// NormalizeEntities normalizes a slice of entities in place.
func (n *EntityNormalizer) NormalizeEntities(entities []Entity) {
	for i := range entities {
		n.Normalize(&entities[i])
	}
}

// NormalizeAll normalizes all entities in an extraction result.
func (n *EntityNormalizer) NormalizeAll(result *ExtractionResult) {
	n.NormalizeEntities(result.Entities)
}
