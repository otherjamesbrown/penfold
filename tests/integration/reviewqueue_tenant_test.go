//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewQueue_TenantIsolation_GetStats verifies that GetStats respects tenant isolation.
// This test reproduces bug pf-7f950e where GetStats leaks cross-tenant data.
//
// Root cause: Repository GetStats() method has no tenant_id parameter.
// Expected behavior: GetStats should accept tenant_id and only count items for that tenant.
// Actual behavior (bug): GetStats counts ALL items across all tenants.
//
// This test MUST FAIL with the current code to demonstrate the bug.
func TestReviewQueue_TenantIsolation_GetStats(t *testing.T) {
	db := SetupTestDB(t)
	repo := reviewqueue.NewRepository(db.Pool)
	ctx := context.Background()

	// Create unique tenant IDs for this test
	tenant1ID := "11111111-0000-0000-0000-000000000001"
	tenant2ID := "22222222-0000-0000-0000-000000000002"

	// Cleanup any existing test data
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, "DELETE FROM review_queue WHERE tenant_id IN ($1, $2)", tenant1ID, tenant2ID)
	})

	// Create items for tenant 1
	tenant1Items := []reviewqueue.ReviewItemInput{
		{
			QuestionType: reviewqueue.QuestionTypeAcronym,
			Priority:     reviewqueue.PriorityHigh,
			Question:     "What does TER mean?",
			Context:      "Tenant 1 context",
			Confidence:   0.9,
		},
		{
			QuestionType: reviewqueue.QuestionTypeAcronym,
			Priority:     reviewqueue.PriorityMedium,
			Question:     "What does SLI mean?",
			Context:      "Tenant 1 context",
			Confidence:   0.7,
		},
		{
			QuestionType: reviewqueue.QuestionTypePerson,
			Priority:     reviewqueue.PriorityHigh,
			Question:     "Who is John?",
			Context:      "Tenant 1 context",
			Confidence:   0.8,
		},
	}

	// Create items for tenant 2
	tenant2Items := []reviewqueue.ReviewItemInput{
		{
			QuestionType: reviewqueue.QuestionTypeAcronym,
			Priority:     reviewqueue.PriorityLow,
			Question:     "What does API mean?",
			Context:      "Tenant 2 context",
			Confidence:   0.5,
		},
		{
			QuestionType: reviewqueue.QuestionTypePerson,
			Priority:     reviewqueue.PriorityMedium,
			Question:     "Who is Alice?",
			Context:      "Tenant 2 context",
			Confidence:   0.6,
		},
	}

	// Insert tenant 1 items
	for _, input := range tenant1Items {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO review_queue (
				tenant_id, question_type, priority, question, context, confidence, status
			) VALUES ($1, $2, $3, $4, $5, $6, 'pending')
		`, tenant1ID, input.QuestionType, input.Priority, input.Question, input.Context, input.Confidence)
		require.NoError(t, err, "failed to insert tenant 1 item")
	}

	// Insert tenant 2 items
	for _, input := range tenant2Items {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO review_queue (
				tenant_id, question_type, priority, question, context, confidence, status
			) VALUES ($1, $2, $3, $4, $5, $6, 'pending')
		`, tenant2ID, input.QuestionType, input.Priority, input.Question, input.Context, input.Confidence)
		require.NoError(t, err, "failed to insert tenant 2 item")
	}

	// Get stats for tenant 1 - should now be scoped to tenant
	stats, err := repo.GetStats(ctx, tenant1ID)
	require.NoError(t, err, "GetStats should not error")

	// After fix: stats should only include tenant 1 items
	assert.Equal(t, 3, stats.TotalPending,
		"GetStats should return only tenant 1 items (3), not combined with tenant 2 (2)")

	// Verify the breakdown is tenant-isolated
	// Tenant 1: 2 acronym (TER, SLI), 1 person (John)
	assert.Equal(t, 2, stats.ByType["acronym"], "GetStats ByType should show only tenant 1 acronyms")
	assert.Equal(t, 1, stats.ByType["person"], "GetStats ByType should show only tenant 1 persons")

	// Priority counts for tenant 1 only
	// High: 2 (TER acronym + John person)
	// Medium: 1 (SLI acronym)
	assert.Equal(t, 2, stats.ByPriority["high"], "GetStats ByPriority should show only tenant 1 high priority items")
	assert.Equal(t, 1, stats.ByPriority["medium"], "GetStats ByPriority should show only tenant 1 medium priority items")
	assert.Equal(t, 0, stats.ByPriority["low"], "GetStats ByPriority should show no low priority items for tenant 1")
}

