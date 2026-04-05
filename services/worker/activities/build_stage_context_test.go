package activities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// ── Mock PipelineRepository for BuildStageContext tests ──────────────────────

// mockContextPipelineRepo implements PipelineRepository for BuildStageContext tests.
// It stubs CreateRun and RecordOverrides as no-ops and returns configurable providers.
type mockContextPipelineRepo struct {
	providers    []string
	providerErr  error
}

func (m *mockContextPipelineRepo) CreateRun(_ context.Context, _ PipelineRunInput) error {
	return nil
}

func (m *mockContextPipelineRepo) RecordOverrides(_ context.Context, _ int64, _ map[string]string) error {
	return nil
}

func (m *mockContextPipelineRepo) GetContextProviders(_ context.Context, _, _, _ string) ([]string, error) {
	return m.providers, m.providerErr
}

// ── Mock ContextProvider for unit tests ─────────────────────────────────────

// mockContextProvider records calls and returns configurable output.
type mockContextProvider struct {
	name       string
	buildResult string
	buildErr   error
	callCount  int
}

func (m *mockContextProvider) Name() string { return m.name }

func (m *mockContextProvider) Build(_ context.Context, _ ContextProviderInput) (string, error) {
	m.callCount++
	return m.buildResult, m.buildErr
}

// registerMockProviders registers a set of mock providers in the registry and returns
// a cleanup function that restores the previous registry state.
func registerMockProviders(providers ...*mockContextProvider) func() {
	saved := make(map[string]ContextProvider, len(providers))
	for _, p := range providers {
		saved[p.name] = providerRegistry[p.name]
		providerRegistry[p.name] = p
	}
	return func() {
		for name, prev := range saved {
			if prev == nil {
				delete(providerRegistry, name)
			} else {
				providerRegistry[name] = prev
			}
		}
	}
}

// newMinimalContextBuilderActivities creates a ContextBuilderActivities with only the
// required dependencies, for testing BuildStageContext specifically.
func newMinimalContextBuilderActivities(logger logging.Logger, pipelineRepo PipelineRepository) *ContextBuilderActivities {
	return &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: &mockEntityResolver{},
		entityRepo:     &mockEntityLookup{},
		contextRepo:    &mockContextPackageRepo{},
		pipelineRepo:   pipelineRepo,
	}
}

// ── Unit tests ───────────────────────────────────────────────────────────────

func TestBuildStageContext_NilPipelineRepo(t *testing.T) {
	logger := logging.NewNopLogger()
	a := newMinimalContextBuilderActivities(logger, nil)

	result, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
	})
	require.NoError(t, err)
	assert.Empty(t, result, "should return empty context when pipelineRepo is nil")
}

func TestBuildStageContext_DBError(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &mockContextPipelineRepo{providerErr: errors.New("db unavailable")}
	a := newMinimalContextBuilderActivities(logger, repo)

	_, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading context providers")
}

func TestBuildStageContext_NoProviders(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &mockContextPipelineRepo{providers: []string{}}
	a := newMinimalContextBuilderActivities(logger, repo)

	result, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
	})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestBuildStageContext_UnknownProvider(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &mockContextPipelineRepo{providers: []string{"nonexistent_provider"}}
	a := newMinimalContextBuilderActivities(logger, repo)

	// nonexistent_provider is not in the registry — should log warning and return empty
	result, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
	})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestBuildStageContext_ProviderAssemblyOrder(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &mockContextPipelineRepo{providers: []string{"alpha", "beta", "gamma"}}
	a := newMinimalContextBuilderActivities(logger, repo)

	alpha := &mockContextProvider{name: "alpha", buildResult: "### Alpha\nalpha content"}
	beta := &mockContextProvider{name: "beta", buildResult: "### Beta\nbeta content"}
	gamma := &mockContextProvider{name: "gamma", buildResult: "### Gamma\ngamma content"}
	cleanup := registerMockProviders(alpha, beta, gamma)
	defer cleanup()

	result, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
	})
	require.NoError(t, err)

	// All three sections must be present in declared order.
	alphaPos := strings.Index(result, "### Alpha")
	betaPos := strings.Index(result, "### Beta")
	gammaPos := strings.Index(result, "### Gamma")

	assert.Greater(t, alphaPos, -1, "alpha section missing")
	assert.Greater(t, betaPos, -1, "beta section missing")
	assert.Greater(t, gammaPos, -1, "gamma section missing")
	assert.Less(t, alphaPos, betaPos, "alpha must come before beta")
	assert.Less(t, betaPos, gammaPos, "beta must come before gamma")

	// Sections are separated by double newlines.
	assert.Contains(t, result, "alpha content\n\n### Beta")
}

