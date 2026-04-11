package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// KBTriageInput is the input for KBTriageWorkflow.
type KBTriageInput struct {
	// ScheduleID is the schedule that triggered this run (used for logging).
	ScheduleID string `json:"schedule_id"`
}

// KBTriageResult is the output of KBTriageWorkflow.
type KBTriageResult struct {
	Status           string `json:"status"` // completed, failed
	Error            string `json:"error,omitempty"`
	GapsRead         bool   `json:"gaps_read"`
	TasksCreated     int    `json:"tasks_created"`
	Escalations      int    `json:"escalations"`
	ReportShardTitle string `json:"report_shard_title,omitempty"`
}

// KBReadGapsInput is the input for the KBTriageReadGaps activity.
type KBReadGapsInput struct {
	ShardID string `json:"shard_id"` // e.g. "pf-kb-gaps"
}

// KBReadGapsOutput is the output of the KBTriageReadGaps activity.
type KBReadGapsOutput struct {
	Content string `json:"content"`
	Empty   bool   `json:"empty"`
}

// KBAnalyzeInput is the input for the KBTriageAnalyze activity.
type KBAnalyzeInput struct {
	GapsContent string `json:"gaps_content"`
}

// KBTriageAction is a single proposed corrective action from the LLM.
type KBTriageAction struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Category string `json:"category"` // hallucination, omission, drift-detected, retrieval-failure, coverage-hole
}

// KBEscalationEntry is a gap that has been logged 3+ times without resolution.
type KBEscalationEntry struct {
	GapRef string `json:"gap_ref"` // reference or description of the gap
	Body   string `json:"body"`    // escalation entry text
}

// KBAnalyzeOutput is the output of the KBTriageAnalyze activity.
type KBAnalyzeOutput struct {
	Actions     []KBTriageAction    `json:"actions"`
	Escalations []KBEscalationEntry `json:"escalations"`
	Summary     string              `json:"summary"`
}

// KBCreateTaskInput is the input for the KBTriageCreateTask activity.
type KBCreateTaskInput struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Parent string `json:"parent"` // e.g. "pf-kb-gaps"
}

// KBCreateTaskOutput is the output of the KBTriageCreateTask activity.
type KBCreateTaskOutput struct {
	TaskID string `json:"task_id"`
}

// KBAppendEscalationInput is the input for the KBTriageAppendEscalation activity.
type KBAppendEscalationInput struct {
	ShardID string `json:"shard_id"` // e.g. "pf-kb-escalations"
	Body    string `json:"body"`
}

// KBCreateReportInput is the input for the KBTriageCreateReport activity.
type KBCreateReportInput struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Parent  string `json:"parent,omitempty"` // optional parent shard ID
}

// KBCreateReportOutput is the output of the KBTriageCreateReport activity.
type KBCreateReportOutput struct {
	ShardID string `json:"shard_id"`
}

