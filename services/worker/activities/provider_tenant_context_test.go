package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// mockTenantContextRepo is a configurable test double for TenantContextRepository.
type mockTenantContextRepo struct {
	entries []TenantContextEntry
	err     error
}

func (m *mockTenantContextRepo) GetActiveEntries(_ context.Context, _ string) ([]TenantContextEntry, error) {
	return m.entries, m.err
}

// mockOperConfig is a configurable test double for OperationalConfigReader.
type mockOperConfig struct {
	values map[string]string
}

func (m *mockOperConfig) GetString(_ context.Context, _, key string) (string, error) {
	if v, ok := m.values[key]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

// ── TenantContextProvider unit tests ─────────────────────────────────────────

func TestTenantContextProvider_Name(t *testing.T) {
	p := NewTenantContextProvider(nil, nil, testLogger())
	assert.Equal(t, "tenant_context", p.Name())
}

func TestTenantContextProvider_NilRepo(t *testing.T) {
	p := NewTenantContextProvider(nil, nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestTenantContextProvider_RepoError(t *testing.T) {
	repo := &mockTenantContextRepo{err: errors.New("db error")}
	p := NewTenantContextProvider(repo, nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestTenantContextProvider_NoEntries(t *testing.T) {
	repo := &mockTenantContextRepo{}
	p := NewTenantContextProvider(repo, nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestTenantContextProvider_AlwaysInjectAppearsForEveryEmail(t *testing.T) {
	repo := &mockTenantContextRepo{
		entries: []TenantContextEntry{
			{
				ID:           1,
				Category:     "identity",
				Label:        "James Brown",
				Details:      map[string]interface{}{"role": "VP Products"},
				AlwaysInject: true,
				Conditions:   nil,
			},
		},
	}
	p := NewTenantContextProvider(repo, nil, testLogger())

	// No matching content — but AlwaysInject=true so it must appear.
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "completely unrelated email about spreadsheets",
		Subject:  "Q4 planning",
	})
	require.NoError(t, err)
	assert.Contains(t, got, "## Personal Context")
	assert.Contains(t, got, "**James Brown** (identity)")
	assert.Contains(t, got, "role=VP Products")
}

func TestTenantContextProvider_MatchOnContentKeyword(t *testing.T) {
	repo := &mockTenantContextRepo{
		entries: []TenantContextEntry{
			{
				ID:       2,
				Category: "vehicle",
				Label:    "Porsche Macan",
				Details:  map[string]interface{}{"reg": "VO72 UHX", "status": "owned"},
				Conditions: []TenantContextCondition{
					{Field: "content", MatchType: "contains", Value: "macan"},
				},
			},
		},
	}
	p := NewTenantContextProvider(repo, nil, testLogger())

	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "Your Porsche Macan service appointment is confirmed.",
		Subject:  "Service Booking",
	})
	require.NoError(t, err)
	assert.Contains(t, got, "## Personal Context")
	assert.Contains(t, got, "**Porsche Macan** (vehicle)")
	assert.Contains(t, got, "reg=VO72 UHX")
	assert.Contains(t, got, "status=owned")
}

func TestTenantContextProvider_ExcludedWhenNoMatch(t *testing.T) {
	repo := &mockTenantContextRepo{
		entries: []TenantContextEntry{
			{
				ID:       3,
				Category: "vehicle",
				Label:    "Porsche Macan",
				Details:  map[string]interface{}{"reg": "VO72 UHX"},
				Conditions: []TenantContextCondition{
					{Field: "content", MatchType: "contains", Value: "macan"},
					{Field: "sender_email", MatchType: "contains", Value: "porsche"},
				},
			},
		},
	}
	p := NewTenantContextProvider(repo, nil, testLogger())

	// Email is about something unrelated — no keyword match.
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID:    "t1",
		Content:     "Your weekly AI newsletter is ready.",
		Subject:     "AI Digest",
		SenderEmail: "newsletter@aiweekly.com",
	})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestTenantContextProvider_MatchOnSenderEmail(t *testing.T) {
	repo := &mockTenantContextRepo{
		entries: []TenantContextEntry{
			{
				ID:       4,
				Category: "vehicle",
				Label:    "Porsche Macan",
				Details:  map[string]interface{}{"reg": "VO72 UHX"},
				Conditions: []TenantContextCondition{
					{Field: "sender_email", MatchType: "contains", Value: "porsche"},
				},
			},
		},
	}
	p := NewTenantContextProvider(repo, nil, testLogger())

	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID:    "t1",
		Content:     "We have a slot available for your vehicle.",
		Subject:     "Service availability",
		SenderEmail: "service@porschecentre.com",
	})
	require.NoError(t, err)
	assert.Contains(t, got, "## Personal Context")
	assert.Contains(t, got, "**Porsche Macan** (vehicle)")
}

