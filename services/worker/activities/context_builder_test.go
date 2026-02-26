package activities

import (
	"context"
	"strings"
	"testing"
	"time"

	enrichmentconfig "github.com/otherjamesbrown/penfold/pkg/enrichment/config"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// mockEntityResolver implements EntityResolverInterface for testing.
type mockEntityResolver struct {
	resolveFunc          func(ctx context.Context, tenantID, email string) (*entities.ResolutionResult, error)
	resolveOrCreateFunc  func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error)
}

func (m *mockEntityResolver) Resolve(ctx context.Context, tenantID, email string) (*entities.ResolutionResult, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(ctx, tenantID, email)
	}
	return nil, nil
}

func (m *mockEntityResolver) ResolveOrCreate(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
	if m.resolveOrCreateFunc != nil {
		return m.resolveOrCreateFunc(ctx, tenantID, email, displayName)
	}
	return nil, nil
}

// mockEntityLookup implements EntityLookupInterface for testing.
type mockEntityLookup struct {
	searchPeopleByNameFunc  func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error)
	getProjectByNameFunc    func(ctx context.Context, tenantID, name string) (*entities.Project, error)
	getProjectsWithKeywordsFunc func(ctx context.Context, tenantID string) ([]*entities.Project, error)
	incrementSentCountFunc     func(ctx context.Context, personID int64) error
	incrementReceivedCountFunc func(ctx context.Context, personID int64) error
	updatePersonTitleFunc      func(ctx context.Context, personID int64, title string) error
}

func (m *mockEntityLookup) SearchPeopleByName(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
	if m.searchPeopleByNameFunc != nil {
		return m.searchPeopleByNameFunc(ctx, tenantID, name, limit)
	}
	return nil, nil
}

func (m *mockEntityLookup) GetProjectByName(ctx context.Context, tenantID, name string) (*entities.Project, error) {
	if m.getProjectByNameFunc != nil {
		return m.getProjectByNameFunc(ctx, tenantID, name)
	}
	return nil, nil
}

func (m *mockEntityLookup) GetProjectsWithKeywords(ctx context.Context, tenantID string) ([]*entities.Project, error) {
	if m.getProjectsWithKeywordsFunc != nil {
		return m.getProjectsWithKeywordsFunc(ctx, tenantID)
	}
	return nil, nil
}

func (m *mockEntityLookup) IncrementSentCount(ctx context.Context, personID int64) error {
	if m.incrementSentCountFunc != nil {
		return m.incrementSentCountFunc(ctx, personID)
	}
	return nil
}

func (m *mockEntityLookup) IncrementReceivedCount(ctx context.Context, personID int64) error {
	if m.incrementReceivedCountFunc != nil {
		return m.incrementReceivedCountFunc(ctx, personID)
	}
	return nil
}

func (m *mockEntityLookup) UpdatePersonTitle(ctx context.Context, personID int64, title string) error {
	if m.updatePersonTitleFunc != nil {
		return m.updatePersonTitleFunc(ctx, personID, title)
	}
	return nil
}

// Note: mockContextPackageRepo is defined in context_repo_test.go

// mockConfigRepository implements enrichmentconfig.ConfigRepository for testing.
type mockConfigRepository struct {
	tenant        *enrichmentconfig.Tenant
	domains       []enrichmentconfig.TenantDomain
	emailPatterns []enrichmentconfig.TenantEmailPattern
}

func (m *mockConfigRepository) GetTenant(_ context.Context, _ string) (*enrichmentconfig.Tenant, error) {
	return m.tenant, nil
}

func (m *mockConfigRepository) GetTenantBySlug(_ context.Context, _ string) (*enrichmentconfig.Tenant, error) {
	return m.tenant, nil
}

func (m *mockConfigRepository) GetTenantDomains(_ context.Context, _ string) ([]enrichmentconfig.TenantDomain, error) {
	return m.domains, nil
}

func (m *mockConfigRepository) GetTenantEmailPatterns(_ context.Context, _ string) ([]enrichmentconfig.TenantEmailPattern, error) {
	return m.emailPatterns, nil
}

func (m *mockConfigRepository) GetTenantProcessingRules(_ context.Context, _ string) ([]enrichmentconfig.TenantProcessingRule, error) {
	return nil, nil
}

func (m *mockConfigRepository) GetTenantIntegrations(_ context.Context, _ string) ([]enrichmentconfig.TenantIntegration, error) {
	return nil, nil
}

func TestBuildContext_PersonResolution(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	tests := []struct {
		name           string
		extraction     *ExtractEntitiesOutput
		setupMock      func(*mockEntityLookup)
		expectedPeople int
		expectedSource string
	}{
		{
			name: "exact name match",
			extraction: &ExtractEntitiesOutput{
				People: []PersonResult{
					{Name: "John Doe", Role: "Engineer"},
				},
			},
			setupMock: func(m *mockEntityLookup) {
				m.searchPeopleByNameFunc = func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
					return []*entities.Person{
						{
							ID:            123,
							CanonicalName: "John Doe",
							Title:         "Senior Engineer",
							IsInternal:    true,
						},
					}, nil
				}
			},
			expectedPeople: 1,
			expectedSource: "fuzzy",
		},
		{
			name: "fuzzy name match",
			extraction: &ExtractEntitiesOutput{
				People: []PersonResult{
					{Name: "Jon Doe", Role: "Engineer"},
				},
			},
			setupMock: func(m *mockEntityLookup) {
				m.searchPeopleByNameFunc = func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
					return []*entities.Person{
						{
							ID:            123,
							CanonicalName: "John Doe",
							Title:         "Senior Engineer",
							IsInternal:    true,
						},
					}, nil
				}
			},
			expectedPeople: 1,
			expectedSource: "fuzzy",
		},
		{
			name: "unresolved person",
			extraction: &ExtractEntitiesOutput{
				People: []PersonResult{
					{Name: "Unknown Person", Role: "Guest"},
				},
			},
			setupMock: func(m *mockEntityLookup) {
				m.searchPeopleByNameFunc = func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
					return nil, nil
				}
			},
			expectedPeople: 0,
			expectedSource: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entityLookup := &mockEntityLookup{}
			tt.setupMock(entityLookup)

			activities := NewContextBuilderActivities(
				logger,
				&mockEntityResolver{},
				entityLookup,
				&mockContextPackageRepo{},
				nil,
				nil,
				nil,
			)

			input := BuildContextInput{
				TenantID:    "test-tenant",
				SourceID:    1,
				ContentType: "email",
				Extraction:  tt.extraction,
			}

			output, err := activities.BuildContextPackage(ctx, input)
			if err != nil {
				t.Fatalf("BuildContextPackage failed: %v", err)
			}

			if len(output.ResolvedPeople) != tt.expectedPeople {
				t.Errorf("Expected %d resolved people, got %d", tt.expectedPeople, len(output.ResolvedPeople))
			}

			if tt.expectedPeople > 0 && output.ResolvedPeople[0].Source != tt.expectedSource {
				t.Errorf("Expected source %s, got %s", tt.expectedSource, output.ResolvedPeople[0].Source)
			}
		})
	}
}

