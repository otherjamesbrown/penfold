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
// Checks both pipeline_runs (most stages) and content_enrichment (structured extracts
// that persist directly to extracted_data without recording in pipeline_runs).
func getCompletedStages(env *QualityEnv, sourceID int64) ([]string, error) {
	ctx := context.Background()
	rows, err := env.DB.Query(ctx,
		`SELECT stage FROM pipeline_runs WHERE source_id = $1 AND status = 'completed'`,
		sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stageSet := make(map[string]bool)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		stageSet[s] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Check for structured extract stages that write to content_enrichment.extracted_data
	// but don't record in pipeline_runs. Match via pipeline_definitions.persist_key,
	// filtered to the pipeline that was actually routed for this source.
	extractRows, err := env.DB.Query(ctx, `
		SELECT pd.stage FROM pipeline_definitions pd
		JOIN content_enrichment ce ON ce.tenant_id = pd.tenant_id::text
		JOIN pipeline_routing pr ON pr.tenant_id = pd.tenant_id
		  AND pr.pipeline = pd.pipeline
		  AND pr.content_subtype = ce.content_subtype
		WHERE ce.source_id = $1 AND ce.tenant_id = $2
		  AND pd.persist_key IS NOT NULL
		  AND ce.extracted_data IS NOT NULL
		  AND pd.tenant_id = $2::uuid
	`, sourceID, env.TenantID)
	if err == nil {
		defer extractRows.Close()
		for extractRows.Next() {
			var s string
			if err := extractRows.Scan(&s); err == nil {
				stageSet[s] = true
			}
		}
	}

	var stages []string
	for s := range stageSet {
		stages = append(stages, s)
	}
	return stages, nil
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

	// Check 4: pipeline name (infer from stages + actual subtype from DB)
	if expected.Pipeline != "" {
		actualSubtype, _ := getContentSubtype(env, sourceID)
		inferredPipeline := inferPipeline(completedStages, actualSubtype)
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

// inferPipeline infers the pipeline name from completed stages and optional content subtype.
// Uses stage presence first, then falls back to content_subtype for cases where
// category-specific stages didn't fire (e.g. newsletter_extract skipped).
func inferPipeline(stages []string, contentSubtype ...string) string {
	subtype := ""
	if len(contentSubtype) > 0 {
		subtype = contentSubtype[0]
	}

	// Stage-based inference (most reliable when category stages ran)
	if slices.Contains(stages, "newsletter_extract") {
		if subtype == "NEWSLETTER_INTERNAL" {
			return "newsletter_internal"
		}
		if subtype == "NEWSLETTER_DIGEST" {
			return "newsletter_digest"
		}
		return "newsletter"
	}
	if slices.Contains(stages, "notification_extract") {
		return "notification"
	}
	if slices.Contains(stages, "extract_semantic") || slices.Contains(stages, "extract_ner") {
		if subtype == "NOTIFICATION" {
			return "notification"
		}
		return "standard"
	}
	// legacy: notification pipeline used summarize before notification_extract existed
	if slices.Contains(stages, "summarize") && !slices.Contains(stages, "extract_semantic") {
		return "notification"
	}

	// Fallback: infer from content_subtype when category stages didn't fire
	switch subtype {
	case "NEWSLETTER":
		return "newsletter"
	case "NEWSLETTER_INTERNAL":
		return "newsletter_internal"
	case "NEWSLETTER_DIGEST":
		return "newsletter_digest"
	case "NOTIFICATION":
		return "notification"
	}

	return "unknown"
}