// KBTriageWorkflow is the weekly KB triage workflow.
//
// It reads the pf-kb-gaps shard, groups failures by category, proposes corrective
// actions, creates penfold tasks for each action, escalates recurring unresolvable
// gaps, and produces a weekly report shard.
//
// Designed to be triggered by a Temporal schedule (Mondays at 05:00 UTC).
func KBTriageWorkflow(ctx workflow.Context, input KBTriageInput) (*KBTriageResult, error) {
	logger := workflow.GetLogger(ctx)
	result := &KBTriageResult{Status: "completed"}

	reportDate := workflow.Now(ctx).UTC().Format("2006-01-02")
	logger.Info("Starting KBTriageWorkflow", "schedule_id", input.ScheduleID, "report_date", reportDate)

	// Activity options — generous timeout for LLM calls and CLI operations.
	ao := workflow.ActivityOptions{
		StartToCloseTimeout:    10 * time.Minute,
		ScheduleToCloseTimeout: 20 * time.Minute,
		HeartbeatTimeout:       2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// -------------------------------------------------------------------------
	// Step 1: Read pf-kb-gaps shard
	// -------------------------------------------------------------------------
	var gapsOut KBReadGapsOutput
	if err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityKBTriageReadGaps, KBReadGapsInput{
		ShardID: "pf-kb-gaps",
	}).Get(ctx, &gapsOut); err != nil {
		logger.Error("Failed to read pf-kb-gaps shard", "error", err)
		result.Status = "failed"
		result.Error = fmt.Sprintf("read gaps: %v", err)
		return result, nil
	}

	result.GapsRead = true

	if gapsOut.Empty {
		logger.Info("pf-kb-gaps is empty — no triage needed")
		reportTitle := fmt.Sprintf("KB Triage Report %s", reportDate)
		_ = createTriageReport(ctx, reportTitle, "No gaps logged this week.", pkgtemporal.ActivityKBTriageCreateReport, result)
		return result, nil
	}

	// -------------------------------------------------------------------------
	// Step 2: Call LLM to analyze gaps and propose actions
	// -------------------------------------------------------------------------
	var analyzeOut KBAnalyzeOutput
	if err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityKBTriageAnalyze, KBAnalyzeInput{
		GapsContent: gapsOut.Content,
	}).Get(ctx, &analyzeOut); err != nil {
		logger.Error("Failed to analyze KB gaps", "error", err)
		result.Status = "failed"
		result.Error = fmt.Sprintf("analyze gaps: %v", err)
		return result, nil
	}

	logger.Info("KB gaps analyzed",
		"actions", len(analyzeOut.Actions),
		"escalations", len(analyzeOut.Escalations),
	)

	// -------------------------------------------------------------------------
	// Step 3: Create a penfold task for each proposed action
	// -------------------------------------------------------------------------
	for _, action := range analyzeOut.Actions {
		var taskOut KBCreateTaskOutput
		if err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityKBTriageCreateTask, KBCreateTaskInput{
			Title:  action.Title,
			Body:   action.Body,
			Parent: "pf-kb-gaps",
		}).Get(ctx, &taskOut); err != nil {
			// Non-fatal: log and continue to the next action.
			logger.Warn("Failed to create KB triage task",
				"title", action.Title,
				"error", err,
			)
			continue
		}
		result.TasksCreated++
		logger.Info("Created KB triage task", "task_id", taskOut.TaskID, "title", action.Title)
	}

	// -------------------------------------------------------------------------
	// Step 4: Escalate gaps seen 3+ times without resolution
	// -------------------------------------------------------------------------
	for _, esc := range analyzeOut.Escalations {
		if err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityKBTriageAppendEscalation, KBAppendEscalationInput{
			ShardID: "pf-kb-escalations",
			Body:    esc.Body,
		}).Get(ctx, nil); err != nil {
			// Non-fatal: log and continue.
			logger.Warn("Failed to append escalation entry",
				"gap_ref", esc.GapRef,
				"error", err,
			)
			continue
		}
		result.Escalations++
		logger.Info("Escalated gap", "gap_ref", esc.GapRef)
	}

	// -------------------------------------------------------------------------
	// Step 5: Create the weekly triage report shard
	// -------------------------------------------------------------------------
	reportTitle := fmt.Sprintf("KB Triage Report %s", reportDate)
	reportBody := buildTriageReportBody(reportDate, analyzeOut)
	_ = createTriageReport(ctx, reportTitle, reportBody, pkgtemporal.ActivityKBTriageCreateReport, result)

	logger.Info("KBTriageWorkflow complete",
		"tasks_created", result.TasksCreated,
		"escalations", result.Escalations,
		"report", result.ReportShardTitle,
	)

	return result, nil
}

// createTriageReport fires the create-report activity and records the shard title in result.
// Errors are non-fatal (logged, not propagated).
func createTriageReport(ctx workflow.Context, title, body, activityName string, result *KBTriageResult) error {
	logger := workflow.GetLogger(ctx)
	var reportOut KBCreateReportOutput
	if err := workflow.ExecuteActivity(ctx, activityName, KBCreateReportInput{
		Title: title,
		Body:  body,
	}).Get(ctx, &reportOut); err != nil {
		logger.Warn("Failed to create KB triage report shard", "title", title, "error", err)
		return err
	}
	result.ReportShardTitle = title
	return nil
}

// buildTriageReportBody constructs the Markdown body for the triage report shard.
func buildTriageReportBody(date string, analysis KBAnalyzeOutput) string {
	var body string
	body += fmt.Sprintf("# KB Triage Report — %s\n\n", date)
	body += fmt.Sprintf("**Summary:** %s\n\n", analysis.Summary)

	// Group actions by category for the report
	byCategory := make(map[string][]KBTriageAction)
	for _, a := range analysis.Actions {
		byCategory[a.Category] = append(byCategory[a.Category], a)
	}

	categories := []string{"hallucination", "omission", "drift-detected", "retrieval-failure", "coverage-hole"}
	for _, cat := range categories {
		actions, ok := byCategory[cat]
		if !ok || len(actions) == 0 {
			continue
		}
		body += fmt.Sprintf("## %s (%d action(s))\n\n", cat, len(actions))
		for _, a := range actions {
			body += fmt.Sprintf("- **%s**\n\n  %s\n\n", a.Title, a.Body)
		}
	}

	if len(analysis.Escalations) > 0 {
		body += fmt.Sprintf("## Escalations (%d)\n\n", len(analysis.Escalations))
		for _, e := range analysis.Escalations {
			body += fmt.Sprintf("- %s\n\n", e.Body)
		}
	}

	return body
}
