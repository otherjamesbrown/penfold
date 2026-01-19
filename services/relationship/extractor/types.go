// Package extractor provides relationship extraction capabilities from content.
// It extracts relationships between entities (people, organizations, topics, etc.)
// from various content types including emails, meetings, and documents.
package extractor

import (
	"time"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
)

// EntityType represents the type of an entity that can participate in relationships.
type EntityType string

const (
	// EntityTypePerson represents an individual human entity.
	EntityTypePerson EntityType = "person"

	// EntityTypeOrganization represents a company, team, or department.
	EntityTypeOrganization EntityType = "organization"

	// EntityTypeTopic represents a subject area or theme.
	EntityTypeTopic EntityType = "topic"

	// EntityTypeProject represents a project or initiative.
	EntityTypeProject EntityType = "project"

	// EntityTypeLocation represents a physical or virtual location.
	EntityTypeLocation EntityType = "location"

	// EntityTypeEvent represents a meeting, conference, or other event.
	EntityTypeEvent EntityType = "event"

	// EntityTypeDocument represents a document or artifact.
	EntityTypeDocument EntityType = "document"

	// EntityTypeUnknown represents an unclassified entity.
	EntityTypeUnknown EntityType = "unknown"
)

// ToProto converts EntityType to its protobuf representation.
func (e EntityType) ToProto() relationshipv1.EntityType {
	switch e {
	case EntityTypePerson:
		return relationshipv1.EntityType_ENTITY_TYPE_PERSON
	case EntityTypeOrganization:
		return relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION
	case EntityTypeTopic:
		return relationshipv1.EntityType_ENTITY_TYPE_TOPIC
	case EntityTypeProject:
		return relationshipv1.EntityType_ENTITY_TYPE_PROJECT
	case EntityTypeLocation:
		return relationshipv1.EntityType_ENTITY_TYPE_LOCATION
	case EntityTypeEvent:
		return relationshipv1.EntityType_ENTITY_TYPE_EVENT
	case EntityTypeDocument:
		return relationshipv1.EntityType_ENTITY_TYPE_DOCUMENT
	default:
		return relationshipv1.EntityType_ENTITY_TYPE_UNSPECIFIED
	}
}

// EntityTypeFromProto converts a protobuf EntityType to its domain representation.
func EntityTypeFromProto(e relationshipv1.EntityType) EntityType {
	switch e {
	case relationshipv1.EntityType_ENTITY_TYPE_PERSON:
		return EntityTypePerson
	case relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION:
		return EntityTypeOrganization
	case relationshipv1.EntityType_ENTITY_TYPE_TOPIC:
		return EntityTypeTopic
	case relationshipv1.EntityType_ENTITY_TYPE_PROJECT:
		return EntityTypeProject
	case relationshipv1.EntityType_ENTITY_TYPE_LOCATION:
		return EntityTypeLocation
	case relationshipv1.EntityType_ENTITY_TYPE_EVENT:
		return EntityTypeEvent
	case relationshipv1.EntityType_ENTITY_TYPE_DOCUMENT:
		return EntityTypeDocument
	default:
		return EntityTypeUnknown
	}
}

// RelationshipType represents the type of relationship between two entities.
type RelationshipType string

const (
	// RelationshipTypeKnows indicates that one person knows another.
	RelationshipTypeKnows RelationshipType = "knows"

	// RelationshipTypeWorksAt indicates a person works at an organization.
	RelationshipTypeWorksAt RelationshipType = "works_at"

	// RelationshipTypeWorksWith indicates entities collaborate together.
	RelationshipTypeWorksWith RelationshipType = "works_with"

	// RelationshipTypeReportsTo indicates a reporting relationship.
	RelationshipTypeReportsTo RelationshipType = "reports_to"

	// RelationshipTypeManages indicates a management relationship.
	RelationshipTypeManages RelationshipType = "manages"

	// RelationshipTypeKnowsAbout indicates knowledge of a topic.
	RelationshipTypeKnowsAbout RelationshipType = "knows_about"

	// RelationshipTypeMentionedWith indicates co-mention in content.
	RelationshipTypeMentionedWith RelationshipType = "mentioned_with"

	// RelationshipTypeDiscussed indicates a discussion about a topic.
	RelationshipTypeDiscussed RelationshipType = "discussed"

	// RelationshipTypeAttended indicates attendance at an event.
	RelationshipTypeAttended RelationshipType = "attended"

	// RelationshipTypeCollaboratesWith indicates collaboration.
	RelationshipTypeCollaboratesWith RelationshipType = "collaborates_with"

	// RelationshipTypeMemberOf indicates membership in a group or organization.
	RelationshipTypeMemberOf RelationshipType = "member_of"

	// RelationshipTypePartOf indicates a part-of relationship.
	RelationshipTypePartOf RelationshipType = "part_of"

	// RelationshipTypeCreated indicates creation of something.
	RelationshipTypeCreated RelationshipType = "created"

	// RelationshipTypeLocatedAt indicates location.
	RelationshipTypeLocatedAt RelationshipType = "located_at"

	// RelationshipTypeRelatedTo indicates a generic relationship.
	RelationshipTypeRelatedTo RelationshipType = "related_to"
)

