// Package entity provides entity extraction capabilities from content.
// It extracts named entities (people, organizations, locations, dates, etc.)
// from various content types using pattern matching and AI-based NER.
package entity

import (
	"time"
)

// EntityType represents the type of a named entity.
type EntityType string

const (
	// EntityTypePerson represents a person's name.
	EntityTypePerson EntityType = "person"

	// EntityTypeOrganization represents a company, team, or institution.
	EntityTypeOrganization EntityType = "organization"

	// EntityTypeLocation represents a physical location or address.
	EntityTypeLocation EntityType = "location"

	// EntityTypeDate represents a date or time reference.
	EntityTypeDate EntityType = "date"

	// EntityTypeMoney represents a monetary amount.
	EntityTypeMoney EntityType = "money"

	// EntityTypeEmail represents an email address.
	EntityTypeEmail EntityType = "email"

	// EntityTypeURL represents a URL or web address.
	EntityTypeURL EntityType = "url"

	// EntityTypePhone represents a phone number.
	EntityTypePhone EntityType = "phone"

	// EntityTypeEvent represents an event or meeting.
	EntityTypeEvent EntityType = "event"

	// EntityTypeProduct represents a product or service name.
	EntityTypeProduct EntityType = "product"

	// EntityTypeMisc represents a miscellaneous entity type.
	EntityTypeMisc EntityType = "misc"

	// EntityTypeUnknown represents an unclassified entity type.
	EntityTypeUnknown EntityType = "unknown"
)

// String returns the string representation of EntityType.
func (e EntityType) String() string {
	return string(e)
}

// IsValid returns true if the entity type is a recognized type.
func (e EntityType) IsValid() bool {
	switch e {
	case EntityTypePerson, EntityTypeOrganization, EntityTypeLocation,
		EntityTypeDate, EntityTypeMoney, EntityTypeEmail, EntityTypeURL,
		EntityTypePhone, EntityTypeEvent, EntityTypeProduct, EntityTypeMisc:
		return true
	default:
		return false
	}
}

// TextPosition represents the position of text in the original content.
type TextPosition struct {
	// Start is the starting byte offset in the original text.
	Start int

	// End is the ending byte offset in the original text.
	End int

	// Line is the 1-indexed line number (0 if not tracked).
	Line int

	// Column is the 1-indexed column number (0 if not tracked).
	Column int
}

// Length returns the length of the text span.
func (p TextPosition) Length() int {
	return p.End - p.Start
}

// Entity represents a named entity extracted from content.
type Entity struct {
	// ID is the unique identifier for this entity instance.
	ID string

	// Type indicates the category of entity.
	Type EntityType

	// Value is the raw text value as it appears in the content.
	Value string

	// NormalizedValue is the standardized/normalized form of the entity.
	// For dates: ISO8601, for phones: E.164, for names: "First Last".
	NormalizedValue string

	// Confidence is the extraction confidence score (0.0 to 1.0).
	Confidence float32

	// Position tracks where the entity appears in the original text.
	Position TextPosition

	// Metadata contains additional information about the entity.
	Metadata map[string]string

	// Source indicates how this entity was extracted (pattern, ner, hybrid).
	Source ExtractionSource

	// LinkedEntityID is the ID of a linked entity in the knowledge base.
	// Empty if not linked.
	LinkedEntityID string

	// Aliases are alternative names/forms for this entity.
	Aliases []string

	// CreatedAt is when this entity was extracted.
	CreatedAt time.Time
}

// ExtractionSource indicates how an entity was extracted.
type ExtractionSource string

const (
	// ExtractionSourcePattern indicates pattern/regex-based extraction.
	ExtractionSourcePattern ExtractionSource = "pattern"

	// ExtractionSourceNER indicates AI-based named entity recognition.
	ExtractionSourceNER ExtractionSource = "ner"

	// ExtractionSourceHybrid indicates combined pattern + NER extraction.
	ExtractionSourceHybrid ExtractionSource = "hybrid"

	// ExtractionSourceDomain indicates domain-specific extraction rules.
	ExtractionSourceDomain ExtractionSource = "domain"
)

