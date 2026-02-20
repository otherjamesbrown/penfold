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
// This file contains the acceptance test for the direct Langfuse integration
// with domain-level phases (pf-62044a). It validates trace structure in Langfuse
// after pipeline processing and is designed to FAIL on the current codebase
// (OTel-based approach) to demonstrate what the new phase-based integration will fix.
//
// Run with:
//   go test -tags e2e ./tests/e2e/ -run TestPipelineTraceLangfuse -v -timeout 5m
//
// Expected failures on current codebase (OTel approach):
//   - AssertNoOrphanTraces: stage spans end up in separate orphaned traces
//   - AssertPhases: phase SPANs ("Triage", "Summarize", etc.) do not appear in the trace
//   - AssertGenerationsNestedUnderPhases: generations not parented to phase spans
//   - AssertTraceDuration: root span ends immediately (near-zero duration)
//
// Expected passes on current codebase:
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
		ta.t.Logf("AssertSingleTrace: found %d recent traces for %s (last 10m), expected 1:", len(recent), contentID)
		for _, tr := range recent {
			ta.t.Logf("  trace id=%s name=%q ts=%s", tr.ID, tr.Name, tr.Timestamp)
		}
		assert.Len(ta.t, recent, 1, "expected exactly 1 trace for content %s in last 10 minutes, got %d", contentID, len(recent))
	}
}

// AssertNoOrphanTraces asserts that no traces exist which:
//   - Were created after the 'since' timestamp
//   - Are unnamed (name="") indicating a worker-side orphaned trace
//
// On current code this will FAIL because the worker creates stage spans
// in its own OTel provider without propagating the pipeline trace context,
// producing separate unnamed orphaned traces with the same content tags.
func (ta *TraceAssertion) AssertNoOrphanTraces(since time.Time) {
	ta.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Fetch recent traces. We look for any traces created after 'since' that are
	// unnamed (name="") — these are orphaned span trees from the worker's OTel provider.
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
				"unnamed traces (name=\"\") indicate worker spans are created in an isolated context "+
				"without inheriting the pipeline trace. "+
				"The new domain-level phase integration (pf-62044a) fixes this.",
			since.Format(time.RFC3339))
	}
}

// AssertPhases asserts that each phase name in 'phases' has a corresponding
// SPAN observation in the trace. Missing phase spans indicate the pipeline
// is not recording domain-level phases in Langfuse.
//
// Phase names are domain-level: "Triage", "Summarize", "Extract", "Analyze", "Embeddings".
// "Extract" is a single phase grouping both entity and assertion extraction.
//
// On the current codebase (OTel approach) this will FAIL because the worker
// creates OTel spans with names like "stage.triage", not the new phase names.
func (ta *TraceAssertion) AssertPhases(phases []string) {
	ta.t.Helper()

	// Index observations by type and name.
	spanNames := make(map[string]bool)
	for _, obs := range ta.observations {
		if obs.Type == "SPAN" {
			spanNames[obs.Name] = true
		}
	}

	ta.t.Logf("AssertPhases: found SPAN observations: %v", spanNames)

	for _, phase := range phases {
		if !spanNames[phase] {
			ta.t.Errorf(
				"AssertPhases: SPAN %q not found in trace %s — "+
					"expected domain-level phase span to appear in the pipeline trace. "+
					"Current behavior (OTel approach): stage spans use names like 'stage.triage' "+
					"and land in separate orphaned traces, not here with the new phase names.",
				phase, ta.traceID,
			)
		}
	}
}

