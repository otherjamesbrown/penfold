package server

import (
	"context"
	"testing"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

func TestParseNERResponse_Valid(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		expected *nerResult
	}{
		{
			name: "valid NER response with all fields",
			jsonStr: `{
				"people": [{"name": "Dan Spataro", "role": "CEO"}],
				"dates": [{"date": "January 15th", "context": "deadline"}],
				"projects": ["CLIC", "Project Alpha"],
				"organisations": ["Acme Corp", "Beta Team"]
			}`,
			wantErr: false,
			expected: &nerResult{
				People: []personResult{
					{Name: "Dan Spataro", Role: "CEO"},
				},
				Dates: []dateResult{
					{Date: "January 15th", Context: "deadline"},
				},
				Projects:      []string{"CLIC", "Project Alpha"},
				Organisations: []string{"Acme Corp", "Beta Team"},
			},
		},
		{
			name: "valid NER response with empty arrays",
			jsonStr: `{
				"people": [],
				"dates": [],
				"projects": [],
				"organisations": []
			}`,
			wantErr: false,
			expected: &nerResult{
				People:        []personResult{},
				Dates:         []dateResult{},
				Projects:      []string{},
				Organisations: []string{},
			},
		},
		{
			name: "valid NER response with markdown code blocks",
			jsonStr: "```json\n" + `{
				"people": [{"name": "Alice", "role": "Engineer"}],
				"dates": [],
				"projects": ["Beta"],
				"organisations": []
			}` + "\n```",
			wantErr: false,
			expected: &nerResult{
				People: []personResult{
					{Name: "Alice", Role: "Engineer"},
				},
				Dates:         []dateResult{},
				Projects:      []string{"Beta"},
				Organisations: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseNERResponse(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNERResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(result.People) != len(tt.expected.People) {
					t.Errorf("People count = %d, want %d", len(result.People), len(tt.expected.People))
				}
				if len(result.Dates) != len(tt.expected.Dates) {
					t.Errorf("Dates count = %d, want %d", len(result.Dates), len(tt.expected.Dates))
				}
				if len(result.Projects) != len(tt.expected.Projects) {
					t.Errorf("Projects count = %d, want %d", len(result.Projects), len(tt.expected.Projects))
				}
				if len(result.Organisations) != len(tt.expected.Organisations) {
					t.Errorf("Organisations count = %d, want %d", len(result.Organisations), len(tt.expected.Organisations))
				}
			}
		})
	}
}

func TestParseNERResponse_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
	}{
		{
			name:    "invalid JSON",
			jsonStr: `{invalid json`,
		},
		{
			name:    "empty string",
			jsonStr: "",
		},
		{
			name:    "not JSON",
			jsonStr: "This is not JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseNERResponse(tt.jsonStr)
			if err == nil {
				t.Errorf("parseNERResponse() expected error for invalid input, got nil")
			}
		})
	}
}

func TestParseSemanticResponse_Valid(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		expected *semanticResult
	}{
		{
			name: "valid semantic response with all fields",
			jsonStr: `{
				"action_items": [{"assignee": "Alice", "action": "Fix bug", "due": "next week"}],
				"decisions": ["Approved budget", "Hired new engineer"],
				"risks": ["Server downtime risk", "Budget overrun"]
			}`,
			wantErr: false,
			expected: &semanticResult{
				ActionItems: []actionItemResult{
					{Assignee: "Alice", Action: "Fix bug", Due: "next week"},
				},
				Decisions: []string{"Approved budget", "Hired new engineer"},
				Risks:     []string{"Server downtime risk", "Budget overrun"},
			},
		},
		{
			name: "valid semantic response with empty arrays",
			jsonStr: `{
				"action_items": [],
				"decisions": [],
				"risks": []
			}`,
			wantErr: false,
			expected: &semanticResult{
				ActionItems: []actionItemResult{},
				Decisions:   []string{},
				Risks:       []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSemanticResponse(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSemanticResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(result.ActionItems) != len(tt.expected.ActionItems) {
					t.Errorf("ActionItems count = %d, want %d", len(result.ActionItems), len(tt.expected.ActionItems))
				}
				if len(result.Decisions) != len(tt.expected.Decisions) {
					t.Errorf("Decisions count = %d, want %d", len(result.Decisions), len(tt.expected.Decisions))
				}
				if len(result.Risks) != len(tt.expected.Risks) {
					t.Errorf("Risks count = %d, want %d", len(result.Risks), len(tt.expected.Risks))
				}
			}
		})
	}
}

func TestParseSemanticResponse_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
	}{
		{
			name:    "invalid JSON",
			jsonStr: `{invalid json`,
		},
		{
			name:    "empty string",
			jsonStr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSemanticResponse(tt.jsonStr)
			if err == nil {
				t.Errorf("parseSemanticResponse() expected error for invalid input, got nil")
			}
		})
	}
}