func TestBuildContext_ProjectResolution(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	tests := []struct {
		name             string
		extraction       *ExtractEntitiesOutput
		setupEntityMock  func(*mockEntityLookup)
		setupContextRepo func() *mockContextPackageRepo
		expectedProjects int
		expectedSource   string
	}{
		{
			name: "exact name match",
			extraction: &ExtractEntitiesOutput{
				Projects: []string{"CLIC"},
			},
			setupEntityMock: func(m *mockEntityLookup) {
				m.getProjectByNameFunc = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
					if name == "CLIC" {
						return &entities.Project{
							ID:   456,
							Name: "CLIC",
						}, nil
					}
					return nil, nil
				}
			},
			setupContextRepo: func() *mockContextPackageRepo {
				return &mockContextPackageRepo{}
			},
			expectedProjects: 1,
			expectedSource:   "exact_match",
		},
		{
			name: "keyword match",
			extraction: &ExtractEntitiesOutput{
				Projects: []string{"PLT"},
			},
			setupEntityMock: func(m *mockEntityLookup) {
				m.getProjectByNameFunc = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
					return nil, nil
				}
			},
			setupContextRepo: func() *mockContextPackageRepo {
				id := int64(789)
				return &mockContextPackageRepo{
					projectsByKeyword: map[string]*int64{
						"PLT": &id,
					},
				}
			},
			expectedProjects: 1,
			expectedSource:   "keyword",
		},
		{
			name: "name containment match",
			extraction: &ExtractEntitiesOutput{
				Projects: []string{"Oslo NLB Workflow"},
			},
			setupEntityMock: func(m *mockEntityLookup) {
				m.getProjectByNameFunc = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
					return nil, nil // No exact match
				}
			},
			setupContextRepo: func() *mockContextPackageRepo {
				id := int64(101)
				return &mockContextPackageRepo{
					projectsByNameContains: map[string]*int64{
						"Oslo NLB Workflow": &id,
					},
				}
			},
			expectedProjects: 1,
			expectedSource:   "name_contains",
		},
		{
			name: "unresolved project",
			extraction: &ExtractEntitiesOutput{
				Projects: []string{"UnknownProject"},
			},
			setupEntityMock: func(m *mockEntityLookup) {
				m.getProjectByNameFunc = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
					return nil, nil
				}
			},
			setupContextRepo: func() *mockContextPackageRepo {
				return &mockContextPackageRepo{}
			},
			expectedProjects: 0,
			expectedSource:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entityLookup := &mockEntityLookup{}
			tt.setupEntityMock(entityLookup)
			contextRepo := tt.setupContextRepo()

			activities := NewContextBuilderActivities(
				logger,
				&mockEntityResolver{},
				entityLookup,
				contextRepo,
				nil,
				nil,
				nil,
			)

			input := BuildContextInput{
				TenantID:    "test-tenant",
				SourceID:    1,
				ContentType: "email",
				Extraction:  tt.extraction,
			}

			output, err := activities.BuildContextPackage(ctx, input)
			if err != nil {
				t.Fatalf("BuildContextPackage failed: %v", err)
			}

			if len(output.ResolvedProjects) != tt.expectedProjects {
				t.Errorf("Expected %d resolved projects, got %d", tt.expectedProjects, len(output.ResolvedProjects))
			}

			if tt.expectedProjects > 0 && output.ResolvedProjects[0].Source != tt.expectedSource {
				t.Errorf("Expected source %s, got %s", tt.expectedSource, output.ResolvedProjects[0].Source)
			}
		})
	}
}

func TestBuildContext_TokenBudget_Meeting(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil, nil, nil)

	input := BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "meeting",
		Extraction:  &ExtractEntitiesOutput{},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	if output.TokenBudget != 3000 {
		t.Errorf("Expected meeting token budget of 3000, got %d", output.TokenBudget)
	}
}

func TestBuildContext_TokenBudget_Email(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil, nil, nil)

	input := BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		Extraction:  &ExtractEntitiesOutput{},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	if output.TokenBudget != 2000 {
		t.Errorf("Expected email token budget of 2000, got %d", output.TokenBudget)
	}
}

func TestBuildContext_TokenBudget_Slack(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil, nil, nil)

	input := BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "slack",
		Extraction:  &ExtractEntitiesOutput{},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	if output.TokenBudget != 1000 {
		t.Errorf("Expected slack token budget of 1000, got %d", output.TokenBudget)
	}
}

func TestBuildContext_TokenBudget_Truncation(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Create a project ID
	projectID := int64(1)

	// Create data structures for mock
	risks := make([]ContextAssertion, 10)
	for i := range risks {
		risks[i] = ContextAssertion{Description: "Risk", Severity: "high"}
	}

	actions := make([]ContextAssertion, 10)
	for i := range actions {
		actions[i] = ContextAssertion{Description: "Action", Status: "open"}
	}

	decisions := make([]ContextAssertion, 5)
	for i := range decisions {
		decisions[i] = ContextAssertion{Description: "Decision"}
	}

	events := make([]ContextProductEvent, 10)
	for i := range events {
		events[i] = ContextProductEvent{Title: "Event", EventType: "milestone", OccurredAt: time.Now()}
	}

	glossary := make([]ContextGlossaryTerm, 20)
	for i := range glossary {
		glossary[i] = ContextGlossaryTerm{Term: "TERM", Expansion: "Expanded Term"}
	}

	contextRepo := &mockContextPackageRepo{
		activeRisks:     risks,
		openActions:     actions,
		recentDecisions: decisions,
		productEvents:   events,
		glossaryTerms:   glossary,
	}

	entityLookup := &mockEntityLookup{
		getProjectByNameFunc: func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
			return &entities.Project{ID: projectID, Name: "TestProject"}, nil
		},
	}

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, entityLookup, contextRepo, nil, nil, nil)

	input := BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "slack", // 1000 token budget
		Extraction: &ExtractEntitiesOutput{
			Projects: []string{"TestProject"},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	if output.TokensUsed > output.TokenBudget {
		t.Errorf("Token usage (%d) exceeded budget (%d)", output.TokensUsed, output.TokenBudget)
	}

	// Verify truncation happened (glossary should be truncated first)
	totalItems := len(output.ContextPackage.ActiveRisks) +
		len(output.ContextPackage.OpenActions) +
		len(output.ContextPackage.RecentDecisions) +
		len(output.ContextPackage.ProductEvents) +
		len(output.ContextPackage.GlossaryTerms)

	// We should have fewer items than the total available (10+10+5+10+20 = 55)
	if totalItems >= 55 {
		t.Errorf("Expected truncation, but got all %d items", totalItems)
	}
}

func TestBuildContext_EmptyExtraction(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil, nil, nil)

	input := BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		Extraction:  nil, // Nil extraction
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	if len(output.ResolvedPeople) != 0 {
		t.Errorf("Expected 0 resolved people, got %d", len(output.ResolvedPeople))
	}

	if len(output.ResolvedProjects) != 0 {
		t.Errorf("Expected 0 resolved projects, got %d", len(output.ResolvedProjects))
	}

	if output.ContextPackage == nil {
		t.Error("Expected non-nil context package")
	}
}

func TestBuildContext_UnknownEntities(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	entityLookup := &mockEntityLookup{
		searchPeopleByNameFunc: func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
			return nil, nil // No matches
		},
		getProjectByNameFunc: func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
			return nil, nil // No matches
		},
	}

	contextRepo := &mockContextPackageRepo{
		projectsByKeyword: map[string]*int64{}, // Empty - no keyword matches
		glossaryTerms:     []ContextGlossaryTerm{}, // Empty - no glossary matches
	}

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, entityLookup, contextRepo, nil, nil, nil)

	input := BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		Extraction: &ExtractEntitiesOutput{
			People: []PersonResult{
				{Name: "Unknown Person"},
			},
			Projects: []string{"UnknownProject"},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	if output.EntitiesUnresolved != 2 {
		t.Errorf("Expected 2 unresolved entities (1 person, 1 project), got %d", output.EntitiesUnresolved)
	}

	if len(output.UnresolvedTerms) != 1 || output.UnresolvedTerms[0] != "UnknownProject" {
		t.Errorf("Expected unresolved term 'UnknownProject', got %v", output.UnresolvedTerms)
	}
}

