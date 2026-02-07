package activities

import (
	"context"
	"testing"
	"time"

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

// Note: mockContextPackageRepo is defined in context_repo_test.go

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

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil)

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

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil)

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

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil)

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

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, entityLookup, contextRepo, nil)

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

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil)

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

	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, entityLookup, contextRepo, nil)

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

	tests := []struct {
		name              string
		participantEmails []workflows.Participant
		expectedPeople    int
		expectedFiltered  []string
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
		extraction           *ExtractEntitiesOutput
		expectedPeople       int
		expectedSources      []string
		verifyDeduplication  bool
	}{
		{
			name:              "participant emails only",
			participantEmails: toParticipants([]string{"alice@example.com", "bob@example.com"}),
			extraction:        &ExtractEntitiesOutput{},
			expectedPeople:    2,
			expectedSources:   []string{"auto_created", "auto_created"},
		},
		{
			name:              "sender + participants",
			senderEmail:       "alice@example.com",
			senderName:        "Alice Smith",
			participantEmails: toParticipants([]string{"bob@example.com", "carol@example.com"}),
			extraction:        &ExtractEntitiesOutput{},
			expectedPeople:    3,
			expectedSources:   []string{"auto_created", "auto_created", "auto_created"},
		},
		{
			name:               "deduplication: sender in participants",
			senderEmail:        "alice@example.com",
			senderName:         "Alice Smith",
			participantEmails:  toParticipants([]string{"alice@example.com", "bob@example.com"}),
			extraction:         &ExtractEntitiesOutput{},
			expectedPeople:     2,
			expectedSources:    []string{"auto_created", "auto_created"},
			verifyDeduplication: true,
		},
		{
			name:        "participants + extracted people",
			senderEmail: "alice@example.com",
			participantEmails: toParticipants([]string{"bob@example.com"}),
			extraction: &ExtractEntitiesOutput{
				People: []PersonResult{
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
			)

			input := BuildContextInput{
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
