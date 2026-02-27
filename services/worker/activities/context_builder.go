package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	enrichmentconfig "github.com/otherjamesbrown/penfold/pkg/enrichment/config"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
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
	IncrementSentCount(ctx context.Context, personID int64) error
	IncrementReceivedCount(ctx context.Context, personID int64) error
	UpdatePersonTitle(ctx context.Context, personID int64, title string) error
}

// TopicLookupInterface provides topic resolution for the context builder.
type TopicLookupInterface interface {
	GetByName(ctx context.Context, tenantID, name string) (TopicResult, error)
	ResolveByKeyword(ctx context.Context, tenantID, keyword string) (*int64, error)
	GetByID(ctx context.Context, id int64) (TopicResult, error)
	ListForContext(ctx context.Context, tenantID string, names []string) ([]TopicResult, error)
	ScanContentForTopics(ctx context.Context, tenantID string, content string) ([]TopicResult, error)
}

// TopicResult is a simplified topic for context building (avoids importing pkg/topics).
type TopicResult struct {
	ID          int64
	Name        string
	Description string
}

// ContextBuilderActivities holds dependencies for context building activities.
type ContextBuilderActivities struct {
	logger         logging.Logger
	entityResolver EntityResolverInterface
	entityRepo     EntityLookupInterface
	contextRepo    ContextPackageRepository
	topicRepo      TopicLookupInterface
	pipelineRepo   PipelineRepository
	configResolver *enrichmentconfig.ConfigResolver
}

// NewContextBuilderActivities creates a new ContextBuilderActivities instance.
func NewContextBuilderActivities(
	logger logging.Logger,
	entityResolver EntityResolverInterface,
	entityRepo EntityLookupInterface,
	contextRepo ContextPackageRepository,
	topicRepo TopicLookupInterface,
	pipelineRepo PipelineRepository,
	configResolver *enrichmentconfig.ConfigResolver,
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
	// topicRepo is optional (topic resolution)
	// pipelineRepo is optional (provenance recording)
	// configResolver is optional (pattern detection)
	return &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: entityResolver,
		entityRepo:     entityRepo,
		contextRepo:    contextRepo,
		topicRepo:      topicRepo,
		pipelineRepo:   pipelineRepo,
		configResolver: configResolver,
	}
}

