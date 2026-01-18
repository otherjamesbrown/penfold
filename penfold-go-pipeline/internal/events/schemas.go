// Package events provides event type definitions and infrastructure for the Penfold pipeline.
package events

import (
	"encoding/json"
	"time"
)

// BaseEvent contains common fields for all events.
// This matches the Python BaseEvent schema exactly.
type BaseEvent struct {
	EventType     string    `json:"event_type"`
	Timestamp     time.Time `json:"timestamp"`
	CorrelationID *string   `json:"correlation_id,omitempty"`
	Source        string    `json:"source"`
	Version       string    `json:"version"`
}

// NewBaseEvent creates a BaseEvent with sensible defaults.
func NewBaseEvent(eventType string) BaseEvent {
	return BaseEvent{
		EventType: eventType,
		Timestamp: time.Now().UTC(),
		Source:    "penfold",
		Version:   "1.0",
	}
}

// ManualEmailIngestedEvent is published when an email is successfully ingested from .eml file.
// Triggers the AI processing pipeline (embeddings, assertions, entity resolution).
// Event type: "manual_email.ingested"
type ManualEmailIngestedEvent struct {
	BaseEvent

	// Identifiers
	SourceID  int64  `json:"source_id"`
	TenantID  string `json:"tenant_id"`
	MessageID string `json:"message_id"`
	JobID     string `json:"job_id"`

	// Email metadata
	FromEmail string   `json:"from_email"`
	FromName  *string  `json:"from_name,omitempty"`
	ToEmails  []string `json:"to_emails"`
	CcEmails  []string `json:"cc_emails"`
	Subject   *string  `json:"subject,omitempty"`
	EmailDate time.Time `json:"email_date"`
	DateIsFallback bool `json:"date_is_fallback"`

	// Threading
	InReplyTo *string `json:"in_reply_to,omitempty"`
	ThreadID  *string `json:"thread_id,omitempty"`

	// Content info
	HasAttachments  bool   `json:"has_attachments"`
	AttachmentCount int    `json:"attachment_count"`
	ContentHash     string `json:"content_hash"`

	// Source tracking
	SourceTag        string   `json:"source_tag"`
	OriginalFilePath string   `json:"original_file_path"`
	Labels           []string `json:"labels"`
}

// ContentIngestedEvent is published when content (email, document) is ingested and ready for processing.
// Event type: "content.ingested"
type ContentIngestedEvent struct {
	BaseEvent

	// Email identification
	GmailMessageID  string  `json:"gmail_message_id"`
	GmailThreadID   string  `json:"gmail_thread_id"`
	ConnectionID    int     `json:"connection_id"`
	MessageIDHeader *string `json:"message_id_header,omitempty"`

	// Enhanced participant information
	Participants           map[string]interface{} `json:"participants"`
	UniqueParticipantCount int                    `json:"unique_participant_count"`
	HasExternalParticipants bool                  `json:"has_external_participants"`

	// Enhanced subject and threading
	Subject           *string `json:"subject,omitempty"`
	NormalizedSubject *string `json:"normalized_subject,omitempty"`
	SubjectHash       string  `json:"subject_hash"`

	// Legacy participant fields (for backward compatibility)
	FromEmail string   `json:"from_email"`
	FromName  *string  `json:"from_name,omitempty"`
	ToEmails  []string `json:"to_emails"`
	CcEmails  []string `json:"cc_emails,omitempty"`
	BccEmails []string `json:"bcc_emails,omitempty"`

	// Thread relationship information
	ThreadInfo      map[string]interface{} `json:"thread_info"`
	IsRootMessage   bool                   `json:"is_root_message"`
	ReplyDepth      int                    `json:"reply_depth"`
	ParentMessageID *string                `json:"parent_message_id,omitempty"`
	References      []string               `json:"references"`

	// Enhanced content information
	ContentStructure map[string]interface{} `json:"content_structure"`
	MessageFormat    string                 `json:"message_format"`
	HasBodyText      bool                   `json:"has_body_text"`
	HasBodyHTML      bool                   `json:"has_body_html"`
	HasAttachments   bool                   `json:"has_attachments"`
	AttachmentCount  int                    `json:"attachment_count"`
	ContentTypes     []string               `json:"content_types"`

	// Enhanced priority and importance
	PriorityInfo      map[string]interface{} `json:"priority_info"`
	ImportanceLevel   string                 `json:"importance_level"`
	PriorityScore     float64                `json:"priority_score"`
	UrgencyIndicators []string               `json:"urgency_indicators"`
	IsAutomated       bool                   `json:"is_automated"`

	// Enhanced Gmail metadata
	InternalDate  time.Time  `json:"internal_date"`
	SentDate      *time.Time `json:"sent_date,omitempty"`
	ReceivedDate  *time.Time `json:"received_date,omitempty"`
	GmailLabels   []string   `json:"gmail_labels"`
	IsUnread      bool       `json:"is_unread"`
	IsStarred     bool       `json:"is_starred"`
	IsImportant   bool       `json:"is_important"`
	SizeEstimate  int        `json:"size_estimate"`

	// Security and authenticity
	AuthenticationResults map[string]interface{} `json:"authentication_results"`
	SpamScore             *float64               `json:"spam_score,omitempty"`

	// Thread information
	ThreadMessageCount int  `json:"thread_message_count"`
	IsFirstMessage     bool `json:"is_first_message"`

	// Processing context
	ContentPriority         string                 `json:"content_priority"`
	RequiresAIProcessing    bool                   `json:"requires_ai_processing"`
	EntityResolutionHints   map[string]interface{} `json:"entity_resolution_hints"`

	// Additional metadata
	CustomHeaders map[string]string      `json:"custom_headers"`
	DeliveryInfo  map[string]interface{} `json:"delivery_info"`
}

