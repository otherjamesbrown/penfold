// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
)

// SourceRepository defines the interface for source data access.
type SourceRepository interface {
	// GetSource fetches source content by ID.
	GetSource(ctx context.Context, tenantID string, sourceID int64) (*Source, error)

	// UpdateSourceStatus updates the processing status of a source.
	UpdateSourceStatus(ctx context.Context, tenantID string, sourceID int64, status string) error
}

// Source represents a content source record from the database.
type Source struct {
	ID          int64
	TenantID    string
	ContentText string
	ContentType string
	ContentHash string
	Status      string
	Metadata    map[string]string
}

// EmbeddingRepository defines the interface for embedding data access.
type EmbeddingRepository interface {
	// StoreEmbedding stores an embedding vector for a source.
	StoreEmbedding(ctx context.Context, tenantID string, sourceID int64, vector []float32, model string, dimensions int32) (int64, error)

	// GetEmbedding fetches an embedding by ID.
	GetEmbedding(ctx context.Context, tenantID string, embeddingID int64) (*Embedding, error)
}

// Embedding represents a stored embedding vector.
type Embedding struct {
	ID         int64
	SourceID   int64
	TenantID   string
	Vector     []float32
	Model      string
	Dimensions int32
}

// SummaryRepository defines the interface for summary data access.
type SummaryRepository interface {
	// StoreSummary stores a generated summary for a source.
	StoreSummary(ctx context.Context, tenantID string, sourceID int64, summary string, keyPoints []string, model string) (int64, error)
}

// Summary represents a stored summary.
type Summary struct {
	ID        int64
	SourceID  int64
	TenantID  string
	Summary   string
	KeyPoints []string
	Model     string
}

// AssertionRepository defines the interface for assertion data access.
type AssertionRepository interface {
	// StoreAssertions stores extracted assertions for a source.
	StoreAssertions(ctx context.Context, tenantID string, sourceID int64, assertions []*Assertion, model string) (int, error)
}

// Assertion represents an extracted assertion (subject-predicate-object triple).
type Assertion struct {
	Subject    string
	Predicate  string
	Object     string
	Confidence float32
	SourceText string
	Category   string
}

// EntityRepository defines the interface for entity data access.
type EntityRepository interface {
	// StoreEntities stores extracted entities for a source.
	StoreEntities(ctx context.Context, tenantID string, sourceID int64, entities []*Entity) (int, error)
}

// Entity represents an extracted named entity.
type Entity struct {
	Name       string
	Type       string // person, organization, location, etc.
	Confidence float32
	StartPos   int
	EndPos     int
}

// ContentRepository defines the interface for content data access.
type ContentRepository interface {
	// StoreContent stores processed content.
	StoreContent(ctx context.Context, content *ContentRecord) (int64, error)

	// GetContent retrieves content by ID.
	GetContent(ctx context.Context, tenantID string, contentID int64) (*ContentRecord, error)

	// UpdateContent updates an existing content record.
	UpdateContent(ctx context.Context, content *ContentRecord) error
}

// ContentRecord represents a content item in the database.
type ContentRecord struct {
	ID               int64
	TenantID         string
	SourceID         int64
	SourceType       string
	RawContent       string
	ProcessedContent string
	ContentHash      string
	Status           string
	Metadata         map[string]string
}

// RelationshipRepository defines the interface for relationship data access.
type RelationshipRepository interface {
	// StoreRelationship stores a relationship between entities.
	StoreRelationship(ctx context.Context, rel *Relationship) (int64, error)

	// GetRelationships retrieves relationships for an entity.
	GetRelationships(ctx context.Context, tenantID string, entityID string) ([]*Relationship, error)
}

// Relationship represents a relationship between entities.
type Relationship struct {
	ID             int64
	TenantID       string
	SourceEntityID string
	TargetEntityID string
	RelationType   string
	Confidence     float32
	SourceID       int64
	Metadata       map[string]string
}

// AIClient defines the interface for AI service operations.
type AIClient interface {
	// GenerateEmbedding generates a vector embedding for text.
	GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error)

	// GenerateSummary generates a summary for content.
	GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error)

	// ExtractAssertions extracts assertions from content.
	ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error)
}

// NotificationClient defines the interface for sending notifications.
type NotificationClient interface {
	// SendNotification sends a notification.
	SendNotification(ctx context.Context, notification *Notification) error
}

// Notification represents a notification to be sent.
type Notification struct {
	TenantID    string
	Type        string // email, slack, webhook
	Recipient   string
	Subject     string
	Body        string
	Priority    string // low, normal, high
	Metadata    map[string]string
}

// NotificationResult represents the result of sending a notification.
type NotificationResult struct {
	Success   bool
	MessageID string
	Error     string
}
