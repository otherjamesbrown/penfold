// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	"go.temporal.io/sdk/activity"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/digest"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/pkg/schedule"

	"github.com/jackc/pgx/v5/pgxpool"
)

// === Digest Rollup Input/Output Types ===
// These types are JSON-compatible with the unexported types defined in the workflows package.
// Temporal serialises/deserialises activity parameters via JSON, so field tags must match.

// DigestRollupGatherInput is the input for the GatherRollupContent activity.
type DigestRollupGatherInput struct {
	TenantID      string          `json:"tenant_id"`
	Window        string          `json:"window"`
	SourceFilter  json.RawMessage `json:"source_filter"`
	MaxItems      int             `json:"max_items"`
	ReferenceDate string          `json:"reference_date"`
	ScheduleID    string          `json:"schedule_id"`
}

// DigestRollupGatherOutput is the output of the GatherRollupContent activity.
type DigestRollupGatherOutput struct {
	Items      json.RawMessage `json:"items"`
	SourceIDs  []int64         `json:"source_ids"`
	WindowFrom string          `json:"window_from"`
	WindowTo   string          `json:"window_to"`
	Count      int             `json:"count"`
}

// DigestRollupGenerateInput is the input for the GenerateRollupSummary activity.
type DigestRollupGenerateInput struct {
	TenantID   string          `json:"tenant_id"`
	Name       string          `json:"name"`
	PromptID   string          `json:"prompt_id,omitempty"`
	Prompt     string          `json:"prompt,omitempty"`
	Items      json.RawMessage `json:"items"`
	WindowFrom string          `json:"window_from"`
	WindowTo   string          `json:"window_to"`
}

// DigestRollupGenerateOutput is the output of the GenerateRollupSummary activity.
type DigestRollupGenerateOutput struct {
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	PromptTemplateID int64           `json:"prompt_template_id"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
	ItemCount        int             `json:"item_count"`
}

// DigestRollupDeliverInput is the input for the DeliverRollupResults activity.
type DigestRollupDeliverInput struct {
	TenantID         string          `json:"tenant_id"`
	Name             string          `json:"name"`
	Delivery         []string        `json:"delivery"`
	DeliveryTarget   string          `json:"delivery_target,omitempty"`
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	PromptTemplateID int64           `json:"prompt_template_id"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
	SourceIDs        []int64         `json:"source_ids"`
	WindowFrom       string          `json:"window_from"`
	WindowTo         string          `json:"window_to"`
}

// DigestRollupDeliverOutput is the output of the DeliverRollupResults activity.
type DigestRollupDeliverOutput struct {
	DigestID       string            `json:"digest_id"`
	DeliveryStatus map[string]string `json:"delivery_status"`
}