// IngestJobProgressEvent is published periodically during batch processing to track progress.
// Event type: "ingest_job.progress"
type IngestJobProgressEvent struct {
	BaseEvent

	// Identifiers
	JobID    string `json:"job_id"`
	TenantID string `json:"tenant_id"`

	// Progress stats
	TotalFiles      int `json:"total_files"`
	ProcessedCount  int `json:"processed_count"`
	ImportedCount   int `json:"imported_count"`
	SkippedCount    int `json:"skipped_count"`
	FailedCount     int `json:"failed_count"`

	// Current file
	CurrentFile *string `json:"current_file,omitempty"`

	// Timing
	ElapsedSeconds            float64  `json:"elapsed_seconds"`
	EstimatedRemainingSeconds *float64 `json:"estimated_remaining_seconds,omitempty"`

	// Status
	Status string `json:"status"`
}

// IngestJobCompletedEvent is published when a batch ingest job finishes.
// Event type: "ingest_job.completed"
type IngestJobCompletedEvent struct {
	BaseEvent

	// Identifiers
	JobID     string `json:"job_id"`
	TenantID  string `json:"tenant_id"`
	SourceTag string `json:"source_tag"`

	// Final stats
	TotalFiles    int `json:"total_files"`
	ImportedCount int `json:"imported_count"`
	SkippedCount  int `json:"skipped_count"`
	FailedCount   int `json:"failed_count"`

	// Timing
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
	DurationSeconds float64   `json:"duration_seconds"`

	// Status
	Success     bool   `json:"success"`
	FinalStatus string `json:"final_status"`

	// Labels and attachments
	LabelsCreated        []string `json:"labels_created"`
	AttachmentsExtracted int      `json:"attachments_extracted"`
	AttachmentsSkipped   int      `json:"attachments_skipped"`
}

// ManualAttachmentExtractedEvent is published when an attachment is extracted from a manually ingested email.
// Event type: "manual_attachment.extracted"
type ManualAttachmentExtractedEvent struct {
	BaseEvent

	// Identifiers
	AttachmentID string `json:"attachment_id"`
	SourceID     int64  `json:"source_id"`
	TenantID     string `json:"tenant_id"`

	// Attachment info
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	SizeBytes int    `json:"size_bytes"`

	// Document reference
	DocumentID *string `json:"document_id,omitempty"`

	// Status
	ExtractionStatus string `json:"extraction_status"`
}

// EventEnvelope wraps an event for Redis pub/sub transmission.
type EventEnvelope struct {
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

// ParseManualEmailIngestedEvent parses a ManualEmailIngestedEvent from JSON.
func ParseManualEmailIngestedEvent(data []byte) (*ManualEmailIngestedEvent, error) {
	var event ManualEmailIngestedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// ParseContentIngestedEvent parses a ContentIngestedEvent from JSON.
func ParseContentIngestedEvent(data []byte) (*ContentIngestedEvent, error) {
	var event ContentIngestedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// ToJSON serializes an event to JSON.
func (e *ManualEmailIngestedEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// ToJSON serializes an event to JSON.
func (e *ContentIngestedEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// ToJSON serializes an event to JSON.
func (e *IngestJobProgressEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// ToJSON serializes an event to JSON.
func (e *IngestJobCompletedEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// ToJSON serializes an event to JSON.
func (e *ManualAttachmentExtractedEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}
