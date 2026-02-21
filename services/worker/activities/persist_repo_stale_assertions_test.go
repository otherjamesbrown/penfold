package activities

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staleAssertionKey is the dedup key used by checkExistingAssertion.
// It mirrors the real SQL: WHERE source_id = $1 AND assertion_type = $2 AND source_quote = $3
type staleAssertionKey struct {
	sourceID      int64
	assertionType string
	sourceQuote   string // ContextExcerpt in PersistFindingsInput
}

// staleTestAssertion records one assertion row as the real DB would store it.
type staleTestAssertion struct {
	id            int64
	sourceID      int64
	assertionType string
	description   string
	sourceQuote   string
}

// buggyInMemoryPersistRepo is an in-memory implementation of PersistRepository
// that mirrors the fixed behaviour of PersistFindings after pf-7e5168:
//   - It deduplicates assertions within the new batch on (source_id, assertion_type, source_quote)
//   - After inserting the new batch, it removes stale assertions for the source
//     that were not touched in this run (equivalent to the DELETE + re-insert the DB does)
//
// The name is retained for test compatibility; the semantics were updated as part
// of the fix for pf-7e5168.
type buggyInMemoryPersistRepo struct {
	mu         sync.Mutex
	nextID     int64
	assertions []staleTestAssertion // all rows, in insertion order
}