// BuildContextPackage builds a context package from extraction output.
// This is Stage 3: resolve entities and assemble context for Stage 4.
//
// Deprecated: use BuildContext with stage="deep_analyze" for context assembly.
// Kept as a shim for running Temporal workflows during deploy (pf-7aa11e).
// Entity resolution (people, projects, org reclassification) stays here.
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
				ActiveRisks:       []workflows.ContextAssertion{},
				OpenActions:       []workflows.ContextAssertion{},
				RecentDecisions:   []workflows.ContextAssertion{},
				ProductEvents:     []workflows.ContextProductEvent{},
				GlossaryTerms:     []workflows.ContextGlossaryTerm{},
				TopicDescriptions: []workflows.ContextTopicDescription{},
			},
		}, nil
	}

	output := &workflows.BuildContextOutput{
		ResolvedPeople:   []workflows.ResolvedPerson{},
		ResolvedProjects: []workflows.ResolvedProject{},
		UnresolvedTerms:  []string{},
	}

	// Step 0: Reclassify organisations that are actually projects.
	// NER sometimes puts internal projects (e.g. CLIC, MTC) in the organisations list.
	// Check each org against project resolution; move matches to the projects list.
	a.reclassifyOrganisations(ctx, input.TenantID, input.Extraction)

	// Step 0.5: Enrich first-name-only NER people with full names from headers.
	// e.g., NER says "Tim" but To header has "Tim Dunn" → upgrade to "Tim Dunn".
	// Done here so CorrectedExtraction carries enriched names to Stage 4.
	input.Extraction.People = enrichPeopleFromHeaders(input.Extraction.People, input.SenderEmail, input.SenderName, input.ParticipantEmails)

	// Carry the corrected extraction back to the workflow. Temporal serialises
	// activity I/O, so in-place mutations to input don't propagate; Stage 4
	// needs the reclassified org/project lists and enriched people names.
	output.CorrectedExtraction = input.Extraction

	// Step 1: Resolve people
	recordHeartbeat(ctx, "resolving people entities")
	resolvedPeople, unresolvedPeople := a.resolvePeople(ctx, input.TenantID, input.Extraction.People, input.SenderEmail, input.SenderName, input.ParticipantEmails)

	// Dedup resolved people by person ID, preserving IsPrimaryUser flag (pf-26c835).
	// Without this, the same person can appear twice — once from headers (with [Primary user])
	// and once from NER resolution (without the tag).
	{
		seen := make(map[int64]int) // person_id → index in deduped
		var deduped []workflows.ResolvedPerson
		for _, p := range resolvedPeople {
			if p.PersonID == nil {
				deduped = append(deduped, p)
				continue
			}
			if idx, ok := seen[*p.PersonID]; ok {
				// If this duplicate has PrimaryUser flag, update the kept entry
				if p.IsPrimaryUser {
					deduped[idx].IsPrimaryUser = true
				}
				continue
			}
			seen[*p.PersonID] = len(deduped)
			deduped = append(deduped, p)
		}
		resolvedPeople = deduped
	}

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

	// Step 3: Build context package via unified BuildContext (pf-7aa11e).
	recordHeartbeat(ctx, "building context package")
	contextPackage, err := a.BuildContext(ctx, "deep_analyze", input, resolvedPeople, resolvedProjects)
	if err != nil {
		pe := perrors.ClassifyError(err, "resolve")
		logger.Error("Failed to build context package", logging.Err(pe))
		return nil, WrapForTemporal(pe)
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
		logging.F("glossary_terms", len(contextPackage.GlossaryTerms)),
		logging.F("topic_descriptions", len(contextPackage.TopicDescriptions)))

	// Record pipeline run for provenance tracking (Stage 3: resolve)
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())

		// Capture IO data for code-only stage
		inputJSON, _ := json.Marshal(map[string]interface{}{
			"source_id":      input.SourceID,
			"content_type":   input.ContentType,
			"has_extraction": input.Extraction != nil,
			"people_count":   len(input.Extraction.People),
			"projects_count": len(input.Extraction.Projects),
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"resolved_people":   len(resolvedPeople),
			"resolved_projects": len(resolvedProjects),
			"tokens_used":       contextPackage.TotalTokensUsed,
			"token_budget":      contextPackage.TokenBudget,
		})
		parsedJSON, _ := json.Marshal(output)

		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "resolve",
			ModelID:    "", // code-only stage
			Status:     "completed",
			DurationMS: durationMS,
			InputData:  inputJSON,
			OutputData: outputJSON,
			ParsedData: parsedJSON,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run", logging.Err(runErr))
		}
	}

	return output, nil
}

// getTenantPatterns loads tenant-specific patterns for account type detection.
// Returns nil if config resolver is not available or if loading fails.
func (a *ContextBuilderActivities) getTenantPatterns(ctx context.Context, tenantID string) *entities.AccountTypePatterns {
	if a.configResolver == nil {
		return nil
	}

	config, err := a.configResolver.GetConfig(ctx, tenantID)
	if err != nil {
		a.logger.WithContext(ctx).Warn("Failed to load tenant config for pattern detection",
			logging.Err(err),
			logging.F("tenant_id", tenantID))
		return nil
	}

	// Convert tenant config patterns to AccountTypePatterns
	return &entities.AccountTypePatterns{
		BotPatterns:          config.BotPatterns,
		DistributionPatterns: config.DistributionPatterns,
		RolePatterns:         config.RoleAccountPatterns,
		ExternalDomains:      config.ExternalServiceDomains,
	}
}

// getPrimaryUserEmail loads the primary user email from tenant config.
// Returns empty string if not configured or if loading fails.
func (a *ContextBuilderActivities) getPrimaryUserEmail(ctx context.Context, tenantID string) string {
	if a.configResolver == nil {
		return ""
	}
	config, err := a.configResolver.GetConfig(ctx, tenantID)
	if err != nil {
		return ""
	}
	return config.PrimaryUserEmail
}

// headerRoleLabel converts a raw header role ("to", "cc") to a display label.
func headerRoleLabel(role string) string {
	switch strings.ToLower(role) {
	case "to":
		return "To"
	case "cc":
		return "CC"
	default:
		return ""
	}
}

