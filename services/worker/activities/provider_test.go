package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// ── Shared test helpers ───────────────────────────────────────────────────────

// mockNewsletterContextRepo is a configurable test double for NewsletterContextRepository.
type mockNewsletterContextRepo struct {
	getUserContextJSONFn func(ctx context.Context, tenantID string) (string, error)
	listActiveProjectsFn func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error)
	listActiveProductsFn func(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error)
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

// mockContextPackageRepoForNL is a minimal ContextPackageRepository for provider tests.
// Only GetGlossaryTerms is exercised; the rest return empty/nil.
type mockContextPackageRepoForNL struct {
	glossaryTerms []ContextGlossaryTerm
	glossaryErr   error
}

func (m *mockContextPackageRepoForNL) GetActiveRisks(_ context.Context, _ []int64, _ int) ([]ContextAssertion, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) GetOpenActions(_ context.Context, _ string, _ []int64, _ int) ([]ContextAssertion, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) GetRecentDecisions(_ context.Context, _ []int64, _ int, _ int) ([]ContextAssertion, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) GetProductEvents(_ context.Context, _ []int64, _ int, _ int) ([]ContextProductEvent, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) GetGlossaryTerms(_ context.Context, _ string, _ []string, _ []int64, _ int) ([]ContextGlossaryTerm, error) {
	return m.glossaryTerms, m.glossaryErr
}
func (m *mockContextPackageRepoForNL) ResolveProjectByName(_ context.Context, _ string, _ string) (*int64, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) ResolveProjectByKeyword(_ context.Context, _ string, _ string) (*int64, error) {
	return nil, nil
}
func (m *mockContextPackageRepoForNL) ResolveProjectByNameContains(_ context.Context, _ string, _ string) (*int64, error) {
	return nil, nil
}

// ── Shared test logger ────────────────────────────────────────────────────────

func testLogger() logging.Logger {
	return logging.NewNopLogger()
}

// ── UserContextProvider tests ─────────────────────────────────────────────────

func TestUserContextProvider_Name(t *testing.T) {
	p := NewUserContextProvider(nil, testLogger())
	assert.Equal(t, "user_context", p.Name())
}

func TestUserContextProvider_NilRepo(t *testing.T) {
	p := NewUserContextProvider(nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestUserContextProvider_NoConfigKey(t *testing.T) {
	repo := &mockNewsletterContextRepo{}
	p := NewUserContextProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestUserContextProvider_RepoError(t *testing.T) {
	repo := &mockNewsletterContextRepo{
		getUserContextJSONFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("db error")
		},
	}
	p := NewUserContextProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestUserContextProvider_FullContext(t *testing.T) {
	repo := &mockNewsletterContextRepo{
		getUserContextJSONFn: func(_ context.Context, _ string) (string, error) {
			return `{"name":"James Brown","role":"VP Products","priorities":"Ship MTC by Q2","interest_areas":"infra,AI"}`, nil
		},
	}
	p := NewUserContextProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Contains(t, got, "### User Context")
	assert.Contains(t, got, "James Brown")
	assert.Contains(t, got, "VP Products")
	assert.Contains(t, got, "Ship MTC by Q2")
}

func TestUserContextProvider_InvalidJSON(t *testing.T) {
	repo := &mockNewsletterContextRepo{
		getUserContextJSONFn: func(_ context.Context, _ string) (string, error) {
			return `{not valid json}`, nil
		},
	}
	p := NewUserContextProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// ── GlossaryProvider tests ────────────────────────────────────────────────────

func TestGlossaryProvider_Name(t *testing.T) {
	p := NewGlossaryProvider(nil, testLogger())
	assert.Equal(t, "glossary", p.Name())
}

func TestGlossaryProvider_NilRepo(t *testing.T) {
	p := NewGlossaryProvider(nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "The NLB and AWS services are key.",
	})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGlossaryProvider_NoAcronymsInContent(t *testing.T) {
	repo := &mockContextPackageRepoForNL{}
	p := NewGlossaryProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "just lowercase words here",
	})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGlossaryProvider_RepoError(t *testing.T) {
	repo := &mockContextPackageRepoForNL{
		glossaryErr: errors.New("db error"),
	}
	p := NewGlossaryProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "the NLB is critical",
	})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGlossaryProvider_AcronymsFromContent(t *testing.T) {
	repo := &mockContextPackageRepoForNL{
		glossaryTerms: []ContextGlossaryTerm{
			{Term: "NLB", Expansion: "Network Load Balancer", Definition: "Network Load Balancer"},
		},
	}
	p := NewGlossaryProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "The NLB routing is broken.",
	})
	require.NoError(t, err)
	assert.Contains(t, got, "### Glossary")
	assert.Contains(t, got, "NLB")
	assert.Contains(t, got, "Network Load Balancer")
}

