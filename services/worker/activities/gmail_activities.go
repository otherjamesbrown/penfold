// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"
	"google.golang.org/protobuf/types/known/timestamppb"

	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmailv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// GmailClient defines the interface for Gmail API operations.
// This interface matches the proto-generated GmailConnectorServiceClient.
type GmailClient interface {
	// SyncEmails triggers synchronization of emails from Gmail.
	// Returns a sync job ID for tracking progress.
	SyncEmails(ctx context.Context, req *gmailv1.SyncEmailsRequest) (*gmailv1.SyncEmailsResponse, error)

	// GetSyncStatus retrieves the current status of a sync operation.
	GetSyncStatus(ctx context.Context, req *gmailv1.GetSyncStatusRequest) (*gmailv1.SyncStatus, error)

	// ListEmails returns a paginated list of processed emails.
	ListEmails(ctx context.Context, req *gmailv1.ListEmailsRequest) (*gmailv1.ListEmailsResponse, error)

	// GetEmail retrieves a single email by ID.
	GetEmail(ctx context.Context, req *gmailv1.GetEmailRequest) (*gmailv1.Email, error)
}

// EmailRepository defines the interface for email storage operations.
type EmailRepository interface {
	// StoreEmail stores a processed email.
	StoreEmail(ctx context.Context, email *ProcessedEmail) (int64, error)

	// GetEmail retrieves an email by ID.
	GetEmail(ctx context.Context, tenantID string, emailID int64) (*ProcessedEmail, error)

	// UpdateEmailStatus updates the processing status of an email.
	UpdateEmailStatus(ctx context.Context, tenantID string, emailID int64, status string) error

	// StoreAttachment stores an email attachment.
	StoreAttachment(ctx context.Context, attachment *StoredAttachment) (int64, error)
}

// ProcessedEmail represents a processed email ready for storage.
type ProcessedEmail struct {
	ID             int64
	TenantID       string
	MessageID      string
	ThreadID       string
	FromEmail      string
	FromName       string
	Subject        string
	ToEmails       []string
	CcEmails       []string
	Body           string
	BodyHTML       string
	ReceivedAt     time.Time
	Labels         []string
	HasAttachments bool
	ContentHash    string
	Status         string
	Metadata       map[string]string
}

// StoredAttachment represents a stored email attachment.
type StoredAttachment struct {
	ID           int64
	EmailID      int64
	TenantID     string
	FileName     string
	MimeType     string
	Size         int64
	ContentHash  string
	StoragePath  string
	ExtractedText string
}

// GmailActivities holds dependencies for Gmail-related activities.
type GmailActivities struct {
	logger      logging.Logger
	gmailClient GmailClient
	emailRepo   EmailRepository
}

// NewGmailActivities creates a new GmailActivities instance.
func NewGmailActivities(
	logger logging.Logger,
	gmailClient GmailClient,
	emailRepo EmailRepository,
) *GmailActivities {
	return &GmailActivities{
		logger:      logger.With(logging.F("component", "gmail_activities")),
		gmailClient: gmailClient,
		emailRepo:   emailRepo,
	}
}

