// Package push provides Gmail push notification handling via Cloud Pub/Sub.
package push

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/gmail/oauth"
)

// Gmail Watch API constants.
const (
	GmailWatchURL       = "https://gmail.googleapis.com/gmail/v1/users/me/watch"
	GmailStopWatchURL   = "https://gmail.googleapis.com/gmail/v1/users/me/stop"
	WatchExpirationDays = 7 // Gmail watch expires after 7 days
	RenewalBufferHours  = 24 // Renew 24 hours before expiry
)

// Subscription represents a Gmail push notification subscription.
type Subscription struct {
	// ID is the unique identifier for this subscription.
	ID string `json:"id"`

	// TenantID is the tenant that owns this subscription.
	TenantID string `json:"tenant_id"`

	// Email is the Gmail account email address.
	Email string `json:"email"`

	// TopicName is the Cloud Pub/Sub topic name.
	TopicName string `json:"topic_name"`

	// HistoryID is the Gmail history ID when the watch was created.
	HistoryID uint64 `json:"history_id"`

	// ExpiresAt is when the subscription expires.
	ExpiresAt time.Time `json:"expires_at"`

	// Labels to watch (empty means all labels).
	Labels []string `json:"labels,omitempty"`

	// Status of the subscription.
	Status SubscriptionStatus `json:"status"`

	// CreatedAt is when the subscription was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the subscription was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// LastNotificationAt is when the last notification was received.
	LastNotificationAt time.Time `json:"last_notification_at,omitempty"`
}

// SubscriptionStatus represents the status of a subscription.
type SubscriptionStatus string

const (
	// SubscriptionStatusActive indicates the subscription is active.
	SubscriptionStatusActive SubscriptionStatus = "active"

	// SubscriptionStatusExpired indicates the subscription has expired.
	SubscriptionStatusExpired SubscriptionStatus = "expired"

	// SubscriptionStatusSuspended indicates the subscription was suspended.
	SubscriptionStatusSuspended SubscriptionStatus = "suspended"

	// SubscriptionStatusError indicates the subscription has an error.
	SubscriptionStatusError SubscriptionStatus = "error"
)

// IsExpired returns true if the subscription is expired or will expire within the buffer.
func (s *Subscription) IsExpired() bool {
	return time.Now().Add(RenewalBufferHours * time.Hour).After(s.ExpiresAt)
}

// NeedsRenewal returns true if the subscription should be renewed.
func (s *Subscription) NeedsRenewal() bool {
	if s.Status != SubscriptionStatusActive {
		return false
	}
	return s.IsExpired()
}

// WatchRequest represents a Gmail watch request.
type WatchRequest struct {
	TopicName  string   `json:"topicName"`
	LabelIDs   []string `json:"labelIds,omitempty"`
	LabelFilterAction string `json:"labelFilterAction,omitempty"`
}

// WatchResponse represents a Gmail watch response.
type WatchResponse struct {
	HistoryID  uint64 `json:"historyId,string"`
	Expiration int64  `json:"expiration,string"`
}

// SubscriptionManagerConfig holds configuration for the subscription manager.
type SubscriptionManagerConfig struct {
	// OAuth2Manager for token management.
	OAuth2Manager *oauth.OAuth2Manager

	// SubscriptionStore for persistence.
	SubscriptionStore SubscriptionStore

	// TopicName is the Cloud Pub/Sub topic for notifications.
	TopicName string

	// HTTPClient for API requests.
	HTTPClient *http.Client

	// Logger for logging.
	Logger logging.Logger

	// Metrics for monitoring.
	Metrics *PushMetrics
}

// SubscriptionManager manages Gmail watch subscriptions.
type SubscriptionManager struct {
	config *SubscriptionManagerConfig
}

// NewSubscriptionManager creates a new subscription manager.
func NewSubscriptionManager(config *SubscriptionManagerConfig) (*SubscriptionManager, error) {
	if config == nil {
		return nil, fmt.Errorf("push: config is required")
	}

	if config.OAuth2Manager == nil {
		return nil, fmt.Errorf("push: oauth2 manager is required")
	}

	if config.SubscriptionStore == nil {
		return nil, fmt.Errorf("push: subscription store is required")
	}

	if config.TopicName == "" {
		return nil, fmt.Errorf("push: topic name is required")
	}

	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &SubscriptionManager{
		config: config,
	}, nil
}

