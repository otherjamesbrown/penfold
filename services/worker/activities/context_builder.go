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
	"github.com/otherjamesbrown/penfold/pkg/langfuse"
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
	logger             logging.Logger
	entityResolver     EntityResolverInterface
	entityRepo         EntityLookupInterface
	contextRepo        ContextPackageRepository
	topicRepo          TopicLookupInterface
	pipelineRepo       PipelineRepository
	configResolver     *enrichmentconfig.ConfigResolver
	langfuseIngestion  *langfuse.Ingestion // optional; nil disables Langfuse spans
	newsletterRepo     NewsletterContextRepository
	tenantContextRepo  TenantContextRepository
}

// NewContextBuilderActivities creates a new ContextBuilderActivities instance.
// newsletterRepo is optional; when non-nil it enables user_context, active_projects,
// and tracked_products context providers for newsletter pipeline stages.
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
	a := &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: entityResolver,
		entityRepo:     entityRepo,
		contextRepo:    contextRepo,
		topicRepo:      topicRepo,
		pipelineRepo:   pipelineRepo,
		configResolver: configResolver,
	}
	// Register built-in context providers with nil optional repos.
	// Call WithNewsletterContextRepo / WithTenantContextRepo after construction to enable those providers.
	RegisterContextProviders(a.logger, contextRepo, nil, topicRepo, nil)
	return a
}

// WithNewsletterContextRepo injects an optional NewsletterContextRepository and
// re-registers context providers so user_context, active_projects, and
// tracked_products providers use the new repo.
func (a *ContextBuilderActivities) WithNewsletterContextRepo(repo NewsletterContextRepository) *ContextBuilderActivities {
	a.newsletterRepo = repo
	RegisterContextProviders(a.logger, a.contextRepo, repo, a.topicRepo, a.tenantContextRepo)
	return a
}

// WithTenantContextRepo injects a TenantContextRepository and re-registers context providers
// so the tenant_context provider uses the new repo.
func (a *ContextBuilderActivities) WithTenantContextRepo(repo TenantContextRepository) *ContextBuilderActivities {
	a.tenantContextRepo = repo
	RegisterContextProviders(a.logger, a.contextRepo, a.newsletterRepo, a.topicRepo, repo)
	return a
}

