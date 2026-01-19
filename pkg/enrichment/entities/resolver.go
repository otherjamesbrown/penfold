package entities

import (
	"context"
	"fmt"

	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/processors"
	"github.com/rs/zerolog"
)

// Resolver handles entity resolution for the enrichment pipeline.
type Resolver struct {
	repo            *Repository
	internalDomains []string
	logger          zerolog.Logger
}

// ResolverOption configures the resolver.
type ResolverOption func(*Resolver)

// WithInternalDomains sets the internal email domains.
func WithInternalDomains(domains []string) ResolverOption {
	return func(r *Resolver) {
		r.internalDomains = domains
	}
}

// WithLogger sets the logger.
func WithResolverLogger(logger zerolog.Logger) ResolverOption {
	return func(r *Resolver) {
		r.logger = logger
	}
}

// NewResolver creates a new entity resolver.
func NewResolver(repo *Repository, opts ...ResolverOption) *Resolver {
	r := &Resolver{
		repo:            repo,
		internalDomains: []string{},
		logger:          zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(r)
	}
	r.logger = r.logger.With().Str("component", "entity_resolver").Logger()
	return r
}

// Name returns the processor name.
func (r *Resolver) Name() string {
	return "EntityResolver"
}

// Stage returns the pipeline stage.
func (r *Resolver) Stage() processors.Stage {
	return processors.StageCommonEnrichment
}

// CanProcess returns true - entity resolution runs for all content.
func (r *Resolver) CanProcess(classification *enrichment.Classification) bool {
	return true
}

// Process resolves all participants in the enrichment to person records.
func (r *Resolver) Process(ctx context.Context, pctx *processors.ProcessorContext) error {
	e := pctx.Enrichment

	resolved := make([]enrichment.ResolvedParticipant, 0, len(e.Participants))

	for _, p := range e.Participants {
		if p.Email == "" {
			continue
		}

		result, err := r.ResolveOrCreate(ctx, pctx.TenantID, p.Email, p.Name)
		if err != nil {
			r.logger.Warn().
				Err(err).
				Str("email", p.Email).
				Msg("Failed to resolve participant")
			// Continue with other participants
			resolved = append(resolved, enrichment.ResolvedParticipant{
				Participant: p,
			})
			continue
		}

		rp := enrichment.ResolvedParticipant{
			Participant: p,
			PersonID:    &result.Person.ID,
			Confidence:  result.Confidence,
			Source:      result.Source,
		}

		// Set internal flag from resolved person
		internal := result.Person.IsInternal
		rp.IsInternal = &internal
		rp.AccountType = string(result.Person.AccountType)

		resolved = append(resolved, rp)
	}

	e.ResolvedParticipants = resolved
	return nil
}

