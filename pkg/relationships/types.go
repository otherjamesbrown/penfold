// Package relationships provides domain types and repository for relationship graph operations.
package relationships

import (
	"time"
)

// EntityType represents the type of an entity in the relationship graph.
type EntityType string

const (
	EntityTypePerson       EntityType = "person"
	EntityTypeOrganization EntityType = "organization"
	EntityTypeTopic        EntityType = "topic"
	EntityTypeProject      EntityType = "project"
	EntityTypeLocation     EntityType = "location"
	EntityTypeProduct      EntityType = "product"
)

// RelationshipType represents the type of relationship between entities.
type RelationshipType string

const (
	RelationshipTypeColleague     RelationshipType = "colleague"
	RelationshipTypeReportsTo     RelationshipType = "reports_to"
	RelationshipTypeMemberOf      RelationshipType = "member_of"
	RelationshipTypeWorksOn       RelationshipType = "works_on"
	RelationshipTypeDiscusses     RelationshipType = "discusses"
	RelationshipTypeMentions      RelationshipType = "mentions"
	RelationshipTypeLocatedAt     RelationshipType = "located_at"
	RelationshipTypeRelatedTo     RelationshipType = "related_to"
	RelationshipTypeCollaborates  RelationshipType = "collaborates_with"
	RelationshipTypeKnows         RelationshipType = "knows"
	RelationshipTypeMentionedWith RelationshipType = "mentioned_with"
)

// RelationshipStatus represents the validation status of a relationship.
type RelationshipStatus string

const (
	RelationshipStatusDiscovered RelationshipStatus = "discovered"
	RelationshipStatusConfirmed  RelationshipStatus = "confirmed"
	RelationshipStatusRejected   RelationshipStatus = "rejected"
	RelationshipStatusArchived   RelationshipStatus = "archived"
)

// ConflictStatus represents the status of a relationship conflict.
type ConflictStatus string

const (
	ConflictStatusPending  ConflictStatus = "pending"
	ConflictStatusResolved ConflictStatus = "resolved"
)

// ConflictResolutionStrategy defines how to resolve relationship conflicts.
type ConflictResolutionStrategy string

const (
	ConflictStrategyKeepLatest ConflictResolutionStrategy = "keep_latest"
	ConflictStrategyKeepFirst  ConflictResolutionStrategy = "keep_first"
	ConflictStrategyMerge      ConflictResolutionStrategy = "merge"
	ConflictStrategyManual     ConflictResolutionStrategy = "manual"
)

// Entity represents a node in the relationship graph.
type Entity struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Type           EntityType        `json:"type"`
	CanonicalName  string            `json:"canonical_name,omitempty"`
	Aliases        []string          `json:"aliases,omitempty"`
	Confidence     float64           `json:"confidence"`
	SourceCount    int               `json:"source_count"`
	FirstSeen      time.Time         `json:"first_seen"`
	LastSeen       time.Time         `json:"last_seen"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	RelationCount  int               `json:"relation_count"`
	SentCount             int               `json:"sent_count"`
	ReceivedCount         int               `json:"received_count"`
	CommunicationPatterns string            `json:"communication_patterns,omitempty"`
	ExpertiseAreas        []string          `json:"expertise_areas,omitempty"`
	OrgPosition           string            `json:"org_position,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// Relationship represents an edge in the relationship graph.
type Relationship struct {
	ID           string             `json:"id"`
	SourceID     string             `json:"source_id"`
	SourceName   string             `json:"source_name"`
	SourceType   EntityType         `json:"source_type"`
	TargetID     string             `json:"target_id"`
	TargetName   string             `json:"target_name"`
	TargetType   EntityType         `json:"target_type"`
	Type         RelationshipType   `json:"type"`
	Status       RelationshipStatus `json:"status"`
	Confidence   float64            `json:"confidence"`
	Weight       float64            `json:"weight"`
	Evidence     []Evidence         `json:"evidence,omitempty"`
	Notes        string             `json:"notes,omitempty"`
	FirstSeen    time.Time          `json:"first_seen"`
	LastSeen     time.Time          `json:"last_seen"`
	SourceCount  int                `json:"source_count"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	TenantID     string             `json:"tenant_id"`
}

// Evidence represents supporting evidence for a discovered relationship.
type Evidence struct {
	SourceID               string    `json:"source_id"`
	SourceType             string    `json:"source_type"`
	Excerpt                string    `json:"excerpt"`
	DiscoveredAt           time.Time `json:"discovered_at"`
	ConfidenceContribution float64   `json:"confidence_contribution"`
}

// RelationshipConflict represents a detected conflict between relationships.
type RelationshipConflict struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	Type            string         `json:"type"` // duplicate_entity, contradictory_relationship, low_confidence
	Description     string         `json:"description"`
	Relationships   []Relationship `json:"relationships"`
	SuggestedAction string         `json:"suggested_action"`
	Status          ConflictStatus `json:"status"`
	Resolution      string         `json:"resolution,omitempty"`
	ResolvedBy      string         `json:"resolved_by,omitempty"`
	ResolvedAt      *time.Time     `json:"resolved_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

// NetworkCluster represents a cluster in the relationship network.
type NetworkCluster struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	EntityCount int      `json:"entity_count"`
	TopEntities []Entity `json:"top_entities"`
	Density     float64  `json:"density"`
}

