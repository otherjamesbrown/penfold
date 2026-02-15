// Package activities provides tests for activity registration verification.
package activities

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/worker/config"
)

// Stubs for registration verification tests only.
// These implement the minimum interface methods required to construct activity structs.
// They are never called — only used so RegisterAll can register activities by name.

type regVerifyAIClient struct{}

func (r *regVerifyAIClient) GenerateEmbedding(context.Context, *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	return nil, nil
}
func (r *regVerifyAIClient) GenerateSummary(context.Context, *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	return nil, nil
}
func (r *regVerifyAIClient) ExtractAssertions(context.Context, *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
	return nil, nil
}
func (r *regVerifyAIClient) ExtractEntities(context.Context, *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
	return nil, nil
}
func (r *regVerifyAIClient) TriageContent(context.Context, *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
	return nil, nil
}
func (r *regVerifyAIClient) DeepAnalyze(context.Context, *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error) {
	return nil, nil
}

type regVerifyEmbeddingRepo struct{}

func (r *regVerifyEmbeddingRepo) StoreEmbedding(context.Context, string, int64, []float32, string, int32) (int64, error) {
	return 0, nil
}
func (r *regVerifyEmbeddingRepo) GetEmbedding(context.Context, string, int64) (*Embedding, error) {
	return nil, nil
}
func (r *regVerifyEmbeddingRepo) StoreMultiLevelEmbedding(context.Context, *MultiLevelEmbeddingInput) (int64, error) {
	return 0, nil
}
func (r *regVerifyEmbeddingRepo) GetEmbeddingsForSource(context.Context, string, int64, string) ([]*Embedding, error) {
	return nil, nil
}
func (r *regVerifyEmbeddingRepo) GetStaleEmbeddings(context.Context, string, string, string, int) ([]int64, error) {
	return nil, nil
}
func (r *regVerifyEmbeddingRepo) DeleteEmbeddingsForSource(context.Context, string, int64) error {
	return nil
}
func (r *regVerifyEmbeddingRepo) DeleteEmbedding(context.Context, int64) error { return nil }

type regVerifySummaryRepo struct{}

func (r *regVerifySummaryRepo) StoreSummary(context.Context, string, int64, string, []string, string) (int64, error) {
	return 0, nil
}
func (r *regVerifySummaryRepo) DeleteSummary(context.Context, int64) error { return nil }

type regVerifyAssertionRepo struct{}

func (r *regVerifyAssertionRepo) StoreAssertions(context.Context, string, int64, []*Assertion, string) (int, error) {
	return 0, nil
}

type regVerifyEntityRepo struct{}

func (r *regVerifyEntityRepo) StoreEntities(context.Context, string, int64, []*Entity) (int, error) {
	return 0, nil
}

type regVerifyPersistRepo struct{}

func (r *regVerifyPersistRepo) PersistFindings(context.Context, *PersistFindingsInput) (*PersistFindingsOutput, error) {
	return nil, nil
}

type regVerifyPipelineRepo struct{}

func (r *regVerifyPipelineRepo) CreateRun(context.Context, PipelineRunInput) error {
	return nil
}

func (r *regVerifyPipelineRepo) RecordOverrides(context.Context, int64, map[string]string) error {
	return nil
}

type regVerifyContextPackageRepo struct{}

func (r *regVerifyContextPackageRepo) GetActiveRisks(context.Context, []int64, int) ([]ContextAssertion, error) {
	return nil, nil
}
func (r *regVerifyContextPackageRepo) GetOpenActions(context.Context, []int64, int) ([]ContextAssertion, error) {
	return nil, nil
}
func (r *regVerifyContextPackageRepo) GetRecentDecisions(context.Context, []int64, int, int) ([]ContextAssertion, error) {
	return nil, nil
}
func (r *regVerifyContextPackageRepo) GetProductEvents(context.Context, []int64, int, int) ([]ContextProductEvent, error) {
	return nil, nil
}
func (r *regVerifyContextPackageRepo) GetGlossaryTerms(context.Context, []string, []int64, int) ([]ContextGlossaryTerm, error) {
	return nil, nil
}
func (r *regVerifyContextPackageRepo) ResolveProjectByName(context.Context, string, string) (*int64, error) {
	return nil, nil
}
func (r *regVerifyContextPackageRepo) ResolveProjectByKeyword(context.Context, string, string) (*int64, error) {
	return nil, nil
}

type regVerifyEntityResolver struct{}

func (r *regVerifyEntityResolver) Resolve(context.Context, string, string) (*entities.ResolutionResult, error) {
	return nil, nil
}
func (r *regVerifyEntityResolver) ResolveOrCreate(context.Context, string, string, string) (*entities.ResolutionResult, error) {
	return nil, nil
}

type regVerifyEntityLookup struct{}