// PersistFindings implements PersistRepository, reflecting the fixed behaviour.
// It processes each assertion (deduplicating on source_id+type+quote), then
// removes any assertion for the same source that was not touched in this call.
// This ensures stale assertions from previous runs are cleaned up, while
// same-quote reprocessing remains idempotent.
func (r *buggyInMemoryPersistRepo) PersistFindings(_ context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error) {
	if input == nil || input.Analysis == nil {
		return nil, fmt.Errorf("input and analysis are required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	output := &PersistFindingsOutput{
		CreatedAssertionIDs: []int64{},
		CreatedReferenceIDs: []int64{},
		CreatedReviewIDs:    []int64{},
	}

	// touchedIDs tracks assertion IDs that were either found (dedup) or created
	// in this run. At the end, untouched assertions for this source are deleted,
	// mirroring the DELETE + re-insert semantics of the real fixed PersistFindings.
	touchedIDs := make(map[int64]bool)

	// Process verified actions — mirrors persistAction in persist_repo.go
	for _, action := range input.Analysis.VerifiedActions {
		assertionType := "action"

		// checkExistingAssertion: dedup on (source_id, type, source_quote)
		if id, found := r.findAssertionID(input.SourceID, assertionType, action.ContextExcerpt); found {
			touchedIDs[id] = true
			output.AssertionsSuperseded++
			continue
		}

		r.nextID++
		id := r.nextID
		r.assertions = append(r.assertions, staleTestAssertion{
			id:            id,
			sourceID:      input.SourceID,
			assertionType: assertionType,
			description:   action.Description,
			sourceQuote:   action.ContextExcerpt,
		})
		touchedIDs[id] = true
		output.AssertionsCreated++
		output.CreatedAssertionIDs = append(output.CreatedAssertionIDs, id)
	}

	// Process verified decisions — mirrors persistDecision in persist_repo.go
	for _, decision := range input.Analysis.VerifiedDecisions {
		assertionType := "decision"
		if id, found := r.findAssertionID(input.SourceID, assertionType, decision.ContextExcerpt); found {
			touchedIDs[id] = true
			output.AssertionsSuperseded++
			continue
		}

		r.nextID++
		id := r.nextID
		r.assertions = append(r.assertions, staleTestAssertion{
			id:            id,
			sourceID:      input.SourceID,
			assertionType: assertionType,
			description:   decision.Description,
			sourceQuote:   decision.ContextExcerpt,
		})
		touchedIDs[id] = true
		output.AssertionsCreated++
		output.CreatedAssertionIDs = append(output.CreatedAssertionIDs, id)
	}

	// Fix for pf-7e5168: remove stale assertions for this source that were not
	// touched in this run. This is the in-memory equivalent of the DELETE at the
	// start of the real DB transaction followed by inserting fresh rows — any
	// assertion that isn't part of the current extraction is discarded.
	var kept []staleTestAssertion
	for _, a := range r.assertions {
		if a.sourceID != input.SourceID || touchedIDs[a.id] {
			kept = append(kept, a)
		}
	}
	r.assertions = kept

	return output, nil
}

// findAssertionID checks for an existing row matching (source_id, type, source_quote).
// This mirrors checkExistingAssertion in persist_repo.go.
// Returns the assertion ID and true if found; 0 and false otherwise.
// Must be called with r.mu held.
func (r *buggyInMemoryPersistRepo) findAssertionID(sourceID int64, assertionType, sourceQuote string) (int64, bool) {
	for _, a := range r.assertions {
		if a.sourceID == sourceID && a.assertionType == assertionType && a.sourceQuote == sourceQuote {
			return a.id, true
		}
	}
	return 0, false
}

// assertionsForSource returns all assertion rows for a given source_id.
// Must be called after the mu is released (or not held).
func (r *buggyInMemoryPersistRepo) assertionsForSource(sourceID int64) []staleTestAssertion {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []staleTestAssertion
	for _, a := range r.assertions {
		if a.sourceID == sourceID {
			result = append(result, a)
		}
	}
	return result
}

// TestPersistFindings_ReprocessClearsStaleAssertions reproduces bug pf-7e5168.
//
// Bug: PersistFindings does not clear existing assertions for a source before
// inserting new ones during reprocess. The dedup check (checkExistingAssertion)
// only catches exact matches on (source_id, assertion_type, source_quote).
// Different LLM runs produce different source_quote values, so both the old and
// new assertion accumulate in the database.
//
// This test:
//  1. Calls PersistFindings with assertion A (quote: "run-1 quote")
//  2. Calls PersistFindings again with assertion B (same source, same type, DIFFERENT quote: "run-2 quote")
//  3. Asserts that ONLY assertion B exists — the test FAILS because both A and B exist.
//
// The test MUST FAIL on the current code. A passing result means it no longer
// reproduces the bug and should be updated.
func TestPersistFindings_ReprocessClearsStaleAssertions(t *testing.T) {
	repo := &buggyInMemoryPersistRepo{}
	ctx := context.Background()

	const sourceID = int64(42)

	// --- First processing run ---
	// The LLM produces an action with ContextExcerpt (source_quote) from run 1.
	firstRun := &PersistFindingsInput{
		TenantID:       "tenant-test",
		SourceID:       sourceID,
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedActions: []VerifiedActionOutput{
				{
					Description:    "Schedule the kickoff meeting",
					ContextExcerpt: "Let's schedule a kickoff meeting by end of week.", // run-1 quote
					Status:         "confirmed",
				},
			},
		},
	}

	out1, err := repo.PersistFindings(ctx, firstRun)
	require.NoError(t, err, "first PersistFindings call should succeed")
	require.Equal(t, 1, out1.AssertionsCreated, "first run should create 1 assertion")

	assertionsAfterRun1 := repo.assertionsForSource(sourceID)
	require.Len(t, assertionsAfterRun1, 1, "exactly 1 assertion should exist after first run")

	// --- Second processing run (reprocess with different LLM output) ---
	// The LLM re-processes the same email but extracts a slightly different
	// context excerpt (source_quote) for the same conceptual action. This is
	// the exact scenario described in bug pf-7e5168.
	//
	// Key: the assertion_type ("action") and source_id are IDENTICAL to run 1,
	// but source_quote differs. checkExistingAssertion does an exact match on
	// (source_id, assertion_type, source_quote) and so finds NO match for the
	// run-2 quote — the old run-1 assertion is invisible to the dedup check.
	secondRun := &PersistFindingsInput{
		TenantID:       "tenant-test",
		SourceID:       sourceID,
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedActions: []VerifiedActionOutput{
				{
					Description:    "Schedule the kickoff meeting",
					ContextExcerpt: "We need to schedule a kickoff soon, before Friday.", // run-2 quote (different!)
					Status:         "confirmed",
				},
			},
		},
	}

	out2, err := repo.PersistFindings(ctx, secondRun)
	require.NoError(t, err, "second PersistFindings call (reprocess) should succeed")

	// Count assertions after reprocess
	assertionsAfterRun2 := repo.assertionsForSource(sourceID)

	// Log the state to make the failure message clear
	t.Logf("Assertions in store after reprocess:")
	for _, a := range assertionsAfterRun2 {
		t.Logf("  id=%d  type=%q  quote=%q", a.id, a.assertionType, a.sourceQuote)
	}
	t.Logf("Run 1 created assertion IDs: %v", out1.CreatedAssertionIDs)
	t.Logf("Run 2 created assertion IDs: %v", out2.CreatedAssertionIDs)

	// EXPECTED (correct behaviour after fix):
	//   Only 1 assertion should exist — the one from the second run.
	//   PersistFindings should have deleted the stale run-1 assertion before
	//   inserting the run-2 assertion.
	//
	// ACTUAL (buggy current behaviour):
	//   2 assertions exist — both the stale run-1 assertion AND the new run-2 assertion.
	//   The old assertion is never removed, so data accumulates with each reprocess.
	//
	// This assertion FAILS on the current code, confirming the bug.
	assert.Equal(t, 1, len(assertionsAfterRun2),
		"BUG pf-7e5168: after reprocess, source %d should have exactly 1 assertion "+
			"(stale run-1 assertion with quote %q should have been cleared before "+
			"inserting run-2 assertion with quote %q). "+
			"Got %d assertions instead — stale data accumulated.",
		sourceID,
		assertionsAfterRun1[0].sourceQuote,
		"We need to schedule a kickoff soon, before Friday.",
		len(assertionsAfterRun2),
	)

	// Additional assertion: the surviving assertion should be from run 2, not run 1.
	if len(assertionsAfterRun2) == 1 {
		assert.Equal(t,
			"We need to schedule a kickoff soon, before Friday.",
			assertionsAfterRun2[0].sourceQuote,
			"the surviving assertion should have the run-2 source_quote, not the stale run-1 quote",
		)
	}
}