func TestBuildContext_FilterNonPersonEmails(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Helper to convert email strings to Participant structs
	toParticipants := func(emails []string) []workflows.Participant {
		participants := make([]workflows.Participant, len(emails))
		for i, email := range emails {
			participants[i] = workflows.Participant{Email: email, DisplayName: ""}
		}
		return participants
	}

	// ConfigResolver with tenant-specific patterns for emails externalized from base lists.
	// mailer.aha.io is an external service domain and prb-facilitator is a role account,
	// both now loaded from tenant config rather than hardcoded.
	tenantConfigResolver := enrichmentconfig.NewConfigResolver(&mockConfigRepository{
		tenant: &enrichmentconfig.Tenant{ID: "test-tenant", Name: "Test", Slug: "test", IsActive: true},
		domains: []enrichmentconfig.TenantDomain{
			{TenantID: "test-tenant", Domain: "mailer.aha.io", DomainType: "external_known"},
		},
		emailPatterns: []enrichmentconfig.TenantEmailPattern{
			{TenantID: "test-tenant", Pattern: "prb-facilitator", PatternType: "role_account", Enabled: true},
		},
	})

	tests := []struct {
		name              string
		participantEmails []workflows.Participant
		expectedPeople    int
		expectedFiltered  []string
		configResolver    *enrichmentconfig.ConfigResolver
	}{
		{
			name:              "filter distribution lists",
			participantEmails: toParticipants([]string{"alice@example.com", "dl-ttmtc-SteerCo@akamai.com", "bob@example.com"}),
			expectedPeople:    2, // only alice and bob
			expectedFiltered:  []string{"dl-ttmtc-SteerCo@akamai.com"},
		},
		{
			name:              "filter automated senders",
			participantEmails: toParticipants([]string{"alice@example.com", "updates@mailer.aha.io", "bob@example.com"}),
			expectedPeople:    2, // only alice and bob
			expectedFiltered:  []string{"updates@mailer.aha.io"},
			configResolver:    tenantConfigResolver,
		},
		{
			name:              "filter service accounts",
			participantEmails: toParticipants([]string{"alice@example.com", "gsd-jira@akamai.com", "bob@example.com"}),
			expectedPeople:    2, // only alice and bob
			expectedFiltered:  []string{"gsd-jira@akamai.com"},
		},
		{
			name:              "filter role accounts",
			participantEmails: toParticipants([]string{"alice@example.com", "prb-facilitator@akamai.com", "bob@example.com"}),
			expectedPeople:    2, // only alice and bob
			expectedFiltered:  []string{"prb-facilitator@akamai.com"},
			configResolver:    tenantConfigResolver,
		},
		{
			name:              "filter noreply addresses",
			participantEmails: toParticipants([]string{"alice@example.com", "noreply@company.com", "bob@example.com"}),
			expectedPeople:    2, // only alice and bob
			expectedFiltered:  []string{"noreply@company.com"},
		},
		{
			name:              "filter docs.google.com senders",
			participantEmails: toParticipants([]string{"alice@example.com", "comments-noreply@docs.google.com", "bob@example.com"}),
			expectedPeople:    2, // only alice and bob
			expectedFiltered:  []string{"comments-noreply@docs.google.com"},
		},
		{
			name:              "all person emails",
			participantEmails: toParticipants([]string{"alice@example.com", "bob@example.com"}),
			expectedPeople:    2,
			expectedFiltered:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := make(map[string]int)

			entityResolver := &mockEntityResolver{
				resolveOrCreateFunc: func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
					callCount[email]++
					return &entities.ResolutionResult{
						Person: &entities.Person{
							ID:            int64(100 + len(callCount)),
							CanonicalName: displayName,
							PrimaryEmail:  email,
							IsInternal:    false,
							Confidence:    0.7,
						},
						Confidence: 0.7,
						Source:     "auto_created",
						IsNew:      true,
					}, nil
				},
			}

			activities := NewContextBuilderActivities(
				logger,
				entityResolver,
				&mockEntityLookup{},
				&mockContextPackageRepo{},
				nil,
				nil,
				tt.configResolver,
			)

			input := BuildContextInput{
				TenantID:          "test-tenant",
				SourceID:          1,
				ContentType:       "email",
				ParticipantEmails: tt.participantEmails,
				Extraction:        &ExtractEntitiesOutput{},
			}

			output, err := activities.BuildContextPackage(ctx, input)
			if err != nil {
				t.Fatalf("BuildContextPackage failed: %v", err)
			}

			if len(output.ResolvedPeople) != tt.expectedPeople {
				t.Errorf("Expected %d resolved people, got %d", tt.expectedPeople, len(output.ResolvedPeople))
			}

			// Verify filtered emails were NOT passed to ResolveOrCreate
			for _, filtered := range tt.expectedFiltered {
				if count, exists := callCount[filtered]; exists && count > 0 {
					t.Errorf("Expected email %s to be filtered, but it was processed %d times", filtered, count)
				}
			}
		})
	}
}

func TestBuildContext_DisplayNamesPassedToResolver(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// This test reproduces bug pf-22a545: display names exist in metadata but are not passed to ResolveOrCreate.
	// The FetchSource activity reads participant_emails (text array, emails only).
	// The context builder calls ResolveOrCreate(ctx, tenantID, email, "") with empty displayName.
	// This test MUST fail until the bug is fixed.

	receivedCalls := make(map[string]string) // email -> displayName

	entityResolver := &mockEntityResolver{
		resolveOrCreateFunc: func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
			receivedCalls[email] = displayName
			return &entities.ResolutionResult{
				Person: &entities.Person{
					ID:            int64(100 + len(receivedCalls)),
					CanonicalName: displayName,
					PrimaryEmail:  email,
					IsInternal:    false,
					Confidence:    0.7,
				},
				Confidence: 0.7,
				Source:     "auto_created",
				IsNew:      true,
			}, nil
		},
	}

	activities := NewContextBuilderActivities(
		logger,
		entityResolver,
		&mockEntityLookup{},
		&mockContextPackageRepo{},
		nil,
		nil,
		nil,
	)

	input := BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		SenderEmail: "alice@example.com",
		SenderName:  "Alice Smith",
		ParticipantEmails: []workflows.Participant{
			{Email: "bob@example.com", DisplayName: "Bob Jones"},
			{Email: "carol@example.com", DisplayName: "Carol White"},
		},
		Extraction: &ExtractEntitiesOutput{},
	}

	_, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// Verify sender display name was passed
	if senderName, ok := receivedCalls["alice@example.com"]; !ok {
		t.Error("Sender email was not resolved")
	} else if senderName != "Alice Smith" {
		t.Errorf("Expected sender displayName 'Alice Smith', got '%s' (empty means bug exists)", senderName)
	}

	// Verify participant display names were passed correctly
	if bobName, ok := receivedCalls["bob@example.com"]; !ok {
		t.Error("Participant bob@example.com was not resolved")
	} else if bobName != "Bob Jones" {
		t.Errorf("Expected bob@example.com displayName 'Bob Jones', got '%s'", bobName)
	}

	if carolName, ok := receivedCalls["carol@example.com"]; !ok {
		t.Error("Participant carol@example.com was not resolved")
	} else if carolName != "Carol White" {
		t.Errorf("Expected carol@example.com displayName 'Carol White', got '%s'", carolName)
	}

	// Verify all participants have display names
	for email, displayName := range receivedCalls {
		if displayName == "" {
			t.Errorf("BUG: Participant %s was resolved with empty displayName", email)
		}
	}
}

func TestBuildContext_ParticipantEmailsResolution(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Helper to convert email strings to Participant structs
	toParticipants := func(emails []string) []workflows.Participant {
		participants := make([]workflows.Participant, len(emails))
		for i, email := range emails {
			participants[i] = workflows.Participant{Email: email, DisplayName: ""}
		}
		return participants
	}

	tests := []struct {
		name                 string
		senderEmail          string
		senderName           string
		participantEmails    []workflows.Participant
		extraction           *workflows.SLMPipelineExtractEntitiesOutput
		expectedPeople       int
		expectedSources      []string
		verifyDeduplication  bool
	}{
		{
			name:              "participant emails only",
			participantEmails: toParticipants([]string{"alice@example.com", "bob@example.com"}),
			extraction:        &workflows.SLMPipelineExtractEntitiesOutput{},
			expectedPeople:    2,
			expectedSources:   []string{"auto_created", "auto_created"},
		},
		{
			name:              "sender + participants",
			senderEmail:       "alice@example.com",
			senderName:        "Alice Smith",
			participantEmails: toParticipants([]string{"bob@example.com", "carol@example.com"}),
			extraction:        &workflows.SLMPipelineExtractEntitiesOutput{},
			expectedPeople:    3,
			expectedSources:   []string{"auto_created", "auto_created", "auto_created"},
		},
		{
			name:               "deduplication: sender in participants",
			senderEmail:        "alice@example.com",
			senderName:         "Alice Smith",
			participantEmails:  toParticipants([]string{"alice@example.com", "bob@example.com"}),
			extraction:         &workflows.SLMPipelineExtractEntitiesOutput{},
			expectedPeople:     2,
			expectedSources:    []string{"auto_created", "auto_created"},
			verifyDeduplication: true,
		},
		{
			name:        "participants + extracted people",
			senderEmail: "alice@example.com",
			participantEmails: toParticipants([]string{"bob@example.com"}),
			extraction: &workflows.SLMPipelineExtractEntitiesOutput{
				People: []workflows.PersonResult{
					{Name: "Carol Davis", Role: "PM"},
				},
			},
			expectedPeople:  2, // bob created, carol unresolved (no fuzzy match)
			expectedSources: []string{"auto_created"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := make(map[string]int)

			entityResolver := &mockEntityResolver{
				resolveOrCreateFunc: func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
					callCount[email]++
					// Simulate creating a new person for each unique email
					return &entities.ResolutionResult{
						Person: &entities.Person{
							ID:            int64(100 + len(callCount)),
							CanonicalName: displayName,
							PrimaryEmail:  email,
							IsInternal:    false,
							Confidence:    0.7,
						},
						Confidence: 0.7,
						Source:     "auto_created",
						IsNew:      true,
					}, nil
				},
			}

			entityLookup := &mockEntityLookup{
				searchPeopleByNameFunc: func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
					// No fuzzy matches
					return nil, nil
				},
			}

			activities := NewContextBuilderActivities(
				logger,
				entityResolver,
				entityLookup,
				&mockContextPackageRepo{},
				nil,
				nil,
				nil,
			)

			input := workflows.BuildContextInput{
				TenantID:          "test-tenant",
				SourceID:          1,
				ContentType:       "email",
				SenderEmail:       tt.senderEmail,
				SenderName:        tt.senderName,
				ParticipantEmails: tt.participantEmails,
				Extraction:        tt.extraction,
			}

			output, err := activities.BuildContextPackage(ctx, input)
			if err != nil {
				t.Fatalf("BuildContextPackage failed: %v", err)
			}

			if len(output.ResolvedPeople) != tt.expectedPeople {
				t.Errorf("Expected %d resolved people, got %d", tt.expectedPeople, len(output.ResolvedPeople))
			}

			for i, expectedSource := range tt.expectedSources {
				if i < len(output.ResolvedPeople) && output.ResolvedPeople[i].Source != expectedSource {
					t.Errorf("Person %d: expected source %s, got %s", i, expectedSource, output.ResolvedPeople[i].Source)
				}
			}

			// Verify deduplication: each email should be called at most once
			if tt.verifyDeduplication {
				for email, count := range callCount {
					if count > 1 {
						t.Errorf("Email %s was processed %d times, expected 1 (deduplication failed)", email, count)
					}
				}
			}
		})
	}
}

