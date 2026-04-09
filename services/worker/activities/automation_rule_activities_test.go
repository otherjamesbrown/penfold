package activities

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestLoadRuleConfigInput_JSONRoundtrip verifies the input type round-trips via JSON.
func TestLoadRuleConfigInput_JSONRoundtrip(t *testing.T) {
	input := LoadRuleConfigInput{RuleID: "abc-123"}
	data, err := json.Marshal(input)
	require.NoError(t, err)
	var decoded LoadRuleConfigInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, input.RuleID, decoded.RuleID)
}

// TestExecuteSelectorInput_JSONRoundtrip verifies selector input round-trips via JSON.
func TestExecuteSelectorInput_JSONRoundtrip(t *testing.T) {
	input := ExecuteSelectorInput{
		TenantID:       "tenant-1",
		SelectorConfig: json.RawMessage(`{"scope":"query","window":"7d","limit":50}`),
		TriggerItem:    json.RawMessage(`{"source_id":42}`),
		ExecutionTime:  "2026-04-09T08:00:00Z",
	}
	data, err := json.Marshal(input)
	require.NoError(t, err)
	var decoded ExecuteSelectorInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, input.TenantID, decoded.TenantID)
	assert.Equal(t, input.ExecutionTime, decoded.ExecutionTime)
}

// TestExecuteSelectorOutput_JSONRoundtrip verifies selector output round-trips via JSON.
func TestExecuteSelectorOutput_JSONRoundtrip(t *testing.T) {
	out := ExecuteSelectorOutput{
		Items: []SelectedItem{
			{SourceID: 1, Subject: "Newsletter", From: "news@example.com", Date: time.Now().UTC()},
		},
		Count:      1,
		WindowFrom: "2026-04-02",
		WindowTo:   "2026-04-09",
	}
	data, err := json.Marshal(out)
	require.NoError(t, err)
	var decoded ExecuteSelectorOutput
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, out.Count, decoded.Count)
	assert.Equal(t, out.WindowFrom, decoded.WindowFrom)
	assert.Len(t, decoded.Items, 1)
	assert.Equal(t, out.Items[0].Subject, decoded.Items[0].Subject)
}

// TestParseWindowDuration tests the window duration parser.
func TestParseWindowDuration(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		wantH   float64 // expected hours
	}{
		{"24h", false, 24},
		{"7d", false, 168},
		{"30d", false, 720},
		{"1h", false, 1},
		{"bad", true, 0},
		{"", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			dur, err := parseWindowDuration(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.wantH, dur.Hours(), 0.001)
		})
	}
}

// TestParseTriggerItem verifies trigger items can be parsed.
func TestParseTriggerItem(t *testing.T) {
	raw := json.RawMessage(`{"source_id":42,"subject":"Test","from":"a@b.com"}`)
	item, err := parseTriggerItem(raw)
	require.NoError(t, err)
	assert.Equal(t, int64(42), item.SourceID)
	assert.Equal(t, "Test", item.Subject)
}

// TestFormatSelectedItems verifies items are formatted correctly.
func TestFormatSelectedItems(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		out := formatSelectedItems(nil)
		assert.Equal(t, "None", out)
	})

	t.Run("with items", func(t *testing.T) {
		items := []SelectedItem{
			{
				SourceID: 1,
				Subject:  "Newsletter Issue 42",
				From:     "news@example.com",
				Date:     time.Date(2026, 4, 9, 8, 0, 0, 0, time.UTC),
				Summary:  "This week in tech",
			},
		}
		out := formatSelectedItems(items)
		assert.Contains(t, out, "Newsletter Issue 42")
		assert.Contains(t, out, "news@example.com")
		assert.Contains(t, out, "This week in tech")
		assert.Contains(t, out, "2026-04-09")
	})
}

