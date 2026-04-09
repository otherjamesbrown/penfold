package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// AutomationRuleWorkflow is the generic execution engine for automation rules.
// It replaces DigestRollupWorkflow with a composable trigger → selector → skill → output pipeline.
//
// Steps:
//  1. LoadRuleConfig  — fetch rule from DB
//  2. ExecuteSelector — gather content items based on rule's selector_config
//  3. LoadAndRenderSkill — read skill .md file and render template
//  4. ExecuteSkill    — call AI coordinator with rendered prompt
//  5. DeliverOutput   — route result to configured channels
//  6. ExecuteChains   — resolve and start child workflows for chain rules (if any)
//  7. RecordExecution — write execution history to DB
func AutomationRuleWorkflow(ctx workflow.Context, input pkgtemporal.AutomationRuleInput) (*pkgtemporal.AutomationRuleResult, error) {
	logger := workflow.GetLogger(ctx)
	startedAt := workflow.Now(ctx)

	logger.Info("Starting AutomationRuleWorkflow",
		"rule_id", input.RuleID,
		"tenant_id", input.TenantID,
		"trigger_type", input.TriggerType,
		"chain_depth", input.ChainDepth,
	)

	result := &pkgtemporal.AutomationRuleResult{
		Status: "completed",
	}

	// -------------------------------------------------------------------------
	// Step 1: Load rule config
	// -------------------------------------------------------------------------
	fastCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())

	var loadOut LoadRuleConfigOutput
	if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityARLoadRuleConfig, LoadRuleConfigInput{
		RuleID: input.RuleID,
	}).Get(ctx, &loadOut); err != nil {
		return failResult(result, fmt.Errorf("load rule config: %w", err)), nil
	}

	rule := loadOut.Rule
	logger.Info("Rule loaded", "rule_name", rule.Name, "skill", rule.SkillName)

	// -------------------------------------------------------------------------
	// Step 2: Execute selector
	// -------------------------------------------------------------------------
	var selectOut ExecuteSelectorOutput
	if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityARExecuteSelector, ExecuteSelectorInput{
		TenantID:       input.TenantID,
		SelectorConfig: rule.SelectorConfig,
		TriggerItem:    input.TriggerItem,
		ExecutionTime:  startedAt.UTC().Format(time.RFC3339),
	}).Get(ctx, &selectOut); err != nil {
		return failResult(result, fmt.Errorf("execute selector: %w", err)), nil
	}

	result.ItemsSelected = selectOut.Count
	logger.Info("Selector complete", "items", selectOut.Count)

	// -------------------------------------------------------------------------
	// Step 3: Load and render skill
	// -------------------------------------------------------------------------
	var renderOut LoadAndRenderSkillOutput
	if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityARLoadAndRenderSkill, LoadAndRenderSkillInput{
		SkillName:     rule.SkillName,
		RuleName:      rule.Name,
		Items:         selectOut.Items,
		TriggerItem:   input.TriggerItem, // passed through as raw JSON; activity parses it
		ExecutionTime: startedAt.UTC().Format(time.RFC3339),
		WindowFrom:    selectOut.WindowFrom,
		WindowTo:      selectOut.WindowTo,
	}).Get(ctx, &renderOut); err != nil {
		return failResult(result, fmt.Errorf("load and render skill: %w", err)), nil
	}

	// -------------------------------------------------------------------------
	// Step 4: Execute skill via AI coordinator
	// -------------------------------------------------------------------------
	llmCtx := workflow.WithActivityOptions(ctx, pkgtemporal.LLMActivityOptions())

	var skillOut ExecuteSkillOutput
	if err := workflow.ExecuteActivity(llmCtx, pkgtemporal.ActivityARExecuteSkill, ExecuteSkillInput{
		TenantID:       input.TenantID,
		SkillName:      rule.SkillName,
		RenderedPrompt: renderOut.RenderedPrompt,
	}).Get(ctx, &skillOut); err != nil {
		return failResult(result, fmt.Errorf("execute skill: %w", err)), nil
	}

	result.TokensUsed = skillOut.InputTokens + skillOut.OutputTokens
	logger.Info("Skill executed", "model_used", skillOut.ModelUsed, "tokens", result.TokensUsed)

	// -------------------------------------------------------------------------
	// Step 5: Deliver output
	// -------------------------------------------------------------------------
	var deliverOut DeliverOutputOutput
	if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityARDeliverOutput, DeliverOutputInput{
		TenantID:     input.TenantID,
		RuleName:     rule.Name,
		OutputConfig: rule.OutputConfig,
		SkillOutput:  skillOut.Output,
		WindowFrom:   selectOut.WindowFrom,
		WindowTo:     selectOut.WindowTo,
	}).Get(ctx, &deliverOut); err != nil {
		return failResult(result, fmt.Errorf("deliver output: %w", err)), nil
	}

	// Build delivery status map
	deliveryStatus := make(map[string]string, len(deliverOut.Results))
	for _, r := range deliverOut.Results {
		deliveryStatus[r.Channel] = r.Status
	}
	result.DeliveryStatus = deliveryStatus

	// -------------------------------------------------------------------------
	// Step 6: Execute chains
	// -------------------------------------------------------------------------
	var chainOut ExecuteChainsOutput
	if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityARExecuteChains, ExecuteChainsInput{
		TenantID:     input.TenantID,
		OutputConfig: rule.OutputConfig,
		ChainDepth:   input.ChainDepth,
		SkillOutput:  skillOut.Output,
	}).Get(ctx, &chainOut); err != nil {
		// Non-fatal: log and continue
		logger.Warn("ExecuteChains failed, skipping chains", "error", err)
	}

	// Start child workflows for each chain
	chainsFired := 0
	if !chainOut.Skipped && len(chainOut.Chains) > 0 {
		childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
			RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 2},
		})
		for _, chain := range chainOut.Chains {
			chainInput := pkgtemporal.AutomationRuleInput{
				RuleID:        chain.RuleID,
				TenantID:      input.TenantID,
				TriggerType:   "chain",
				TriggerSource: input.RuleID,
				TriggerItem:   chain.TriggerItem,
				ChainDepth:    input.ChainDepth + 1,
			}
			// Fire-and-forget: don't block on child workflow completion
			workflow.ExecuteChildWorkflow(childCtx, AutomationRuleWorkflow, chainInput)
			chainsFired++
			logger.Info("Chain workflow started", "rule_id", chain.RuleID, "rule_name", chain.RuleName)
		}
	}
	result.ChainsFired = chainsFired

	// -------------------------------------------------------------------------
	// Step 7: Record execution
	// -------------------------------------------------------------------------
	var recordOut RecordExecutionOutput
	recordErr := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityARRecordExecution, RecordExecutionInput{
		RuleID:          input.RuleID,
		TriggerType:     input.TriggerType,
		TriggerSource:   input.TriggerSource,
		StartedAt:       startedAt.UTC().Format(time.RFC3339),
		Status:          result.Status,
		ItemsSelected:   result.ItemsSelected,
		SkillTokensUsed: result.TokensUsed,
		DeliveryStatus:  deliverOut.Results,
		ChainsFired:     chainsFired,
	}).Get(ctx, &recordOut)
	if recordErr != nil {
		// Non-fatal — best effort
		logger.Warn("RecordExecution failed", "error", recordErr)
	} else {
		result.ExecutionID = recordOut.ExecutionID
	}

	logger.Info("AutomationRuleWorkflow complete",
		"rule_id", input.RuleID,
		"rule_name", rule.Name,
		"items_selected", result.ItemsSelected,
		"tokens_used", result.TokensUsed,
		"chains_fired", chainsFired,
	)

	return result, nil
}

// failResult sets the workflow result to failed with the given error.
func failResult(result *pkgtemporal.AutomationRuleResult, err error) *pkgtemporal.AutomationRuleResult {
	result.Status = "failed"
	result.Error = err.Error()
	return result
}
