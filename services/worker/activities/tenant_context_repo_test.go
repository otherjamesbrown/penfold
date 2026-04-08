package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTenantContextRepository is a test double for TenantContextRepository.
type mockTenantContextRepository struct {
	entries []TenantContextEntry
	err     error
}

func (m *mockTenantContextRepository) GetActiveEntries(_ context.Context, tenantID string) ([]TenantContextEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	if tenantID == "" {
		return nil, nil
	}
	return m.entries, nil
}

// ── Interface contract tests ───────────────────────────────────────────────────

func TestTenantContextRepository_EmptyResult(t *testing.T) {
	repo := &mockTenantContextRepository{entries: nil}
	entries, err := repo.GetActiveEntries(context.Background(), "tenant-1")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestTenantContextRepository_EmptyTenantID(t *testing.T) {
	repo := &mockTenantContextRepository{
		entries: []TenantContextEntry{
			{ID: 1, TenantID: "tenant-1", Category: "vehicle", Label: "Car"},
		},
	}
	entries, err := repo.GetActiveEntries(context.Background(), "")
	require.NoError(t, err)
	assert.Nil(t, entries)
}

func TestTenantContextRepository_EntryWithConditions(t *testing.T) {
	repo := &mockTenantContextRepository{
		entries: []TenantContextEntry{
			{
				ID:           1,
				TenantID:     "tenant-1",
				Category:     "vehicle",
				Label:        "Porsche Macan",
				Details:      map[string]interface{}{"reg": "VO72 UHX"},
				AlwaysInject: false,
				Conditions: []TenantContextCondition{
					{ID: 10, ContextID: 1, Field: "content", MatchType: "contains", Value: "macan", CaseSensitive: false},
					{ID: 11, ContextID: 1, Field: "sender_email", MatchType: "contains", Value: "porsche", CaseSensitive: false},
				},
			},
		},
	}

	entries, err := repo.GetActiveEntries(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, 1, e.ID)
	assert.Equal(t, "vehicle", e.Category)
	assert.Equal(t, "Porsche Macan", e.Label)
	assert.Equal(t, "VO72 UHX", e.Details["reg"])
	assert.False(t, e.AlwaysInject)
	require.Len(t, e.Conditions, 2)
	assert.Equal(t, "content", e.Conditions[0].Field)
	assert.Equal(t, "contains", e.Conditions[0].MatchType)
	assert.Equal(t, "macan", e.Conditions[0].Value)
}

func TestTenantContextRepository_EntryWithoutConditions(t *testing.T) {
	repo := &mockTenantContextRepository{
		entries: []TenantContextEntry{
			{
				ID:           2,
				TenantID:     "tenant-1",
				Category:     "identity",
				Label:        "James Brown",
				Details:      map[string]interface{}{"role": "owner"},
				AlwaysInject: true,
				Conditions:   []TenantContextCondition{},
			},
		},
	}

	entries, err := repo.GetActiveEntries(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "identity", e.Category)
	assert.True(t, e.AlwaysInject)
	assert.Empty(t, e.Conditions)
}

func TestTenantContextRepository_ErrorPropagated(t *testing.T) {
	repo := &mockTenantContextRepository{err: errors.New("db error")}
	_, err := repo.GetActiveEntries(context.Background(), "tenant-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

// ── PostgresTenantContextRepository unit tests ────────────────────────────────

func TestPostgresTenantContextRepository_EmptyTenantID(t *testing.T) {
	// No DB needed — empty tenantID returns immediately.
	repo := &PostgresTenantContextRepository{pool: nil}
	entries, err := repo.GetActiveEntries(context.Background(), "")
	require.NoError(t, err)
	assert.Nil(t, entries)
}
