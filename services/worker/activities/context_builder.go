package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// EntityResolverInterface provides entity resolution for the context builder.
type EntityResolverInterface interface {
	Resolve(ctx context.Context, tenantID, email string) (*entities.ResolutionResult, error)
	ResolveOrCreate(ctx context.Context, tenantID, email, displayName string) (*entities.ResolutionResult, error)
}

// EntityLookupInterface provides entity lookups for the context builder.
type EntityLookupInterface interface {
	SearchPeopleByName(ctx context.Context, tenantID, name string, limit int) ([]*entities.Person, error)
	GetProjectByName(ctx context.Context, tenantID, name string) (*entities.Project, error)
	GetProjectsWithKeywords(ctx context.Context, tenantID string) ([]*entities.Project, error)
}

// ContextBuilderActivities holds dependencies for context building activities.
type ContextBuilderActivities struct {
	logger         logging.Logger
	entityResolver EntityResolverInterface
	entityRepo     EntityLookupInterface
	contextRepo    ContextPackageRepository
	pipelineRepo   PipelineRepository
}

// NewContextBuilderActivities creates a new ContextBuilderActivities instance.
func NewContextBuilderActivities(
	logger logging.Logger,
	entityResolver EntityResolverInterface,
	entityRepo EntityLookupInterface,
	contextRepo ContextPackageRepository,
	pipelineRepo PipelineRepository,
) *ContextBuilderActivities {
	if logger == nil {
		panic("NewContextBuilderActivities: logger is required")
	}
	if entityResolver == nil {
		panic("NewContextBuilderActivities: entityResolver is required")
	}
	if entityRepo == nil {
		panic("NewContextBuilderActivities: entityRepo is required")
	}
	if contextRepo == nil {
		panic("NewContextBuilderActivities: contextRepo is required")
	}
	// pipelineRepo is optional (provenance recording)
	return &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: entityResolver,
		entityRepo:     entityRepo,
		contextRepo:    contextRepo,
		pipelineRepo:   pipelineRepo,
	}
}

// BuildContextPackage builds a context package from extraction output.
// This is Stage 3: resolve entities and assemble context for Stage 4.
func (a *ContextBuilderActivities) BuildContextPackage(ctx context.Context, input workflows.BuildContextInput) (*workflows.BuildContextOutput, error) {
	// Set trace_id in context for log correlation
	if input.ContentID != "" {
		ctx = context.WithValue(ctx, logging.TraceIDKey, input.ContentID)
	}
	startTime := time.Now()
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "BuildContextPackage"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_type", input.ContentType),
	)

	// Record heartbeat
	recordHeartbeat(ctx, "starting context building")

	logger.Info("Building context package from extraction output")

	// Handle empty extraction
	if input.Extraction == nil {
		logger.Warn("Empty extraction input, returning empty context package")
		return &workflows.BuildContextOutput{
			ResolvedPeople:   []workflows.ResolvedPerson{},
			ResolvedProjects: []workflows.ResolvedProject{},
			UnresolvedTerms:  []string{},
			ContextPackage: &workflows.ContextPackage{
				ActiveRisks:     []workflows.ContextAssertion{},
				OpenActions:     []workflows.ContextAssertion{},
				RecentDecisions: []workflows.ContextAssertion{},
				ProductEvents:   []workflows.ContextProductEvent{},
				GlossaryTerms:   []workflows.ContextGlossaryTerm{},
			},
		}, nil
	}

	output := &workflows.BuildContextOutput{
		ResolvedPeople:   []workflows.ResolvedPerson{},
		ResolvedProjects: []workflows.ResolvedProject{},
		UnresolvedTerms:  []string{},
	}

	// Step 1: Resolve people
	recordHeartbeat(ctx, "resolving people entities")
	resolvedPeople, unresolvedPeople := a.resolvePeople(ctx, input.TenantID, input.Extraction.People, input.SenderEmail, input.SenderName)
	output.ResolvedPeople = resolvedPeople
	output.EntitiesResolved = len(resolvedPeople)
	output.EntitiesUnresolved = unresolvedPeople

	logger.Info("People resolution complete",
		logging.F("resolved", len(resolvedPeople)),
		logging.F("unresolved", unresolvedPeople))

	// Step 2: Resolve projects
	recordHeartbeat(ctx, "resolving project entities")
	resolvedProjects, unresolvedProjects := a.resolveProjects(ctx, input.TenantID, input.Extraction.Projects)
	output.ResolvedProjects = resolvedProjects
	output.UnresolvedTerms = unresolvedProjects
	output.EntitiesUnresolved += len(unresolvedProjects)

	logger.Info("Project resolution complete",
		logging.F("resolved", len(resolvedProjects)),
		logging.F("unresolved", len(unresolvedProjects)))

	// Step 3: Build context package
	recordHeartbeat(ctx, "building context package")
	contextPackage, err := a.buildContextPackage(ctx, input, resolvedPeople, resolvedProjects)
	if err != nil {
		logger.Error("Failed to build context package", logging.Err(err))
		return nil, fmt.Errorf("failed to build context package: %w", err)
	}

	output.ContextPackage = contextPackage
	output.TokensUsed = contextPackage.TotalTokensUsed
	output.TokenBudget = contextPackage.TokenBudget

	logger.Info("Context package built successfully",
		logging.F("tokens_used", contextPackage.TotalTokensUsed),
		logging.F("token_budget", contextPackage.TokenBudget),
		logging.F("active_risks", len(contextPackage.ActiveRisks)),
		logging.F("open_actions", len(contextPackage.OpenActions)),
		logging.F("recent_decisions", len(contextPackage.RecentDecisions)),
		logging.F("product_events", len(contextPackage.ProductEvents)),
		logging.F("glossary_terms", len(contextPackage.GlossaryTerms)))

	// Record pipeline run for provenance tracking (Stage 3: resolve)
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())
		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "resolve",
			ModelID:    "", // code-only stage
			Status:     "completed",
			DurationMS: durationMS,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run", logging.Err(runErr))
		}
	}

	return output, nil
}

