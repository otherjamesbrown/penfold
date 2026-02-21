// Package parse provides deterministic parsing utilities for content processing.
// This is Stage 0 of the SLM/LLM pipeline - pure library-based parsing with no AI.
package parse

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// EmailParseResult holds the result of parsing email content for the pipeline.
type EmailParseResult struct {
	CleanBody           string   // HTML-stripped, clean text body
	NewContent          string   // Content before quoted reply (the new message)
	QuotedContent       string   // Content after quoted reply marker (previous messages)
	QuotedReplyDetected bool     // True if a quoted reply was detected
	Warnings            []string // Parsing warnings (non-fatal)
}

// ParseEmailBody takes raw email body (text or HTML) and produces clean text.
// If bodyHTML is non-empty, it strips HTML. Otherwise uses bodyText directly.
// Then detects and separates quoted replies.
func ParseEmailBody(bodyText, bodyHTML string) *EmailParseResult {
	result := &EmailParseResult{
		Warnings: []string{},
	}

	// Step 1: Convert HTML to text if present
	if bodyHTML != "" {
		result.CleanBody = htmlToText(bodyHTML)
	} else {
		result.CleanBody = bodyText
	}

	// Trim and normalize whitespace
	result.CleanBody = normalizeWhitespace(result.CleanBody)

	// Step 2: Detect and separate quoted replies
	newContent, quotedContent, detected := separateQuotedReply(result.CleanBody)
	result.NewContent = newContent
	result.QuotedContent = quotedContent
	result.QuotedReplyDetected = detected

	return result
}

// htmlToText converts HTML to plain text.
// - Strips all tags
// - Preserves paragraph breaks (p, div, br → newlines)
// - Decodes HTML entities (&amp; &lt; etc.)
// - Removes style and script blocks entirely
// - Collapses consecutive newlines to max 2
func htmlToText(htmlContent string) string {
	var text strings.Builder
	var inStyle, inScript bool

	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))

	for {
		tokenType := tokenizer.Next()

		switch tokenType {
		case html.ErrorToken:
			// End of document
			result := text.String()
			// Decode HTML entities
			result = html.UnescapeString(result)
			return result

		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			tagName := strings.ToLower(token.Data)

			// Track style/script blocks
			if tagName == "style" {
				inStyle = true
			} else if tagName == "script" {
				inScript = true
			}

			// Add newlines for block elements
			if isBlockElement(tagName) && text.Len() > 0 {
				text.WriteString("\n")
			}

			// Special handling for links (extract href if needed in future)
			// For now, just the link text will be captured in TextToken

		case html.EndTagToken:
			token := tokenizer.Token()
			tagName := strings.ToLower(token.Data)

			if tagName == "style" {
				inStyle = false
			} else if tagName == "script" {
				inScript = false
			}

			// Add newline after block elements
			if isBlockElement(tagName) && text.Len() > 0 {
				text.WriteString("\n")
			}

		case html.TextToken:
			// Skip text inside style/script tags
			if inStyle || inScript {
				continue
			}

			token := tokenizer.Token()
			content := strings.TrimSpace(token.Data)
			if content != "" {
				// Add space before text if needed (but not before punctuation)
				if text.Len() > 0 && !strings.HasSuffix(text.String(), "\n") && !strings.HasSuffix(text.String(), " ") {
					// Don't add space if content starts with punctuation
					firstChar := content[0]
					if !strings.ContainsRune(".,!?;:)]}'\"", rune(firstChar)) {
						text.WriteString(" ")
					}
				}
				text.WriteString(content)
			}
		}
	}
}

// isBlockElement returns true if the HTML tag should create a paragraph break.
func isBlockElement(tagName string) bool {
	blockElements := map[string]bool{
		"p":          true,
		"div":        true,
		"br":         true,
		"h1":         true,
		"h2":         true,
		"h3":         true,
		"h4":         true,
		"h5":         true,
		"h6":         true,
		"blockquote": true,
		"pre":        true,
		"ul":         true,
		"ol":         true,
		"li":         true,
		"table":      true,
		"tr":         true,
		"td":         true,
		"th":         true,
		"hr":         true,
		"section":    true,
		"article":    true,
		"header":     true,
		"footer":     true,
		"nav":        true,
		"aside":      true,
	}
	return blockElements[tagName]
}

// normalizeWhitespace collapses consecutive newlines to max 2 and trims whitespace.
func normalizeWhitespace(text string) string {
	// Replace Windows line endings with Unix
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// Collapse multiple spaces to single space (but preserve newlines)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	text = strings.Join(lines, "\n")

	// Collapse 3+ consecutive newlines to 2
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(text)
}

