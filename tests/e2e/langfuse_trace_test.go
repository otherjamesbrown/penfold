//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Langfuse Trace Assertion Test Harness
//
// This file contains the acceptance test for the distributed tracing
// architecture. It validates trace structure in Langfuse after pipeline
// processing and is designed to FAIL on the current codebase (f70a646)
// to demonstrate what Phase 2/3/4 will fix.
//
// Run with:
//   go test -tags e2e ./tests/e2e/ -run TestPipelineTraceLangfuse -v -timeout 5m
//
// Expected failures on f70a646:
//   - AssertNoOrphanTraces: stage spans end up in separate orphaned traces
//   - AssertStageSpansExist: stage spans may not appear in the main pipeline trace
//   - AssertGenerationsNestedUnderStages: generations not parented to stage spans
//   - AssertPipelineSpanDuration: root span ends immediately (near-zero duration)
//
// Expected passes on f70a646:
//   - AssertTenantTag: tenant tag is attached to the trace
//   - AssertGenerationsHaveIO: most generations have input/output populated
// ============================================================================

// TraceAssertion holds a fetched Langfuse trace and its observations,
// providing assertion helpers for validating trace structure.
type TraceAssertion struct {
	t            *testing.T
	client       *langfuseClient
	traceID      string
	trace        LangfuseTrace
	observations []LangfuseObservation
}

// GetTraceForContent fetches the latest named trace tagged with contentID from Langfuse.
// It prefers traces with name="email-processing" over unnamed/orphaned traces.
// It skips the test if no traces are found.
func GetTraceForContent(t *testing.T, contentID string) *TraceAssertion {
	t.Helper()

	lf := newLangfuseClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	traces, err := lf.fetchTracesByTag(ctx, contentID)
	require.NoError(t, err, "fetching traces for content %s from Langfuse (%s)", contentID, langfuseHostFromEnv())
	require.NotEmpty(t, traces, "expected at least one Langfuse trace tagged with %q", contentID)

	// Log all found traces for debugging.
	t.Logf("GetTraceForContent: found %d trace(s) tagged with %q:", len(traces), contentID)
	for _, tr := range traces {
		t.Logf("  trace id=%s name=%q ts=%s tags=%v", tr.ID, tr.Name, tr.Timestamp, tr.Tags)
	}

	// Prefer the named pipeline trace (email-processing) over unnamed orphaned traces.
	// The API returns traces ordered by timestamp.desc so the most recent is first.
	// Current behaviour on f70a646: there are often two traces — one named
	// "email-processing" (the real pipeline root) and one unnamed (an orphaned span tree
	// from the worker OTel context). We pick the named one as the canonical trace.
	var trace LangfuseTrace
	for _, tr := range traces {
		if tr.Name == "email-processing" {
			trace = tr
			break
		}
	}
	if trace.ID == "" {
		// Fall back to the most recent trace if no named trace found.
		trace = traces[0]
	}

	t.Logf("GetTraceForContent: selected trace %s (name=%q, timestamp=%s)", trace.ID, trace.Name, trace.Timestamp)

	// Fetch observations for the trace.
	observations, err := lf.fetchObservations(ctx, trace.ID)
	require.NoError(t, err, "fetching observations for trace %s", trace.ID)
	t.Logf("GetTraceForContent: fetched %d observations for trace %s", len(observations), trace.ID)

	for _, obs := range observations {
		parentID := "<root>"
		if obs.ParentObservationID != nil {
			parentID = *obs.ParentObservationID
		}
		t.Logf("  observation: type=%s name=%q id=%s parent=%s level=%s", obs.Type, obs.Name, obs.ID, parentID, obs.Level)
	}

	return &TraceAssertion{
		t:            t,
		client:       lf,
		traceID:      trace.ID,
		trace:        trace,
		observations: observations,
	}
}