// TestReviewQueue_TenantIsolation_List verifies that List respects tenant isolation.
// This test verifies that the v0.9.6 fix for List() is working correctly.
func TestReviewQueue_TenantIsolation_List(t *testing.T) {
	db := SetupTestDB(t)
	repo := reviewqueue.NewRepository(db.Pool)
	ctx := context.Background()

	// Create unique tenant IDs for this test
	tenant1ID := "33333333-0000-0000-0000-000000000001"
	tenant2ID := "44444444-0000-0000-0000-000000000002"

	// Cleanup any existing test data
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, "DELETE FROM review_queue WHERE tenant_id IN ($1, $2)", tenant1ID, tenant2ID)
	})

	// Insert items for tenant 1
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO review_queue (
			tenant_id, question_type, priority, question, context, confidence, status
		) VALUES ($1, 'acronym', 'high', 'Tenant 1 question', 'context', 0.9, 'pending')
	`, tenant1ID)
	require.NoError(t, err)

	// Insert items for tenant 2
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO review_queue (
			tenant_id, question_type, priority, question, context, confidence, status
		) VALUES ($1, 'acronym', 'high', 'Tenant 2 question', 'context', 0.8, 'pending')
	`, tenant2ID)
	require.NoError(t, err)

	// List items for tenant 1 only
	filter := reviewqueue.ReviewFilter{
		TenantID: tenant1ID,
		Status:   reviewqueue.StatusPending,
	}
	items, err := repo.List(ctx, filter)
	require.NoError(t, err, "List should not error")

	// Should only get tenant 1's item
	assert.Len(t, items, 1, "List should return only tenant 1 items")
	if len(items) > 0 {
		assert.Equal(t, tenant1ID, items[0].TenantID, "Returned item should belong to tenant 1")
		assert.Contains(t, items[0].Question, "Tenant 1", "Should return tenant 1 data")
	}

	// List items for tenant 2 only
	filter.TenantID = tenant2ID
	items, err = repo.List(ctx, filter)
	require.NoError(t, err, "List should not error")

	// Should only get tenant 2's item
	assert.Len(t, items, 1, "List should return only tenant 2 items")
	if len(items) > 0 {
		assert.Equal(t, tenant2ID, items[0].TenantID, "Returned item should belong to tenant 2")
		assert.Contains(t, items[0].Question, "Tenant 2", "Should return tenant 2 data")
	}
}

// TestReviewQueue_TenantIsolation_GetNext verifies that GetNext respects tenant isolation.
// This test reproduces the bug where GetNext doesn't filter by tenant_id.
func TestReviewQueue_TenantIsolation_GetNext(t *testing.T) {
	db := SetupTestDB(t)
	repo := reviewqueue.NewRepository(db.Pool)
	ctx := context.Background()

	// Create unique tenant IDs for this test
	tenant1ID := "55555555-0000-0000-0000-000000000001"
	tenant2ID := "66666666-0000-0000-0000-000000000002"

	// Cleanup any existing test data
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, "DELETE FROM review_queue WHERE tenant_id IN ($1, $2)", tenant1ID, tenant2ID)
	})

	// Insert older item for tenant 2 (created first, so older timestamp)
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO review_queue (
			tenant_id, question_type, priority, question, context, confidence, status, created_at
		) VALUES ($1, 'acronym', 'high', 'Tenant 2 OLD question', 'context', 0.8, 'pending', NOW() - INTERVAL '2 hours')
	`, tenant2ID)
	require.NoError(t, err)

	// Insert newer item for tenant 1 (created second, so newer timestamp)
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO review_queue (
			tenant_id, question_type, priority, question, context, confidence, status, created_at
		) VALUES ($1, 'acronym', 'high', 'Tenant 1 NEW question', 'context', 0.9, 'pending', NOW())
	`, tenant1ID)
	require.NoError(t, err)

	// Get next item for tenant 1 - should be scoped to tenant
	item, err := repo.GetNext(ctx, reviewqueue.QuestionTypeAcronym, tenant1ID)
	require.NoError(t, err, "GetNext should not error")
	require.NotNil(t, item, "GetNext should return an item")

	// After fix: should return tenant 1's item only
	assert.Equal(t, tenant1ID, item.TenantID,
		"GetNext should return only tenant 1 items")
	assert.Contains(t, item.Question, "Tenant 1 NEW",
		"GetNext returned newest tenant 1 item (only pending item for tenant 1)")
}
