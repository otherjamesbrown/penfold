package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestTrackedProductsProvider_Name(t *testing.T) {
	p := NewTrackedProductsProvider(nil, logging.NewNopLogger())
	require.Equal(t, "tracked_products", p.Name())
}

func TestTrackedProductsProvider_Build_NoRepo(t *testing.T) {
	// nil repo → empty string, no error
	p := &TrackedProductsProvider{}
	result, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestTrackedProductsProvider_Build_NoProducts(t *testing.T) {
	repo := &mockNewsletterContextRepo{}
	p := NewTrackedProductsProvider(repo, logging.NewNopLogger())

	result, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestTrackedProductsProvider_Build_WithProducts(t *testing.T) {
	repo := &mockNewsletterContextRepo{
		listActiveProductsFn: func(_ context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
			require.Equal(t, "t1", tenantID)
			require.Equal(t, 20, limit)
			return []NewsletterNameDescription{
				{Name: "Alpha", Description: "First product"},
				{Name: "Beta", Description: ""},
			}, nil
		},
	}
	p := NewTrackedProductsProvider(repo, logging.NewNopLogger())

	result, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	require.Contains(t, result, "### Tracked Products")
	require.Contains(t, result, "**Alpha**: First product")
	require.Contains(t, result, "**Beta**")
}

func TestTrackedProductsProvider_Build_DBError(t *testing.T) {
	// Non-fatal error: provider returns empty string, no error propagated
	repo := &mockNewsletterContextRepo{
		listActiveProductsFn: func(_ context.Context, _ string, _ int) ([]NewsletterNameDescription, error) {
			return nil, errors.New("db connection lost")
		},
	}
	p := NewTrackedProductsProvider(repo, logging.NewNopLogger())

	result, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestTrackedProductsProvider_RegisteredInRegistry(t *testing.T) {
	_, ok := providerRegistry["tracked_products"]
	require.True(t, ok, "tracked_products must be registered in providerRegistry")
}
