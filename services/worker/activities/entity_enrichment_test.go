package activities

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// mockPersonRepository implements person update operations for testing.
type mockPersonRepository struct {
	updatePersonFunc     func(ctx context.Context, p *entities.Person) error
	getPersonByIDFunc    func(ctx context.Context, id int64) (*entities.Person, error)
	getPeopleByDomainFunc func(ctx context.Context, tenantID, domain string) ([]*entities.Person, error)
}

func (m *mockPersonRepository) UpdatePerson(ctx context.Context, p *entities.Person) error {
	if m.updatePersonFunc != nil {
		return m.updatePersonFunc(ctx, p)
	}
	return nil
}

func (m *mockPersonRepository) GetPersonByID(ctx context.Context, id int64) (*entities.Person, error) {
	if m.getPersonByIDFunc != nil {
		return m.getPersonByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockPersonRepository) GetPeopleByDomain(ctx context.Context, tenantID, domain string) ([]*entities.Person, error) {
	if m.getPeopleByDomainFunc != nil {
		return m.getPeopleByDomainFunc(ctx, tenantID, domain)
	}
	return nil, nil
}

// TestEnrichPersonMetadata_ActivityExists verifies the activity can be constructed.
func TestEnrichPersonMetadata_ActivityExists(t *testing.T) {
	logger := logging.MustGlobal()
	personRepo := &mockPersonRepository{}

	// This should NOT panic
	activity := NewPersonEnrichmentActivities(logger, personRepo, []string{"example.com"})

	if activity == nil {
		t.Fatal("Expected non-nil activity")
	}
}

// TestEnrichPersonMetadata_CompanyFromDomain verifies company enrichment from email domain.
func TestEnrichPersonMetadata_CompanyFromDomain(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	tests := []struct {
		name            string
		email           string
		existingCompany string
		expectedCompany string
		shouldUpdate    bool
	}{
		{
			name:            "akamai.com domain",
			email:           "john@akamai.com",
			existingCompany: "",
			expectedCompany: "Akamai",
			shouldUpdate:    true,
		},
		{
			name:            "google.com domain",
			email:           "jane@google.com",
			existingCompany: "",
			expectedCompany: "Google",
			shouldUpdate:    true,
		},
		{
			name:            "existing company not overwritten",
			email:           "bob@akamai.com",
			existingCompany: "Akamai Technologies",
			expectedCompany: "Akamai Technologies",
			shouldUpdate:    false,
		},
		{
			name:            "unknown domain gets empty company",
			email:           "user@unknowndomain123.com",
			existingCompany: "",
			expectedCompany: "",
			shouldUpdate:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateCalled := false
			var updatedPerson *entities.Person

			personRepo := &mockPersonRepository{
				updatePersonFunc: func(ctx context.Context, p *entities.Person) error {
					updateCalled = true
					updatedPerson = p
					return nil
				},
			}

			activity := NewPersonEnrichmentActivities(logger, personRepo, []string{"example.com"})

			input := workflows.EnrichPersonMetadataInput{
				TenantID: "test-tenant",
				ResolvedPeople: []workflows.ResolvedPerson{
					{
						PersonID:   func() *int64 { id := int64(123); return &id }(),
						Name:       "Test Person",
						Title:      "",
						IsInternal: false,
					},
				},
			}

			// Simulate fetching the person from DB
			personRepo.getPersonByIDFunc = func(ctx context.Context, id int64) (*entities.Person, error) {
				return &entities.Person{
					ID:           id,
					TenantID:     "test-tenant",
					CanonicalName: "Test Person",
					PrimaryEmail: tt.email,
					Company:      tt.existingCompany,
					Title:        "",
					IsInternal:   false,
				}, nil
			}

			output, err := activity.EnrichPersonMetadata(ctx, input)
			if err != nil {
				t.Fatalf("EnrichPersonMetadata failed: %v", err)
			}

			if tt.shouldUpdate != updateCalled {
				t.Errorf("Expected updateCalled=%v, got %v", tt.shouldUpdate, updateCalled)
			}

			if tt.shouldUpdate && updatedPerson != nil {
				if updatedPerson.Company != tt.expectedCompany {
					t.Errorf("Expected company '%s', got '%s'", tt.expectedCompany, updatedPerson.Company)
				}
			}

			if output.PeopleEnriched != 0 && !tt.shouldUpdate {
				t.Errorf("Expected 0 people enriched, got %d", output.PeopleEnriched)
			}
		})
	}
}

