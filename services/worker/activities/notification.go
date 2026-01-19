// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// NotificationActivities holds dependencies for notification-related activities.
type NotificationActivities struct {
	logger       zerolog.Logger
	notifyClient NotificationClient
}

// NewNotificationActivities creates a new NotificationActivities instance.
func NewNotificationActivities(logger zerolog.Logger, notifyClient NotificationClient) *NotificationActivities {
	return &NotificationActivities{
		logger:       logger.With().Str("component", "notification_activities").Logger(),
		notifyClient: notifyClient,
	}
}

// SendNotificationInput is the input for the SendNotification activity.
type SendNotificationInput struct {
	TenantID    string            `json:"tenant_id"`
	Type        string            `json:"type"`        // email, slack, webhook
	Recipient   string            `json:"recipient"`   // email address, channel, or webhook URL
	Subject     string            `json:"subject"`     // notification subject/title
	Body        string            `json:"body"`        // notification body/message
	Priority    string            `json:"priority"`    // low, normal, high
	Metadata    map[string]string `json:"metadata,omitempty"`
	RetryOnFail bool              `json:"retry_on_fail"`
}

// SendNotificationOutput contains the result of sending a notification.
type SendNotificationOutput struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id,omitempty"`
	Error     string `json:"error,omitempty"`
	SentAt    string `json:"sent_at,omitempty"`
}

