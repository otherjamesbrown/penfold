package activities

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/parse"
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

// mockDomainCompanyRepository implements DomainCompanyRepository for testing.
type mockDomainCompanyRepository struct {
	getCompanyByDomainFunc func(ctx context.Context, tenantID, domain string) (string, error)
}

func (m *mockDomainCompanyRepository) GetCompanyByDomain(ctx context.Context, tenantID, domain string) (string, error) {
	if m.getCompanyByDomainFunc != nil {
		return m.getCompanyByDomainFunc(ctx, tenantID, domain)
	}
	return "", nil
}

// TestEnrichPersonMetadata_ActivityExists verifies the activity can be constructed.
func TestEnrichPersonMetadata_ActivityExists(t *testing.T) {
	logger := logging.MustGlobal()
	personRepo := &mockPersonRepository{}

	// This should NOT panic
	activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{"example.com"})

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

			activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{"example.com"})

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

			activity := NewPersonEnrichmentActivities(logger, personRepo, nil, internalDomains)

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

			activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{"example.com"})

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

	activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{"example.com"})

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
	activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{"example.com"})

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

	activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{"akamai.com"})

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

// TestEnrichPersonMetadata_ThreadedEmailSignatures verifies that title extraction works correctly
// for threaded emails with multiple senders and signatures. This is a regression test for bug pf-420aec.
//
// FIX: extractSignaturesPerSender() in pipeline.go now extracts per-sender signatures by splitting
// the email body into message blocks. EnrichPersonMetadata() uses PerSenderSignatures map to apply
// the correct signature to each person, preventing title cross-contamination.
//
// Example: Thread with John Smith (VP Engineering) and Sarah Chen (Product Manager).
// Each person now gets their own title from their own signature block.
func TestEnrichPersonMetadata_ThreadedEmailSignatures(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	// Simulate a threaded email body with two different signatures
	threadedEmailBody := `From: John Smith <john@akamai.com>
Subject: Re: Project Update

Thanks for the update. I'll review the roadmap this week.

John Smith
VP Engineering
Akamai Technologies

On Mon, Feb 10, 2026, Sarah Chen wrote:

Here's the latest project update. Please review.

Sarah Chen
Product Manager
Akamai Technologies`

	// Track updates for both people
	peopleUpdates := make(map[int64]*entities.Person)

	personRepo := &mockPersonRepository{
		updatePersonFunc: func(ctx context.Context, p *entities.Person) error {
			peopleUpdates[p.ID] = p
			return nil
		},
		getPersonByIDFunc: func(ctx context.Context, id int64) (*entities.Person, error) {
			// Return different people based on ID
			if id == 1 {
				return &entities.Person{
					ID:           1,
					TenantID:     "test-tenant",
					CanonicalName: "John Smith",
					PrimaryEmail: "john@akamai.com",
					Title:        "", // Missing - needs enrichment
					Company:      "",
					IsInternal:   false,
				}, nil
			}
			if id == 2 {
				return &entities.Person{
					ID:           2,
					TenantID:     "test-tenant",
					CanonicalName: "Sarah Chen",
					PrimaryEmail: "sarah@akamai.com",
					Title:        "", // Missing - needs enrichment
					Company:      "",
					IsInternal:   false,
				}, nil
			}
			return nil, nil
		},
	}

	activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{"akamai.com"})

	// FIX: Use per-sender signatures to map each person to their own signature
	perSenderSignatures := map[string]string{
		"John Smith": `John Smith
VP Engineering
Akamai Technologies`,
		"Sarah Chen": `Sarah Chen
Product Manager
Akamai Technologies`,
	}

	input := workflows.EnrichPersonMetadataInput{
		TenantID:            "test-tenant",
		PerSenderSignatures: perSenderSignatures, // FIX: Per-sender signatures
		BodyText:            threadedEmailBody,
		ResolvedPeople: []workflows.ResolvedPerson{
			{
				PersonID: func() *int64 { id := int64(1); return &id }(),
				Name:     "John Smith",
				Title:    "",
			},
			{
				PersonID: func() *int64 { id := int64(2); return &id }(),
				Name:     "Sarah Chen",
				Title:    "",
			},
		},
	}

	output, err := activity.EnrichPersonMetadata(ctx, input)
	if err != nil {
		t.Fatalf("EnrichPersonMetadata failed: %v", err)
	}

	// Verify both people were updated
	if output.PeopleEnriched != 2 {
		t.Errorf("Expected 2 people enriched, got %d", output.PeopleEnriched)
	}

	// CRITICAL ASSERTION: John Smith should have title "VP Engineering"
	johnUpdate, johnUpdated := peopleUpdates[1]
	if !johnUpdated {
		t.Fatal("Expected John Smith to be updated")
	}

	// FIX VERIFICATION: John should get "VP Engineering" from his own signature
	if johnUpdate.Title != "VP Engineering" {
		t.Errorf("John Smith title incorrect! Expected 'VP Engineering', got '%s'",
			johnUpdate.Title)
	}

	// CRITICAL ASSERTION: Sarah Chen should have title "Product Manager"
	sarahUpdate, sarahUpdated := peopleUpdates[2]
	if !sarahUpdated {
		t.Fatal("Expected Sarah Chen to be updated")
	}

	// FIX VERIFICATION: Sarah should get "Product Manager" from her own signature
	if sarahUpdate.Title != "Product Manager" {
		t.Errorf("Sarah Chen title incorrect! Expected 'Product Manager', got '%s'",
			sarahUpdate.Title)
	}
}