// resolvePeople resolves people from extraction output.
func (a *ContextBuilderActivities) resolvePeople(ctx context.Context, tenantID string, people []workflows.PersonResult, senderEmail, senderName string) ([]workflows.ResolvedPerson, int) {
	var resolved []workflows.ResolvedPerson
	unresolvedCount := 0

	// If we have sender email, resolve it first
	if senderEmail != "" && a.entityResolver != nil {
		result, err := a.entityResolver.Resolve(ctx, tenantID, senderEmail)
		if err == nil && result != nil {
			resolved = append(resolved, workflows.ResolvedPerson{
				Name:       result.Person.CanonicalName,
				PersonID:   &result.Person.ID,
				Confidence: result.Confidence,
				Source:     result.Source,
				Title:      result.Person.Title,
				Department: result.Person.Department,
				IsInternal: result.Person.IsInternal,
			})
		}
	}

	// Resolve extracted people
	for _, person := range people {
		rp := a.resolvePerson(ctx, tenantID, person)
		if rp.PersonID != nil {
			resolved = append(resolved, rp)
		} else {
			unresolvedCount++
		}
	}

	return resolved, unresolvedCount
}

// resolvePerson resolves a single person from extraction.
func (a *ContextBuilderActivities) resolvePerson(ctx context.Context, tenantID string, person workflows.PersonResult) workflows.ResolvedPerson {
	rp := workflows.ResolvedPerson{
		Name:       person.Name,
		Role:       person.Role,
		PersonID:   nil,
		Confidence: 0,
		Source:     "unresolved",
		IsInternal: false,
	}

	if a.entityRepo == nil {
		return rp
	}

	// Try fuzzy name search
	candidates, err := a.entityRepo.SearchPeopleByName(ctx, tenantID, person.Name, 10)
	if err != nil || len(candidates) == 0 {
		return rp
	}

	// Find best match using NameSimilarity
	var bestMatch *entities.Person
	var bestSimilarity float64

	for _, candidate := range candidates {
		similarity := entities.NameSimilarity(person.Name, candidate.CanonicalName)
		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestMatch = candidate
		}
	}

	// Only use matches with similarity > 0.7
	if bestSimilarity > 0.7 && bestMatch != nil {
		rp.PersonID = &bestMatch.ID
		rp.Confidence = float32(bestSimilarity)
		rp.Source = "fuzzy"
		rp.Title = bestMatch.Title
		rp.Department = bestMatch.Department
		rp.IsInternal = bestMatch.IsInternal
	}

	return rp
}

// resolveProjects resolves projects from extraction output.
func (a *ContextBuilderActivities) resolveProjects(ctx context.Context, tenantID string, projects []string) ([]workflows.ResolvedProject, []string) {
	var resolved []workflows.ResolvedProject
	var unresolved []string

	if a.entityRepo == nil || a.contextRepo == nil {
		return resolved, projects
	}

	for _, projectName := range projects {
		rp := a.resolveProject(ctx, tenantID, projectName)
		if rp.ProjectID != nil {
			resolved = append(resolved, rp)
		} else {
			unresolved = append(unresolved, projectName)
		}
	}

	return resolved, unresolved
}