func TestGlossaryProvider_AcronymsFromExtraction(t *testing.T) {
	repo := &mockContextPackageRepoForNL{
		glossaryTerms: []ContextGlossaryTerm{
			{Term: "MTC", Expansion: "Mobile Traffic Controller", Definition: "Mobile Traffic Controller"},
		},
	}
	p := NewGlossaryProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "some email body",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			Projects: []string{"MTC"},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, got, "### Glossary")
	assert.Contains(t, got, "MTC")
}

func TestGlossaryProvider_NoMatchesInDB(t *testing.T) {
	repo := &mockContextPackageRepoForNL{
		glossaryTerms: nil,
	}
	p := NewGlossaryProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "NLB AWS GCP",
	})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// ── ActiveProjectsProvider tests ──────────────────────────────────────────────

func TestActiveProjectsProvider_Name(t *testing.T) {
	p := NewActiveProjectsProvider(nil, testLogger())
	assert.Equal(t, "active_projects", p.Name())
}

func TestActiveProjectsProvider_NilRepo(t *testing.T) {
	p := NewActiveProjectsProvider(nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestActiveProjectsProvider_RepoError(t *testing.T) {
	repo := &mockNewsletterContextRepo{
		listActiveProjectsFn: func(_ context.Context, _ string, _ int) ([]NewsletterNameDescription, error) {
			return nil, errors.New("db error")
		},
	}
	p := NewActiveProjectsProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestActiveProjectsProvider_NoProjects(t *testing.T) {
	repo := &mockNewsletterContextRepo{}
	p := NewActiveProjectsProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestActiveProjectsProvider_WithProjects(t *testing.T) {
	repo := &mockNewsletterContextRepo{
		listActiveProjectsFn: func(_ context.Context, _ string, _ int) ([]NewsletterNameDescription, error) {
			return []NewsletterNameDescription{
				{Name: "MTC", Description: "Mobile Traffic Controller"},
				{Name: "CLIC", Description: ""},
			}, nil
		},
	}
	p := NewActiveProjectsProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{TenantID: "t1"})
	require.NoError(t, err)
	assert.Contains(t, got, "### Active Projects")
	assert.Contains(t, got, "MTC")
	assert.Contains(t, got, "Mobile Traffic Controller")
	assert.Contains(t, got, "CLIC")
}

// ── TopicsProvider tests ──────────────────────────────────────────────────────

// mockTopicRepo is a configurable test double for TopicLookupInterface.
type mockTopicRepo struct {
	listForContextFn       func(ctx context.Context, tenantID string, names []string) ([]TopicResult, error)
	scanContentForTopicsFn func(ctx context.Context, tenantID string, content string) ([]TopicResult, error)
}

func (m *mockTopicRepo) GetByName(_ context.Context, _, _ string) (TopicResult, error) {
	return TopicResult{}, nil
}
func (m *mockTopicRepo) ResolveByKeyword(_ context.Context, _, _ string) (*int64, error) {
	return nil, nil
}
func (m *mockTopicRepo) GetByID(_ context.Context, _ int64) (TopicResult, error) {
	return TopicResult{}, nil
}
func (m *mockTopicRepo) ListForContext(ctx context.Context, tenantID string, names []string) ([]TopicResult, error) {
	if m.listForContextFn != nil {
		return m.listForContextFn(ctx, tenantID, names)
	}
	return nil, nil
}
func (m *mockTopicRepo) ScanContentForTopics(ctx context.Context, tenantID string, content string) ([]TopicResult, error) {
	if m.scanContentForTopicsFn != nil {
		return m.scanContentForTopicsFn(ctx, tenantID, content)
	}
	return nil, nil
}

func TestTopicsProvider_Name(t *testing.T) {
	p := NewTopicsProvider(nil, testLogger())
	assert.Equal(t, "topics", p.Name())
}

func TestTopicsProvider_NilRepo(t *testing.T) {
	p := NewTopicsProvider(nil, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "cloud networking topics",
	})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestTopicsProvider_NoTopicsFound(t *testing.T) {
	repo := &mockTopicRepo{}
	p := NewTopicsProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "some content",
	})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestTopicsProvider_FromBodyScan(t *testing.T) {
	repo := &mockTopicRepo{
		scanContentForTopicsFn: func(_ context.Context, _ string, _ string) ([]TopicResult, error) {
			return []TopicResult{
				{ID: 1, Name: "Cloud NAT", Description: "Network Address Translation in the cloud"},
			}, nil
		},
	}
	p := NewTopicsProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "Cloud NAT configuration",
	})
	require.NoError(t, err)
	assert.Contains(t, got, "### Topic Context")
	assert.Contains(t, got, "Cloud NAT")
	assert.Contains(t, got, "Network Address Translation")
}