// EntityRelation represents a relationship between two entities found in text.
type EntityRelation struct {
	// ID is the unique identifier for this relation.
	ID string

	// SourceEntity is the source entity in the relation.
	SourceEntity *Entity

	// TargetEntity is the target entity in the relation.
	TargetEntity *Entity

	// RelationType describes the type of relation (e.g., "works_at", "located_in").
	RelationType string

	// Confidence is the confidence score for this relation.
	Confidence float32

	// Evidence is the text that supports this relation.
	Evidence string

	// CreatedAt is when this relation was extracted.
	CreatedAt time.Time
}

// ExtractionOptions configures the entity extraction process.
type ExtractionOptions struct {
	// MinConfidence is the minimum confidence threshold (0.0 to 1.0).
	MinConfidence float32

	// MaxEntities is the maximum number of entities to extract (0 for unlimited).
	MaxEntities int

	// EntityTypes filters extraction to specific entity types.
	// If nil or empty, all types are extracted.
	EntityTypes []EntityType

	// UsePatterns enables pattern-based extraction.
	UsePatterns bool

	// UseNER enables AI-based named entity recognition.
	UseNER bool

	// UseDomain enables domain-specific extraction rules.
	UseDomain bool

	// NormalizeEntities enables normalization of extracted entities.
	NormalizeEntities bool

	// DeduplicateEntities enables deduplication of entities.
	DeduplicateEntities bool

	// TrackPositions enables position tracking in original text.
	TrackPositions bool

	// LinkToKnowledgeBase enables entity linking to knowledge base.
	LinkToKnowledgeBase bool

	// DomainHints provides context for domain-specific extraction.
	DomainHints map[string]string
}

// DefaultExtractionOptions returns sensible default extraction options.
func DefaultExtractionOptions() ExtractionOptions {
	return ExtractionOptions{
		MinConfidence:       0.5,
		MaxEntities:         0, // Unlimited
		EntityTypes:         nil,
		UsePatterns:         true,
		UseNER:              true,
		UseDomain:           false,
		NormalizeEntities:   true,
		DeduplicateEntities: true,
		TrackPositions:      true,
		LinkToKnowledgeBase: false,
		DomainHints:         nil,
	}
}

// ShouldExtractType checks if a given entity type should be extracted.
func (o ExtractionOptions) ShouldExtractType(t EntityType) bool {
	if len(o.EntityTypes) == 0 {
		return true
	}
	for _, et := range o.EntityTypes {
		if et == t {
			return true
		}
	}
	return false
}

// ExtractionResult contains the results of entity extraction.
type ExtractionResult struct {
	// Entities is the list of extracted entities.
	Entities []Entity

	// Relations is the list of extracted entity relations.
	Relations []EntityRelation

	// ProcessingTimeMs is the extraction processing time in milliseconds.
	ProcessingTimeMs int64

	// ModelUsed identifies the AI model used for NER extraction.
	ModelUsed string

	// TotalFound is the total number of entities found before filtering.
	TotalFound int

	// FilteredCount is the number filtered out due to confidence/type filters.
	FilteredCount int

	// DuplicatesRemoved is the number of duplicates removed during deduplication.
	DuplicatesRemoved int

	// SourceText is the original text that was processed (truncated if long).
	SourceText string

	// Errors contains any non-fatal errors encountered during extraction.
	Errors []error
}

// ByType returns entities filtered by the given type.
func (r *ExtractionResult) ByType(t EntityType) []Entity {
	var entities []Entity
	for _, e := range r.Entities {
		if e.Type == t {
			entities = append(entities, e)
		}
	}
	return entities
}

// ByMinConfidence returns entities with confidence at or above the threshold.
func (r *ExtractionResult) ByMinConfidence(threshold float32) []Entity {
	var entities []Entity
	for _, e := range r.Entities {
		if e.Confidence >= threshold {
			entities = append(entities, e)
		}
	}
	return entities
}