// CreateSubscription creates a new Gmail watch subscription for a tenant.
func (m *SubscriptionManager) CreateSubscription(ctx context.Context, tenantID string, labels []string) (*Subscription, error) {
	if m.config.Metrics != nil {
		m.config.Metrics.SubscriptionsCreated.Inc()
	}

	// Get access token for the tenant.
	accessToken, err := m.config.OAuth2Manager.GetValidToken(ctx, tenantID)
	if err != nil {
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		return nil, fmt.Errorf("getting access token: %w", err)
	}

	// Get the user's email address.
	email, err := m.getUserEmail(ctx, accessToken)
	if err != nil {
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		return nil, fmt.Errorf("getting user email: %w", err)
	}

	// Check for existing subscription.
	existing, err := m.config.SubscriptionStore.GetSubscriptionByTenant(ctx, tenantID)
	if err == nil && existing.Status == SubscriptionStatusActive && !existing.IsExpired() {
		m.log().Info("existing active subscription found, returning it",
			logging.F("tenant_id", tenantID),
			logging.F("subscription_id", existing.ID),
		)
		return existing, nil
	}

	// Create watch request.
	watchReq := &WatchRequest{
		TopicName: m.config.TopicName,
		LabelIDs:  labels,
	}

	if len(labels) > 0 {
		watchReq.LabelFilterAction = "include"
	}

	// Call Gmail watch API.
	watchResp, err := m.createWatch(ctx, accessToken, watchReq)
	if err != nil {
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		return nil, fmt.Errorf("creating watch: %w", err)
	}

	// Create subscription record.
	now := time.Now()
	subscription := &Subscription{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		Email:     email,
		TopicName: m.config.TopicName,
		HistoryID: watchResp.HistoryID,
		ExpiresAt: time.UnixMilli(watchResp.Expiration),
		Labels:    labels,
		Status:    SubscriptionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Save the subscription.
	if err := m.config.SubscriptionStore.SaveSubscription(ctx, subscription); err != nil {
		// Try to stop the watch since we couldn't save.
		_ = m.stopWatch(ctx, accessToken)
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		return nil, fmt.Errorf("saving subscription: %w", err)
	}

	m.log().Info("created Gmail watch subscription",
		logging.F("tenant_id", tenantID),
		logging.F("subscription_id", subscription.ID),
		logging.F("email", email),
		logging.F("expires_at", subscription.ExpiresAt),
	)

	return subscription, nil
}

// RenewSubscription renews an existing subscription.
func (m *SubscriptionManager) RenewSubscription(ctx context.Context, subscriptionID string) error {
	if m.config.Metrics != nil {
		m.config.Metrics.SubscriptionsRenewed.Inc()
	}

	// Get the existing subscription.
	subscription, err := m.config.SubscriptionStore.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		return fmt.Errorf("getting subscription: %w", err)
	}

	// Get access token.
	accessToken, err := m.config.OAuth2Manager.GetValidToken(ctx, subscription.TenantID)
	if err != nil {
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		return fmt.Errorf("getting access token: %w", err)
	}

	// Create a new watch (this replaces the old one).
	watchReq := &WatchRequest{
		TopicName: m.config.TopicName,
		LabelIDs:  subscription.Labels,
	}

	if len(subscription.Labels) > 0 {
		watchReq.LabelFilterAction = "include"
	}

	watchResp, err := m.createWatch(ctx, accessToken, watchReq)
	if err != nil {
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		subscription.Status = SubscriptionStatusError
		m.config.SubscriptionStore.SaveSubscription(ctx, subscription)
		return fmt.Errorf("renewing watch: %w", err)
	}

	// Update subscription.
	subscription.HistoryID = watchResp.HistoryID
	subscription.ExpiresAt = time.UnixMilli(watchResp.Expiration)
	subscription.Status = SubscriptionStatusActive
	subscription.UpdatedAt = time.Now()

	if err := m.config.SubscriptionStore.SaveSubscription(ctx, subscription); err != nil {
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		return fmt.Errorf("saving renewed subscription: %w", err)
	}

	m.log().Info("renewed Gmail watch subscription",
		logging.F("subscription_id", subscriptionID),
		logging.F("expires_at", subscription.ExpiresAt),
	)

	return nil
}