// RollupRecordMetadataInput is the input for the RecordExecutionMetadata activity.
type RollupRecordMetadataInput struct {
	TenantID       string            `json:"tenant_id"`
	ScheduleID     string            `json:"schedule_id"`
	WorkflowType   string            `json:"workflow_type"`
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

// DigestRollupActivities holds dependencies for digest rollup activities.
type DigestRollupActivities struct {
	db           *pgxpool.Pool
	digestRepo   *digest.Repository
	scheduleRepo *schedule.Repository
	aiClient     AIClient
	promptRepo   *pipeline.Repository
	logger       logging.Logger
}

// NewDigestRollupActivities creates a new DigestRollupActivities instance.
func NewDigestRollupActivities(
	db *pgxpool.Pool,
	digestRepo *digest.Repository,
	scheduleRepo *schedule.Repository,
	aiClient AIClient,
	promptRepo *pipeline.Repository,
	logger logging.Logger,
) *DigestRollupActivities {
	if logger == nil {
		panic("NewDigestRollupActivities: logger is required")
	}
	if db == nil {
		panic("NewDigestRollupActivities: db is required")
	}
	if digestRepo == nil {
		panic("NewDigestRollupActivities: digestRepo is required")
	}
	if scheduleRepo == nil {
		panic("NewDigestRollupActivities: scheduleRepo is required")
	}
	if promptRepo == nil {
		panic("NewDigestRollupActivities: promptRepo is required")
	}
	return &DigestRollupActivities{
		db:           db,
		digestRepo:   digestRepo,
		scheduleRepo: scheduleRepo,
		aiClient:     aiClient,
		promptRepo:   promptRepo,
		logger:       logger.With(logging.F("component", "digest_rollup_activities")),
	}
}

// GatherRollupContent gathers content matching the source filter within the configured window.
func (a *DigestRollupActivities) GatherRollupContent(ctx context.Context, input DigestRollupGatherInput) (*DigestRollupGatherOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GatherRollupContent"),
		logging.F("tenant_id", input.TenantID),
		logging.F("window", input.Window),
		logging.F("reference_date", input.ReferenceDate),
	)

	recordHeartbeat(ctx, "parsing reference date")
	logger.Info("Starting rollup content gathering")

	// 1. Parse reference date
	refDate, err := time.Parse("2006-01-02", input.ReferenceDate)
	if err != nil {
		logger.Error("Failed to parse reference date", logging.Err(err))
		return nil, fmt.Errorf("parse reference_date %q: %w", input.ReferenceDate, err)
	}

	// 2. Calculate time window
	window, err := digest.CalculateWindow(input.Window, refDate, nil)
	if err != nil {
		logger.Error("Failed to calculate window", logging.Err(err))
		return nil, fmt.Errorf("calculate window %q: %w", input.Window, err)
	}

	// 3. Unmarshal source filter
	var filter digest.SourceFilter
	if len(input.SourceFilter) > 0 && string(input.SourceFilter) != "null" {
		if err := json.Unmarshal(input.SourceFilter, &filter); err != nil {
			logger.Warn("Failed to unmarshal source filter, using empty filter", logging.Err(err))
		}
	}

	// 4. Gather content
	recordHeartbeat(ctx, "gathering content by source filter")
	maxItems := input.MaxItems
	if maxItems <= 0 {
		maxItems = 50
	}

	summaries, err := digest.GatherContentBySourceFilter(ctx, a.db, input.TenantID, window, filter, maxItems)
	if err != nil {
		logger.Error("Failed to gather content by source filter", logging.Err(err))
		return nil, fmt.Errorf("gather content by source filter: %w", err)
	}

	// 5. Marshal items and collect source IDs
	itemsJSON, err := json.Marshal(summaries)
	if err != nil {
		logger.Error("Failed to marshal content summaries", logging.Err(err))
		return nil, fmt.Errorf("marshal content summaries: %w", err)
	}

	sourceIDs := make([]int64, 0, len(summaries))
	for _, s := range summaries {
		sourceIDs = append(sourceIDs, s.SourceID)
	}

	output := &DigestRollupGatherOutput{
		Items:      itemsJSON,
		SourceIDs:  sourceIDs,
		WindowFrom: window.From.Format("2006-01-02"),
		WindowTo:   window.To.Format("2006-01-02"),
		Count:      len(summaries),
	}

	logger.Info("Rollup content gathering complete",
		logging.F("item_count", len(summaries)),
		logging.F("window_from", output.WindowFrom),
		logging.F("window_to", output.WindowTo),
	)

	return output, nil
}

// rollupPromptData holds template variables for the rollup summary generation prompt.
type rollupPromptData struct {
	Name       string
	WindowFrom string
	WindowTo   string
	Items      string
}

