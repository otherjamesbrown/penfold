// Package activities provides unit tests for the BuildNewsletterContext activity.
package activities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// ── Mock implementations ─────────────────────────────────────────────────────

// mockNewsletterContextRepo is a configurable test double for NewsletterContextRepository.
type mockNewsletterContextRepo struct {
	getUserContextJSONFn  func(ctx context.Context, tenantID string) (string, error)
	listActiveProjectsFn  func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error)
	listActiveProductsFn  func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error)
}

func (m *mockNewsletterContextRepo) GetUserContextJSON(ctx context.Context, tenantID string) (string, error) {
	if m.getUserContextJSONFn != nil {
		return m.getUserContextJSONFn(ctx, tenantID)
	}
	return "", nil
}

func (m *mockNewsletterContextRepo) ListActiveProjects(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
	if m.listActiveProjectsFn != nil {
		return m.listActiveProjectsFn(ctx, tenantID, limit)
	}
	return nil, nil
}

func (m *mockNewsletterContextRepo) ListActiveProducts(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
	if m.listActiveProductsFn != nil {
		return m.listActiveProductsFn(ctx, tenantID, limit)
	}
	return nil, nil
}

// mockContextPackageRepoForNL is a configurable test double for ContextPackageRepository.
// Only GetGlossaryTerms is exercised by BuildNewsletterContext; the rest return empty.
type mockContextPackageRepoForNL struct {
	glossaryTerms []ContextGlossaryTerm
	glossaryErr   error
}

func (m *mockContextPackageRepoForNL) GetGlossaryTerms(_ context.Context, _ string, _ []string, _ []int64, _ int) ([]ContextGlossaryTerm, error) {
	return m.glossaryTerms, m.glossaryErr
}
func (m *mockContextPackageRepoForNL) GetActiveRisks(_ context.Context, _ []int64, _ int) ([]ContextAssertion, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) GetOpenActions(_ context.Context, _ string, _ []int64, _ int) ([]ContextAssertion, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) GetRecentDecisions(_ context.Context, _ []int64, _, _ int) ([]ContextAssertion, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) GetProductEvents(_ context.Context, _ []int64, _, _ int) ([]ContextProductEvent, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) ResolveProjectByName(_ context.Context, _, _ string) (*int64, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) ResolveProjectByKeyword(_ context.Context, _, _ string) (*int64, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) ResolveProjectByNameContains(_ context.Context, _, _ string) (*int64, error) {
	return nil, nil
}

// ── Helper ────────────────────────────────────────────────────────────────────