// TestEnrichPersonMetadata_IsInternalFlag verifies is_internal flag based on configured domains.
func TestEnrichPersonMetadata_IsInternalFlag(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	internalDomains := []string{"example.com", "internal.org"}

	tests := []struct {
		name               string
		email              string
		existingIsInternal bool
		expectedIsInternal bool
		shouldUpdate       bool
	}{
		{
			name:               "internal domain sets flag",
			email:              "alice@example.com",
			existingIsInternal: false,
			expectedIsInternal: true,
			shouldUpdate:       true,
		},
		{
			name:               "internal subdomain sets flag",
			email:              "bob@sub.example.com",
			existingIsInternal: false,
			expectedIsInternal: true,
			shouldUpdate:       true,
		},
		{
			name:               "external domain does not set flag",
			email:              "carol@external.com",
			existingIsInternal: false,
			expectedIsInternal: false,
			shouldUpdate:       false,
		},
		{
			name:               "already internal not updated",
			email:              "david@example.com",
			existingIsInternal: true,
			expectedIsInternal: true,
			shouldUpdate:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateCalled := false
			var updatedPerson *entities.Person

			personRepo := &mockPersonRepository{
				updatePersonFunc: func(ctx context.Context, p *entities.Person) error {
					updateCalled = true
					updatedPerson = p
					return nil
				},
				getPersonByIDFunc: func(ctx context.Context, id int64) (*entities.Person, error) {
					return &entities.Person{
						ID:           id,
						TenantID:     "test-tenant",
						CanonicalName: "Test Person",
						PrimaryEmail: tt.email,
						IsInternal:   tt.existingIsInternal,
						Company:      "",
						Title:        "",
					}, nil
				},
			}

			activity := NewPersonEnrichmentActivities(logger, personRepo, internalDomains)

			input := workflows.EnrichPersonMetadataInput{
				TenantID: "test-tenant",
				ResolvedPeople: []workflows.ResolvedPerson{
					{
						PersonID:   func() *int64 { id := int64(123); return &id }(),
						Name:       "Test Person",
						IsInternal: tt.existingIsInternal,
					},
				},
			}

			output, err := activity.EnrichPersonMetadata(ctx, input)
			if err != nil {
				t.Fatalf("EnrichPersonMetadata failed: %v", err)
			}

			if tt.shouldUpdate != updateCalled {
				t.Errorf("Expected updateCalled=%v, got %v", tt.shouldUpdate, updateCalled)
			}

			if tt.shouldUpdate && updatedPerson != nil {
				if updatedPerson.IsInternal != tt.expectedIsInternal {
					t.Errorf("Expected is_internal=%v, got %v", tt.expectedIsInternal, updatedPerson.IsInternal)
				}
			}

			if output.PeopleEnriched == 0 && tt.shouldUpdate {
				t.Error("Expected at least 1 person enriched")
			}
		})
	}
}

