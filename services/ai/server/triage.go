package server

import (
	"context"
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
	"FYI":             true,
}

// Valid importance levels.
var validImportance = map[string]bool{
	"HIGH":   true,
	"MEDIUM": true,
	"LOW":    true,
}

// Valid content contribution levels.
var validContribution = map[string]bool{
	"HIGH":   true,
	"MEDIUM": true,
	"LOW":    true,
	"NONE":   true,
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
- FYI: informational content, no action required, does not fit any category above

Rate importance: HIGH, MEDIUM, LOW

Assess content_contribution (how much NEW information the message body contributes):
- HIGH: Contains substantial new information (decisions, action items, detailed updates, important facts)
- MEDIUM: Contains some new information mixed with boilerplate/context
- LOW: Mostly acknowledgements, brief replies with minimal new content
- NONE: Pure acknowledgements, automated replies, no actionable information (e.g., "Thanks", "Got it", "OK")

Assess contribution INDEPENDENT of thread importance. A brief "I resign" has HIGH contribution despite being 2 words.

Respond ONLY with JSON:
{"category": "...", "importance": "...", "reason": "one sentence", "content_contribution": "...", "contribution_reason": "one sentence"}`

// buildTriagePrompt constructs the triage prompt from subject, sender, and content.
// Content is truncated to the first 500 characters as specified in the design.
// It tries the DB prompt store first; falls back to the hardcoded triagePromptTemplate on error.
// Returns the system prompt, user prompt, and the prompt version (0 when using hardcoded fallback).
func (s *AIServer) buildTriagePrompt(ctx context.Context, subject, sender, content string) (systemPrompt, userPrompt string, promptVersion int32) {
	systemPrompt, promptVersion = s.getPrompt(ctx, "triage", triagePromptTemplate)

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
	return systemPrompt, userPrompt, promptVersion
}

// triageResult holds the parsed triage response.
type triageResult struct {
	Category            string `json:"category"`
	Importance          string `json:"importance"`
	Reason              string `json:"reason"`
	ContentContribution string `json:"content_contribution"`
	ContributionReason  string `json:"contribution_reason"`
}

// parseTriageResponse parses the JSON response from the LLM.
// Returns the parsed triageResult and any parsing error.
func parseTriageResponse(jsonStr string) (*triageResult, error) {
	// Clean up the response
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	var result triageResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	// Validate the parsed result
	if err := validateTriageResult(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// validateTriageResult checks if the category, importance, and content_contribution are valid.
func validateTriageResult(result *triageResult) error {
	if !validCategories[result.Category] {
		return fmt.Errorf("invalid category: %s (must be one of PROJECT_UPDATE, CUSTOMER, RISK_ISSUE, ACTION_REQUEST, DECISION, INTERNAL_COMMS, PERSONAL, FYI)", result.Category)
	}
	if !validImportance[result.Importance] {
		return fmt.Errorf("invalid importance: %s (must be one of HIGH, MEDIUM, LOW)", result.Importance)
	}
	// content_contribution is optional; if present, must be valid
	if result.ContentContribution != "" && !validContribution[result.ContentContribution] {
		return fmt.Errorf("invalid content_contribution: %s (must be one of HIGH, MEDIUM, LOW, NONE)", result.ContentContribution)
	}
	return nil
}
