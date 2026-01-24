package push

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/services/gmail/oauth"
	"github.com/otherjamesbrown/penfold/services/gmail/sync"
)

// TestValidateNotification tests notification validation.
func TestValidateNotification(t *testing.T) {
	// Create minimal handler for validation.
	store := NewMemorySubscriptionStore()
	mockEngine := &mockSyncEngine{}
	processor, _ := NewNotificationProcessor(&ProcessorConfig{
		SyncEngine:  mockEngine.toRealEngine(),
		WorkerCount: 1,
		QueueSize:   10,
	})

	handler, err := NewHandler(&HandlerConfig{
		SubscriptionStore:     store,
		NotificationProcessor: processor,
	})
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name: "valid notification",
			input: PushNotification{
				Message: PubSubMessage{
					MessageID: "msg-123",
					Data:      base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"test@example.com","historyId":12345}`)),
				},
				Subscription: "projects/test/subscriptions/gmail-push",
			},
			wantErr: false,
		},
		{
			name: "missing message ID",
			input: PushNotification{
				Message: PubSubMessage{
					Data: base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"test@example.com","historyId":12345}`)),
				},
			},
			wantErr: true,
		},
		{
			name: "missing data",
			input: PushNotification{
				Message: PubSubMessage{
					MessageID: "msg-123",
				},
			},
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   "not a valid json",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil && !tt.wantErr {
				t.Fatalf("failed to marshal input: %v", err)
			}

			_, err = handler.ValidateNotification(data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNotification() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestDecodeNotificationData tests notification data decoding.
func TestDecodeNotificationData(t *testing.T) {
	store := NewMemorySubscriptionStore()
	mockEngine := &mockSyncEngine{}
	processor, _ := NewNotificationProcessor(&ProcessorConfig{
		SyncEngine:  mockEngine.toRealEngine(),
		WorkerCount: 1,
		QueueSize:   10,
	})

	handler, _ := NewHandler(&HandlerConfig{
		SubscriptionStore:     store,
		NotificationProcessor: processor,
	})

	tests := []struct {
		name      string
		data      GmailNotificationData
		wantEmail string
		wantHist  uint64
		wantErr   bool
	}{
		{
			name: "valid data",
			data: GmailNotificationData{
				EmailAddress: "test@example.com",
				HistoryID:    12345,
			},
			wantEmail: "test@example.com",
			wantHist:  12345,
			wantErr:   false,
		},
		{
			name: "missing email",
			data: GmailNotificationData{
				HistoryID: 12345,
			},
			wantErr: true,
		},
		{
			name: "missing history ID",
			data: GmailNotificationData{
				EmailAddress: "test@example.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(mustJSON(tt.data))

			result, err := handler.decodeNotificationData(encoded)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeNotificationData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.EmailAddress != tt.wantEmail {
					t.Errorf("EmailAddress = %v, want %v", result.EmailAddress, tt.wantEmail)
				}
				if result.HistoryID != tt.wantHist {
					t.Errorf("HistoryID = %v, want %v", result.HistoryID, tt.wantHist)
				}
			}
		})
	}
}

// TestMemorySubscriptionStore tests the in-memory subscription store.
func TestMemorySubscriptionStore(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySubscriptionStore()

	// Test SaveSubscription and GetSubscriptionByID.
	sub := &Subscription{
		ID:        "sub-123",
		TenantID:  "tenant-1",
		Email:     "test@example.com",
		TopicName: "projects/test/topics/gmail",
		HistoryID: 12345,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		Status:    SubscriptionStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.SaveSubscription(ctx, sub)
	if err != nil {
		t.Fatalf("SaveSubscription() error = %v", err)
	}

	// Get by ID.
	retrieved, err := store.GetSubscriptionByID(ctx, "sub-123")
	if err != nil {
		t.Fatalf("GetSubscriptionByID() error = %v", err)
	}
	if retrieved.ID != sub.ID {
		t.Errorf("GetSubscriptionByID() ID = %v, want %v", retrieved.ID, sub.ID)
	}

	// Get by email.
	retrieved, err = store.GetSubscriptionByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetSubscriptionByEmail() error = %v", err)
	}
	if retrieved.Email != sub.Email {
		t.Errorf("GetSubscriptionByEmail() Email = %v, want %v", retrieved.Email, sub.Email)
	}

	// Get by tenant.
	retrieved, err = store.GetSubscriptionByTenant(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetSubscriptionByTenant() error = %v", err)
	}
	if retrieved.TenantID != sub.TenantID {
		t.Errorf("GetSubscriptionByTenant() TenantID = %v, want %v", retrieved.TenantID, sub.TenantID)
	}

	// List by tenant.
	subs, err := store.ListSubscriptionsByTenant(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("ListSubscriptionsByTenant() error = %v", err)
	}
	if len(subs) != 1 {
		t.Errorf("ListSubscriptionsByTenant() len = %v, want 1", len(subs))
	}

	// List all active.
	subs, err = store.ListAllActiveSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListAllActiveSubscriptions() error = %v", err)
	}
	if len(subs) != 1 {
		t.Errorf("ListAllActiveSubscriptions() len = %v, want 1", len(subs))
	}

	// Delete subscription.
	err = store.DeleteSubscription(ctx, "sub-123")
	if err != nil {
		t.Fatalf("DeleteSubscription() error = %v", err)
	}

	// Verify deletion.
	_, err = store.GetSubscriptionByID(ctx, "sub-123")
	if err != ErrSubscriptionNotFound {
		t.Errorf("GetSubscriptionByID() after delete error = %v, want ErrSubscriptionNotFound", err)
	}
}

