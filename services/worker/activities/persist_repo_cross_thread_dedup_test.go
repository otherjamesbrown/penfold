package activities

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// crossThreadTestAssertion records one assertion row with thread context.
type crossThreadTestAssertion struct {
	id            int64
	sourceID      int64
	threadID      *int64
	assertionType string
	description   string
	sourceQuote   string
}

// crossThreadTestThreadMessage records a thread_messages row.
type crossThreadTestThreadMessage struct {
	threadID    int64
	sourceID    int64
	messageDate time.Time
}

// crossThreadInMemoryPersistRepo is an in-memory implementation that mirrors the
// cross-thread dedup behaviour of PersistFindings after pf-eddc2d:
//   - Resolves thread context from thread_messages
//   - Checks for duplicate source_quote in earlier thread messages before inserting
//   - Deletes + re-inserts for same-source reprocessing (pf-7e5168 fix)
type crossThreadInMemoryPersistRepo struct {
	mu             sync.Mutex
	nextID         int64
	assertions     []crossThreadTestAssertion
	threadMessages []crossThreadTestThreadMessage
}

// addThreadMessage simulates the GroupEmailThread activity adding a source to a thread.
func (r *crossThreadInMemoryPersistRepo) addThreadMessage(threadID, sourceID int64, messageDate time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.threadMessages = append(r.threadMessages, crossThreadTestThreadMessage{
		threadID:    threadID,
		sourceID:    sourceID,
		messageDate: messageDate,
	})
}

// resolveThreadContext mirrors PersistRepo.resolveThreadContext.
func (r *crossThreadInMemoryPersistRepo) resolveThreadContext(sourceID int64) *threadContext {
	for _, tm := range r.threadMessages {
		if tm.sourceID == sourceID {
			return &threadContext{
				threadID:    tm.threadID,
				messageDate: tm.messageDate,
			}
		}
	}
	return nil
}

// isQuotedTextDuplicate mirrors PersistRepo.isQuotedTextDuplicate.
// Checks if the same source_quote exists in an assertion from an earlier message in the same thread.
func (r *crossThreadInMemoryPersistRepo) isQuotedTextDuplicate(sourceID int64, tc *threadContext, sourceQuote string) bool {
	if tc == nil || sourceQuote == "" {
		return false
	}

	for _, a := range r.assertions {
		if a.sourceID == sourceID {
			continue // Skip own source
		}
		// Find the thread message for this assertion's source
		for _, tm := range r.threadMessages {
			if tm.sourceID == a.sourceID && tm.threadID == tc.threadID {
				// Same thread, check if earlier
				if tm.messageDate.Before(tc.messageDate) && a.sourceQuote == sourceQuote {
					return true
				}
			}
		}
	}
	return false
}

// PersistFindings implements the cross-thread dedup behaviour.
func (r *crossThreadInMemoryPersistRepo) PersistFindings(_ context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error) {
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

	// Delete existing assertions for this source (reprocess cleanup)
	var kept []crossThreadTestAssertion
	for _, a := range r.assertions {
		if a.sourceID != input.SourceID {
			kept = append(kept, a)
		}
	}
	r.assertions = kept

	// Resolve thread context
	tc := r.resolveThreadContext(input.SourceID)

	// Process verified actions
	for _, action := range input.Analysis.VerifiedActions {
		assertionType := "action"

		// Cross-thread dedup
		if r.isQuotedTextDuplicate(input.SourceID, tc, action.ContextExcerpt) {
			output.AssertionsDeduplicated++
			continue
		}

		r.nextID++
		id := r.nextID
		r.assertions = append(r.assertions, crossThreadTestAssertion{
			id:            id,
			sourceID:      input.SourceID,
			threadID:      input.ThreadID,
			assertionType: assertionType,
			description:   action.Description,
			sourceQuote:   action.ContextExcerpt,
		})
		output.AssertionsCreated++
		output.CreatedAssertionIDs = append(output.CreatedAssertionIDs, id)
	}

	// Process verified decisions
	for _, decision := range input.Analysis.VerifiedDecisions {
		assertionType := "decision"

		if r.isQuotedTextDuplicate(input.SourceID, tc, decision.ContextExcerpt) {
			output.AssertionsDeduplicated++
			continue
		}

		r.nextID++
		id := r.nextID
		r.assertions = append(r.assertions, crossThreadTestAssertion{
			id:            id,
			sourceID:      input.SourceID,
			threadID:      input.ThreadID,
			assertionType: assertionType,
			description:   decision.Description,
			sourceQuote:   decision.ContextExcerpt,
		})
		output.AssertionsCreated++
		output.CreatedAssertionIDs = append(output.CreatedAssertionIDs, id)
	}

	return output, nil
}