// separateQuotedReply detects and separates quoted reply content from new content.
// Returns: (newContent, quotedContent, detected)
//
// Detection patterns:
// 1. "On ... wrote:" (Gmail, Apple Mail)
// 2. Lines starting with ">" (standard quoting)
// 3. "From: ... Sent: ... To: ... Subject: ..." (Outlook)
// 4. "-----Original Message-----" (Outlook separator)
func separateQuotedReply(text string) (newContent, quotedContent string, detected bool) {
	lines := strings.Split(text, "\n")

	// Pattern 1: "On ... wrote:" attribution line
	onWrotePattern := regexp.MustCompile(`(?i)^On .*(,| ).*wrote:`)

	// Pattern 4: "-----Original Message-----"
	originalMessagePattern := regexp.MustCompile(`(?i)^-+\s*original message\s*-+`)

	// Pattern 5: "---------- Forwarded message ----------" (Gmail forward)
	gmailForwardPattern := regexp.MustCompile(`(?i)^-+\s*forwarded message\s*-+`)

	// Pattern 6: "Begin forwarded message:" (Apple Mail forward)
	appleForwardPattern := regexp.MustCompile(`(?i)^Begin forwarded message:`)

	// Pattern 3: Outlook-style "From: ... Sent: ..." block
	// Look for "From:" followed by "Sent:" or "Date:" within a few lines
	// "Date:" is used by Apple Mail and Outlook for Mac instead of "Sent:"
	// Note: No ^ anchor — after HTML-to-text, "From:" may not be at line start
	// (e.g., Outlook separator text preceding "From:" on same line).
	// The Sent/Date lookahead prevents false positives.
	outlookFromPattern := regexp.MustCompile(`(?i)From:\s+.+`)
	outlookSentPattern := regexp.MustCompile(`(?i)^(Sent|Date):\s+.+`)

	splitIndex := -1
	detectionMethod := ""

	// Scan for quoted reply markers
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for "On ... wrote:"
		if onWrotePattern.MatchString(trimmed) {
			splitIndex = i
			detectionMethod = "on-wrote"
			break
		}

		// Check for "-----Original Message-----"
		if originalMessagePattern.MatchString(trimmed) {
			splitIndex = i
			detectionMethod = "original-message"
			break
		}

		// Check for "---------- Forwarded message ----------" (Gmail)
		if gmailForwardPattern.MatchString(trimmed) {
			splitIndex = i
			detectionMethod = "gmail-forward"
			break
		}

		// Check for "Begin forwarded message:" (Apple Mail)
		if appleForwardPattern.MatchString(trimmed) {
			splitIndex = i
			detectionMethod = "apple-forward"
			break
		}

		// Check for Outlook block (From: followed by Sent: or Date: within 5 lines)
		if outlookFromPattern.MatchString(trimmed) {
			// Look ahead for Sent: within next 5 lines
			for j := i + 1; j < len(lines) && j < i+6; j++ {
				if outlookSentPattern.MatchString(strings.TrimSpace(lines[j])) {
					splitIndex = i
					detectionMethod = "outlook-block"
					break
				}
			}
			if splitIndex != -1 {
				break
			}
		}

		// Check for lines starting with ">" (but need several in a row to be confident)
		// Count consecutive ">" lines
		if strings.HasPrefix(trimmed, ">") && i < len(lines)-2 {
			consecutiveQuoted := 1
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), ">") {
					consecutiveQuoted++
				} else if strings.TrimSpace(lines[j]) != "" {
					break
				}
			}
			// If we have 3+ consecutive quoted lines, consider this the split point
			if consecutiveQuoted >= 3 {
				splitIndex = i
				detectionMethod = "angle-bracket"
				break
			}
		}
	}

	// No quoted reply detected
	if splitIndex == -1 {
		return text, "", false
	}

	// Split at the detected point
	newLines := lines[:splitIndex]
	quotedLines := lines[splitIndex:]

	newContent = strings.TrimSpace(strings.Join(newLines, "\n"))
	quotedContent = strings.TrimSpace(strings.Join(quotedLines, "\n"))

	// Clean up quoted content if it uses ">" prefix
	if detectionMethod == "angle-bracket" {
		quotedContent = cleanAngleBracketQuoting(quotedContent)
	}

	return newContent, quotedContent, true
}

// cleanAngleBracketQuoting removes leading ">" characters from quoted lines.
func cleanAngleBracketQuoting(quotedText string) string {
	lines := strings.Split(quotedText, "\n")
	for i, line := range lines {
		// Remove leading "> " or ">" from each line
		trimmed := strings.TrimSpace(line)
		for strings.HasPrefix(trimmed, ">") {
			trimmed = strings.TrimPrefix(trimmed, ">")
			trimmed = strings.TrimSpace(trimmed)
		}
		lines[i] = trimmed
	}
	return strings.Join(lines, "\n")
}

// QuotedReplyParticipant represents an email participant extracted from quoted reply headers.
type QuotedReplyParticipant struct {
	Email       string // Email address (lowercase, normalized)
	DisplayName string // Display name if present (e.g., "John Doe" from "Doe, John <john@example.com>")
}

