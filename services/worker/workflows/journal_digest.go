package workflows

import (
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/pkg/digest"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// JournalDigestWorkflowInput is the input for the JournalDigestWorkflow.
type JournalDigestWorkflowInput struct {
	TenantID    string `json:"tenant_id"`
	ProjectID   int64  `json:"project_id"`
	ProjectName string `json:"project_name"`
	Date        string `json:"date"`  // YYYY-MM-DD
	Focus       string `json:"focus"` // optional focus/question
}

// journalGatherInput mirrors activities.GatherJournalDataInput for JSON dispatch.
type journalGatherInput struct {
	TenantID    string `json:"tenant_id"`
	ProjectID   int64  `json:"project_id"`
	ProjectName string `json:"project_name"`
	Date        string `json:"date"`
}

// journalGatherOutput mirrors activities.GatherJournalDataOutput for JSON dispatch.
type journalGatherOutput struct {
	DailyDigests     json.RawMessage `json:"daily_digests"`
	ContentSummaries json.RawMessage `json:"content_summaries"`
	HasContent       bool            `json:"has_content"`
	PeriodStart      string          `json:"period_start"`
}

// journalGenerateInput mirrors activities.GenerateJournalNarrativeInput for JSON dispatch.
type journalGenerateInput struct {
	TenantID         string          `json:"tenant_id"`
	ProjectID        int64           `json:"project_id"`
	ProjectName      string          `json:"project_name"`
	Date             string          `json:"date"`
	Focus            string          `json:"focus"`
	PeriodStart      string          `json:"period_start"`
	DailyDigests     json.RawMessage `json:"daily_digests"`
	ContentSummaries json.RawMessage `json:"content_summaries"`
}

// JournalDigestWorkflow orchestrates on-demand journal digest generation for a project.
// Unlike daily/weekly digests, journals are NOT idempotent — each trigger creates a new entry.
// Journals synthesise from existing daily digests and attributed content, optionally
// directed by a user-provided focus question.
func JournalDigestWorkflow(ctx workflow.Context, input json.RawMessage) (json.RawMessage, error) {
	logger := workflow.GetLogger(ctx)

	// 1. Unmarshal input
	var wfInput JournalDigestWorkflowInput
	if err := json.Unmarshal(input, &wfInput); err != nil {
		return nil, fmt.Errorf("unmarshal journal digest workflow input: %w", err)
	}

	logger.Info("Starting journal digest workflow",
		"tenant_id", wfInput.TenantID,
		"project_id", wfInput.ProjectID,
		"project_name", wfInput.ProjectName,
		"date", wfInput.Date,
		"focus", wfInput.Focus,
	)

	// 2. Parse date (for validation only)
	if _, err := time.Parse("2006-01-02", wfInput.Date); err != nil {
		return nil, fmt.Errorf("parse date %q: %w", wfInput.Date, err)
	}

	// 3. Gather journal data (daily digests + attributed content) — NO idempotency check
	gatherCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var gatherOut journalGatherOutput
	gatherInput := journalGatherInput{
		TenantID:    wfInput.TenantID,
		ProjectID:   wfInput.ProjectID,
		ProjectName: wfInput.ProjectName,
		Date:        wfInput.Date,
	}
	if err := workflow.ExecuteActivity(gatherCtx, pkgtemporal.ActivityGatherJournalData, gatherInput).Get(ctx, &gatherOut); err != nil {
		return nil, fmt.Errorf("gather journal data: %w", err)
	}

	// 4. Skip if no content
	if !gatherOut.HasContent {
		result := &DigestWorkflowOutput{
			Skipped: true,
			Reason:  "no content for journal",
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("marshal skipped result: %w", err)
		}
		logger.Info("No content for journal, skipping")
		return resultJSON, nil
	}

	// 5. Generate journal narrative via LLM (pass focus)
	llmCtx := workflow.WithActivityOptions(ctx, pkgtemporal.LLMActivityOptions())
	var generateOut digestGenerateOutput
	genInput := journalGenerateInput{
		TenantID:         wfInput.TenantID,
		ProjectID:        wfInput.ProjectID,
		ProjectName:      wfInput.ProjectName,
		Date:             wfInput.Date,
		Focus:            wfInput.Focus,
		PeriodStart:      gatherOut.PeriodStart,
		DailyDigests:     gatherOut.DailyDigests,
		ContentSummaries: gatherOut.ContentSummaries,
	}
	if err := workflow.ExecuteActivity(llmCtx, pkgtemporal.ActivityGenerateJournalNarrative, genInput).Get(ctx, &generateOut); err != nil {
		return nil, fmt.Errorf("generate journal narrative: %w", err)
	}

	// 6. Save digest with type="journal"
	saveCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var saveOut digestSaveOutput
	saveInput := digestSaveInput{
		TenantID:         wfInput.TenantID,
		ProjectID:        wfInput.ProjectID,
		Date:             gatherOut.PeriodStart,
		PeriodEnd:        wfInput.Date,
		DigestType:       digest.DigestTypeJournal,
		Body:             generateOut.Body,
		ModelUsed:        generateOut.ModelUsed,
		PromptTemplateID: generateOut.PromptTemplateID,
		InputTokenCount:  generateOut.InputTokenCount,
		OutputTokenCount: generateOut.OutputTokenCount,
		SourceContentIDs: nil,
	}
	if err := workflow.ExecuteActivity(saveCtx, pkgtemporal.ActivitySaveDigest, saveInput).Get(ctx, &saveOut); err != nil {
		return nil, fmt.Errorf("save journal digest: %w", err)
	}

	logger.Info("Journal digest workflow complete",
		"digest_id", saveOut.DigestID,
	)

	// 7. Return result
	result := &DigestWorkflowOutput{
		DigestID: saveOut.DigestID,
		Skipped:  false,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal journal digest result: %w", err)
	}
	return resultJSON, nil
}
