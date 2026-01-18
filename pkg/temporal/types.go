package temporal

import "time"

// EmailProcessingInput is the input for email processing workflows.
type EmailProcessingInput struct {
	TenantID    string    `json:"tenant_id"`
	SourceID    int64     `json:"source_id"`
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
	TenantID  string   `json:"tenant_id"`
	SourceID  string   `json:"source_id"`
	EntityIDs []string `json:"entity_ids"`
}
