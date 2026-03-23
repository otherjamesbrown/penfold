package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider is a test-only ContextProvider implementation.
type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Build(_ context.Context, _ ContextProviderInput) (string, error) {
	return "## " + m.name, nil
}

func TestLookupProvider_Found(t *testing.T) {
	mock := &mockProvider{name: "test_provider"}
	providerRegistry["test_provider"] = mock
	t.Cleanup(func() { delete(providerRegistry, "test_provider") })

	got, ok := LookupProvider("test_provider")
	require.True(t, ok)
	assert.Equal(t, "test_provider", got.Name())
}

func TestLookupProvider_NotFound(t *testing.T) {
	_, ok := LookupProvider("nonexistent_provider")
	assert.False(t, ok)
}