// SyncEmailsInput is the input for the SyncEmails activity.
type SyncEmailsInput struct {
	TenantID         string    `json:"tenant_id"`
	JobID            string    `json:"job_id"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	Labels           []string  `json:"labels,omitempty"`
	Query            string    `json:"query,omitempty"`
	MaxResults       int32     `json:"max_results"`
	ForceFullSync    bool      `json:"force_full_sync"`
	IncludeSpamTrash bool      `json:"include_spam_trash"`
}

// SyncEmailsOutput is the output from the SyncEmails activity.
// Since SyncEmails is async, this returns the sync job ID for tracking.
type SyncEmailsOutput struct {
	SyncID  string `json:"sync_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// SyncEmailsActivity triggers email synchronization from Gmail.
// This is an async operation - it starts a sync job and returns the job ID.
// Use GetSyncStatusActivity to poll for completion.
func (a *GmailActivities) SyncEmailsActivity(ctx context.Context, input SyncEmailsInput) (*SyncEmailsOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "SyncEmailsActivity"),
		logging.F("tenant_id", input.TenantID),
		logging.F("job_id", input.JobID),
		logging.F("start_time", input.StartTime),
		logging.F("end_time", input.EndTime),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting email sync")

	logger.Info("Starting Gmail sync")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.StartTime.After(input.EndTime) {
		return nil, NewValidationError("start_time must be before end_time")
	}

	// Check if Gmail client is available
	if a.gmailClient == nil {
		logger.Warn("Gmail client not configured")
		return nil, NewConfigurationError("Gmail client not configured")
	}

	// Build sync request matching the proto definition
	maxResults := input.MaxResults
	if maxResults == 0 {
		maxResults = 500
	}

	syncReq := &gmailv1.SyncEmailsRequest{
		TenantId:         input.TenantID,
		Labels:           input.Labels,
		MaxResults:       maxResults,
		ForceFullSync:    input.ForceFullSync,
		IncludeSpamTrash: input.IncludeSpamTrash,
	}

	// Set optional fields
	if !input.StartTime.IsZero() {
		startTs := timestamppb.New(input.StartTime)
		syncReq.StartDate = startTs
	}
	if !input.EndTime.IsZero() {
		endTs := timestamppb.New(input.EndTime)
		syncReq.EndDate = endTs
	}
	if input.Query != "" {
		syncReq.Query = &input.Query
	}

	// Call Gmail service
	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling Gmail API")

	resp, err := a.gmailClient.SyncEmails(ctx, syncReq)
	if err != nil {
		logger.Error("Failed to start email sync", logging.Err(err))

		// Check if error is retryable
		if isGmailRetryableError(err) {
			return nil, NewTemporaryErrorWithCause("failed to start email sync", err)
		}
		return nil, fmt.Errorf("failed to start email sync: %w", err)
	}

	activity.RecordHeartbeat(ctx, "sync job started")

	// Extract status from response
	status := "unknown"
	if resp.GetStatus() != nil {
		status = resp.GetStatus().GetState().String()
	}

	output := &SyncEmailsOutput{
		SyncID:  resp.GetSyncId(),
		Status:  status,
		Message: resp.GetMessage(),
	}

	logger.Info("Gmail sync job started",
		logging.F("duration", time.Since(startTime)),
		logging.F("sync_id", output.SyncID),
		logging.F("status", output.Status),
	)

	return output, nil
}

// ProcessEmailInput is the input for the ProcessEmail activity.
type ProcessEmailInput struct {
	TenantID  string `json:"tenant_id"`
	JobID     string `json:"job_id"`
	MessageID string `json:"message_id"`
}

// ProcessEmailOutput is the output from the ProcessEmail activity.
type ProcessEmailOutput struct {
	EmailID        int64    `json:"email_id"`
	MessageID      string   `json:"message_id"`
	ContentHash    string   `json:"content_hash"`
	HasAttachments bool     `json:"has_attachments"`
	AttachmentIDs  []string `json:"attachment_ids,omitempty"`
	Subject        string   `json:"subject"`
	FromEmail      string   `json:"from_email"`
}