// resolvePeople resolves people from extraction output and structured participant emails.
func (a *ContextBuilderActivities) resolvePeople(ctx context.Context, tenantID string, people []workflows.PersonResult, senderEmail, senderName string, participantEmails []workflows.Participant) ([]workflows.ResolvedPerson, int) {
	var resolved []workflows.ResolvedPerson
	unresolvedCount := 0
	seenEmails := make(map[string]bool)
	seenPersonIDs := make(map[int64]bool) // pf-0f08e0: dedup NER-resolved against header-resolved
	logger := a.logger.WithContext(ctx)

	// Track sender person ID for message count increment
	var senderPersonID *int64

	// Load tenant-specific patterns for account type detection
	tenantPatterns := a.getTenantPatterns(ctx, tenantID)

	// Load primary user email from tenant config
	primaryUserEmail := a.getPrimaryUserEmail(ctx, tenantID)

	// If we have sender email, check if it's a person account before resolving
	if senderEmail != "" && a.entityResolver != nil {
		accountType := entities.DetectAccountTypeWithPatterns(senderEmail, senderName, tenantPatterns)
		if accountType != entities.AccountTypePerson {
			logger.Debug("Skipping non-person sender",
				logging.F("email", senderEmail),
				logging.F("account_type", accountType))
		} else {
			result, err := a.entityResolver.ResolveOrCreate(ctx, tenantID, senderEmail, senderName)
			if err == nil && result != nil {
				resolved = append(resolved, workflows.ResolvedPerson{
					Name:          result.Person.CanonicalName,
					PersonID:      &result.Person.ID,
					Confidence:    result.Confidence,
					Source:        result.Source,
					Role:          "Sender",
					Title:         result.Person.Title,
					Department:    result.Person.Department,
					IsInternal:    result.Person.IsInternal,
					IsPrimaryUser: strings.EqualFold(senderEmail, primaryUserEmail),
				})
				seenEmails[senderEmail] = true
				senderPersonID = &result.Person.ID
				seenPersonIDs[result.Person.ID] = true

				// Increment sent_count for the sender
				if a.entityRepo != nil {
					if err := a.entityRepo.IncrementSentCount(ctx, result.Person.ID); err != nil {
						logger.Warn("Failed to increment sent_count for sender",
							logging.Err(err),
							logging.F("person_id", result.Person.ID))
					}
				}
			}
		}
	}

	// Process participant_emails array (From/To/Cc from email headers with display names)
	// Filter out non-person accounts (distribution lists, bots, role accounts, external services)
	if a.entityResolver != nil {
		for _, participant := range participantEmails {
			if participant.Email == "" || seenEmails[participant.Email] {
				continue
			}

			// Check if this is a person account
			accountType := entities.DetectAccountTypeWithPatterns(participant.Email, participant.DisplayName, tenantPatterns)
			if accountType != entities.AccountTypePerson {
				logger.Debug("Skipping non-person participant",
					logging.F("email", participant.Email),
					logging.F("account_type", accountType))
				continue
			}

			// Pass display name to ResolveOrCreate - this is the bug fix
			result, err := a.entityResolver.ResolveOrCreate(ctx, tenantID, participant.Email, participant.DisplayName)
			if err == nil && result != nil {
				resolved = append(resolved, workflows.ResolvedPerson{
					Name:          result.Person.CanonicalName,
					PersonID:      &result.Person.ID,
					Confidence:    result.Confidence,
					Source:        result.Source,
					Role:          headerRoleLabel(participant.HeaderRole),
					Title:         result.Person.Title,
					Department:    result.Person.Department,
					IsInternal:    result.Person.IsInternal,
					IsPrimaryUser: strings.EqualFold(participant.Email, primaryUserEmail),
				})
				seenEmails[participant.Email] = true
				seenPersonIDs[result.Person.ID] = true

				// Increment received_count for recipients (but not the sender)
				// If sender is also in the recipient list (e.g., CC'd on their own email), skip them
				if senderPersonID == nil || result.Person.ID != *senderPersonID {
					if a.entityRepo != nil {
						if err := a.entityRepo.IncrementReceivedCount(ctx, result.Person.ID); err != nil {
							logger.Warn("Failed to increment received_count for recipient",
								logging.Err(err),
								logging.F("person_id", result.Person.ID))
						}
					}
				}
			}
		}
	}

	// Resolve extracted people (from LLM - names only, fuzzy match).
	// Note: people names are already enriched by BuildContextPackage (Step 0.5).
	// pf-0f08e0: Skip NER people that already resolved from email headers (dedup by person_id).
	for _, person := range people {
		rp := a.resolvePerson(ctx, tenantID, person)
		if rp.PersonID != nil {
			if seenPersonIDs[*rp.PersonID] {
				continue // Already in resolved list from header resolution
			}
			seenPersonIDs[*rp.PersonID] = true
			resolved = append(resolved, rp)
		} else {
			unresolvedCount++
		}
	}

	return resolved, unresolvedCount
}