// ToProto converts RelationshipType to its protobuf representation.
func (r RelationshipType) ToProto() relationshipv1.RelationshipType {
	switch r {
	case RelationshipTypeKnows:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS
	case RelationshipTypeWorksAt:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT
	case RelationshipTypeWorksWith, RelationshipTypeCollaboratesWith:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_COLLABORATES_WITH
	case RelationshipTypeReportsTo:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO
	case RelationshipTypeManages:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MANAGES
	case RelationshipTypeKnowsAbout, RelationshipTypeDiscussed:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_DISCUSSED
	case RelationshipTypeMentionedWith:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MENTIONED_WITH
	case RelationshipTypeAttended:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_ATTENDED
	case RelationshipTypeMemberOf:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MEMBER_OF
	case RelationshipTypePartOf:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_PART_OF
	case RelationshipTypeCreated:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_CREATED
	case RelationshipTypeLocatedAt:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_LOCATED_AT
	case RelationshipTypeRelatedTo:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_RELATED_TO
	default:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_UNSPECIFIED
	}
}

// RelationshipTypeFromProto converts a protobuf RelationshipType to its domain representation.
func RelationshipTypeFromProto(r relationshipv1.RelationshipType) RelationshipType {
	switch r {
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS:
		return RelationshipTypeKnows
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT:
		return RelationshipTypeWorksAt
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_COLLABORATES_WITH:
		return RelationshipTypeCollaboratesWith
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO:
		return RelationshipTypeReportsTo
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MANAGES:
		return RelationshipTypeManages
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_DISCUSSED:
		return RelationshipTypeDiscussed
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MENTIONED_WITH:
		return RelationshipTypeMentionedWith
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_ATTENDED:
		return RelationshipTypeAttended
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MEMBER_OF:
		return RelationshipTypeMemberOf
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_PART_OF:
		return RelationshipTypePartOf
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_CREATED:
		return RelationshipTypeCreated
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_LOCATED_AT:
		return RelationshipTypeLocatedAt
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_RELATED_TO:
		return RelationshipTypeRelatedTo
	default:
		return RelationshipTypeRelatedTo
	}
}

// Entity represents a named entity that can participate in relationships.
type Entity struct {
	// ID is the unique identifier for the entity.
	ID string

	// Type indicates the category of entity.
	Type EntityType

	// Name is the display name of the entity.
	Name string

	// CanonicalName is the normalized/deduplicated name.
	CanonicalName string

	// Metadata contains additional information about the entity.
	Metadata map[string]string

	// Confidence is the extraction confidence score (0.0 to 1.0).
	Confidence float32
}

// ToProto converts Entity to its protobuf representation.
func (e *Entity) ToProto() *relationshipv1.Entity {
	return &relationshipv1.Entity{
		Id:            e.ID,
		Name:          e.Name,
		Type:          e.Type.ToProto(),
		CanonicalName: &e.CanonicalName,
		Metadata:      e.Metadata,
	}
}

// Evidence represents supporting evidence for a discovered relationship.
type Evidence struct {
	// SourceID identifies where the relationship was found.
	SourceID string

	// SourceType indicates the type of source content (email, meeting, document).
	SourceType string

	// Excerpt is the text excerpt supporting the relationship.
	Excerpt string

	// DiscoveredAt is when the evidence was discovered.
	DiscoveredAt time.Time

	// ConfidenceContribution is how much this evidence contributes to confidence.
	ConfidenceContribution float32
}

// ToProto converts Evidence to its protobuf representation.
func (e *Evidence) ToProto() *relationshipv1.Evidence {
	return &relationshipv1.Evidence{
		SourceId:               e.SourceID,
		SourceType:             e.SourceType,
		Excerpt:                e.Excerpt,
		ConfidenceContribution: e.ConfidenceContribution,
	}
}