// ProcessEmailActivity processes a single email from Gmail.
// This activity fetches the full email content and stores it in the database.
func (a *GmailActivities) ProcessEmailActivity(ctx context.Context, input ProcessEmailInput) (*ProcessEmailOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ProcessEmailActivity"),
		logging.F("tenant_id", input.TenantID),
		logging.F("job_id", input.JobID),
		logging.F("message_id", input.MessageID),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting email processing")

	logger.Info("Processing email")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.MessageID == "" {
		return nil, NewValidationError("message_id is required")
	}

	// Check if Gmail client is available
	if a.gmailClient == nil {
		return nil, NewConfigurationError("Gmail client not configured")
	}

	// Fetch full email from Gmail
	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "fetching email from Gmail")

	getReq := &gmailv1.GetEmailRequest{
		TenantId: input.TenantID,
		EmailId:  input.MessageID,
		Format:   gmailv1.EmailFormat_EMAIL_FORMAT_FULL,
	}
	email, err := a.gmailClient.GetEmail(ctx, getReq)
	if err != nil {
		logger.Error("Failed to fetch email from Gmail", logging.Err(err))

		if isGmailRetryableError(err) {
			return nil, NewTemporaryErrorWithCause("failed to fetch email", err)
		}
		return nil, fmt.Errorf("failed to fetch email: %w", err)
	}

	activity.RecordHeartbeat(ctx, "processing email content")

	// Extract email content
	bodyText := ""
	bodyHTML := ""
	if email.GetBody() != nil {
		bodyText = email.GetBody().GetPlainText()
		bodyHTML = email.GetBody().GetHtml()
	}

	// Get sender info safely
	fromAddress := ""
	fromName := ""
	if email.GetFrom() != nil {
		fromAddress = email.GetFrom().GetAddress()
		fromName = email.GetFrom().GetName()
	}

	// Generate content hash
	contentHash := generateContentHash(email.GetSubject(), bodyText, fromAddress)

	// Build processed email
	processedEmail := &ProcessedEmail{
		TenantID:       input.TenantID,
		MessageID:      email.GetId(),
		ThreadID:       email.GetThreadId(),
		FromEmail:      fromAddress,
		FromName:       fromName,
		Subject:        email.GetSubject(),
		ToEmails:       extractEmailAddresses(email.GetTo()),
		CcEmails:       extractEmailAddresses(email.GetCc()),
		Body:           bodyText,
		BodyHTML:       bodyHTML,
		ReceivedAt:     email.GetTimestamp().AsTime(),
		Labels:         email.GetLabels(),
		HasAttachments: email.GetHasAttachments(),
		ContentHash:    contentHash,
		Status:         "pending",
		Metadata: map[string]string{
			"gmail_internal_date": fmt.Sprintf("%d", email.GetInternalDate()),
			"size_estimate":       fmt.Sprintf("%d", email.GetSizeEstimate()),
		},
	}

	// Store the email
	activity.RecordHeartbeat(ctx, "storing email")

	var emailID int64
	if a.emailRepo != nil {
		emailID, err = a.emailRepo.StoreEmail(ctx, processedEmail)
		if err != nil {
			logger.Error("Failed to store email", logging.Err(err))
			return nil, fmt.Errorf("failed to store email: %w", err)
		}
	}

	// Extract attachment IDs
	attachmentIDs := make([]string, 0, len(email.GetAttachments()))
	for _, att := range email.GetAttachments() {
		attachmentIDs = append(attachmentIDs, att.GetAttachmentId())
	}

	output := &ProcessEmailOutput{
		EmailID:        emailID,
		MessageID:      email.GetId(),
		ContentHash:    contentHash,
		HasAttachments: email.GetHasAttachments(),
		AttachmentIDs:  attachmentIDs,
		Subject:        email.GetSubject(),
		FromEmail:      fromAddress,
	}

	logger.Info("Email processed successfully",
		logging.F("duration", time.Since(startTime)),
		logging.F("email_id", emailID),
		logging.F("has_attachments", output.HasAttachments),
		logging.F("attachment_count", len(attachmentIDs)),
	)

	return output, nil
}

// AttachmentMetadata contains attachment info for processing.
type AttachmentMetadata struct {
	AttachmentID string `json:"attachment_id"`
	FileName     string `json:"file_name"`
	MimeType     string `json:"mime_type"`
	Size         int64  `json:"size"`
}

// StoreAttachmentsInput is the input for the StoreAttachments activity.
type StoreAttachmentsInput struct {
	TenantID    string               `json:"tenant_id"`
	JobID       string               `json:"job_id"`
	MessageID   string               `json:"message_id"`
	EmailID     int64                `json:"email_id"`
	Attachments []AttachmentMetadata `json:"attachments"`
}

// StoreAttachmentsOutput is the output from the StoreAttachments activity.
type StoreAttachmentsOutput struct {
	ProcessedCount int                 `json:"processed_count"`
	FailedCount    int                 `json:"failed_count"`
	Attachments    []AttachmentSummary `json:"attachments"`
}

// AttachmentSummary contains summary information about a processed attachment.
type AttachmentSummary struct {
	AttachmentID     string `json:"attachment_id"`
	FileName         string `json:"file_name"`
	MimeType         string `json:"mime_type"`
	Size             int64  `json:"size"`
	StoredID         int64  `json:"stored_id,omitempty"`
	Success          bool   `json:"success"`
	Error            string `json:"error,omitempty"`
	HasExtractedText bool   `json:"has_extracted_text"`
}