// SendNotification sends a notification via the configured notification client.
// This activity supports multiple notification types (email, slack, webhook).
func (a *NotificationActivities) SendNotification(ctx context.Context, input SendNotificationInput) (*SendNotificationOutput, error) {
	logger := a.logger.With().
		Str("activity", "SendNotification").
		Str("tenant_id", input.TenantID).
		Str("type", input.Type).
		Str("recipient", input.Recipient).
		Str("priority", input.Priority).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "preparing notification")

	logger.Info().Msg("Sending notification")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if err := a.validateNotificationInput(input); err != nil {
		return nil, err
	}

	// Check if notification client is available
	if a.notifyClient == nil {
		logger.Warn().Msg("Notification client not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"notification client not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Build notification
	notification := &Notification{
		TenantID:  input.TenantID,
		Type:      input.Type,
		Recipient: input.Recipient,
		Subject:   input.Subject,
		Body:      input.Body,
		Priority:  input.Priority,
		Metadata:  input.Metadata,
	}

	// Send the notification
	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "sending notification")

	err := a.notifyClient.SendNotification(ctx, notification)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to send notification")

		if input.RetryOnFail {
			// Return error to trigger retry
			return nil, fmt.Errorf("failed to send notification: %w", err)
		}

		// Return failure without error (won't retry)
		return &SendNotificationOutput{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Notification sent successfully")

	return &SendNotificationOutput{
		Success:   true,
		MessageID: fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		SentAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// validateNotificationInput validates the notification input.
func (a *NotificationActivities) validateNotificationInput(input SendNotificationInput) error {
	if input.TenantID == "" {
		return temporal.NewApplicationError("tenant_id is required", "ValidationError")
	}

	validTypes := map[string]bool{
		"email":   true,
		"slack":   true,
		"webhook": true,
	}
	if !validTypes[input.Type] {
		return temporal.NewApplicationError(
			fmt.Sprintf("invalid notification type: %s (must be email, slack, or webhook)", input.Type),
			"ValidationError",
		)
	}

	if input.Recipient == "" {
		return temporal.NewApplicationError("recipient is required", "ValidationError")
	}

	if input.Subject == "" && input.Type != "webhook" {
		return temporal.NewApplicationError("subject is required for email and slack notifications", "ValidationError")
	}

	if input.Body == "" {
		return temporal.NewApplicationError("body is required", "ValidationError")
	}

	validPriorities := map[string]bool{
		"":       true, // default to normal
		"low":    true,
		"normal": true,
		"high":   true,
	}
	if !validPriorities[input.Priority] {
		return temporal.NewApplicationError(
			fmt.Sprintf("invalid priority: %s (must be low, normal, or high)", input.Priority),
			"ValidationError",
		)
	}

	return nil
}

// SendBatchNotificationInput is the input for batch notification sending.
type SendBatchNotificationInput struct {
	TenantID      string                  `json:"tenant_id"`
	Notifications []SendNotificationInput `json:"notifications"`
	StopOnFailure bool                    `json:"stop_on_failure"`
}

// SendBatchNotificationOutput contains the results of batch notification sending.
type SendBatchNotificationOutput struct {
	SuccessCount int                        `json:"success_count"`
	FailureCount int                        `json:"failure_count"`
	Results      []SendNotificationOutput   `json:"results"`
}

// SendBatchNotification sends multiple notifications in a single activity.
// This is more efficient than calling SendNotification multiple times.
func (a *NotificationActivities) SendBatchNotification(ctx context.Context, input SendBatchNotificationInput) (*SendBatchNotificationOutput, error) {
	logger := a.logger.With().
		Str("activity", "SendBatchNotification").
		Str("tenant_id", input.TenantID).
		Int("batch_size", len(input.Notifications)).
		Bool("stop_on_failure", input.StopOnFailure).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting batch notification")

	logger.Info().Msg("Sending batch notifications")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if len(input.Notifications) == 0 {
		return &SendBatchNotificationOutput{
			SuccessCount: 0,
			FailureCount: 0,
			Results:      []SendNotificationOutput{},
		}, nil
	}

	results := make([]SendNotificationOutput, len(input.Notifications))
	successCount := 0
	failureCount := 0

	for i, notif := range input.Notifications {
		// Record heartbeat for each notification
		activity.RecordHeartbeat(ctx, fmt.Sprintf("sending notification %d/%d", i+1, len(input.Notifications)))

		// Check for cancellation between notifications
		if ctx.Err() != nil {
			// Mark remaining notifications as failed
			for j := i; j < len(input.Notifications); j++ {
				results[j] = SendNotificationOutput{
					Success: false,
					Error:   "context cancelled",
				}
				failureCount++
			}
			return &SendBatchNotificationOutput{
				SuccessCount: successCount,
				FailureCount: failureCount,
				Results:      results,
			}, ctx.Err()
		}

		// Ensure tenant ID is set
		if notif.TenantID == "" {
			notif.TenantID = input.TenantID
		}

		// Send this notification
		result, err := a.SendNotification(ctx, notif)
		if err != nil {
			logger.Warn().
				Str("recipient", notif.Recipient).
				Str("type", notif.Type).
				Err(err).
				Msg("Failed to send notification in batch")

			results[i] = SendNotificationOutput{
				Success: false,
				Error:   err.Error(),
			}
			failureCount++

			if input.StopOnFailure {
				// Mark remaining as not attempted
				for j := i + 1; j < len(input.Notifications); j++ {
					results[j] = SendNotificationOutput{
						Success: false,
						Error:   "batch stopped due to previous failure",
					}
					failureCount++
				}
				return &SendBatchNotificationOutput{
					SuccessCount: successCount,
					FailureCount: failureCount,
					Results:      results,
				}, nil
			}
		} else {
			results[i] = *result
			if result.Success {
				successCount++
			} else {
				failureCount++
			}
		}
	}

	logger.Info().
		Int("success_count", successCount).
		Int("failure_count", failureCount).
		Msg("Batch notification completed")

	return &SendBatchNotificationOutput{
		SuccessCount: successCount,
		FailureCount: failureCount,
		Results:      results,
	}, nil
}

// NotifyProcessingCompleteInput is the input for notifying about processing completion.
type NotifyProcessingCompleteInput struct {
	TenantID     string `json:"tenant_id"`
	SourceID     int64  `json:"source_id"`
	SourceType   string `json:"source_type"`
	Status       string `json:"status"` // completed, failed
	Recipient    string `json:"recipient"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// NotifyProcessingComplete sends a notification about processing completion.
// This is a convenience activity for the common use case of notifying users about job completion.
func (a *NotificationActivities) NotifyProcessingComplete(ctx context.Context, input NotifyProcessingCompleteInput) (*SendNotificationOutput, error) {
	logger := a.logger.With().
		Str("activity", "NotifyProcessingComplete").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("source_type", input.SourceType).
		Str("status", input.Status).
		Logger()

	logger.Info().Msg("Sending processing completion notification")

	// Build subject and body based on status
	var subject, body string
	var priority string

	switch input.Status {
	case "completed":
		subject = fmt.Sprintf("[Penfold] %s processing completed", input.SourceType)
		body = fmt.Sprintf("Processing of %s (ID: %d) has completed successfully.", input.SourceType, input.SourceID)
		priority = "normal"
	case "failed":
		subject = fmt.Sprintf("[Penfold] %s processing failed", input.SourceType)
		body = fmt.Sprintf("Processing of %s (ID: %d) has failed.", input.SourceType, input.SourceID)
		if input.ErrorMessage != "" {
			body += fmt.Sprintf("\n\nError: %s", input.ErrorMessage)
		}
		priority = "high"
	default:
		subject = fmt.Sprintf("[Penfold] %s processing update", input.SourceType)
		body = fmt.Sprintf("Processing of %s (ID: %d) status: %s", input.SourceType, input.SourceID, input.Status)
		priority = "normal"
	}

	return a.SendNotification(ctx, SendNotificationInput{
		TenantID:    input.TenantID,
		Type:        "email",
		Recipient:   input.Recipient,
		Subject:     subject,
		Body:        body,
		Priority:    priority,
		RetryOnFail: true,
		Metadata: map[string]string{
			"source_id":   fmt.Sprintf("%d", input.SourceID),
			"source_type": input.SourceType,
			"status":      input.Status,
		},
	})
}

// Ensure NotificationActivities implements the expected interface at compile time.
var _ interface {
	SendNotification(ctx context.Context, input SendNotificationInput) (*SendNotificationOutput, error)
	SendBatchNotification(ctx context.Context, input SendBatchNotificationInput) (*SendBatchNotificationOutput, error)
} = (*NotificationActivities)(nil)