// GenerateRollupSummary loads the prompt, renders it with gathered items, calls the LLM,
// and returns the summary body.
func (a *DigestRollupActivities) GenerateRollupSummary(ctx context.Context, input DigestRollupGenerateInput) (*DigestRollupGenerateOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GenerateRollupSummary"),
		logging.F("tenant_id", input.TenantID),
		logging.F("name", input.Name),
	)

	logger.Info("Starting rollup summary generation")

	// 1. Resolve prompt content
	var promptContent string
	var promptTemplateID int64

	if input.PromptID != "" {
		// Explicit prompt_id — look up by stage name
		pt, err := a.promptRepo.GetPromptByStage(ctx, input.PromptID, 0)
		if err != nil {
			return nil, fmt.Errorf("load prompt by stage %q: %w", input.PromptID, err)
		}
		promptContent = pt.Content
		promptTemplateID = pt.ID
	} else if input.Prompt != "" {
		// Inline prompt text
		promptContent = input.Prompt
	} else {
		// Default prompt
		pt, err := a.promptRepo.GetPromptByStage(ctx, "digest_rollup_default", 0)
		if err != nil {
			return nil, fmt.Errorf("load default rollup prompt: %w", err)
		}
		promptContent = pt.Content
		promptTemplateID = pt.ID
	}

	// 2. Format items as readable text
	var itemsText string
	if len(input.Items) > 0 && string(input.Items) != "null" {
		var summaries []digest.ContentSummary
		if err := json.Unmarshal(input.Items, &summaries); err != nil {
			logger.Warn("Failed to unmarshal items, using raw JSON", logging.Err(err))
			itemsText = string(input.Items)
		} else {
			itemsText = formatContentSummaries(summaries)
		}
	}
	if itemsText == "" {
		itemsText = "None"
	}

	// 3. Parse and render prompt template
	tmpl, err := template.New("rollup").Parse(promptContent)
	if err != nil {
		return nil, fmt.Errorf("parse rollup prompt template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, rollupPromptData{
		Name:       input.Name,
		WindowFrom: input.WindowFrom,
		WindowTo:   input.WindowTo,
		Items:      itemsText,
	}); err != nil {
		return nil, fmt.Errorf("render rollup prompt: %w", err)
	}

	// 4. Count items from input JSON
	itemCount := 0
	if len(input.Items) > 0 && string(input.Items) != "null" {
		var items []json.RawMessage
		if err := json.Unmarshal(input.Items, &items); err == nil {
			itemCount = len(items)
		}
	}

	// 5. Call LLM
	recordHeartbeat(ctx, "calling LLM for rollup summary")
	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  rendered.String(),
		JsonMode: &jsonMode,
	})
	if err != nil {
		logger.Error("LLM rollup summary generation failed", logging.Err(err))
		return nil, fmt.Errorf("LLM rollup generation: %w", err)
	}

	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	output := &DigestRollupGenerateOutput{
		Body:             json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		PromptTemplateID: promptTemplateID,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
		ItemCount:        itemCount,
	}

	logger.Info("Rollup summary generated",
		logging.F("model_used", resp.ModelUsed),
		logging.F("input_tokens", inputTokens),
		logging.F("output_tokens", outputTokens),
		logging.F("item_count", itemCount),
	)

	return output, nil
}

// DeliverRollupResults stores the generated rollup summary and handles delivery targets.
func (a *DigestRollupActivities) DeliverRollupResults(ctx context.Context, input DigestRollupDeliverInput) (*DigestRollupDeliverOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "DeliverRollupResults"),
		logging.F("tenant_id", input.TenantID),
		logging.F("name", input.Name),
		logging.F("delivery", input.Delivery),
	)

	recordHeartbeat(ctx, "delivering rollup results")
	logger.Info("Starting rollup delivery")

	deliveryStatus := make(map[string]string)
	var digestID string

	// Parse window boundaries for period_start/period_end
	periodStart := time.Now().UTC()
	periodEnd := time.Now().UTC()
	if input.WindowFrom != "" {
		if t, err := time.Parse("2006-01-02", input.WindowFrom); err == nil {
			periodStart = t
		}
	}
	if input.WindowTo != "" {
		if t, err := time.Parse("2006-01-02", input.WindowTo); err == nil {
			periodEnd = t
		}
	}

	for _, delivery := range input.Delivery {
		switch delivery {
		case "store":
			// Save to digests table with digest_type="rollup"
			d := digest.Digest{
				TenantID:         input.TenantID,
				ProjectID:        0, // tenant-level, not project-scoped
				DigestType:       digest.DigestTypeRollup,
				PeriodStart:      periodStart,
				PeriodEnd:        periodEnd,
				Body:             input.Body,
				ModelUsed:        input.ModelUsed,
				PromptTemplateID: input.PromptTemplateID,
				InputTokenCount:  input.InputTokenCount,
				OutputTokenCount: input.OutputTokenCount,
				SourceContentIDs: input.SourceIDs,
			}

			recordHeartbeat(ctx, "saving rollup digest")
			id, err := a.digestRepo.Create(ctx, &d)
			if err != nil {
				logger.Error("Failed to create rollup digest", logging.Err(err))
				deliveryStatus["store"] = fmt.Sprintf("error: %s", err.Error())
			} else {
				digestID = id
				deliveryStatus["store"] = "ok"
				logger.Info("Rollup digest saved", logging.F("digest_id", id))
			}

		case "email":
			// Email delivery not yet implemented
			logger.Info("Email delivery not yet implemented, skipping")
			deliveryStatus["email"] = "not_yet_implemented"

		default:
			logger.Warn("Unknown delivery type, skipping", logging.F("delivery_type", delivery))
			deliveryStatus[delivery] = "unknown_delivery_type"
		}
	}

	output := &DigestRollupDeliverOutput{
		DigestID:       digestID,
		DeliveryStatus: deliveryStatus,
	}

	logger.Info("Rollup delivery complete",
		logging.F("digest_id", digestID),
		logging.F("delivery_status", deliveryStatus),
	)

	return output, nil
}