// StoreAttachmentsActivity stores email attachment metadata.
// Attachment content fetching is handled separately by the Gmail service.
// This activity records the attachment metadata for tracking and later processing.
func (a *GmailActivities) StoreAttachmentsActivity(ctx context.Context, input StoreAttachmentsInput) (*StoreAttachmentsOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "StoreAttachmentsActivity"),
		logging.F("tenant_id", input.TenantID),
		logging.F("job_id", input.JobID),
		logging.F("message_id", input.MessageID),
		logging.F("email_id", input.EmailID),
		logging.F("attachment_count", len(input.Attachments)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting attachment storage")

	logger.Info("Storing attachment metadata")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.MessageID == "" {
		return nil, NewValidationError("message_id is required")
	}

	if len(input.Attachments) == 0 {
		return &StoreAttachmentsOutput{
			ProcessedCount: 0,
			FailedCount:    0,
			Attachments:    []AttachmentSummary{},
		}, nil
	}

	startTime := time.Now()
	output := &StoreAttachmentsOutput{
		Attachments: make([]AttachmentSummary, 0, len(input.Attachments)),
	}

	// Process each attachment metadata
	for i, attMeta := range input.Attachments {
		// Check for cancellation between attachments
		if ctx.Err() != nil {
			// Mark remaining as failed
			for j := i; j < len(input.Attachments); j++ {
				output.Attachments = append(output.Attachments, AttachmentSummary{
					AttachmentID: input.Attachments[j].AttachmentID,
					FileName:     input.Attachments[j].FileName,
					Success:      false,
					Error:        "context cancelled",
				})
				output.FailedCount++
			}
			return output, ctx.Err()
		}

		// Record heartbeat
		activity.RecordHeartbeat(ctx, fmt.Sprintf("storing attachment %d/%d", i+1, len(input.Attachments)))

		// Generate content hash for attachment metadata
		contentHash := generateContentHash(attMeta.AttachmentID, attMeta.FileName, attMeta.MimeType)

		// Store attachment metadata if repository is available
		var storedID int64
		var err error
		if a.emailRepo != nil {
			storedAtt := &StoredAttachment{
				EmailID:     input.EmailID,
				TenantID:    input.TenantID,
				FileName:    attMeta.FileName,
				MimeType:    attMeta.MimeType,
				Size:        attMeta.Size,
				ContentHash: contentHash,
			}
			storedID, err = a.emailRepo.StoreAttachment(ctx, storedAtt)
			if err != nil {
				logger.Warn("Failed to store attachment", logging.Err(err), logging.F("attachment_id", attMeta.AttachmentID))
				output.Attachments = append(output.Attachments, AttachmentSummary{
					AttachmentID: attMeta.AttachmentID,
					FileName:     attMeta.FileName,
					MimeType:     attMeta.MimeType,
					Size:         attMeta.Size,
					Success:      false,
					Error:        err.Error(),
				})
				output.FailedCount++
				continue
			}
		}

		output.Attachments = append(output.Attachments, AttachmentSummary{
			AttachmentID:     attMeta.AttachmentID,
			FileName:         attMeta.FileName,
			MimeType:         attMeta.MimeType,
			Size:             attMeta.Size,
			StoredID:         storedID,
			Success:          true,
			HasExtractedText: false, // Text extraction happens in a separate activity
		})
		output.ProcessedCount++
	}

	logger.Info("Attachment storage completed",
		logging.F("duration", time.Since(startTime)),
		logging.F("processed_count", output.ProcessedCount),
		logging.F("failed_count", output.FailedCount),
	)

	return output, nil
}