// ExtractQuotedReplyParticipantsWithNames scans body text for email addresses and display names
// in quoted reply headers. Handles multiple formats:
// - Gmail: "On Mon, Feb 10, 2026, Name <email@example.com> wrote:"
// - Outlook: "From: Lastname, Firstname <email@example.com>"
// - Generic: "From: email@example.com"
// Also handles nested quoting with ">" prefix (e.g., "> From: Name <email@example.com>")
// Returns deduplicated list of participants with email and display name.
// Display names in "Lastname, Firstname" format are normalized to "Firstname Lastname".
func ExtractQuotedReplyParticipantsWithNames(bodyText string) []QuotedReplyParticipant {
	seen := make(map[string]bool)
	participants := make([]QuotedReplyParticipant, 0)

	// Pattern 1: Gmail-style "On ... Name <email@example.com> wrote:"
	// Capture group 1: display name, group 2: email
	// Matches: "On Mon, Feb 10, 2026, Aurore Defiolles <aurore@example.com> wrote:"
	// Matches: "On Mon, Feb 10, 2026, Pandya, Parag <ppandya@akamai.com> wrote:"
	// Strategy: Match "On ... wrote:" and extract name from between last date/time and email
	// Look for pattern after year (4 digits) and before email angle bracket
	gmailPattern := regexp.MustCompile(`(?i)On\s+.+\d{4},\s+([^<]+?)\s+<([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})>\s+wrote:`)

	// Pattern 2: Outlook-style "From: Name <email@example.com>"
	// Capture group 1: display name, group 2: email
	// Matches: "From: Jane Doe <jane@example.com>" or "From: Doe, Jane <jane@example.com>"
	outlookPattern := regexp.MustCompile(`(?i)From:\s+(.+?)\s+<([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})>`)

	// Pattern 3: Simple "From: email@example.com" (no angle brackets, no display name)
	// Capture group 1: email
	simpleFromPattern := regexp.MustCompile(`(?i)From:\s+([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)

	lines := strings.Split(bodyText, "\n")
	for _, line := range lines {
		// Remove leading ">" characters (for nested quoting) before matching
		trimmed := strings.TrimSpace(line)
		for strings.HasPrefix(trimmed, ">") {
			trimmed = strings.TrimPrefix(trimmed, ">")
			trimmed = strings.TrimSpace(trimmed)
		}

		// Try Gmail pattern first (most specific)
		// Capture groups: [0]=full match, [1]=display name, [2]=email
		if matches := gmailPattern.FindStringSubmatch(trimmed); len(matches) > 2 {
			email := strings.ToLower(matches[2])
			if !seen[email] {
				seen[email] = true
				displayName := normalizeDisplayName(matches[1])
				participants = append(participants, QuotedReplyParticipant{
					Email:       email,
					DisplayName: displayName,
				})
			}
			continue
		}

		// Try Outlook pattern with angle brackets
		// Capture groups: [0]=full match, [1]=display name, [2]=email
		if matches := outlookPattern.FindStringSubmatch(trimmed); len(matches) > 2 {
			email := strings.ToLower(matches[2])
			if !seen[email] {
				seen[email] = true
				displayName := normalizeDisplayName(matches[1])
				participants = append(participants, QuotedReplyParticipant{
					Email:       email,
					DisplayName: displayName,
				})
			}
			continue
		}

		// Try simple From: pattern (no display name)
		// Capture groups: [0]=full match, [1]=email
		if matches := simpleFromPattern.FindStringSubmatch(trimmed); len(matches) > 1 {
			email := strings.ToLower(matches[1])
			if !seen[email] {
				seen[email] = true
				participants = append(participants, QuotedReplyParticipant{
					Email:       email,
					DisplayName: "", // No display name available
				})
			}
		}
	}

	return participants
}

// normalizeDisplayName normalizes a display name from quoted reply header.
// - "Zeeshan, Usama" → "Usama Zeeshan" (flip "Lastname, Firstname" to "Firstname Lastname")
// - "  James  Brown  " → "James Brown" (trim and collapse whitespace)
func normalizeDisplayName(name string) string {
	if name == "" {
		return ""
	}

	// Trim whitespace
	name = strings.TrimSpace(name)

	// Handle "Lastname, Firstname" format
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

	// Normalize whitespace (collapse multiple spaces)
	name = strings.Join(strings.Fields(name), " ")

	return name
}

// ExtractQuotedReplyParticipants scans body text for email addresses in quoted reply headers.
// This is a backwards-compatible wrapper around ExtractQuotedReplyParticipantsWithNames
// that returns only email addresses (no display names).
// For new code, prefer ExtractQuotedReplyParticipantsWithNames to capture display names.
func ExtractQuotedReplyParticipants(bodyText string) []string {
	participants := ExtractQuotedReplyParticipantsWithNames(bodyText)
	emails := make([]string, len(participants))
	for i, p := range participants {
		emails[i] = p.Email
	}
	return emails
}