// enrichPeopleFromHeaders cross-references NER-extracted people (often first-name only)
// with email header participants (who have full names). When a single-word NER name
// matches the first name of a header participant, the NER entry is upgraded to the full name.
// This improves both provenance accuracy and downstream fuzzy matching.
func enrichPeopleFromHeaders(people []workflows.PersonResult, senderEmail, senderName string, participants []workflows.Participant) []workflows.PersonResult {
	if len(people) == 0 {
		return people
	}

	// Build first-name → full-name lookup from all header participants.
	// Key: lowercase first name, Value: full display name.
	// If multiple participants share a first name, skip enrichment for that name (ambiguous).
	firstNameToFull := make(map[string]string)
	firstNameConflict := make(map[string]bool)

	addName := func(displayName string) {
		if displayName == "" {
			return
		}

		var firstName, normalizedFullName string

		// Handle "Last, First" format common in email headers (e.g. "Dunn, Tim")
		if commaIdx := strings.IndexByte(displayName, ','); commaIdx >= 0 {
			last := strings.TrimSpace(displayName[:commaIdx])
			firstPart := strings.TrimSpace(displayName[commaIdx+1:])
			if last == "" || firstPart == "" {
				return
			}
			firstWords := strings.Fields(firstPart)
			firstName = strings.ToLower(firstWords[0])
			normalizedFullName = firstPart + " " + last // "Tim Dunn" from "Dunn, Tim"
		} else {
			parts := strings.Fields(displayName)
			if len(parts) < 2 {
				return // single-word display name, nothing to enrich with
			}
			firstName = strings.ToLower(parts[0])
			normalizedFullName = displayName
		}

		if existing, ok := firstNameToFull[firstName]; ok {
			if !strings.EqualFold(existing, normalizedFullName) {
				firstNameConflict[firstName] = true // ambiguous — two different people share first name
			}
		} else {
			firstNameToFull[firstName] = normalizedFullName
		}
	}

	// Add sender
	addName(senderName)

	// Add all To/CC participants
	for _, p := range participants {
		addName(p.DisplayName)
	}

	// Enrich NER-extracted people
	enriched := make([]workflows.PersonResult, len(people))
	for i, person := range people {
		enriched[i] = person

		// Only enrich single-word names (first-name only)
		nameParts := strings.Fields(person.Name)
		if len(nameParts) != 1 {
			continue
		}

		firstName := strings.ToLower(nameParts[0])
		if firstNameConflict[firstName] {
			continue // ambiguous, skip
		}
		if fullName, ok := firstNameToFull[firstName]; ok {
			enriched[i].Name = fullName
		}
	}

	// pf-0f08e0: Dedup enriched list by normalized name.
	// After enrichment, "Miroslav" → "Miroslav Ponec" may duplicate an existing
	// "Miroslav Ponec" entry. Keep the first occurrence (preserves role if any).
	seen := make(map[string]bool)
	deduped := make([]workflows.PersonResult, 0, len(enriched))
	for _, p := range enriched {
		key := strings.ToLower(strings.TrimSpace(p.Name))
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, p)
	}

	return deduped
}

// isGarbageTitle filters out meeting invitation text, email header fragments,
// and other non-job-title strings.
// Returns true if the title should be rejected.
func isGarbageTitle(title string) bool {
	if title == "" {
		return true
	}

	lowerTitle := strings.ToLower(title)

	// Meeting invitation patterns
	garbagePatterns := []string{
		"tap to",
		"join my meeting",
		"join webex",
		"join zoom",
		"join teams",
		"attendees only",
		"dial in",
		"click here",
		"conference",
		"meeting id",
		"passcode",
		"http://",
		"https://",
		"www.",
	}

	for _, pattern := range garbagePatterns {
		if strings.Contains(lowerTitle, pattern) {
			return true
		}
	}

	// Email header prefixes (raw header fragments leaking into titles)
	headerPrefixes := []string{"cc:", "to:", "bcc:", "from:", "reply-to:", "sent:"}
	for _, prefix := range headerPrefixes {
		if strings.HasPrefix(lowerTitle, prefix) {
			return true
		}
	}

	// Contains email addresses or angle brackets (header fragments)
	if strings.Contains(title, "@") || strings.Contains(title, "<") || strings.Contains(title, ">") {
		return true
	}

	// Reject if it looks like a phone number (contains multiple digits and dashes/spaces)
	digitCount := 0
	for _, ch := range title {
		if ch >= '0' && ch <= '9' {
			digitCount++
		}
	}
	if digitCount > 5 {
		return true
	}

	// Reject single generic words that aren't real job titles
	trimmed := strings.TrimSpace(title)
	if !strings.Contains(trimmed, " ") && len(trimmed) < 12 {
		genericWords := []string{
			"overall", "general", "other", "various", "multiple",
			"none", "n/a", "na", "unknown", "tbd", "tba",
		}
		for _, word := range genericWords {
			if strings.EqualFold(trimmed, word) {
				return true
			}
		}
	}

	return false
}