// Relationship represents a discovered connection between two entities.
type Relationship struct {
	// ID is the unique identifier for the relationship.
	ID string

	// Source is the source entity in the relationship.
	Source *Entity

	// Target is the target entity in the relationship.
	Target *Entity

	// Type indicates the kind of relationship.
	Type RelationshipType

	// Strength is the relationship strength/weight (0.0 to 1.0).
	Strength float32

	// Confidence is the extraction confidence score (0.0 to 1.0).
	Confidence float32

	// Context provides additional context about the relationship.
	Context string

	// Evidence contains supporting evidence for this relationship.
	Evidence []Evidence

	// TenantID is the tenant identifier for multi-tenancy.
	TenantID string

	// CreatedAt is when the relationship was discovered.
	CreatedAt time.Time

	// UpdatedAt is when the relationship was last updated.
	UpdatedAt time.Time
}

// ToProto converts Relationship to its protobuf representation.
func (r *Relationship) ToProto() *relationshipv1.Relationship {
	evidence := make([]*relationshipv1.Evidence, len(r.Evidence))
	for i, e := range r.Evidence {
		evidence[i] = e.ToProto()
	}

	rel := &relationshipv1.Relationship{
		Id:               r.ID,
		SourceEntity:     r.Source.ToProto(),
		TargetEntity:     r.Target.ToProto(),
		RelationshipType: r.Type.ToProto(),
		Confidence:       r.Confidence,
		Evidence:         evidence,
		Status:           relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED,
		TenantId:         r.TenantID,
	}

	if r.Context != "" {
		rel.Notes = &r.Context
	}

	return rel
}

// EmailContent represents parsed email content for relationship extraction.
type EmailContent struct {
	// MessageID is the unique identifier for the email.
	MessageID string

	// From is the sender's email address.
	From string

	// FromName is the sender's display name.
	FromName string

	// To is the list of recipient email addresses.
	To []string

	// ToNames is the list of recipient display names.
	ToNames []string

	// CC is the list of CC recipient email addresses.
	CC []string

	// CCNames is the list of CC recipient display names.
	CCNames []string

	// Subject is the email subject line.
	Subject string

	// Body is the email body text.
	Body string

	// ThreadID identifies the email thread.
	ThreadID string

	// Timestamp is when the email was sent.
	Timestamp time.Time
}

// MeetingContent represents parsed meeting content for relationship extraction.
type MeetingContent struct {
	// MeetingID is the unique identifier for the meeting.
	MeetingID string

	// Title is the meeting title.
	Title string

	// Organizer is the meeting organizer's email.
	Organizer string

	// OrganizerName is the organizer's display name.
	OrganizerName string

	// Attendees is the list of attendee email addresses.
	Attendees []string

	// AttendeeNames is the list of attendee display names.
	AttendeeNames []string

	// Description is the meeting description.
	Description string

	// Notes is the meeting notes or transcript.
	Notes string

	// StartTime is when the meeting started.
	StartTime time.Time

	// EndTime is when the meeting ended.
	EndTime time.Time

	// Location is the meeting location.
	Location string
}

// ExtractionOptions configures the relationship extraction process.
type ExtractionOptions struct {
	// MinConfidence is the minimum confidence threshold (0.0 to 1.0).
	MinConfidence float32

	// MaxRelationships is the maximum number of relationships to extract.
	MaxRelationships int

	// EntityTypes filters extraction to specific entity types.
	EntityTypes []EntityType

	// RelationshipTypes filters extraction to specific relationship types.
	RelationshipTypes []RelationshipType

	// UseAI enables AI-powered relationship inference.
	UseAI bool
}

// DefaultExtractionOptions returns sensible default extraction options.
func DefaultExtractionOptions() ExtractionOptions {
	return ExtractionOptions{
		MinConfidence:    0.5,
		MaxRelationships: 100,
		EntityTypes:      nil, // All types
		RelationshipTypes: nil, // All types
		UseAI:            true,
	}
}

// ExtractionResult contains the results of relationship extraction.
type ExtractionResult struct {
	// Relationships is the list of extracted relationships.
	Relationships []Relationship

	// Entities is the list of extracted entities.
	Entities []Entity

	// ProcessingTimeMs is the extraction processing time in milliseconds.
	ProcessingTimeMs int64

	// ModelUsed identifies the AI model used for extraction.
	ModelUsed string

	// TotalFound is the total number of relationships found before filtering.
	TotalFound int

	// FilteredCount is the number filtered out due to confidence threshold.
	FilteredCount int
}