// TestEnrichPersonMetadata_HTMLSignatureTitleExtraction verifies that titles are extracted
// correctly from HTML signatures. This is a regression test for bug pf-a9fa80.
//
// BUG: htmlToText() in pkg/parse/email.go collapses HTML structure, losing line breaks.
// Signatures like '<div>Name</div><div>Title</div>' may collapse to a single line,
// causing parseSignatureForTitle() to fail since it expects line-based plaintext.
//
// Example HTML signatures that should work:
//   1. Div-based: <div>Aurore Defiolles</div><div>Cloud Sales Director - France</div>
//   2. Table-based: <table><tr><td>Luca Olivari</td></tr><tr><td>EMEA Vice President, Cloud</td></tr></table>
func TestEnrichPersonMetadata_HTMLSignatureTitleExtraction(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	tests := []struct {
		name           string
		htmlSignature  string
		personName     string
		expectedTitle  string
		shouldExtract  bool
		description    string
	}{
		{
			name: "div-based HTML signature",
			htmlSignature: `<div>Aurore Defiolles</div>
<div>Cloud Sales Director - France</div>
<div>Example Corp</div>`,
			personName:    "Aurore Defiolles",
			expectedTitle: "Cloud Sales Director - France",
			shouldExtract: true,
			description:   "Div tags should create line breaks, allowing title extraction",
		},
		{
			name: "table-based HTML signature",
			htmlSignature: `<table>
<tr><td>Luca Olivari</td></tr>
<tr><td>EMEA Vice President, Cloud</td></tr>
<tr><td>Tech Company</td></tr>
</table>`,
			personName:    "Luca Olivari",
			expectedTitle: "EMEA Vice President, Cloud",
			shouldExtract: true,
			description:   "Table rows should create line breaks, allowing title extraction",
		},
		{
			name: "inline HTML signature with pipes - FIXED",
			htmlSignature: `<span>Jane Smith</span><span> | </span><span>Senior Product Manager</span><span> | </span><span>Startup Inc</span>`,
			personName:    "Jane Smith",
			expectedTitle: "Senior Product Manager",
			shouldExtract: true, // FIXED: Pipe-check now happens BEFORE name-skip
			description:   "FIXED: Single-line signature with name | title | company now extracts title correctly",
		},
		{
			name: "inline HTML signature without separators - FAILS",
			htmlSignature: `<font>Carlos Mendez</font><font>VP Engineering</font><font>TechCorp</font>`,
			personName:    "Carlos Mendez",
			expectedTitle: "VP Engineering",
			shouldExtract: false, // BUG: This WILL fail - no line breaks, no separators
			description:   "Inline font tags collapse to single line without keywords proximity",
		},
		{
			name: "nested div signature",
			htmlSignature: `<div class="signature">
	<div class="name">Bob Jones</div>
	<div class="title">Technical Director</div>
	<div class="company">Engineering Firm</div>
</div>`,
			personName:    "Bob Jones",
			expectedTitle: "Technical Director",
			shouldExtract: true,
			description:   "Nested divs should preserve line structure",
		},
		{
			name: "Gmail-style signature with font and div - realistic",
			htmlSignature: `<div dir="ltr" class="gmail_signature">
<div dir="ltr">
<div><font face="arial, helvetica, sans-serif">Pierre Dubois</font></div>
<div><font face="arial, helvetica, sans-serif">Cloud Architect - EMEA</font></div>
<div><font face="arial, helvetica, sans-serif">Tech Solutions Inc.</font></div>
</div>
</div>`,
			personName:    "Pierre Dubois",
			expectedTitle: "Cloud Architect - EMEA",
			shouldExtract: true,
			description:   "Real-world Gmail signatures with nested divs and font tags",
		},
		{
			name: "Outlook signature with paragraphs",
			htmlSignature: `<p class="MsoNormal"><b><span style='font-size:10.0pt'>Maria Garcia</span></b></p>
<p class="MsoNormal"><span style='font-size:9.0pt'>Director of Product</span></p>
<p class="MsoNormal"><span style='font-size:9.0pt'>Innovation Labs</span></p>`,
			personName:    "Maria Garcia",
			expectedTitle: "Director of Product",
			shouldExtract: true,
			description:   "Outlook-style signatures with paragraph tags",
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
						CanonicalName: tt.personName,
						PrimaryEmail: "test@example.com",
						Title:        "", // Missing - needs enrichment from signature
						Company:      "",
						IsInternal:   false,
					}, nil
				},
			}

			activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{"example.com"})

			// Convert HTML signature to text using the current htmlToText() implementation
			// (via ParseEmailBody in pkg/parse/email.go)
			parsedEmail := parse.ParseEmailBody("", tt.htmlSignature)
			signatureText := parsedEmail.CleanBody

			input := workflows.EnrichPersonMetadataInput{
				TenantID:      "test-tenant",
				SignatureText: signatureText, // Pass the HTML-converted signature
				ResolvedPeople: []workflows.ResolvedPerson{
					{
						PersonID: func() *int64 { id := int64(123); return &id }(),
						Name:     tt.personName,
						Title:    "",
					},
				},
			}

			output, err := activity.EnrichPersonMetadata(ctx, input)
			if err != nil {
				t.Fatalf("EnrichPersonMetadata failed: %v", err)
			}

			// Check if title was extracted
			titleExtracted := updateCalled && updatedPerson != nil && updatedPerson.Title != ""

			if tt.shouldExtract && !titleExtracted {
				t.Errorf("BUG REPRODUCED: Title not extracted from HTML signature!\n"+
					"Expected title: '%s'\n"+
					"Signature text after HTML conversion:\n%s\n"+
					"Description: %s",
					tt.expectedTitle, signatureText, tt.description)
			}

			if tt.shouldExtract && titleExtracted {
				if updatedPerson.Title != tt.expectedTitle {
					t.Errorf("Title extracted but incorrect.\n"+
						"Expected: '%s'\n"+
						"Got: '%s'\n"+
						"Signature text after HTML conversion:\n%s",
						tt.expectedTitle, updatedPerson.Title, signatureText)
				}
			}

			// CRITICAL: For known bug cases, verify bug still exists
			// When bug is fixed, update shouldExtract to true for these cases
			if !tt.shouldExtract {
				if titleExtracted {
					t.Errorf("BUG FIXED! Update this test case to shouldExtract=true.\n"+
						"Title was extracted: '%s'\n"+
						"Description: %s",
						updatedPerson.Title, tt.description)
				} else {
					// Bug still exists - this is expected for now
					t.Logf("Bug confirmed: title not extracted (expected behavior). %s", tt.description)
				}
			}

			// Verify output count
			if tt.shouldExtract && output.PeopleEnriched == 0 {
				t.Error("Expected at least 1 person enriched")
			}
		})
	}
}

