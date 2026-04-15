package automation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTriggerConfigJSON verifies TriggerConfig round-trips through JSON.
func TestTriggerConfigJSON(t *testing.T) {
	t.Run("cron", func(t *testing.T) {
		cfg := TriggerConfig{Cron: "0 8 * * MON-FRI"}
		b, err := json.Marshal(cfg)
		require.NoError(t, err)

		var got TriggerConfig
		require.NoError(t, json.Unmarshal(b, &got))
		assert.Equal(t, cfg.Cron, got.Cron)
		assert.Nil(t, got.Match)
	})

	t.Run("event", func(t *testing.T) {
		cfg := TriggerConfig{
			Match: map[string]string{
				"project": "Healthcare",
				"urgency": "high",
			},
		}
		b, err := json.Marshal(cfg)
		require.NoError(t, err)

		var got TriggerConfig
		require.NoError(t, json.Unmarshal(b, &got))
		assert.Empty(t, got.Cron)
		assert.Equal(t, "Healthcare", got.Match["project"])
		assert.Equal(t, "high", got.Match["urgency"])
	})
}

// TestSelectorConfigJSON verifies SelectorConfig round-trips through JSON.
func TestSelectorConfigJSON(t *testing.T) {
	cases := []struct {
		name string
		cfg  SelectorConfig
	}{
		{
			name: "trigger_item",
			cfg:  SelectorConfig{Scope: "trigger_item"},
		},
		{
			name: "query",
			cfg: SelectorConfig{
				Scope:        "query",
				Query:        "project:Healthcare",
				ContentTypes: []string{"email"},
				Window:       "24h",
				Limit:        50,
				Sort:         "date",
			},
		},
		{
			name: "combined",
			cfg: SelectorConfig{
				Scope:       "combined",
				TriggerItem: true,
				Query:       "project:Healthcare",
				Window:      "7d",
				Limit:       20,
			},
		},
		{
			name: "none",
			cfg:  SelectorConfig{Scope: "none"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.cfg)
			require.NoError(t, err)

			var got SelectorConfig
			require.NoError(t, json.Unmarshal(b, &got))
			assert.Equal(t, tc.cfg, got)
		})
	}
}

// TestOutputConfigJSON verifies OutputConfig round-trips through JSON.
func TestOutputConfigJSON(t *testing.T) {
	cfg := OutputConfig{
		Channels: []OutputChannel{
			{
				Type:            "email",
				To:              "james@brown.chat",
				SubjectTemplate: "Healthcare Summary — {{.Date}}",
			},
			{
				Type:        "store",
				ContentType: "digest",
			},
		},
		Chain: []ChainedRule{
			{
				Rule: "calendar-conflict-check",
				SelectorOverride: &SelectorConfig{
					Scope: "trigger_item",
				},
			},
		},
	}

	b, err := json.Marshal(cfg)
	require.NoError(t, err)

	var got OutputConfig
	require.NoError(t, json.Unmarshal(b, &got))
	require.Len(t, got.Channels, 2)
	assert.Equal(t, "email", got.Channels[0].Type)
	assert.Equal(t, "james@brown.chat", got.Channels[0].To)
	assert.Equal(t, "store", got.Channels[1].Type)
	require.Len(t, got.Chain, 1)
	assert.Equal(t, "calendar-conflict-check", got.Chain[0].Rule)
	require.NotNil(t, got.Chain[0].SelectorOverride)
	assert.Equal(t, "trigger_item", got.Chain[0].SelectorOverride.Scope)
}

// TestListRuleOpts_Enabled verifies the Enabled pointer semantics.
func TestListRuleOpts_Enabled(t *testing.T) {
	t.Run("nil means no filter", func(t *testing.T) {
		opts := ListRuleOpts{}
		assert.Nil(t, opts.Enabled)
	})

	t.Run("false filters to disabled", func(t *testing.T) {
		v := false
		opts := ListRuleOpts{Enabled: &v}
		assert.NotNil(t, opts.Enabled)
		assert.False(t, *opts.Enabled)
	})

	t.Run("true filters to enabled", func(t *testing.T) {
		v := true
		opts := ListRuleOpts{Enabled: &v}
		require.NotNil(t, opts.Enabled)
		assert.True(t, *opts.Enabled)
	})
}

