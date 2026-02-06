// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// EmailProcessingInput contains the input data for processing an email.
// This mirrors the type in pkg/temporal/types.go for convenience.
type EmailProcessingInput = pkgtemporal.EmailProcessingInput

// EmailProcessingResult contains the output from email processing.
// This mirrors the type in pkg/temporal/types.go for convenience.
type EmailProcessingResult = pkgtemporal.EmailProcessingResult

// Activity input types for email processing.
type (
	// FetchSourceInput is the input for the FetchSource activity.
	FetchSourceInput struct {
		TenantID string `json:"tenant_id"`
		SourceID int64  `json:"source_id"`
	}

	// FetchSourceOutput is the output from the FetchSource activity.
	FetchSourceOutput struct {
		ContentText string `json:"content_text"`
		ContentType string `json:"content_type"`
		Subject     string `json:"subject,omitempty"`
		SenderEmail string `json:"sender_email,omitempty"`
		SenderName  string `json:"sender_name,omitempty"`
	}

	// GenerateEmbeddingInput is the input for the GenerateEmbedding activity.
	GenerateEmbeddingInput struct {
		TenantID    string `json:"tenant_id"`
		SourceID    int64  `json:"source_id"`
		// ContentID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
		ContentID   string `json:"content_id,omitempty"`
		Content     string `json:"content"`
		ContentHash string `json:"content_hash"`
	}

	// GenerateSummaryInput is the input for the GenerateSummary activity.
	GenerateSummaryInput struct {
		TenantID string `json:"tenant_id"`
		SourceID int64  `json:"source_id"`
		// ContentID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
		ContentID string `json:"content_id,omitempty"`
		JobID    string `json:"job_id"`
		Content  string `json:"content"`
	}

	// ExtractAssertionsInput is the input for the ExtractAssertions activity.
	ExtractAssertionsInput struct {
		TenantID string `json:"tenant_id"`
		SourceID int64  `json:"source_id"`
		// ContentID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
		ContentID string `json:"content_id,omitempty"`
		JobID    string `json:"job_id"`
		Content  string `json:"content"`
	}

	// UpdateSourceStatusInput is the input for the UpdateSourceStatus activity.
	UpdateSourceStatusInput struct {
		TenantID         string `json:"tenant_id"`
		SourceID         int64  `json:"source_id"`
		Status           string `json:"status"`
		FailureCategory  string `json:"failure_category,omitempty"`
		FailureReason    string `json:"failure_reason,omitempty"`
		TriageCategory   string `json:"triage_category,omitempty"`
		TriageImportance string `json:"triage_importance,omitempty"`
		SkipDeep         *bool  `json:"skip_deep,omitempty"`
	}
)

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

	// Use activity option presets from pkg/temporal
	fastOpts := pkgtemporal.FastActivityOptions()
	embeddingOpts := pkgtemporal.EmbeddingActivityOptions()
	llmOpts := pkgtemporal.LLMActivityOptions()

	// Add non-retryable errors
	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	embeddingOpts = pkgtemporal.WithNonRetryableErrors(embeddingOpts, pkgtemporal.NonRetryableErrors()...)
	llmOpts = pkgtemporal.WithNonRetryableErrors(llmOpts, pkgtemporal.NonRetryableErrors()...)

	// Step 1: Fetch source content from database
	var fetchOutput FetchSourceOutput
	ctx1 := workflow.WithActivityOptions(ctx, fastOpts)
	err := workflow.ExecuteActivity(ctx1, pkgtemporal.ActivityFetchSource, FetchSourceInput{
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
	err = workflow.ExecuteActivity(ctx2, pkgtemporal.ActivityGenerateEmbedding, GenerateEmbeddingInput{
		TenantID:    input.TenantID,
		SourceID:    input.SourceID,
		ContentID:   input.ContentID,
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
	err = workflow.ExecuteActivity(ctx3, pkgtemporal.ActivityGenerateSummary, GenerateSummaryInput{
		TenantID:  input.TenantID,
		SourceID:  input.SourceID,
		ContentID: input.ContentID,
		JobID:     input.JobID,
		Content:   emailContext,
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
	err = workflow.ExecuteActivity(ctx4, pkgtemporal.ActivityExtractAssertions, ExtractAssertionsInput{
		TenantID:  input.TenantID,
		SourceID:  input.SourceID,
		ContentID: input.ContentID,
		JobID:     input.JobID,
		Content:   emailContext,
	}).Get(ctx, &assertionCount)
	if err != nil {
		logger.Warn("Assertion extraction failed, continuing", "error", err)
	} else {
		result.AssertionCount = assertionCount
		logger.Debug("Assertions extracted", "count", assertionCount)
	}

	// Step 5: Update source status
	ctx5 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx5, pkgtemporal.ActivityUpdateSourceStatus, UpdateSourceStatusInput{
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

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