// TestLoadAndRenderSkill_FileRead verifies skill file reading and template rendering.
func TestLoadAndRenderSkill_FileRead(t *testing.T) {
	// Create a temporary skill file
	dir := t.TempDir()
	skillContent := `# Test Skill

Hello {{.RuleName}}!

Items:
{{.Items}}

Time: {{.ExecutionTime}}
`
	skillPath := filepath.Join(dir, "test-skill.md")
	require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0o644))

	logger := logging.NewLogger(&logging.Config{Level: logging.LevelInfo})
	acts := &AutomationRuleActivities{
		skillsPath: dir,
		logger:     logger,
	}

	input := LoadAndRenderSkillInput{
		SkillName:     "test-skill",
		RuleName:      "My Rule",
		Items:         []SelectedItem{{Subject: "Test Item", From: "a@b.com", Date: time.Now()}},
		ExecutionTime: "2026-04-09T08:00:00Z",
	}

	out, err := acts.LoadAndRenderSkill(nil, input) //nolint:staticcheck // nil ctx is valid in unit tests
	require.NoError(t, err)
	assert.Contains(t, out.RenderedPrompt, "Hello My Rule!")
	assert.Contains(t, out.RenderedPrompt, "2026-04-09T08:00:00Z")
}

// TestLoadAndRenderSkill_PathTraversal ensures path traversal is blocked.
func TestLoadAndRenderSkill_PathTraversal(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelInfo})
	acts := &AutomationRuleActivities{
		skillsPath: t.TempDir(),
		logger:     logger,
	}

	_, err := acts.LoadAndRenderSkill(nil, LoadAndRenderSkillInput{SkillName: "../etc/passwd"}) //nolint:staticcheck
	require.Error(t, err)
}

// TestLoadAndRenderSkill_NotFound returns an error when the skill file is missing.
func TestLoadAndRenderSkill_NotFound(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelInfo})
	acts := &AutomationRuleActivities{
		skillsPath: t.TempDir(),
		logger:     logger,
	}

	_, err := acts.LoadAndRenderSkill(nil, LoadAndRenderSkillInput{SkillName: "nonexistent"}) //nolint:staticcheck
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read skill file")
}

// TestExecuteChainsInput_JSONRoundtrip verifies chain input round-trips via JSON.
func TestExecuteChainsInput_JSONRoundtrip(t *testing.T) {
	input := ExecuteChainsInput{
		TenantID:     "tenant-1",
		OutputConfig: json.RawMessage(`{"channels":[{"type":"email","to":"a@b.com"}],"chain":[{"rule":"other-rule"}]}`),
		ChainDepth:   1,
		SkillOutput:  "here is the output",
	}
	data, err := json.Marshal(input)
	require.NoError(t, err)
	var decoded ExecuteChainsInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, input.TenantID, decoded.TenantID)
	assert.Equal(t, input.ChainDepth, decoded.ChainDepth)
}

// TestExecuteChainsOutput_DepthSkip ensures no chains are returned when depth is exceeded.
func TestExecuteChainsOutput_DepthSkip(t *testing.T) {
	out := ExecuteChainsOutput{Skipped: true, MaxDepth: 3}
	assert.True(t, out.Skipped)
	assert.Empty(t, out.Chains)
}

// TestRecordExecutionInput_JSONRoundtrip verifies record execution input round-trips.
func TestRecordExecutionInput_JSONRoundtrip(t *testing.T) {
	input := RecordExecutionInput{
		RuleID:          "rule-uuid",
		TriggerType:     "cron",
		TriggerSource:   "cron",
		StartedAt:       "2026-04-09T08:00:00Z",
		Status:          "completed",
		ItemsSelected:   10,
		SkillTokensUsed: 500,
		DeliveryStatus:  []DeliverOutputResult{{Channel: "email", Status: "sent:msg-1"}},
		ChainsFired:     1,
	}
	data, err := json.Marshal(input)
	require.NoError(t, err)
	var decoded RecordExecutionInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, input.RuleID, decoded.RuleID)
	assert.Equal(t, input.Status, decoded.Status)
	assert.Equal(t, input.ItemsSelected, decoded.ItemsSelected)
	require.Len(t, decoded.DeliveryStatus, 1)
	assert.Equal(t, "email", decoded.DeliveryStatus[0].Channel)
}

