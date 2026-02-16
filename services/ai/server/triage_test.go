package server

import (
	"strings"
	"testing"
)

func TestParseTriageResponse_Valid(t *testing.T) {
	tests := []struct {
		name       string
		jsonStr    string
		wantCat    string
		wantImp    string
		wantReason string
	}{
		{
			name:       "valid PROJECT_UPDATE",
			jsonStr:    `{"category": "PROJECT_UPDATE", "importance": "HIGH", "reason": "Sprint planning meeting notes"}`,
			wantCat:    "PROJECT_UPDATE",
			wantImp:    "HIGH",
			wantReason: "Sprint planning meeting notes",
		},
		{
			name:       "valid CUSTOMER with MEDIUM importance",
			jsonStr:    `{"category": "CUSTOMER", "importance": "MEDIUM", "reason": "Customer asking about pricing"}`,
			wantCat:    "CUSTOMER",
			wantImp:    "MEDIUM",
			wantReason: "Customer asking about pricing",
		},
		{
			name:       "valid PERSONAL with LOW importance",
			jsonStr:    `{"category": "PERSONAL", "importance": "LOW", "reason": "Lunch invitation"}`,
			wantCat:    "PERSONAL",
			wantImp:    "LOW",
			wantReason: "Lunch invitation",
		},
		{
			name:       "json with code fences",
			jsonStr:    "```json\n{\"category\": \"RISK_ISSUE\", \"importance\": \"HIGH\", \"reason\": \"Critical security vulnerability\"}\n```",
			wantCat:    "RISK_ISSUE",
			wantImp:    "HIGH",
			wantReason: "Critical security vulnerability",
		},
		{
			name:       "json with whitespace",
			jsonStr:    "  \n  {\"category\": \"DECISION\", \"importance\": \"MEDIUM\", \"reason\": \"Architecture decision required\"}  \n  ",
			wantCat:    "DECISION",
			wantImp:    "MEDIUM",
			wantReason: "Architecture decision required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTriageResponse(tt.jsonStr)
			if err != nil {
				t.Fatalf("parseTriageResponse() error = %v, want nil", err)
			}
			if result.Category != tt.wantCat {
				t.Errorf("category = %v, want %v", result.Category, tt.wantCat)
			}
			if result.Importance != tt.wantImp {
				t.Errorf("importance = %v, want %v", result.Importance, tt.wantImp)
			}
			if result.Reason != tt.wantReason {
				t.Errorf("reason = %v, want %v", result.Reason, tt.wantReason)
			}
		})
	}
}

func TestParseTriageResponse_MalformedJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
	}{
		{
			name:    "not JSON",
			jsonStr: "This is not JSON",
		},
		{
			name:    "incomplete JSON",
			jsonStr: `{"category": "PROJECT_UPDATE"`,
		},
		{
			name:    "invalid JSON syntax",
			jsonStr: `{category: PROJECT_UPDATE, importance: HIGH}`,
		},
		{
			name:    "empty string",
			jsonStr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTriageResponse(tt.jsonStr)
			if err == nil {
				t.Errorf("parseTriageResponse() error = nil, want error for malformed JSON")
			}
		})
	}
}