// TestSubscriptionExpiry tests subscription expiry checking.
func TestSubscriptionExpiry(t *testing.T) {
	// Active subscription (expires in 7 days).
	active := &Subscription{
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		Status:    SubscriptionStatusActive,
	}
	if active.IsExpired() {
		t.Error("active subscription should not be expired")
	}
	if active.NeedsRenewal() {
		t.Error("active subscription should not need renewal")
	}

	// Expiring soon (within 24 hours).
	expiringSoon := &Subscription{
		ExpiresAt: time.Now().Add(12 * time.Hour),
		Status:    SubscriptionStatusActive,
	}
	if !expiringSoon.IsExpired() {
		t.Error("subscription expiring within buffer should be considered expired")
	}
	if !expiringSoon.NeedsRenewal() {
		t.Error("subscription expiring within buffer should need renewal")
	}

	// Already expired.
	expired := &Subscription{
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		Status:    SubscriptionStatusActive,
	}
	if !expired.IsExpired() {
		t.Error("expired subscription should be considered expired")
	}

	// Suspended subscription (doesn't need renewal).
	suspended := &Subscription{
		ExpiresAt: time.Now().Add(12 * time.Hour),
		Status:    SubscriptionStatusSuspended,
	}
	if suspended.NeedsRenewal() {
		t.Error("suspended subscription should not need renewal")
	}
}