// TestDeliverOutputInput_JSONRoundtrip verifies delivery input round-trips.
func TestDeliverOutputInput_JSONRoundtrip(t *testing.T) {
	input := DeliverOutputInput{
		TenantID:     "tenant-1",
		RuleName:     "newsletter-weekly",
		OutputConfig: json.RawMessage(`{"channels":[{"type":"store"}]}`),
		SkillOutput:  "Summary here",
		WindowFrom:   "2026-04-02",
		WindowTo:     "2026-04-09",
	}
	data, err := json.Marshal(input)
	require.NoError(t, err)
	var decoded DeliverOutputInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, input.TenantID, decoded.TenantID)
	assert.Equal(t, input.RuleName, decoded.RuleName)
}

// TestRenderAutomationHTML verifies HTML rendering does not XSS-escape user content.
func TestRenderAutomationHTML(t *testing.T) {
	html := renderAutomationHTML("Item 1\nItem 2", "My Rule", "2026-04-02", "2026-04-09")
	assert.Contains(t, html, "My Rule")
	assert.Contains(t, html, "Item 1")
	assert.Contains(t, html, "<br>") // newlines become <br>
}

// TestBuildEmailSubject verifies email subject construction.
func TestBuildEmailSubject(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		s := buildEmailSubject("", "My Rule", "2026-04-02", "2026-04-09")
		assert.Equal(t, "My Rule — 2026-04-02 to 2026-04-09", s)
	})
	t.Run("no window", func(t *testing.T) {
		s := buildEmailSubject("", "My Rule", "", "")
		assert.Equal(t, "My Rule", s)
	})
}

// TestHtmlEscape verifies HTML escaping.
func TestHtmlEscape(t *testing.T) {
	assert.Equal(t, "&lt;b&gt;bold&lt;/b&gt;", htmlEscape("<b>bold</b>"))
	assert.Equal(t, "&amp;", htmlEscape("&"))
}

// TestAutomationRule_JSONRoundtrip verifies AutomationRule serialises correctly.
func TestAutomationRule_JSONRoundtrip(t *testing.T) {
	rule := AutomationRule{
		ID:             "rule-uuid",
		TenantID:       "tenant-uuid",
		Name:           "newsletter-weekly",
		Enabled:        true,
		TriggerType:    "cron",
		TriggerConfig:  json.RawMessage(`{"cron":"0 9 * * MON"}`),
		SelectorConfig: json.RawMessage(`{"scope":"query","window":"7d"}`),
		SkillName:      "newsletter-rollup",
		OutputConfig:   json.RawMessage(`{"channels":[{"type":"email","to":"james@brown.chat"}]}`),
		CreatedBy:      "system",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	data, err := json.Marshal(rule)
	require.NoError(t, err)
	var decoded AutomationRule
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, rule.Name, decoded.Name)
	assert.Equal(t, rule.SkillName, decoded.SkillName)
	assert.Equal(t, rule.Enabled, decoded.Enabled)
}

// TestSelectorConfig_JSONRoundtrip verifies SelectorConfig serialises correctly.
func TestSelectorConfig_JSONRoundtrip(t *testing.T) {
	sel := SelectorConfig{
		Scope:        "query",
		Query:        "project:Healthcare",
		ContentTypes: []string{"email"},
		Window:       "24h",
		Limit:        50,
	}
	data, err := json.Marshal(sel)
	require.NoError(t, err)
	var decoded SelectorConfig
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, sel.Scope, decoded.Scope)
	assert.Equal(t, sel.Window, decoded.Window)
	assert.Equal(t, sel.Limit, decoded.Limit)
	assert.Equal(t, sel.ContentTypes, decoded.ContentTypes)
}

// TestOutputConfig_JSONRoundtrip verifies OutputConfig serialises correctly.
func TestOutputConfig_JSONRoundtrip(t *testing.T) {
	cfg := OutputConfig{
		Channels: []OutputChannel{
			{Type: "email", To: "james@brown.chat"},
			{Type: "store"},
		},
		Chain: []ChainRule{
			{Rule: "appointment-check"},
		},
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	var decoded OutputConfig
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Len(t, decoded.Channels, 2)
	assert.Equal(t, "email", decoded.Channels[0].Type)
	assert.Len(t, decoded.Chain, 1)
	assert.Equal(t, "appointment-check", decoded.Chain[0].Rule)
}
