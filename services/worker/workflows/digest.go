package workflows

import (
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/pkg/digest"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// DigestWorkflowInput is the input for the DigestWorkflow.
type DigestWorkflowInput struct {
	TenantID    string `json:"tenant_id"`
	ProjectID   int64  `json:"project_id"`
	ProjectName string `json:"project_name"`
	Date        string `json:"date"` // YYYY-MM-DD
}

// DigestWorkflowOutput is the output from the DigestWorkflow.
type DigestWorkflowOutput struct {
	DigestID string `json:"digest_id"`
	Skipped  bool   `json:"skipped"`
	Reason   string `json:"reason,omitempty"`
}

// digestGatherInput mirrors activities.GatherDigestDataInput for JSON dispatch.
type digestGatherInput struct {
	TenantID  string    `json:"tenant_id"`
	ProjectID int64     `json:"project_id"`
	Date      time.Time `json:"date"`
}

// digestGatherOutput mirrors activities.GatherDigestDataOutput for JSON dispatch.
// The slice types use json.RawMessage so the workflow doesn't need to import pkg/digest.
type digestGatherOutput struct {
	ContentSummaries   json.RawMessage `json:"content_summaries"`
	Assertions         json.RawMessage `json:"assertions"`
	InstructionMatches json.RawMessage `json:"instruction_matches"`
	LedgerEntries      json.RawMessage `json:"ledger_entries"`
	SourceIDs          []int64         `json:"source_ids"`
	HasContent         bool            `json:"has_content"`
	AlreadyExists      bool            `json:"already_exists"`
	ExistingDigestID   string          `json:"existing_digest_id,omitempty"`
}

// digestGenerateInput mirrors activities.GenerateDigestInput for JSON dispatch.
type digestGenerateInput struct {
	TenantID           string          `json:"tenant_id"`
	ProjectID          int64           `json:"project_id"`
	ProjectName        string          `json:"project_name"`
	Date               string          `json:"date"`
	ContentSummaries   json.RawMessage `json:"content_summaries"`
	Assertions         json.RawMessage `json:"assertions"`
	InstructionMatches json.RawMessage `json:"instruction_matches"`
	LedgerEntries      json.RawMessage `json:"ledger_entries"`
}

// digestGenerateOutput mirrors activities.GenerateDigestOutput for JSON dispatch.
type digestGenerateOutput struct {
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	PromptTemplateID int64           `json:"prompt_template_id"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// digestSaveInput mirrors activities.SaveDigestInput for JSON dispatch.
type digestSaveInput struct {
	TenantID         string          `json:"tenant_id"`
	ProjectID        int64           `json:"project_id"`
	Date             string          `json:"date"`
	PeriodEnd        string          `json:"period_end,omitempty"` // if empty, same as Date
	DigestType       string          `json:"digest_type"`
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	PromptTemplateID int64           `json:"prompt_template_id"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
	SourceContentIDs []int64         `json:"source_content_ids"`
}

// digestSaveOutput mirrors activities.SaveDigestOutput for JSON dispatch.
type digestSaveOutput struct {
	DigestID string `json:"digest_id"`
}

// DigestWorkflow orchestrates daily digest generation for a project.
// It gathers attributed content, assertions, instruction matches, and ledger entries,
// calls the LLM to produce a structured narrative, and persists the result.
// Temporal's data converter handles JSON serialisation of the typed input/output.
func DigestWorkflow(ctx workflow.Context, input DigestWorkflowInput) (*DigestWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)

	// Use a local copy so we can mutate without side-effects on the caller.
	wfInput := input

	// 1. Default empty date to yesterday (for scheduled runs that don't pass a date)
	if wfInput.Date == "" {
		wfInput.Date = workflow.Now(ctx).AddDate(0, 0, -1).Format("2006-01-02")
	}

	logger.Info("Starting digest workflow",
		"tenant_id", wfInput.TenantID,
		"project_id", wfInput.ProjectID,
		"project_name", wfInput.ProjectName,
		"date", wfInput.Date,
	)

	// 3. Parse the date string
	date, err := time.Parse("2006-01-02", wfInput.Date)
	if err != nil {
		return nil, fmt.Errorf("parse date %q: %w", wfInput.Date, err)
	}

	// 3. Gather digest data (fast DB reads, idempotency check)
	gatherCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var gatherOut digestGatherOutput
	gatherInput := digestGatherInput{
		TenantID:  wfInput.TenantID,
		ProjectID: wfInput.ProjectID,
		Date:      date,
	}
	if err := workflow.ExecuteActivity(gatherCtx, pkgtemporal.ActivityGatherDigestData, gatherInput).Get(ctx, &gatherOut); err != nil {
		return nil, fmt.Errorf("gather digest data: %w", err)
	}

	// 4. Skip if digest already exists
	if gatherOut.AlreadyExists {
		logger.Info("Digest already exists, skipping",
			"existing_digest_id", gatherOut.ExistingDigestID,
		)
		return &DigestWorkflowOutput{
			DigestID: gatherOut.ExistingDigestID,
			Skipped:  true,
			Reason:   "digest already exists",
		}, nil
	}

	// 5. Skip if no content for the date
	if !gatherOut.HasContent {
		logger.Info("No content for date, skipping",
			"date", wfInput.Date,
		)
		return &DigestWorkflowOutput{
			Skipped: true,
			Reason:  "no content for date",
		}, nil
	}

	// 6. Generate digest narrative via LLM
	llmCtx := workflow.WithActivityOptions(ctx, pkgtemporal.LLMActivityOptions())
	var generateOut digestGenerateOutput
	generateInput := digestGenerateInput{
		TenantID:           wfInput.TenantID,
		ProjectID:          wfInput.ProjectID,
		ProjectName:        wfInput.ProjectName,
		Date:               wfInput.Date,
		ContentSummaries:   gatherOut.ContentSummaries,
		Assertions:         gatherOut.Assertions,
		InstructionMatches: gatherOut.InstructionMatches,
		LedgerEntries:      gatherOut.LedgerEntries,
	}
	if err := workflow.ExecuteActivity(llmCtx, pkgtemporal.ActivityGenerateDigestNarrative, generateInput).Get(ctx, &generateOut); err != nil {
		return nil, fmt.Errorf("generate digest narrative: %w", err)
	}

	// 7. Save digest
	saveCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	var saveOut digestSaveOutput
	saveInput := digestSaveInput{
		TenantID:         wfInput.TenantID,
		ProjectID:        wfInput.ProjectID,
		Date:             wfInput.Date,
		DigestType:       digest.DigestTypeDaily,
		Body:             generateOut.Body,
		ModelUsed:        generateOut.ModelUsed,
		PromptTemplateID: generateOut.PromptTemplateID,
		InputTokenCount:  generateOut.InputTokenCount,
		OutputTokenCount: generateOut.OutputTokenCount,
		SourceContentIDs: gatherOut.SourceIDs,
	}
	if err := workflow.ExecuteActivity(saveCtx, pkgtemporal.ActivitySaveDigest, saveInput).Get(ctx, &saveOut); err != nil {
		return nil, fmt.Errorf("save digest: %w", err)
	}

	logger.Info("Digest workflow complete",
		"digest_id", saveOut.DigestID,
	)

	// 8. Return result
	return &DigestWorkflowOutput{
		DigestID: saveOut.DigestID,
		Skipped:  false,
	}, nil
}