func TestTenantContextProvider_MaxEntriesCapRespected(t *testing.T) {
	// Create 15 always-inject entries; default max is 10.
	entries := make([]TenantContextEntry, 15)
	for i := range entries {
		entries[i] = TenantContextEntry{
			ID:           i + 1,
			Category:     "other",
			Label:        "Entry",
			AlwaysInject: true,
		}
	}
	repo := &mockTenantContextRepo{entries: entries}
	p := NewTenantContextProvider(repo, nil, testLogger())

	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)

	// Count "**Entry**" occurrences — should be capped at 10.
	count := 0
	for i := 0; i+len("**Entry**") <= len(got); i++ {
		if got[i:i+len("**Entry**")] == "**Entry**" {
			count++
		}
	}
	assert.Equal(t, 10, count, "should inject at most 10 entries")
}

func TestTenantContextProvider_MaxEntriesFromConfig(t *testing.T) {
	entries := make([]TenantContextEntry, 5)
	for i := range entries {
		entries[i] = TenantContextEntry{
			ID:           i + 1,
			Category:     "other",
			Label:        "Entry",
			AlwaysInject: true,
		}
	}
	repo := &mockTenantContextRepo{entries: entries}
	cfg := &mockOperConfig{values: map[string]string{
		"context.tenant_context.max_entries_per_email": "3",
	}}
	p := NewTenantContextProvider(repo, cfg, testLogger())

	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)

	count := 0
	for i := 0; i+len("**Entry**") <= len(got); i++ {
		if got[i:i+len("**Entry**")] == "**Entry**" {
			count++
		}
	}
	assert.Equal(t, 3, count, "should respect max_entries_per_email from config")
}

func TestTenantContextProvider_ContentScanLengthApplied(t *testing.T) {
	matched := false
	repo := &mockTenantContextRepo{
		entries: []TenantContextEntry{
			{
				ID:       5,
				Category: "vehicle",
				Label:    "Porsche Macan",
				Details:  nil,
				Conditions: []TenantContextCondition{
					{Field: "content", MatchType: "contains", Value: "macan"},
				},
			},
		},
	}
	cfg := &mockOperConfig{values: map[string]string{
		"context.tenant_context.content_scan_length": "10",
	}}
	p := NewTenantContextProvider(repo, cfg, testLogger())

	// "macan" appears at position 50, beyond the scan length of 10.
	content := "hello world                                       macan service"
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  content,
	})
	require.NoError(t, err)
	// The keyword is beyond the scan window, so the entry should not be injected.
	_ = matched
	assert.Empty(t, got)
}

func TestTenantContextProvider_EmptyDetailsNoColon(t *testing.T) {
	repo := &mockTenantContextRepo{
		entries: []TenantContextEntry{
			{
				ID:           6,
				Category:     "identity",
				Label:        "James Brown",
				Details:      nil,
				AlwaysInject: true,
			},
		},
	}
	p := NewTenantContextProvider(repo, nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Contains(t, got, "**James Brown** (identity)")
	// No colon suffix when details is empty.
	assert.NotContains(t, got, "**James Brown** (identity):")
}

func TestTenantContextProvider_AlwaysInjectTakesPrecedenceOverConditions(t *testing.T) {
	repo := &mockTenantContextRepo{
		entries: []TenantContextEntry{
			{
				ID:           7,
				Category:     "identity",
				Label:        "Always Here",
				AlwaysInject: true,
				Conditions: []TenantContextCondition{
					// Condition that would never match.
					{Field: "content", MatchType: "exact", Value: "IMPOSSIBLE_STRING_XYZ"},
				},
			},
		},
	}
	p := NewTenantContextProvider(repo, nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "completely different content",
	})
	require.NoError(t, err)
	assert.Contains(t, got, "**Always Here** (identity)")
}

func TestTenantContextProvider_ToAddressMatchFromParticipants(t *testing.T) {
	repo := &mockTenantContextRepo{
		entries: []TenantContextEntry{
			{
				ID:       8,
				Category: "subscription",
				Label:    "Direct Line Insurance",
				Details:  map[string]interface{}{"policy": "home"},
				Conditions: []TenantContextCondition{
					{Field: "to_address", MatchType: "contains", Value: "james@"},
				},
			},
		},
	}
	p := NewTenantContextProvider(repo, nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "Your renewal is due.",
		ParticipantEmails: []workflows.Participant{
			{Email: "james@example.com", HeaderRole: "to"},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, got, "**Direct Line Insurance** (subscription)")
}

func TestTenantContextProvider_HeaderSectionPresent(t *testing.T) {
	repo := &mockTenantContextRepo{
		entries: []TenantContextEntry{
			{
				ID:           9,
				Category:     "other",
				Label:        "Something",
				AlwaysInject: true,
			},
		},
	}
	p := NewTenantContextProvider(repo, nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.True(t, len(got) > 0)
	assert.Contains(t, got, "## Personal Context")
}