// TestBugPf99a49b_RolePersistence tests bug pf-99a49b: extracted Role field is never persisted to job_title.
// This test MUST FAIL until the bug is fixed.
//
// BUG: When resolvePeople() processes a PersonResult with a non-empty Role field,
// and the matched person's current job_title is NULL, the Role should be persisted
// to the database via job_title. Currently, the Role flows through PersonResult and
// ResolvedPerson structs but is never written to the people.job_title column.
func TestBugPf99a49b_RolePersistence(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Track what gets written to the repository
	var updatedPersons []struct {
		personID int64
		jobTitle string
	}

	// Mock entity lookup that returns a person with NULL job_title
	entityLookup := &mockEntityLookup{
		searchPeopleByNameFunc: func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
			if name == "John Doe" {
				return []*entities.Person{
					{
						ID:            123,
						CanonicalName: "John Doe",
						Title:         "", // NULL job_title in database
						IsInternal:    true,
					},
				}, nil
			}
			return nil, nil
		},
		updatePersonTitleFunc: func(ctx context.Context, personID int64, title string) error {
			updatedPersons = append(updatedPersons, struct {
				personID int64
				jobTitle string
			}{personID, title})
			return nil
		},
	}

	activities := NewContextBuilderActivities(
		logger,
		&mockEntityResolver{},
		entityLookup,
		&mockContextPackageRepo{},
		nil,
		nil,
		nil,
	)

	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			People: []workflows.PersonResult{
				{Name: "John Doe", Role: "Senior Engineer"},
			},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	if len(output.ResolvedPeople) != 1 {
		t.Fatalf("Expected 1 resolved person, got %d", len(output.ResolvedPeople))
	}

	resolvedPerson := output.ResolvedPeople[0]

	// Verify the Role is present in the PersonResult input
	if input.Extraction.People[0].Role != "Senior Engineer" {
		t.Errorf("Expected PersonResult.Role to be 'Senior Engineer', got '%s'", input.Extraction.People[0].Role)
	}

	// Verify the Role flows through to ResolvedPerson
	if resolvedPerson.Role != "Senior Engineer" {
		t.Errorf("Expected ResolvedPerson.Role to be 'Senior Engineer', got '%s'", resolvedPerson.Role)
	}

	// FIXED: The Title field should have been updated from the Role
	// Expected behavior: When person.Title is empty/NULL and PersonResult.Role is non-empty,
	// the Role should be written to job_title in the database and reflected in Title.
	if resolvedPerson.Title != "Senior Engineer" {
		t.Errorf("Person 123 should have Title 'Senior Engineer', got '%s'", resolvedPerson.Title)
	}

	// Verify database update was attempted
	if len(updatedPersons) != 1 {
		t.Errorf("Expected UpdatePersonTitle to be called once, got %d calls", len(updatedPersons))
	} else {
		update := updatedPersons[0]
		if update.personID != 123 {
			t.Errorf("Expected UpdatePersonTitle for person 123, got %d", update.personID)
		}
		if update.jobTitle != "Senior Engineer" {
			t.Errorf("Expected UpdatePersonTitle with title 'Senior Engineer', got '%s'", update.jobTitle)
		}
	}
}

// TestBugPf99a49b_GarbageTitleFilter tests bug pf-99a49b: garbage titles from meeting invites.
// This test MUST FAIL until the bug is fixed.
//
// BUG: Meeting invitation text like "Tap to call in from a mobile device (attendees only)"
// is being extracted as a Role and stored as job_title. We need filtering to prevent
// obviously non-job-title strings from being persisted.
func TestBugPf99a49b_GarbageTitleFilter(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	garbageTitles := []string{
		"Tap to call in from a mobile device (attendees only)",
		"Join my meeting",
		"Click here to join the meeting",
		"Attendees only",
		"Join Webex Meeting",
	}

	for _, garbageTitle := range garbageTitles {
		t.Run(garbageTitle, func(t *testing.T) {
			// Track UpdatePersonTitle calls
			updateCalled := false

			entityLookup := &mockEntityLookup{
				searchPeopleByNameFunc: func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
					return []*entities.Person{
						{
							ID:            456,
							CanonicalName: "Jane Smith",
							Title:         "", // NULL job_title
							IsInternal:    true,
						},
					}, nil
				},
				updatePersonTitleFunc: func(ctx context.Context, personID int64, title string) error {
					updateCalled = true
					t.Errorf("UpdatePersonTitle should NOT be called for garbage title '%s'", title)
					return nil
				},
			}

			activities := NewContextBuilderActivities(
				logger,
				&mockEntityResolver{},
				entityLookup,
				&mockContextPackageRepo{},
				nil,
				nil,
				nil,
			)

			input := workflows.BuildContextInput{
				TenantID:    "test-tenant",
				SourceID:    1,
				ContentType: "meeting",
				Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
					People: []workflows.PersonResult{
						{Name: "Jane Smith", Role: garbageTitle},
					},
				},
			}

			output, err := activities.BuildContextPackage(ctx, input)
			if err != nil {
				t.Fatalf("BuildContextPackage failed: %v", err)
			}

			if len(output.ResolvedPeople) != 1 {
				t.Fatalf("Expected 1 resolved person, got %d", len(output.ResolvedPeople))
			}

			resolvedPerson := output.ResolvedPeople[0]

			// FIXED: Garbage meeting text should NOT be persisted as job_title
			// The Role field may still contain the garbage text (that's fine - it's metadata)
			// but the Title should remain empty and UpdatePersonTitle should not be called
			if resolvedPerson.Title == garbageTitle {
				t.Errorf("Garbage title '%s' should not be persisted to Title field", garbageTitle)
			}

			if updateCalled {
				t.Errorf("UpdatePersonTitle should not be called for garbage title '%s'", garbageTitle)
			}

			t.Logf("PASS: Garbage title '%s' was filtered out - Title='%s', UpdatePersonTitle called=%v",
				garbageTitle, resolvedPerson.Title, updateCalled)
		})
	}
}

// TestBugPf99a49b_NoOverwrite tests bug pf-99a49b: Role should not overwrite existing job_title.
// This test documents the expected contract: if a person already has a job_title,
// the extracted Role should NOT overwrite it.
//
// This may pass or fail depending on current behavior, but it documents the requirement.
func TestBugPf99a49b_NoOverwrite(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	entityLookup := &mockEntityLookup{
		searchPeopleByNameFunc: func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
			if name == "Bob Jones" {
				return []*entities.Person{
					{
						ID:            789,
						CanonicalName: "Bob Jones",
						Title:         "VP Engineering", // Existing non-NULL job_title
						IsInternal:    true,
					},
				}, nil
			}
			return nil, nil
		},
	}

	activities := NewContextBuilderActivities(
		logger,
		&mockEntityResolver{},
		entityLookup,
		&mockContextPackageRepo{},
		nil,
		nil,
		nil,
	)

	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			People: []workflows.PersonResult{
				{Name: "Bob Jones", Role: "Engineer"}, // Extracted role (less specific than existing)
			},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	if len(output.ResolvedPeople) != 1 {
		t.Fatalf("Expected 1 resolved person, got %d", len(output.ResolvedPeople))
	}

	resolvedPerson := output.ResolvedPeople[0]

	// EXPECTED CONTRACT: Existing job_title should NOT be overwritten by extracted Role
	// Current behavior: Title comes from database lookup, so it's preserved (good!)
	// But we need to ensure the update logic (when added) respects this rule
	if resolvedPerson.Title != "VP Engineering" {
		t.Errorf("Existing Title should be preserved: expected 'VP Engineering', got '%s'", resolvedPerson.Title)
	}

	// The Role field may still contain the extracted value (that's fine - it's metadata)
	if resolvedPerson.Role != "Engineer" {
		t.Logf("Role field: expected 'Engineer', got '%s'", resolvedPerson.Role)
	}

	t.Logf("PASS: Existing job_title 'VP Engineering' was not overwritten by extracted Role 'Engineer'")
}