// NetworkStats represents statistics about the relationship network.
type NetworkStats struct {
	TotalNodes       int                    `json:"total_nodes"`
	TotalEdges       int                    `json:"total_edges"`
	Density          float64                `json:"density"`
	AvgConnections   float64                `json:"avg_connections"`
	ClusterCount     int                    `json:"cluster_count"`
	EntityTypeCounts map[EntityType]int     `json:"entity_type_counts"`
	RelTypeCounts    map[RelationshipType]int `json:"relationship_type_counts"`
}

// ListRelationshipsFilter specifies criteria for listing relationships.
type ListRelationshipsFilter struct {
	TenantID        string
	EntityID        string           // Filter by entity (source or target)
	RelationshipType RelationshipType
	Status          RelationshipStatus
	MinConfidence   float64
	SourceEntityType EntityType
	TargetEntityType EntityType
	Limit           int
	Offset          int
}

// ListEntitiesFilter specifies criteria for listing entities.
type ListEntitiesFilter struct {
	TenantID      string
	EntityType    EntityType
	Search        string  // Search in name/aliases
	MinConfidence float64
	Limit         int
	Offset        int
}

// SearchRelationshipsQuery specifies criteria for searching relationships.
type SearchRelationshipsQuery struct {
	TenantID      string
	Query         string  // Search in entity names
	MinConfidence float64
	Limit         int
}

// ListConflictsFilter specifies criteria for listing conflicts.
type ListConflictsFilter struct {
	TenantID string
	Status   ConflictStatus
	Limit    int
	Offset   int
}

// GraphNode represents an entity in the network graph for visualization.
type GraphNode struct {
	ID         string            `json:"id"`
	Label      string            `json:"label"`
	Type       EntityType        `json:"type"`
	Degree     int               `json:"degree"`
	Properties map[string]string `json:"properties,omitempty"`
}

// GraphEdge represents a relationship in the network graph for visualization.
type GraphEdge struct {
	ID       string           `json:"id"`
	Source   string           `json:"source"`
	Target   string           `json:"target"`
	Type     RelationshipType `json:"type"`
	Weight   float64          `json:"weight"`
	Label    string           `json:"label"`
	Properties map[string]string `json:"properties,omitempty"`
}

// NetworkGraph represents the relationship network for visualization.
type NetworkGraph struct {
	Nodes          []GraphNode `json:"nodes"`
	Edges          []GraphEdge `json:"edges"`
	TotalNodes     int         `json:"total_nodes"`
	TotalEdges     int         `json:"total_edges"`
	Truncated      bool        `json:"truncated"`
	CenterEntityID string      `json:"center_entity_id,omitempty"`
	Depth          int         `json:"depth"`
}

// MergeEntitiesRequest specifies parameters for merging two entities.
type MergeEntitiesRequest struct {
	TenantID        string
	PrimaryEntityID string
	MergedEntityID  string
}

// MergeEntitiesResult contains the result of an entity merge operation.
type MergeEntitiesResult struct {
	PrimaryEntity           *Entity `json:"primary_entity"`
	RelationshipsTransferred int     `json:"relationships_transferred"`
}

// ResolveConflictRequest specifies parameters for resolving a conflict.
type ResolveConflictRequest struct {
	TenantID   string
	ConflictID string
	Strategy   ConflictResolutionStrategy
	ResolvedBy string
	Notes      string
}

// ResolveConflictResult contains the result of a conflict resolution.
type ResolveConflictResult struct {
	Conflict             *RelationshipConflict `json:"conflict"`
	RelationshipsUpdated int                   `json:"relationships_updated"`
}

// InsertRelationshipRequest specifies parameters for manually creating a relationship.
type InsertRelationshipRequest struct {
	TenantID       string
	FromEntityID   string
	FromEntityType EntityType
	ToEntityID     string
	ToEntityType   EntityType
	Type           RelationshipType
	Subtype        string
	Confidence     float64
	UserConfirmed  bool
}

// DuplicatePair represents a pair of potentially duplicate entities.
type DuplicatePair struct {
	EntityID1   string
	EntityID2   string
	EntityName1 string
	EntityName2 string
	Similarity  float64
	Signals     []string // e.g., ["name_match", "domain_match", "shared_sources"]
}

// MergePreview shows what would happen if two entities were merged.
type MergePreview struct {
	MergedEntity              *Entity
	TransferringAliases       []string
	TransferringRelationships []Relationship
	ConflictFields            []string
}

// AutoMergeResult contains the result of an auto-merge operation.
type AutoMergeResult struct {
	MergedCount  int
	MergedPairs  []DuplicatePair
	SkippedPairs []DuplicatePair
}