// resolveProject resolves a single project from extraction.
func (a *ContextBuilderActivities) resolveProject(ctx context.Context, tenantID string, projectName string) workflows.ResolvedProject {
	rp := workflows.ResolvedProject{
		Name:      projectName,
		ProjectID: nil,
		Source:    "unresolved",
	}

	// Try exact name match
	project, err := a.entityRepo.GetProjectByName(ctx, tenantID, projectName)
	if err == nil && project != nil {
		rp.ProjectID = &project.ID
		rp.Source = "exact_match"
		return rp
	}

	// Try keyword match
	projectID, err := a.contextRepo.ResolveProjectByKeyword(ctx, tenantID, projectName)
	if err == nil && projectID != nil {
		rp.ProjectID = projectID
		rp.Source = "keyword"
		return rp
	}

	// Try glossary lookup
	glossaryTerms, err := a.contextRepo.GetGlossaryTerms(ctx, []string{projectName}, []int64{}, 1)
	if err == nil && len(glossaryTerms) > 0 {
		rp.Expansion = glossaryTerms[0].Expansion
		rp.Source = "glossary"
		// Note: glossary term might not have a project_id, so ProjectID stays nil
	}

	return rp
}

// buildContextPackage assembles the context package for Stage 4.
func (a *ContextBuilderActivities) buildContextPackage(
	ctx context.Context,
	input workflows.BuildContextInput,
	resolvedPeople []workflows.ResolvedPerson,
	resolvedProjects []workflows.ResolvedProject,
) (*workflows.ContextPackage, error) {
	// Determine token budget based on content type
	tokenBudget := a.getTokenBudget(input.ContentType)

	cp := &workflows.ContextPackage{
		TokenBudget:        tokenBudget,
		TotalTokensUsed:    0,
		ActiveRisks:        []workflows.ContextAssertion{},
		OpenActions:        []workflows.ContextAssertion{},
		RecentDecisions:    []workflows.ContextAssertion{},
		ProductEvents:      []workflows.ContextProductEvent{},
		GlossaryTerms:      []workflows.ContextGlossaryTerm{},
		ParticipantContext: resolvedPeople,
	}

	if a.contextRepo == nil {
		return cp, nil
	}

	// Collect resolved project IDs
	projectIDs := make([]int64, 0, len(resolvedProjects))
	for _, rp := range resolvedProjects {
		if rp.ProjectID != nil {
			projectIDs = append(projectIDs, *rp.ProjectID)
		}
	}

	// Track tokens as we add sections
	tokensUsed := 0

	// 1. Participant context (always include, ~15 tokens per participant)
	participantTokens := len(resolvedPeople) * 15
	tokensUsed += participantTokens

	// 2. Active risks (~25 tokens each)
	if len(projectIDs) > 0 && tokensUsed+250 <= tokenBudget {
		risks, err := a.contextRepo.GetActiveRisks(ctx, projectIDs, 10)
		if err == nil {
			riskTokens := len(risks) * 25
			if tokensUsed+riskTokens <= tokenBudget {
				cp.ActiveRisks = toWorkflowAssertions(risks)
				tokensUsed += riskTokens
			}
		}
	}

	// 3. Open actions (~25 tokens each)
	if len(projectIDs) > 0 && tokensUsed+250 <= tokenBudget {
		actions, err := a.contextRepo.GetOpenActions(ctx, projectIDs, 10)
		if err == nil {
			actionTokens := len(actions) * 25
			if tokensUsed+actionTokens <= tokenBudget {
				cp.OpenActions = toWorkflowAssertions(actions)
				tokensUsed += actionTokens
			}
		}
	}

	// 4. Recent decisions (~25 tokens each)
	if len(projectIDs) > 0 && tokensUsed+125 <= tokenBudget {
		decisions, err := a.contextRepo.GetRecentDecisions(ctx, projectIDs, 60, 5)
		if err == nil {
			decisionTokens := len(decisions) * 25
			if tokensUsed+decisionTokens <= tokenBudget {
				cp.RecentDecisions = toWorkflowAssertions(decisions)
				tokensUsed += decisionTokens
			}
		}
	}

	// 5. Product events (~30 tokens each)
	if len(projectIDs) > 0 && tokensUsed+300 <= tokenBudget {
		events, err := a.contextRepo.GetProductEvents(ctx, projectIDs, 90, 10)
		if err == nil {
			eventTokens := len(events) * 30
			if tokensUsed+eventTokens <= tokenBudget {
				cp.ProductEvents = toWorkflowEvents(events)
				tokensUsed += eventTokens
			}
		}
	}

	// 6. Glossary terms (~20 tokens each)
	if input.Extraction != nil && tokensUsed+400 <= tokenBudget {
		// Collect terms from extraction
		terms := a.collectGlossaryTerms(input.Extraction)
		if len(terms) > 0 {
			glossary, err := a.contextRepo.GetGlossaryTerms(ctx, terms, projectIDs, 20)
			if err == nil {
				glossaryTokens := len(glossary) * 20
				if tokensUsed+glossaryTokens <= tokenBudget {
					cp.GlossaryTerms = toWorkflowGlossary(glossary)
					tokensUsed += glossaryTokens
				}
			}
		}
	}

	// Apply token budget enforcement: if over budget, truncate sections
	if tokensUsed > tokenBudget {
		tokensUsed = a.applyTokenBudget(cp, tokenBudget)
	}

	cp.TotalTokensUsed = tokensUsed
	return cp, nil
}

