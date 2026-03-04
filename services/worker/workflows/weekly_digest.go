package workflows

import (
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/pkg/digest"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// WeeklyDigestWorkflowInput is the input for the WeeklyDigestWorkflow.
type WeeklyDigestWorkflowInput struct {
	TenantID    string `json:"tenant_id"`
	ProjectID   int64  `json:"project_id"`
	ProjectName string `json:"project_name"`
	Date        string `json:"date"` // YYYY-MM-DD, snapped to Monday by the caller or computed here
}

// weeklyGatherInput mirrors activities.GatherWeeklyDigestDataInput for JSON dispatch.
type weeklyGatherInput struct {
	TenantID    string    `json:"tenant_id"`
	ProjectID   int64     `json:"project_id"`
	ProjectName string    `json:"project_name"`
	WeekStart   time.Time `json:"week_start"`
	WeekEnd     time.Time `json:"week_end"`
}

// weeklyGatherOutput mirrors activities.GatherWeeklyDigestDataOutput for JSON dispatch.
// Slice fields use json.RawMessage so the workflow package does not need to import pkg/digest.
type weeklyGatherOutput struct {
	DailyDigests     json.RawMessage `json:"daily_digests"`
	PreviousRollup   json.RawMessage `json:"previous_rollup"`
	ThemeContexts    json.RawMessage `json:"theme_contexts"`
	HasContent       bool            `json:"has_content"`
	AlreadyExists    bool            `json:"already_exists"`
	ExistingDigestID string          `json:"existing_digest_id,omitempty"`
}

// weeklyGenerateInput mirrors activities.GenerateWeeklyNarrativeInput for JSON dispatch.
type weeklyGenerateInput struct {
	TenantID       string          `json:"tenant_id"`
	ProjectID      int64           `json:"project_id"`
	ProjectName    string          `json:"project_name"`
	WeekStart      string          `json:"week_start"`
	WeekEnd        string          `json:"week_end"`
	DailyDigests   json.RawMessage `json:"daily_digests"`
	PreviousRollup json.RawMessage `json:"previous_rollup"`
	ThemeContexts  json.RawMessage `json:"theme_contexts"`
}

// weeklyThemeUpdateInput mirrors activities.UpdateThemeContextsInput for JSON dispatch.
type weeklyThemeUpdateInput struct {
	TenantID  string          `json:"tenant_id"`
	ProjectID int64           `json:"project_id"`
	Body      json.RawMessage `json:"body"`
}

// WeeklyDigestWorkflow orchestrates weekly digest (rollup) generation for a project.
// It gathers daily digests for the week, the previous weekly rollup, and active project themes,
// calls the LLM to produce a structured weekly narrative, persists the result, and updates
// theme running contexts from the generated output.
// It accepts json.RawMessage as input because Temporal schedules pass workflow_params JSONB from the DB.
func WeeklyDigestWorkflow(ctx workflow.Context, input json.RawMessage) (json.RawMessage, error) {
	logger := workflow.GetLogger(ctx)

	// 1. Unmarshal input
	var wfInput WeeklyDigestWorkflowInput
	if err := json.Unmarshal(input, &wfInput); err != nil {
		return nil, fmt.Errorf("unmarshal weekly digest workflow input: %w", err)
	}

	logger.Info("Starting weekly digest workflow",
		"tenant_id", wfInput.TenantID,
		"project_id", wfInput.ProjectID,
		"project_name", wfInput.ProjectName,
		"date", wfInput.Date,
	)

	// 2. Parse the date and snap to Monday for the week start
	date, err := time.Parse("2006-01-02", wfInput.Date)
	if err != nil {
		return nil, fmt.Errorf("parse date %q: %w", wfInput.Date, err)
	}

	// Snap to Monday: treat Sunday as weekday 7 so Monday = 1
	weekday := date.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	weekStart := date.AddDate(0, 0, -int(weekday-time.Monday))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)
	weekEnd := weekStart.AddDate(0, 0, 6)

	logger.Info("Computed week bounds",
		"week_start", weekStart.Format("2006-01-02"),
		"week_end", weekEnd.Format("2006-01-02"),
	)

	// 3. Gather weekly digest data (idempotency check + data fetch)
	gatherCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var gatherOut weeklyGatherOutput
	gatherInput := weeklyGatherInput{
		TenantID:    wfInput.TenantID,
		ProjectID:   wfInput.ProjectID,
		ProjectName: wfInput.ProjectName,
		WeekStart:   weekStart,
		WeekEnd:     weekEnd,
	}
	if err := workflow.ExecuteActivity(gatherCtx, pkgtemporal.ActivityGatherWeeklyDigestData, gatherInput).Get(ctx, &gatherOut); err != nil {
		return nil, fmt.Errorf("gather weekly digest data: %w", err)
	}

	// 4. Skip if weekly digest already exists
	if gatherOut.AlreadyExists {
		result := &DigestWorkflowOutput{
			DigestID: gatherOut.ExistingDigestID,
			Skipped:  true,
			Reason:   "weekly digest already exists",
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("marshal skipped result: %w", err)
		}
		logger.Info("Weekly digest already exists, skipping",
			"existing_digest_id", gatherOut.ExistingDigestID,
		)
		return resultJSON, nil
	}

	// 5. Skip if no daily digests exist for the week
	if !gatherOut.HasContent {
		result := &DigestWorkflowOutput{
			Skipped: true,
			Reason:  "no daily digests for week",
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("marshal skipped result: %w", err)
		}
		logger.Info("No daily digests for week, skipping weekly digest generation",
			"week_start", weekStart.Format("2006-01-02"),
		)
		return resultJSON, nil
	}

	// 6. Generate weekly narrative via LLM
	llmCtx := workflow.WithActivityOptions(ctx, pkgtemporal.LLMActivityOptions())
	var generateOut digestGenerateOutput
	generateInput := weeklyGenerateInput{
		TenantID:       wfInput.TenantID,
		ProjectID:      wfInput.ProjectID,
		ProjectName:    wfInput.ProjectName,
		WeekStart:      weekStart.Format("2006-01-02"),
		WeekEnd:        weekEnd.Format("2006-01-02"),
		DailyDigests:   gatherOut.DailyDigests,
		PreviousRollup: gatherOut.PreviousRollup,
		ThemeContexts:  gatherOut.ThemeContexts,
	}
	if err := workflow.ExecuteActivity(llmCtx, pkgtemporal.ActivityGenerateWeeklyNarrative, generateInput).Get(ctx, &generateOut); err != nil {
		return nil, fmt.Errorf("generate weekly narrative: %w", err)
	}

	// 7. Save the weekly digest (PeriodStart = weekStart, PeriodEnd = weekEnd)
	saveCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var saveOut digestSaveOutput
	saveInput := digestSaveInput{
		TenantID:         wfInput.TenantID,
		ProjectID:        wfInput.ProjectID,
		Date:             weekStart.Format("2006-01-02"),
		PeriodEnd:        weekEnd.Format("2006-01-02"),
		DigestType:       digest.DigestTypeWeekly,
		Body:             generateOut.Body,
		ModelUsed:        generateOut.ModelUsed,
		PromptTemplateID: generateOut.PromptTemplateID,
		InputTokenCount:  generateOut.InputTokenCount,
		OutputTokenCount: generateOut.OutputTokenCount,
		SourceContentIDs: nil, // weekly digests reference daily digests, not raw content
	}
	if err := workflow.ExecuteActivity(saveCtx, pkgtemporal.ActivitySaveDigest, saveInput).Get(ctx, &saveOut); err != nil {
		return nil, fmt.Errorf("save weekly digest: %w", err)
	}

	logger.Info("Weekly digest saved", "digest_id", saveOut.DigestID)

	// 8. Update theme running contexts (best effort — log error but don't fail the workflow)
	themeCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	themeInput := weeklyThemeUpdateInput{
		TenantID:  wfInput.TenantID,
		ProjectID: wfInput.ProjectID,
		Body:      generateOut.Body,
	}
	if err := workflow.ExecuteActivity(themeCtx, pkgtemporal.ActivityUpdateThemeContexts, themeInput).Get(ctx, nil); err != nil {
		logger.Warn("UpdateThemeContexts failed (non-fatal)", "error", err)
	}

	// 9. Return result
	result := &DigestWorkflowOutput{
		DigestID: saveOut.DigestID,
		Skipped:  false,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal weekly digest result: %w", err)
	}
	return resultJSON, nil
}
