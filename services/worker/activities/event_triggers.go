package activities

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/automation"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// EventTriggerActivities evaluates enabled event-triggered automation rules
// after pipeline processing completes and starts AutomationRuleWorkflow for
// each rule whose match criteria is satisfied.
type EventTriggerActivities struct {
	repo           automation.AutomationRuleRepository
	temporalClient client.Client
	taskQueue      string
	logger         logging.Logger
}

// NewEventTriggerActivities creates a new EventTriggerActivities.
func NewEventTriggerActivities(
	repo automation.AutomationRuleRepository,
	temporalClient client.Client,
	taskQueue string,
	logger logging.Logger,
) *EventTriggerActivities {
	return &EventTriggerActivities{
		repo:           repo,
		temporalClient: temporalClient,
		taskQueue:      taskQueue,
		logger:         logger.With(logging.F("component", "event_trigger_activities")),
	}
}

// EvaluateEventTriggers loads all enabled event-triggered automation rules for the
// tenant and starts AutomationRuleWorkflow for each rule whose match criteria is
// satisfied by the completed content item.
//
// This activity is best-effort: individual workflow start failures are logged but
// do not cause the activity to fail.
func (a *EventTriggerActivities) EvaluateEventTriggers(ctx context.Context, input workflows.EvaluateEventTriggersInput) (*workflows.EvaluateEventTriggersOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "EvaluateEventTriggers"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("content_type", input.ContentType),
	)

	activity.RecordHeartbeat(ctx, "loading event rules")

	tenantUUID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}

	rules, err := a.repo.ListEnabledByTriggerType(ctx, tenantUUID, "event")
	if err != nil {
		return nil, fmt.Errorf("list enabled event rules: %w", err)
	}

	output := &workflows.EvaluateEventTriggersOutput{
		RulesEvaluated: len(rules),
	}

	if len(rules) == 0 {
		logger.Debug("No enabled event rules for tenant")
		return output, nil
	}

	logger.Info("Evaluating event trigger rules",
		logging.F("rule_count", len(rules)),
	)

	for _, rule := range rules {
		if !matchesContent(rule, input) {
			continue
		}

		output.RulesMatched++
		activity.RecordHeartbeat(ctx, fmt.Sprintf("starting workflow for rule %s", rule.Name))

		workflowID := fmt.Sprintf("automation-rule-%s-%d-%d",
			rule.ID.String(), input.SourceID, time.Now().UnixNano())

		wfInput := pkgtemporal.AutomationRuleWorkflowInput{
			RuleID:        rule.ID.String(),
			TenantID:      input.TenantID,
			TriggerType:   "event",
			TriggerSource: fmt.Sprintf("%d", input.SourceID),
			TriggerItem: &pkgtemporal.AutomationTriggerItem{
				SourceID:       input.SourceID,
				TenantID:       input.TenantID,
				ContentType:    input.ContentType,
				ContentSubtype: input.ContentSubtype,
			},
		}

		_, startErr := a.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: a.taskQueue,
		}, pkgtemporal.AutomationRuleWorkflowName, wfInput)
		if startErr != nil {
			logger.Warn("Failed to start AutomationRuleWorkflow (non-fatal)",
				logging.F("rule_id", rule.ID.String()),
				logging.F("rule_name", rule.Name),
				logging.F("source_id", input.SourceID),
				logging.Err(startErr),
			)
			continue
		}

		output.WorkflowsStarted++
		logger.Info("Started AutomationRuleWorkflow",
			logging.F("rule_id", rule.ID.String()),
			logging.F("rule_name", rule.Name),
			logging.F("workflow_id", workflowID),
		)
	}

	logger.Info("Event trigger evaluation complete",
		logging.F("rules_evaluated", output.RulesEvaluated),
		logging.F("rules_matched", output.RulesMatched),
		logging.F("workflows_started", output.WorkflowsStarted),
	)

	return output, nil
}

// matchesContent checks whether a rule's trigger_config.match is satisfied by
// the given content metadata. All specified match fields are AND'd. Unspecified
// fields (empty or absent) match everything.
//
// Supported fields:
//   - content_type: exact string match (case-insensitive)
//   - content_subtype: exact string match (case-insensitive)
//   - source_system: exact string match (case-insensitive)
//   - urgency: exact string match (case-insensitive, e.g. "high", "medium", "low")
//   - from: substring match against sender email (case-insensitive)
//   - subject: regex match against subject line
func matchesContent(rule *automation.AutomationRule, content workflows.EvaluateEventTriggersInput) bool {
	match := rule.TriggerConfig.Match
	if len(match) == 0 {
		// No match criteria — matches everything.
		return true
	}

	for field, want := range match {
		if want == "" {
			continue // empty value = wildcard
		}
		switch field {
		case "content_type":
			if !strings.EqualFold(content.ContentType, want) {
				return false
			}
		case "content_subtype":
			if !strings.EqualFold(content.ContentSubtype, want) {
				return false
			}
		case "source_system":
			if !strings.EqualFold(content.SourceSystem, want) {
				return false
			}
		case "urgency":
			if !strings.EqualFold(content.Urgency, want) {
				return false
			}
		case "from":
			if !strings.Contains(strings.ToLower(content.SenderEmail), strings.ToLower(want)) {
				return false
			}
		case "subject":
			matched, err := regexp.MatchString(want, content.Subject)
			if err != nil || !matched {
				return false
			}
		// "project" and "action_required" require DB lookups; reserved for future enrichment pass.
		}
	}
	return true
}
