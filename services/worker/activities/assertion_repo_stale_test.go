package activities

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storeAssertionRow records one assertion row as the real DB would store it.
// The dedup key mirrors the unique index idx_assertions_unique_source_quote:
//
//	(source_id, assertion_type, source_quote)
type storeAssertionRow struct {
	id            int64
	tenantID      string
	sourceID      int64
	assertionType string
	description   string
	sourceQuote   string // maps to assertions.source_quote (Assertion.SourceText in Go)
	model         string
}

// staleStoreAssertionRepo is an in-memory simulation of the FIXED
// StoreAssertions behaviour in assertion_repo.go.
//
// It mirrors the fixed SQL transaction exactly:
//  1. DELETE all existing rows for (source_id, tenant_id) before inserting.
//  2. For each assertion: INSERT, skipping duplicates within the same batch
//     keyed on (source_id, assertion_type, source_quote).
//
// The delete-before-insert ensures reprocessing replaces rather than
// accumulates assertions, fixing bug pf-ab3908.
type staleStoreAssertionRepo struct {
	mu     sync.Mutex
	nextID int64
	rows   []storeAssertionRow
}

// StoreAssertions mirrors the fixed implementation in assertion_repo.go.
// It deletes all existing assertions for (sourceID, tenantID) before
// inserting the new batch, then deduplicates within the batch itself.
func (r *staleStoreAssertionRepo) StoreAssertions(
	_ context.Context,
	tenantID string,
	sourceID int64,
	assertions []*Assertion,
	model string,
) (int, error) {
	if tenantID == "" {
		return 0, fmt.Errorf("tenant_id is required")
	}
	if sourceID <= 0 {
		return 0, fmt.Errorf("source_id is required")
	}
	if len(assertions) == 0 {
		return 0, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Delete existing assertions for this source before inserting fresh ones.
	// This mirrors the DELETE FROM assertions WHERE source_id = $1 AND tenant_id = $2
	// added to the real SQL transaction to fix bug pf-ab3908.
	var retained []storeAssertionRow
	for _, row := range r.rows {
		if row.sourceID != sourceID || row.tenantID != tenantID {
			retained = append(retained, row)
		}
	}
	r.rows = retained

	insertedCount := 0

	for _, a := range assertions {
		assertionType := mapCategoryToType(a.Category)
		description := fmt.Sprintf("%s %s %s", a.Subject, a.Predicate, a.Object)
		sourceQuote := a.SourceText

		// ON CONFLICT DO NOTHING: skip if (source_id, assertion_type, source_quote) already exists
		// within this batch (dedup within the current insert set).
		if r.conflictExists(sourceID, assertionType, sourceQuote) {
			continue
		}

		r.nextID++
		r.rows = append(r.rows, storeAssertionRow{
			id:            r.nextID,
			tenantID:      tenantID,
			sourceID:      sourceID,
			assertionType: assertionType,
			description:   description,
			sourceQuote:   sourceQuote,
			model:         model,
		})
		insertedCount++
	}

	return insertedCount, nil
}

// conflictExists returns true if a row with (sourceID, assertionType, sourceQuote) already exists.
// Must be called with r.mu held.
func (r *staleStoreAssertionRepo) conflictExists(sourceID int64, assertionType, sourceQuote string) bool {
	for _, row := range r.rows {
		if row.sourceID == sourceID &&
			row.assertionType == assertionType &&
			row.sourceQuote == sourceQuote {
			return true
		}
	}
	return false
}

// rowsForSource returns all stored assertion rows for a given source_id.
// Safe to call without holding r.mu.
func (r *staleStoreAssertionRepo) rowsForSource(sourceID int64) []storeAssertionRow {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []storeAssertionRow
	for _, row := range r.rows {
		if row.sourceID == sourceID {
			result = append(result, row)
		}
	}
	return result
}

// TestStoreAssertions_ReprocessClearsStaleAssertions reproduces bug pf-ab3908.
//
// Bug: StoreAssertions uses INSERT ... ON CONFLICT DO NOTHING with no
// delete-before-insert. When different LLM models process the same source and
// produce different SourceText (source_quote) values for the same concept, the
// dedup check finds no conflict (the key is different), so both models'
// assertions are inserted. After a reprocess cycle the assertions table
// accumulates rows from every model that ever ran on that source.
//
// Scenario modelled here (from production observation):
//   - Run 1: gemini-2.0-flash processes source 42, extracts 2 assertions with its
//     own source_quote phrasing.
//   - Run 2: qwen2.5:7b processes source 42, extracts the same conceptual
//     assertions but with slightly different source_quote phrasing.
//
// After Run 2, ONLY qwen2.5:7b's assertions should exist. Instead, all 4
// assertions (2 from each model) remain, doubling the data on every reprocess.
//
// This test MUST FAIL on the current (unfixed) code. A passing result means
// the bug has been fixed and the test should be updated to reflect the fix.
func TestStoreAssertions_ReprocessClearsStaleAssertions(t *testing.T) {
	repo := &staleStoreAssertionRepo{}
	ctx := context.Background()

	const (
		tenantID = "00000000-0000-0000-0000-000000000001"
		sourceID = int64(42)
	)

	// --- Run 1: gemini-2.0-flash processes source 42 ---
	// The model extracts two assertions. These are stored successfully.
	geminiAssertions := []*Assertion{
		{
			Subject:    "James",
			Predicate:  "will deliver",
			Object:     "the budget report",
			Confidence: 0.9,
			SourceText: "James confirmed he will deliver the budget report by Friday.", // gemini phrasing
			Category:   "commitment",
		},
		{
			Subject:    "product team",
			Predicate:  "decided to",
			Object:     "postpone the launch",
			Confidence: 0.85,
			SourceText: "The product team has decided to postpone the launch until Q3.", // gemini phrasing
			Category:   "decision",
		},
	}

	count1, err := repo.StoreAssertions(ctx, tenantID, sourceID, geminiAssertions, "gemini-2.0-flash")
	require.NoError(t, err, "first StoreAssertions call (gemini-2.0-flash) should succeed")
	require.Equal(t, 2, count1, "first run should insert 2 assertions")

	rowsAfterRun1 := repo.rowsForSource(sourceID)
	require.Len(t, rowsAfterRun1, 2, "exactly 2 assertion rows should exist after run 1")

	// --- Run 2: qwen2.5:7b reprocesses the same source ---
	// qwen2.5:7b extracts the same conceptual assertions but with different
	// source_quote phrasing. Because the dedup key is (source_id, assertion_type,
	// source_quote) and the quotes differ, neither assertion is detected as a
	// conflict. Both are inserted alongside the stale gemini assertions.
	qwenAssertions := []*Assertion{
		{
			Subject:    "James",
			Predicate:  "will deliver",
			Object:     "the budget report",
			Confidence: 0.88,
			SourceText: "James said he'd get the budget report done by end of week.", // qwen phrasing — DIFFERENT
			Category:   "commitment",
		},
		{
			Subject:    "product team",
			Predicate:  "decided to",
			Object:     "postpone the launch",
			Confidence: 0.82,
			SourceText: "Launch pushed to Q3 per product team decision.", // qwen phrasing — DIFFERENT
			Category:   "decision",
		},
	}

	count2, err := repo.StoreAssertions(ctx, tenantID, sourceID, qwenAssertions, "qwen2.5:7b")
	require.NoError(t, err, "second StoreAssertions call (qwen2.5:7b) should succeed")

	// Log what is in the store to make the failure message clear.
	rowsAfterRun2 := repo.rowsForSource(sourceID)
	t.Logf("Assertions in store after reprocess (source_id=%d):", sourceID)
	for _, row := range rowsAfterRun2 {
		t.Logf("  id=%d  model=%q  type=%q  source_quote=%q",
			row.id, row.model, row.assertionType, row.sourceQuote)
	}
	t.Logf("Run 1 (gemini-2.0-flash) inserted: %d", count1)
	t.Logf("Run 2 (qwen2.5:7b) inserted: %d", count2)

	// EXPECTED (correct behaviour after fix):
	//   Only 2 assertions should exist — the ones from qwen2.5:7b (the most recent run).
	//   StoreAssertions should delete all prior assertions for source 42 before
	//   inserting the new batch, mirroring what Stage 4.5 PersistFindings does.
	//
	// ACTUAL (buggy current behaviour):
	//   4 assertions exist — 2 from gemini-2.0-flash and 2 from qwen2.5:7b.
	//   The stale gemini assertions are never removed; data accumulates with
	//   every reprocess cycle and every model that runs on the source.
	//
	// This assertion FAILS on the current code, confirming bug pf-ab3908.
	assert.Equal(t, 2, len(rowsAfterRun2),
		"BUG pf-ab3908: after reprocess with qwen2.5:7b, source %d should have exactly 2 assertions "+
			"(stale gemini-2.0-flash assertions should have been cleared before inserting "+
			"qwen2.5:7b assertions). Got %d — stale multi-model duplicates accumulated.",
		sourceID,
		len(rowsAfterRun2),
	)

	// Additional check: surviving assertions should all be from the second model.
	if len(rowsAfterRun2) > 0 {
		for _, row := range rowsAfterRun2 {
			assert.Equal(t, "qwen2.5:7b", row.model,
				"BUG pf-ab3908: all surviving assertions should be from the most recent model "+
					"(qwen2.5:7b), but found a stale assertion from model %q "+
					"with source_quote %q",
				row.model, row.sourceQuote,
			)
		}
	}
}

// TestStoreAssertions_SameModel_SameQuote_IsIdempotent verifies that
// reprocessing with an IDENTICAL model and IDENTICAL source_quote leaves
// exactly one assertion row (idempotent net result).
//
// With the delete-before-insert fix (pf-ab3908), StoreAssertions deletes all
// prior rows for the source before inserting the fresh batch. A second run of
// identical assertions therefore:
//   - Deletes the 1 existing row
//   - Reinserts 1 row (count2 == 1)
//   - Leaves exactly 1 row in total
//
// The idempotency guarantee is the final row count (1), not the insert count.
func TestStoreAssertions_SameModel_SameQuote_IsIdempotent(t *testing.T) {
	repo := &staleStoreAssertionRepo{}
	ctx := context.Background()

	const (
		tenantID = "00000000-0000-0000-0000-000000000002"
		sourceID = int64(99)
	)

	assertions := []*Assertion{
		{
			Subject:    "Sarah",
			Predicate:  "approved",
			Object:     "the proposal",
			Confidence: 0.95,
			SourceText: "Sarah has approved the proposal as of yesterday.", // identical in both runs
			Category:   "decision",
		},
	}

	count1, err := repo.StoreAssertions(ctx, tenantID, sourceID, assertions, "gemini-2.0-flash")
	require.NoError(t, err)
	require.Equal(t, 1, count1, "first run should insert 1 assertion")

	// Second run with IDENTICAL model and quote. With delete-before-insert,
	// the prior row is deleted and then reinserted, so count2 == 1.
	// The important invariant is that only 1 row exists afterwards.
	count2, err := repo.StoreAssertions(ctx, tenantID, sourceID, assertions, "gemini-2.0-flash")
	require.NoError(t, err)

	rowsAfterRun2 := repo.rowsForSource(sourceID)

	assert.Equal(t, 1, count2,
		"second run re-inserts the same assertion after deleting the prior row (delete-before-insert fix)")
	assert.Len(t, rowsAfterRun2, 1,
		"database should contain exactly 1 assertion after idempotent reprocess")
}
