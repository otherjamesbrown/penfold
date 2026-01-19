package entities

import (
	"regexp"
	"strings"
	"unicode"
)

// NormalizeDisplayName normalizes a display name to canonical form.
// - "Eskelsen, Rick" → "Rick Eskelsen"
// - "  James  Brown  " → "James Brown"
// - "\"John Doe\"" → "John Doe"
func NormalizeDisplayName(name string) string {
	if name == "" {
		return ""
	}

	// Remove quotes
	name = strings.Trim(name, `"'`)

	// Handle "Last, First" format
	if strings.Contains(name, ",") {
		parts := strings.SplitN(name, ",", 2)
		if len(parts) == 2 {
			first := strings.TrimSpace(parts[1])
			last := strings.TrimSpace(parts[0])
			if first != "" && last != "" {
				name = first + " " + last
			}
		}
	}

	// Normalize whitespace
	name = strings.Join(strings.Fields(name), " ")

	// Title case each word
	name = titleCase(name)

	return name
}

// titleCase converts a string to title case, handling edge cases.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) == 0 {
			continue
		}
		// Convert to title case
		runes := []rune(strings.ToLower(word))
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

// ExtractDomain extracts the domain from an email address.
func ExtractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	// Both local part and domain must be non-empty
	if parts[0] == "" || parts[1] == "" {
		return ""
	}
	return strings.ToLower(parts[1])
}

// Common patterns for account type detection
var (
	botPatterns = []string{
		"noreply",
		"no-reply",
		"donotreply",
		"do-not-reply",
		"mailer-daemon",
		"postmaster",
		"jira",
		"jenkins",
		"github",
		"gitlab",
		"circleci",
		"travis",
		"bot",
		"automation",
		"system",
		"alert",
		"notification",
	}

	distributionPatterns = []string{
		"team-",
		"all-",
		"group-",
		"list-",
		"dl-",
		"dept-",
		"everyone",
		"staff",
		"employees",
	}

	rolePatterns = []string{
		"support",
		"sales",
		"info",
		"contact",
		"help",
		"admin",
		"security",
		"hr",
		"recruiting",
		"careers",
		"press",
		"media",
		"legal",
		"finance",
		"billing",
		"accounts",
		"facilitator",
	}

	externalServiceDomains = []string{
		"docs.google.com",
		"calendar.google.com",
		"slack.com",
		"atlassian.net",
		"github.com",
		"gitlab.com",
		"circleci.com",
		"travis-ci.org",
		"travis-ci.com",
	}
)

// DetectAccountType determines the account type based on email and display name.
func DetectAccountType(email, displayName string) AccountType {
	emailLower := strings.ToLower(email)
	domain := ExtractDomain(email)

	// Check for external service first
	for _, svcDomain := range externalServiceDomains {
		if strings.HasSuffix(domain, svcDomain) || domain == svcDomain {
			return AccountTypeExternalService
		}
	}

	// Check local part patterns
	localPart := strings.ToLower(strings.Split(email, "@")[0])

	// Bot patterns
	for _, pattern := range botPatterns {
		if strings.Contains(localPart, pattern) || strings.Contains(emailLower, pattern) {
			return AccountTypeBot
		}
	}

	// Distribution list patterns
	for _, pattern := range distributionPatterns {
		if strings.HasPrefix(localPart, pattern) {
			return AccountTypeDistribution
		}
	}

	// Role account patterns
	for _, pattern := range rolePatterns {
		if localPart == pattern || strings.HasPrefix(localPart, pattern+"-") {
			return AccountTypeRole
		}
	}

	// Default to person
	return AccountTypePerson
}

// NameSimilarity calculates similarity between two names (0.0 to 1.0).
func NameSimilarity(a, b string) float64 {
	// Normalize both names
	a = strings.ToLower(NormalizeDisplayName(a))
	b = strings.ToLower(NormalizeDisplayName(b))

	if a == "" || b == "" {
		return 0.0
	}

	// Exact match
	if a == b {
		return 1.0
	}

	// Check if one contains the other
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return 0.9
	}

	// Check if all words from shorter are in longer
	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)

	shorter, longer := wordsA, wordsB
	if len(wordsA) > len(wordsB) {
		shorter, longer = wordsB, wordsA
	}

	matchCount := 0
	for _, word := range shorter {
		for _, other := range longer {
			if word == other {
				matchCount++
				break
			}
		}
	}

	if len(shorter) > 0 && matchCount == len(shorter) {
		return 0.85
	}

	// Levenshtein similarity for close matches
	return levenshteinSimilarity(a, b)
}

// levenshteinSimilarity calculates similarity based on Levenshtein distance.
func levenshteinSimilarity(a, b string) float64 {
	distance := levenshteinDistance(a, b)
	maxLen := max(len(a), len(b))
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(distance)/float64(maxLen)
}

// levenshteinDistance calculates the Levenshtein distance between two strings.
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create distance matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
	}

	// Initialize first row and column
	for i := 0; i <= len(a); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		matrix[0][j] = j
	}

	// Fill in the rest
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

func min(values ...int) int {
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// IsInternalDomain checks if an email is from an internal domain.
func IsInternalDomain(email string, internalDomains []string) bool {
	domain := ExtractDomain(email)
	for _, internal := range internalDomains {
		if domain == internal || strings.HasSuffix(domain, "."+internal) {
			return true
		}
	}
	return false
}

// emailLocalPartPattern matches the local part of an email
var emailLocalPartPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+$`)

// IsValidEmail checks if a string looks like a valid email address.
func IsValidEmail(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	local, domain := parts[0], parts[1]
	if local == "" || domain == "" {
		return false
	}
	if !emailLocalPartPattern.MatchString(local) {
		return false
	}
	if !strings.Contains(domain, ".") {
		return false
	}
	return true
}