// AssertGenerationsNestedUnderPhases asserts that every GENERATION observation
// has a parentObservationId that refers to a SPAN observation in the same trace.
// On current code this will FAIL because generations are not nested under phase spans.
func (ta *TraceAssertion) AssertGenerationsNestedUnderPhases() {
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
				"GENERATION %q (id=%s) has no parent — expected a phase SPAN parent",
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

	ta.t.Logf("AssertGenerationsNestedUnderPhases: checked %d GENERATIONs, %d violations", generations, len(violations))

	for _, v := range violations {
		ta.t.Errorf("AssertGenerationsNestedUnderPhases: %s — "+
			"On the current codebase, generations appear as direct children of the trace root "+
			"or in separate orphaned traces without phase SPAN parents.", v)
	}
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

// AssertEnvironment asserts that the trace's environment field is set to the
// expected human-readable tenant name (e.g. "Akamai"), not a UUID.
//
// Currently FAILS because pipeline.go:721 passes input.TenantID (the UUID)
// as TenantName to CreateLangfuseTrace.
func (ta *TraceAssertion) AssertEnvironment(expected string) {
	ta.t.Helper()

	ta.t.Logf("AssertEnvironment: trace environment=%q, expected=%q", ta.trace.Environment, expected)

	assert.Equal(ta.t, expected, ta.trace.Environment,
		"AssertEnvironment: trace %s has environment=%q, expected=%q. "+
			"The pipeline passes TenantID (UUID) as TenantName to CreateLangfuseTrace "+
			"(pipeline.go:721). Fix: thread the actual tenant name through PipelineInput.",
		ta.traceID, ta.trace.Environment, expected)
}

// AssertPhasesHaveGenerations asserts that each named phase SPAN has at least
// one GENERATION child observation. A phase span without generations means the
// LLM call for that stage is not being reported to Langfuse.
//
// Currently FAILS for "Summarize" (span wraps wrong activity + missing Langfuse
// metadata on GenerateSummaryInput) and "Embeddings" (no Langfuse fields on
// GenerateEmbeddingInput + no CreateGeneration in AI server handler).
func (ta *TraceAssertion) AssertPhasesHaveGenerations(phases []string) {
	ta.t.Helper()

	// Build map of SPAN name -> observation ID.
	spanIDByName := make(map[string]string)
	for _, obs := range ta.observations {
		if obs.Type == "SPAN" {
			spanIDByName[obs.Name] = obs.ID
		}
	}

	// Count GENERATION children per parent observation ID.
	generationCountByParent := make(map[string]int)
	for _, obs := range ta.observations {
		if obs.Type == "GENERATION" && obs.ParentObservationID != nil {
			generationCountByParent[*obs.ParentObservationID]++
		}
	}

	for _, phase := range phases {
		spanID, exists := spanIDByName[phase]
		if !exists {
			ta.t.Errorf("AssertPhasesHaveGenerations: phase SPAN %q not found in trace %s "+
				"— cannot check for child generations", phase, ta.traceID)
			continue
		}

		count := generationCountByParent[spanID]
		ta.t.Logf("AssertPhasesHaveGenerations: phase %q (span %s) has %d generation(s)", phase, spanID, count)

		if count == 0 {
			ta.t.Errorf(
				"AssertPhasesHaveGenerations: phase SPAN %q (id=%s) has no GENERATION children — "+
					"the LLM call for this stage is not being reported to Langfuse. "+
					"Check that the activity input passes LangfuseTraceID/LangfusePhaseID "+
					"and the AI server handler calls CreateGeneration.",
				phase, spanID)
		}
	}
}

// AssertTraceDuration asserts that the root-level SPAN (the pipeline trace root)
// has a duration of at least minSeconds. A near-zero duration indicates that the
// pipeline span was ended immediately rather than at pipeline completion.
//
// On the current codebase this FAILS because the OTel-based approach ends the
// root span immediately — the span has zero or near-zero duration.
func (ta *TraceAssertion) AssertTraceDuration(minSeconds float64) {
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
		ta.t.Errorf("AssertTraceDuration: no root SPAN found in trace %s "+
			"(expected a SPAN with no parent representing the pipeline root). "+
			"On the current codebase pipeline trace spans are created in separate contexts.",
			ta.traceID)
		return
	}

	if rootSpan.EndTime == nil {
		ta.t.Errorf("AssertTraceDuration: root SPAN %q (id=%s) has no EndTime — "+
			"span was never properly closed. Duration cannot be computed.",
			rootSpan.Name, rootSpan.ID)
		return
	}

	duration := rootSpan.EndTime.Sub(rootSpan.StartTime).Seconds()
	ta.t.Logf("AssertTraceDuration: root SPAN %q duration=%.1fs (min=%.1fs)",
		rootSpan.Name, duration, minSeconds)

	assert.GreaterOrEqual(ta.t, duration, minSeconds,
		"AssertTraceDuration: root SPAN %q has duration %.1fs, expected >= %.1fs. "+
			"The FinishLangfuseTrace activity at the end of the workflow will fix this "+
			"by properly closing the trace span after all phases complete.",
		rootSpan.Name, duration, minSeconds)
}

