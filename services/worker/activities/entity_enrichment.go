package activities

import (
	"context"
	"regexp"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// PersonRepository defines the interface for person data access in enrichment.
type PersonRepository interface {
	GetPersonByID(ctx context.Context, id int64) (*entities.Person, error)
	UpdatePerson(ctx context.Context, p *entities.Person) error
	GetPeopleByDomain(ctx context.Context, tenantID, domain string) ([]*entities.Person, error)
}

// PersonEnrichmentActivities holds dependencies for person metadata enrichment activities.
type PersonEnrichmentActivities struct {
	logger          logging.Logger
	personRepo      PersonRepository
	internalDomains []string
}

// NewPersonEnrichmentActivities creates a new PersonEnrichmentActivities instance.
func NewPersonEnrichmentActivities(
	logger logging.Logger,
	personRepo PersonRepository,
	internalDomains []string,
) *PersonEnrichmentActivities {
	if logger == nil {
		panic("NewPersonEnrichmentActivities: logger is required")
	}
	if personRepo == nil {
		panic("NewPersonEnrichmentActivities: personRepo is required")
	}
	return &PersonEnrichmentActivities{
		logger:          logger.With(logging.F("component", "person_enrichment_activities")),
		personRepo:      personRepo,
		internalDomains: internalDomains,
	}
}

// EnrichPersonMetadata enriches person metadata from email content (Stage 3.5).
func (a *PersonEnrichmentActivities) EnrichPersonMetadata(ctx context.Context, input workflows.EnrichPersonMetadataInput) (*workflows.EnrichPersonMetadataOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "EnrichPersonMetadata"),
		logging.F("tenant_id", input.TenantID),
		logging.F("people_count", len(input.ResolvedPeople)),
	)

	// Record heartbeat for long-running activity
	recordHeartbeat(ctx, "starting person metadata enrichment")

	// Handle empty input
	if len(input.ResolvedPeople) == 0 {
		return &workflows.EnrichPersonMetadataOutput{PeopleEnriched: 0}, nil
	}

	enrichedCount := 0

	// Process each resolved person
	for _, person := range input.ResolvedPeople {
		// Skip people without person IDs (unresolved)
		if person.PersonID == nil {
			continue
		}

		// Fetch current person record from DB
		dbPerson, err := a.personRepo.GetPersonByID(ctx, *person.PersonID)
		if err != nil {
			logger.Warn("Failed to fetch person for enrichment",
				logging.Err(err),
				logging.F("person_id", *person.PersonID))
			continue
		}
		if dbPerson == nil {
			logger.Warn("Person not found in database",
				logging.F("person_id", *person.PersonID))
			continue
		}

		// Track if we need to update this person
		needsUpdate := false
		updatedPerson := *dbPerson

		// Check if this person is eligible for metadata enrichment (missing title or company)
		metadataIncomplete := (updatedPerson.Title == "" || updatedPerson.Company == "")

		// Enrich title from signature if missing
		if updatedPerson.Title == "" {
			// Try per-sender signature first (for threaded emails)
			var signatureForThisPerson string
			if len(input.PerSenderSignatures) > 0 {
				signatureForThisPerson = input.PerSenderSignatures[person.Name]
			} else if input.SignatureText != "" {
				// Fall back to shared signature, but scope to sender only when sender email is known.
				// This prevents a single sender's title leaking to all mentioned people.
				if input.SenderEmail == "" || strings.EqualFold(dbPerson.PrimaryEmail, input.SenderEmail) {
					signatureForThisPerson = input.SignatureText
				}
			}

			if signatureForThisPerson != "" {
				title := parseSignatureForTitle(signatureForThisPerson, updatedPerson.CanonicalName)
				if title != "" {
					updatedPerson.Title = title
					needsUpdate = true
				}
			}
		}

		// Enrich company from email domain if missing
		if updatedPerson.Company == "" && updatedPerson.PrimaryEmail != "" {
			company := deriveCompanyFromDomain(updatedPerson.PrimaryEmail)
			if company != "" {
				updatedPerson.Company = company
				needsUpdate = true
			}
		}

		// Enrich is_internal flag from domain if metadata was incomplete AND we have
		// domain config to evaluate against. When internalDomains is empty (config not
		// loaded), skip to avoid clobbering existing is_internal values set by
		// bulk-enrich or manual review.
		if len(a.internalDomains) > 0 && metadataIncomplete {
			domain := entities.ExtractDomain(updatedPerson.PrimaryEmail)
			if domain != "" {
				isInternal := entities.IsInternalDomain(updatedPerson.PrimaryEmail, a.internalDomains)
				if isInternal != updatedPerson.IsInternal {
					updatedPerson.IsInternal = isInternal
					needsUpdate = true
				}
			}
		}

		// Update person if any field was enriched
		if needsUpdate {
			if err := a.personRepo.UpdatePerson(ctx, &updatedPerson); err != nil {
				logger.Warn("Failed to update enriched person",
					logging.Err(err),
					logging.F("person_id", *person.PersonID))
				continue
			}
			enrichedCount++
			logger.Debug("Person enriched",
				logging.F("person_id", *person.PersonID),
				logging.F("company", updatedPerson.Company),
				logging.F("title", updatedPerson.Title),
				logging.F("is_internal", updatedPerson.IsInternal))
		}
	}

	logger.Info("Person metadata enrichment complete",
		logging.F("enriched_count", enrichedCount))

	return &workflows.EnrichPersonMetadataOutput{
		PeopleEnriched: enrichedCount,
	}, nil
}

