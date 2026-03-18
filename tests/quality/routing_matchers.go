//go:build quality

package quality

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

// getContentSubtype queries content_enrichment for the classified subtype.
func getContentSubtype(env *QualityEnv, sourceID int64) (string, error) {
	var subtype string
	err := env.DB.QueryRow(context.Background(),
		`SELECT content_subtype FROM content_enrichment WHERE source_id = $1 AND tenant_id = $2`,
		sourceID, env.TenantID).Scan(&subtype)
	return subtype, err
}

// getCompletedStages returns all stages with status='completed' for a source.
func getCompletedStages(env *QualityEnv, sourceID int64) ([]string, error) {
	rows, err := env.DB.Query(context.Background(),
		`SELECT stage FROM pipeline_runs WHERE source_id = $1 AND tenant_id = $2 AND status = 'completed'`,
		sourceID, env.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stages []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		stages = append(stages, s)
	}
	return stages, rows.Err()
}

// MatchRouting validates L1 routing expectations and returns structured results.
// It also calls t.Error on failures so the test fails naturally.
func MatchRouting(t *testing.T, env *QualityEnv, sourceID int64, expected *RoutingExpectation) []MatchDetail {
	t.Helper()
	if expected == nil {
		return nil
	}
	var details []MatchDetail

	// Check 1: content_subtype
	if expected.ContentSubtype != "" {
		actual, err := getContentSubtype(env, sourceID)
		if err != nil {
			t.Errorf("routing.content_subtype: query failed: %v", err)
			details = append(details, MatchDetail{
				Check: "routing.content_subtype", Pass: false,
				Message: fmt.Sprintf("query failed: %v", err),
			})
		} else {
			pass := actual == expected.ContentSubtype
			if !pass {
				t.Errorf("routing.content_subtype: expected %q, got %q", expected.ContentSubtype, actual)
			} else {
				t.Logf("  routing.content_subtype: %s (matched)", actual)
			}
			details = append(details, MatchDetail{
				Check: "routing.content_subtype", Pass: pass,
				Expected: expected.ContentSubtype, Actual: actual,
			})
		}
	}

	// Check 2+3: must_complete and must_not_run stages
	completedStages, err := getCompletedStages(env, sourceID)
	if err != nil {
		t.Errorf("routing: failed to query completed stages: %v", err)
		details = append(details, MatchDetail{
			Check: "routing.stages", Pass: false,
			Message: fmt.Sprintf("query failed: %v", err),
		})
		return details
	}
	t.Logf("  routing.completed_stages: %v", completedStages)

	for _, stage := range expected.MustComplete {
		found := slices.Contains(completedStages, stage)
		if !found {
			t.Errorf("routing.must_complete: stage %q not completed", stage)
		}
		details = append(details, MatchDetail{
			Check:    fmt.Sprintf("routing.must_complete.%s", stage),
			Pass:     found,
			Expected: "completed",
			Actual:   fmt.Sprintf("present=%v", found),
		})
	}

	for _, stage := range expected.MustNotRun {
		found := slices.Contains(completedStages, stage)
		if found {
			t.Errorf("routing.must_not_run: stage %q should not have run but completed", stage)
		}
		details = append(details, MatchDetail{
			Check:    fmt.Sprintf("routing.must_not_run.%s", stage),
			Pass:     !found,
			Expected: "not run",
			Actual:   fmt.Sprintf("present=%v", found),
		})
	}

	// Check 4: pipeline name (infer from stages)
	if expected.Pipeline != "" {
		inferredPipeline := inferPipeline(completedStages)
		pass := inferredPipeline == expected.Pipeline
		if !pass {
			t.Errorf("routing.pipeline: expected %q, inferred %q from stages", expected.Pipeline, inferredPipeline)
		} else {
			t.Logf("  routing.pipeline: %s (matched)", inferredPipeline)
		}
		details = append(details, MatchDetail{
			Check: "routing.pipeline", Pass: pass,
			Expected: expected.Pipeline, Actual: inferredPipeline,
		})
	}

	return details
}

// inferPipeline infers the pipeline name from completed stages.
func inferPipeline(stages []string) string {
	if slices.Contains(stages, "newsletter_extract") {
		return "newsletter"
	}
	if slices.Contains(stages, "extract_semantic") || slices.Contains(stages, "extract_ner") {
		return "standard"
	}
	// notification pipeline uses summarize but not extract_semantic
	if slices.Contains(stages, "summarize") && !slices.Contains(stages, "extract_semantic") {
		return "notification"
	}
	return "unknown"
}