func (r *regVerifyEntityLookup) SearchPeopleByName(context.Context, string, string, int) ([]*entities.Person, error) {
	return nil, nil
}
func (r *regVerifyEntityLookup) GetProjectByName(context.Context, string, string) (*entities.Project, error) {
	return nil, nil
}
func (r *regVerifyEntityLookup) GetProjectsWithKeywords(context.Context, string) ([]*entities.Project, error) {
	return nil, nil
}
func (r *regVerifyEntityLookup) IncrementSentCount(context.Context, int64) error {
	return nil
}
func (r *regVerifyEntityLookup) IncrementReceivedCount(context.Context, int64) error {
	return nil
}
func (r *regVerifyEntityLookup) UpdatePersonTitle(context.Context, int64, string) error {
	return nil
}

type regVerifyPersonRepo struct{}

func (r *regVerifyPersonRepo) GetPersonByID(context.Context, int64) (*entities.Person, error) {
	return nil, nil
}
func (r *regVerifyPersonRepo) UpdatePerson(context.Context, *entities.Person) error {
	return nil
}
func (r *regVerifyPersonRepo) GetPeopleByDomain(context.Context, string, string) ([]*entities.Person, error) {
	return nil, nil
}

type regVerifyProjectTaggingRepo struct{}

func (r *regVerifyProjectTaggingRepo) GetProjectsWithKeywords(context.Context, string) ([]*entities.Project, error) {
	return nil, nil
}
func (r *regVerifyProjectTaggingRepo) GetContentText(context.Context, string, int64) (string, error) {
	return "", nil
}
func (r *regVerifyProjectTaggingRepo) CreateContentMention(context.Context, string, int64, string, string, int64) error {
	return nil
}

// Compile-time interface verification for stubs.
var (
	_ AIClient                   = (*regVerifyAIClient)(nil)
	_ EmbeddingRepository        = (*regVerifyEmbeddingRepo)(nil)
	_ SummaryRepository          = (*regVerifySummaryRepo)(nil)
	_ AssertionRepository        = (*regVerifyAssertionRepo)(nil)
	_ EntityRepository           = (*regVerifyEntityRepo)(nil)
	_ PersistRepository          = (*regVerifyPersistRepo)(nil)
	_ PipelineRepository         = (*regVerifyPipelineRepo)(nil)
	_ ContextPackageRepository   = (*regVerifyContextPackageRepo)(nil)
	_ EntityResolverInterface    = (*regVerifyEntityResolver)(nil)
	_ EntityLookupInterface      = (*regVerifyEntityLookup)(nil)
	_ PersonRepository           = (*regVerifyPersonRepo)(nil)
	_ ProjectTaggingRepository   = (*regVerifyProjectTaggingRepo)(nil)
)

// newFullRegistrar creates a fully-configured Registrar with all activity types.
// Activity structs use stub dependencies — only registration names matter.
func newFullRegistrar() *Registrar {
	logger := logging.NewNopLogger()
	ai := &regVerifyAIClient{}

	r := NewRegistrar(NewActivities(logger))

	// MentionsActivities uses concrete types (*resolver.Resolver, *mentions.PostgresRepository)
	// that can't be mocked with interfaces. Construct the struct directly with nil fields —
	// this is safe because RegisterAll only reads the struct pointer, not the fields.
	return r.
		WithEmbeddingActivities(&EmbeddingActivities{
			logger:        logger,
			aiClient:      ai,
			embeddingRepo: &regVerifyEmbeddingRepo{},
		}).
		WithSummarizationActivities(&SummarizationActivities{
			logger:      logger,
			aiClient:    ai,
			summaryRepo: &regVerifySummaryRepo{},
		}).
		WithExtractionActivities(&ExtractionActivities{
			logger:        logger,
			aiClient:      ai,
			assertionRepo: &regVerifyAssertionRepo{},
			entityRepo:    &regVerifyEntityRepo{},
		}).
		WithMentionsActivities(&MentionsActivities{
			logger: logger,
		}).
		WithParseActivities(NewParseActivities(logger)).
		WithPersistActivities(&PersistActivities{
			logger:     logger,
			repository: &regVerifyPersistRepo{},
		}).
		WithTriageActivities(&TriageActivities{
			logger:   logger,
			aiClient: ai,
		}).
		WithContextBuilderActivities(&ContextBuilderActivities{
			logger:         logger,
			entityResolver: &regVerifyEntityResolver{},
			entityRepo:     &regVerifyEntityLookup{},
			contextRepo:    &regVerifyContextPackageRepo{},
		}).
		WithAnalysisActivities(&AnalysisActivities{
			logger:   logger,
			aiClient: ai,
		}).
		WithPipelineActivities(&PipelineActivities{
			logger:       logger,
			pipelineRepo: &regVerifyPipelineRepo{},
			baseRepo:     nil, // nil is safe for registration testing
		}).
		WithPersonEnrichmentActivities(&PersonEnrichmentActivities{
			logger:     logger,
			personRepo: &regVerifyPersonRepo{},
		}).
		WithProjectTaggingActivities(&ProjectTaggingActivities{
			logger: logger,
			repo:   &regVerifyProjectTaggingRepo{},
		}).
		WithThreadActivities(&ThreadActivities{
			logger: logger,
		})
}