// WithLangfuse injects an optional Langfuse ingestion client.
// When set, BuildStageContext logs a span with assembly metadata for each call.
func (a *ContextBuilderActivities) WithLangfuse(ingestion *langfuse.Ingestion) *ContextBuilderActivities {
	a.langfuseIngestion = ingestion
	return a
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

	// Step 3: Build context via BuildStageContext (config-driven provider assembly).
	recordHeartbeat(ctx, "building context package")
	bgContext, err := a.BuildStageContext(ctx, workflows.BuildStageContextInput{
		TenantID:          input.TenantID,
		Pipeline:          "standard",
		Stage:             "deep_analyze",
		SourceID:          input.SourceID,
		ContentID:         input.ContentID,
		JobID:             input.JobID,
		ContentType:       input.ContentType,
		ContentSubtype:    input.ContentSubtype,
		Content:           input.Content,
		Subject:           input.Subject,
		SenderEmail:       input.SenderEmail,
		SenderName:        input.SenderName,
		ThreadID:          input.ThreadID,
		ParticipantEmails: input.ParticipantEmails,
		ConversationID:    input.ConversationID,
		Extraction:        input.Extraction,
		ResolvedPeople:    resolvedPeople,
		ResolvedProjects:  resolvedProjects,
	})
	if err != nil {
		pe := perrors.ClassifyError(err, "resolve")
		logger.Error("Failed to build stage context", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	output.BackgroundContext = bgContext

	logger.Info("Context assembled via BuildStageContext",
		logging.F("context_length", len(bgContext)))

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
			"context_length":    len(bgContext),
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

	// pf-0f08e0: Dedup enriched list by canonical name.
	// After enrichment, "Miroslav" → "Miroslav Ponec" may duplicate an existing
	// "Miroslav Ponec" entry. NormalizeDisplayName handles "Last, First" → "First Last"
	// and lowercasing collapses different representations of the same person.
	// Keep the first occurrence (preserves role if any).
	seen := make(map[string]bool)
	deduped := make([]workflows.PersonResult, 0, len(enriched))
	for _, p := range enriched {
		key := strings.ToLower(entities.NormalizeDisplayName(p.Name))
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

// BuildExtractionContext builds a lightweight context (glossary + topics only)
// for injection into the extraction stage.
func (a *ContextBuilderActivities) BuildExtractionContext(ctx context.Context, input workflows.BuildExtractionContextInput) (*workflows.BuildExtractionContextOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "BuildExtractionContext"),
		logging.F("tenant_id", input.TenantID),
	)

	bgContext, err := a.BuildStageContext(ctx, workflows.BuildStageContextInput{
		TenantID: input.TenantID,
		Pipeline: "standard",
		Stage:    "extract_ner",
		Subject:  input.Subject,
		Content:  input.Content,
	})
	if err != nil {
		return nil, err
	}

	output := &workflows.BuildExtractionContextOutput{
		BackgroundContext: bgContext,
	}
	if bgContext != "" {
		logger.Info("built extraction context", logging.F("context_length", len(bgContext)))
	}
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

// isAcronym checks if a term looks like an acronym (all caps, 2-6 chars).
func isAcronym(term string) bool {
	if len(term) < 2 || len(term) > 6 {
		return false
	}
	return strings.ToUpper(term) == term
}

// BuildStageContext assembles a BackgroundContext markdown string for a pipeline stage.
//
// It reads context_providers from pipeline_definitions for the given tenant/pipeline/stage,
// iterates them in declared order, calls each provider via the registry, and concatenates
// non-empty results. Provider failures are non-blocking: unknown or erroring providers are
// logged as warnings and skipped. Returns an empty string when no providers are configured.
//
// When a Langfuse ingestion is configured via WithLangfuse and a LangfuseTraceID is provided,
// a span is created with assembly metadata (providers requested/succeeded/failed, context
// length, per-provider timing).
func (a *ContextBuilderActivities) BuildStageContext(ctx context.Context, input workflows.BuildStageContextInput) (string, error) {
	startTime := time.Now()
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "BuildStageContext"),
		logging.F("tenant_id", input.TenantID),
		logging.F("pipeline", input.Pipeline),
		logging.F("stage", input.Stage),
	)

	if a.pipelineRepo == nil {
		// pipelineRepo is optional — return empty context when not configured
		logger.Debug("BuildStageContext: pipelineRepo not configured, returning empty context")
		return "", nil
	}

	// Load context_providers from pipeline_definitions.
	providerNames, err := a.pipelineRepo.GetContextProviders(ctx, input.TenantID, input.Pipeline, input.Stage)
	if err != nil {
		return "", fmt.Errorf("BuildStageContext: loading context providers: %w", err)
	}

	if len(providerNames) == 0 {
		logger.Debug("BuildStageContext: no context providers configured for stage")
		return "", nil
	}

	logger.Debug("BuildStageContext: assembling context",
		logging.F("providers", strings.Join(providerNames, ",")),
	)

	// providerTiming tracks per-provider latency for Langfuse metadata.
	type providerTiming struct {
		name  string
		durMS int
	}

	var (
		sections  []string
		succeeded []string
		failed    []string
		timings   []providerTiming
	)

	for _, name := range providerNames {
		provider, ok := LookupProvider(name)
		if !ok {
			logger.Warn("BuildStageContext: unknown provider — skipping",
				logging.F("provider", name),
				logging.F("pipeline", input.Pipeline),
				logging.F("stage", input.Stage),
			)
			failed = append(failed, name)
			continue
		}

		t := time.Now()
		section, buildErr := provider.Build(ctx, ContextProviderInput{
			TenantID:          input.TenantID,
			SourceID:          input.SourceID,
			ContentID:         input.ContentID,
			JobID:             input.JobID,
			ContentType:       input.ContentType,
			ContentSubtype:    input.ContentSubtype,
			Content:           input.Content,
			Subject:           input.Subject,
			SenderEmail:       input.SenderEmail,
			SenderName:        input.SenderName,
			ThreadID:          input.ThreadID,
			ParticipantEmails: input.ParticipantEmails,
			ConversationID:    input.ConversationID,
			Extraction:        input.Extraction,
			ResolvedPeople:    input.ResolvedPeople,
			ResolvedProjects:  input.ResolvedProjects,
		})
		durMS := int(time.Since(t).Milliseconds())
		timings = append(timings, providerTiming{name, durMS})

		if buildErr != nil {
			logger.Warn("BuildStageContext: provider failed — skipping",
				logging.F("provider", name),
				logging.F("error", buildErr.Error()),
			)
			failed = append(failed, name)
			continue
		}

		succeeded = append(succeeded, name)
		if section != "" {
			sections = append(sections, section)
		}
	}

	assembled := strings.Join(sections, "\n\n")

	logger.Info("BuildStageContext: assembly complete",
		logging.F("providers_requested", len(providerNames)),
		logging.F("providers_succeeded", len(succeeded)),
		logging.F("providers_failed", len(failed)),
		logging.F("context_length", len(assembled)),
	)

	// Log Langfuse span when configured.
	if a.langfuseIngestion != nil && input.LangfuseTraceID != "" {
		timingMap := make(map[string]int, len(timings))
		for _, t := range timings {
			timingMap[t.name] = t.durMS
		}
		a.langfuseIngestion.CreateSpan(langfuse.SpanEvent{
			ID:        newLangfuseID(),
			TraceID:   input.LangfuseTraceID,
			ParentID:  input.LangfusePhaseID,
			Name:      "build_stage_context",
			StartTime: startTime,
			EndTime:   time.Now(),
			Metadata: map[string]any{
				"providers_requested": providerNames,
				"providers_succeeded": succeeded,
				"providers_failed":    failed,
				"context_length":      len(assembled),
				"provider_timings_ms": timingMap,
			},
		})
	}

	return assembled, nil
}
