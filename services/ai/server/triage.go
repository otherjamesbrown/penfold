package server

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Valid triage categories.
var validCategories = map[string]bool{
	"PROJECT_UPDATE":  true,
	"CUSTOMER":        true,
	"RISK_ISSUE":      true,
	"ACTION_REQUEST":  true,
	"DECISION":        true,
	"INTERNAL_COMMS":  true,
	"PERSONAL":        true,
	"OTHER":           true,
}

// Valid importance levels.
var validImportance = map[string]bool{
	"HIGH":   true,
	"MEDIUM": true,
	"LOW":    true,
}

// triagePromptTemplate is the system prompt for content triage.
// Matches the spec in specs/020-slm-llm-architecture/design.md lines 170-193.
const triagePromptTemplate = `You are a content classifier for a business knowledge management system.

Classify this content into exactly ONE category:
- PROJECT_UPDATE: project status, meeting notes, deliverables, milestones
- CUSTOMER: customer communications, sales, deals, account management
- RISK_ISSUE: risks, problems, escalations, blockers, vulnerabilities
- ACTION_REQUEST: someone asking for specific action to be done
- DECISION: a decision has been made or is being requested
- INTERNAL_COMMS: HR, training, company announcements, policy changes
- PERSONAL: lunch, social, casual conversation
- OTHER: does not fit any category above

Rate importance: HIGH, MEDIUM, LOW

Respond ONLY with JSON:
{"category": "...", "importance": "...", "reason": "one sentence"}`

// buildTriagePrompt constructs the triage prompt from subject, sender, and content.
// Content is truncated to the first 500 characters as specified in the design.
func buildTriagePrompt(subject, sender, content string) (systemPrompt, userPrompt string) {
	systemPrompt = triagePromptTemplate

	// Truncate content to first 500 characters
	truncatedContent := content
	if len(truncatedContent) > 500 {
		truncatedContent = truncatedContent[:500]
	}

	// Build user prompt with subject and sender context
	var parts []string
	if subject != "" {
		parts = append(parts, fmt.Sprintf("Subject: %s", subject))
	}
	if sender != "" {
		parts = append(parts, fmt.Sprintf("From: %s", sender))
	}
	if truncatedContent != "" {
		parts = append(parts, "")
		parts = append(parts, truncatedContent)
	}

	userPrompt = strings.Join(parts, "\n")
	return systemPrompt, userPrompt
}

// triageResult holds the parsed triage response.
type triageResult struct {
	Category   string `json:"category"`
	Importance string `json:"importance"`
	Reason     string `json:"reason"`
}

// parseTriageResponse parses the JSON response from the LLM.
// Returns the category, importance, reason, and any parsing error.
func parseTriageResponse(jsonStr string) (category, importance, reason string, err error) {
	// Clean up the response
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	var result triageResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", "", "", fmt.Errorf("json unmarshal: %w", err)
	}

	// Validate the parsed result
	if err := validateTriageResult(result.Category, result.Importance); err != nil {
		return "", "", "", err
	}

	return result.Category, result.Importance, result.Reason, nil
}

// validateTriageResult checks if the category and importance are valid.
func validateTriageResult(category, importance string) error {
	if !validCategories[category] {
		return fmt.Errorf("invalid category: %s (must be one of PROJECT_UPDATE, CUSTOMER, RISK_ISSUE, ACTION_REQUEST, DECISION, INTERNAL_COMMS, PERSONAL, OTHER)", category)
	}
	if !validImportance[importance] {
		return fmt.Errorf("invalid importance: %s (must be one of HIGH, MEDIUM, LOW)", importance)
	}
	return nil
}
