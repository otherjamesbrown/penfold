package workflows

import (
	"encoding/json"
	"time"
)

// =============================================================================
// Domain types (used by both activities and workflow)
// =============================================================================

// AutomationRule represents a row in the automation_rules table.
type AutomationRule struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Enabled        bool            `json:"enabled"`
	TriggerType    string          `json:"trigger_type"`
	TriggerConfig  json.RawMessage `json:"trigger_config"`
	SelectorConfig json.RawMessage `json:"selector_config"`
	SkillName      string          `json:"skill_name"`
	OutputConfig   json.RawMessage `json:"output_config"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	CreatedBy      string          `json:"created_by"`
}

// SelectorConfig is the parsed selector_config from automation_rules.
type SelectorConfig struct {
	Scope        string   `json:"scope"`                   // 'trigger_item', 'query', 'combined'
	Query        string   `json:"query,omitempty"`
	ContentTypes []string `json:"content_types,omitempty"`
	Window       string   `json:"window,omitempty"` // "24h", "7d"
	Limit        int      `json:"limit,omitempty"`
	Sort         string   `json:"sort,omitempty"`
}

// OutputChannel represents a single delivery channel in output_config.channels.
type OutputChannel struct {
	Type            string `json:"type"`                      // 'email', 'store'
	To              string `json:"to,omitempty"`
	SubjectTemplate string `json:"subject_template,omitempty"`
}

// ChainRule represents a rule to chain from output_config.chain.
type ChainRule struct {
	Rule             string          `json:"rule"`
	SelectorOverride json.RawMessage `json:"selector_override,omitempty"`
}

// OutputConfig is the parsed output_config from automation_rules.
type OutputConfig struct {
	Channels []OutputChannel `json:"channels"`
	Chain    []ChainRule     `json:"chain,omitempty"`
}

// SelectedItem is a single content item selected by the selector.
type SelectedItem struct {
	SourceID int64     `json:"source_id"`
	Subject  string    `json:"subject"`
	From     string    `json:"from"`
	Date     time.Time `json:"date"`
	Summary  string    `json:"summary"`
	Snippet  string    `json:"snippet"`
}

// =============================================================================
// Activity 1: LoadRuleConfig
// =============================================================================

// LoadRuleConfigInput is the input for the LoadRuleConfig activity.
type LoadRuleConfigInput struct {
	RuleID string `json:"rule_id"`
}

// LoadRuleConfigOutput is the output of the LoadRuleConfig activity.
type LoadRuleConfigOutput struct {
	Rule AutomationRule `json:"rule"`
}

// =============================================================================
// Activity 2: ExecuteSelector
// =============================================================================

// ExecuteSelectorInput is the input for the ExecuteSelector activity.
type ExecuteSelectorInput struct {
	TenantID       string          `json:"tenant_id"`
	SelectorConfig json.RawMessage `json:"selector_config"`
	TriggerItem    json.RawMessage `json:"trigger_item,omitempty"`
	ExecutionTime  string          `json:"execution_time"` // RFC3339
}

// ExecuteSelectorOutput is the output of the ExecuteSelector activity.
type ExecuteSelectorOutput struct {
	Items      []SelectedItem `json:"items"`
	Count      int            `json:"count"`
	WindowFrom string         `json:"window_from,omitempty"`
	WindowTo   string         `json:"window_to,omitempty"`
}

// =============================================================================
// Activity 3: LoadAndRenderSkill
// =============================================================================

// LoadAndRenderSkillInput is the input for the LoadAndRenderSkill activity.
type LoadAndRenderSkillInput struct {
	SkillName     string          `json:"skill_name"`
	RuleName      string          `json:"rule_name"`
	Items         []SelectedItem  `json:"items"`
	TriggerItem   json.RawMessage `json:"trigger_item,omitempty"`
	ExecutionTime string          `json:"execution_time"` // RFC3339
	WindowFrom    string          `json:"window_from,omitempty"`
	WindowTo      string          `json:"window_to,omitempty"`
}

// LoadAndRenderSkillOutput is the output of the LoadAndRenderSkill activity.
type LoadAndRenderSkillOutput struct {
	RenderedPrompt string `json:"rendered_prompt"`
	SkillName      string `json:"skill_name"`
}

// =============================================================================
// Activity 4: ExecuteSkill
// =============================================================================

// ExecuteSkillInput is the input for the ExecuteSkill activity.
type ExecuteSkillInput struct {
	TenantID       string `json:"tenant_id"`
	SkillName      string `json:"skill_name"`
	RenderedPrompt string `json:"rendered_prompt"`
}

// ExecuteSkillOutput is the output of the ExecuteSkill activity.
type ExecuteSkillOutput struct {
	Output       string `json:"output"`
	ModelUsed    string `json:"model_used"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// =============================================================================
// Activity 5: DeliverOutput
// =============================================================================

// DeliverOutputInput is the input for the DeliverOutput activity.
type DeliverOutputInput struct {
	TenantID     string          `json:"tenant_id"`
	RuleName     string          `json:"rule_name"`
	OutputConfig json.RawMessage `json:"output_config"`
	SkillOutput  string          `json:"skill_output"`
	WindowFrom   string          `json:"window_from,omitempty"`
	WindowTo     string          `json:"window_to,omitempty"`
}

// DeliverOutputResult holds the per-channel delivery result.
type DeliverOutputResult struct {
	Channel string `json:"channel"`
	Status  string `json:"status"` // "ok:<id>", "error: <msg>", "skipped"
}

// DeliverOutputOutput is the output of the DeliverOutput activity.
type DeliverOutputOutput struct {
	Results  []DeliverOutputResult `json:"results"`
	DigestID string                `json:"digest_id,omitempty"`
}

// =============================================================================
// Activity 6: ExecuteChains
// =============================================================================

// ExecuteChainsInput is the input for the ExecuteChains activity.
type ExecuteChainsInput struct {
	TenantID     string          `json:"tenant_id"`
	OutputConfig json.RawMessage `json:"output_config"`
	ChainDepth   int             `json:"chain_depth"`
	SkillOutput  string          `json:"skill_output"`
}

// ChainToExecute describes a single chain workflow to start.
type ChainToExecute struct {
	RuleID           string          `json:"rule_id"`
	RuleName         string          `json:"rule_name"`
	SelectorOverride json.RawMessage `json:"selector_override,omitempty"`
	TriggerItem      json.RawMessage `json:"trigger_item"`
}

// ExecuteChainsOutput is the output of the ExecuteChains activity.
type ExecuteChainsOutput struct {
	Chains   []ChainToExecute `json:"chains"`
	Skipped  bool             `json:"skipped"`
	MaxDepth int              `json:"max_depth"`
}

// =============================================================================
// Activity 7: RecordExecution
// =============================================================================

// RecordExecutionInput is the input for the RecordExecution activity.
type RecordExecutionInput struct {
	RuleID          string               `json:"rule_id"`
	TriggerType     string               `json:"trigger_type"`
	TriggerSource   string               `json:"trigger_source,omitempty"`
	StartedAt       string               `json:"started_at"` // RFC3339
	Status          string               `json:"status"`
	ItemsSelected   int                  `json:"items_selected"`
	SkillTokensUsed int                  `json:"skill_tokens_used"`
	DeliveryStatus  []DeliverOutputResult `json:"delivery_status"`
	ChainsFired     int                  `json:"chains_fired"`
	ErrorMsg        string               `json:"error,omitempty"`
}

// RecordExecutionOutput is the output of the RecordExecution activity.
type RecordExecutionOutput struct {
	ExecutionID string `json:"execution_id"`
}
