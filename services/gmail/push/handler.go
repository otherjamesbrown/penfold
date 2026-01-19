// Package push provides Gmail push notification handling via Cloud Pub/Sub.
// It handles incoming push notifications from Gmail's watch API, validates them,
// and triggers incremental sync operations for updated mailboxes.
package push

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PushNotification represents a Gmail push notification from Pub/Sub.
type PushNotification struct {
	// Message is the Pub/Sub message containing the notification data.
	Message PubSubMessage `json:"message"`

	// Subscription is the Pub/Sub subscription name.
	Subscription string `json:"subscription"`
}

// PubSubMessage represents a Cloud Pub/Sub message.
type PubSubMessage struct {
	// Data is the base64-encoded message data.
	Data string `json:"data"`

	// MessageID is the unique message identifier.
	MessageID string `json:"messageId"`

	// PublishTime is when the message was published.
	PublishTime time.Time `json:"publishTime"`

	// Attributes contains message attributes.
	Attributes map[string]string `json:"attributes,omitempty"`
}

// GmailNotificationData represents the decoded Gmail notification payload.
type GmailNotificationData struct {
	// EmailAddress is the Gmail account that received the notification.
	EmailAddress string `json:"emailAddress"`

	// HistoryID is the Gmail history ID for incremental sync.
	HistoryID uint64 `json:"historyId"`
}

// HandlerConfig holds configuration for the push notification handler.
type HandlerConfig struct {
	// SubscriptionStore for subscription lookups.
	SubscriptionStore SubscriptionStore

	// NotificationProcessor for processing notifications.
	NotificationProcessor *NotificationProcessor

	// Logger for logging.
	Logger logging.Logger

	// Metrics for monitoring.
	Metrics *PushMetrics
}

// Handler processes Gmail push notifications.
type Handler struct {
	config *HandlerConfig
}

// NewHandler creates a new push notification handler.
func NewHandler(config *HandlerConfig) (*Handler, error) {
	if config == nil {
		return nil, fmt.Errorf("push: config is required")
	}

	if config.SubscriptionStore == nil {
		return nil, fmt.Errorf("push: subscription store is required")
	}

	if config.NotificationProcessor == nil {
		return nil, fmt.Errorf("push: notification processor is required")
	}

	return &Handler{
		config: config,
	}, nil
}

// HandlePush processes an incoming push notification.
// It validates the notification, looks up the subscription, and queues
// the notification for processing.
func (h *Handler) HandlePush(ctx context.Context, notification *PushNotification) error {
	if h.config.Metrics != nil {
		h.config.Metrics.NotificationsReceived.Inc()
	}

	h.log().Debug("received push notification",
		logging.F("message_id", notification.Message.MessageID),
		logging.F("subscription", notification.Subscription),
	)

	// Decode the notification data.
	data, err := h.decodeNotificationData(notification.Message.Data)
	if err != nil {
		if h.config.Metrics != nil {
			h.config.Metrics.NotificationErrors.Inc()
		}
		return fmt.Errorf("decoding notification data: %w", err)
	}

	h.log().Info("processing Gmail push notification",
		logging.F("email", data.EmailAddress),
		logging.F("history_id", data.HistoryID),
		logging.F("message_id", notification.Message.MessageID),
	)

	// Look up the subscription by email address.
	subscription, err := h.config.SubscriptionStore.GetSubscriptionByEmail(ctx, data.EmailAddress)
	if err != nil {
		if h.config.Metrics != nil {
			h.config.Metrics.NotificationErrors.Inc()
		}
		return fmt.Errorf("looking up subscription for %s: %w", data.EmailAddress, err)
	}

	// Queue the notification for processing.
	task := &ProcessingTask{
		NotificationID: notification.Message.MessageID,
		TenantID:       subscription.TenantID,
		EmailAddress:   data.EmailAddress,
		HistoryID:      data.HistoryID,
		ReceivedAt:     time.Now(),
		Subscription:   subscription,
	}

	if err := h.config.NotificationProcessor.Submit(ctx, task); err != nil {
		if h.config.Metrics != nil {
			h.config.Metrics.NotificationErrors.Inc()
		}
		return fmt.Errorf("submitting task for processing: %w", err)
	}

	if h.config.Metrics != nil {
		h.config.Metrics.NotificationsProcessed.Inc()
	}

	return nil
}

// ValidateNotification validates and parses raw push notification data.
func (h *Handler) ValidateNotification(data []byte) (*PushNotification, error) {
	var notification PushNotification
	if err := json.Unmarshal(data, &notification); err != nil {
		return nil, fmt.Errorf("parsing notification JSON: %w", err)
	}

	// Validate required fields.
	if notification.Message.MessageID == "" {
		return nil, fmt.Errorf("missing message ID")
	}

	if notification.Message.Data == "" {
		return nil, fmt.Errorf("missing message data")
	}

	return &notification, nil
}

// ProcessHistoryChange processes a history change for a specific account.
// This is used when manually triggering sync from a known history ID.
func (h *Handler) ProcessHistoryChange(ctx context.Context, tenantID string, historyID uint64) error {
	// Look up the subscription.
	subscription, err := h.config.SubscriptionStore.GetSubscriptionByTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("looking up subscription: %w", err)
	}

	// Create a processing task.
	task := &ProcessingTask{
		NotificationID: fmt.Sprintf("manual-%d-%d", time.Now().UnixNano(), historyID),
		TenantID:       tenantID,
		EmailAddress:   subscription.Email,
		HistoryID:      historyID,
		ReceivedAt:     time.Now(),
		Subscription:   subscription,
	}

	return h.config.NotificationProcessor.Submit(ctx, task)
}

// decodeNotificationData decodes the base64-encoded notification data.
func (h *Handler) decodeNotificationData(encodedData string) (*GmailNotificationData, error) {
	decoded, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		// Try URL-safe encoding as fallback.
		decoded, err = base64.URLEncoding.DecodeString(encodedData)
		if err != nil {
			return nil, fmt.Errorf("base64 decode failed: %w", err)
		}
	}

	var data GmailNotificationData
	if err := json.Unmarshal(decoded, &data); err != nil {
		return nil, fmt.Errorf("parsing notification payload: %w", err)
	}

	// Validate required fields.
	if data.EmailAddress == "" {
		return nil, fmt.Errorf("missing email address in notification")
	}

	if data.HistoryID == 0 {
		return nil, fmt.Errorf("missing or invalid history ID in notification")
	}

	return &data, nil
}

// ParseHistoryID parses a history ID from a string.
func ParseHistoryID(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}

func (h *Handler) log() logging.Logger {
	if h.config.Logger != nil {
		return h.config.Logger
	}
	return logging.MustGlobal()
}