// TestAllMainQueueActivitiesRegistered verifies that all expected main queue activities are registered.
func TestAllMainQueueActivitiesRegistered(t *testing.T) {
	w := newMockWorker()
	r := newFullRegistrar()

	r.RegisterAll(w, config.MainTaskQueue)

	expected := pkgtemporal.AllMainQueueActivities()
	registered := toSet(w.registeredActivities)

	for _, name := range expected {
		require.True(t, registered[name],
			"expected activity %q to be registered on MainTaskQueue", name)
	}
}

// TestAllAIQueueActivitiesRegistered verifies that all expected AI queue activities are registered.
func TestAllAIQueueActivitiesRegistered(t *testing.T) {
	w := newMockWorker()
	r := newFullRegistrar()

	r.RegisterAll(w, config.AITaskQueue)

	expected := pkgtemporal.AllAIQueueActivities()
	registered := toSet(w.registeredActivities)

	for _, name := range expected {
		require.True(t, registered[name],
			"expected activity %q to be registered on AITaskQueue", name)
	}
}

// TestAllEmailQueueActivitiesRegistered verifies that all expected email queue activities are registered.
// Email queue = AllEmailQueueActivities() UNION AllAIQueueActivities()
// because registerEmailQueueActivities calls registerAIQueueActivities.
func TestAllEmailQueueActivitiesRegistered(t *testing.T) {
	w := newMockWorker()
	r := newFullRegistrar()

	r.RegisterAll(w, config.EmailTaskQueue)

	expected := make(map[string]bool)
	for _, name := range pkgtemporal.AllEmailQueueActivities() {
		expected[name] = true
	}
	for _, name := range pkgtemporal.AllAIQueueActivities() {
		expected[name] = true
	}

	registered := toSet(w.registeredActivities)

	for name := range expected {
		require.True(t, registered[name],
			"expected activity %q to be registered on EmailTaskQueue", name)
	}
}

// TestNoOrphanActivitiesRegistered verifies that no unexpected activities are registered on any queue.
func TestNoOrphanActivitiesRegistered(t *testing.T) {
	r := newFullRegistrar()

	t.Run("MainTaskQueue", func(t *testing.T) {
		w := newMockWorker()
		r.RegisterAll(w, config.MainTaskQueue)

		expected := toSet(pkgtemporal.AllMainQueueActivities())
		for _, name := range w.registeredActivities {
			require.True(t, expected[name],
				"unexpected activity %q registered on MainTaskQueue", name)
		}
	})

	t.Run("AITaskQueue", func(t *testing.T) {
		w := newMockWorker()
		r.RegisterAll(w, config.AITaskQueue)

		expected := toSet(pkgtemporal.AllAIQueueActivities())
		for _, name := range w.registeredActivities {
			require.True(t, expected[name],
				"unexpected activity %q registered on AITaskQueue", name)
		}
	})

	t.Run("EmailTaskQueue", func(t *testing.T) {
		w := newMockWorker()
		r.RegisterAll(w, config.EmailTaskQueue)

		expected := make(map[string]bool)
		for _, name := range pkgtemporal.AllEmailQueueActivities() {
			expected[name] = true
		}
		for _, name := range pkgtemporal.AllAIQueueActivities() {
			expected[name] = true
		}

		for _, name := range w.registeredActivities {
			require.True(t, expected[name],
				"unexpected activity %q registered on EmailTaskQueue", name)
		}
	})
}

// TestActivityCountsMatchConstants verifies ActivityCount() matches the length of constant lists.
func TestActivityCountsMatchConstants(t *testing.T) {
	r := newFullRegistrar()

	t.Run("MainTaskQueue", func(t *testing.T) {
		expected := len(pkgtemporal.AllMainQueueActivities())
		actual := r.ActivityCount(config.MainTaskQueue)
		require.Equal(t, expected, actual,
			"ActivityCount() should match len(AllMainQueueActivities())")
	})

	t.Run("AITaskQueue", func(t *testing.T) {
		expected := len(pkgtemporal.AllAIQueueActivities())
		actual := r.ActivityCount(config.AITaskQueue)
		require.Equal(t, expected, actual,
			"ActivityCount() should match len(AllAIQueueActivities())")
	})

	t.Run("EmailTaskQueue", func(t *testing.T) {
		// Email queue = AllEmailQueueActivities() + AllAIQueueActivities()
		expected := len(pkgtemporal.AllEmailQueueActivities()) + len(pkgtemporal.AllAIQueueActivities())
		actual := r.ActivityCount(config.EmailTaskQueue)
		require.Equal(t, expected, actual,
			"ActivityCount() should equal len(AllEmailQueueActivities()) + len(AllAIQueueActivities())")
	})
}

// toSet converts a string slice to a map for O(1) lookups.
func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// Suppress unused import warnings for time package (used in interface stubs).
var _ = time.Now