func (r *crossThreadInMemoryPersistRepo) assertionsForSource(sourceID int64) []crossThreadTestAssertion {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []crossThreadTestAssertion
	for _, a := range r.assertions {
		if a.sourceID == sourceID {
			result = append(result, a)
		}
	}
	return result
}

func (r *crossThreadInMemoryPersistRepo) allAssertions() []crossThreadTestAssertion {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]crossThreadTestAssertion, len(r.assertions))
	copy(result, r.assertions)
	return result
}

// TestCrossThreadDedup_QuotedReplyChain tests the primary fix for pf-eddc2d:
// When a reply email quotes the original, assertions extracted from the quoted
// text should be deduplicated against assertions already in the thread.
//
// Scenario:
//   - Email A (root): "Let's schedule a kickoff meeting by end of week."
//   - Email B (reply): quotes A + adds new content
//   - LLM extracts the same assertion from both A and B
//
// Expected: B's duplicate assertion is skipped (deduplicated), B's new assertion is kept.
func TestCrossThreadDedup_QuotedReplyChain(t *testing.T) {
	repo := &crossThreadInMemoryPersistRepo{}
	ctx := context.Background()

	const (
		threadID int64 = 100
		sourceA  int64 = 1001 // Original email
		sourceB  int64 = 1002 // Reply that quotes A
	)

	dateA := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	dateB := time.Date(2026, 2, 15, 14, 0, 0, 0, time.UTC)

	// Simulate GroupEmailThread adding both sources to the same thread
	repo.addThreadMessage(threadID, sourceA, dateA)
	repo.addThreadMessage(threadID, sourceB, dateB)

	sharedQuote := "Let's schedule a kickoff meeting by end of week."

	// Process email A (original)
	outA, err := repo.PersistFindings(ctx, &PersistFindingsInput{
		TenantID:       "tenant-test",
		SourceID:       sourceA,
		ThreadID:       intPtr(threadID),
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedActions: []VerifiedActionOutput{
				{
					Description:    "Schedule kickoff meeting",
					ContextExcerpt: sharedQuote,
					Status:         "confirmed",
				},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, outA.AssertionsCreated, "email A should create 1 assertion")
	assert.Equal(t, 0, outA.AssertionsDeduplicated, "email A has no earlier messages to dedup against")

	// Process email B (reply quoting A)
	// LLM extracts:
	//   1. Same quote from A (should be deduplicated)
	//   2. New content unique to B (should be kept)
	outB, err := repo.PersistFindings(ctx, &PersistFindingsInput{
		TenantID:       "tenant-test",
		SourceID:       sourceB,
		ThreadID:       intPtr(threadID),
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedActions: []VerifiedActionOutput{
				{
					Description:    "Schedule kickoff meeting", // same as A
					ContextExcerpt: sharedQuote,                // identical quote from quoted text
					Status:         "confirmed",
				},
				{
					Description:    "Send agenda to team",
					ContextExcerpt: "I'll send the agenda to the team before the meeting.", // new content in B
					Status:         "confirmed",
				},
			},
		},
	})
	require.NoError(t, err)

	t.Logf("Email B output: created=%d, deduplicated=%d", outB.AssertionsCreated, outB.AssertionsDeduplicated)

	assert.Equal(t, 1, outB.AssertionsDeduplicated,
		"email B should deduplicate 1 assertion (shared quote from A)")
	assert.Equal(t, 1, outB.AssertionsCreated,
		"email B should create 1 new assertion (unique content)")

	// Verify total assertions
	allAssertions := repo.allAssertions()
	assert.Len(t, allAssertions, 2,
		"total assertions should be 2 (1 from A + 1 new from B), not 3")

	assertionsA := repo.assertionsForSource(sourceA)
	assertionsB := repo.assertionsForSource(sourceB)
	assert.Len(t, assertionsA, 1, "source A should have 1 assertion")
	assert.Len(t, assertionsB, 1, "source B should have 1 assertion (the new one)")
	if len(assertionsB) == 1 {
		assert.Equal(t, "I'll send the agenda to the team before the meeting.",
			assertionsB[0].sourceQuote,
			"source B's assertion should be the new content, not the quoted duplicate")
	}
}

// TestCrossThreadDedup_RootEmailNotDeduplicated verifies that the root email's
// assertions are never deduplicated (no earlier messages exist in thread).
func TestCrossThreadDedup_RootEmailNotDeduplicated(t *testing.T) {
	repo := &crossThreadInMemoryPersistRepo{}
	ctx := context.Background()

	const (
		threadID int64 = 200
		sourceA  int64 = 2001
	)

	dateA := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	repo.addThreadMessage(threadID, sourceA, dateA)

	out, err := repo.PersistFindings(ctx, &PersistFindingsInput{
		TenantID:       "tenant-test",
		SourceID:       sourceA,
		ThreadID:       intPtr(threadID),
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedActions: []VerifiedActionOutput{
				{
					Description:    "Root action",
					ContextExcerpt: "Some important action item.",
					Status:         "confirmed",
				},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, out.AssertionsCreated, "root email should create all assertions")
	assert.Equal(t, 0, out.AssertionsDeduplicated, "root email should never have dedup")
}

// TestCrossThreadDedup_NoThreadContext verifies that dedup is skipped entirely
// when the source is not part of any thread (non-threaded email).
func TestCrossThreadDedup_NoThreadContext(t *testing.T) {
	repo := &crossThreadInMemoryPersistRepo{}
	ctx := context.Background()

	// No thread messages — source is not part of a thread
	out, err := repo.PersistFindings(ctx, &PersistFindingsInput{
		TenantID:       "tenant-test",
		SourceID:       3001,
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedActions: []VerifiedActionOutput{
				{
					Description:    "Some action",
					ContextExcerpt: "Do the thing.",
					Status:         "confirmed",
				},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, out.AssertionsCreated, "non-threaded email should create all assertions")
	assert.Equal(t, 0, out.AssertionsDeduplicated, "non-threaded email should have no dedup")
}

// TestCrossThreadDedup_ThreeEmailChain tests a chain A → B → C where C quotes B which quotes A.
func TestCrossThreadDedup_ThreeEmailChain(t *testing.T) {
	repo := &crossThreadInMemoryPersistRepo{}
	ctx := context.Background()

	const (
		threadID int64 = 300
		sourceA  int64 = 3001
		sourceB  int64 = 3002
		sourceC  int64 = 3003
	)

	dateA := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	dateB := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	dateC := time.Date(2026, 2, 15, 14, 0, 0, 0, time.UTC)

	repo.addThreadMessage(threadID, sourceA, dateA)
	repo.addThreadMessage(threadID, sourceB, dateB)
	repo.addThreadMessage(threadID, sourceC, dateC)

	quoteFromA := "We need to finalize the budget before Friday."
	quoteFromB := "I've reviewed the numbers and approve the budget."

	// Process A
	outA, err := repo.PersistFindings(ctx, &PersistFindingsInput{
		TenantID: "tenant-test", SourceID: sourceA, ThreadID: intPtr(threadID),
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedDecisions: []VerifiedDecisionOutput{
				{Description: "Budget deadline", ContextExcerpt: quoteFromA, Status: "confirmed"},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, outA.AssertionsCreated)

	// Process B — quotes A + adds new content
	outB, err := repo.PersistFindings(ctx, &PersistFindingsInput{
		TenantID: "tenant-test", SourceID: sourceB, ThreadID: intPtr(threadID),
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedDecisions: []VerifiedDecisionOutput{
				{Description: "Budget deadline", ContextExcerpt: quoteFromA, Status: "confirmed"},   // quoted from A
				{Description: "Budget approved", ContextExcerpt: quoteFromB, Status: "confirmed"},    // new in B
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, outB.AssertionsDeduplicated, "B should dedup 1 (quote from A)")
	assert.Equal(t, 1, outB.AssertionsCreated, "B should create 1 (new content)")

	// Process C — quotes both A and B + adds new content
	outC, err := repo.PersistFindings(ctx, &PersistFindingsInput{
		TenantID: "tenant-test", SourceID: sourceC, ThreadID: intPtr(threadID),
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedDecisions: []VerifiedDecisionOutput{
				{Description: "Budget deadline", ContextExcerpt: quoteFromA, Status: "confirmed"},   // quoted from A
				{Description: "Budget approved", ContextExcerpt: quoteFromB, Status: "confirmed"},    // quoted from B
				{Description: "Team notified", ContextExcerpt: "I've notified the team about the approved budget.", Status: "confirmed"}, // new in C
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, outC.AssertionsDeduplicated, "C should dedup 2 (quotes from A and B)")
	assert.Equal(t, 1, outC.AssertionsCreated, "C should create 1 (new content)")

	// Verify totals: 3 unique assertions (1 from each email)
	all := repo.allAssertions()
	assert.Len(t, all, 3, "total unique assertions should be 3, not 6")
}

// TestCrossThreadDedup_DifferentTypesSameQuote verifies that cross-thread dedup
// catches duplicates even when the LLM classifies the same quote with different types.
// Bug cause #3 from pf-eddc2d: "Type variation on reprocess"
func TestCrossThreadDedup_DifferentTypesSameQuote(t *testing.T) {
	repo := &crossThreadInMemoryPersistRepo{}
	ctx := context.Background()

	const (
		threadID int64 = 400
		sourceA  int64 = 4001
		sourceB  int64 = 4002
	)

	dateA := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	dateB := time.Date(2026, 2, 15, 14, 0, 0, 0, time.UTC)

	repo.addThreadMessage(threadID, sourceA, dateA)
	repo.addThreadMessage(threadID, sourceB, dateB)

	sharedQuote := "We should address the security vulnerability before launch."

	// Process A — LLM classifies as "action"
	outA, err := repo.PersistFindings(ctx, &PersistFindingsInput{
		TenantID: "tenant-test", SourceID: sourceA, ThreadID: intPtr(threadID),
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedActions: []VerifiedActionOutput{
				{Description: "Fix security vulnerability", ContextExcerpt: sharedQuote, Status: "confirmed"},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, outA.AssertionsCreated)

	// Process B — LLM classifies same quote as "decision" (type variation!)
	outB, err := repo.PersistFindings(ctx, &PersistFindingsInput{
		TenantID: "tenant-test", SourceID: sourceB, ThreadID: intPtr(threadID),
		ResolvedPeople: map[string]int64{},
		Analysis: &DeepAnalyzeOutput{
			VerifiedDecisions: []VerifiedDecisionOutput{
				{Description: "Address security before launch", ContextExcerpt: sharedQuote, Status: "confirmed"},
			},
		},
	})
	require.NoError(t, err)

	// Cross-thread dedup matches on source_quote only (ignoring assertion_type)
	assert.Equal(t, 1, outB.AssertionsDeduplicated,
		"should dedup even though assertion_type differs (action vs decision)")
	assert.Equal(t, 0, outB.AssertionsCreated,
		"no new assertions should be created for B")

	all := repo.allAssertions()
	assert.Len(t, all, 1, "total assertions should be 1 (from A only)")
}

func intPtr(v int64) *int64 {
	return &v
}