// AssertSingleTrace asserts that exactly ONE trace exists for the contentID in the
// last 5 results. Multiple traces indicate duplicate pipeline runs or orphaned traces.
func (ta *TraceAssertion) AssertSingleTrace(contentID string) {
	ta.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	traces, err := ta.client.fetchTracesByTag(ctx, contentID)
	if err != nil {
		ta.t.Errorf("AssertSingleTrace: failed to fetch traces: %v", err)
		return
	}

	// Filter to traces within the last 10 minutes to avoid old runs.
	// The pipeline can take 2-4 minutes, plus polling overhead.
	cutoff := time.Now().Add(-10 * time.Minute)
	var recent []LangfuseTrace
	for _, tr := range traces {
		if tr.Timestamp.After(cutoff) {
			recent = append(recent, tr)
		}
	}

	if len(recent) != 1 {
		ta.t.Logf("AssertSingleTrace: found %d recent traces for %s (last 5m), expected 1:", len(recent), contentID)
		for _, tr := range recent {
			ta.t.Logf("  trace id=%s name=%q ts=%s", tr.ID, tr.Name, tr.Timestamp)
		}
		assert.Len(ta.t, recent, 1, "expected exactly 1 trace for content %s in last 5 minutes, got %d", contentID, len(recent))
	}
}

// AssertNoOrphanTraces asserts that no traces exist which:
//   - Were created after the 'since' timestamp
//   - Are unnamed (name="") indicating a worker-side orphaned trace
//
// On current code (f70a646) this will FAIL because the worker creates stage spans
// in its own OTel provider without propagating the pipeline trace context,
// producing separate unnamed orphaned traces with the same content tags.
func (ta *TraceAssertion) AssertNoOrphanTraces(since time.Time) {
	ta.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Fetch recent traces. We look for any traces created after 'since' that are
	// unnamed (name="") — these are orphaned span trees from the worker's OTel provider.
	// The Langfuse API doesn't have a direct "unnamed" filter, so we fetch recent
	// traces and inspect them.
	apiURL := fmt.Sprintf("%s/api/public/traces?limit=30&orderBy=timestamp.desc", ta.client.host)

	var result struct {
		Data []LangfuseTrace `json:"data"`
	}
	if err := ta.client.doGet(ctx, apiURL, &result); err != nil {
		ta.t.Logf("AssertNoOrphanTraces: error fetching traces (non-fatal): %v", err)
		return
	}

	var orphans []LangfuseTrace
	for _, tr := range result.Data {
		// Only consider traces created after 'since'.
		if !tr.Timestamp.After(since) {
			continue
		}
		// An orphan trace from the worker is unnamed (name="").
		// The real pipeline trace has name="email-processing".
		// We skip our own canonical trace to avoid false positives.
		if tr.ID == ta.traceID {
			continue
		}
		if tr.Name == "" {
			orphans = append(orphans, tr)
		}
	}

	if len(orphans) > 0 {
		ta.t.Logf("AssertNoOrphanTraces: found %d orphan trace(s) created after %s:", len(orphans), since.Format(time.RFC3339))
		for _, tr := range orphans {
			ta.t.Logf("  orphan: id=%s name=%q tags=%v ts=%s", tr.ID, tr.Name, tr.Tags, tr.Timestamp)
		}
		assert.Empty(ta.t, orphans,
			"expected no unnamed orphan traces after %s — "+
				"unnamed traces (name=\"\") indicate worker stage spans are created in an isolated OTel context "+
				"without inheriting the pipeline trace. This is the primary failure on f70a646. "+
				"Phases 2-4 (Temporal OTel interceptors + otelgrpc) will fix this.",
			since.Format(time.RFC3339))
	}
}

