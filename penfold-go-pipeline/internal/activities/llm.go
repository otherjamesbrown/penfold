package activities

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/otherjamesbrown/penfold-go-pipeline/internal/storage"
	"go.temporal.io/sdk/activity"
)

// GenerateSummaryInput contains the input for the GenerateSummary activity.
type GenerateSummaryInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	JobID    string `json:"job_id"`
	Content  string `json:"content"`
}

// ExtractAssertionsInput contains the input for the ExtractAssertions activity.
type ExtractAssertionsInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	JobID    string `json:"job_id"`
	Content  string `json:"content"`
}

// GenerateSummary generates a summary of the content using the LLM.
// Returns the processing result ID on success.
func (a *Activities) GenerateSummary(ctx context.Context, input GenerateSummaryInput) (int64, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Generating summary", "source_id", input.SourceID)

	// Record heartbeat before the slow LLM call
	activity.RecordHeartbeat(ctx, "starting_llm_call")

	// Call LLM to generate summary (this is the slow part, 30-60 seconds)
	summary, err := a.llmClient.GenerateSummary(ctx, input.Content)
	if err != nil {
		a.logger.Error().Err(err).Int64("source_id", input.SourceID).Msg("LLM summary generation failed")
		return 0, fmt.Errorf("llm call failed: %w", err)
	}

	activity.RecordHeartbeat(ctx, "llm_complete_storing_result")

	// Store summary as a processing result
	summaryData, _ := json.Marshal(map[string]interface{}{
		"summary":   summary,
		"source_id": input.SourceID,
	})

	result := &storage.ProcessingResult{
		TenantID:   input.TenantID,
		JobID:      input.JobID,
		ResultType: "brief_summary",
		ResultData: summaryData,
		ModelName:  &a.config.LLMModel,
	}

	if err := a.resultRepo.Store(ctx, result); err != nil {
		a.logger.Error().Err(err).Int64("source_id", input.SourceID).Msg("Failed to store summary result")
		return 0, fmt.Errorf("store result failed: %w", err)
	}

	a.logger.Debug().
		Int64("source_id", input.SourceID).
		Int64("result_id", result.ID).
		Int("summary_length", len(summary)).
		Msg("Summary stored successfully")

	return result.ID, nil
}

// ExtractAssertions extracts factual assertions from the content using the LLM.
// Returns the number of assertions extracted on success.
func (a *Activities) ExtractAssertions(ctx context.Context, input ExtractAssertionsInput) (int, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Extracting assertions", "source_id", input.SourceID)

	// Record heartbeat before the slow LLM call
	activity.RecordHeartbeat(ctx, "starting_llm_call")

	// Call LLM to extract assertions (this is slow, 30-60 seconds)
	assertions, err := a.llmClient.ExtractAssertions(ctx, input.Content)
	if err != nil {
		a.logger.Error().Err(err).Int64("source_id", input.SourceID).Msg("LLM assertion extraction failed")
		return 0, fmt.Errorf("llm call failed: %w", err)
	}

	activity.RecordHeartbeat(ctx, "llm_complete_storing_results")

	// Store each assertion as a processing result
	for _, assertion := range assertions {
		confidence := assertion.Confidence
		assertionData, _ := json.Marshal(map[string]interface{}{
			"statement":  assertion.Statement,
			"confidence": assertion.Confidence,
			"category":   assertion.Category,
			"source_id":  input.SourceID,
		})

		result := &storage.ProcessingResult{
			TenantID:        input.TenantID,
			JobID:           input.JobID,
			ResultType:      "assertion_" + assertion.Category,
			ConfidenceScore: &confidence,
			ResultData:      assertionData,
			ModelName:       &a.config.LLMModel,
		}

		if err := a.resultRepo.Store(ctx, result); err != nil {
			a.logger.Error().
				Err(err).
				Int64("source_id", input.SourceID).
				Str("category", assertion.Category).
				Msg("Failed to store assertion")
			// Continue storing other assertions
		}
	}

	a.logger.Debug().
		Int64("source_id", input.SourceID).
		Int("assertion_count", len(assertions)).
		Msg("Assertions stored successfully")

	return len(assertions), nil
}
