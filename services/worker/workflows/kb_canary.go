package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// canaryQuestion is one retrieval test question from the pf-kb-canaries shard.
// Matches the YAML schema: - q / expected_facts / source_kb.
type canaryQuestion struct {
	Q             string   `json:"q"`
	ExpectedFacts []string `json:"expected_facts"`
	SourceKB      string   `json:"source_kb"`
}

// kbCanaryFetchOutput is returned by the KBCanaryFetchCanaries activity.
type kbCanaryFetchOutput struct {
	Questions []canaryQuestion `json:"questions"`
}

// kbCanaryRunInput is the per-question input for KBCanaryRunQuestion.
// Matches activities.KBCanaryRunInput by JSON field names.
type kbCanaryRunInput struct {
	Question      string   `json:"question"`
	ExpectedFacts []string `json:"expected_facts"`
	SourceKB      string   `json:"source_kb"`
}

// kbCanaryRunOutput is returned by KBCanaryRunQuestion.
type kbCanaryRunOutput struct {
	Question     string   `json:"question"`
	Passed       bool     `json:"passed"`
	Response     string   `json:"response"`
	MissingFacts []string `json:"missing_facts"`
}

// kbCanaryLogGapInput is the input for KBCanaryLogGap.
type kbCanaryLogGapInput struct {
	Date          string   `json:"date"`
	Question      string   `json:"question"`
	Response      string   `json:"response"`
	ExpectedFacts []string `json:"expected_facts"`
	MissingFacts  []string `json:"missing_facts"`
}

// kbCanaryCreateSummaryInput is the input for KBCanaryCreateSummary.
type kbCanaryCreateSummaryInput struct {
	Date   string `json:"date"`
	Passed int    `json:"passed"`
	Total  int    `json:"total"`
}

// KBCanaryResult is the output of KBCanaryWorkflow.
type KBCanaryResult struct {
	Date    string `json:"date"`
	Passed  int    `json:"passed"`
	Total   int    `json:"total"`
	Summary string `json:"summary"`
}

// KBCanaryWorkflow runs daily retrieval quality tests against pf-kb-canaries.
//
// For each canary question it:
//  1. Dispatches a fresh KB lookup (cxp kb search + cxp kb show) — the agent
//     receives ONLY KB context, simulating a clean session with restricted tools.
//  2. Asks the AI to answer using only that KB content.
//  3. Verifies the response contains every expected_fact (case-insensitive substring).
//  4. Logs retrieval-failure gaps to pf-kb-gaps for any question that fails.
//  5. Creates a dated summary shard: "KB Canary YYYY-MM-DD: X/Y passed".
//
// Runs daily at 06:00 per schedule row seeded in migration 168.
func KBCanaryWorkflow(ctx workflow.Context) (*KBCanaryResult, error) {
	logger := workflow.GetLogger(ctx)
	date := workflow.Now(ctx).UTC().Format("2006-01-02")

	logger.Info("KB canary workflow starting", "date", date)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout:    5 * time.Minute,
		ScheduleToCloseTimeout: 15 * time.Minute,
		HeartbeatTimeout:       2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Step 1: Fetch canary questions from pf-kb-canaries shard.
	var fetchOut kbCanaryFetchOutput
	if err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityKBCanaryFetchCanaries).Get(ctx, &fetchOut); err != nil {
		return nil, fmt.Errorf("fetch kb-canaries shard: %w", err)
	}

	if len(fetchOut.Questions) == 0 {
		logger.Info("No canary questions found — skipping canary run")
		return &KBCanaryResult{
			Date:    date,
			Passed:  0,
			Total:   0,
			Summary: fmt.Sprintf("KB Canary %s: no questions configured", date),
		}, nil
	}

	logger.Info("Canary questions loaded", "count", len(fetchOut.Questions))

	// Step 2: Run each question sequentially to avoid flooding the AI coordinator.
	results := make([]kbCanaryRunOutput, 0, len(fetchOut.Questions))
	for _, q := range fetchOut.Questions {
		input := kbCanaryRunInput{
			Question:      q.Q,
			ExpectedFacts: q.ExpectedFacts,
			SourceKB:      q.SourceKB,
		}
		var result kbCanaryRunOutput
		if err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityKBCanaryRunQuestion, input).Get(ctx, &result); err != nil {
			logger.Error("KB canary question activity error", "question", q.Q, "error", err)
			result = kbCanaryRunOutput{
				Question:     q.Q,
				Passed:       false,
				Response:     fmt.Sprintf("activity error: %v", err),
				MissingFacts: q.ExpectedFacts,
			}
		}
		results = append(results, result)
		logger.Info("Canary question result", "question", q.Q, "passed", result.Passed)
	}

	// Step 3: Log retrieval-failure gaps for each failing question.
	passed := 0
	for i, r := range results {
		if r.Passed {
			passed++
			continue
		}
		gapInput := kbCanaryLogGapInput{
			Date:          date,
			Question:      r.Question,
			Response:      r.Response,
			ExpectedFacts: fetchOut.Questions[i].ExpectedFacts,
			MissingFacts:  r.MissingFacts,
		}
		if err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityKBCanaryLogGap, gapInput).Get(ctx, nil); err != nil {
			logger.Error("Failed to log KB canary gap", "question", r.Question, "error", err)
			// Non-fatal — continue with remaining gaps.
		}
	}

	// Step 4: Create dated summary shard.
	summaryInput := kbCanaryCreateSummaryInput{
		Date:   date,
		Passed: passed,
		Total:  len(results),
	}
	if err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityKBCanaryCreateSummary, summaryInput).Get(ctx, nil); err != nil {
		logger.Error("Failed to create KB canary summary shard", "error", err)
		// Non-fatal — results are already persisted in pf-kb-gaps.
	}

	summary := fmt.Sprintf("KB Canary %s: %d/%d passed", date, passed, len(results))
	logger.Info("KB canary workflow complete", "date", date, "passed", passed, "total", len(results))

	return &KBCanaryResult{
		Date:    date,
		Passed:  passed,
		Total:   len(results),
		Summary: summary,
	}, nil
}