// AssertStageSpansExist asserts that each stage name in 'stages' has a corresponding
// SPAN observation in the trace. Missing stage spans indicate the worker is not
// creating spans within the pipeline trace context.
func (ta *TraceAssertion) AssertStageSpansExist(stages []string) {
	ta.t.Helper()

	// Index observations by type and name.
	spanNames := make(map[string]bool)
	for _, obs := range ta.observations {
		if obs.Type == "SPAN" {
			spanNames[obs.Name] = true
		}
	}

	ta.t.Logf("AssertStageSpansExist: found SPAN observations: %v", spanNames)

	for _, stage := range stages {
		if !spanNames[stage] {
			ta.t.Errorf(
				"AssertStageSpansExist: SPAN %q not found in trace %s — "+
					"expected stage span to appear in the main pipeline trace. "+
					"Current behavior (f70a646): stage spans are created in the worker's own "+
					"OTel provider and land in separate orphaned traces, not here.",
				stage, ta.traceID,
			)
		}
	}
}

// AssertGenerationsNestedUnderStages asserts that every GENERATION observation
// has a parentObservationId that refers to a SPAN observation in the same trace.
// On current code this will FAIL because generations are not nested under stage spans.
func (ta *TraceAssertion) AssertGenerationsNestedUnderStages() {
	ta.t.Helper()

	// Build a set of SPAN observation IDs.
	spanIDs := make(map[string]string) // id -> name
	for _, obs := range ta.observations {
		if obs.Type == "SPAN" {
			spanIDs[obs.ID] = obs.Name
		}
	}

	var violations []string
	var generations int

	for _, obs := range ta.observations {
		if obs.Type != "GENERATION" {
			continue
		}
		generations++

		if obs.ParentObservationID == nil {
			violations = append(violations, fmt.Sprintf(
				"GENERATION %q (id=%s) has no parent — expected a SPAN parent",
				obs.Name, obs.ID,
			))
			continue
		}

		parentID := *obs.ParentObservationID
		if _, ok := spanIDs[parentID]; !ok {
			violations = append(violations, fmt.Sprintf(
				"GENERATION %q (id=%s) has parent=%s which is NOT a SPAN observation in this trace",
				obs.Name, obs.ID, parentID,
			))
		}
	}

	ta.t.Logf("AssertGenerationsNestedUnderStages: checked %d GENERATIONs, %d violations", generations, len(violations))

	for _, v := range violations {
		ta.t.Errorf("AssertGenerationsNestedUnderStages: %s — "+
			"On f70a646, generations appear as direct children of the trace root or "+
			"in separate orphaned traces without SPAN parents.", v)
	}
}

// AssertStageCardinality asserts that exactly 'expected' SPAN observations exist
// with the given stage name. Used to detect duplicate stage spans.
func (ta *TraceAssertion) AssertStageCardinality(stage string, expected int) {
	ta.t.Helper()

	var count int
	for _, obs := range ta.observations {
		if obs.Type == "SPAN" && obs.Name == stage {
			count++
		}
	}

	assert.Equal(ta.t, expected, count,
		"AssertStageCardinality: expected %d SPAN(s) named %q in trace %s, got %d",
		expected, stage, ta.traceID, count)
}

// AssertGenerationsHaveIO asserts that all GENERATION observations have non-null
// input and output. This validates that the AI coordinator is recording prompts
// and responses correctly.
func (ta *TraceAssertion) AssertGenerationsHaveIO() {
	ta.t.Helper()

	var missing []string
	var total int

	for _, obs := range ta.observations {
		if obs.Type != "GENERATION" {
			continue
		}
		total++

		if obs.Input == nil {
			missing = append(missing, fmt.Sprintf("GENERATION %q (id=%s) has null input", obs.Name, obs.ID))
		}
		if obs.Output == nil {
			missing = append(missing, fmt.Sprintf("GENERATION %q (id=%s) has null output", obs.Name, obs.ID))
		}
	}

	ta.t.Logf("AssertGenerationsHaveIO: checked %d GENERATIONs, %d missing I/O fields", total, len(missing))
	for _, m := range missing {
		ta.t.Errorf("AssertGenerationsHaveIO: %s", m)
	}
}

