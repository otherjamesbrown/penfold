// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
)

// SourceRepository defines the interface for source data access.
type SourceRepository interface {
	// GetSource fetches source content by ID.
	GetSource(ctx context.Context, tenantID string, sourceID int64) (*Source, error)

	// UpdateSourceStatus updates the processing status of a source.
	UpdateSourceStatus(ctx context.Context, tenantID string, sourceID int64, status string) error

	// UpdateSourceStatusWithFailure updates the processing status and failure info of a source.
	// If triage metadata is provided as a variadic argument, it is persisted to the ingestion_metadata JSONB column.
	UpdateSourceStatusWithFailure(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error
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

	// StoreMultiLevelEmbedding stores an embedding with entity type and representation type.
	StoreMultiLevelEmbedding(ctx context.Context, input *MultiLevelEmbeddingInput) (int64, error)

	// GetEmbeddingsForSource returns all embeddings for a source, optionally filtered by representation type.
	// Pass empty string for representationType to get all embeddings.
	GetEmbeddingsForSource(ctx context.Context, tenantID string, sourceID int64, representationType string) ([]*Embedding, error)

	// GetStaleEmbeddings returns source IDs with embeddings from an older model/version.
	GetStaleEmbeddings(ctx context.Context, tenantID string, currentModel string, currentVersion string, limit int) ([]int64, error)

	// DeleteEmbeddingsForSource deletes all embeddings for a source (for re-embedding).
	DeleteEmbeddingsForSource(ctx context.Context, tenantID string, sourceID int64) error

	// DeleteEmbedding deletes a single embedding by ID (for saga compensation).
	DeleteEmbedding(ctx context.Context, embeddingID int64) error
}

// MultiLevelEmbeddingInput contains all fields for storing a multi-level embedding.
type MultiLevelEmbeddingInput struct {
	TenantID           string
	SourceID           int64
	EntityType         string  // "source" or "assertion"
	EntityID           int64   // source_id or assertion_id
	RepresentationType string  // "content", "summary", "action_item", "decision", "risk"
	Vector             []float32
	Model              string
	ModelVersion       string
	TextContent        string // The text that was embedded
	ContentHash        string // SHA-256 for deduplication
	ChunkIndex         int    // 0-indexed position of this chunk
	ChunkTotal         int    // Total number of chunks for this content
}

// Embedding represents a stored embedding vector.
type Embedding struct {
	ID                 int64
	SourceID           int64
	TenantID           string
	EntityType         string
	EntityID           int64
	RepresentationType string
	Vector             []float32
	Model              string
	ModelVersion       string
	Dimensions         int32
}

// SummaryRepository defines the interface for summary data access.
type SummaryRepository interface {
	// StoreSummary stores a generated summary for a source.
	StoreSummary(ctx context.Context, tenantID string, sourceID int64, summary string, keyPoints []string, model string) (int64, error)

	// DeleteSummary deletes a summary by source ID (for saga compensation).
	DeleteSummary(ctx context.Context, sourceID int64) error
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

// Attribution represents a person's relationship to an assertion.
type Attribution struct {
	EntityID string // Person canonical name or email
	Role     string // owner, assignee, decision_maker, mentioned
}

// Assertion represents an extracted assertion (subject-predicate-object triple).
type Assertion struct {
	Subject      string
	Predicate    string
	Object       string
	Confidence   float32
	SourceText   string
	Category     string
	Attributions []Attribution // People associated with this assertion
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

	// ExtractEntities performs two-pass entity extraction from content.
	ExtractEntities(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error)

	// TriageContent classifies content into categories and importance levels (Stage 1).
	TriageContent(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error)

	// DeepAnalyze performs Stage 4 deep analysis using a remote LLM.
	DeepAnalyze(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error)
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

// ContextPackageRepository provides data access for Stage 3 context assembly.
type ContextPackageRepository interface {
	// GetActiveRisks returns active risks/issues for the given projects, ordered by severity.
	GetActiveRisks(ctx context.Context, projectIDs []int64, limit int) ([]ContextAssertion, error)
	// GetOpenActions returns open action items for the given projects, ordered by due date.
	GetOpenActions(ctx context.Context, projectIDs []int64, limit int) ([]ContextAssertion, error)
	// GetRecentDecisions returns recent decisions for the given projects.
	GetRecentDecisions(ctx context.Context, projectIDs []int64, days int, limit int) ([]ContextAssertion, error)
	// GetProductEvents returns recent product timeline events.
	GetProductEvents(ctx context.Context, productIDs []int64, days int, limit int) ([]ContextProductEvent, error)
	// GetGlossaryTerms returns glossary expansions for the given terms.
	GetGlossaryTerms(ctx context.Context, terms []string, productIDs []int64, limit int) ([]ContextGlossaryTerm, error)
	// ResolveProjectByName resolves a project/product name to an ID.
	ResolveProjectByName(ctx context.Context, tenantID string, name string) (*int64, error)
	// ResolveProjectByKeyword resolves a project by keyword match.
	ResolveProjectByKeyword(ctx context.Context, tenantID string, keyword string) (*int64, error)
}

// ContextAssertion is a simplified assertion for context packages.
type ContextAssertion struct {
	Description   string
	Severity      string // For risks
	Status        string // For actions
	DueDate       *time.Time // For actions
	OwnerName     string
	AssigneeName  string
	DecisionMaker string // For decisions
	Rationale     string // For decisions
	ProjectName   string
	SourceQuote   string
}

// ContextProductEvent is a product timeline event for context packages.
type ContextProductEvent struct {
	Title       string
	Description string
	EventType   string
	OccurredAt  time.Time
}

// ContextGlossaryTerm is a glossary term for context packages.
type ContextGlossaryTerm struct {
	Term       string
	Expansion  string
	Definition string
}

// PersistRepository defines the interface for Stage 4.5 persistence operations.
type PersistRepository interface {
	// PersistFindings validates and writes all findings from Stage 4 output in a single transaction.
	// Returns the IDs of created/updated assertions and any compensation function for rollback.
	PersistFindings(ctx context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error)
}

// PersistFindingsInput is the input for persisting Stage 4 analysis findings.
type PersistFindingsInput struct {
	TenantID   string
	SourceID   int64
	ThreadID   *int64
	ProjectID  *int64
	Analysis   *DeepAnalyzeOutput // From Stage 4
	// Resolved person IDs from Stage 3
	ResolvedPeople map[string]int64 // name -> person_id
	// Original email content for acronym detection
	BodyText string
	Subject  string
}

// PersistFindingsOutput is the output from persisting findings.
type PersistFindingsOutput struct {
	AssertionsCreated      int
	AssertionsSuperseded   int
	AssertionsDeduplicated int // Cross-thread dedup: skipped because identical quote exists in earlier thread message
	ReferencesCreated      int
	ReviewItemsCreated     int
	AffinityUpdates        int
	// For saga compensation
	CreatedAssertionIDs []int64
	CreatedReferenceIDs []int64
	CreatedReviewIDs    []int64
}

// PipelineRepository defines the interface for pipeline run recording.
type PipelineRepository interface {
	CreateRun(ctx context.Context, input PipelineRunInput) error
	RecordOverrides(ctx context.Context, runID int64, overrides map[string]string) error
}

// EnrichmentRepository defines the interface for enrichment data access.
type EnrichmentRepository interface {
	// GetBySourceID retrieves an enrichment by source ID.
	GetBySourceID(ctx context.Context, sourceID int64) (*EnrichmentRecord, error)
	// Update updates an existing enrichment record.
	Update(ctx context.Context, e *EnrichmentRecord) error
	// Create inserts a new enrichment record.
	// Note: This uses enrichment.Enrichment from pkg/enrichment, not EnrichmentRecord.
	// The activity will need to import "github.com/otherjamesbrown/penfold/pkg/enrichment"
	// and pass enrichment.Enrichment to this method.
	Create(ctx context.Context, e interface{}) error
}

// EnrichmentRecord represents an enrichment record for content subtype updates.
type EnrichmentRecord struct {
	ID                   int64
	SourceID             int64
	TenantID             string
	ContentSubtype       string
	ClassificationReason string
}

// PipelineRunInput contains the data needed to record a pipeline run.
type PipelineRunInput struct {
	SourceID        int64
	Stage           string
	ModelID         string
	PromptVersion   int
	ConfigHash      string
	Status          string
	DurationMS      int
	InputData       json.RawMessage // Prompt/input sent to stage
	OutputData      json.RawMessage // Raw response from stage
	ParsedData      json.RawMessage // Parsed/structured output
	InputTokens     int             // LLM input token count, 0 for code-only stages
	OutputTokens    int             // LLM output token count, 0 for code-only stages
	SkipReason      string          // e.g. "contribution_gating:LOW", empty if not skipped
	LangfuseTraceID string          // trace ID for Langfuse drill-through
}

// ThreadRepository defines the interface for email thread data access.
type ThreadRepository interface {
	// UpsertThread creates or updates an email thread, returning the thread ID.
	UpsertThread(ctx context.Context, input *UpsertThreadInput) (int64, error)

	// AddThreadMessage adds a message to a thread with the correct position.
	AddThreadMessage(ctx context.Context, input *AddThreadMessageInput) error

	// SetContentEnrichmentThreadID updates the thread_id column in content_enrichment.
	SetContentEnrichmentThreadID(ctx context.Context, sourceID int64, threadID string) error

	// GetThreadByRootMessageID retrieves a thread by its root message ID.
	GetThreadByRootMessageID(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error)

	// GetThreadByMemberMessageID retrieves the thread containing a given message ID
	// (looks up thread_messages.message_id, not just the root). Returns nil if not found.
	GetThreadByMemberMessageID(ctx context.Context, tenantID, messageID string) (*EmailThread, error)
}

// UpsertThreadInput contains data for creating or updating an email thread.
type UpsertThreadInput struct {
	TenantID          string
	RootMessageID     string
	NormalizedSubject string
	LatestSourceID    int64
	FirstMessageAt    time.Time
	LastMessageAt     time.Time
}

// AddThreadMessageInput contains data for adding a message to a thread.
type AddThreadMessageInput struct {
	ThreadID         int64
	SourceID         int64
	MessageID        string
	PositionInThread int
	IsReply          bool
	ReplyToMessageID string
	MessageDate      time.Time
}

// EmailThread represents an email thread from the database.
type EmailThread struct {
	ID            int64
	RootMessageID string
	Subject       string
	MessageCount  int
}

// GroupEmailThreadInput is the input for the GroupEmailThread activity.
type GroupEmailThreadInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
}

// GroupEmailThreadOutput is the output from the GroupEmailThread activity.
type GroupEmailThreadOutput struct {
	ThreadID      *string `json:"thread_id,omitempty"`       // Root message ID (nil if not threaded)
	EmailThreadID *int64  `json:"email_thread_id,omitempty"` // Numeric email_threads.id for assertion dedup
}

// ConversationItem represents a content item in a conversation.
type ConversationItem struct {
	ConversationID string
	ContentID      string
	SourceID       *int64
	AddedAt        time.Time
	TenantID       string
}

// ConversationRepository defines the interface for conversation data access.
type ConversationRepository interface {
	// UpsertConversation creates or updates a conversation.
	// If thread_key matches an existing conversation, updates it and returns the existing ID.
	// Otherwise, creates a new conversation and returns the new ID.
	UpsertConversation(ctx context.Context, conversation *Conversation) (string, error)

	// AddConversationItem adds a content item to a conversation.
	// Idempotent: duplicate (conversation_id, content_id) pairs are ignored.
	AddConversationItem(ctx context.Context, conversationID, contentID string, sourceID *int64, tenantID string) error

	// AddConversationParticipant adds a participant to a conversation.
	// Idempotent: duplicate participants are ignored.
	AddConversationParticipant(ctx context.Context, conversationID string, name, address *string, tenantID string) error

	// CleanInvalidParticipants removes participants with obviously-invalid addresses
	// (JSON artifacts from old comma-split parser). Returns count of deleted rows.
	CleanInvalidParticipants(ctx context.Context, conversationID, tenantID string) (int64, error)

	// UpdateConversationStats recalculates counts and timestamps for a conversation.
	UpdateConversationStats(ctx context.Context, conversationID string) error

	// UpdateSummary updates the rolling summary for a conversation.
	UpdateSummary(ctx context.Context, conversationID, summary string, version int32) error

	// UpdateState updates the state of a conversation and logs to history.
	UpdateState(ctx context.Context, conversationID, state, reason string) error

	// GetConversationItems returns the most recent items for a conversation.
	GetConversationItems(ctx context.Context, conversationID string, limit int) ([]ConversationItem, error)

	// GetConversation returns a conversation by ID.
	GetConversation(ctx context.Context, tenantID, conversationID string) (*Conversation, error)
}
