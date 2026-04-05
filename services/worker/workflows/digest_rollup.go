package workflows

import (
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// digestRollupGatherInput is the input for the GatherRollupContent activity.
type digestRollupGatherInput struct {
	TenantID      string          `json:"tenant_id"`
	Window        string          `json:"window"`         // e.g. "7d", "30d", "since_last_run"
	SourceFilter  json.RawMessage `json:"source_filter"`  // SourceFilter JSON
	MaxItems      int             `json:"max_items"`
	ReferenceDate string          `json:"reference_date"` // YYYY-MM-DD
	ScheduleID    string          `json:"schedule_id"`
}

// digestRollupGatherOutput is the output of the GatherRollupContent activity.
type digestRollupGatherOutput struct {
	Items      json.RawMessage `json:"items"`
	SourceIDs  []int64         `json:"source_ids"`
	WindowFrom string          `json:"window_from"`
	WindowTo   string          `json:"window_to"`
	Count      int             `json:"count"`
}

// digestRollupGenerateInput is the input for the GenerateRollupSummary activity.
type digestRollupGenerateInput struct {
	TenantID  string          `json:"tenant_id"`
	Name      string          `json:"name"`
	PromptID  string          `json:"prompt_id,omitempty"`
	Prompt    string          `json:"prompt,omitempty"`
	Items     json.RawMessage `json:"items"`
	WindowFrom string         `json:"window_from"`
	WindowTo  string          `json:"window_to"`
}

// digestRollupGenerateOutput is the output of the GenerateRollupSummary activity.
type digestRollupGenerateOutput struct {
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	PromptTemplateID int64           `json:"prompt_template_id"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
	ItemCount        int             `json:"item_count"`
}

// digestRollupDeliverInput is the input for the DeliverRollupResults activity.
type digestRollupDeliverInput struct {
	TenantID       string          `json:"tenant_id"`
	Name           string          `json:"name"`
	Delivery       []string        `json:"delivery"`
	DeliveryTarget string          `json:"delivery_target,omitempty"`
	Body           json.RawMessage `json:"body"`
	ModelUsed      string          `json:"model_used"`
	PromptTemplateID int64         `json:"prompt_template_id"`
	InputTokenCount  int           `json:"input_token_count"`
	OutputTokenCount int           `json:"output_token_count"`
	SourceIDs      []int64         `json:"source_ids"`
	WindowFrom     string          `json:"window_from"`
	WindowTo       string          `json:"window_to"`
}

// digestRollupDeliverOutput is the output of the DeliverRollupResults activity.
type digestRollupDeliverOutput struct {
	DigestID       string            `json:"digest_id"`
	DeliveryStatus map[string]string `json:"delivery_status"`
}

// rollupRecordMetadataInput is the input for the RecordExecutionMetadata activity.
type rollupRecordMetadataInput struct {
	TenantID       string            `json:"tenant_id"`
	ScheduleID     string            `json:"schedule_id"`
	WorkflowType   string            `json:"workflow_type"` // "digest_rollup" or "journal_rollup"
	ReferenceDate  string            `json:"reference_date"`
	DigestID       string            `json:"digest_id,omitempty"`
	ItemsGathered  int               `json:"items_gathered"`
	WindowFrom     string            `json:"window_from,omitempty"`
	WindowTo       string            `json:"window_to,omitempty"`
	DeliveryStatus map[string]string `json:"delivery_status,omitempty"`
	ModelUsed      string            `json:"model_used,omitempty"`
	InputTokens    int               `json:"input_tokens,omitempty"`
	OutputTokens   int               `json:"output_tokens,omitempty"`
}

// DigestRollupWorkflow orchestrates content gathering, LLM summarisation, delivery, and
// execution history recording for a tenant-configured digest rollup schedule.
// Temporal's data converter handles JSON serialisation of the typed input/output.
func DigestRollupWorkflow(ctx workflow.Context, input pkgtemporal.DigestRollupInput) (*pkgtemporal.DigestRollupResult, error) {
	logger := workflow.GetLogger(ctx)

	wfInput := input

	// 1. Default reference date to today
	if wfInput.ReferenceDate == "" {
		wfInput.ReferenceDate = workflow.Now(ctx).Format("2006-01-02")
	}

	// 3. Default max items
	if wfInput.MaxItems <= 0 {
		wfInput.MaxItems = 50
	}

	logger.Info("Starting digest rollup workflow",
		"tenant_id", wfInput.TenantID,
		"name", wfInput.Name,
		"window", wfInput.Window,
		"reference_date", wfInput.ReferenceDate,
	)

	// 4. Gather rollup content
	gatherCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var gatherOut digestRollupGatherOutput
	gatherIn := digestRollupGatherInput{
		TenantID:      wfInput.TenantID,
		Window:        wfInput.Window,
		SourceFilter:  wfInput.SourceFilter,
		MaxItems:      wfInput.MaxItems,
		ReferenceDate: wfInput.ReferenceDate,
		ScheduleID:    wfInput.ScheduleID,
	}
	if err := workflow.ExecuteActivity(gatherCtx, pkgtemporal.ActivityGatherRollupContent, gatherIn).Get(ctx, &gatherOut); err != nil {
		return nil, fmt.Errorf("gather rollup content: %w", err)
	}

	// 5. Generate rollup summary via LLM
	llmCtx := workflow.WithActivityOptions(ctx, pkgtemporal.LLMActivityOptions())
	var generateOut digestRollupGenerateOutput
	generateIn := digestRollupGenerateInput{
		TenantID:   wfInput.TenantID,
		Name:       wfInput.Name,
		PromptID:   wfInput.PromptID,
		Prompt:     wfInput.Prompt,
		Items:      gatherOut.Items,
		WindowFrom: gatherOut.WindowFrom,
		WindowTo:   gatherOut.WindowTo,
	}
	if err := workflow.ExecuteActivity(llmCtx, pkgtemporal.ActivityGenerateRollupSummary, generateIn).Get(ctx, &generateOut); err != nil {
		return nil, fmt.Errorf("generate rollup summary: %w", err)
	}

	// 6. Deliver rollup results (store, email, etc.)
	deliverCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var deliverOut digestRollupDeliverOutput
	deliverIn := digestRollupDeliverInput{
		TenantID:         wfInput.TenantID,
		Name:             wfInput.Name,
		Delivery:         wfInput.Delivery,
		DeliveryTarget:   wfInput.DeliveryTarget,
		Body:             generateOut.Body,
		ModelUsed:        generateOut.ModelUsed,
		PromptTemplateID: generateOut.PromptTemplateID,
		InputTokenCount:  generateOut.InputTokenCount,
		OutputTokenCount: generateOut.OutputTokenCount,
		SourceIDs:        gatherOut.SourceIDs,
		WindowFrom:       gatherOut.WindowFrom,
		WindowTo:         gatherOut.WindowTo,
	}
	if err := workflow.ExecuteActivity(deliverCtx, pkgtemporal.ActivityDeliverRollupResults, deliverIn).Get(ctx, &deliverOut); err != nil {
		return nil, fmt.Errorf("deliver rollup results: %w", err)
	}

	// 7. Record execution metadata
	metaCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	metaIn := rollupRecordMetadataInput{
		TenantID:       wfInput.TenantID,
		ScheduleID:     wfInput.ScheduleID,
		WorkflowType:   "digest_rollup",
		ReferenceDate:  wfInput.ReferenceDate,
		DigestID:       deliverOut.DigestID,
		ItemsGathered:  gatherOut.Count,
		WindowFrom:     gatherOut.WindowFrom,
		WindowTo:       gatherOut.WindowTo,
		DeliveryStatus: deliverOut.DeliveryStatus,
		ModelUsed:      generateOut.ModelUsed,
		InputTokens:    generateOut.InputTokenCount,
		OutputTokens:   generateOut.OutputTokenCount,
	}
	if err := workflow.ExecuteActivity(metaCtx, pkgtemporal.ActivityRecordExecutionMetadata, metaIn).Get(ctx, nil); err != nil {
		// Non-fatal: log but don't fail the workflow
		logger.Warn("Failed to record execution metadata", "error", err)
	}

	logger.Info("Digest rollup workflow complete",
		"digest_id", deliverOut.DigestID,
		"items_gathered", gatherOut.Count,
		"window_from", gatherOut.WindowFrom,
		"window_to", gatherOut.WindowTo,
	)

	// 8. Build and return result
	return &pkgtemporal.DigestRollupResult{
		ItemsGathered:   gatherOut.Count,
		ItemsSummarized: generateOut.ItemCount,
		WindowFrom:      gatherOut.WindowFrom,
		WindowTo:        gatherOut.WindowTo,
		DigestID:        deliverOut.DigestID,
		DeliveryStatus:  deliverOut.DeliveryStatus,
		ModelUsed:       generateOut.ModelUsed,
		InputTokens:     generateOut.InputTokenCount,
		OutputTokens:    generateOut.OutputTokenCount,
	}, nil
}

// digestRollupTraceName returns a Langfuse-style trace name for a digest rollup run.
// Format: digest-rollup/{name}/{reference_date}
func digestRollupTraceName(name, referenceDate string) string {
	if referenceDate == "" {
		referenceDate = time.Now().Format("2006-01-02")
	}
	return fmt.Sprintf("digest-rollup/%s/%s", name, referenceDate)
}