func TestTopicsProvider_FromNERExtraction(t *testing.T) {
	repo := &mockTopicRepo{
		listForContextFn: func(_ context.Context, _ string, names []string) ([]TopicResult, error) {
			for _, n := range names {
				if n == "DevCloud" {
					return []TopicResult{
						{ID: 2, Name: "DevCloud", Description: "Internal dev cloud platform"},
					}, nil
				}
			}
			return nil, nil
		},
	}
	p := NewTopicsProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "some email",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			Organisations: []string{"DevCloud"},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, got, "### Topic Context")
	assert.Contains(t, got, "DevCloud")
}

func TestTopicsProvider_DeduplicatesAcrossSources(t *testing.T) {
	repo := &mockTopicRepo{
		listForContextFn: func(_ context.Context, _ string, _ []string) ([]TopicResult, error) {
			return []TopicResult{{ID: 1, Name: "Cloud NAT", Description: "desc"}}, nil
		},
		scanContentForTopicsFn: func(_ context.Context, _ string, _ string) ([]TopicResult, error) {
			// Same topic ID returned from both sources
			return []TopicResult{{ID: 1, Name: "Cloud NAT", Description: "desc"}}, nil
		},
	}
	p := NewTopicsProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "Cloud NAT",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			Organisations: []string{"Cloud NAT"},
		},
	})
	require.NoError(t, err)
	// Should appear exactly once
	count := 0
	for _, line := range splitLines(got) {
		if line == "- **Cloud NAT**: desc" {
			count++
		}
	}
	assert.Equal(t, 1, count, "topic should appear exactly once after dedup")
}

func TestTopicsProvider_SkipsTopicsWithoutDescription(t *testing.T) {
	repo := &mockTopicRepo{
		scanContentForTopicsFn: func(_ context.Context, _ string, _ string) ([]TopicResult, error) {
			return []TopicResult{
				{ID: 1, Name: "WithDesc", Description: "has a description"},
				{ID: 2, Name: "NoDesc", Description: ""},
			}, nil
		},
	}
	p := NewTopicsProvider(repo, testLogger())
	got, err := p.Build(context.Background(), ContextProviderInput{
		TenantID: "t1",
		Content:  "content",
	})
	require.NoError(t, err)
	assert.Contains(t, got, "WithDesc")
	assert.NotContains(t, got, "NoDesc")
}

// ── RegisterContextProviders test ─────────────────────────────────────────────

func TestRegisterContextProviders(t *testing.T) {
	logger := testLogger()
	contextRepo := &mockContextPackageRepoForNL{}
	newsletterRepo := &mockNewsletterContextRepo{}
	topicRepo := &mockTopicRepo{}

	RegisterContextProviders(logger, contextRepo, newsletterRepo, topicRepo, nil, nil)
	t.Cleanup(func() {
		delete(providerRegistry, "user_context")
		delete(providerRegistry, "glossary")
		delete(providerRegistry, "active_projects")
		delete(providerRegistry, "topics")
		delete(providerRegistry, "tenant_context")
	})

	for _, name := range []string{"user_context", "glossary", "active_projects", "topics", "tracked_products", "tenant_context"} {
		p, ok := LookupProvider(name)
		require.True(t, ok, "provider %q should be registered", name)
		assert.Equal(t, name, p.Name())
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