// resolvePerson resolves a single person from extraction via fuzzy name matching.
// NOTE: For name-only extractions (no email), we use fuzzy matching to find existing people.
// Person creation from structured data (email headers) happens via participant_emails processing.
func (a *ContextBuilderActivities) resolvePerson(ctx context.Context, tenantID string, person workflows.PersonResult) workflows.ResolvedPerson {
	logger := a.logger.WithContext(ctx)
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
		rp.Name = bestMatch.CanonicalName // pf-0f08e0: use DB canonical name, not raw NER name
		rp.Confidence = float32(bestSimilarity)
		rp.Source = "fuzzy"
		rp.Title = bestMatch.Title
		rp.Department = bestMatch.Department
		rp.IsInternal = bestMatch.IsInternal

		// Persist extracted Role to job_title if:
		// 1. Person has no current job_title (Title is empty/NULL)
		// 2. PersonResult has a non-empty Role
		// 3. The Role is not garbage (meeting invitation text, etc.)
		if bestMatch.Title == "" && person.Role != "" && !isGarbageTitle(person.Role) {
			if err := a.entityRepo.UpdatePersonTitle(ctx, bestMatch.ID, person.Role); err != nil {
				logger.Warn("Failed to update person title from extracted role",
					logging.Err(err),
					logging.F("person_id", bestMatch.ID),
					logging.F("extracted_role", person.Role))
			} else {
				// Update the returned ResolvedPerson to reflect the new title
				rp.Title = person.Role
				logger.Debug("Updated person title from extracted role",
					logging.F("person_id", bestMatch.ID),
					logging.F("title", person.Role))
			}
		}
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

	// Try name containment match (e.g., "Oslo NLB Workflow" contains project "Oslo")
	projectID, err = a.contextRepo.ResolveProjectByNameContains(ctx, tenantID, projectName)
	if err == nil && projectID != nil {
		rp.ProjectID = projectID
		rp.Source = "name_contains"
		return rp
	}

	// Try glossary lookup
	glossaryTerms, err := a.contextRepo.GetGlossaryTerms(ctx, tenantID, []string{projectName}, []int64{}, 1)
	if err == nil && len(glossaryTerms) > 0 {
		rp.Expansion = glossaryTerms[0].Expansion
		rp.Source = "glossary"
		// Note: glossary term might not have a project_id, so ProjectID stays nil
	}

	return rp
}

// reclassifyOrganisations checks each extracted organisation against project resolution.
// If an org resolves as a known project (via exact name match, keyword match, or glossary),
// it's moved from the organisations list to the projects list. This fixes NER misclassification
// of internal projects like CLIC, MTC that look like organisation acronyms.
func (a *ContextBuilderActivities) reclassifyOrganisations(ctx context.Context, tenantID string, extraction *workflows.SLMPipelineExtractEntitiesOutput) {
	if extraction == nil || len(extraction.Organisations) == 0 {
		return
	}

	logger := a.logger.WithContext(ctx)

	// Build set of existing project names for dedup
	existingProjects := make(map[string]bool)
	for _, p := range extraction.Projects {
		existingProjects[normalizeString(p)] = true
	}

	var remainingOrgs []string
	for _, org := range extraction.Organisations {
		// Try to resolve as a project
		rp := a.resolveProject(ctx, tenantID, org)
		if rp.ProjectID != nil || rp.Source == "glossary" {
			// This org is actually a known project — move it
			if !existingProjects[normalizeString(org)] {
				extraction.Projects = append(extraction.Projects, org)
				existingProjects[normalizeString(org)] = true
				logger.Debug("Reclassified organisation as project",
					logging.F("name", org),
					logging.F("source", rp.Source))
			}
		} else {
			remainingOrgs = append(remainingOrgs, org)
		}
	}

	extraction.Organisations = remainingOrgs
}

