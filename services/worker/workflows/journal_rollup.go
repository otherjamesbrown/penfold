package workflows

import (
	"encoding/json"
	"fmt"

	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// journalRollupGatherInput is the input for the GatherDailyLedgerEntries activity.
type journalRollupGatherInput struct {
	TenantID      string `json:"tenant_id"`
	ReferenceDate string `json:"reference_date"` // YYYY-MM-DD
}

// journalRollupGatherOutput is the output of the GatherDailyLedgerEntries activity.
type journalRollupGatherOutput struct {
	LedgerEntries    json.RawMessage `json:"ledger_entries"`
	Consolidations   json.RawMessage `json:"consolidations"`
	ExistingSummaries json.RawMessage `json:"existing_summaries"`
	HasContent       bool            `json:"has_content"`
	DailyCount       int             `json:"daily_count"` // number of existing daily summaries for this week
}

// journalRollupDailyInput is the input for the GenerateDailySummary activity.
type journalRollupDailyInput struct {
	TenantID         string          `json:"tenant_id"`
	ReferenceDate    string          `json:"reference_date"`
	LedgerEntries    json.RawMessage `json:"ledger_entries"`
	Consolidations   json.RawMessage `json:"consolidations"`
	ExistingSummaries json.RawMessage `json:"existing_summaries"`
}

// journalRollupDailyOutput is the output of the GenerateDailySummary activity.
type journalRollupDailyOutput struct {
	DigestID         string          `json:"digest_id"`
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	PromptTemplateID int64           `json:"prompt_template_id"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// journalRollupWeeklyInput is the input for the GenerateWeeklySummary activity.
type journalRollupWeeklyInput struct {
	TenantID      string `json:"tenant_id"`
	ReferenceDate string `json:"reference_date"` // used to identify the week
}

// journalRollupWeeklyOutput is the output of the GenerateWeeklySummary activity.
type journalRollupWeeklyOutput struct {
	DigestID         string          `json:"digest_id"`          // empty if skipped
	Skipped          bool            `json:"skipped"`
	Body             json.RawMessage `json:"body,omitempty"`
	ModelUsed        string          `json:"model_used,omitempty"`
	InputTokenCount  int             `json:"input_token_count,omitempty"`
	OutputTokenCount int             `json:"output_token_count,omitempty"`
}

// journalRollupOverviewInput is the input for the GenerateOverviewSummary activity.
type journalRollupOverviewInput struct {
	TenantID         string          `json:"tenant_id"`
	ReferenceDate    string          `json:"reference_date"`
	WeeklyDigestID   string          `json:"weekly_digest_id,omitempty"`
	WeeklyBody       json.RawMessage `json:"weekly_body,omitempty"`
}

// journalRollupOverviewOutput is the output of the GenerateOverviewSummary activity.
type journalRollupOverviewOutput struct {
	DigestID         string          `json:"digest_id"`
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// JournalRollupWorkflow orchestrates daily, weekly, and overview journal rollup for a tenant.
// Each run:
//  1. Gathers ledger entries + consolidations for reference_date.
//  2. Generates (or updates) the daily journal summary.
//  3. Conditionally generates the weekly summary when 7 daily summaries exist for the week.
//  4. Updates the running overview summary.
//  5. Records execution metadata.
//
// Temporal's data converter handles JSON serialisation of the typed input/output.
func JournalRollupWorkflow(ctx workflow.Context, input pkgtemporal.JournalRollupInput) (*pkgtemporal.JournalRollupResult, error) {
	logger := workflow.GetLogger(ctx)

	wfInput := input

	// 1. Default reference date to today
	if wfInput.ReferenceDate == "" {
		wfInput.ReferenceDate = workflow.Now(ctx).Format("2006-01-02")
	}

	logger.Info("Starting journal rollup workflow",
		"tenant_id", wfInput.TenantID,
		"reference_date", wfInput.ReferenceDate,
	)

	// 3. Gather daily ledger entries and existing summaries
	gatherCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var gatherOut journalRollupGatherOutput
	gatherIn := journalRollupGatherInput{
		TenantID:      wfInput.TenantID,
		ReferenceDate: wfInput.ReferenceDate,
	}
	if err := workflow.ExecuteActivity(gatherCtx, pkgtemporal.ActivityGatherDailyLedgerEntries, gatherIn).Get(ctx, &gatherOut); err != nil {
		return nil, fmt.Errorf("gather daily ledger entries: %w", err)
	}

	// 4. Generate daily summary
	llmCtx := workflow.WithActivityOptions(ctx, pkgtemporal.LLMActivityOptions())
	var dailyOut journalRollupDailyOutput
	dailyIn := journalRollupDailyInput{
		TenantID:          wfInput.TenantID,
		ReferenceDate:     wfInput.ReferenceDate,
		LedgerEntries:     gatherOut.LedgerEntries,
		Consolidations:    gatherOut.Consolidations,
		ExistingSummaries: gatherOut.ExistingSummaries,
	}
	if err := workflow.ExecuteActivity(llmCtx, pkgtemporal.ActivityGenerateDailySummary, dailyIn).Get(ctx, &dailyOut); err != nil {
		return nil, fmt.Errorf("generate daily summary: %w", err)
	}

	// 5. Conditionally generate weekly summary (only when 7+ daily summaries exist for the week)
	fastCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var weeklyOut journalRollupWeeklyOutput
	weeklyIn := journalRollupWeeklyInput{
		TenantID:      wfInput.TenantID,
		ReferenceDate: wfInput.ReferenceDate,
	}
	// The activity itself checks the count and sets Skipped=true if < 7
	if err := workflow.ExecuteActivity(llmCtx, pkgtemporal.ActivityGenerateWeeklySummary, weeklyIn).Get(ctx, &weeklyOut); err != nil {
		return nil, fmt.Errorf("generate weekly summary: %w", err)
	}

	if weeklyOut.Skipped {
		logger.Info("Weekly summary skipped (fewer than 7 daily summaries for week)",
			"reference_date", wfInput.ReferenceDate,
		)
	}

	// 6. Update running overview summary
	var overviewOut journalRollupOverviewOutput
	overviewIn := journalRollupOverviewInput{
		TenantID:       wfInput.TenantID,
		ReferenceDate:  wfInput.ReferenceDate,
		WeeklyDigestID: weeklyOut.DigestID,
		WeeklyBody:     weeklyOut.Body,
	}
	if err := workflow.ExecuteActivity(llmCtx, pkgtemporal.ActivityGenerateOverviewSummary, overviewIn).Get(ctx, &overviewOut); err != nil {
		return nil, fmt.Errorf("generate overview summary: %w", err)
	}

	// 7. Record execution metadata
	metaIn := rollupRecordMetadataInput{
		TenantID:      wfInput.TenantID,
		ScheduleID:    wfInput.ScheduleID,
		WorkflowType:  "journal_rollup",
		ReferenceDate: wfInput.ReferenceDate,
		DigestID:      dailyOut.DigestID,
		ModelUsed:     dailyOut.ModelUsed,
		InputTokens:   dailyOut.InputTokenCount,
		OutputTokens:  dailyOut.OutputTokenCount,
	}
	if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityRecordExecutionMetadata, metaIn).Get(ctx, nil); err != nil {
		// Non-fatal: log but don't fail the workflow
		logger.Warn("Failed to record execution metadata", "error", err)
	}

	logger.Info("Journal rollup workflow complete",
		"daily_digest_id", dailyOut.DigestID,
		"weekly_digest_id", weeklyOut.DigestID,
		"overview_digest_id", overviewOut.DigestID,
	)

	// 8. Build and return result
	return &pkgtemporal.JournalRollupResult{
		DailyDigestID:    dailyOut.DigestID,
		WeeklyDigestID:   weeklyOut.DigestID,
		OverviewDigestID: overviewOut.DigestID,
		Status:           "completed",
	}, nil
}
