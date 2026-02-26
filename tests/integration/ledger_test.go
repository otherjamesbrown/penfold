//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/ledger"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLedgerRepo(t *testing.T) (*ledger.Repository, *TestDB) {
	t.Helper()
	db := SetupTestDBNoMigrations(t)
	db.EnsureTenantExists(t)

	// Clean up ledger entries for the test tenant.
	ctx := context.Background()
	_, err := db.Pool.Exec(ctx,
		"DELETE FROM session_ledger_entries WHERE tenant_id = $1", db.TenantID)
	require.NoError(t, err)

	repo := ledger.NewRepository(db.Pool, logging.NewNopLogger())
	return repo, db
}

func TestLedgerCreateAndGetEntry(t *testing.T) {
	repo, db := setupLedgerRepo(t)
	ctx := context.Background()

	entry := &ledger.Entry{
		TenantID:  db.TenantID,
		SessionID: "test-create-001",
		EntryType: "decision",
		Title:     "Test decision",
		Body:      ledgerStrPtr("Because reasons"),
		Source:    "manual",
		Agent:     "agent-penfold",
	}

	err := repo.CreateEntry(ctx, entry)
	require.NoError(t, err)
	assert.NotZero(t, entry.ID, "entry ID should be auto-generated")
	assert.False(t, entry.CreatedAt.IsZero(), "created_at should be set")

	// GetEntry
	got, err := repo.GetEntry(ctx, db.TenantID, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, entry.ID, got.ID)
	assert.Equal(t, "decision", got.EntryType)
	assert.Equal(t, "Test decision", got.Title)
	assert.Equal(t, "Because reasons", *got.Body)
}

func TestLedgerListSessionsEmptyLabels(t *testing.T) {
	repo, db := setupLedgerRepo(t)
	ctx := context.Background()

	// Create entries with NO labels (empty array) — this is the bug trigger.
	err := repo.CreateEntry(ctx, &ledger.Entry{
		TenantID:  db.TenantID,
		SessionID: "test-sessions-001",
		EntryType: "decision",
		Title:     "Test decision",
		Body:      ledgerStrPtr("Because reasons"),
		Source:    "manual",
		Agent:     "agent-penfold",
	})
	require.NoError(t, err)

	err = repo.CreateEntry(ctx, &ledger.Entry{
		TenantID:  db.TenantID,
		SessionID: "test-sessions-001",
		EntryType: "narrative",
		Title:     "Some context",
		Source:    "manual",
		Agent:     "agent-penfold",
	})
	require.NoError(t, err)

	err = repo.CreateEntry(ctx, &ledger.Entry{
		TenantID:  db.TenantID,
		SessionID: "test-sessions-002",
		EntryType: "discovery",
		Title:     "Found something",
		Source:    "manual",
		Agent:     "agent-mycroft",
	})
	require.NoError(t, err)

	// ListSessions — this was failing with "cannot accumulate empty arrays".
	summaries, total, err := repo.ListSessions(ctx, ledger.SessionFilter{
		TenantID: db.TenantID,
	})
	require.NoError(t, err, "ListSessions should not error on empty label arrays")
	assert.Equal(t, int32(2), total)
	assert.Len(t, summaries, 2)

	// Verify session summaries.
	sessionMap := map[string]*ledger.SessionSummary{}
	for _, s := range summaries {
		sessionMap[s.SessionID] = s
	}

	s1 := sessionMap["test-sessions-001"]
	require.NotNil(t, s1)
	assert.Equal(t, int64(2), s1.EntryCount)
	assert.Empty(t, s1.Labels, "labels should be empty, not error")

	s2 := sessionMap["test-sessions-002"]
	require.NotNil(t, s2)
	assert.Equal(t, int64(1), s2.EntryCount)
}

func TestLedgerListSessionsWithLabels(t *testing.T) {
	repo, db := setupLedgerRepo(t)
	ctx := context.Background()

	// Mix of entries: some with labels, some without.
	err := repo.CreateEntry(ctx, &ledger.Entry{
		TenantID:  db.TenantID,
		SessionID: "test-labels-001",
		EntryType: "decision",
		Title:     "Labeled entry",
		Source:    "manual",
		Agent:     "agent-penfold",
		Labels:    []string{"important", "architecture"},
	})
	require.NoError(t, err)

	err = repo.CreateEntry(ctx, &ledger.Entry{
		TenantID:  db.TenantID,
		SessionID: "test-labels-001",
		EntryType: "narrative",
		Title:     "No labels entry",
		Source:    "manual",
		Agent:     "agent-penfold",
	})
	require.NoError(t, err)

	summaries, _, err := repo.ListSessions(ctx, ledger.SessionFilter{
		TenantID: db.TenantID,
	})
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	assert.ElementsMatch(t, []string{"important", "architecture"}, summaries[0].Labels)
}

func TestLedgerGetSessionEmptyLabels(t *testing.T) {
	repo, db := setupLedgerRepo(t)
	ctx := context.Background()

	err := repo.CreateEntry(ctx, &ledger.Entry{
		TenantID:  db.TenantID,
		SessionID: "test-getsession-001",
		EntryType: "narrative",
		Title:     "Entry with no labels",
		Source:    "manual",
		Agent:     "agent-penfold",
	})
	require.NoError(t, err)

	// GetSession has the same ARRAY_AGG pattern — verify it works too.
	summary, entries, err := repo.GetSession(ctx, db.TenantID, "test-getsession-001")
	require.NoError(t, err)
	assert.Equal(t, int64(1), summary.EntryCount)
	assert.Empty(t, summary.Labels)
	assert.Len(t, entries, 1)
}

func TestLedgerSearchEntries(t *testing.T) {
	repo, db := setupLedgerRepo(t)
	ctx := context.Background()

	err := repo.CreateEntry(ctx, &ledger.Entry{
		TenantID:  db.TenantID,
		SessionID: "test-search-001",
		EntryType: "decision",
		Title:     "Test decision",
		Body:      ledgerStrPtr("Because reasons"),
		Source:    "manual",
		Agent:     "agent-penfold",
	})
	require.NoError(t, err)

	results, total, err := repo.SearchEntries(ctx, ledger.SearchFilter{
		TenantID: db.TenantID,
		Query:    "reasons",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), total)
	require.Len(t, results, 1)
	assert.Equal(t, "Test decision", results[0].Title)
}

func TestLedgerGetLatestHandoff(t *testing.T) {
	repo, db := setupLedgerRepo(t)
	ctx := context.Background()

	// No handoff entries — should return not found.
	_, err := repo.GetLatestHandoff(ctx, db.TenantID, nil)
	assert.ErrorIs(t, err, ledger.ErrNotFound)

	// Create a handoff entry.
	err = repo.CreateEntry(ctx, &ledger.Entry{
		TenantID:  db.TenantID,
		SessionID: "test-handoff-001",
		EntryType: "handoff",
		Title:     "Handoff - test",
		Source:    "manual",
		Agent:     "agent-penfold",
	})
	require.NoError(t, err)

	handoff, err := repo.GetLatestHandoff(ctx, db.TenantID, nil)
	require.NoError(t, err)
	assert.Equal(t, "Handoff - test", handoff.Title)
	assert.Equal(t, "handoff", handoff.EntryType)
}

func ledgerStrPtr(s string) *string {
	return &s
}