// DeleteSubscription deletes a subscription and stops the watch.
func (m *SubscriptionManager) DeleteSubscription(ctx context.Context, subscriptionID string) error {
	if m.config.Metrics != nil {
		m.config.Metrics.SubscriptionsDeleted.Inc()
	}

	// Get the subscription.
	subscription, err := m.config.SubscriptionStore.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		return fmt.Errorf("getting subscription: %w", err)
	}

	// Get access token.
	accessToken, err := m.config.OAuth2Manager.GetValidToken(ctx, subscription.TenantID)
	if err != nil {
		m.log().Warn("failed to get access token for watch stop",
			logging.Err(err),
			logging.F("subscription_id", subscriptionID),
		)
		// Continue to delete from storage even if we can't stop the watch.
	} else {
		// Stop the watch.
		if err := m.stopWatch(ctx, accessToken); err != nil {
			m.log().Warn("failed to stop Gmail watch",
				logging.Err(err),
				logging.F("subscription_id", subscriptionID),
			)
			// Continue to delete from storage.
		}
	}

	// Delete from storage.
	if err := m.config.SubscriptionStore.DeleteSubscription(ctx, subscriptionID); err != nil {
		if m.config.Metrics != nil {
			m.config.Metrics.SubscriptionErrors.Inc()
		}
		return fmt.Errorf("deleting subscription: %w", err)
	}

	m.log().Info("deleted Gmail watch subscription",
		logging.F("subscription_id", subscriptionID),
	)

	return nil
}

// ListSubscriptions returns all subscriptions for a tenant.
func (m *SubscriptionManager) ListSubscriptions(ctx context.Context, tenantID string) ([]*Subscription, error) {
	return m.config.SubscriptionStore.ListSubscriptionsByTenant(ctx, tenantID)
}

// RenewExpiringSubscriptions renews all subscriptions that are about to expire.
func (m *SubscriptionManager) RenewExpiringSubscriptions(ctx context.Context) (renewed int, errors int, err error) {
	// Get all expiring subscriptions.
	expiring, err := m.config.SubscriptionStore.ListExpiringSubscriptions(ctx, RenewalBufferHours*time.Hour)
	if err != nil {
		return 0, 0, fmt.Errorf("listing expiring subscriptions: %w", err)
	}

	for _, sub := range expiring {
		if err := m.RenewSubscription(ctx, sub.ID); err != nil {
			m.log().Error("failed to renew subscription",
				logging.Err(err),
				logging.F("subscription_id", sub.ID),
			)
			errors++
		} else {
			renewed++
		}
	}

	return renewed, errors, nil
}

// createWatch calls the Gmail watch API.
func (m *SubscriptionManager) createWatch(ctx context.Context, accessToken string, watchReq *WatchRequest) (*WatchResponse, error) {
	body, err := json.Marshal(watchReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling watch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GmailWatchURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("watch API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var watchResp WatchResponse
	if err := json.Unmarshal(respBody, &watchResp); err != nil {
		return nil, fmt.Errorf("parsing watch response: %w", err)
	}

	return &watchResp, nil
}

// stopWatch calls the Gmail stop watch API.
func (m *SubscriptionManager) stopWatch(ctx context.Context, accessToken string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GmailStopWatchURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// 204 No Content is success for stop watch.
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stop watch API error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// getUserEmail retrieves the user's email address from the Gmail API.
func (m *SubscriptionManager) getUserEmail(ctx context.Context, accessToken string) (string, error) {
	profileURL := "https://gmail.googleapis.com/gmail/v1/users/me/profile"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("profile API error (status %d): %s", resp.StatusCode, string(body))
	}

	var profile struct {
		EmailAddress string `json:"emailAddress"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return "", fmt.Errorf("parsing profile response: %w", err)
	}

	return profile.EmailAddress, nil
}

func (m *SubscriptionManager) log() logging.Logger {
	if m.config.Logger != nil {
		return m.config.Logger
	}
	return logging.MustGlobal()
}

// BuildTopicName constructs a Cloud Pub/Sub topic name.
func BuildTopicName(projectID, topicID string) string {
	return fmt.Sprintf("projects/%s/topics/%s", url.PathEscape(projectID), url.PathEscape(topicID))
}