// TestBugPf96c91a_SingleCharacterFuzzyMatch tests bug pf-96c91a: single-character mentions
// incorrectly resolve via fuzzy matching without minimum length validation.
// This test MUST FAIL until the bug is fixed.
//
// BUG: NameSimilarity("K", "Mike") returns 0.9 due to substring contains() check at line 227.
// Single-character mentions like "K" in an email should NOT match "Mike Johnson" with 0.9 similarity.
// The resolvePerson function uses NameSimilarity without minimum length validation, so "K" can
// incorrectly resolve to person_id 123 (Mike Johnson) with confidence 0.9.
//
// ROOT CAUSE:
// 1. NameSimilarity (pkg/enrichment/entities/normalize.go:227) returns 0.9 for ANY substring match
// 2. resolvePerson (services/worker/activities/context_builder.go:408) uses NameSimilarity without
//    checking minimum name length
// 3. No email participant context is used for disambiguation
//
// EXPECTED: Single-character names should score very low (< 0.3) and NOT resolve.
func TestBugPf96c91a_SingleCharacterFuzzyMatch(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	tests := []struct {
		name                 string
		extractedName        string
		candidateName        string
		shouldMatch          bool
		expectedConfidence   float32
		maxAllowedConfidence float32 // Bug threshold: confidence should be below this
	}{
		{
			name:                 "single char K vs Mike Johnson",
			extractedName:        "K",
			candidateName:        "Mike Johnson",
			shouldMatch:          false,
			expectedConfidence:   0.0,
			maxAllowedConfidence: 0.3, // BUG: currently returns 0.9
		},
		{
			name:                 "single char M vs Mike Johnson",
			extractedName:        "M",
			candidateName:        "Mike Johnson",
			shouldMatch:          false,
			expectedConfidence:   0.0,
			maxAllowedConfidence: 0.3, // BUG: currently returns 0.9
		},
		{
			name:                 "single char a vs Sarah",
			extractedName:        "a",
			candidateName:        "Sarah",
			shouldMatch:          false,
			expectedConfidence:   0.0,
			maxAllowedConfidence: 0.3, // BUG: currently returns 0.9
		},
		{
			name:                 "two char Mi vs Mike",
			extractedName:        "Mi",
			candidateName:        "Mike",
			shouldMatch:          false,
			expectedConfidence:   0.0,
			maxAllowedConfidence: 0.7, // BUG: currently returns 0.9
		},
		{
			name:                 "full name Mike vs Mike Johnson",
			extractedName:        "Mike",
			candidateName:        "Mike Johnson",
			shouldMatch:          true,
			expectedConfidence:   0.85, // This is intended behavior (partial match)
			maxAllowedConfidence: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entityLookup := &mockEntityLookup{
				searchPeopleByNameFunc: func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
					// Return a candidate that matches the test case
					return []*entities.Person{
						{
							ID:            123,
							CanonicalName: tt.candidateName,
							IsInternal:    true,
						},
					}, nil
				},
			}

			activities := NewContextBuilderActivities(
				logger,
				&mockEntityResolver{},
				entityLookup,
				&mockContextPackageRepo{},
				nil,
				nil,
				nil,
			)

			input := workflows.BuildContextInput{
				TenantID:    "test-tenant",
				SourceID:    1,
				ContentType: "email",
				Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
					People: []workflows.PersonResult{
						{Name: tt.extractedName, Role: ""},
					},
				},
			}

			output, err := activities.BuildContextPackage(ctx, input)
			if err != nil {
				t.Fatalf("BuildContextPackage failed: %v", err)
			}

			if !tt.shouldMatch {
				// BUG: Single-character names should NOT resolve (confidence should be < 0.7 threshold)
				if len(output.ResolvedPeople) > 0 && output.ResolvedPeople[0].PersonID != nil {
					confidence := output.ResolvedPeople[0].Confidence
					if confidence > tt.maxAllowedConfidence {
						t.Errorf("BUG pf-96c91a: '%s' matched '%s' with confidence %.2f (should be <= %.2f)",
							tt.extractedName, tt.candidateName, confidence, tt.maxAllowedConfidence)
						t.Errorf("Expected: unresolved (PersonID = nil)")
						t.Errorf("Got: PersonID = %d, confidence = %.2f", *output.ResolvedPeople[0].PersonID, confidence)
					}
				}
			} else {
				// This case should match (full name like "Mike")
				if len(output.ResolvedPeople) == 0 || output.ResolvedPeople[0].PersonID == nil {
					t.Errorf("Expected '%s' to match '%s', but it didn't resolve", tt.extractedName, tt.candidateName)
				} else {
					confidence := output.ResolvedPeople[0].Confidence
					if confidence < 0.7 {
						t.Errorf("Expected '%s' to match '%s' with confidence >= 0.7, got %.2f",
							tt.extractedName, tt.candidateName, confidence)
					}
				}
			}
		})
	}
}

// TestEnrichPeopleFromHeaders tests pf-4d7830: NER first-name-only enrichment from email headers.
func TestEnrichPeopleFromHeaders(t *testing.T) {
	tests := []struct {
		name         string
		people       []workflows.PersonResult
		senderEmail  string
		senderName   string
		participants []workflows.Participant
		expected     map[string]string // original name → expected enriched name
	}{
		{
			name: "single first name enriched from participant",
			people: []workflows.PersonResult{
				{Name: "Tim", Role: ""},
				{Name: "James", Role: ""},
			},
			participants: []workflows.Participant{
				{Email: "tim.dunn@example.com", DisplayName: "Tim Dunn", HeaderRole: "to"},
				{Email: "james.dement@example.com", DisplayName: "James DeMent", HeaderRole: "cc"},
			},
			expected: map[string]string{
				"Tim":   "Tim Dunn",
				"James": "James DeMent",
			},
		},
		{
			name: "enriched from sender",
			people: []workflows.PersonResult{
				{Name: "Miroslav", Role: ""},
			},
			senderEmail: "miroslav.ponec@example.com",
			senderName:  "Miroslav Ponec",
			expected: map[string]string{
				"Miroslav": "Miroslav Ponec",
			},
		},
		{
			name: "multi-word name not enriched",
			people: []workflows.PersonResult{
				{Name: "Toby Paler", Role: ""},
			},
			participants: []workflows.Participant{
				{Email: "toby.paler@example.com", DisplayName: "Toby Paler", HeaderRole: "to"},
			},
			expected: map[string]string{
				"Toby Paler": "Toby Paler", // unchanged
			},
		},
		{
			name: "ambiguous first name not enriched",
			people: []workflows.PersonResult{
				{Name: "James", Role: ""},
			},
			participants: []workflows.Participant{
				{Email: "james.a@example.com", DisplayName: "James Anderson", HeaderRole: "to"},
				{Email: "james.b@example.com", DisplayName: "James Brown", HeaderRole: "cc"},
			},
			expected: map[string]string{
				"James": "James", // ambiguous, not enriched
			},
		},
		{
			name: "no matching participant",
			people: []workflows.PersonResult{
				{Name: "Unknown", Role: ""},
			},
			participants: []workflows.Participant{
				{Email: "alice@example.com", DisplayName: "Alice Smith", HeaderRole: "to"},
			},
			expected: map[string]string{
				"Unknown": "Unknown", // no match
			},
		},
		{
			name:     "empty people list",
			people:   []workflows.PersonResult{},
			expected: map[string]string{},
		},
		{
			name: "participant with single-word display name",
			people: []workflows.PersonResult{
				{Name: "Admin", Role: ""},
			},
			participants: []workflows.Participant{
				{Email: "admin@example.com", DisplayName: "Admin", HeaderRole: "to"},
			},
			expected: map[string]string{
				"Admin": "Admin", // single-word display name, can't enrich
			},
		},
		{
			name: "Last, First format in headers",
			people: []workflows.PersonResult{
				{Name: "Tim", Role: ""},
				{Name: "Miroslav", Role: ""},
			},
			participants: []workflows.Participant{
				{Email: "tim.dunn@example.com", DisplayName: "Dunn, Tim", HeaderRole: "to"},
				{Email: "miroslav.ponec@example.com", DisplayName: "Ponec, Miroslav", HeaderRole: "cc"},
			},
			expected: map[string]string{
				"Tim":      "Tim Dunn",
				"Miroslav": "Miroslav Ponec",
			},
		},
		{
			name: "Last, First Middle format",
			people: []workflows.PersonResult{
				{Name: "James", Role: ""},
			},
			participants: []workflows.Participant{
				{Email: "james@example.com", DisplayName: "DeMent, James Robert", HeaderRole: "to"},
			},
			expected: map[string]string{
				"James": "James Robert DeMent",
			},
		},
		{
			name: "mixed First Last and Last, First formats",
			people: []workflows.PersonResult{
				{Name: "Tim", Role: ""},
				{Name: "Alice", Role: ""},
			},
			participants: []workflows.Participant{
				{Email: "tim@example.com", DisplayName: "Dunn, Tim", HeaderRole: "to"},
				{Email: "alice@example.com", DisplayName: "Alice Smith", HeaderRole: "cc"},
			},
			expected: map[string]string{
				"Tim":   "Tim Dunn",
				"Alice": "Alice Smith",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enrichPeopleFromHeaders(tt.people, tt.senderEmail, tt.senderName, tt.participants)

			if len(result) != len(tt.people) {
				t.Fatalf("Expected %d results, got %d", len(tt.people), len(result))
			}

			for i, person := range result {
				originalName := tt.people[i].Name
				expectedName, ok := tt.expected[originalName]
				if !ok {
					continue
				}
				if person.Name != expectedName {
					t.Errorf("Person %q: expected enriched name %q, got %q", originalName, expectedName, person.Name)
				}
			}
		})
	}
}