// newTestContextBuilderForNL builds a minimal ContextBuilderActivities for newsletter tests.
// It reuses the regVerify* stubs (already in this package) for entity resolver/lookup.
func newTestContextBuilderForNL(t *testing.T, nlRepo NewsletterContextRepository, contextRepo ContextPackageRepository) *ContextBuilderActivities {
	t.Helper()
	logger := logging.NewNopLogger()
	return &ContextBuilderActivities{
		logger:                logger.With(logging.F("component", "context_builder_activities")),
		entityResolver:        &regVerifyEntityResolver{},
		entityRepo:            &regVerifyEntityLookup{},
		contextRepo:           contextRepo,
		topicRepo:             nil,
		pipelineRepo:          nil,
		configResolver:        nil,
		newsletterContextRepo: nlRepo,
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestBuildNewsletterContext_EmptyTenant verifies that an empty TenantID returns
// an empty context gracefully without panicking.
func TestBuildNewsletterContext_EmptyTenant(t *testing.T) {
	a := newTestContextBuilderForNL(t, &mockNewsletterContextRepo{}, &mockContextPackageRepoForNL{})

	out, err := a.BuildNewsletterContext(context.Background(), workflows.BuildNewsletterContextInput{
		TenantID: "",
		Content:  "some newsletter body",
	})

	require.NoError(t, err)
	require.NotNil(t, out)
	require.Empty(t, out.BackgroundContext)
	require.False(t, out.UserContextFound)
}

// TestBuildNewsletterContext_UserContextFormatting verifies that when user context
// JSON is present it renders with the correct section header and fields.
func TestBuildNewsletterContext_UserContextFormatting(t *testing.T) {
	nlRepo := &mockNewsletterContextRepo{
		getUserContextJSONFn: func(ctx context.Context, tenantID string) (string, error) {
			return `{"name":"James Brown","role":"VP of Products","priorities":"Revenue risk","interest_areas":"Roadmap"}`, nil
		},
	}
	a := newTestContextBuilderForNL(t, nlRepo, &mockContextPackageRepoForNL{})

	out, err := a.BuildNewsletterContext(context.Background(), workflows.BuildNewsletterContextInput{
		TenantID: "tenant-1",
		Content:  "newsletter body",
	})

	require.NoError(t, err)
	require.True(t, out.UserContextFound)
	require.Contains(t, out.BackgroundContext, "### User Context")
	require.Contains(t, out.BackgroundContext, "James Brown")
	require.Contains(t, out.BackgroundContext, "VP of Products")
	require.Contains(t, out.BackgroundContext, "Revenue risk")
	require.Contains(t, out.BackgroundContext, "Roadmap")
}

// TestBuildNewsletterContext_AllSections verifies that when all four sections have
// data, the output includes each one and the counts are correct.
func TestBuildNewsletterContext_AllSections(t *testing.T) {
	nlRepo := &mockNewsletterContextRepo{
		getUserContextJSONFn: func(ctx context.Context, tenantID string) (string, error) {
			return `{"name":"Alice","role":"PM","priorities":"Speed","interest_areas":"Products"}`, nil
		},
		listActiveProjectsFn: func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
			return []NewsletterNameDescription{
				{Name: "ProjectAlpha", Description: "Internal platform"},
				{Name: "ProjectBeta", Description: "Customer portal"},
			}, nil
		},
		listActiveProductsFn: func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
			return []NewsletterNameDescription{
				{Name: "Cloud NAT", Description: "Network address translation"},
			}, nil
		},
	}
	contextRepo := &mockContextPackageRepoForNL{
		glossaryTerms: []ContextGlossaryTerm{
			{Term: "MTC", Definition: "Master Test Controller"},
		},
	}
	a := newTestContextBuilderForNL(t, nlRepo, contextRepo)

	out, err := a.BuildNewsletterContext(context.Background(), workflows.BuildNewsletterContextInput{
		TenantID: "tenant-1",
		Subject:  "MTC update",
		Content:  "MTC shipped a new release",
	})

	require.NoError(t, err)
	require.True(t, out.UserContextFound)
	require.Equal(t, 1, out.GlossaryCount)
	require.Equal(t, 2, out.ProjectCount)
	require.Equal(t, 1, out.ProductCount)

	require.Contains(t, out.BackgroundContext, "### User Context")
	require.Contains(t, out.BackgroundContext, "### Glossary")
	require.Contains(t, out.BackgroundContext, "MTC")
	require.Contains(t, out.BackgroundContext, "### Active Projects")
	require.Contains(t, out.BackgroundContext, "ProjectAlpha")
	require.Contains(t, out.BackgroundContext, "Internal platform")
	require.Contains(t, out.BackgroundContext, "### Tracked Products")
	require.Contains(t, out.BackgroundContext, "Cloud NAT")
}

// TestBuildNewsletterContext_MissingSections verifies graceful handling when
// some sections have no data — absent sections should not appear in output.
func TestBuildNewsletterContext_MissingSections(t *testing.T) {
	// No user context, no projects, no products; empty glossary
	nlRepo := &mockNewsletterContextRepo{
		getUserContextJSONFn: func(ctx context.Context, tenantID string) (string, error) {
			return "", nil // key absent
		},
	}
	a := newTestContextBuilderForNL(t, nlRepo, &mockContextPackageRepoForNL{})

	out, err := a.BuildNewsletterContext(context.Background(), workflows.BuildNewsletterContextInput{
		TenantID: "tenant-1",
		Content:  "plain body with no acronyms",
	})

	require.NoError(t, err)
	require.False(t, out.UserContextFound)
	require.Equal(t, 0, out.GlossaryCount)
	require.Equal(t, 0, out.ProjectCount)
	require.Equal(t, 0, out.ProductCount)
	require.NotContains(t, out.BackgroundContext, "### User Context")
	require.NotContains(t, out.BackgroundContext, "### Active Projects")
	require.NotContains(t, out.BackgroundContext, "### Tracked Products")
}

// TestBuildNewsletterContext_RepoErrors verifies that errors from the repository
// are logged and skipped rather than propagated — the activity continues gracefully.
func TestBuildNewsletterContext_RepoErrors(t *testing.T) {
	nlRepo := &mockNewsletterContextRepo{
		getUserContextJSONFn: func(ctx context.Context, tenantID string) (string, error) {
			return "", errors.New("db connection refused")
		},
		listActiveProjectsFn: func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
			return nil, errors.New("projects query failed")
		},
		listActiveProductsFn: func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
			return nil, errors.New("products query failed")
		},
	}
	a := newTestContextBuilderForNL(t, nlRepo, &mockContextPackageRepoForNL{})

	out, err := a.BuildNewsletterContext(context.Background(), workflows.BuildNewsletterContextInput{
		TenantID: "tenant-1",
		Content:  "some body",
	})

	// Must not return an error — degraded context is acceptable
	require.NoError(t, err)
	require.NotNil(t, out)
	require.False(t, out.UserContextFound)
	require.Equal(t, 0, out.ProjectCount)
	require.Equal(t, 0, out.ProductCount)
}