// deriveCompanyFromDomain derives a company name from an email domain.
// Examples:
//   - akamai.com → Akamai
//   - google.com → Google
//   - unknown123.com → "" (empty)
func deriveCompanyFromDomain(email string) string {
	domain := entities.ExtractDomain(email)
	if domain == "" {
		return ""
	}

	// Known company domain mappings
	knownDomains := map[string]string{
		"akamai.com":   "Akamai",
		"google.com":   "Google",
		"microsoft.com": "Microsoft",
		"amazon.com":   "Amazon",
		"facebook.com": "Facebook",
		"apple.com":    "Apple",
		"netflix.com":  "Netflix",
		"linkedin.com": "LinkedIn",
		"twitter.com":  "Twitter",
		"meta.com":     "Meta",
		"salesforce.com": "Salesforce",
		"oracle.com":   "Oracle",
		"ibm.com":      "IBM",
		"cisco.com":    "Cisco",
		"intel.com":    "Intel",
		"adobe.com":    "Adobe",
		"nvidia.com":   "Nvidia",
		"uber.com":     "Uber",
		"airbnb.com":   "Airbnb",
		"stripe.com":   "Stripe",
	}

	if company, ok := knownDomains[domain]; ok {
		return company
	}

	// For unknown domains, return empty string
	// We don't want to guess incorrectly
	return ""
}

// parseSignatureForTitle attempts to extract a job title from an email signature.
// It looks for patterns like:
//   - Name\nTitle\nCompany
//   - Name | Title
//
// Returns empty string if no clear title is found.
func parseSignatureForTitle(signatureText, personName string) string {
	if signatureText == "" {
		return ""
	}

	// Split by lines
	lines := strings.Split(signatureText, "\n")

	// Common job title keywords to help identify title lines
	titleKeywords := []string{
		"engineer", "manager", "director", "senior", "lead", "developer",
		"architect", "analyst", "consultant", "specialist", "coordinator",
		"administrator", "executive", "officer", "president", "vice",
		"product", "software", "principal", "staff", "technical",
	}

	// Try to find a line that looks like a title
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip lines that are just separators
		if isSeparatorLine(line) {
			continue
		}

		// Check if line contains a pipe separator (common in signatures: "Name | Title")
		// Do this BEFORE skipping lines with person name, so "Name | Title | Company" works
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" && !strings.Contains(strings.ToLower(part), strings.ToLower(personName)) {
					// Check if this part looks like a title
					if containsAny(strings.ToLower(part), titleKeywords) {
						return part
					}
				}
			}
		}

		// Skip the person's name line (for non-pipe-separated signatures)
		if strings.Contains(strings.ToLower(line), strings.ToLower(personName)) {
			continue
		}

		// Check if this line contains title keywords
		lineLower := strings.ToLower(line)
		if containsAny(lineLower, titleKeywords) {
			// Skip if line looks like a company name (too long, has "Inc", "LLC", etc.)
			if !looksLikeCompanyName(line) {
				return line
			}
		}

		// If we're past the name and see a short line (< 50 chars) that could be a title
		// and it's within the first few lines of the signature
		if i > 0 && i < 5 && len(line) < 50 && len(line) > 3 {
			// This might be a title line between name and company
			// Accept it if it's not obviously a company name
			if !looksLikeCompanyName(line) && containsAny(lineLower, titleKeywords) {
				return line
			}
		}
	}

	return ""
}

// isSeparatorLine checks if a line is just a separator (dashes, equals, etc.)
func isSeparatorLine(line string) bool {
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return false
	}
	// Check if line is all dashes, equals, underscores, or asterisks
	matched, _ := regexp.MatchString(`^[-=_*]+$`, line)
	return matched
}

// containsAny checks if a string contains any of the given substrings
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// looksLikeCompanyName checks if a string looks more like a company name than a job title
func looksLikeCompanyName(s string) bool {
	s = strings.ToLower(s)
	companyIndicators := []string{"inc", "llc", "ltd", "corp", "corporation", "technologies", "company", "co."}
	for _, indicator := range companyIndicators {
		if strings.Contains(s, indicator) {
			return true
		}
	}
	return false
}