// TestReclassifyOrganisations tests pf-a2cf48: NER org→project reclassification.
func TestReclassifyOrganisations(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	tests := []struct {
		name               string
		extraction         *workflows.SLMPipelineExtractEntitiesOutput
		setupEntityMock    func(*mockEntityLookup)
		setupContextRepo   func() *mockContextPackageRepo
		expectedProjects   []string
		expectedOrgs       []string
	}{
		{
			name: "CLIC reclassified from org to project",
			extraction: &workflows.SLMPipelineExtractEntitiesOutput{
				Projects:      []string{"DevCloud"},
				Organisations: []string{"TikTok", "CLIC", "Juniper"},
			},
			setupEntityMock: func(m *mockEntityLookup) {
				m.getProjectByNameFunc = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
					if name == "CLIC" {
						return &entities.Project{ID: 1, Name: "CLIC"}, nil
					}
					return nil, nil
				}
			},
			setupContextRepo: func() *mockContextPackageRepo {
				return &mockContextPackageRepo{}
			},
			expectedProjects: []string{"DevCloud", "CLIC"},
			expectedOrgs:     []string{"TikTok", "Juniper"},
		},
		{
			name: "MTC reclassified via keyword match",
			extraction: &workflows.SLMPipelineExtractEntitiesOutput{
				Projects:      []string{"Oslo"},
				Organisations: []string{"MTC", "Akamai"},
			},
			setupEntityMock: func(m *mockEntityLookup) {
				m.getProjectByNameFunc = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
					return nil, nil // no exact match
				}
			},
			setupContextRepo: func() *mockContextPackageRepo {
				id := int64(42)
				return &mockContextPackageRepo{
					projectsByKeyword: map[string]*int64{
						"MTC": &id,
					},
				}
			},
			expectedProjects: []string{"Oslo", "MTC"},
			expectedOrgs:     []string{"Akamai"},
		},
		{
			name: "no reclassification needed",
			extraction: &workflows.SLMPipelineExtractEntitiesOutput{
				Projects:      []string{"DevCloud"},
				Organisations: []string{"TikTok", "Juniper"},
			},
			setupEntityMock: func(m *mockEntityLookup) {
				m.getProjectByNameFunc = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
					return nil, nil
				}
			},
			setupContextRepo: func() *mockContextPackageRepo {
				return &mockContextPackageRepo{}
			},
			expectedProjects: []string{"DevCloud"},
			expectedOrgs:     []string{"TikTok", "Juniper"},
		},
		{
			name: "dedup: org already in projects list",
			extraction: &workflows.SLMPipelineExtractEntitiesOutput{
				Projects:      []string{"CLIC"},
				Organisations: []string{"CLIC", "Juniper"},
			},
			setupEntityMock: func(m *mockEntityLookup) {
				m.getProjectByNameFunc = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
					if name == "CLIC" {
						return &entities.Project{ID: 1, Name: "CLIC"}, nil
					}
					return nil, nil
				}
			},
			setupContextRepo: func() *mockContextPackageRepo {
				return &mockContextPackageRepo{}
			},
			expectedProjects: []string{"CLIC"}, // not duplicated
			expectedOrgs:     []string{"Juniper"},
		},
		{
			name: "glossary match triggers reclassification",
			extraction: &workflows.SLMPipelineExtractEntitiesOutput{
				Projects:      []string{},
				Organisations: []string{"CLIC"},
			},
			setupEntityMock: func(m *mockEntityLookup) {
				m.getProjectByNameFunc = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
					return nil, nil
				}
			},
			setupContextRepo: func() *mockContextPackageRepo {
				return &mockContextPackageRepo{
					glossaryTerms: []ContextGlossaryTerm{
						{Term: "CLIC", Expansion: "Client Integration Component"},
					},
				}
			},
			expectedProjects: []string{"CLIC"},
			expectedOrgs:     []string(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entityLookup := &mockEntityLookup{}
			tt.setupEntityMock(entityLookup)
			contextRepo := tt.setupContextRepo()

			activities := NewContextBuilderActivities(
				logger,
				&mockEntityResolver{},
				entityLookup,
				contextRepo,
				nil,
				nil,
				nil,
			)

			activities.reclassifyOrganisations(ctx, "test-tenant", tt.extraction)

			// Check projects
			if len(tt.extraction.Projects) != len(tt.expectedProjects) {
				t.Errorf("Expected %d projects %v, got %d %v",
					len(tt.expectedProjects), tt.expectedProjects,
					len(tt.extraction.Projects), tt.extraction.Projects)
			} else {
				for i, expected := range tt.expectedProjects {
					if tt.extraction.Projects[i] != expected {
						t.Errorf("Project[%d]: expected %q, got %q", i, expected, tt.extraction.Projects[i])
					}
				}
			}

			// Check organisations
			if len(tt.extraction.Organisations) != len(tt.expectedOrgs) {
				t.Errorf("Expected %d orgs %v, got %d %v",
					len(tt.expectedOrgs), tt.expectedOrgs,
					len(tt.extraction.Organisations), tt.extraction.Organisations)
			} else {
				for i, expected := range tt.expectedOrgs {
					if tt.extraction.Organisations[i] != expected {
						t.Errorf("Org[%d]: expected %q, got %q", i, expected, tt.extraction.Organisations[i])
					}
				}
			}
		})
	}
}

// TestBuildContext_EnrichPeopleIntegration tests that enrichPeopleFromHeaders integrates
// correctly into the full BuildContextPackage flow (pf-4d7830).
func TestBuildContext_EnrichPeopleIntegration(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Mock: "Tim Dunn" exists in DB, NER extracted "Tim" (first name only)
	entityLookup := &mockEntityLookup{
		searchPeopleByNameFunc: func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
			if name == "Tim Dunn" { // enriched name should be used for lookup
				return []*entities.Person{
					{
						ID:            100,
						CanonicalName: "Tim Dunn",
						IsInternal:    true,
						Department:    "Cloud Networking",
					},
				}, nil
			}
			return nil, nil
		},
	}

	entityResolver := &mockEntityResolver{
		resolveOrCreateFunc: func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
			return &entities.ResolutionResult{
				Person: &entities.Person{
					ID:            200,
					CanonicalName: displayName,
					PrimaryEmail:  email,
					IsInternal:    true,
				},
				Confidence: 0.95,
				Source:     "exact_match",
			}, nil
		},
	}

	activities := NewContextBuilderActivities(
		logger,
		entityResolver,
		entityLookup,
		&mockContextPackageRepo{},
		nil,
		nil,
		nil,
	)

	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		SenderEmail: "james@example.com",
		SenderName:  "James Brown",
		ParticipantEmails: []workflows.Participant{
			{Email: "tim.dunn@example.com", DisplayName: "Tim Dunn", HeaderRole: "to"},
		},
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			People: []workflows.PersonResult{
				{Name: "Tim", Role: ""}, // first-name only from NER
			},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// "Tim" should have been enriched to "Tim Dunn" and resolved via fuzzy match
	foundTimDunn := false
	for _, rp := range output.ResolvedPeople {
		if rp.Name == "Tim Dunn" && rp.Source == "fuzzy" {
			foundTimDunn = true
			break
		}
	}
	if !foundTimDunn {
		t.Errorf("Expected 'Tim Dunn' to be resolved via enrichment + fuzzy match; resolved people: %+v", output.ResolvedPeople)
	}
}