// RecordExecutionMetadata records schedule execution history for rollup workflows.
// This activity is shared by both DigestRollupWorkflow and JournalRollupWorkflow.
func (a *DigestRollupActivities) RecordExecutionMetadata(ctx context.Context, input RollupRecordMetadataInput) error {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "RecordExecutionMetadata"),
		logging.F("tenant_id", input.TenantID),
		logging.F("schedule_id", input.ScheduleID),
		logging.F("workflow_type", input.WorkflowType),
	)

	logger.Info("Recording execution metadata")

	// Skip if no schedule ID (e.g. manual trigger without schedule)
	if input.ScheduleID == "" {
		logger.Info("No schedule ID, skipping execution metadata recording")
		return nil
	}

	// 1. Build result_metadata JSON
	resultMeta := map[string]interface{}{
		"workflow_type":   input.WorkflowType,
		"reference_date":  input.ReferenceDate,
		"items_gathered":  input.ItemsGathered,
		"window_from":     input.WindowFrom,
		"window_to":       input.WindowTo,
		"delivery_status": input.DeliveryStatus,
		"model_used":      input.ModelUsed,
		"input_tokens":    input.InputTokens,
		"output_tokens":   input.OutputTokens,
	}
	if input.DigestID != "" {
		resultMeta["digest_id"] = input.DigestID
	}

	metaJSON, err := json.Marshal(resultMeta)
	if err != nil {
		logger.Warn("Failed to marshal result metadata, using empty", logging.Err(err))
		metaJSON = json.RawMessage("{}")
	}

	// 2. Get workflow run ID from activity info
	runID := activity.GetInfo(ctx).WorkflowExecution.RunID

	// 3. Create execution record
	exec := &schedule.ScheduleExecution{
		ScheduleID:     input.ScheduleID,
		TenantID:       input.TenantID,
		WorkflowRunID:  runID,
		Status:         "running",
		StartedAt:      time.Now().UTC(),
		ResultMetadata: metaJSON,
	}

	execID, err := a.scheduleRepo.CreateExecution(ctx, exec)
	if err != nil {
		logger.Warn("Failed to create execution record", logging.Err(err))
		// Non-fatal: best-effort metadata recording
		return nil
	}

	// 4. Immediately complete the execution
	if err := a.scheduleRepo.CompleteExecution(ctx, execID, "completed", metaJSON, ""); err != nil {
		logger.Warn("Failed to complete execution record", logging.Err(err))
	}

	// 5. Update schedule last_run_at and last_status
	now := time.Now().UTC()
	status := "success"
	updates := map[string]interface{}{
		"last_run_at": now,
		"last_status": status,
	}
	if err := a.scheduleRepo.Update(ctx, input.TenantID, input.ScheduleID, updates); err != nil {
		logger.Warn("Failed to update schedule last_run_at/last_status",
			logging.Err(err),
			logging.F("schedule_id", input.ScheduleID),
		)
	}

	// Fix ErrNotFound wrapping: ignore not-found on schedule update (may have been deleted)
	_ = pferrors.IsNotFound(err)

	logger.Info("Execution metadata recorded",
		logging.F("execution_id", execID),
		logging.F("run_id", runID),
	)

	return nil
}