func TestParseQualityGateResponse_Valid(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		expected *qualityGateResult
	}{
		{
			name: "valid quality gate response",
			jsonStr: `{
				"risks": [
					{"description": "Database outage", "severity_hint": "high", "owner_hint": "DevOps team"},
					{"description": "Budget overrun", "severity_hint": "medium", "owner_hint": "Finance"}
				]
			}`,
			wantErr: false,
			expected: &qualityGateResult{
				Risks: []riskResult{
					{Description: "Database outage", SeverityHint: "high", OwnerHint: "DevOps team"},
					{Description: "Budget overrun", SeverityHint: "medium", OwnerHint: "Finance"},
				},
			},
		},
		{
			name: "valid quality gate response with empty risks",
			jsonStr: `{
				"risks": []
			}`,
			wantErr: false,
			expected: &qualityGateResult{
				Risks: []riskResult{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseQualityGateResponse(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseQualityGateResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(result.Risks) != len(tt.expected.Risks) {
					t.Errorf("Risks count = %d, want %d", len(result.Risks), len(tt.expected.Risks))
				}
			}
		})
	}
}

func TestBuildNERPrompt(t *testing.T) {
	s := &AIServer{} // no promptStore — uses hardcoded fallback
	content := "Dan Spataro is the CEO of CLIC. Meeting on January 15th."
	prompt, _, version := s.buildNERPrompt(context.Background(), content, "", 0)

	if version != 0 {
		t.Errorf("buildNERPrompt() version = %d, want 0 (hardcoded fallback)", version)
	}
	if !containsString(prompt, content) {
		t.Errorf("buildNERPrompt() should contain the content")
	}
	if !containsString(prompt, "People mentioned") {
		t.Errorf("buildNERPrompt() should contain NER instructions")
	}
	if !containsString(prompt, "Respond ONLY with JSON") {
		t.Errorf("buildNERPrompt() should contain JSON instruction")
	}
}

// TestBuildNERPrompt_BackgroundContext verifies that buildNERPrompt injects
// background context (glossary + topic) when provided (pf-2f0d4b).
func TestBuildNERPrompt_BackgroundContext(t *testing.T) {
	s := &AIServer{}
	content := "Dan Spataro is the CEO of CLIC. Meeting on January 15th."
	bgCtx := "### Glossary\nCLIC: Community Lending Investment Company\n\n### Topic Context\nDevCloud: Internal cloud platform"
	prompt, _, _ := s.buildNERPrompt(context.Background(), content, bgCtx, 0)

	if !containsString(prompt, "## Background Context") {
		t.Errorf("buildNERPrompt() with backgroundContext should contain '## Background Context' header")
	}
	if !containsString(prompt, "CLIC: Community Lending Investment Company") {
		t.Errorf("buildNERPrompt() with backgroundContext should contain glossary content")
	}
	if !containsString(prompt, "DevCloud: Internal cloud platform") {
		t.Errorf("buildNERPrompt() with backgroundContext should contain topic context")
	}
	if !containsString(prompt, content) {
		t.Errorf("buildNERPrompt() with backgroundContext should still contain the original content")
	}
}

func TestBuildSemanticPrompt(t *testing.T) {
	s := &AIServer{} // no promptStore — uses hardcoded fallback
	content := "Alice should fix the bug by next week. We decided to approve the budget."
	prompt, _, version := s.buildSemanticPrompt(context.Background(), content, "", 0)

	if version != 0 {
		t.Errorf("buildSemanticPrompt() version = %d, want 0 (hardcoded fallback)", version)
	}
	if !containsString(prompt, content) {
		t.Errorf("buildSemanticPrompt() should contain the content")
	}
	if !containsString(prompt, "action items") {
		t.Errorf("buildSemanticPrompt() should contain semantic instructions")
	}
	if !containsString(prompt, "Respond ONLY with JSON") {
		t.Errorf("buildSemanticPrompt() should contain JSON instruction")
	}
}

func TestBuildQualityGatePrompt(t *testing.T) {
	s := &AIServer{} // no promptStore — uses hardcoded fallback
	content := "There is a critical database outage affecting production."
	prompt, _, version := s.buildQualityGatePrompt(context.Background(), content)

	if version != 0 {
		t.Errorf("buildQualityGatePrompt() version = %d, want 0 (hardcoded fallback)", version)
	}
	if !containsString(prompt, content) {
		t.Errorf("buildQualityGatePrompt() should contain the content")
	}
	if !containsString(prompt, "risks or issues") {
		t.Errorf("buildQualityGatePrompt() should contain risk-focused instructions")
	}
	if !containsString(prompt, "Respond ONLY with JSON") {
		t.Errorf("buildQualityGatePrompt() should contain JSON instruction")
	}
}

