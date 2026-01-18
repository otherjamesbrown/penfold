package push

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