// ApplyPrivacyFilterInput is the input for the ApplyPrivacyFilter activity.
type ApplyPrivacyFilterInput struct {
	TenantID    string            `json:"tenant_id"`
	ContentType string            `json:"content_type"` // email, attachment
	Content     string            `json:"content"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ApplyPrivacyFilterOutput is the output from the ApplyPrivacyFilter activity.
type ApplyPrivacyFilterOutput struct {
	FilteredContent string   `json:"filtered_content"`
	RedactedCount   int      `json:"redacted_count"`
	RedactedTypes   []string `json:"redacted_types"`
	IsApproved      bool     `json:"is_approved"`
	Warnings        []string `json:"warnings,omitempty"`
}

// ApplyPrivacyFilterActivity applies privacy filtering to content.
// This activity redacts PII, sensitive information, and ensures compliance.
func (a *GmailActivities) ApplyPrivacyFilterActivity(ctx context.Context, input ApplyPrivacyFilterInput) (*ApplyPrivacyFilterOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ApplyPrivacyFilterActivity"),
		logging.F("tenant_id", input.TenantID),
		logging.F("content_type", input.ContentType),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting privacy filtering")

	logger.Info("Applying privacy filter")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.Content == "" {
		return &ApplyPrivacyFilterOutput{
			FilteredContent: "",
			RedactedCount:   0,
			RedactedTypes:   []string{},
			IsApproved:      true,
		}, nil
	}

	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "applying privacy filters")

	filteredContent := input.Content
	redactedTypes := make([]string, 0)
	redactedCount := 0
	warnings := make([]string, 0)

	// Apply email address redaction
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	emailMatches := emailPattern.FindAllString(filteredContent, -1)
	if len(emailMatches) > 0 {
		filteredContent = emailPattern.ReplaceAllString(filteredContent, "[EMAIL_REDACTED]")
		redactedTypes = append(redactedTypes, "email")
		redactedCount += len(emailMatches)
	}

	// Apply phone number redaction (various formats)
	phonePatterns := []*regexp.Regexp{
		regexp.MustCompile(`\+?1?[-.\s]?\(?[0-9]{3}\)?[-.\s]?[0-9]{3}[-.\s]?[0-9]{4}`),
		regexp.MustCompile(`\+?[0-9]{1,3}[-.\s]?[0-9]{1,4}[-.\s]?[0-9]{1,4}[-.\s]?[0-9]{1,9}`),
	}
	for _, pattern := range phonePatterns {
		matches := pattern.FindAllString(filteredContent, -1)
		if len(matches) > 0 {
			filteredContent = pattern.ReplaceAllString(filteredContent, "[PHONE_REDACTED]")
			if !containsString(redactedTypes, "phone") {
				redactedTypes = append(redactedTypes, "phone")
			}
			redactedCount += len(matches)
		}
	}

	// Apply SSN redaction
	ssnPattern := regexp.MustCompile(`\b[0-9]{3}[-\s]?[0-9]{2}[-\s]?[0-9]{4}\b`)
	ssnMatches := ssnPattern.FindAllString(filteredContent, -1)
	if len(ssnMatches) > 0 {
		filteredContent = ssnPattern.ReplaceAllString(filteredContent, "[SSN_REDACTED]")
		redactedTypes = append(redactedTypes, "ssn")
		redactedCount += len(ssnMatches)
		warnings = append(warnings, "SSN patterns detected and redacted")
	}

	// Apply credit card number redaction
	ccPattern := regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\b`)
	ccMatches := ccPattern.FindAllString(filteredContent, -1)
	if len(ccMatches) > 0 {
		filteredContent = ccPattern.ReplaceAllString(filteredContent, "[CC_REDACTED]")
		redactedTypes = append(redactedTypes, "credit_card")
		redactedCount += len(ccMatches)
		warnings = append(warnings, "Credit card patterns detected and redacted")
	}

	// Apply IP address redaction
	ipPattern := regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)
	ipMatches := ipPattern.FindAllString(filteredContent, -1)
	if len(ipMatches) > 0 {
		filteredContent = ipPattern.ReplaceAllString(filteredContent, "[IP_REDACTED]")
		redactedTypes = append(redactedTypes, "ip_address")
		redactedCount += len(ipMatches)
	}

	output := &ApplyPrivacyFilterOutput{
		FilteredContent: filteredContent,
		RedactedCount:   redactedCount,
		RedactedTypes:   redactedTypes,
		IsApproved:      true,
		Warnings:        warnings,
	}

	logger.Info("Privacy filter applied",
		logging.F("duration", time.Since(startTime)),
		logging.F("redacted_count", redactedCount),
		logging.F("redacted_types", redactedTypes),
	)

	return output, nil
}

// Helper functions

// generateContentHash generates a SHA256 hash from content parts.
func generateContentHash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// extractEmailAddresses extracts email addresses from proto EmailAddress slice.
func extractEmailAddresses(addrs []*gmailv1.EmailAddress) []string {
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if addr != nil && addr.GetAddress() != "" {
			result = append(result, addr.GetAddress())
		}
	}
	return result
}

// isGmailRetryableError checks if a Gmail API error is retryable.
func isGmailRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	// Retryable errors
	retryablePatterns := []string{
		"rate limit",
		"quota exceeded",
		"internal error",
		"service unavailable",
		"timeout",
		"connection refused",
		"temporary",
		"503",
		"429",
		"500",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// containsString checks if a string slice contains a value.
func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// Ensure GmailActivities implements required interfaces at compile time.
var _ interface {
	SyncEmailsActivity(ctx context.Context, input SyncEmailsInput) (*SyncEmailsOutput, error)
	ProcessEmailActivity(ctx context.Context, input ProcessEmailInput) (*ProcessEmailOutput, error)
	StoreAttachmentsActivity(ctx context.Context, input StoreAttachmentsInput) (*StoreAttachmentsOutput, error)
	ApplyPrivacyFilterActivity(ctx context.Context, input ApplyPrivacyFilterInput) (*ApplyPrivacyFilterOutput, error)
} = (*GmailActivities)(nil)