// TestEnrichPersonMetadata_SignatureParsing verifies basic signature parsing for title extraction.
func TestEnrichPersonMetadata_SignatureParsing(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	tests := []struct {
		name          string
		signatureText string
		expectedTitle string
		shouldUpdate  bool
	}{
		{
			name: "simple signature block",
			signatureText: `--
John Smith
Senior Engineer
Akamai Technologies`,
			expectedTitle: "Senior Engineer",
			shouldUpdate:  true,
		},
		{
			name: "signature with separators",
			signatureText: `-----
Jane Doe | Product Manager
Example Corp`,
			expectedTitle: "Product Manager",
			shouldUpdate:  true,
		},
		{
			name:          "no signature",
			signatureText: "",
			expectedTitle: "",
			shouldUpdate:  true, // is_internal flips from false to true for example.com domain
		},
		{
			name: "signature without title",
			signatureText: `--
Bob Jones
Example Inc`,
			expectedTitle: "",
			shouldUpdate:  true, // is_internal flips from false to true for example.com domain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateCalled := false
			var updatedPerson *entities.Person

			personRepo := &mockPersonRepository{
				updatePersonFunc: func(ctx context.Context, p *entities.Person) error {
					updateCalled = true
					updatedPerson = p
					return nil
				},
				getPersonByIDFunc: func(ctx context.Context, id int64) (*entities.Person, error) {
					return &entities.Person{
						ID:           id,
						TenantID:     "test-tenant",
						CanonicalName: "Test Person",
						PrimaryEmail: "test@example.com",
						Title:        "", // No existing title
						Company:      "",
						IsInternal:   false,
					}, nil
				},
			}

			activity := NewPersonEnrichmentActivities(logger, personRepo, []string{"example.com"})

			input := workflows.EnrichPersonMetadataInput{
				TenantID:      "test-tenant",
				SignatureText: tt.signatureText,
				ResolvedPeople: []workflows.ResolvedPerson{
					{
						PersonID: func() *int64 { id := int64(123); return &id }(),
						Name:     "Test Person",
						Title:    "",
					},
				},
			}

			output, err := activity.EnrichPersonMetadata(ctx, input)
			if err != nil {
				t.Fatalf("EnrichPersonMetadata failed: %v", err)
			}

			if tt.shouldUpdate != updateCalled {
				t.Errorf("Expected updateCalled=%v, got %v", tt.shouldUpdate, updateCalled)
			}

			if tt.shouldUpdate && updatedPerson != nil {
				if updatedPerson.Title != tt.expectedTitle {
					t.Errorf("Expected title '%s', got '%s'", tt.expectedTitle, updatedPerson.Title)
				}
			}

			if output.PeopleEnriched == 0 && tt.shouldUpdate {
				t.Error("Expected at least 1 person enriched")
			}
		})
	}
}

// TestEnrichPersonMetadata_DoesNotOverwriteExisting verifies existing values are not overwritten.
func TestEnrichPersonMetadata_DoesNotOverwriteExisting(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	updateCalled := false

	personRepo := &mockPersonRepository{
		updatePersonFunc: func(ctx context.Context, p *entities.Person) error {
			updateCalled = true
			return nil
		},
		getPersonByIDFunc: func(ctx context.Context, id int64) (*entities.Person, error) {
			return &entities.Person{
				ID:           id,
				TenantID:     "test-tenant",
				CanonicalName: "Test Person",
				PrimaryEmail: "test@akamai.com",
				Title:        "Existing Title", // Already has title
				Company:      "Existing Company", // Already has company
				IsInternal:   true, // Already marked internal
			}, nil
		},
	}

	activity := NewPersonEnrichmentActivities(logger, personRepo, []string{"example.com"})

	input := workflows.EnrichPersonMetadataInput{
		TenantID: "test-tenant",
		SignatureText: `--
Test Person
New Title
New Company`,
		ResolvedPeople: []workflows.ResolvedPerson{
			{
				PersonID: func() *int64 { id := int64(123); return &id }(),
				Name:     "Test Person",
				Title:    "Existing Title",
			},
		},
	}

	output, err := activity.EnrichPersonMetadata(ctx, input)
	if err != nil {
		t.Fatalf("EnrichPersonMetadata failed: %v", err)
	}

	// Should NOT update because all fields already have values
	if updateCalled {
		t.Error("Expected no update since all fields have existing values")
	}

	if output.PeopleEnriched != 0 {
		t.Errorf("Expected 0 people enriched, got %d", output.PeopleEnriched)
	}
}

