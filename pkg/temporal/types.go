package temporal

import "time"

// EmailProcessingInput is the input for email processing workflows.
type EmailProcessingInput struct {
	TenantID    string    `json:"tenant_id"`
	SourceID    int64     `json:"source_id"`
	// ContentID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
	ContentID   string    `json:"content_id,omitempty"`
	MessageID   string    `json:"message_id"`
	ThreadID    string    `json:"thread_id"`
	FromEmail   string    `json:"from_email"`
	FromName    *string   `json:"from_name,omitempty"`
	Subject     *string   `json:"subject,omitempty"`
	ToEmails    []string  `json:"to_emails"`
	CcEmails    []string  `json:"cc_emails"`
	EmailDate   time.Time `json:"email_date"`
	ContentHash string    `json:"content_hash"`
	JobID       string    `json:"job_id"`
}

// EmailProcessingResult is the result of email processing workflows.
type EmailProcessingResult struct {
	SourceID       int64  `json:"source_id"`
	EmbeddingID    *int64 `json:"embedding_id,omitempty"`
	SummaryID      *int64 `json:"summary_id,omitempty"`
	AssertionCount int    `json:"assertion_count"`
	Status         string `json:"status"` // completed, failed
	Error          string `json:"error,omitempty"`
}

// ContentProcessingInput is the input for content processing workflows.
type ContentProcessingInput struct {
	TenantID   string            `json:"tenant_id"`
	SourceID   string            `json:"source_id"`
	SourceType string            `json:"source_type"`
	Content    string            `json:"content"`
	Metadata   map[string]string `json:"metadata"`
}

// RelationshipDiscoveryInput is the input for relationship discovery workflows.
type RelationshipDiscoveryInput struct {
	TenantID     string   `json:"tenant_id"`
	JobID        string   `json:"job_id"`
	SourceID     string   `json:"source_id,omitempty"`
	EntityIDs    []string `json:"entity_ids,omitempty"`
	DiscoverMode string   `json:"discover_mode"` // incremental, full, entity_focused
	TimeWindow   string   `json:"time_window,omitempty"`
	MaxDepth     int      `json:"max_depth"`
}

// RelationshipDiscoveryResult is the result of relationship discovery workflows.
type RelationshipDiscoveryResult struct {
	TenantID             string                `json:"tenant_id"`
	Status               string                `json:"status"` // completed, failed, cancelled
	Error                string                `json:"error,omitempty"`
	EntitiesProcessed    int                   `json:"entities_processed"`
	SourcesAnalyzed      int                   `json:"sources_analyzed"`
	RelationshipsCreated int                   `json:"relationships_created"`
	RelationshipsUpdated int                   `json:"relationships_updated"`
	TopRelationships     []RelationshipSummary `json:"top_relationships,omitempty"`
	NetworkStats         *NetworkStats         `json:"network_stats,omitempty"`
}

// RelationshipSummary provides a summary of a discovered relationship.
type RelationshipSummary struct {
	FromEntityID  string  `json:"from_entity_id"`
	ToEntityID    string  `json:"to_entity_id"`
	RelationType  string  `json:"relation_type"`
	Confidence    float64 `json:"confidence"`
	EvidenceCount int     `json:"evidence_count"`
}

// NetworkStats contains statistics about the relationship network.
type NetworkStats struct {
	TotalEntities      int     `json:"total_entities"`
	TotalRelationships int     `json:"total_relationships"`
	AvgConnections     float64 `json:"avg_connections"`
	MaxConnections     int     `json:"max_connections"`
	Density            float64 `json:"density"`
}

