// Package extractor provides relationship extraction capabilities from content.
package extractor

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
)

// AIClient defines the interface for AI service operations used by the extractor.
type AIClient interface {
	// ExtractAssertions extracts structured assertions from content.
	ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error)
}

// RelationshipExtractor extracts relationships between entities from content.
// It uses pattern-based extraction for structured content (emails, meetings)
// and AI-powered extraction for unstructured content.
type RelationshipExtractor struct {
	logger   zerolog.Logger
	aiClient AIClient
	options  ExtractionOptions
}

// NewRelationshipExtractor creates a new RelationshipExtractor instance.
func NewRelationshipExtractor(logger zerolog.Logger, aiClient AIClient, opts *ExtractionOptions) *RelationshipExtractor {
	options := DefaultExtractionOptions()
	if opts != nil {
		options = *opts
	}

	return &RelationshipExtractor{
		logger:   logger.With().Str("component", "relationship_extractor").Logger(),
		aiClient: aiClient,
		options:  options,
	}
}

// Extract extracts relationships from generic content.
// It uses AI-powered extraction to identify entities and their relationships.
func (e *RelationshipExtractor) Extract(ctx context.Context, content string) (*ExtractionResult, error) {
	startTime := time.Now()

	e.logger.Debug().
		Int("content_length", len(content)).
		Float32("min_confidence", e.options.MinConfidence).
		Bool("use_ai", e.options.UseAI).
		Msg("Starting relationship extraction from content")

	if content == "" {
		return &ExtractionResult{
			Relationships:    []Relationship{},
			Entities:         []Entity{},
			ProcessingTimeMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	result := &ExtractionResult{
		Relationships: []Relationship{},
		Entities:      []Entity{},
	}

	// Extract entities and relationships using AI if enabled
	if e.options.UseAI && e.aiClient != nil {
		aiResult, err := e.extractWithAI(ctx, content)
		if err != nil {
			e.logger.Warn().Err(err).Msg("AI extraction failed, falling back to pattern-based extraction")
		} else {
			result.Relationships = append(result.Relationships, aiResult.Relationships...)
			result.Entities = append(result.Entities, aiResult.Entities...)
			result.ModelUsed = aiResult.ModelUsed
		}
	}

	// Extract entities using pattern-based extraction as fallback/supplement
	patternEntities := e.extractEntitiesFromText(content)
	result.Entities = mergeEntities(result.Entities, patternEntities)

	// Extract co-mention relationships from content
	coMentions := e.extractCoMentionRelationships(result.Entities, content)
	result.Relationships = append(result.Relationships, coMentions...)

	// Filter by confidence threshold
	result.TotalFound = len(result.Relationships)
	result.Relationships = e.filterByConfidence(result.Relationships)
	result.FilteredCount = result.TotalFound - len(result.Relationships)

	// Limit to max relationships
	if e.options.MaxRelationships > 0 && len(result.Relationships) > e.options.MaxRelationships {
		result.Relationships = result.Relationships[:e.options.MaxRelationships]
	}

	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()

	e.logger.Info().
		Int("relationships_found", len(result.Relationships)).
		Int("entities_found", len(result.Entities)).
		Int64("processing_time_ms", result.ProcessingTimeMs).
		Msg("Relationship extraction completed")

	return result, nil
}

// ExtractFromEmail extracts relationships from email content.
// It identifies communication relationships from email headers and
// topic relationships from the email body.
func (e *RelationshipExtractor) ExtractFromEmail(ctx context.Context, email *EmailContent) (*ExtractionResult, error) {
	startTime := time.Now()

	if email == nil {
		return &ExtractionResult{
			Relationships:    []Relationship{},
			Entities:         []Entity{},
			ProcessingTimeMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	e.logger.Debug().
		Str("message_id", email.MessageID).
		Str("from", email.From).
		Int("to_count", len(email.To)).
		Int("cc_count", len(email.CC)).
		Msg("Starting relationship extraction from email")

	result := &ExtractionResult{
		Relationships: []Relationship{},
		Entities:      []Entity{},
	}

	// Create entity for sender
	senderEntity := e.createPersonEntity(email.From, email.FromName)
	result.Entities = append(result.Entities, senderEntity)

	// Create entities for recipients and extract communication relationships
	for i, to := range email.To {
		toName := ""
		if i < len(email.ToNames) {
			toName = email.ToNames[i]
		}
		recipientEntity := e.createPersonEntity(to, toName)
		result.Entities = append(result.Entities, recipientEntity)

		// Create "knows" relationship from sender to recipient
		rel := Relationship{
			ID:         uuid.New().String(),
			Source:     &senderEntity,
			Target:     &recipientEntity,
			Type:       RelationshipTypeKnows,
			Strength:   0.8,
			Confidence: 0.9, // High confidence for direct email communication
			Context:    fmt.Sprintf("Email communication: %s", email.Subject),
			Evidence: []Evidence{
				{
					SourceID:               email.MessageID,
					SourceType:             "email",
					Excerpt:                fmt.Sprintf("Email from %s to %s: %s", email.From, to, email.Subject),
					DiscoveredAt:           time.Now(),
					ConfidenceContribution: 0.9,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		result.Relationships = append(result.Relationships, rel)
	}

	// Create entities and relationships for CC recipients (weaker relationship)
	for i, cc := range email.CC {
		ccName := ""
		if i < len(email.CCNames) {
			ccName = email.CCNames[i]
		}
		ccEntity := e.createPersonEntity(cc, ccName)
		result.Entities = append(result.Entities, ccEntity)

		// Create "mentioned_with" relationship for CC recipients
		rel := Relationship{
			ID:         uuid.New().String(),
			Source:     &senderEntity,
			Target:     &ccEntity,
			Type:       RelationshipTypeMentionedWith,
			Strength:   0.5,
			Confidence: 0.7, // Lower confidence for CC
			Context:    fmt.Sprintf("CC'd on email: %s", email.Subject),
			Evidence: []Evidence{
				{
					SourceID:               email.MessageID,
					SourceType:             "email",
					Excerpt:                fmt.Sprintf("CC'd on email from %s: %s", email.From, email.Subject),
					DiscoveredAt:           time.Now(),
					ConfidenceContribution: 0.7,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		result.Relationships = append(result.Relationships, rel)
	}

	// Extract "collaborates_with" relationships between all To recipients
	for i := 0; i < len(email.To); i++ {
		for j := i + 1; j < len(email.To); j++ {
			toNameI := ""
			if i < len(email.ToNames) {
				toNameI = email.ToNames[i]
			}
			toNameJ := ""
			if j < len(email.ToNames) {
				toNameJ = email.ToNames[j]
			}

			entityI := e.createPersonEntity(email.To[i], toNameI)
			entityJ := e.createPersonEntity(email.To[j], toNameJ)

			rel := Relationship{
				ID:         uuid.New().String(),
				Source:     &entityI,
				Target:     &entityJ,
				Type:       RelationshipTypeCollaboratesWith,
				Strength:   0.6,
				Confidence: 0.7,
				Context:    fmt.Sprintf("Co-recipients on email: %s", email.Subject),
				Evidence: []Evidence{
					{
						SourceID:               email.MessageID,
						SourceType:             "email",
						Excerpt:                fmt.Sprintf("Both recipients on email: %s", email.Subject),
						DiscoveredAt:           time.Now(),
						ConfidenceContribution: 0.7,
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			result.Relationships = append(result.Relationships, rel)
		}
	}

	// Extract topic relationships from email body using AI
	if e.options.UseAI && e.aiClient != nil && email.Body != "" {
		bodyResult, err := e.Extract(ctx, email.Body)
		if err == nil {
			result.Relationships = append(result.Relationships, bodyResult.Relationships...)
			result.Entities = mergeEntities(result.Entities, bodyResult.Entities)
			result.ModelUsed = bodyResult.ModelUsed
		}
	}

	// Filter and limit results
	result.TotalFound = len(result.Relationships)
	result.Relationships = e.filterByConfidence(result.Relationships)
	result.FilteredCount = result.TotalFound - len(result.Relationships)

	if e.options.MaxRelationships > 0 && len(result.Relationships) > e.options.MaxRelationships {
		result.Relationships = result.Relationships[:e.options.MaxRelationships]
	}

	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()

	e.logger.Info().
		Str("message_id", email.MessageID).
		Int("relationships_found", len(result.Relationships)).
		Int("entities_found", len(result.Entities)).
		Int64("processing_time_ms", result.ProcessingTimeMs).
		Msg("Email relationship extraction completed")

	return result, nil
}

// ExtractFromMeeting extracts relationships from meeting content.
// It identifies collaboration relationships from attendees and
// topic relationships from the meeting description and notes.
func (e *RelationshipExtractor) ExtractFromMeeting(ctx context.Context, meeting *MeetingContent) (*ExtractionResult, error) {
	startTime := time.Now()

	if meeting == nil {
		return &ExtractionResult{
			Relationships:    []Relationship{},
			Entities:         []Entity{},
			ProcessingTimeMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	e.logger.Debug().
		Str("meeting_id", meeting.MeetingID).
		Str("title", meeting.Title).
		Int("attendee_count", len(meeting.Attendees)).
		Msg("Starting relationship extraction from meeting")

	result := &ExtractionResult{
		Relationships: []Relationship{},
		Entities:      []Entity{},
	}

	// Create entity for organizer
	organizerEntity := e.createPersonEntity(meeting.Organizer, meeting.OrganizerName)
	result.Entities = append(result.Entities, organizerEntity)

	// Create meeting event entity
	meetingEntity := Entity{
		ID:            meeting.MeetingID,
		Type:          EntityTypeEvent,
		Name:          meeting.Title,
		CanonicalName: strings.ToLower(meeting.Title),
		Metadata: map[string]string{
			"start_time": meeting.StartTime.Format(time.RFC3339),
			"end_time":   meeting.EndTime.Format(time.RFC3339),
			"location":   meeting.Location,
		},
		Confidence: 1.0,
	}
	result.Entities = append(result.Entities, meetingEntity)

	// Create "attended" relationship for organizer
	organizerAttended := Relationship{
		ID:         uuid.New().String(),
		Source:     &organizerEntity,
		Target:     &meetingEntity,
		Type:       RelationshipTypeAttended,
		Strength:   1.0, // Organizer definitely attended
		Confidence: 1.0,
		Context:    fmt.Sprintf("Organized meeting: %s", meeting.Title),
		Evidence: []Evidence{
			{
				SourceID:               meeting.MeetingID,
				SourceType:             "meeting",
				Excerpt:                fmt.Sprintf("Organized by %s", meeting.OrganizerName),
				DiscoveredAt:           time.Now(),
				ConfidenceContribution: 1.0,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	result.Relationships = append(result.Relationships, organizerAttended)

	// Create entities and relationships for attendees
	attendeeEntities := make([]Entity, 0, len(meeting.Attendees))
	for i, attendee := range meeting.Attendees {
		attendeeName := ""
		if i < len(meeting.AttendeeNames) {
			attendeeName = meeting.AttendeeNames[i]
		}
		attendeeEntity := e.createPersonEntity(attendee, attendeeName)
		attendeeEntities = append(attendeeEntities, attendeeEntity)
		result.Entities = append(result.Entities, attendeeEntity)

		// Create "attended" relationship
		attendedRel := Relationship{
			ID:         uuid.New().String(),
			Source:     &attendeeEntity,
			Target:     &meetingEntity,
			Type:       RelationshipTypeAttended,
			Strength:   0.8,
			Confidence: 0.9,
			Context:    fmt.Sprintf("Attended meeting: %s", meeting.Title),
			Evidence: []Evidence{
				{
					SourceID:               meeting.MeetingID,
					SourceType:             "meeting",
					Excerpt:                fmt.Sprintf("Attendee at %s", meeting.Title),
					DiscoveredAt:           time.Now(),
					ConfidenceContribution: 0.9,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		result.Relationships = append(result.Relationships, attendedRel)

		// Create "knows" relationship between organizer and attendee
		knowsRel := Relationship{
			ID:         uuid.New().String(),
			Source:     &organizerEntity,
			Target:     &attendeeEntity,
			Type:       RelationshipTypeKnows,
			Strength:   0.7,
			Confidence: 0.85,
			Context:    fmt.Sprintf("Meeting attendee: %s", meeting.Title),
			Evidence: []Evidence{
				{
					SourceID:               meeting.MeetingID,
					SourceType:             "meeting",
					Excerpt:                fmt.Sprintf("Both participated in %s", meeting.Title),
					DiscoveredAt:           time.Now(),
					ConfidenceContribution: 0.85,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		result.Relationships = append(result.Relationships, knowsRel)
	}

	// Create "collaborates_with" relationships between all attendees
	for i := 0; i < len(attendeeEntities); i++ {
		for j := i + 1; j < len(attendeeEntities); j++ {
			entityI := attendeeEntities[i]
			entityJ := attendeeEntities[j]

			rel := Relationship{
				ID:         uuid.New().String(),
				Source:     &entityI,
				Target:     &entityJ,
				Type:       RelationshipTypeCollaboratesWith,
				Strength:   0.7,
				Confidence: 0.8,
				Context:    fmt.Sprintf("Co-attendees at meeting: %s", meeting.Title),
				Evidence: []Evidence{
					{
						SourceID:               meeting.MeetingID,
						SourceType:             "meeting",
						Excerpt:                fmt.Sprintf("Both attended %s", meeting.Title),
						DiscoveredAt:           time.Now(),
						ConfidenceContribution: 0.8,
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			result.Relationships = append(result.Relationships, rel)
		}
	}

	// Extract topic relationships from meeting description and notes
	combinedText := meeting.Description + "\n" + meeting.Notes
	if e.options.UseAI && e.aiClient != nil && combinedText != "" {
		textResult, err := e.Extract(ctx, combinedText)
		if err == nil {
			result.Relationships = append(result.Relationships, textResult.Relationships...)
			result.Entities = mergeEntities(result.Entities, textResult.Entities)
			result.ModelUsed = textResult.ModelUsed
		}
	}

	// Filter and limit results
	result.TotalFound = len(result.Relationships)
	result.Relationships = e.filterByConfidence(result.Relationships)
	result.FilteredCount = result.TotalFound - len(result.Relationships)

	if e.options.MaxRelationships > 0 && len(result.Relationships) > e.options.MaxRelationships {
		result.Relationships = result.Relationships[:e.options.MaxRelationships]
	}

	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()

	e.logger.Info().
		Str("meeting_id", meeting.MeetingID).
		Int("relationships_found", len(result.Relationships)).
		Int("entities_found", len(result.Entities)).
		Int64("processing_time_ms", result.ProcessingTimeMs).
		Msg("Meeting relationship extraction completed")

	return result, nil
}

// extractWithAI uses the AI service to extract relationships from content.
func (e *RelationshipExtractor) extractWithAI(ctx context.Context, content string) (*ExtractionResult, error) {
	if e.aiClient == nil {
		return nil, fmt.Errorf("AI client not configured")
	}

	minConfidence := e.options.MinConfidence
	maxAssertions := int32(50) // Extract more assertions to find relationships

	req := &aiv1.AssertionRequest{
		Content:       content,
		MinConfidence: &minConfidence,
		MaxAssertions: &maxAssertions,
	}

	resp, err := e.aiClient.ExtractAssertions(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("AI assertion extraction failed: %w", err)
	}

	result := &ExtractionResult{
		Relationships: []Relationship{},
		Entities:      []Entity{},
		ModelUsed:     resp.ModelUsed,
	}

	// Convert assertions to entities and relationships
	entityMap := make(map[string]*Entity)

	for _, assertion := range resp.Assertions {
		// Create or get source entity
		sourceKey := strings.ToLower(assertion.Subject)
		sourceEntity, exists := entityMap[sourceKey]
		if !exists {
			entityType := inferEntityTypeFromCategory(assertion.GetCategory())
			sourceEntity = &Entity{
				ID:            uuid.New().String(),
				Type:          entityType,
				Name:          assertion.Subject,
				CanonicalName: sourceKey,
				Metadata:      map[string]string{},
				Confidence:    assertion.Confidence,
			}
			entityMap[sourceKey] = sourceEntity
			result.Entities = append(result.Entities, *sourceEntity)
		}

		// Create or get target entity
		targetKey := strings.ToLower(assertion.Object)
		targetEntity, exists := entityMap[targetKey]
		if !exists {
			targetEntity = &Entity{
				ID:            uuid.New().String(),
				Type:          inferEntityTypeFromText(assertion.Object),
				Name:          assertion.Object,
				CanonicalName: targetKey,
				Metadata:      map[string]string{},
				Confidence:    assertion.Confidence,
			}
			entityMap[targetKey] = targetEntity
			result.Entities = append(result.Entities, *targetEntity)
		}

		// Create relationship from assertion
		relType := inferRelationshipTypeFromPredicate(assertion.Predicate)
		rel := Relationship{
			ID:         uuid.New().String(),
			Source:     sourceEntity,
			Target:     targetEntity,
			Type:       relType,
			Strength:   assertion.Confidence,
			Confidence: assertion.Confidence,
			Context:    assertion.Predicate,
			Evidence: []Evidence{
				{
					SourceID:               "ai_extraction",
					SourceType:             "assertion",
					Excerpt:                assertion.GetSourceText(),
					DiscoveredAt:           time.Now(),
					ConfidenceContribution: assertion.Confidence,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		result.Relationships = append(result.Relationships, rel)
	}

	return result, nil
}

// extractEntitiesFromText uses pattern matching to extract entities from text.
func (e *RelationshipExtractor) extractEntitiesFromText(text string) []Entity {
	entities := []Entity{}

	// Email pattern for person entities
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	emails := emailPattern.FindAllString(text, -1)
	for _, email := range emails {
		entity := Entity{
			ID:            uuid.New().String(),
			Type:          EntityTypePerson,
			Name:          email,
			CanonicalName: strings.ToLower(email),
			Metadata:      map[string]string{"email": email},
			Confidence:    0.9,
		}
		entities = append(entities, entity)
	}

	// URL pattern for potential document/resource entities
	urlPattern := regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	urls := urlPattern.FindAllString(text, -1)
	for _, url := range urls {
		entity := Entity{
			ID:            uuid.New().String(),
			Type:          EntityTypeDocument,
			Name:          url,
			CanonicalName: strings.ToLower(url),
			Metadata:      map[string]string{"url": url},
			Confidence:    0.8,
		}
		entities = append(entities, entity)
	}

	// Hashtag pattern for topic entities
	hashtagPattern := regexp.MustCompile(`#[a-zA-Z][a-zA-Z0-9_]*`)
	hashtags := hashtagPattern.FindAllString(text, -1)
	for _, tag := range hashtags {
		entity := Entity{
			ID:            uuid.New().String(),
			Type:          EntityTypeTopic,
			Name:          tag[1:], // Remove the #
			CanonicalName: strings.ToLower(tag[1:]),
			Metadata:      map[string]string{"hashtag": tag},
			Confidence:    0.85,
		}
		entities = append(entities, entity)
	}

	return entities
}

// extractCoMentionRelationships creates relationships between entities that appear together in text.
func (e *RelationshipExtractor) extractCoMentionRelationships(entities []Entity, text string) []Relationship {
	relationships := []Relationship{}
	textLower := strings.ToLower(text)

	// Find entities that appear in the text
	mentionedEntities := []Entity{}
	for _, entity := range entities {
		if strings.Contains(textLower, strings.ToLower(entity.Name)) ||
			strings.Contains(textLower, entity.CanonicalName) {
			mentionedEntities = append(mentionedEntities, entity)
		}
	}

	// Create co-mention relationships between mentioned entities
	for i := 0; i < len(mentionedEntities); i++ {
		for j := i + 1; j < len(mentionedEntities); j++ {
			entityI := mentionedEntities[i]
			entityJ := mentionedEntities[j]

			rel := Relationship{
				ID:         uuid.New().String(),
				Source:     &entityI,
				Target:     &entityJ,
				Type:       RelationshipTypeMentionedWith,
				Strength:   0.5,
				Confidence: 0.6,
				Context:    "Co-mentioned in content",
				Evidence: []Evidence{
					{
						SourceID:               "pattern_extraction",
						SourceType:             "co_mention",
						Excerpt:                fmt.Sprintf("%s and %s mentioned together", entityI.Name, entityJ.Name),
						DiscoveredAt:           time.Now(),
						ConfidenceContribution: 0.6,
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			relationships = append(relationships, rel)
		}
	}

	return relationships
}

// createPersonEntity creates an Entity for a person from email and name.
func (e *RelationshipExtractor) createPersonEntity(email, name string) Entity {
	displayName := name
	if displayName == "" {
		displayName = email
	}

	return Entity{
		ID:            uuid.New().String(),
		Type:          EntityTypePerson,
		Name:          displayName,
		CanonicalName: strings.ToLower(email),
		Metadata: map[string]string{
			"email": email,
		},
		Confidence: 1.0, // High confidence for explicit email addresses
	}
}

// filterByConfidence removes relationships below the confidence threshold.
func (e *RelationshipExtractor) filterByConfidence(relationships []Relationship) []Relationship {
	filtered := make([]Relationship, 0, len(relationships))
	for _, rel := range relationships {
		if rel.Confidence >= e.options.MinConfidence {
			filtered = append(filtered, rel)
		}
	}
	return filtered
}

// mergeEntities combines two entity slices, deduplicating by canonical name.
func mergeEntities(a, b []Entity) []Entity {
	seen := make(map[string]bool)
	result := make([]Entity, 0, len(a)+len(b))

	for _, entity := range a {
		key := entity.CanonicalName
		if !seen[key] {
			seen[key] = true
			result = append(result, entity)
		}
	}

	for _, entity := range b {
		key := entity.CanonicalName
		if !seen[key] {
			seen[key] = true
			result = append(result, entity)
		}
	}

	return result
}

// inferEntityTypeFromCategory infers entity type from assertion category.
func inferEntityTypeFromCategory(category string) EntityType {
	switch strings.ToLower(category) {
	case "person", "personnel", "individual":
		return EntityTypePerson
	case "organization", "organizational", "company", "team":
		return EntityTypeOrganization
	case "topic", "subject", "theme":
		return EntityTypeTopic
	case "project", "initiative":
		return EntityTypeProject
	case "location", "geographical", "place":
		return EntityTypeLocation
	case "event", "meeting", "conference":
		return EntityTypeEvent
	case "document", "artifact", "file":
		return EntityTypeDocument
	default:
		return EntityTypeUnknown
	}
}

// inferEntityTypeFromText attempts to infer entity type from the text content.
func inferEntityTypeFromText(text string) EntityType {
	textLower := strings.ToLower(text)

	// Check for organization indicators
	orgPatterns := []string{"inc.", "corp.", "company", "team", "department", "group", "org"}
	for _, pattern := range orgPatterns {
		if strings.Contains(textLower, pattern) {
			return EntityTypeOrganization
		}
	}

	// Check for project indicators
	projectPatterns := []string{"project", "initiative", "program", "sprint"}
	for _, pattern := range projectPatterns {
		if strings.Contains(textLower, pattern) {
			return EntityTypeProject
		}
	}

	// Check for location indicators
	locationPatterns := []string{"street", "avenue", "city", "state", "country", "room", "building"}
	for _, pattern := range locationPatterns {
		if strings.Contains(textLower, pattern) {
			return EntityTypeLocation
		}
	}

	// Default to unknown
	return EntityTypeUnknown
}

// inferRelationshipTypeFromPredicate infers relationship type from assertion predicate.
func inferRelationshipTypeFromPredicate(predicate string) RelationshipType {
	predicateLower := strings.ToLower(predicate)

	// Work relationships
	if strings.Contains(predicateLower, "works at") || strings.Contains(predicateLower, "employed by") {
		return RelationshipTypeWorksAt
	}
	if strings.Contains(predicateLower, "works with") || strings.Contains(predicateLower, "collaborates") {
		return RelationshipTypeWorksWith
	}
	if strings.Contains(predicateLower, "reports to") {
		return RelationshipTypeReportsTo
	}
	if strings.Contains(predicateLower, "manages") || strings.Contains(predicateLower, "supervises") {
		return RelationshipTypeManages
	}

	// Knowledge relationships - check more specific "knows about" before general "knows"
	if strings.Contains(predicateLower, "knows about") || strings.Contains(predicateLower, "expert") {
		return RelationshipTypeKnowsAbout
	}
	if strings.Contains(predicateLower, "knows") || strings.Contains(predicateLower, "met") {
		return RelationshipTypeKnows
	}

	// Topic relationships
	if strings.Contains(predicateLower, "discussed") || strings.Contains(predicateLower, "talked about") {
		return RelationshipTypeDiscussed
	}

	// Event relationships
	if strings.Contains(predicateLower, "attended") || strings.Contains(predicateLower, "participated") {
		return RelationshipTypeAttended
	}

	// Membership relationships
	if strings.Contains(predicateLower, "member of") || strings.Contains(predicateLower, "belongs to") {
		return RelationshipTypeMemberOf
	}
	if strings.Contains(predicateLower, "part of") {
		return RelationshipTypePartOf
	}

	// Creation relationships
	if strings.Contains(predicateLower, "created") || strings.Contains(predicateLower, "authored") {
		return RelationshipTypeCreated
	}

	// Location relationships
	if strings.Contains(predicateLower, "located") || strings.Contains(predicateLower, "based in") {
		return RelationshipTypeLocatedAt
	}

	// Default to generic relationship
	return RelationshipTypeRelatedTo
}

// Ensure RelationshipExtractor implements expected interface at compile time.
var _ interface {
	Extract(ctx context.Context, content string) (*ExtractionResult, error)
	ExtractFromEmail(ctx context.Context, email *EmailContent) (*ExtractionResult, error)
	ExtractFromMeeting(ctx context.Context, meeting *MeetingContent) (*ExtractionResult, error)
} = (*RelationshipExtractor)(nil)