// BuildContext is the unified entry point for assembling background context for any
// pipeline stage (pf-7aa11e). Pre-NER stages pass Extraction=nil and gather functions
// degrade to body-scan only. Post-NER stages get full context with NER candidates merged.
//
// resolvedPeople and resolvedProjects are only populated for post-NER stages; pre-NER
// callers pass nil/empty slices.
func (a *ContextBuilderActivities) BuildContext(
	ctx context.Context,
	stage string,
	input workflows.BuildContextInput,
	resolvedPeople []workflows.ResolvedPerson,
	resolvedProjects []workflows.ResolvedProject,
) (*workflows.ContextPackage, error) {
	cfg, ok := DefaultConfigs()[stage]
	if !ok {
		return nil, fmt.Errorf("no context config for stage: %s", stage)
	}

	// Determine token budget: content-type override for deep_analyze, config default otherwise.
	tokenBudget := cfg.TokenBudget
	if stage == "deep_analyze" {
		tokenBudget = a.getTokenBudget(input.ContentType)
	}

	// Reduce context for notification content (pf-bcb565).
	// Notifications (Jira, Google Docs, Aha, etc.) need minimal grounding —
	// just enough glossary for name resolution, not full topic context.
	if strings.HasPrefix(input.ContentSubtype, "notification/") {
		tokenBudget = tokenBudget / 2
	}

	cp := &workflows.ContextPackage{
		TokenBudget:        tokenBudget,
		TotalTokensUsed:    0,
		ActiveRisks:        []workflows.ContextAssertion{},
		OpenActions:        []workflows.ContextAssertion{},
		RecentDecisions:    []workflows.ContextAssertion{},
		ProductEvents:      []workflows.ContextProductEvent{},
		GlossaryTerms:      []workflows.ContextGlossaryTerm{},
		TopicDescriptions:  []workflows.ContextTopicDescription{},
		ParticipantContext: resolvedPeople,
	}

	if a.contextRepo == nil {
		return cp, nil
	}

	// Collect resolved project IDs (empty for pre-NER stages).
	projectIDs := make([]int64, 0, len(resolvedProjects))
	for _, rp := range resolvedProjects {
		if rp.ProjectID != nil {
			projectIDs = append(projectIDs, *rp.ProjectID)
		}
	}

	tokensUsed := 0

	// Participant context (always include, ~15 tokens per participant).
	participantTokens := len(resolvedPeople) * 15
	tokensUsed += participantTokens

	// Iterate configured sections.
	for _, section := range cfg.Sections {
		switch section {
		case SectionRisks:
			if len(projectIDs) > 0 && tokensUsed+250 <= tokenBudget {
				risks, err := a.contextRepo.GetActiveRisks(ctx, projectIDs, cfg.QueryLimit(SectionRisks, 10))
				if err == nil {
					riskTokens := len(risks) * 25
					if tokensUsed+riskTokens <= tokenBudget {
						cp.ActiveRisks = toWorkflowAssertions(risks)
						tokensUsed += riskTokens
					}
				}
			}

		case SectionActions:
			if len(projectIDs) > 0 && tokensUsed+250 <= tokenBudget {
				actions, err := a.contextRepo.GetOpenActions(ctx, input.ConversationID, projectIDs, cfg.QueryLimit(SectionActions, 10))
				if err == nil {
					actionTokens := len(actions) * 25
					if tokensUsed+actionTokens <= tokenBudget {
						cp.OpenActions = toWorkflowAssertions(actions)
						tokensUsed += actionTokens
					}
				}
			}

		case SectionDecisions:
			if len(projectIDs) > 0 && tokensUsed+125 <= tokenBudget {
				decisions, err := a.contextRepo.GetRecentDecisions(ctx, projectIDs, cfg.Recency(SectionDecisions, 60), cfg.QueryLimit(SectionDecisions, 5))
				if err == nil {
					decisionTokens := len(decisions) * 25
					if tokensUsed+decisionTokens <= tokenBudget {
						cp.RecentDecisions = toWorkflowAssertions(decisions)
						tokensUsed += decisionTokens
					}
				}
			}

		case SectionEvents:
			if len(projectIDs) > 0 && tokensUsed+300 <= tokenBudget {
				events, err := a.contextRepo.GetProductEvents(ctx, projectIDs, cfg.Recency(SectionEvents, 90), cfg.QueryLimit(SectionEvents, 10))
				if err == nil {
					eventTokens := len(events) * 30
					if tokensUsed+eventTokens <= tokenBudget {
						cp.ProductEvents = toWorkflowEvents(events)
						tokensUsed += eventTokens
					}
				}
			}

		case SectionGlossary:
			if tokensUsed+400 <= tokenBudget {
				terms := a.gatherGlossary(ctx, input, projectIDs, cfg)
				if len(terms) > 0 {
					glossaryTokens := len(terms) * 20
					if tokensUsed+glossaryTokens <= tokenBudget {
						cp.GlossaryTerms = toWorkflowGlossary(terms)
						tokensUsed += glossaryTokens
					}
				}
			}

		case SectionTopics:
			if a.topicRepo != nil && tokensUsed+300 <= tokenBudget {
				topics := a.gatherTopics(ctx, input, resolvedProjects, cfg)
				if len(topics) > 0 {
					topicTokens := len(topics) * 30
					if tokensUsed+topicTokens <= tokenBudget {
						cp.TopicDescriptions = toWorkflowTopics(topics)
						tokensUsed += topicTokens
					}
				}
			}
		}
	}

	// Apply token budget enforcement: if over budget, truncate sections.
	if tokensUsed > tokenBudget {
		tokensUsed = a.applyTokenBudget(cp, tokenBudget)
	}

	cp.TotalTokensUsed = tokensUsed
	return cp, nil
}

