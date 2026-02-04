package server

import (
	"testing"
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
	content := "Dan Spataro is the CEO of CLIC. Meeting on January 15th."
	prompt := buildNERPrompt(content)

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

func TestBuildSemanticPrompt(t *testing.T) {
	content := "Alice should fix the bug by next week. We decided to approve the budget."
	prompt := buildSemanticPrompt(content)

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
	content := "There is a critical database outage affecting production."
	prompt := buildQualityGatePrompt(content)

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