// getTokenBudget returns the token budget based on content type.
func (a *ContextBuilderActivities) getTokenBudget(contentType string) int {
	switch contentType {
	case "meeting":
		return 3000
	case "email":
		return 2000
	case "slack":
		return 1000
	default:
		return 2000
	}
}

// collectGlossaryTerms extracts potential glossary terms from extraction output.
func (a *ContextBuilderActivities) collectGlossaryTerms(extraction *workflows.SLMPipelineExtractEntitiesOutput) []string {
	terms := make(map[string]bool)

	// Add projects as potential terms
	for _, proj := range extraction.Projects {
		if isAcronym(proj) {
			terms[proj] = true
		}
	}

	// Add organisations as potential terms
	for _, org := range extraction.Organisations {
		if isAcronym(org) {
			terms[org] = true
		}
	}

	// Convert to slice
	result := make([]string, 0, len(terms))
	for term := range terms {
		result = append(result, term)
	}

	return result
}

// isAcronym checks if a term looks like an acronym (all caps, 2-6 chars).
func isAcronym(term string) bool {
	if len(term) < 2 || len(term) > 6 {
		return false
	}
	return strings.ToUpper(term) == term
}

// toWorkflowAssertions converts repository ContextAssertions to workflow ContextAssertions.
func toWorkflowAssertions(assertions []ContextAssertion) []workflows.ContextAssertion {
	result := make([]workflows.ContextAssertion, len(assertions))
	for i, a := range assertions {
		result[i] = workflows.ContextAssertion{
			Subject:    a.OwnerName,
			Predicate:  a.Description,
			Object:     a.ProjectName,
			SourceText: a.SourceQuote,
		}
	}
	return result
}

// toWorkflowEvents converts repository ContextProductEvents to workflow ContextProductEvents.
func toWorkflowEvents(events []ContextProductEvent) []workflows.ContextProductEvent {
	result := make([]workflows.ContextProductEvent, len(events))
	for i, e := range events {
		result[i] = workflows.ContextProductEvent{
			EventType:   e.EventType,
			Description: e.Description,
			Timestamp:   e.OccurredAt.Format(time.RFC3339),
		}
	}
	return result
}

// toWorkflowGlossary converts repository ContextGlossaryTerms to workflow ContextGlossaryTerms.
func toWorkflowGlossary(terms []ContextGlossaryTerm) []workflows.ContextGlossaryTerm {
	result := make([]workflows.ContextGlossaryTerm, len(terms))
	for i, t := range terms {
		result[i] = workflows.ContextGlossaryTerm{
			Term:       t.Term,
			Definition: t.Definition,
		}
	}
	return result
}

// applyTokenBudget truncates sections from tail to fit within budget.
// Order of truncation (least valuable first):
// 1. Glossary terms (drop from tail)
// 2. Product events (drop from tail)
// 3. Decisions (drop from tail)
// 4. Actions (drop from tail)
// 5. Risks (drop from tail)
// 6. Participants (never drop - always included)
func (a *ContextBuilderActivities) applyTokenBudget(cp *workflows.ContextPackage, budget int) int {
	tokensUsed := len(cp.ParticipantContext) * 15

	// Try to fit everything, dropping from tail of each section if needed
	sections := []struct {
		name        string
		items       int
		tokensEach  int
		applyTrunc  func(int)
	}{
		{"risks", len(cp.ActiveRisks), 25, func(keep int) { cp.ActiveRisks = cp.ActiveRisks[:keep] }},
		{"actions", len(cp.OpenActions), 25, func(keep int) { cp.OpenActions = cp.OpenActions[:keep] }},
		{"decisions", len(cp.RecentDecisions), 25, func(keep int) { cp.RecentDecisions = cp.RecentDecisions[:keep] }},
		{"events", len(cp.ProductEvents), 30, func(keep int) { cp.ProductEvents = cp.ProductEvents[:keep] }},
		{"glossary", len(cp.GlossaryTerms), 20, func(keep int) { cp.GlossaryTerms = cp.GlossaryTerms[:keep] }},
	}

	for i := len(sections) - 1; i >= 0; i-- {
		section := sections[i]
		sectionTokens := section.items * section.tokensEach

		if tokensUsed+sectionTokens <= budget {
			tokensUsed += sectionTokens
		} else {
			// Fit as many items as possible
			available := budget - tokensUsed
			keep := available / section.tokensEach
			if keep > 0 {
				section.applyTrunc(keep)
				tokensUsed += keep * section.tokensEach
			} else {
				section.applyTrunc(0)
			}
		}
	}

	return tokensUsed
}