// TestReclassifyOrganisations_CorrectedExtraction verifies that BuildContextPackage
// returns the reclassified extraction in CorrectedExtraction so the workflow can
// pass it to Stage 4 (pf-a2cf48: Temporal serialisation breaks in-place mutation).
func TestReclassifyOrganisations_CorrectedExtraction(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	entityLookup := &mockEntityLookup{
		getProjectByNameFunc: func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
			if name == "CLIC" {
				return &entities.Project{ID: 1, Name: "CLIC"}, nil
			}
			return nil, nil
		},
	}

	activities := NewContextBuilderActivities(
		logger,
		&mockEntityResolver{},
		entityLookup,
		&mockContextPackageRepo{},
		nil,
		nil,
		nil,
	)

	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			Projects:      []string{"DevCloud"},
			Organisations: []string{"TikTok", "CLIC", "Juniper"},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// CorrectedExtraction must be populated
	if output.CorrectedExtraction == nil {
		t.Fatal("CorrectedExtraction is nil — Stage 4 won't see reclassified orgs")
	}

	// CLIC should have moved from Organisations to Projects
	for _, org := range output.CorrectedExtraction.Organisations {
		if org == "CLIC" {
			t.Errorf("CLIC should not be in CorrectedExtraction.Organisations, got %v",
				output.CorrectedExtraction.Organisations)
		}
	}

	foundCLIC := false
	for _, proj := range output.CorrectedExtraction.Projects {
		if proj == "CLIC" {
			foundCLIC = true
		}
	}
	if !foundCLIC {
		t.Errorf("CLIC should be in CorrectedExtraction.Projects, got %v",
			output.CorrectedExtraction.Projects)
	}
}

// TestEnrichPeople_CorrectedExtraction verifies that BuildContextPackage
// propagates enriched people names (first-name → full name) to CorrectedExtraction
// so Stage 4 sees full names in the Entities section (pf-4d7830).
func TestEnrichPeople_CorrectedExtraction(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	activities := NewContextBuilderActivities(
		logger,
		&mockEntityResolver{},
		&mockEntityLookup{},
		&mockContextPackageRepo{},
		nil,
		nil,
		nil,
	)

	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		SenderEmail: "miroslav.ponec@example.com",
		SenderName:  "Ponec, Miroslav",
		ParticipantEmails: []workflows.Participant{
			{Email: "tim.dunn@example.com", DisplayName: "Tim Dunn", HeaderRole: "to"},
		},
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			People: []workflows.PersonResult{
				{Name: "Miroslav"},
				{Name: "Tim"},
				{Name: "Toby Paler", Role: "PM"},
			},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	if output.CorrectedExtraction == nil {
		t.Fatal("CorrectedExtraction is nil — Stage 4 won't see enriched people")
	}

	// Build a name lookup from CorrectedExtraction.People
	names := make(map[string]bool)
	for _, p := range output.CorrectedExtraction.People {
		names[p.Name] = true
	}

	// "Miroslav" should be enriched to "Miroslav Ponec" (from "Ponec, Miroslav" sender header)
	if !names["Miroslav Ponec"] {
		t.Errorf("expected 'Miroslav Ponec' in CorrectedExtraction.People, got %v",
			output.CorrectedExtraction.People)
	}
	if names["Miroslav"] {
		t.Errorf("'Miroslav' should have been enriched to full name, got %v",
			output.CorrectedExtraction.People)
	}

	// "Tim" should be enriched to "Tim Dunn" (from participant header)
	if !names["Tim Dunn"] {
		t.Errorf("expected 'Tim Dunn' in CorrectedExtraction.People, got %v",
			output.CorrectedExtraction.People)
	}

	// "Toby Paler" already has a full name — should be unchanged
	if !names["Toby Paler"] {
		t.Errorf("expected 'Toby Paler' unchanged in CorrectedExtraction.People, got %v",
			output.CorrectedExtraction.People)
	}
}

// pf-0f08e0: Test that NER-resolved people are deduplicated against header-resolved people.
func TestBuildContext_NERDedupAgainstHeaders(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Miroslav is in both email headers (sender) and NER extraction.
	// After enrichment, NER "Miroslav" becomes "Miroslav Ponec" which fuzzy-matches
	// the same DB person. The resolved list should contain only one entry.
	entityLookup := &mockEntityLookup{
		searchPeopleByNameFunc: func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
			if name == "Miroslav Ponec" {
				return []*entities.Person{{
					ID:            42,
					CanonicalName: "Miroslav Ponec",
					IsInternal:    true,
				}}, nil
			}
			return nil, nil
		},
	}
	resolver := &mockEntityResolver{
		resolveOrCreateFunc: func(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error) {
			if email == "miroslav.ponec@example.com" {
				return &entities.ResolutionResult{
					Person:     &entities.Person{ID: 42, CanonicalName: "Miroslav Ponec", IsInternal: true},
					Confidence: 1.0,
					Source:     "exact",
				}, nil
			}
			return nil, nil
		},
	}

	activities := NewContextBuilderActivities(logger, resolver, entityLookup, &mockContextPackageRepo{}, nil, nil, nil)

	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		SenderEmail: "miroslav.ponec@example.com",
		SenderName:  "Miroslav Ponec",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			People: []workflows.PersonResult{
				{Name: "Miroslav", Role: ""},  // Will be enriched to "Miroslav Ponec"
				{Name: "Toby Paler", Role: ""}, // Not in headers, won't resolve
			},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// Should have exactly 1 resolved person (Miroslav Ponec from sender), not 2
	if len(output.ResolvedPeople) != 1 {
		t.Errorf("Expected 1 resolved person, got %d", len(output.ResolvedPeople))
		for _, rp := range output.ResolvedPeople {
			t.Logf("  - %s (source=%s, role=%s)", rp.Name, rp.Source, rp.Role)
		}
	}

	// The resolved person should be the sender entry
	if len(output.ResolvedPeople) > 0 {
		if output.ResolvedPeople[0].Role != "Sender" {
			t.Errorf("Expected role 'Sender', got %q", output.ResolvedPeople[0].Role)
		}
	}
}

// pf-0f08e0: Test that enrichPeopleFromHeaders deduplicates by name after enrichment.
func TestEnrichPeopleFromHeaders_DedupEnrichedNames(t *testing.T) {
	// NER extracts both "Miroslav Ponec" (full name) and "Miroslav" (first name).
	// After enrichment, "Miroslav" becomes "Miroslav Ponec" — duplicate of the first entry.
	people := []workflows.PersonResult{
		{Name: "Miroslav Ponec", Role: "Cloud Networking"},
		{Name: "Miroslav", Role: ""},
		{Name: "Tim", Role: ""},
	}
	participants := []workflows.Participant{
		{Email: "miroslav@example.com", DisplayName: "Miroslav Ponec", HeaderRole: "to"},
		{Email: "tim@example.com", DisplayName: "Tim Dunn", HeaderRole: "cc"},
	}

	result := enrichPeopleFromHeaders(people, "", "", participants)

	// Should have 2 unique people: "Miroslav Ponec" and "Tim Dunn"
	if len(result) != 2 {
		t.Fatalf("Expected 2 people after dedup, got %d", len(result))
	}
	if result[0].Name != "Miroslav Ponec" {
		t.Errorf("Expected first person to be 'Miroslav Ponec', got %q", result[0].Name)
	}
	// First occurrence (with Role) should be preserved
	if result[0].Role != "Cloud Networking" {
		t.Errorf("Expected role 'Cloud Networking' preserved, got %q", result[0].Role)
	}
	if result[1].Name != "Tim Dunn" {
		t.Errorf("Expected second person to be 'Tim Dunn', got %q", result[1].Name)
	}
}