// TestAutomationRule_Fields verifies AutomationRule struct field types are correct.
func TestAutomationRule_Fields(t *testing.T) {
	tenantID := uuid.New()
	ruleID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rule := &AutomationRule{
		ID:          ruleID,
		TenantID:    tenantID,
		Name:        "healthcare-morning-summary",
		Description: "Daily healthcare summary",
		Enabled:     true,
		TriggerType: "cron",
		TriggerConfig: TriggerConfig{
			Cron: "0 8 * * MON-FRI",
		},
		SelectorConfig: SelectorConfig{
			Scope:        "query",
			Query:        "project:Healthcare",
			ContentTypes: []string{"email"},
			Window:       "24h",
			Limit:        50,
		},
		SkillName: "healthcare-daily-summary",
		OutputConfig: OutputConfig{
			Channels: []OutputChannel{
				{Type: "email", To: "james@brown.chat"},
				{Type: "store"},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "agent-mycroft",
	}

	assert.Equal(t, ruleID, rule.ID)
	assert.Equal(t, tenantID, rule.TenantID)
	assert.Equal(t, "cron", rule.TriggerType)
	assert.Equal(t, "0 8 * * MON-FRI", rule.TriggerConfig.Cron)
	assert.Equal(t, "query", rule.SelectorConfig.Scope)
	assert.Len(t, rule.OutputConfig.Channels, 2)
}

// TestAutomationRuleExecution_Fields verifies AutomationRuleExecution struct field types.
func TestAutomationRuleExecution_Fields(t *testing.T) {
	ruleID := uuid.New()
	execID := uuid.New()
	startedAt := time.Now().UTC().Truncate(time.Second)
	completedAt := startedAt.Add(30 * time.Second)

	itemsSelected := 10
	tokensUsed := 1500

	exec := &AutomationRuleExecution{
		ID:            execID,
		RuleID:        ruleID,
		TriggerType:   "cron",
		TriggerSource: "cron",
		StartedAt:     startedAt,
		CompletedAt:   &completedAt,
		Status:        "completed",
		ItemsSelected: &itemsSelected,
		SkillTokensUsed: &tokensUsed,
		DeliveryStatus: []DeliveryResult{
			{Channel: "email", Status: "sent:msg-abc"},
		},
		ChainExecutions: json.RawMessage(`{"chains_fired":0}`),
		Error:           "",
		CreatedAt:       startedAt,
	}

	assert.Equal(t, execID, exec.ID)
	assert.Equal(t, ruleID, exec.RuleID)
	assert.Equal(t, "completed", exec.Status)
	assert.Equal(t, 10, *exec.ItemsSelected)
	assert.Equal(t, 1500, *exec.SkillTokensUsed)
	assert.NotNil(t, exec.CompletedAt)
	require.Len(t, exec.DeliveryStatus, 1)
	assert.Equal(t, "email", exec.DeliveryStatus[0].Channel)
	assert.Equal(t, "sent:msg-abc", exec.DeliveryStatus[0].Status)
	assert.NotEmpty(t, exec.ChainExecutions)
}

// TestDeliveryStatus_GoldenRow verifies that delivery_status JSON written by the worker
// (a JSON array of {channel, status} objects) round-trips through the read model without error.
// This is the canonical shape stored in automation_rule_executions.delivery_status.
func TestDeliveryStatus_GoldenRow(t *testing.T) {
	// Golden JSON — exactly what RecordExecution activity writes to the DB.
	goldenJSON := `[{"channel":"store","status":"ok:d5e2f1"},{"channel":"email","status":"sent:msg-abc"}]`

	var ds []DeliveryResult
	require.NoError(t, json.Unmarshal([]byte(goldenJSON), &ds), "must unmarshal array into []DeliveryResult")
	require.Len(t, ds, 2)
	assert.Equal(t, "store", ds[0].Channel)
	assert.Equal(t, "ok:d5e2f1", ds[0].Status)
	assert.Equal(t, "email", ds[1].Channel)
	assert.Equal(t, "sent:msg-abc", ds[1].Status)

	// Verify old map shape is rejected — guards against regression.
	goldenMapJSON := `{"store":"ok:d5e2f1","email":"sent:msg-abc"}`
	var ds2 []DeliveryResult
	err := json.Unmarshal([]byte(goldenMapJSON), &ds2)
	assert.Error(t, err, "old map shape must not silently unmarshal into []DeliveryResult")
}

// TestChainExecutions_GoldenRow verifies that chain_executions JSON written by the worker
// (currently {"chains_fired":N}) round-trips through json.RawMessage without error.
func TestChainExecutions_GoldenRow(t *testing.T) {
	goldenJSON := `{"chains_fired":2}`

	exec := &AutomationRuleExecution{}
	require.NoError(t, json.Unmarshal([]byte(goldenJSON), &exec.ChainExecutions))
	assert.Equal(t, json.RawMessage(goldenJSON), exec.ChainExecutions)
}

// TestMarshalNullable verifies nil and non-nil marshalling.
func TestMarshalNullable(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		b, err := marshalNullable(nil)
		require.NoError(t, err)
		assert.Nil(t, b)
	})

	t.Run("non-nil marshals", func(t *testing.T) {
		v := map[string]interface{}{"key": "value"}
		b, err := marshalNullable(v)
		require.NoError(t, err)
		assert.Contains(t, string(b), "key")
	})
}

// TestNullString verifies empty vs non-empty string handling.
func TestNullString(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		assert.Nil(t, nullString(""))
	})

	t.Run("non-empty returns value", func(t *testing.T) {
		assert.Equal(t, "hello", nullString("hello"))
	})
}

// TestPostgresRepository_InterfaceCompliance verifies PostgresRepository implements the interface.
func TestPostgresRepository_InterfaceCompliance(t *testing.T) {
	// This is a compile-time check — if PostgresRepository doesn't satisfy
	// AutomationRuleRepository, this test file will not compile.
	var _ AutomationRuleRepository = (*PostgresRepository)(nil)
}