// EntityCount returns a map of entity counts by type.
func (r *ExtractionResult) EntityCount() map[EntityType]int {
	counts := make(map[EntityType]int)
	for _, e := range r.Entities {
		counts[e.Type]++
	}
	return counts
}

// HasEntities returns true if any entities were extracted.
func (r *ExtractionResult) HasEntities() bool {
	return len(r.Entities) > 0
}

// HasRelations returns true if any relations were extracted.
func (r *ExtractionResult) HasRelations() bool {
	return len(r.Relations) > 0
}

// KnowledgeBaseLinkRequest represents a request to link an entity to the knowledge base.
type KnowledgeBaseLinkRequest struct {
	// Entity is the entity to link.
	Entity *Entity

	// CandidateIDs are potential matching entity IDs in the knowledge base.
	CandidateIDs []string

	// SearchQuery is a query to find candidates if CandidateIDs is empty.
	SearchQuery string

	// TenantID is the tenant for multi-tenancy.
	TenantID string
}

// KnowledgeBaseLinkResult represents the result of entity linking.
type KnowledgeBaseLinkResult struct {
	// LinkedEntityID is the matched entity ID in the knowledge base.
	LinkedEntityID string

	// Confidence is the confidence of the link.
	Confidence float32

	// Matched indicates if a match was found.
	Matched bool

	// NewEntityCreated indicates if a new entity was created.
	NewEntityCreated bool
}

// EntityLinker is the interface for linking entities to a knowledge base.
type EntityLinker interface {
	// Link attempts to link an entity to the knowledge base.
	Link(entity *Entity, tenantID string) (*KnowledgeBaseLinkResult, error)

	// LinkBatch links multiple entities efficiently.
	LinkBatch(entities []Entity, tenantID string) ([]KnowledgeBaseLinkResult, error)
}

// ChunkEntityMap maps chunk IDs to their extracted entities.
// Used for deduplication across multiple chunks.
type ChunkEntityMap struct {
	// ChunkID -> EntityType -> []Entity
	chunks map[string]map[EntityType][]Entity
}

// NewChunkEntityMap creates a new ChunkEntityMap.
func NewChunkEntityMap() *ChunkEntityMap {
	return &ChunkEntityMap{
		chunks: make(map[string]map[EntityType][]Entity),
	}
}

// Add adds entities for a chunk.
func (m *ChunkEntityMap) Add(chunkID string, entities []Entity) {
	if m.chunks[chunkID] == nil {
		m.chunks[chunkID] = make(map[EntityType][]Entity)
	}
	for _, e := range entities {
		m.chunks[chunkID][e.Type] = append(m.chunks[chunkID][e.Type], e)
	}
}

// GetAll returns all entities across all chunks.
func (m *ChunkEntityMap) GetAll() []Entity {
	var all []Entity
	for _, byType := range m.chunks {
		for _, entities := range byType {
			all = append(all, entities...)
		}
	}
	return all
}

// DeduplicateAcrossChunks deduplicates entities that appear in multiple chunks.
// Returns deduplicated entities and the count of duplicates removed.
func (m *ChunkEntityMap) DeduplicateAcrossChunks() ([]Entity, int) {
	seen := make(map[string]Entity) // normalized value + type -> entity
	duplicates := 0

	for _, byType := range m.chunks {
		for entityType, entities := range byType {
			for _, e := range entities {
				key := string(entityType) + ":" + e.NormalizedValue
				if key == string(entityType)+":" {
					key = string(entityType) + ":" + e.Value
				}

				if existing, exists := seen[key]; exists {
					// Keep the one with higher confidence
					if e.Confidence > existing.Confidence {
						seen[key] = e
					}
					duplicates++
				} else {
					seen[key] = e
				}
			}
		}
	}

	result := make([]Entity, 0, len(seen))
	for _, e := range seen {
		result = append(result, e)
	}

	return result, duplicates
}