// pf-0f08e0: Test that fuzzy-matched NER people use canonical DB name.
func TestResolvePerson_UsesCanonicalName(t *testing.T) {
	logger := logging.MustGlobal()

	entityLookup := &mockEntityLookup{
		searchPeopleByNameFunc: func(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error) {
			return []*entities.Person{{
				ID:            99,
				CanonicalName: "Timothy Dunn",
				Title:         "Engineer",
				IsInternal:    true,
			}}, nil
		},
	}

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, entityLookup, &mockContextPackageRepo{}, nil, nil, nil)

	rp := activities.resolvePerson(context.Background(), "test-tenant", workflows.PersonResult{
		Name: "Tim Dunn",
		Role: "lead",
	})

	if rp.PersonID == nil {
		t.Fatal("Expected person to be resolved")
	}
	// Name should be canonical DB name, not the NER-extracted name
	if rp.Name != "Timothy Dunn" {
		t.Errorf("Expected canonical name 'Timothy Dunn', got %q", rp.Name)
	}
}

func TestHeaderRoleLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"to", "To"},
		{"cc", "CC"},
		{"To", "To"},
		{"CC", "CC"},
		{"", ""},
		{"bcc", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := headerRoleLabel(tt.input)
			if got != tt.want {
				t.Errorf("headerRoleLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// mockTopicLookup implements TopicLookupInterface for testing.
type mockTopicLookup struct {
	listForContextFunc func(ctx context.Context, tenantID string, names []string) ([]TopicResult, error)
}

func (m *mockTopicLookup) GetByName(ctx context.Context, tenantID, name string) (TopicResult, error) {
	return TopicResult{}, nil
}
func (m *mockTopicLookup) ResolveByKeyword(ctx context.Context, tenantID, keyword string) (*int64, error) {
	return nil, nil
}
func (m *mockTopicLookup) GetByID(ctx context.Context, id int64) (TopicResult, error) {
	return TopicResult{}, nil
}
func (m *mockTopicLookup) ListForContext(ctx context.Context, tenantID string, names []string) ([]TopicResult, error) {
	if m.listForContextFunc != nil {
		return m.listForContextFunc(ctx, tenantID, names)
	}
	return nil, nil
}

func (m *mockTopicLookup) ScanContentForTopics(ctx context.Context, tenantID string, content string) ([]TopicResult, error) {
	return nil, nil
}

func TestBuildContextPackage_TopicResolution(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	topicLookup := &mockTopicLookup{
		listForContextFunc: func(ctx context.Context, tenantID string, names []string) ([]TopicResult, error) {
			// Return topics for "DevCloud" and "Oslo" but not "Unknown"
			var results []TopicResult
			for _, name := range names {
				switch name {
				case "DevCloud":
					results = append(results, TopicResult{ID: 1, Name: "DevCloud", Description: "Internal testing environment shared across teams"})
				case "Oslo":
					results = append(results, TopicResult{ID: 2, Name: "Oslo", Description: "Dedicated Linode region for MTC"})
				}
			}
			return results, nil
		},
	}

	activities := NewContextBuilderActivities(
		logger,
		&mockEntityResolver{},
		&mockEntityLookup{},
		&mockContextPackageRepo{},
		topicLookup,
		nil,
		nil,
	)

	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			// "DevCloud" and "Unknown" are unresolved projects (no DB match)
			// "Oslo" is an organisation that might be a topic
			Projects:      []string{"DevCloud", "Unknown"},
			Organisations: []string{"Oslo"},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// Should have 2 topic descriptions (DevCloud, Oslo) — "Unknown" has no topic match
	if len(output.ContextPackage.TopicDescriptions) != 2 {
		t.Fatalf("expected 2 topic descriptions, got %d: %+v",
			len(output.ContextPackage.TopicDescriptions), output.ContextPackage.TopicDescriptions)
	}

	// Verify topic content
	topicsByName := make(map[string]string)
	for _, td := range output.ContextPackage.TopicDescriptions {
		topicsByName[td.Name] = td.Description
	}

	if desc, ok := topicsByName["DevCloud"]; !ok || desc != "Internal testing environment shared across teams" {
		t.Errorf("DevCloud topic missing or wrong description: %q", desc)
	}
	if desc, ok := topicsByName["Oslo"]; !ok || desc != "Dedicated Linode region for MTC" {
		t.Errorf("Oslo topic missing or wrong description: %q", desc)
	}
}

func TestBuildContextPackage_TopicSkipsDescriptionless(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	topicLookup := &mockTopicLookup{
		listForContextFunc: func(ctx context.Context, tenantID string, names []string) ([]TopicResult, error) {
			// Return a topic without description — should be filtered out
			return []TopicResult{
				{ID: 1, Name: "SomeEmptyTopic", Description: ""},
				{ID: 2, Name: "RealTopic", Description: "A real description"},
			}, nil
		},
	}

	activities := NewContextBuilderActivities(
		logger,
		&mockEntityResolver{},
		&mockEntityLookup{},
		&mockContextPackageRepo{},
		topicLookup,
		nil,
		nil,
	)

	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			Organisations: []string{"SomeEmptyTopic", "RealTopic"},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// Only RealTopic should appear (SomeEmptyTopic has no description)
	if len(output.ContextPackage.TopicDescriptions) != 1 {
		t.Fatalf("expected 1 topic description, got %d", len(output.ContextPackage.TopicDescriptions))
	}
	if output.ContextPackage.TopicDescriptions[0].Name != "RealTopic" {
		t.Errorf("expected RealTopic, got %s", output.ContextPackage.TopicDescriptions[0].Name)
	}
}

// TestBuildContextPackage_TopicKeywordMatching verifies that candidates like
// "Oslo NLB Workflow" are passed to ListForContext, enabling keyword-based matching.
// Bug: pf-8700ec — NER extracts "Oslo NLB Workflow" but topic name is just "Oslo".
func TestBuildContextPackage_TopicKeywordMatching(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	var capturedNames []string
	topicLookup := &mockTopicLookup{
		listForContextFunc: func(ctx context.Context, tenantID string, names []string) ([]TopicResult, error) {
			capturedNames = names
			// Simulate keyword-matching repository: "Oslo NLB Workflow" matches
			// topic "Oslo" because keyword "oslo" is in the candidate.
			var results []TopicResult
			for _, name := range names {
				lower := strings.ToLower(name)
				if strings.Contains(lower, "oslo") {
					results = append(results, TopicResult{ID: 1, Name: "Oslo", Description: "Dedicated Linode region for MTC"})
				}
				if lower == "clic" {
					results = append(results, TopicResult{ID: 2, Name: "CLIC", Description: "Cloud Infrastructure Committee"})
				}
				if lower == "devcloud" {
					results = append(results, TopicResult{ID: 3, Name: "DevCloud", Description: "Internal testing environment"})
				}
			}
			return results, nil
		},
	}

	activities := NewContextBuilderActivities(
		logger,
		&mockEntityResolver{},
		&mockEntityLookup{},
		&mockContextPackageRepo{},
		topicLookup,
		nil,
		nil,
	)

	input := workflows.BuildContextInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		ContentType: "email",
		Extraction: &workflows.SLMPipelineExtractEntitiesOutput{
			// NER extracts "Oslo NLB Workflow" (not exact match for topic "Oslo")
			// CLIC and DevCloud match by exact name
			Projects:      []string{"Oslo NLB Workflow", "CLIC"},
			Organisations: []string{"DevCloud"},
		},
	}

	output, err := activities.BuildContextPackage(ctx, input)
	if err != nil {
		t.Fatalf("BuildContextPackage failed: %v", err)
	}

	// Verify candidates were passed including "Oslo NLB Workflow"
	found := false
	for _, name := range capturedNames {
		if name == "Oslo NLB Workflow" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Oslo NLB Workflow' in candidates passed to ListForContext, got: %v", capturedNames)
	}

	// Should have 3 topic descriptions: Oslo (keyword), CLIC (exact), DevCloud (exact)
	if len(output.ContextPackage.TopicDescriptions) != 3 {
		t.Fatalf("expected 3 topic descriptions, got %d: %+v",
			len(output.ContextPackage.TopicDescriptions), output.ContextPackage.TopicDescriptions)
	}

	topicsByName := make(map[string]string)
	for _, td := range output.ContextPackage.TopicDescriptions {
		topicsByName[td.Name] = td.Description
	}

	if _, ok := topicsByName["Oslo"]; !ok {
		t.Error("Oslo topic missing — keyword match should have resolved it")
	}
	if _, ok := topicsByName["CLIC"]; !ok {
		t.Error("CLIC topic missing")
	}
	if _, ok := topicsByName["DevCloud"]; !ok {
		t.Error("DevCloud topic missing")
	}
}