// gatherGlossary collects glossary terms from body-text scanning and (if available)
// NER extraction output, then queries the database for matching definitions.
// Pre-NER stages get body-scan only; post-NER stages get both sources merged.
func (a *ContextBuilderActivities) gatherGlossary(ctx context.Context, input workflows.BuildContextInput, projectIDs []int64, cfg ContextConfig) []ContextGlossaryTerm {
	// Always: scan raw text for acronyms.
	allTerms := make(map[string]bool)
	for _, t := range scanForAcronyms(input.Subject + " " + input.Content) {
		allTerms[t] = true
	}

	// If NER extraction is available, also collect terms from extracted entities.
	if input.Extraction != nil {
		for _, t := range a.collectGlossaryTerms(input.Extraction) {
			allTerms[t] = true
		}
	}

	if len(allTerms) == 0 {
		return nil
	}

	terms := make([]string, 0, len(allTerms))
	for t := range allTerms {
		terms = append(terms, t)
	}

	glossary, err := a.contextRepo.GetGlossaryTerms(ctx, input.TenantID, terms, projectIDs, cfg.QueryLimit(SectionGlossary, 20))
	if err != nil {
		return nil
	}
	return glossary
}

// gatherTopics collects topic candidates from body-text scanning and (if available)
// NER extraction output, then queries the database for matching descriptions.
// Pre-NER stages get body-scan only; post-NER stages get both sources merged and deduped.
func (a *ContextBuilderActivities) gatherTopics(ctx context.Context, input workflows.BuildContextInput, resolvedProjects []workflows.ResolvedProject, cfg ContextConfig) []TopicResult {
	var allTopicResults []TopicResult

	// If NER extraction is available, collect candidates from extracted project/org names.
	if input.Extraction != nil {
		topicNames := a.collectTopicCandidates(input.Extraction, resolvedProjects)
		if len(topicNames) > 0 {
			nerTopics, err := a.topicRepo.ListForContext(ctx, input.TenantID, topicNames)
			if err == nil {
				allTopicResults = append(allTopicResults, nerTopics...)
			}
		}
	}

	// Always: scan full text (subject + body) for topic names and keywords.
	if input.Content != "" || input.Subject != "" {
		fullText := input.Subject + "\n" + input.Content
		bodyTopics, err := a.topicRepo.ScanContentForTopics(ctx, input.TenantID, fullText)
		if err == nil {
			// Merge, dedup by topic ID.
			seen := make(map[int64]bool)
			for _, t := range allTopicResults {
				seen[t.ID] = true
			}
			for _, t := range bodyTopics {
				if !seen[t.ID] {
					allTopicResults = append(allTopicResults, t)
					seen[t.ID] = true
				}
			}
		}
	}

	return allTopicResults
}