// TestEnrichPersonMetadata_LongTitleTruncation verifies that titles longer than 200 characters
// are properly truncated before being passed to UpdatePerson(). This is a regression test for
// bug pf-331505.
//
// ROOT CAUSE: parseSignatureForTitle() in entity_enrichment.go returns untruncated strings
// (lines 252, 267). When the extracted title exceeds 200 characters, the downstream
// UpdatePerson() call fails with "value too long for type character varying(200)".
//
// Database constraints:
//   - people.job_title VARCHAR(200)
//   - people.company VARCHAR(200)
//   - people.department VARCHAR(200)
//
// FIX: EnrichPersonMetadata() now truncates title/company/department fields to 200 chars
// before calling UpdatePerson().
func TestEnrichPersonMetadata_LongTitleTruncation(t *testing.T) {
	ctx := context.Background()
	logger := logging.MustGlobal()

	tests := []struct {
		name          string
		signatureText string
		expectedTitle string
		titleLength   int
		description   string
	}{
		{
			name: "title exceeds 200 chars - line-separated signature",
			signatureText: `--
Test Person
Senior Executive Vice President of Global Enterprise Cloud Infrastructure Solutions and Digital Transformation Services, North America and EMEA Regional Operations Division, Strategic Partnerships and Business Development Group
Example Corp`,
			expectedTitle: "Senior Executive Vice President of Global Enterprise Cloud Infrastructure Solutions and Digital Transformation Services, North America and EMEA Regional Operations Division, Strategic Partnerships and",
			titleLength:   200,
			description:   "Standard line-separated signature with very long title (truncated to 200)",
		},
		{
			name: "title exceeds 200 chars - pipe-separated signature",
			signatureText: "Jane Smith | Chief Executive Officer and Director of Global Strategic Initiatives for Enterprise Cloud Computing Solutions and Digital Transformation Technology Services, Worldwide Operations and Regional Business Development Division | Tech Corp",
			expectedTitle: "Chief Executive Officer and Director of Global Strategic Initiatives for Enterprise Cloud Computing Solutions and Digital Transformation Technology Services, Worldwide Operations and Regional Business",
			titleLength:   200,
			description:   "Pipe-separated signature with very long title (truncated to 200)",
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
						Title:        "", // Missing - needs enrichment from signature
						Company:      "",
						IsInternal:   false,
					}, nil
				},
			}

			activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{"example.com"})

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

			if !updateCalled {
				t.Fatal("Expected UpdatePerson to be called")
			}

			if updatedPerson == nil {
				t.Fatal("Expected updated person to be non-nil")
			}

			// Log what was actually extracted
			t.Logf("Extracted title: %q", updatedPerson.Title)
			t.Logf("Title length: %d", len(updatedPerson.Title))
			t.Logf("Description: %s", tt.description)

			// ADDITIONAL CHECK: Verify the title was actually extracted (proving parseSignatureForTitle works)
			if updatedPerson.Title == "" {
				t.Fatal("Expected title to be extracted from signature, got empty string - signature parsing may have failed")
			}

			// Verify title is truncated to 200 characters max
			if len(updatedPerson.Title) > 200 {
				t.Errorf("Title exceeds 200 character limit!\n"+
					"Title length: %d (expected max 200)\n"+
					"Title: %s\n"+
					"This will cause database constraint violation: value too long for type character varying(200)",
					len(updatedPerson.Title), updatedPerson.Title)
			}

			// Verify the title length matches expected
			if len(updatedPerson.Title) != tt.titleLength {
				t.Errorf("Title length mismatch.\nExpected: %d\nGot: %d",
					tt.titleLength, len(updatedPerson.Title))
			}

			// Verify the extracted title matches expected (truncated version)
			if updatedPerson.Title != tt.expectedTitle {
				t.Errorf("Extracted title does not match expected.\nExpected: %q\nGot: %q",
					tt.expectedTitle, updatedPerson.Title)
			}

			// ADDITIONAL CHECK: Verify output count
			if output.PeopleEnriched != 1 {
				t.Errorf("Expected 1 person enriched, got %d", output.PeopleEnriched)
			}
		})
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
			activity := NewPersonEnrichmentActivities(logger, personRepo, nil, []string{})

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