// AssertNoErrors asserts that no observations have level=ERROR or status messages
// containing "context_cancelled". These indicate pipeline failures.
func (ta *TraceAssertion) AssertNoErrors() {
	ta.t.Helper()

	var errors []string

	for _, obs := range ta.observations {
		if obs.Level == "ERROR" {
			msg := ""
			if obs.StatusMessage != nil {
				msg = *obs.StatusMessage
			}
			errors = append(errors, fmt.Sprintf(
				"observation %q (id=%s type=%s) has level=ERROR: %s",
				obs.Name, obs.ID, obs.Type, msg,
			))
		}
		if obs.StatusMessage != nil && strings.Contains(*obs.StatusMessage, "context_cancelled") {
			errors = append(errors, fmt.Sprintf(
				"observation %q (id=%s type=%s) has context_cancelled in status: %s",
				obs.Name, obs.ID, obs.Type, *obs.StatusMessage,
			))
		}
	}

	ta.t.Logf("AssertNoErrors: checked %d observations, %d error(s) found", len(ta.observations), len(errors))
	for _, e := range errors {
		ta.t.Errorf("AssertNoErrors: %s", e)
	}
}

// AssertTenantTag asserts that the trace has a tag "tenant:<tenantID>".
func (ta *TraceAssertion) AssertTenantTag(tenantID string) {
	ta.t.Helper()

	expected := "tenant:" + tenantID
	for _, tag := range ta.trace.Tags {
		if tag == expected {
			ta.t.Logf("AssertTenantTag: found expected tenant tag %q", expected)
			return
		}
	}

	ta.t.Errorf("AssertTenantTag: trace %s does not have tag %q, got tags: %v",
		ta.traceID, expected, ta.trace.Tags)
}

// AssertPipelineSpanDuration asserts that the root-level SPAN (the pipeline trace
// root) has a duration of at least minSeconds. A near-zero duration indicates
// that the pipeline span was ended immediately rather than at pipeline completion.
//
// On f70a646 this FAILS because StartPipelineTracing starts and ends the root
// span as a no-op — the span has zero or near-zero duration.
func (ta *TraceAssertion) AssertPipelineSpanDuration(minSeconds float64) {
	ta.t.Helper()

	// Find the root SPAN (parentObservationId == nil) — this is the pipeline span.
	var rootSpan *LangfuseObservation
	for i, obs := range ta.observations {
		if obs.Type == "SPAN" && obs.ParentObservationID == nil {
			rootSpan = &ta.observations[i]
			break
		}
	}

	if rootSpan == nil {
		ta.t.Errorf("AssertPipelineSpanDuration: no root SPAN found in trace %s "+
			"(expected a SPAN with no parent representing the pipeline root). "+
			"On f70a646 the pipeline trace spans are created in separate contexts.",
			ta.traceID)
		return
	}

	if rootSpan.EndTime == nil {
		ta.t.Errorf("AssertPipelineSpanDuration: root SPAN %q (id=%s) has no EndTime — "+
			"span was never properly closed. Duration cannot be computed.",
			rootSpan.Name, rootSpan.ID)
		return
	}

	duration := rootSpan.EndTime.Sub(rootSpan.StartTime).Seconds()
	ta.t.Logf("AssertPipelineSpanDuration: root SPAN %q duration=%.1fs (min=%.1fs)",
		rootSpan.Name, duration, minSeconds)

	assert.GreaterOrEqual(ta.t, duration, minSeconds,
		"AssertPipelineSpanDuration: root SPAN %q has duration %.1fs, expected >= %.1fs. "+
			"On f70a646 StartPipelineTracing returns immediately, so the span duration is ~0s. "+
			"Phase 4 (FinishPipelineTracing activity) will fix this.",
		rootSpan.Name, duration, minSeconds)
}

// ============================================================================
// TestPipelineTraceLangfuse — the acceptance test
// ============================================================================