func TestBuildStageContext_EmptySectionOmitted(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &mockContextPipelineRepo{providers: []string{"empty_p", "nonempty_p"}}
	a := newMinimalContextBuilderActivities(logger, repo)

	emptyP := &mockContextProvider{name: "empty_p", buildResult: ""}
	nonemptyP := &mockContextProvider{name: "nonempty_p", buildResult: "### Content\nsome text"}
	cleanup := registerMockProviders(emptyP, nonemptyP)
	defer cleanup()

	result, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
	})
	require.NoError(t, err)
	assert.Equal(t, "### Content\nsome text", result)
	assert.Equal(t, 1, emptyP.callCount, "empty provider must still be called")
}

func TestBuildStageContext_FailedProviderSkipped(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &mockContextPipelineRepo{providers: []string{"good_p", "bad_p", "good2_p"}}
	a := newMinimalContextBuilderActivities(logger, repo)

	goodP := &mockContextProvider{name: "good_p", buildResult: "### Good\ngood content"}
	badP := &mockContextProvider{name: "bad_p", buildErr: errors.New("provider explosion")}
	good2P := &mockContextProvider{name: "good2_p", buildResult: "### Good2\ngood2 content"}
	cleanup := registerMockProviders(goodP, badP, good2P)
	defer cleanup()

	result, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
	})
	// Overall must succeed despite provider failure.
	require.NoError(t, err)
	assert.Contains(t, result, "### Good\n")
	assert.Contains(t, result, "### Good2\n")
	assert.NotContains(t, result, "bad")
}

func TestBuildStageContext_MixedUnknownAndValid(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &mockContextPipelineRepo{providers: []string{"unknown_xyz", "known_p"}}
	a := newMinimalContextBuilderActivities(logger, repo)

	knownP := &mockContextProvider{name: "known_p", buildResult: "### Known\nknown content"}
	cleanup := registerMockProviders(knownP)
	defer cleanup()

	result, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
	})
	require.NoError(t, err)
	assert.Equal(t, "### Known\nknown content", result)
}

func TestBuildStageContext_AllEmpty(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &mockContextPipelineRepo{providers: []string{"empty1", "empty2"}}
	a := newMinimalContextBuilderActivities(logger, repo)

	empty1 := &mockContextProvider{name: "empty1", buildResult: ""}
	empty2 := &mockContextProvider{name: "empty2", buildResult: ""}
	cleanup := registerMockProviders(empty1, empty2)
	defer cleanup()

	result, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
	})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestBuildStageContext_ProviderReceivesInput(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &mockContextPipelineRepo{providers: []string{"input_inspector"}}
	a := newMinimalContextBuilderActivities(logger, repo)

	var capturedInput ContextProviderInput
	inspector := &mockContextProvider{name: "input_inspector"}
	inspector.buildResult = "### Inspector\ncaptured"
	// Override Build to capture input.
	inspectorFull := &inputCapturingProvider{
		name:        "input_inspector",
		buildResult: "### Inspector\ncaptured",
		capturedPtr: &capturedInput,
	}
	restore := providerRegistry["input_inspector"]
	providerRegistry["input_inspector"] = inspectorFull
	defer func() {
		if restore == nil {
			delete(providerRegistry, "input_inspector")
		} else {
			providerRegistry["input_inspector"] = restore
		}
	}()

	result, err := a.BuildStageContext(context.Background(), workflows.BuildStageContextInput{
		TenantID: "t1",
		Pipeline: "standard",
		Stage:    "deep_analyze",
		Content:  "the email body",
		Subject:  "important subject",
	})
	require.NoError(t, err)
	assert.Equal(t, "### Inspector\ncaptured", result)
	assert.Equal(t, "the email body", capturedInput.Content)
	assert.Equal(t, "important subject", capturedInput.Subject)
}

// inputCapturingProvider is a test helper that captures the ContextProviderInput.
type inputCapturingProvider struct {
	name        string
	buildResult string
	capturedPtr *ContextProviderInput
}

func (p *inputCapturingProvider) Name() string { return p.name }
func (p *inputCapturingProvider) Build(_ context.Context, input ContextProviderInput) (string, error) {
	*p.capturedPtr = input
	return p.buildResult, nil
}