// TestServerDeduplication tests the server's notification deduplication.
func TestServerDeduplication(t *testing.T) {
	store := NewMemorySubscriptionStore()

	// Add a subscription for the test email.
	ctx := context.Background()
	_ = store.SaveSubscription(ctx, &Subscription{
		ID:        "sub-1",
		TenantID:  "tenant-1",
		Email:     "test@example.com",
		TopicName: "projects/test/topics/gmail",
		Status:    SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	})

	mockEngine := &mockSyncEngine{}
	processor, _ := NewNotificationProcessor(&ProcessorConfig{
		SyncEngine:  mockEngine.toRealEngine(),
		WorkerCount: 1,
		QueueSize:   100,
		BatchWindow: 0, // Disable batching for this test.
	})
	_ = processor.Start()
	defer processor.Stop()

	handler, _ := NewHandler(&HandlerConfig{
		SubscriptionStore:     store,
		NotificationProcessor: processor,
	})

	server, err := NewServer(&ServerConfig{
		Address:             ":0",
		Handler:             handler,
		DeduplicationWindow: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// First notification should not be a duplicate.
	if server.isDuplicate("msg-001") {
		t.Error("first notification should not be a duplicate")
	}

	// Same message should be a duplicate.
	if !server.isDuplicate("msg-001") {
		t.Error("same message ID should be a duplicate")
	}

	// Different message should not be a duplicate.
	if server.isDuplicate("msg-002") {
		t.Error("different message ID should not be a duplicate")
	}
}

// TestServerHTTPHandler tests the HTTP push endpoint.
func TestServerHTTPHandler(t *testing.T) {
	store := NewMemorySubscriptionStore()

	// Add a subscription.
	ctx := context.Background()
	_ = store.SaveSubscription(ctx, &Subscription{
		ID:        "sub-1",
		TenantID:  "tenant-1",
		Email:     "test@example.com",
		TopicName: "projects/test/topics/gmail",
		Status:    SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	})

	mockEngine := &mockSyncEngine{}
	processor, _ := NewNotificationProcessor(&ProcessorConfig{
		SyncEngine:  mockEngine.toRealEngine(),
		WorkerCount: 1,
		QueueSize:   100,
		BatchWindow: 0,
	})
	_ = processor.Start()
	defer processor.Stop()

	handler, _ := NewHandler(&HandlerConfig{
		SubscriptionStore:     store,
		NotificationProcessor: processor,
	})

	server, _ := NewServer(&ServerConfig{
		Address:             ":0",
		Handler:             handler,
		DeduplicationWindow: 10 * time.Minute,
	})

	// Create test server.
	mux := http.NewServeMux()
	mux.HandleFunc("/gmail/push", server.handlePush)
	mux.HandleFunc("/health", server.handleHealth)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Test health endpoint.
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health check status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	resp.Body.Close()

	// Test push endpoint with GET (should fail).
	resp, err = http.Get(ts.URL + "/gmail/push")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
	resp.Body.Close()

	// Test push endpoint with valid POST.
	notification := PushNotification{
		Message: PubSubMessage{
			MessageID: "msg-http-test",
			Data:      base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"test@example.com","historyId":12345}`)),
		},
		Subscription: "projects/test/subscriptions/gmail-push",
	}

	body, _ := json.Marshal(notification)
	resp, err = http.Post(ts.URL+"/gmail/push", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	resp.Body.Close()

	// Test push endpoint with invalid JSON.
	resp, err = http.Post(ts.URL+"/gmail/push", "application/json", bytes.NewReader([]byte("invalid")))
	if err != nil {
		t.Fatalf("invalid POST request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid POST status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	resp.Body.Close()
}

// TestServerAuth tests server authentication.
func TestServerAuth(t *testing.T) {
	store := NewMemorySubscriptionStore()
	mockEngine := &mockSyncEngine{}
	processor, _ := NewNotificationProcessor(&ProcessorConfig{
		SyncEngine:  mockEngine.toRealEngine(),
		WorkerCount: 1,
		QueueSize:   10,
		BatchWindow: 0,
	})

	handler, _ := NewHandler(&HandlerConfig{
		SubscriptionStore:     store,
		NotificationProcessor: processor,
	})

	server, _ := NewServer(&ServerConfig{
		Address:   ":0",
		Handler:   handler,
		AuthToken: "secret-token",
	})

	// Test without auth.
	req := httptest.NewRequest(http.MethodPost, "/gmail/push", nil)
	if server.verifyAuth(req) {
		t.Error("request without auth should fail")
	}

	// Test with wrong token.
	req.Header.Set("Authorization", "Bearer wrong-token")
	if server.verifyAuth(req) {
		t.Error("request with wrong token should fail")
	}

	// Test with correct token.
	req.Header.Set("Authorization", "Bearer secret-token")
	if !server.verifyAuth(req) {
		t.Error("request with correct token should pass")
	}

	// Test with wrong format.
	req.Header.Set("Authorization", "Basic secret-token")
	if server.verifyAuth(req) {
		t.Error("request with wrong auth format should fail")
	}
}

// TestProcessorBatching tests notification batching.
func TestProcessorBatching(t *testing.T) {
	mockEngine := &mockSyncEngine{}
	processor, err := NewNotificationProcessor(&ProcessorConfig{
		SyncEngine:    mockEngine.toRealEngine(),
		WorkerCount:   1,
		QueueSize:     100,
		BatchWindow:   50 * time.Millisecond,
		RetryDelay:    10 * time.Millisecond,
		MaxRetryDelay: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	ctx := context.Background()

	// Submit multiple notifications for the same tenant.
	for i := 0; i < 5; i++ {
		task := &ProcessingTask{
			NotificationID: "batch-" + string(rune('a'+i)),
			TenantID:       "tenant-batch",
			EmailAddress:   "batch@example.com",
			HistoryID:      uint64(100 + i),
			ReceivedAt:     time.Now(),
		}
		if err := processor.Submit(ctx, task); err != nil {
			t.Errorf("Submit() error = %v", err)
		}
	}

	// Should have 1 pending batch.
	if batches := processor.PendingBatches(); batches != 1 {
		t.Errorf("PendingBatches() = %d, want 1", batches)
	}

	// Wait for batch window to expire.
	time.Sleep(100 * time.Millisecond)

	// Batch should be flushed.
	if batches := processor.PendingBatches(); batches != 0 {
		t.Errorf("PendingBatches() after flush = %d, want 0", batches)
	}

	// Queue should have 1 task (the batched notifications).
	if qLen := processor.QueueLength(); qLen != 1 {
		t.Errorf("QueueLength() = %d, want 1", qLen)
	}
}

// TestBuildTopicName tests topic name construction.
func TestBuildTopicName(t *testing.T) {
	tests := []struct {
		projectID string
		topicID   string
		want      string
	}{
		{"my-project", "gmail-notifications", "projects/my-project/topics/gmail-notifications"},
		{"project-123", "topic", "projects/project-123/topics/topic"},
	}

	for _, tt := range tests {
		got := BuildTopicName(tt.projectID, tt.topicID)
		if got != tt.want {
			t.Errorf("BuildTopicName(%q, %q) = %q, want %q", tt.projectID, tt.topicID, got, tt.want)
		}
	}
}

// TestParseHistoryID tests history ID parsing.
func TestParseHistoryID(t *testing.T) {
	tests := []struct {
		input   string
		want    uint64
		wantErr bool
	}{
		{"12345", 12345, false},
		{"0", 0, false},
		{"18446744073709551615", 18446744073709551615, false},
		{"invalid", 0, true},
		{"-1", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseHistoryID(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseHistoryID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseHistoryID(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// Helper functions.

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// mockSyncEngine is a mock sync engine for testing.
type mockSyncEngine struct {
	syncCount int
	shouldErr bool
}

func (m *mockSyncEngine) toRealEngine() *sync.Engine {
	// Create a minimal real engine for testing.
	// In tests, we don't actually call the sync methods.
	storage := sync.NewMemoryStateStorage()

	// Create a mock OAuth2Manager.
	mockStorage := &mockTokenStorage{}
	oauthConfig := &oauth.Config{
		Credentials: &oauth.Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost/callback",
		},
		Storage: mockStorage,
	}
	oauthMgr, _ := oauth.NewOAuth2Manager(oauthConfig)

	engine, _ := sync.NewEngine(&sync.EngineConfig{
		OAuth2Manager: oauthMgr,
		StateStorage:  storage,
	})
	return engine
}

// mockTokenStorage implements oauth.TokenStorage for testing.
type mockTokenStorage struct{}

func (m *mockTokenStorage) StoreToken(ctx context.Context, token *oauth.Token) error {
	return nil
}

func (m *mockTokenStorage) GetToken(ctx context.Context, tenantID string) (*oauth.Token, error) {
	return &oauth.Token{
		AccessToken:  "mock-access-token",
		RefreshToken: "mock-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		TenantID:     tenantID,
	}, nil
}

func (m *mockTokenStorage) DeleteToken(ctx context.Context, tenantID string) error {
	return nil
}

func (m *mockTokenStorage) ListTokens(ctx context.Context) ([]*oauth.Token, error) {
	return nil, nil
}

// TestPushMetrics tests the push metrics creation and registration.
func TestPushMetrics(t *testing.T) {
	t.Run("NewPushMetrics creates all metrics", func(t *testing.T) {
		metrics := NewPushMetrics("test", "test_service")

		if metrics.NotificationsReceived == nil {
			t.Error("NotificationsReceived is nil")
		}
		if metrics.NotificationsProcessed == nil {
			t.Error("NotificationsProcessed is nil")
		}
		if metrics.NotificationErrors == nil {
			t.Error("NotificationErrors is nil")
		}
		if metrics.DuplicateNotifications == nil {
			t.Error("DuplicateNotifications is nil")
		}
		if metrics.SubscriptionsCreated == nil {
			t.Error("SubscriptionsCreated is nil")
		}
		if metrics.SubscriptionsRenewed == nil {
			t.Error("SubscriptionsRenewed is nil")
		}
		if metrics.SubscriptionsDeleted == nil {
			t.Error("SubscriptionsDeleted is nil")
		}
		if metrics.SubscriptionErrors == nil {
			t.Error("SubscriptionErrors is nil")
		}
		if metrics.ActiveSubscriptions == nil {
			t.Error("ActiveSubscriptions is nil")
		}
		if metrics.HTTPRequests == nil {
			t.Error("HTTPRequests is nil")
		}
		if metrics.HTTPErrors == nil {
			t.Error("HTTPErrors is nil")
		}
		if metrics.TasksSubmitted == nil {
			t.Error("TasksSubmitted is nil")
		}
		if metrics.TasksProcessing == nil {
			t.Error("TasksProcessing is nil")
		}
		if metrics.TasksCompleted == nil {
			t.Error("TasksCompleted is nil")
		}
		if metrics.TasksFailed == nil {
			t.Error("TasksFailed is nil")
		}
		if metrics.TasksRetried == nil {
			t.Error("TasksRetried is nil")
		}
		if metrics.TasksDropped == nil {
			t.Error("TasksDropped is nil")
		}
		if metrics.TaskLatency == nil {
			t.Error("TaskLatency is nil")
		}
		if metrics.BatchesFlushed == nil {
			t.Error("BatchesFlushed is nil")
		}
		if metrics.NotificationsBatched == nil {
			t.Error("NotificationsBatched is nil")
		}
	})

	t.Run("Register and Unregister", func(t *testing.T) {
		metrics := NewPushMetrics("test_register", "test_service")

		err := metrics.Register()
		if err != nil {
			t.Errorf("Register() error = %v", err)
		}

		// Unregister should not panic
		metrics.Unregister()
	})

	t.Run("double registration handled", func(t *testing.T) {
		metrics := NewPushMetrics("test_double", "test_service")

		err := metrics.Register()
		if err != nil {
			t.Errorf("Register() first call error = %v", err)
		}

		// Second registration should not error
		err = metrics.Register()
		if err != nil {
			t.Errorf("Register() second call error = %v", err)
		}

		metrics.Unregister()
	})
}

// TestDefaultProcessorConfig tests the default processor configuration.
func TestDefaultProcessorConfig(t *testing.T) {
	cfg := DefaultProcessorConfig()

	if cfg.WorkerCount != 5 {
		t.Errorf("WorkerCount = %d, want 5", cfg.WorkerCount)
	}
	if cfg.QueueSize != 1000 {
		t.Errorf("QueueSize = %d, want 1000", cfg.QueueSize)
	}
	if cfg.RetryDelay != 5*time.Second {
		t.Errorf("RetryDelay = %v, want 5s", cfg.RetryDelay)
	}
	if cfg.MaxRetryDelay != 5*time.Minute {
		t.Errorf("MaxRetryDelay = %v, want 5m", cfg.MaxRetryDelay)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.BatchWindow != 2*time.Second {
		t.Errorf("BatchWindow = %v, want 2s", cfg.BatchWindow)
	}
}

// TestDefaultServerConfig tests the default server configuration.
func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.Address != ":8082" {
		t.Errorf("Address = %s, want :8082", cfg.Address)
	}
	if cfg.DeduplicationWindow != 10*time.Minute {
		t.Errorf("DeduplicationWindow = %v, want 10m", cfg.DeduplicationWindow)
	}
	if cfg.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %v, want 10s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout = %v, want 10s", cfg.WriteTimeout)
	}
}

// TestSubscriptionStatusValues tests subscription status constants.
func TestSubscriptionStatusValues(t *testing.T) {
	tests := []struct {
		status SubscriptionStatus
		str    string
	}{
		{SubscriptionStatusActive, "active"},
		{SubscriptionStatusExpired, "expired"},
		{SubscriptionStatusSuspended, "suspended"},
		{SubscriptionStatusError, "error"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.str {
			t.Errorf("SubscriptionStatus = %s, want %s", tt.status, tt.str)
		}
	}
}

// TestMemorySubscriptionStore_ListExpiringSubscriptions tests the expiring subscriptions listing.
func TestMemorySubscriptionStore_ListExpiringSubscriptions(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySubscriptionStore()

	// Add subscriptions with different expiry times.
	now := time.Now()

	// Expiring within 1 hour
	expiringSoon := &Subscription{
		ID:        "sub-expiring",
		TenantID:  "tenant-1",
		Email:     "expiring@example.com",
		Status:    SubscriptionStatusActive,
		ExpiresAt: now.Add(30 * time.Minute),
		CreatedAt: now,
	}
	_ = store.SaveSubscription(ctx, expiringSoon)

	// Not expiring (7 days out)
	notExpiring := &Subscription{
		ID:        "sub-not-expiring",
		TenantID:  "tenant-2",
		Email:     "notexpiring@example.com",
		Status:    SubscriptionStatusActive,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		CreatedAt: now,
	}
	_ = store.SaveSubscription(ctx, notExpiring)

	// Already expired
	expired := &Subscription{
		ID:        "sub-expired",
		TenantID:  "tenant-3",
		Email:     "expired@example.com",
		Status:    SubscriptionStatusActive,
		ExpiresAt: now.Add(-1 * time.Hour),
		CreatedAt: now,
	}
	_ = store.SaveSubscription(ctx, expired)

	// List expiring within 2 hours
	expiring, err := store.ListExpiringSubscriptions(ctx, 2*time.Hour)
	if err != nil {
		t.Fatalf("ListExpiringSubscriptions() error = %v", err)
	}

	// Should include the expiring and expired subscriptions
	if len(expiring) != 2 {
		t.Errorf("ListExpiringSubscriptions() returned %d, want 2", len(expiring))
	}
}

// TestMemorySubscriptionStore_UpdateLastNotification tests updating last notification time.
func TestMemorySubscriptionStore_UpdateLastNotification(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySubscriptionStore()

	// Add a subscription
	sub := &Subscription{
		ID:        "sub-update",
		TenantID:  "tenant-1",
		Email:     "update@example.com",
		Status:    SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		HistoryID: 1000,
		CreatedAt: time.Now(),
	}
	_ = store.SaveSubscription(ctx, sub)

	// Update last notification
	notificationTime := time.Now()
	err := store.UpdateLastNotification(ctx, "sub-update", notificationTime)
	if err != nil {
		t.Fatalf("UpdateLastNotification() error = %v", err)
	}

	// Verify update
	updated, err := store.GetSubscriptionByID(ctx, "sub-update")
	if err != nil {
		t.Fatalf("GetSubscriptionByID() error = %v", err)
	}
	if updated.LastNotificationAt.IsZero() {
		t.Error("LastNotificationAt should be set")
	}
}

// TestMemorySubscriptionStore_UpdateLastNotification_NotFound tests updating non-existent subscription.
func TestMemorySubscriptionStore_UpdateLastNotification_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySubscriptionStore()

	err := store.UpdateLastNotification(ctx, "non-existent", time.Now())
	if err != ErrSubscriptionNotFound {
		t.Errorf("UpdateLastNotification() error = %v, want ErrSubscriptionNotFound", err)
	}
}

// TestMemorySubscriptionStore_GetByEmail_NotFound tests getting by email when not found.
func TestMemorySubscriptionStore_GetByEmail_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySubscriptionStore()

	_, err := store.GetSubscriptionByEmail(ctx, "notfound@example.com")
	if err != ErrSubscriptionNotFound {
		t.Errorf("GetSubscriptionByEmail() error = %v, want ErrSubscriptionNotFound", err)
	}
}

// TestMemorySubscriptionStore_GetByTenant_NotFound tests getting by tenant when not found.
func TestMemorySubscriptionStore_GetByTenant_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySubscriptionStore()

	_, err := store.GetSubscriptionByTenant(ctx, "tenant-notfound")
	if err != ErrSubscriptionNotFound {
		t.Errorf("GetSubscriptionByTenant() error = %v, want ErrSubscriptionNotFound", err)
	}
}

// TestMemorySubscriptionStore_DeleteNonExistent tests deleting non-existent subscription.
func TestMemorySubscriptionStore_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySubscriptionStore()

	err := store.DeleteSubscription(ctx, "non-existent")
	if err != ErrSubscriptionNotFound {
		t.Errorf("DeleteSubscription() error = %v, want ErrSubscriptionNotFound", err)
	}
}

// TestProcessingTask_Fields tests ProcessingTask field access.
func TestProcessingTask_Fields(t *testing.T) {
	now := time.Now()
	testErr := fmt.Errorf("connection timeout")
	task := &ProcessingTask{
		NotificationID: "notif-123",
		TenantID:       "tenant-1",
		EmailAddress:   "test@example.com",
		HistoryID:      12345,
		ReceivedAt:     now,
		Attempt:        2,
		LastError:      testErr,
	}

	if task.NotificationID != "notif-123" {
		t.Errorf("NotificationID = %s, want notif-123", task.NotificationID)
	}
	if task.TenantID != "tenant-1" {
		t.Errorf("TenantID = %s, want tenant-1", task.TenantID)
	}
	if task.EmailAddress != "test@example.com" {
		t.Errorf("EmailAddress = %s, want test@example.com", task.EmailAddress)
	}
	if task.HistoryID != 12345 {
		t.Errorf("HistoryID = %d, want 12345", task.HistoryID)
	}
	if task.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", task.Attempt)
	}
	if task.LastError == nil || task.LastError.Error() != "connection timeout" {
		t.Errorf("LastError = %v, want connection timeout", task.LastError)
	}
}

// TestPushNotification_Fields tests PushNotification struct fields.
func TestPushNotification_Fields(t *testing.T) {
	pubTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	notif := PushNotification{
		Message: PubSubMessage{
			MessageID:   "msg-123",
			Data:        "base64data",
			PublishTime: pubTime,
			Attributes: map[string]string{
				"key": "value",
			},
		},
		Subscription: "projects/test/subscriptions/gmail",
	}

	if notif.Message.MessageID != "msg-123" {
		t.Errorf("MessageID = %s, want msg-123", notif.Message.MessageID)
	}
	if notif.Message.Data != "base64data" {
		t.Errorf("Data = %s, want base64data", notif.Message.Data)
	}
	if notif.Subscription != "projects/test/subscriptions/gmail" {
		t.Errorf("Subscription = %s", notif.Subscription)
	}
	if notif.Message.Attributes["key"] != "value" {
		t.Errorf("Attributes[key] = %s, want value", notif.Message.Attributes["key"])
	}
	if !notif.Message.PublishTime.Equal(pubTime) {
		t.Errorf("PublishTime = %v, want %v", notif.Message.PublishTime, pubTime)
	}
}

// TestGmailNotificationData_Fields tests GmailNotificationData struct fields.
func TestGmailNotificationData_Fields(t *testing.T) {
	data := GmailNotificationData{
		EmailAddress: "test@example.com",
		HistoryID:    12345,
	}

	if data.EmailAddress != "test@example.com" {
		t.Errorf("EmailAddress = %s, want test@example.com", data.EmailAddress)
	}
	if data.HistoryID != 12345 {
		t.Errorf("HistoryID = %d, want 12345", data.HistoryID)
	}
}

// TestSubscription_Fields tests Subscription struct fields.
func TestSubscription_Fields(t *testing.T) {
	now := time.Now()
	sub := Subscription{
		ID:                 "sub-123",
		TenantID:           "tenant-1",
		Email:              "test@example.com",
		TopicName:          "projects/test/topics/gmail",
		HistoryID:          12345,
		ExpiresAt:          now.Add(7 * 24 * time.Hour),
		Status:             SubscriptionStatusActive,
		LastNotificationAt: now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if sub.ID != "sub-123" {
		t.Errorf("ID = %s, want sub-123", sub.ID)
	}
	if sub.TenantID != "tenant-1" {
		t.Errorf("TenantID = %s, want tenant-1", sub.TenantID)
	}
	if sub.Email != "test@example.com" {
		t.Errorf("Email = %s, want test@example.com", sub.Email)
	}
	if sub.Status != SubscriptionStatusActive {
		t.Errorf("Status = %s, want %s", sub.Status, SubscriptionStatusActive)
	}
	if sub.TopicName != "projects/test/topics/gmail" {
		t.Errorf("TopicName = %s, want projects/test/topics/gmail", sub.TopicName)
	}
	if sub.HistoryID != 12345 {
		t.Errorf("HistoryID = %d, want 12345", sub.HistoryID)
	}
}

// TestErrors tests error constants.
func TestPushErrors(t *testing.T) {
	if ErrSubscriptionNotFound == nil {
		t.Error("ErrSubscriptionNotFound is nil")
	}
	// Verify the error message is as expected.
	if ErrSubscriptionNotFound.Error() != "push: subscription not found" {
		t.Errorf("ErrSubscriptionNotFound.Error() = %s, want 'push: subscription not found'", ErrSubscriptionNotFound.Error())
	}
}

// TestServerConfig_Fields tests ServerConfig struct fields.
func TestServerConfig_Fields(t *testing.T) {
	cfg := ServerConfig{
		Address:             ":9090",
		AuthToken:           "secret-token",
		DeduplicationWindow: 10 * time.Minute,
		ReadTimeout:         30 * time.Second,
		WriteTimeout:        60 * time.Second,
		MaxRequestSize:      1024 * 1024,
	}

	if cfg.Address != ":9090" {
		t.Errorf("Address = %s, want :9090", cfg.Address)
	}
	if cfg.AuthToken != "secret-token" {
		t.Errorf("AuthToken = %s, want secret-token", cfg.AuthToken)
	}
	if cfg.DeduplicationWindow != 10*time.Minute {
		t.Errorf("DeduplicationWindow = %v, want 10m", cfg.DeduplicationWindow)
	}
	if cfg.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v, want 30s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 60*time.Second {
		t.Errorf("WriteTimeout = %v, want 60s", cfg.WriteTimeout)
	}
	if cfg.MaxRequestSize != 1024*1024 {
		t.Errorf("MaxRequestSize = %d, want %d", cfg.MaxRequestSize, 1024*1024)
	}
}

// TestHandlerConfig_Fields tests HandlerConfig struct fields.
func TestHandlerConfig_Fields(t *testing.T) {
	store := NewMemorySubscriptionStore()
	cfg := HandlerConfig{
		SubscriptionStore: store,
	}

	if cfg.SubscriptionStore != store {
		t.Error("SubscriptionStore mismatch")
	}
}

// TestProcessorConfig_Fields tests ProcessorConfig struct fields.
func TestProcessorConfig_Fields(t *testing.T) {
	cfg := ProcessorConfig{
		WorkerCount:   10,
		QueueSize:     500,
		RetryDelay:    2 * time.Second,
		MaxRetryDelay: 10 * time.Minute,
		MaxRetries:    3,
		BatchWindow:   5 * time.Second,
	}

	if cfg.WorkerCount != 10 {
		t.Errorf("WorkerCount = %d, want 10", cfg.WorkerCount)
	}
	if cfg.QueueSize != 500 {
		t.Errorf("QueueSize = %d, want 500", cfg.QueueSize)
	}
	if cfg.RetryDelay != 2*time.Second {
		t.Errorf("RetryDelay = %v, want 2s", cfg.RetryDelay)
	}
	if cfg.MaxRetryDelay != 10*time.Minute {
		t.Errorf("MaxRetryDelay = %v, want 10m", cfg.MaxRetryDelay)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.BatchWindow != 5*time.Second {
		t.Errorf("BatchWindow = %v, want 5s", cfg.BatchWindow)
	}
}

// TestServerRateLimiting tests the server's rate limiting functionality.
func TestServerRateLimiting(t *testing.T) {
	store := NewMemorySubscriptionStore()
	mockEngine := &mockSyncEngine{}
	processor, _ := NewNotificationProcessor(&ProcessorConfig{
		SyncEngine:  mockEngine.toRealEngine(),
		WorkerCount: 1,
		QueueSize:   100,
		BatchWindow: 0,
	})
	_ = processor.Start()
	defer processor.Stop()

	handler, _ := NewHandler(&HandlerConfig{
		SubscriptionStore:     store,
		NotificationProcessor: processor,
	})

	t.Run("rate limits requests when limit exceeded", func(t *testing.T) {
		// Configure with very low limit for testing.
		server, err := NewServer(&ServerConfig{
			Address:             ":0",
			Handler:             handler,
			DeduplicationWindow: 10 * time.Minute,
			RateLimitConfig: &RateLimitConfig{
				RPS:   1.0,  // 1 request per second
				Burst: 2,    // Allow 2 requests initially
			},
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		// Create test server using the rate-limited handler.
		ts := httptest.NewServer(server.httpServer.Handler)
		defer ts.Close()

		// First two requests should succeed (burst capacity).
		for i := 0; i < 2; i++ {
			notification := PushNotification{
				Message: PubSubMessage{
					MessageID: fmt.Sprintf("rate-limit-test-%d", i),
					Data:      base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"test@example.com","historyId":12345}`)),
				},
				Subscription: "projects/test/subscriptions/gmail-push",
			}
			body, _ := json.Marshal(notification)
			resp, err := http.Post(ts.URL+"/gmail/push", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}
			// Note: May get 400 due to no subscription, but should NOT be 429.
			if resp.StatusCode == http.StatusTooManyRequests {
				t.Errorf("request %d should not be rate limited", i)
			}
			resp.Body.Close()
		}

		// Third request should be rate limited.
		notification := PushNotification{
			Message: PubSubMessage{
				MessageID: "rate-limit-test-blocked",
				Data:      base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"test@example.com","historyId":12345}`)),
			},
			Subscription: "projects/test/subscriptions/gmail-push",
		}
		body, _ := json.Marshal(notification)
		resp, err := http.Post(ts.URL+"/gmail/push", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("rate limited request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("third request should be rate limited, got status %d", resp.StatusCode)
		}

		// Verify Retry-After header is set.
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter == "" {
			t.Error("expected Retry-After header to be set")
		}
	})

	t.Run("health endpoint is not rate limited", func(t *testing.T) {
		server, err := NewServer(&ServerConfig{
			Address:             ":0",
			Handler:             handler,
			DeduplicationWindow: 10 * time.Minute,
			RateLimitConfig: &RateLimitConfig{
				RPS:   1.0,
				Burst: 1, // Very low burst to ensure we'd hit limit quickly.
			},
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		ts := httptest.NewServer(server.httpServer.Handler)
		defer ts.Close()

		// Health endpoint should never be rate limited.
		for i := 0; i < 10; i++ {
			resp, err := http.Get(ts.URL + "/health")
			if err != nil {
				t.Fatalf("health check %d failed: %v", i, err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("health check %d should not be rate limited, got status %d", i, resp.StatusCode)
			}
			resp.Body.Close()
		}
	})

	t.Run("includes rate limit headers", func(t *testing.T) {
		server, err := NewServer(&ServerConfig{
			Address:             ":0",
			Handler:             handler,
			DeduplicationWindow: 10 * time.Minute,
			RateLimitConfig: &RateLimitConfig{
				RPS:   100.0,
				Burst: 50,
			},
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		ts := httptest.NewServer(server.httpServer.Handler)
		defer ts.Close()

		notification := PushNotification{
			Message: PubSubMessage{
				MessageID: "rate-limit-headers-test",
				Data:      base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"test@example.com","historyId":12345}`)),
			},
			Subscription: "projects/test/subscriptions/gmail-push",
		}
		body, _ := json.Marshal(notification)
		resp, err := http.Post(ts.URL+"/gmail/push", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Check that rate limit headers are set.
		if resp.Header.Get("X-RateLimit-Limit") == "" {
			t.Error("expected X-RateLimit-Limit header to be set")
		}
		if resp.Header.Get("X-RateLimit-Remaining") == "" {
			t.Error("expected X-RateLimit-Remaining header to be set")
		}
	})

	t.Run("no rate limiting when not configured", func(t *testing.T) {
		server, err := NewServer(&ServerConfig{
			Address:             ":0",
			Handler:             handler,
			DeduplicationWindow: 10 * time.Minute,
			// No RateLimiter or RateLimitConfig.
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		ts := httptest.NewServer(server.httpServer.Handler)
		defer ts.Close()

		// Should be able to make many requests without rate limiting.
		for i := 0; i < 20; i++ {
			notification := PushNotification{
				Message: PubSubMessage{
					MessageID: fmt.Sprintf("no-rate-limit-test-%d", i),
					Data:      base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"test@example.com","historyId":12345}`)),
				},
				Subscription: "projects/test/subscriptions/gmail-push",
			}
			body, _ := json.Marshal(notification)
			resp, err := http.Post(ts.URL+"/gmail/push", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				t.Errorf("request %d should not be rate limited when rate limiting is disabled", i)
			}
			resp.Body.Close()
		}
	})
}

// TestExtractClientIP tests the client IP extraction function.
func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		wantIP     string
	}{
		{
			name:       "remote addr with port",
			remoteAddr: "192.168.1.100:12345",
			headers:    nil,
			wantIP:     "192.168.1.100",
		},
		{
			name:       "remote addr without port",
			remoteAddr: "192.168.1.100",
			headers:    nil,
			wantIP:     "192.168.1.100",
		},
		{
			name:       "X-Forwarded-For single IP",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			wantIP:     "203.0.113.50",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50, 70.41.3.18, 150.172.238.178"},
			wantIP:     "203.0.113.50",
		},
		{
			name:       "X-Forwarded-For with spaces",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "  203.0.113.50  "},
			wantIP:     "203.0.113.50",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Real-IP": "198.51.100.25"},
			wantIP:     "198.51.100.25",
		},
		{
			name:       "X-Forwarded-For takes precedence over X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.50",
				"X-Real-IP":       "198.51.100.25",
			},
			wantIP: "203.0.113.50",
		},
		{
			name:       "IPv6 address",
			remoteAddr: "[2001:db8::1]:12345",
			headers:    nil,
			wantIP:     "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/gmail/push", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := extractClientIP(req)
			if got != tt.wantIP {
				t.Errorf("extractClientIP() = %q, want %q", got, tt.wantIP)
			}
		})
	}
}

// TestDefaultRateLimitConfig tests the default rate limit configuration.
func TestDefaultRateLimitConfig(t *testing.T) {
	cfg := DefaultRateLimitConfig()

	if cfg.RPS != 50.0 {
		t.Errorf("RPS = %v, want 50.0", cfg.RPS)
	}
	if cfg.Burst != 100 {
		t.Errorf("Burst = %d, want 100", cfg.Burst)
	}
}

// TestRateLimitConfig_Fields tests RateLimitConfig struct fields.
func TestRateLimitConfig_Fields(t *testing.T) {
	cfg := RateLimitConfig{
		RPS:   25.5,
		Burst: 50,
	}

	if cfg.RPS != 25.5 {
		t.Errorf("RPS = %v, want 25.5", cfg.RPS)
	}
	if cfg.Burst != 50 {
		t.Errorf("Burst = %d, want 50", cfg.Burst)
	}
}