// TestPipelineTraceLangfuse is the acceptance test for the distributed tracing
// architecture. It processes a known content item through the pipeline and
// validates the resulting Langfuse trace structure.
//
// This test is designed to FAIL on the current codebase (f70a646):
//   - Orphan traces (stage spans land in separate traces, not the main pipeline trace)
//   - Missing stage span nesting (generations not under stage spans)
//   - Zero-duration pipeline span (StartPipelineTracing ends immediately)
//
// It is designed to PASS once Phases 2-4 are implemented:
//   - Phase 2: Temporal OTel interceptors (context propagation workflow→activity)
//   - Phase 3: otelgrpc handlers (context propagation activity→ai-coordinator)
//   - Phase 4: FinishPipelineTracing activity (real pipeline span duration)
func TestPipelineTraceLangfuse(t *testing.T) {
	// This test operates against the production tenant.
	// Do NOT use SafeCLIRunner (it would panic for the production tenant ID).
	// The content item "em-d2sGbd0H" belongs to tenant c3170310-78bd-409c-b186-126f40bfa6ad.
	const contentID = "em-d2sGbd0H"
	const tenantID = "c3170310-78bd-409c-b186-126f40bfa6ad"

	// Verify Langfuse is reachable before running the full test.
	lf := newLangfuseClient(t)
	ctx := context.Background()
	_, err := lf.fetchTracesByTag(ctx, "healthcheck-probe")
	if err != nil {
		// Connection failure means Langfuse is down, not a test failure.
		t.Skipf("Langfuse not reachable at %s: %v", langfuseHostFromEnv(), err)
	}

	before := time.Now()

	// Trigger reprocessing and wait for the pipeline to fully complete.
	// reprocessAndWait waits for a NEW trace (created after triggerTime) with
	// the "email-processing.finish" span, indicating all stages have completed.
	// The pipeline typically takes 2-4 minutes for a full run.
	reprocessAndWait(t, contentID, 5*time.Minute)

	// Fetch the latest trace for this content item.
	ta := GetTraceForContent(t, contentID)

	// === Assertions ===

	// 1. Only ONE trace should exist for this content in the last 5 minutes.
	ta.AssertSingleTrace(contentID)

	// 2. No orphaned traces (untagged traces created after we started).
	// EXPECTED TO FAIL on f70a646: the worker creates stage spans in its own
	// OTel provider, producing orphaned traces separate from the pipeline trace.
	ta.AssertNoOrphanTraces(before)

	// 3. The trace must have the tenant tag.
	// EXPECTED TO PASS on f70a646.
	ta.AssertTenantTag(tenantID)

	// 4. All six stage SPANs must appear in the trace.
	// EXPECTED TO FAIL on f70a646: stage spans land in orphaned traces.
	ta.AssertStageSpansExist([]string{
		"stage.triage",
		"stage.extract_entities",
		"stage.extract_assertions",
		"stage.deep_analyze",
		"stage.embedding",
		"stage.summarize",
	})

	// 5. Every GENERATION must be parented to a SPAN.
	// EXPECTED TO FAIL on f70a646: generations appear as root-level or unparented.
	ta.AssertGenerationsNestedUnderStages()

	// 6. Each high-cardinality stage should have exactly 1 span.
	ta.AssertStageCardinality("stage.extract_entities", 1)
	ta.AssertStageCardinality("stage.embedding", 1)

	// 7. All GENERATIONs must have non-null input and output.
	// EXPECTED TO MOSTLY PASS on f70a646 (generations in orphaned traces still have I/O).
	ta.AssertGenerationsHaveIO()

	// 8. No ERROR-level observations.
	ta.AssertNoErrors()

	// 9. The root pipeline span must have duration >= 30 seconds.
	// EXPECTED TO FAIL on f70a646: StartPipelineTracing ends immediately (~0s duration).
	ta.AssertPipelineSpanDuration(30.0)
}