// ResolveOrCreate resolves an email/name to a person, creating if necessary.
func (r *Resolver) ResolveOrCreate(ctx context.Context, tenantID, email, displayName string) (*ResolutionResult, error) {
	// 1. Try exact email match
	person, err := r.repo.GetPersonByEmail(ctx, tenantID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup by email: %w", err)
	}
	if person != nil {
		// Update display name alias if we have a new variation
		if displayName != "" && displayName != person.CanonicalName {
			r.addDisplayNameAlias(ctx, person.ID, displayName)
		}
		return &ResolutionResult{
			Person:     person,
			Confidence: person.Confidence,
			Source:     "exact_match",
			IsNew:      false,
		}, nil
	}

	// 2. Try alias match
	person, err = r.repo.GetPersonByAlias(ctx, tenantID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup by alias: %w", err)
	}
	if person != nil {
		return &ResolutionResult{
			Person:     person,
			Confidence: person.Confidence,
			Source:     "alias",
			IsNew:      false,
		}, nil
	}

	// 3. Check for potential duplicates by name similarity
	var potentialDuplicates []int64
	if displayName != "" {
		candidates, err := r.repo.SearchPeopleByName(ctx, tenantID, displayName, 10)
		if err != nil {
			r.logger.Warn().Err(err).Msg("Failed to search for duplicates")
		} else {
			for _, c := range candidates {
				similarity := NameSimilarity(displayName, c.CanonicalName)
				if similarity > 0.8 {
					potentialDuplicates = append(potentialDuplicates, c.ID)
				}
			}
		}
	}

	// 4. Auto-create with appropriate confidence
	normalizedName := NormalizeDisplayName(displayName)
	if normalizedName == "" {
		// Use email local part as fallback name
		normalizedName = email
	}

	accountType := DetectAccountType(email, displayName)
	isInternal := IsInternalDomain(email, r.internalDomains)

	// Higher confidence for internal accounts, lower for external
	confidence := float32(0.6)
	if isInternal {
		confidence = 0.7
	}
	if accountType != AccountTypePerson {
		confidence = 0.8 // More confident about bot/role detection
	}

	person = &Person{
		TenantID:            tenantID,
		CanonicalName:       normalizedName,
		PrimaryEmail:        email,
		IsInternal:          isInternal,
		AccountType:         accountType,
		Confidence:          confidence,
		NeedsReview:         accountType == AccountTypePerson, // Bots don't need review
		AutoCreated:         true,
		PotentialDuplicates: potentialDuplicates,
	}

	if err := r.repo.CreatePerson(ctx, person); err != nil {
		return nil, fmt.Errorf("failed to create person: %w", err)
	}

	// 5. Add email as alias
	alias := &PersonAlias{
		PersonID:   person.ID,
		AliasType:  AliasTypeEmail,
		AliasValue: email,
		Confidence: 1.0,
		Source:     "auto_created",
	}
	if err := r.repo.CreateAlias(ctx, alias); err != nil {
		r.logger.Warn().Err(err).Msg("Failed to create email alias")
	}

	// 6. Add display name as alias if different from canonical
	if displayName != "" && displayName != normalizedName {
		r.addDisplayNameAlias(ctx, person.ID, displayName)
	}

	r.logger.Debug().
		Int64("person_id", person.ID).
		Str("email", email).
		Str("name", normalizedName).
		Str("account_type", string(accountType)).
		Bool("is_internal", isInternal).
		Int("potential_duplicates", len(potentialDuplicates)).
		Msg("Created new person")

	return &ResolutionResult{
		Person:     person,
		Confidence: confidence,
		Source:     "auto_created",
		IsNew:      true,
	}, nil
}

// Resolve resolves an email/name to a person without creating.
func (r *Resolver) Resolve(ctx context.Context, tenantID, email string) (*ResolutionResult, error) {
	// Try exact email match
	person, err := r.repo.GetPersonByEmail(ctx, tenantID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup by email: %w", err)
	}
	if person != nil {
		return &ResolutionResult{
			Person:     person,
			Confidence: person.Confidence,
			Source:     "exact_match",
			IsNew:      false,
		}, nil
	}

	// Try alias match
	person, err = r.repo.GetPersonByAlias(ctx, tenantID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup by alias: %w", err)
	}
	if person != nil {
		return &ResolutionResult{
			Person:     person,
			Confidence: person.Confidence,
			Source:     "alias",
			IsNew:      false,
		}, nil
	}

	// Not found
	return nil, nil
}

// addDisplayNameAlias adds a display name alias if it doesn't exist.
func (r *Resolver) addDisplayNameAlias(ctx context.Context, personID int64, displayName string) {
	alias := &PersonAlias{
		PersonID:   personID,
		AliasType:  AliasTypeDisplayName,
		AliasValue: displayName,
		Confidence: 0.8,
		Source:     "email_header",
	}
	if err := r.repo.CreateAlias(ctx, alias); err != nil {
		// Ignore duplicate errors
		r.logger.Trace().Err(err).Msg("Failed to add display name alias")
	}
}

// Verify interface compliance
var _ processors.Processor = (*Resolver)(nil)
var _ processors.CommonEnrichmentProcessor = (*Resolver)(nil)