func TestStripEmailAddressesKeepSubject_FullHeaders(t *testing.T) {
	content := "EMAIL METADATA:\nFrom: Ponec, Miroslav <mponec@akamai.com>\nTo: Dunn, Tim <tdunn@akamai.com>; DeMent, James <jdement@akamai.com>\nCC: Weisman, Sara <sweisman@akamai.com>\nSubject: Re: GPU requirements\n---\nBODY:\nI have concerns about the approach."

	result := stripEmailAddressesKeepSubject(content)

	if containsString(result, "mponec@akamai.com") {
		t.Error("should not contain email addresses")
	}
	if containsString(result, "From:") {
		t.Error("should not contain From line")
	}
	if containsString(result, "To:") {
		t.Error("should not contain To line")
	}
	if containsString(result, "CC:") {
		t.Error("should not contain CC line")
	}
	if !containsString(result, "Subject: Re: GPU requirements") {
		t.Error("should preserve Subject line")
	}
	if !containsString(result, "I have concerns about the approach.") {
		t.Error("should preserve body content")
	}
}

func TestStripEmailAddressesKeepSubject_NoMetadataBlock(t *testing.T) {
	content := "Just a plain body without any email metadata."
	result := stripEmailAddressesKeepSubject(content)
	if result != content {
		t.Errorf("should return content unchanged, got %q", result)
	}
}

func TestStripEmailAddressesKeepSubject_NoSubject(t *testing.T) {
	content := "EMAIL METADATA:\nFrom: alice@example.com\nTo: bob@example.com\n---\nBODY:\nBody text here."
	result := stripEmailAddressesKeepSubject(content)

	if containsString(result, "alice@example.com") {
		t.Error("should not contain email addresses")
	}
	if !containsString(result, "Body text here.") {
		t.Error("should preserve body content")
	}
	if containsString(result, "Subject:") {
		t.Error("should not have Subject line when none was present")
	}
}

func TestBuildSemanticPrompt_StripsEmailAddresses(t *testing.T) {
	s := &AIServer{}
	content := "EMAIL METADATA:\nFrom: Alice <alice@example.com>\nTo: Bob <bob@example.com>\nSubject: Budget review\n---\nBODY:\nWe need to approve the budget."

	prompt, _, _ := s.buildSemanticPrompt(context.Background(), content, "", 0)

	if containsString(prompt, "alice@example.com") {
		t.Error("semantic prompt should not contain email addresses")
	}
	if !containsString(prompt, "Subject: Budget review") {
		t.Error("semantic prompt should contain Subject line")
	}
	if !containsString(prompt, "We need to approve the budget.") {
		t.Error("semantic prompt should contain body")
	}
}

// Helper function to check if a string contains a substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && contains(s, substr)))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestExtractEntities_StagesGating verifies that the AI coordinator skips semantic
// extraction when the pipeline stages don't include extract_semantic (pf-83a646).
func TestExtractEntities_StagesGating(t *testing.T) {
	tests := []struct {
		name      string
		stages    []string
		wantCalls int
	}{
		{
			name:      "empty stages runs both NER and semantic (backward compat)",
			stages:    nil,
			wantCalls: 2,
		},
		{
			name:      "both stages listed runs both",
			stages:    []string{"extract_ner", "extract_semantic"},
			wantCalls: 2,
		},
		{
			name:      "only extract_ner skips semantic",
			stages:    []string{"extract_ner"},
			wantCalls: 1,
		},
		{
			name:      "extract_ner with unrelated stage still skips semantic",
			stages:    []string{"extract_ner", "summarize"},
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			mb := &mockBackend{
				chatCompletionFunc: func(_ context.Context, _ []backend.Message, _ backend.CompletionOptions) (*backend.CompletionResult, error) {
					calls++
					if calls == 1 {
						return &backend.CompletionResult{
							Content:      `{"people":[],"dates":[],"projects":[],"organisations":[]}`,
							Model:        "test-model",
							InputTokens:  10,
							OutputTokens: 5,
						}, nil
					}
					return &backend.CompletionResult{
						Content:      `{"action_items":[],"decisions":[],"risks":[]}`,
						Model:        "test-model",
						InputTokens:  10,
						OutputTokens: 5,
					}, nil
				},
			}
			srv := NewAIServer(testConfig(), logging.NewNopLogger(), mb)

			resp, err := srv.ExtractEntities(context.Background(), &aiv1.ExtractEntitiesRequest{
				Content: "Test content with people and decisions",
				Stages:  tt.stages,
			})
			if err != nil {
				t.Fatalf("ExtractEntities returned error: %v", err)
			}
			if calls != tt.wantCalls {
				t.Errorf("expected %d ChatCompletion calls, got %d", tt.wantCalls, calls)
			}
			if resp == nil {
				t.Fatal("response is nil")
			}
		})
	}
}