// TestEnrichPersonMetadata_EmptyInput verifies graceful handling of nil/empty input.
func TestEnrichPersonMetadata_EmptyInput(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	personRepo := &mockPersonRepository{}
	activity := NewPersonEnrichmentActivities(logger, personRepo, []string{"example.com"})

	tests := []struct {
		name  string
		input workflows.EnrichPersonMetadataInput
	}{
		{
			name: "nil resolved people",
			input: workflows.EnrichPersonMetadataInput{
				TenantID:       "test-tenant",
				ResolvedPeople: nil,
			},
		},
		{
			name: "empty resolved people",
			input: workflows.EnrichPersonMetadataInput{
				TenantID:       "test-tenant",
				ResolvedPeople: []workflows.ResolvedPerson{},
			},
		},
		{
			name: "people without person IDs",
			input: workflows.EnrichPersonMetadataInput{
				TenantID: "test-tenant",
				ResolvedPeople: []workflows.ResolvedPerson{
					{PersonID: nil, Name: "Unresolved Person"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := activity.EnrichPersonMetadata(ctx, tt.input)
			if err != nil {
				t.Fatalf("EnrichPersonMetadata failed: %v", err)
			}

			if output.PeopleEnriched != 0 {
				t.Errorf("Expected 0 people enriched for empty input, got %d", output.PeopleEnriched)
			}
		})
	}
}

// TestEnrichPersonMetadata_MultipleFields verifies enriching multiple fields at once.
func TestEnrichPersonMetadata_MultipleFields(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	updateCalled := false
	var updatedPerson *entities.Person

	personRepo := &mockPersonRepository{
		updatePersonFunc: func(ctx context.Context, p *entities.Person) error {
			updateCalled = true
			updatedPerson = p
			return nil
		},
		getPersonByIDFunc: func(ctx context.Context, id int64) (*entities.Person, error) {
			return &entities.Person{
				ID:           id,
				TenantID:     "test-tenant",
				CanonicalName: "John Smith",
				PrimaryEmail: "john@akamai.com",
				Title:        "", // Missing
				Company:      "", // Missing
				IsInternal:   false, // Incorrect
			}, nil
		},
	}

	activity := NewPersonEnrichmentActivities(logger, personRepo, []string{"akamai.com"})

	input := workflows.EnrichPersonMetadataInput{
		TenantID: "test-tenant",
		SignatureText: `--
John Smith
Senior Engineer
Akamai Technologies`,
		ResolvedPeople: []workflows.ResolvedPerson{
			{
				PersonID: func() *int64 { id := int64(123); return &id }(),
				Name:     "John Smith",
				Title:    "",
			},
		},
	}

	output, err := activity.EnrichPersonMetadata(ctx, input)
	if err != nil {
		t.Fatalf("EnrichPersonMetadata failed: %v", err)
	}

	if !updateCalled {
		t.Fatal("Expected update to be called")
	}

	if updatedPerson == nil {
		t.Fatal("Expected updated person to be non-nil")
	}

	// Verify all three fields were enriched
	if updatedPerson.Title != "Senior Engineer" {
		t.Errorf("Expected title 'Senior Engineer', got '%s'", updatedPerson.Title)
	}

	if updatedPerson.Company != "Akamai" {
		t.Errorf("Expected company 'Akamai', got '%s'", updatedPerson.Company)
	}

	if !updatedPerson.IsInternal {
		t.Error("Expected is_internal=true for akamai.com domain")
	}

	if output.PeopleEnriched != 1 {
		t.Errorf("Expected 1 person enriched, got %d", output.PeopleEnriched)
	}
}

// TestEnrichPersonMetadata_EmptyDomains_PreservesIsInternal verifies that when internalDomains
// is empty (tenant config not loaded), the activity does NOT clobber existing is_internal values.
// This is a regression test for bug pf-bba49a.
func TestEnrichPersonMetadata_EmptyDomains_PreservesIsInternal(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	tests := []struct {
		name               string
		existingIsInternal bool
		existingCompany    string
		existingTitle      string
		email              string
		signatureText      string
		expectedIsInternal bool
		shouldUpdate       bool
		description        string
	}{
		{
			name:               "preserves is_internal=true when metadata incomplete",
			existingIsInternal: true,
			existingCompany:    "",
			existingTitle:      "",
			email:              "alice@example.com",
			signatureText:      "",
			expectedIsInternal: true,
			shouldUpdate:       false, // No enrichment data available (example.com not in known domains)
			description:        "is_internal=true should NOT be clobbered to false when internalDomains is empty",
		},
		{
			name:               "preserves is_internal=false when metadata incomplete",
			existingIsInternal: false,
			existingCompany:    "",
			existingTitle:      "",
			email:              "bob@external.com",
			signatureText:      "",
			expectedIsInternal: false,
			shouldUpdate:       false, // No enrichment data available
			description:        "is_internal=false should stay false when internalDomains is empty (no false->true either)",
		},
		{
			name:               "preserves is_internal=true with company enrichment",
			existingIsInternal: true,
			existingCompany:    "",
			existingTitle:      "",
			email:              "charlie@akamai.com",
			signatureText: `--
Charlie Brown
Senior Engineer
Akamai Technologies`,
			expectedIsInternal: true,
			shouldUpdate:       true, // Both company and title enrichment
			description:        "Company/title enrichment should work, but is_internal=true must be preserved",
		},
		{
			name:               "no update when metadata complete and is_internal=true",
			existingIsInternal: true,
			existingCompany:    "Existing Company",
			existingTitle:      "Existing Title",
			email:              "david@example.com",
			signatureText:      "",
			expectedIsInternal: true,
			shouldUpdate:       false, // Metadata complete, skip is_internal check entirely
			description:        "Complete metadata should skip is_internal logic entirely",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateCalled := false
			var updatedPerson *entities.Person

			personRepo := &mockPersonRepository{
				updatePersonFunc: func(ctx context.Context, p *entities.Person) error {
					updateCalled = true
					updatedPerson = p
					return nil
				},
				getPersonByIDFunc: func(ctx context.Context, id int64) (*entities.Person, error) {
					return &entities.Person{
						ID:           id,
						TenantID:     "test-tenant",
						CanonicalName: "Test Person",
						PrimaryEmail: tt.email,
						Company:      tt.existingCompany,
						Title:        tt.existingTitle,
						IsInternal:   tt.existingIsInternal,
					}, nil
				},
			}

			// CRITICAL: Empty internalDomains array simulates tenant config not loaded
			activity := NewPersonEnrichmentActivities(logger, personRepo, []string{})

			input := workflows.EnrichPersonMetadataInput{
				TenantID:      "test-tenant",
				SignatureText: tt.signatureText,
				ResolvedPeople: []workflows.ResolvedPerson{
					{
						PersonID:   func() *int64 { id := int64(123); return &id }(),
						Name:       "Test Person",
						Title:      tt.existingTitle,
						IsInternal: tt.existingIsInternal,
					},
				},
			}

			output, err := activity.EnrichPersonMetadata(ctx, input)
			if err != nil {
				t.Fatalf("EnrichPersonMetadata failed: %v", err)
			}

			if tt.shouldUpdate != updateCalled {
				t.Errorf("Expected updateCalled=%v, got %v (reason: %s)",
					tt.shouldUpdate, updateCalled, tt.description)
			}

			// CRITICAL CHECK: is_internal must NOT change when internalDomains is empty
			if updateCalled && updatedPerson != nil {
				if updatedPerson.IsInternal != tt.expectedIsInternal {
					t.Errorf("BUG REPRODUCED: is_internal was clobbered! Expected is_internal=%v, got %v (reason: %s)",
						tt.expectedIsInternal, updatedPerson.IsInternal, tt.description)
				}
			}

			// Verify output count matches expectation
			if tt.shouldUpdate && output.PeopleEnriched == 0 {
				t.Errorf("Expected at least 1 person enriched, got 0")
			}
			if !tt.shouldUpdate && output.PeopleEnriched != 0 {
				t.Errorf("Expected 0 people enriched, got %d", output.PeopleEnriched)
			}
		})
	}
}
