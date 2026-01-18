// Package workflows provides Temporal workflow definitions for the Penfold pipeline.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold-go-pipeline/internal/activities"
)

// EmailProcessingInput contains the input data for processing an email.
type EmailProcessingInput struct {
	TenantID    string    `json:"tenant_id"`
	SourceID    int64     `json:"source_id"`
	MessageID   string    `json:"message_id"`
	FromEmail   string    `json:"from_email"`
	FromName    *string   `json:"from_name,omitempty"`
	Subject     *string   `json:"subject,omitempty"`
	ToEmails    []string  `json:"to_emails"`
	CcEmails    []string  `json:"cc_emails"`
	EmailDate   time.Time `json:"email_date"`
	ContentHash string    `json:"content_hash"`
	JobID       string    `json:"job_id"`
}

// EmailProcessingResult contains the output from email processing.
type EmailProcessingResult struct {
	SourceID       int64   `json:"source_id"`
	EmbeddingID    *int64  `json:"embedding_id,omitempty"`
	SummaryID      *int64  `json:"summary_id,omitempty"`
	AssertionCount int     `json:"assertion_count"`
	Status         string  `json:"status"`
	Error          string  `json:"error,omitempty"`
}

// EmailProcessingWorkflow orchestrates the processing of an ingested email.
// It performs the following steps in sequence:
// 1. Fetch source content from database
// 2. Generate embedding (fast, local MLX)
// 3. Generate summary via LLM (slow)
// 4. Extract assertions via LLM (slow)
// 5. Update source status
func EmailProcessingWorkflow(ctx workflow.Context, input EmailProcessingInput) (*EmailProcessingResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting email processing workflow",
		"source_id", input.SourceID,
		"tenant_id", input.TenantID,
		"message_id", input.MessageID,
	)

	result := &EmailProcessingResult{
		SourceID: input.SourceID,
	}

	// Activity options for fast database operations
	fastOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}

	// Activity options for embedding generation (local MLX, 1-5 seconds)
	embeddingOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		HeartbeatTimeout:    10 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}

	// Activity options for LLM operations (30-60 seconds, expensive)
	llmOpts := workflow.ActivityOptions{
		StartToCloseTimeout:    2 * time.Minute,
		ScheduleToCloseTimeout: 5 * time.Minute,
		HeartbeatTimeout:       15 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    2, // Fewer retries for expensive operations
		},
	}

	// Step 1: Fetch source content from database
	var fetchOutput activities.FetchSourceOutput
	ctx1 := workflow.WithActivityOptions(ctx, fastOpts)
	err := workflow.ExecuteActivity(ctx1, "FetchSource", activities.FetchSourceInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
	}).Get(ctx, &fetchOutput)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("fetch_source: %v", err)
		logger.Error("Failed to fetch source", "error", err)
		return result, nil // Return result, not error (for visibility in Temporal UI)
	}

	// Build email context for AI processing
	emailContext := buildEmailContext(input, fetchOutput.ContentText)

	// Step 2: Generate embedding (can run in parallel with LLM if desired, but keep sequential for simplicity)
	var embeddingID int64
	ctx2 := workflow.WithActivityOptions(ctx, embeddingOpts)
	err = workflow.ExecuteActivity(ctx2, "GenerateEmbedding", activities.GenerateEmbeddingInput{
		TenantID:    input.TenantID,
		SourceID:    input.SourceID,
		Content:     emailContext,
		ContentHash: input.ContentHash,
	}).Get(ctx, &embeddingID)
	if err != nil {
		logger.Warn("Embedding generation failed, continuing", "error", err)
		// Continue - embedding failure shouldn't block other processing
	} else {
		result.EmbeddingID = &embeddingID
		logger.Debug("Embedding generated", "embedding_id", embeddingID)
	}

	// Step 3: Generate summary via LLM
	var summaryID int64
	ctx3 := workflow.WithActivityOptions(ctx, llmOpts)
	err = workflow.ExecuteActivity(ctx3, "GenerateSummary", activities.GenerateSummaryInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		JobID:    input.JobID,
		Content:  emailContext,
	}).Get(ctx, &summaryID)
	if err != nil {
		logger.Warn("Summary generation failed, continuing", "error", err)
	} else {
		result.SummaryID = &summaryID
		logger.Debug("Summary generated", "summary_id", summaryID)
	}

	// Step 4: Extract assertions via LLM
	var assertionCount int
	ctx4 := workflow.WithActivityOptions(ctx, llmOpts)
	err = workflow.ExecuteActivity(ctx4, "ExtractAssertions", activities.ExtractAssertionsInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		JobID:    input.JobID,
		Content:  emailContext,
	}).Get(ctx, &assertionCount)
	if err != nil {
		logger.Warn("Assertion extraction failed, continuing", "error", err)
	} else {
		result.AssertionCount = assertionCount
		logger.Debug("Assertions extracted", "count", assertionCount)
	}

	// Step 5: Update source status
	ctx5 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx5, "UpdateSourceStatus", activities.UpdateSourceStatusInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		Status:   "completed",
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Status update failed", "error", err)
	}

	result.Status = "completed"
	logger.Info("Email processing workflow completed",
		"source_id", input.SourceID,
		"embedding_id", result.EmbeddingID,
		"summary_id", result.SummaryID,
		"assertion_count", result.AssertionCount,
	)

	return result, nil
}

// buildEmailContext creates a text context for AI processing from email metadata.
func buildEmailContext(input EmailProcessingInput, contentText string) string {
	context := "Email from: " + input.FromEmail
	if input.FromName != nil && *input.FromName != "" {
		context = "Email from: " + *input.FromName + " <" + input.FromEmail + ">"
	}

	if input.Subject != nil && *input.Subject != "" {
		context += "\nSubject: " + *input.Subject
	}

	context += "\nDate: " + input.EmailDate.Format(time.RFC3339)

	if len(input.ToEmails) > 0 {
		context += "\nTo: "
		for i, to := range input.ToEmails {
			if i > 0 {
				context += ", "
			}
			context += to
		}
	}

	if len(input.CcEmails) > 0 {
		context += "\nCC: "
		for i, cc := range input.CcEmails {
			if i > 0 {
				context += ", "
			}
			context += cc
		}
	}

	context += "\n\n" + contentText

	return context
}
