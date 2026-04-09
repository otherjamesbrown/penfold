// Package automation provides repository and types for automation rules.
package automation

import (
	"time"

	"github.com/google/uuid"
)

// AutomationRule represents a composable trigger → selector → skill → output rule.
type AutomationRule struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	Name           string
	Description    string
	Enabled        bool
	TriggerType    string // "cron", "event", "manual"
	TriggerConfig  TriggerConfig
	SelectorConfig SelectorConfig
	SkillName      string
	OutputConfig   OutputConfig
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CreatedBy      string
}

// AutomationRuleExecution records a single execution of an automation rule.
type AutomationRuleExecution struct {
	ID              uuid.UUID
	RuleID          uuid.UUID
	TriggerType     string
	TriggerSource   string // source_id for events, "cron" for scheduled, "manual" for CLI
	StartedAt       time.Time
	CompletedAt     *time.Time
	Status          string // "running", "completed", "failed"
	ItemsSelected   *int
	SkillTokensUsed *int
	DeliveryStatus  map[string]interface{}
	ChainExecutions []ChainExecution
	Error           string
	CreatedAt       time.Time
}

// TriggerConfig holds trigger-type-specific configuration.
// Only one of Cron or Match will be populated, depending on TriggerType.
type TriggerConfig struct {
	// For cron triggers
	Cron string `json:"cron,omitempty"`
	// For event triggers
	Match map[string]string `json:"match,omitempty"`
}

// SelectorConfig defines what data the skill receives.
type SelectorConfig struct {
	Scope        string   `json:"scope"`                   // "trigger_item", "query", "combined", "none"
	Query        string   `json:"query,omitempty"`         // search query string
	ContentTypes []string `json:"content_types,omitempty"` // filter by content type
	Window       string   `json:"window,omitempty"`        // time window, e.g. "24h", "7d"
	Limit        int      `json:"limit,omitempty"`         // max items to return
	Sort         string   `json:"sort,omitempty"`          // "date", etc.
	TriggerItem  bool     `json:"trigger_item,omitempty"`  // include trigger item (combined scope)
}

// OutputConfig specifies delivery channels and optional chaining.
type OutputConfig struct {
	Channels []OutputChannel `json:"channels"`
	Chain    []ChainedRule   `json:"chain,omitempty"`
}

// OutputChannel is a single delivery destination.
type OutputChannel struct {
	Type            string `json:"type"`                       // "email", "store", "whatsapp"
	To              string `json:"to,omitempty"`               // recipient for email/whatsapp
	SubjectTemplate string `json:"subject_template,omitempty"` // template for email subject
	ContentType     string `json:"content_type,omitempty"`     // for store channel
}

// ChainedRule specifies a rule to trigger with the current rule's output.
type ChainedRule struct {
	Rule             string          `json:"rule"`
	SelectorOverride *SelectorConfig `json:"selector_override,omitempty"`
}

// ChainExecution records the result of a chained rule invocation.
type ChainExecution struct {
	Rule        string `json:"rule"`
	ExecutionID string `json:"execution_id,omitempty"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

// ListRuleOpts filters for listing automation rules.
type ListRuleOpts struct {
	Enabled     *bool  // filter by enabled state; nil means no filter
	TriggerType string // filter by trigger type; empty means no filter
}