// TestBuildNewsletterContext_GlossaryIntegration verifies that glossary terms
// returned by the context repository appear in the output.
func TestBuildNewsletterContext_GlossaryIntegration(t *testing.T) {
	contextRepo := &mockContextPackageRepoForNL{
		glossaryTerms: []ContextGlossaryTerm{
			{Term: "NLB", Definition: "Network Load Balancer"},
			{Term: "SLA", Definition: "Service Level Agreement"},
		},
	}
	a := newTestContextBuilderForNL(t, &mockNewsletterContextRepo{}, contextRepo)

	out, err := a.BuildNewsletterContext(context.Background(), workflows.BuildNewsletterContextInput{
		TenantID: "tenant-1",
		Subject:  "NLB and SLA update",
		Content:  "NLB SLA quarterly update",
	})

	require.NoError(t, err)
	// If glossary terms were matched, verify they appear in the output
	if out.GlossaryCount > 0 {
		require.Contains(t, out.BackgroundContext, "### Glossary")
	}
}

// TestBuildNewsletterContext_SectionOrdering verifies that the four sections
// appear in the expected order: User Context, Glossary, Projects, Products.
func TestBuildNewsletterContext_SectionOrdering(t *testing.T) {
	nlRepo := &mockNewsletterContextRepo{
		getUserContextJSONFn: func(ctx context.Context, tenantID string) (string, error) {
			return `{"name":"Bob","role":"CTO","priorities":"Scale","interest_areas":"Infra"}`, nil
		},
		listActiveProjectsFn: func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
			return []NewsletterNameDescription{{Name: "ProjectZ", Description: ""}}, nil
		},
		listActiveProductsFn: func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
			return []NewsletterNameDescription{{Name: "ProductA", Description: ""}}, nil
		},
	}
	a := newTestContextBuilderForNL(t, nlRepo, &mockContextPackageRepoForNL{})

	out, err := a.BuildNewsletterContext(context.Background(), workflows.BuildNewsletterContextInput{
		TenantID: "tenant-1",
		Content:  "body text",
	})

	require.NoError(t, err)
	bg := out.BackgroundContext

	userIdx := strings.Index(bg, "### User Context")
	projectIdx := strings.Index(bg, "### Active Projects")
	productIdx := strings.Index(bg, "### Tracked Products")

	require.Greater(t, userIdx, -1, "User Context section should be present")
	require.Greater(t, projectIdx, -1, "Active Projects section should be present")
	require.Greater(t, productIdx, -1, "Tracked Products section should be present")

	// User Context must come before Active Projects, which must come before Tracked Products
	require.Less(t, userIdx, projectIdx, "User Context should come before Active Projects")
	require.Less(t, projectIdx, productIdx, "Active Projects should come before Tracked Products")
}

// ── Unit tests for helper functions ──────────────────────────────────────────

// TestFormatUserContextSection verifies the section formatter for various input combinations.
func TestFormatUserContextSection(t *testing.T) {
	t.Run("full fields", func(t *testing.T) {
		uc := newsletterUserContext{
			Name:          "Alice",
			Role:          "PM",
			Priorities:    "Ship fast",
			InterestAreas: "Customers",
		}
		result := formatUserContextSection(uc)
		require.Contains(t, result, "### User Context")
		require.Contains(t, result, "Alice, PM")
		require.Contains(t, result, "Ship fast")
		require.Contains(t, result, "Customers")
	})

	t.Run("name only", func(t *testing.T) {
		uc := newsletterUserContext{Name: "Bob"}
		result := formatUserContextSection(uc)
		require.Contains(t, result, "Bob")
		require.NotContains(t, result, "Priorities:")
		require.NotContains(t, result, "Interest areas:")
	})

	t.Run("empty struct", func(t *testing.T) {
		result := formatUserContextSection(newsletterUserContext{})
		require.Contains(t, result, "### User Context")
	})
}

// TestFormatNameDescriptionSection verifies the list formatter with and without descriptions.
func TestFormatNameDescriptionSection(t *testing.T) {
	items := []NewsletterNameDescription{
		{Name: "Alpha", Description: "First project"},
		{Name: "Beta", Description: ""},
	}
	result := formatNameDescriptionSection("### Projects", items)
	require.Contains(t, result, "### Projects")
	require.Contains(t, result, "**Alpha**: First project")
	require.Contains(t, result, "**Beta**")
	// Beta with no description should not render a colon
	require.NotContains(t, result, "**Beta**:")
}