// ============================================================================
// TestPipelineTraceLangfuse — the acceptance test
// ============================================================================

// TestPipelineTraceLangfuse is the acceptance test for the direct Langfuse
// integration with domain-level phases (pf-62044a). It processes a known content
// item through the pipeline and validates the resulting Langfuse trace structure.
//
// This test is designed to FAIL on the current codebase (OTel-based approach):
//   - Orphan traces (stage spans land in separate traces, not the main pipeline trace)
//   - Missing phase spans (trace has "stage.triage" etc., not "Triage", "Summarize", etc.)
//   - Missing phase span nesting (generations not under phase spans)
//   - Zero-duration pipeline span (root span ends immediately)
//
// It is designed to PASS once pf-62044a is implemented:
//   - Domain-level phases: Triage, Summarize, Extract, Analyze, Embeddings
//   - All generations nested under their phase span
//   - Single consolidated trace (no orphans)
//   - Real duration from trace start to FinishLangfuseTrace activity
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
	// reprocessAndWait polls until all expected phase spans appear, indicating
	// the full pipeline has run. The pipeline typically takes 2-4 minutes.
	reprocessAndWait(t, contentID, 5*time.Minute)

	// Fetch the latest trace for this content item.
	ta := GetTraceForContent(t, contentID)

	// === Assertions ===

	// 1. Only ONE trace should exist for this content in the last 10 minutes.
	ta.AssertSingleTrace(contentID)

	// 2. No orphaned traces (untagged traces created after we started).
	// EXPECTED TO FAIL on current codebase: the worker creates stage spans in its own
	// OTel provider, producing orphaned traces separate from the pipeline trace.
	ta.AssertNoOrphanTraces(before)

	// 3. Tenant is identified via the Environment field (TenantName), not a tag.
	// The tenant:UUID tag has been removed (pf-da3dbc); use AssertEnvironment instead.

	// 3b. The trace environment must be the tenant NAME, not the UUID.
	// EXPECTED TO FAIL: pipeline.go:721 passes TenantID as TenantName.
	ta.AssertEnvironment("Akamai")

	// 4. All five domain-level phase SPANs must appear in the trace.
	// EXPECTED TO FAIL on current codebase: spans have "stage.triage" etc. (OTel names),
	// not the new domain-level phase names. "Extract" groups both entity and assertion
	// extraction under a single phase span.
	ta.AssertPhases([]string{"Triage", "Summarize", "Extract", "Analyze", "Embeddings"})

	// 5. Every GENERATION must be parented to a phase SPAN.
	// EXPECTED TO FAIL on current codebase: generations appear as root-level or unparented.
	ta.AssertGenerationsNestedUnderPhases()

	// 6. All GENERATIONs must have non-null input and output.
	// EXPECTED TO MOSTLY PASS on current codebase (generations in orphaned traces still have I/O).
	ta.AssertGenerationsHaveIO()

	// 6b. Each phase SPAN that involves an LLM call must have at least one GENERATION child.
	// EXPECTED TO FAIL for "Summarize" and "Embeddings":
	//   - Summarize: span wraps LinkConversation (not LLM call) + GenerateSummaryInput
	//     callsites don't pass LangfuseTraceID/LangfusePhaseID
	//   - Embeddings: GenerateEmbeddingInput has no Langfuse fields + AI server
	//     GenerateEmbedding handler has no CreateGeneration block
	ta.AssertPhasesHaveGenerations([]string{"Triage", "Summarize", "Extract", "Analyze", "Embeddings"})

	// 7. No ERROR-level observations.
	ta.AssertNoErrors()

	// 8. The root pipeline span must have duration >= 30 seconds.
	// EXPECTED TO FAIL on current codebase: root span ends immediately (~0s duration).
	ta.AssertTraceDuration(30.0)
}