func TestParseTriageResponse_UnknownCategory(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantErr  string
	}{
		{
			name:     "invalid category",
			jsonStr:  `{"category": "UNKNOWN_CATEGORY", "importance": "HIGH", "reason": "test"}`,
			wantErr:  "invalid category",
		},
		{
			name:     "empty category",
			jsonStr:  `{"category": "", "importance": "HIGH", "reason": "test"}`,
			wantErr:  "invalid category",
		},
		{
			name:     "lowercase category",
			jsonStr:  `{"category": "customer", "importance": "HIGH", "reason": "test"}`,
			wantErr:  "invalid category",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTriageResponse(tt.jsonStr)
			if err == nil {
				t.Errorf("parseTriageResponse() error = nil, want error containing %q", tt.wantErr)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseTriageResponse() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseTriageResponse_InvalidImportance(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantErr  string
	}{
		{
			name:     "invalid importance",
			jsonStr:  `{"category": "PROJECT_UPDATE", "importance": "CRITICAL", "reason": "test"}`,
			wantErr:  "invalid importance",
		},
		{
			name:     "empty importance",
			jsonStr:  `{"category": "PROJECT_UPDATE", "importance": "", "reason": "test"}`,
			wantErr:  "invalid importance",
		},
		{
			name:     "lowercase importance",
			jsonStr:  `{"category": "PROJECT_UPDATE", "importance": "high", "reason": "test"}`,
			wantErr:  "invalid importance",
		},
		{
			name:     "numeric importance",
			jsonStr:  `{"category": "PROJECT_UPDATE", "importance": "1", "reason": "test"}`,
			wantErr:  "invalid importance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTriageResponse(tt.jsonStr)
			if err == nil {
				t.Errorf("parseTriageResponse() error = nil, want error containing %q", tt.wantErr)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseTriageResponse() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestBuildTriagePrompt(t *testing.T) {
	tests := []struct {
		name        string
		subject     string
		sender      string
		content     string
		wantSystem  string
		wantUserHas []string // substrings that should be in user prompt
	}{
		{
			name:        "all fields provided",
			subject:     "Sprint Planning",
			sender:      "alice@example.com",
			content:     "Let's meet to discuss the next sprint.",
			wantSystem:  triagePromptTemplate,
			wantUserHas: []string{"Subject: Sprint Planning", "From: alice@example.com", "Let's meet to discuss the next sprint."},
		},
		{
			name:        "only content",
			subject:     "",
			sender:      "",
			content:     "Quick update on the project.",
			wantSystem:  triagePromptTemplate,
			wantUserHas: []string{"Quick update on the project."},
		},
		{
			name:        "truncate long content",
			subject:     "Test",
			sender:      "test@example.com",
			content:     strings.Repeat("a", 600),
			wantSystem:  triagePromptTemplate,
			wantUserHas: []string{"Subject: Test", "From: test@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			system, user := buildTriagePrompt(tt.subject, tt.sender, tt.content)
			if system != tt.wantSystem {
				t.Errorf("system prompt mismatch")
			}
			for _, want := range tt.wantUserHas {
				if !strings.Contains(user, want) {
					t.Errorf("user prompt missing %q, got: %s", want, user)
				}
			}

			// Check truncation
			if len(tt.content) > 500 {
				// User prompt should not contain the full content
				if strings.Contains(user, strings.Repeat("a", 600)) {
					t.Errorf("content was not truncated")
				}
			}
		})
	}
}

func TestBuildTriagePrompt_Truncation(t *testing.T) {
	longContent := strings.Repeat("x", 600)
	_, user := buildTriagePrompt("Subject", "sender@example.com", longContent)

	// The user prompt should contain the truncated content (500 chars), not the full 600
	if strings.Contains(user, strings.Repeat("x", 600)) {
		t.Errorf("content should be truncated to 500 chars, but full content found")
	}
	if !strings.Contains(user, strings.Repeat("x", 500)) {
		t.Errorf("truncated content (500 chars) should be present in user prompt")
	}
}

func TestValidateTriageResult(t *testing.T) {
	tests := []struct {
		name       string
		category   string
		importance string
		wantErr    bool
	}{
		{
			name:       "valid PROJECT_UPDATE HIGH",
			category:   "PROJECT_UPDATE",
			importance: "HIGH",
			wantErr:    false,
		},
		{
			name:       "valid CUSTOMER MEDIUM",
			category:   "CUSTOMER",
			importance: "MEDIUM",
			wantErr:    false,
		},
		{
			name:       "valid OTHER LOW",
			category:   "OTHER",
			importance: "LOW",
			wantErr:    false,
		},
		{
			name:       "invalid category",
			category:   "INVALID",
			importance: "HIGH",
			wantErr:    true,
		},
		{
			name:       "invalid importance",
			category:   "PROJECT_UPDATE",
			importance: "CRITICAL",
			wantErr:    true,
		},
		{
			name:       "both invalid",
			category:   "INVALID",
			importance: "CRITICAL",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &triageResult{
				Category:   tt.category,
				Importance: tt.importance,
			}
			err := validateTriageResult(result)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTriageResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestParseTriageResponse_WithContentContribution tests parsing when content_contribution field is present.
// This verifies acceptance criteria: parser handles new content_contribution field.
func TestParseTriageResponse_WithContentContribution(t *testing.T) {
	tests := []struct {
		name               string
		jsonStr            string
		wantCat            string
		wantImp            string
		wantReason         string
		wantContribution   string
		wantContribReason  string
	}{
		{
			name: "HIGH contribution",
			jsonStr: `{
				"category": "PROJECT_UPDATE",
				"importance": "HIGH",
				"reason": "Sprint planning notes",
				"content_contribution": "HIGH",
				"contribution_reason": "Contains actionable decisions and updates"
			}`,
			wantCat:           "PROJECT_UPDATE",
			wantImp:           "HIGH",
			wantReason:        "Sprint planning notes",
			wantContribution:  "HIGH",
			wantContribReason: "Contains actionable decisions and updates",
		},
		{
			name: "MEDIUM contribution",
			jsonStr: `{
				"category": "CUSTOMER",
				"importance": "MEDIUM",
				"reason": "Customer inquiry",
				"content_contribution": "MEDIUM",
				"contribution_reason": "Useful customer feedback"
			}`,
			wantCat:           "CUSTOMER",
			wantImp:           "MEDIUM",
			wantReason:        "Customer inquiry",
			wantContribution:  "MEDIUM",
			wantContribReason: "Useful customer feedback",
		},
		{
			name: "LOW contribution",
			jsonStr: `{
				"category": "INTERNAL_COMMS",
				"importance": "LOW",
				"reason": "Company newsletter",
				"content_contribution": "LOW",
				"contribution_reason": "Generic information, not actionable"
			}`,
			wantCat:           "INTERNAL_COMMS",
			wantImp:           "LOW",
			wantReason:        "Company newsletter",
			wantContribution:  "LOW",
			wantContribReason: "Generic information, not actionable",
		},
		{
			name: "NONE contribution",
			jsonStr: `{
				"category": "PERSONAL",
				"importance": "LOW",
				"reason": "Social chat",
				"content_contribution": "NONE",
				"contribution_reason": "No knowledge value"
			}`,
			wantCat:           "PERSONAL",
			wantImp:           "LOW",
			wantReason:        "Social chat",
			wantContribution:  "NONE",
			wantContribReason: "No knowledge value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTriageResponse(tt.jsonStr)
			if err != nil {
				t.Fatalf("parseTriageResponse() error = %v, want nil", err)
			}
			if result.Category != tt.wantCat {
				t.Errorf("category = %v, want %v", result.Category, tt.wantCat)
			}
			if result.Importance != tt.wantImp {
				t.Errorf("importance = %v, want %v", result.Importance, tt.wantImp)
			}
			if result.Reason != tt.wantReason {
				t.Errorf("reason = %v, want %v", result.Reason, tt.wantReason)
			}
			// Verify contribution and contribution_reason fields
			if result.ContentContribution != tt.wantContribution {
				t.Errorf("content_contribution = %v, want %v", result.ContentContribution, tt.wantContribution)
			}
			if result.ContributionReason != tt.wantContribReason {
				t.Errorf("contribution_reason = %v, want %v", result.ContributionReason, tt.wantContribReason)
			}
		})
	}
}

// TestParseTriageResponse_MissingContentContribution verifies backward compatibility.
// This verifies acceptance criteria: if triage fails to return content_contribution, system still works.
func TestParseTriageResponse_MissingContentContribution(t *testing.T) {
	tests := []struct {
		name       string
		jsonStr    string
		wantCat    string
		wantImp    string
		wantReason string
	}{
		{
			name:       "no contribution fields - existing format",
			jsonStr:    `{"category": "PROJECT_UPDATE", "importance": "HIGH", "reason": "Sprint planning"}`,
			wantCat:    "PROJECT_UPDATE",
			wantImp:    "HIGH",
			wantReason: "Sprint planning",
		},
		{
			name: "only contribution_reason, no content_contribution",
			jsonStr: `{
				"category": "CUSTOMER",
				"importance": "MEDIUM",
				"reason": "Customer inquiry",
				"contribution_reason": "Has some value"
			}`,
			wantCat:    "CUSTOMER",
			wantImp:    "MEDIUM",
			wantReason: "Customer inquiry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTriageResponse(tt.jsonStr)
			if err != nil {
				t.Fatalf("parseTriageResponse() error = %v, want nil (should handle missing contribution gracefully)", err)
			}
			if result.Category != tt.wantCat {
				t.Errorf("category = %v, want %v", result.Category, tt.wantCat)
			}
			if result.Importance != tt.wantImp {
				t.Errorf("importance = %v, want %v", result.Importance, tt.wantImp)
			}
			if result.Reason != tt.wantReason {
				t.Errorf("reason = %v, want %v", result.Reason, tt.wantReason)
			}
			// Verify that missing content_contribution returns empty string (activity will default to HIGH)
			if result.ContentContribution != "" {
				t.Errorf("content_contribution = %v, want empty string when missing", result.ContentContribution)
			}
		})
	}
}

// TestParseTriageResponse_InvalidContentContribution tests validation of content_contribution values.
func TestParseTriageResponse_InvalidContentContribution(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr string
	}{
		{
			name: "invalid contribution value",
			jsonStr: `{
				"category": "PROJECT_UPDATE",
				"importance": "HIGH",
				"reason": "test",
				"content_contribution": "CRITICAL"
			}`,
			wantErr: "invalid content_contribution",
		},
		{
			name: "lowercase contribution",
			jsonStr: `{
				"category": "PROJECT_UPDATE",
				"importance": "HIGH",
				"reason": "test",
				"content_contribution": "high"
			}`,
			wantErr: "invalid content_contribution",
		},
		{
			name: "numeric contribution",
			jsonStr: `{
				"category": "PROJECT_UPDATE",
				"importance": "HIGH",
				"reason": "test",
				"content_contribution": "1"
			}`,
			wantErr: "invalid content_contribution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTriageResponse(tt.jsonStr)
			// Verify validation rejects invalid content_contribution
			if err == nil {
				t.Errorf("parseTriageResponse() error = nil, want error containing %q", tt.wantErr)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseTriageResponse() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