// BuildExtractionContext builds a lightweight context (glossary + topics only)
// for injection into the extraction stage.
//
// Deprecated: use BuildContext with stage="extract_ner" or "extract_semantic".
// Kept as a shim for running Temporal workflows during deploy (pf-7aa11e).
func (a *ContextBuilderActivities) BuildExtractionContext(ctx context.Context, input workflows.BuildExtractionContextInput) (*workflows.BuildExtractionContextOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "BuildExtractionContext"),
		logging.F("tenant_id", input.TenantID),
	)

	cp, err := a.BuildContext(ctx, "extract_ner", workflows.BuildContextInput{
		TenantID: input.TenantID,
		Subject:  input.Subject,
		Content:  input.Content,
	}, nil, nil)
	if err != nil {
		return nil, err
	}

	output := &workflows.BuildExtractionContextOutput{}
	if len(cp.GlossaryTerms) == 0 && len(cp.TopicDescriptions) == 0 {
		return output, nil
	}

	output.BackgroundContext = cp.FormatForPrompt()
	output.GlossaryCount = len(cp.GlossaryTerms)
	output.TopicCount = len(cp.TopicDescriptions)

	logger.Info("built extraction context",
		logging.F("glossary_count", output.GlossaryCount),
		logging.F("topic_count", output.TopicCount),
	)

	return output, nil
}

// scanForAcronyms extracts potential acronyms from raw text using the same
// isAcronym heuristic (all-caps, 2-6 chars). Returns deduplicated terms.
func scanForAcronyms(text string) []string {
	seen := make(map[string]bool)
	// Split on whitespace and punctuation boundaries
	for _, word := range strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if isAcronym(word) && !seen[word] {
			seen[word] = true
		}
	}
	result := make([]string, 0, len(seen))
	for term := range seen {
		result = append(result, term)
	}
	return result
}

// scanForTopicCandidates extracts potential topic names from the subject line.
// Returns individual significant words (>3 chars) and multi-word capitalized phrases.
func scanForTopicCandidates(subject string) []string {
	// Strip common reply/forward prefixes
	subj := subject
	for _, prefix := range []string{"Re: ", "RE: ", "Fwd: ", "FW: ", "Fw: "} {
		subj = strings.TrimPrefix(subj, prefix)
	}

	candidates := make(map[string]bool)
	words := strings.Fields(subj)
	for _, w := range words {
		// Skip short words and common articles/prepositions
		clean := strings.Trim(w, ".,;:!?()[]\"'")
		if len(clean) > 3 && clean != "from" && clean != "with" && clean != "that" && clean != "this" && clean != "have" && clean != "been" {
			candidates[clean] = true
		}
	}

	result := make([]string, 0, len(candidates))
	for name := range candidates {
		result = append(result, name)
	}
	return result
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

// collectTopicCandidates gathers names that might resolve as topics.
// These are: unresolved project names + extracted organisations (which are often topics).
func (a *ContextBuilderActivities) collectTopicCandidates(extraction *workflows.SLMPipelineExtractEntitiesOutput, resolvedProjects []workflows.ResolvedProject) []string {
	if extraction == nil {
		return nil
	}

	// Gather unresolved project names (those that didn't match a project)
	candidates := make(map[string]bool)
	resolvedNames := make(map[string]bool)
	for _, rp := range resolvedProjects {
		if rp.ProjectID != nil {
			resolvedNames[normalizeString(rp.Name)] = true
		}
	}
	for _, name := range extraction.Projects {
		if !resolvedNames[normalizeString(name)] {
			candidates[name] = true
		}
	}

	// Also try organisations — many are actually topics (e.g. "Cloud NAT", "DevCloud")
	for _, org := range extraction.Organisations {
		candidates[org] = true
	}

	result := make([]string, 0, len(candidates))
	for name := range candidates {
		result = append(result, name)
	}
	return result
}

// toWorkflowTopics converts TopicResult slice to workflow ContextTopicDescription slice.
func toWorkflowTopics(topics []TopicResult) []workflows.ContextTopicDescription {
	result := make([]workflows.ContextTopicDescription, 0, len(topics))
	for _, t := range topics {
		if t.Description == "" {
			continue // Skip topics without descriptions — no value for context
		}
		result = append(result, workflows.ContextTopicDescription{
			Name:        t.Name,
			Description: t.Description,
		})
	}
	return result
}

// applyTokenBudget truncates sections from tail to fit within budget.
// Order of truncation (least valuable first):
// 1. Topics (drop from tail)
// 2. Glossary terms (drop from tail)
// 3. Product events (drop from tail)
// 4. Decisions (drop from tail)
// 5. Actions (drop from tail)
// 6. Risks (drop from tail)
// 7. Participants (never drop - always included)
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
		{"topics", len(cp.TopicDescriptions), 30, func(keep int) { cp.TopicDescriptions = cp.TopicDescriptions[:keep] }},
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