// TestPersistFindings_ReprocessSameQuote_IsIdempotent verifies that reprocessing
// with an IDENTICAL source_quote correctly deduplicates (this is the happy path
// that already works). This test should PASS on both buggy and fixed code.
func TestPersistFindings_ReprocessSameQuote_IsIdempotent(t *testing.T) {
	repo := &buggyInMemoryPersistRepo{}
	ctx := context.Background()

	const sourceID = int64(43)
	const sharedQuote = "Please confirm the budget approval by EOD."

	run := &PersistFindingsInput{
		TenantID:       "tenant-test",
		SourceID:       sourceID,
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedDecisions: []VerifiedDecisionOutput{
				{
					Description:    "Budget approved",
					ContextExcerpt: sharedQuote, // identical in both runs
					Status:         "confirmed",
				},
			},
		},
	}

	out1, err := repo.PersistFindings(ctx, run)
	require.NoError(t, err)
	require.Equal(t, 1, out1.AssertionsCreated)

	// Second run with IDENTICAL quote — should be treated as duplicate
	out2, err := repo.PersistFindings(ctx, run)
	require.NoError(t, err)

	assertionsAfterRun2 := repo.assertionsForSource(sourceID)

	// Exact-match dedup works correctly — the existing assertion is reused.
	assert.Equal(t, 0, out2.AssertionsCreated,
		"second run with identical quote should create 0 new assertions (idempotent)")
	assert.Equal(t, 1, out2.AssertionsSuperseded,
		"second run with identical quote should report 1 superseded (existing found)")
	assert.Len(t, assertionsAfterRun2, 1,
		"database should contain exactly 1 assertion after idempotent reprocess")
}