// SLMPipelineInput is the input for starting the SLM pipeline workflow.
// The workflow fetches content and metadata from the database via FetchSource.
type SLMPipelineInput struct {
	TenantID    string `json:"tenant_id"`
	TenantName  string `json:"tenant_name,omitempty"` // Human-readable tenant name for Langfuse environment; falls back to TenantID if empty
	SourceID    int64  `json:"source_id"`
	ContentID   string `json:"content_id,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	JobID       string `json:"job_id"` // Required for tracing and activity context

	// Per-stage timeout overrides (populated from pipeline_config at dispatch time).
	StageTimeouts   map[string]time.Duration `json:"stage_timeouts,omitempty"`
	StageHeartbeats map[string]time.Duration `json:"stage_heartbeats,omitempty"`
}

// ContentIngestionInput is the input for content ingestion workflows.
type ContentIngestionInput struct {
	TenantID    string `json:"tenant_id"`
	SourceID    int64  `json:"source_id"`
	// ContentID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
	ContentID   string `json:"content_id,omitempty"`
	SourceType  string `json:"source_type"`
	ContentHash string `json:"content_hash"`
	JobID       string `json:"job_id"`
}

// ContentIngestionResult is the result of content ingestion workflows.
type ContentIngestionResult struct {
	SourceID        int64    `json:"source_id"`
	Status          string   `json:"status"` // completed, failed, cancelled
	Error           string   `json:"error,omitempty"`
	EmbeddingID     *int64   `json:"embedding_id,omitempty"`
	SummaryID       *int64   `json:"summary_id,omitempty"`
	EntityCount     int      `json:"entity_count"`
	MentionCount    int      `json:"mention_count"`
}

// DailyReviewInput is the input for daily review workflows.
type DailyReviewInput struct {
	TenantID          string    `json:"tenant_id"`
	UserID            string    `json:"user_id,omitempty"`
	JobID             string    `json:"job_id"`
	ReviewDate        time.Time `json:"review_date"`
	MaxItems          int       `json:"max_items"`
	IncludeEmail      bool      `json:"include_email"`
	IncludeDocuments  bool      `json:"include_documents"`
	IncludeAssertions bool      `json:"include_assertions"`
}

// DailyReviewResult is the result of daily review workflows.
type DailyReviewResult struct {
	TenantID          string           `json:"tenant_id"`
	ReviewID          string           `json:"review_id"`
	ReviewDate        time.Time        `json:"review_date"`
	Status            string           `json:"status"` // completed, failed, cancelled
	Error             string           `json:"error,omitempty"`
	ItemCount         int              `json:"item_count"`
	HighPriorityCount int              `json:"high_priority_count"`
	Categories        []ReviewCategory `json:"categories,omitempty"`
	GeneratedAt       time.Time        `json:"generated_at"`
}

// ReviewCategory represents a category of review items.
type ReviewCategory struct {
	Name        string `json:"name"`
	Count       int    `json:"count"`
	Priority    int    `json:"priority"`
	Description string `json:"description,omitempty"`
}

// WorkflowStatus tracks the status of a running workflow.
type WorkflowStatus struct {
	Stage           string    `json:"stage"`
	StepsCompleted  int       `json:"steps_completed"`
	TotalSteps      int       `json:"total_steps"`
	LastActivity    string    `json:"last_activity"`
	StartedAt       time.Time `json:"started_at"`
	LastUpdated     time.Time `json:"last_updated"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	CompensationRan bool      `json:"compensation_ran"`
}

// PriorityUpdateSignal is sent to update workflow priority.
type PriorityUpdateSignal struct {
	NewPriority int    `json:"new_priority"`
	Reason      string `json:"reason,omitempty"`
}

// PauseResumeSignal is sent to pause or resume a workflow.
type PauseResumeSignal struct {
	Paused bool   `json:"paused"`
	Reason string `json:"reason,omitempty"`
}

// CancelWithCompensationSignal is sent to cancel a workflow with compensation.
type CancelWithCompensationSignal struct {
	Reason string `json:"reason"`
}

// GmailSyncInput is the input for Gmail synchronization workflows.
type GmailSyncInput struct {
	TenantID       string    `json:"tenant_id"`
	UserID         string    `json:"user_id"`
	JobID          string    `json:"job_id"`
	SyncMode       string    `json:"sync_mode"` // incremental, full
	LastSyncToken  string    `json:"last_sync_token,omitempty"`
	MaxMessages    int       `json:"max_messages"`
	LabelFilters   []string  `json:"label_filters,omitempty"`
	StartDate      time.Time `json:"start_date,omitempty"`
	IncludeHeaders bool      `json:"include_headers"`
}

// GmailSyncResult is the result of Gmail synchronization workflows.
type GmailSyncResult struct {
	TenantID       string    `json:"tenant_id"`
	UserID         string    `json:"user_id"`
	Status         string    `json:"status"` // completed, failed, cancelled, partial
	Error          string    `json:"error,omitempty"`
	MessagesSynced int       `json:"messages_synced"`
	ThreadsSynced  int       `json:"threads_synced"`
	NewMessages    int       `json:"new_messages"`
	UpdatedMsgs    int       `json:"updated_messages"`
	DeletedMsgs    int       `json:"deleted_messages"`
	NextSyncToken  string    `json:"next_sync_token,omitempty"`
	SyncedAt       time.Time `json:"synced_at"`
}

// AnalysisInput is the input for content analysis workflows.
type AnalysisInput struct {
	TenantID     string   `json:"tenant_id"`
	JobID        string   `json:"job_id"`
	SourceID     int64    `json:"source_id"`
	SourceType   string   `json:"source_type"`
	Content      string   `json:"content"`
	AnalysisType []string `json:"analysis_types"` // categorize, summarize, extract_entities, sentiment
}

// AnalysisResult is the result of content analysis workflows.
type AnalysisResult struct {
	SourceID       int64             `json:"source_id"`
	Status         string            `json:"status"` // completed, failed, cancelled
	Error          string            `json:"error,omitempty"`
	Categories     []string          `json:"categories,omitempty"`
	SummaryID      *int64            `json:"summary_id,omitempty"`
	Summary        string            `json:"summary,omitempty"`
	Entities       []ExtractedEntity `json:"entities,omitempty"`
	SentimentScore *float64          `json:"sentiment_score,omitempty"`
	Sentiment      string            `json:"sentiment,omitempty"` // positive, negative, neutral
	AnalyzedAt     time.Time         `json:"analyzed_at"`
}

// ExtractedEntity represents an entity extracted from content.
type ExtractedEntity struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"` // person, organization, location, topic, etc.
	Name       string            `json:"name"`
	Confidence float64           `json:"confidence"`
	Mentions   int               `json:"mentions"`
	Attributes map[string]string `json:"attributes,omitempty"`
}
