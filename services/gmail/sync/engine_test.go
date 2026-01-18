package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/services/gmail/oauth"
)

// mockTokenStorage implements oauth.TokenStorage for testing.
type mockTokenStorage struct {
	tokens map[string]*oauth.Token
}

func newMockTokenStorage() *mockTokenStorage {
	return &mockTokenStorage{
		tokens: make(map[string]*oauth.Token),
	}
}

func (s *mockTokenStorage) StoreToken(ctx context.Context, token *oauth.Token) error {
	s.tokens[token.TenantID] = token
	return nil
}

func (s *mockTokenStorage) GetToken(ctx context.Context, tenantID string) (*oauth.Token, error) {
	token, ok := s.tokens[tenantID]
	if !ok {
		return nil, oauth.ErrTokenNotFound
	}
	return token, nil
}

func (s *mockTokenStorage) DeleteToken(ctx context.Context, tenantID string) error {
	delete(s.tokens, tenantID)
	return nil
}

func (s *mockTokenStorage) ListTokens(ctx context.Context) ([]*oauth.Token, error) {
	var tokens []*oauth.Token
	for _, t := range s.tokens {
		tokens = append(tokens, t)
	}
	return tokens, nil
}

// setupTestOAuth creates a mock OAuth2Manager for testing.
func setupTestOAuth(t *testing.T, tenantID string) *oauth.OAuth2Manager {
	t.Helper()

	tokenStorage := newMockTokenStorage()

	// Store a valid token.
	token := &oauth.Token{
		TenantID:     tenantID,
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	tokenStorage.StoreToken(context.Background(), token)

	manager, err := oauth.NewOAuth2Manager(&oauth.Config{
		Credentials: &oauth.Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
		Storage: tokenStorage,
	})
	if err != nil {
		t.Fatalf("failed to create OAuth2Manager: %v", err)
	}

	return manager
}

// TestSyncState tests the SyncState struct methods.
func TestSyncState(t *testing.T) {
	t.Run("IsComplete", func(t *testing.T) {
		tests := []struct {
			status   SyncStatus
			expected bool
		}{
			{SyncStatusPending, false},
			{SyncStatusInProgress, false},
			{SyncStatusInterrupted, false},
			{SyncStatusCompleted, true},
			{SyncStatusCompletedWithErrors, true},
			{SyncStatusFailed, true},
			{SyncStatusCancelled, true},
		}

		for _, tt := range tests {
			state := &SyncState{Status: tt.status}
			if got := state.IsComplete(); got != tt.expected {
				t.Errorf("IsComplete() for status %s = %v, want %v", tt.status, got, tt.expected)
			}
		}
	})

	t.Run("CanResume", func(t *testing.T) {
		tests := []struct {
			name     string
			state    SyncState
			expected bool
		}{
			{
				name:     "interrupted with page token",
				state:    SyncState{Status: SyncStatusInterrupted, NextPageToken: "token"},
				expected: true,
			},
			{
				name:     "interrupted without page token",
				state:    SyncState{Status: SyncStatusInterrupted, NextPageToken: ""},
				expected: false,
			},
			{
				name:     "completed",
				state:    SyncState{Status: SyncStatusCompleted, NextPageToken: "token"},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := tt.state.CanResume(); got != tt.expected {
					t.Errorf("CanResume() = %v, want %v", got, tt.expected)
				}
			})
		}
	})

	t.Run("Progress", func(t *testing.T) {
		tests := []struct {
			processed int64
			total     int64
			expected  int32
		}{
			{0, 0, 0},
			{0, 100, 0},
			{50, 100, 50},
			{100, 100, 100},
			{150, 100, 100}, // Capped at 100
		}

		for _, tt := range tests {
			state := &SyncState{ProcessedCount: tt.processed, TotalCount: tt.total}
			if got := state.Progress(); got != tt.expected {
				t.Errorf("Progress() for %d/%d = %d, want %d", tt.processed, tt.total, got, tt.expected)
			}
		}
	})
}

// TestMemoryStateStorage tests the in-memory state storage.
func TestMemoryStateStorage(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStateStorage()

	t.Run("SaveAndGetState", func(t *testing.T) {
		state := &SyncState{
			SyncID:     "sync-1",
			TenantID:   "tenant-1",
			SyncType:   SyncTypeFull,
			Status:     SyncStatusInProgress,
			HistoryID:  12345,
			CreatedAt:  time.Now(),
			Labels:     []string{"INBOX"},
		}

		err := storage.SaveState(ctx, state)
		if err != nil {
			t.Fatalf("SaveState failed: %v", err)
		}

		retrieved, err := storage.GetStateByID(ctx, "sync-1")
		if err != nil {
			t.Fatalf("GetStateByID failed: %v", err)
		}

		if retrieved.SyncID != state.SyncID {
			t.Errorf("SyncID mismatch: got %s, want %s", retrieved.SyncID, state.SyncID)
		}
		if retrieved.TenantID != state.TenantID {
			t.Errorf("TenantID mismatch: got %s, want %s", retrieved.TenantID, state.TenantID)
		}
		if retrieved.HistoryID != state.HistoryID {
			t.Errorf("HistoryID mismatch: got %d, want %d", retrieved.HistoryID, state.HistoryID)
		}
	})

	t.Run("GetStateNotFound", func(t *testing.T) {
		_, err := storage.GetStateByID(ctx, "nonexistent")
		if err != ErrStateNotFound {
			t.Errorf("expected ErrStateNotFound, got %v", err)
		}
	})

	t.Run("GetLatestState", func(t *testing.T) {
		// Save a completed state.
		state := &SyncState{
			SyncID:     "sync-completed",
			TenantID:   "tenant-2",
			SyncType:   SyncTypeFull,
			Status:     SyncStatusCompleted,
			LastSyncAt: time.Now(),
			CreatedAt:  time.Now(),
		}
		storage.SaveState(ctx, state)

		latest, err := storage.GetLatestState(ctx, "tenant-2")
		if err != nil {
			t.Fatalf("GetLatestState failed: %v", err)
		}

		if latest.SyncID != "sync-completed" {
			t.Errorf("got wrong state: %s", latest.SyncID)
		}
	})

	t.Run("ListStates", func(t *testing.T) {
		// Add another state for the same tenant.
		state := &SyncState{
			SyncID:    "sync-2",
			TenantID:  "tenant-2",
			SyncType:  SyncTypeIncremental,
			Status:    SyncStatusPending,
			CreatedAt: time.Now(),
		}
		storage.SaveState(ctx, state)

		states, err := storage.ListStates(ctx, "tenant-2", 10)
		if err != nil {
			t.Fatalf("ListStates failed: %v", err)
		}

		if len(states) < 2 {
			t.Errorf("expected at least 2 states, got %d", len(states))
		}
	})

	t.Run("DeleteState", func(t *testing.T) {
		err := storage.DeleteState(ctx, "sync-2")
		if err != nil {
			t.Fatalf("DeleteState failed: %v", err)
		}

		_, err = storage.GetStateByID(ctx, "sync-2")
		if err != ErrStateNotFound {
			t.Errorf("expected ErrStateNotFound after delete, got %v", err)
		}
	})

	t.Run("CleanupOldStates", func(t *testing.T) {
		// Add an old completed state.
		oldState := &SyncState{
			SyncID:    "old-sync",
			TenantID:  "tenant-3",
			SyncType:  SyncTypeFull,
			Status:    SyncStatusCompleted,
			CreatedAt: time.Now().Add(-48 * time.Hour),
		}
		storage.SaveState(ctx, oldState)

		count, err := storage.CleanupOldStates(ctx, 24*time.Hour)
		if err != nil {
			t.Fatalf("CleanupOldStates failed: %v", err)
		}

		if count != 1 {
			t.Errorf("expected 1 cleaned up, got %d", count)
		}

		_, err = storage.GetStateByID(ctx, "old-sync")
		if err != ErrStateNotFound {
			t.Errorf("expected old state to be deleted")
		}
	})
}

// TestRateLimiter tests the rate limiter.
func TestRateLimiter(t *testing.T) {
	t.Run("BasicOperation", func(t *testing.T) {
		limiter := NewRateLimiter(10, 5)
		ctx := context.Background()

		// Should immediately get 5 tokens (burst).
		for i := 0; i < 5; i++ {
			start := time.Now()
			err := limiter.Wait(ctx)
			if err != nil {
				t.Fatalf("Wait failed: %v", err)
			}
			if time.Since(start) > 100*time.Millisecond {
				t.Errorf("token %d took too long", i)
			}
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		limiter := NewRateLimiter(1, 0)
		ctx, cancel := context.WithCancel(context.Background())

		// Use up any available tokens.
		limiter.tokens = 0

		// Cancel context immediately.
		cancel()

		err := limiter.Wait(ctx)
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

// TestEngineCreation tests engine creation and configuration.
func TestEngineCreation(t *testing.T) {
	t.Run("MissingOAuth2Manager", func(t *testing.T) {
		_, err := NewEngine(&EngineConfig{
			StateStorage: NewMemoryStateStorage(),
		})
		if err == nil {
			t.Error("expected error for missing OAuth2Manager")
		}
	})

	t.Run("MissingStateStorage", func(t *testing.T) {
		oauthManager := setupTestOAuth(t, "test-tenant")
		_, err := NewEngine(&EngineConfig{
			OAuth2Manager: oauthManager,
		})
		if err == nil {
			t.Error("expected error for missing StateStorage")
		}
	})

	t.Run("ValidConfig", func(t *testing.T) {
		oauthManager := setupTestOAuth(t, "test-tenant")
		engine, err := NewEngine(&EngineConfig{
			OAuth2Manager: oauthManager,
			StateStorage:  NewMemoryStateStorage(),
		})
		if err != nil {
			t.Fatalf("NewEngine failed: %v", err)
		}

		if engine.config.BatchSize != DefaultBatchSize {
			t.Errorf("unexpected BatchSize: got %d, want %d", engine.config.BatchSize, DefaultBatchSize)
		}
	})

	t.Run("CustomConfig", func(t *testing.T) {
		oauthManager := setupTestOAuth(t, "test-tenant")
		engine, err := NewEngine(&EngineConfig{
			OAuth2Manager: oauthManager,
			StateStorage:  NewMemoryStateStorage(),
			BatchSize:     50,
			RateLimit:     100,
			MaxRetries:    3,
		})
		if err != nil {
			t.Fatalf("NewEngine failed: %v", err)
		}

		if engine.config.BatchSize != 50 {
			t.Errorf("unexpected BatchSize: got %d, want 50", engine.config.BatchSize)
		}
		if engine.config.RateLimit != 100 {
			t.Errorf("unexpected RateLimit: got %d, want 100", engine.config.RateLimit)
		}
	})
}

// TestFullSync tests the full sync operation with a mock server.
func TestFullSync(t *testing.T) {
	tenantID := "test-tenant"

	// Track API calls.
	var profileCalls int32
	var messageListCalls int32
	var messageGetCalls int32

	// Create a mock Gmail API server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-access-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch {
		case r.URL.Path == "/gmail/v1/users/me/profile":
			atomic.AddInt32(&profileCalls, 1)
			json.NewEncoder(w).Encode(Profile{
				EmailAddress:  "test@example.com",
				MessagesTotal: 2,
				ThreadsTotal:  1,
				HistoryID:     12345,
			})

		case r.URL.Path == "/gmail/v1/users/me/messages" && r.Method == "GET":
			atomic.AddInt32(&messageListCalls, 1)
			json.NewEncoder(w).Encode(MessageList{
				Messages: []MessageRef{
					{ID: "msg-1", ThreadID: "thread-1"},
					{ID: "msg-2", ThreadID: "thread-1"},
				},
				ResultSizeEstimate: 2,
			})

		case r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			atomic.AddInt32(&messageGetCalls, 1)
			json.NewEncoder(w).Encode(Message{
				ID:           "msg-1",
				ThreadID:     "thread-1",
				LabelIDs:     []string{"INBOX"},
				Snippet:      "Test message 1",
				InternalDate: "1609459200000",
				SizeEstimate: 1024,
				Payload: &MessagePayload{
					MimeType: "text/plain",
					Headers: []MessageHeader{
						{Name: "Subject", Value: "Test Subject 1"},
						{Name: "From", Value: "sender@example.com"},
						{Name: "To", Value: "test@example.com"},
					},
				},
			})

		case r.URL.Path == "/gmail/v1/users/me/messages/msg-2":
			atomic.AddInt32(&messageGetCalls, 1)
			json.NewEncoder(w).Encode(Message{
				ID:           "msg-2",
				ThreadID:     "thread-1",
				LabelIDs:     []string{"INBOX", "UNREAD"},
				Snippet:      "Test message 2",
				InternalDate: "1609545600000",
				SizeEstimate: 2048,
				Payload: &MessagePayload{
					MimeType: "text/plain",
					Headers: []MessageHeader{
						{Name: "Subject", Value: "Test Subject 2"},
						{Name: "From", Value: "sender@example.com"},
						{Name: "To", Value: "test@example.com"},
					},
				},
			})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Override the API base URL.
	originalBaseURL := GmailAPIBaseURL
	defer func() {
		// Can't restore const, but in real tests we'd use dependency injection.
	}()

	// Create the engine with the test server.
	oauthManager := setupTestOAuth(t, tenantID)
	stateStorage := NewMemoryStateStorage()

	engine, err := NewEngine(&EngineConfig{
		OAuth2Manager: oauthManager,
		StateStorage:  stateStorage,
		HTTPClient:    server.Client(),
		BatchSize:     10,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Replace the base URL by using a custom HTTP client that rewrites URLs.
	engine.config.HTTPClient = &http.Client{
		Transport: &rewriteTransport{
			base:       server.Client().Transport,
			serverURL:  server.URL,
			originalURL: originalBaseURL,
		},
	}

	// Track messages processed.
	var messagesProcessed []string
	opts := &SyncOptions{
		MessageHandler: func(ctx context.Context, msg *Message) error {
			messagesProcessed = append(messagesProcessed, msg.ID)
			return nil
		},
	}

	// Run the sync.
	ctx := context.Background()
	result, err := engine.FullSync(ctx, tenantID, opts)
	if err != nil {
		t.Fatalf("FullSync failed: %v", err)
	}

	// Verify results.
	if result.SyncType != SyncTypeFull {
		t.Errorf("unexpected sync type: %s", result.SyncType)
	}
	if result.ProcessedCount != 2 {
		t.Errorf("expected 2 processed, got %d", result.ProcessedCount)
	}
	if result.SuccessCount != 2 {
		t.Errorf("expected 2 success, got %d", result.SuccessCount)
	}
	if result.ErrorCount != 0 {
		t.Errorf("expected 0 errors, got %d", result.ErrorCount)
	}
	if len(messagesProcessed) != 2 {
		t.Errorf("expected 2 messages processed, got %d", len(messagesProcessed))
	}

	// Verify API calls.
	if atomic.LoadInt32(&profileCalls) != 1 {
		t.Errorf("expected 1 profile call, got %d", profileCalls)
	}
	if atomic.LoadInt32(&messageListCalls) != 1 {
		t.Errorf("expected 1 message list call, got %d", messageListCalls)
	}
	if atomic.LoadInt32(&messageGetCalls) != 2 {
		t.Errorf("expected 2 message get calls, got %d", messageGetCalls)
	}

	// Verify state was saved.
	state, err := stateStorage.GetStateByID(ctx, result.SyncID)
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if state.Status != SyncStatusCompleted {
		t.Errorf("expected completed status, got %s", state.Status)
	}
}

// TestIncrementalSync tests the incremental sync operation.
func TestIncrementalSync(t *testing.T) {
	tenantID := "test-tenant"

	// Create a mock server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-access-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch {
		case r.URL.Path == "/gmail/v1/users/me/history":
			json.NewEncoder(w).Encode(HistoryList{
				History: []History{
					{
						ID: 12346,
						MessagesAdded: []HistoryMessage{
							{Message: Message{ID: "new-msg-1"}},
						},
					},
				},
				HistoryID: 12346,
			})

		case r.URL.Path == "/gmail/v1/users/me/messages/new-msg-1":
			json.NewEncoder(w).Encode(Message{
				ID:           "new-msg-1",
				ThreadID:     "thread-2",
				LabelIDs:     []string{"INBOX"},
				Snippet:      "New message",
				InternalDate: "1609632000000",
				SizeEstimate: 512,
			})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Setup.
	oauthManager := setupTestOAuth(t, tenantID)
	stateStorage := NewMemoryStateStorage()

	// Save a previous sync state with history ID.
	prevState := &SyncState{
		SyncID:     "prev-sync",
		TenantID:   tenantID,
		SyncType:   SyncTypeFull,
		Status:     SyncStatusCompleted,
		HistoryID:  12345,
		LastSyncAt: time.Now().Add(-1 * time.Hour),
		CreatedAt:  time.Now().Add(-1 * time.Hour),
	}
	stateStorage.SaveState(context.Background(), prevState)

	engine, err := NewEngine(&EngineConfig{
		OAuth2Manager: oauthManager,
		StateStorage:  stateStorage,
		HTTPClient: &http.Client{
			Transport: &rewriteTransport{
				base:        server.Client().Transport,
				serverURL:   server.URL,
				originalURL: GmailAPIBaseURL,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	var messagesProcessed []string
	opts := &SyncOptions{
		MessageHandler: func(ctx context.Context, msg *Message) error {
			messagesProcessed = append(messagesProcessed, msg.ID)
			return nil
		},
	}

	result, err := engine.IncrementalSync(context.Background(), tenantID, opts)
	if err != nil {
		t.Fatalf("IncrementalSync failed: %v", err)
	}

	if result.SyncType != SyncTypeIncremental {
		t.Errorf("unexpected sync type: %s", result.SyncType)
	}
	if result.ProcessedCount != 1 {
		t.Errorf("expected 1 processed, got %d", result.ProcessedCount)
	}
	if len(messagesProcessed) != 1 {
		t.Errorf("expected 1 message, got %d", len(messagesProcessed))
	}
	if messagesProcessed[0] != "new-msg-1" {
		t.Errorf("unexpected message ID: %s", messagesProcessed[0])
	}
}

// TestIncrementalSyncFallbackToFull tests fallback when no previous sync exists.
func TestIncrementalSyncFallbackToFull(t *testing.T) {
	tenantID := "new-tenant"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-access-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch {
		case r.URL.Path == "/gmail/v1/users/me/profile":
			json.NewEncoder(w).Encode(Profile{
				EmailAddress:  "test@example.com",
				MessagesTotal: 1,
				HistoryID:     1000,
			})

		case r.URL.Path == "/gmail/v1/users/me/messages" && r.Method == "GET":
			json.NewEncoder(w).Encode(MessageList{
				Messages: []MessageRef{
					{ID: "msg-1"},
				},
			})

		case r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			json.NewEncoder(w).Encode(Message{ID: "msg-1"})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	oauthManager := setupTestOAuth(t, tenantID)
	stateStorage := NewMemoryStateStorage()

	engine, err := NewEngine(&EngineConfig{
		OAuth2Manager: oauthManager,
		StateStorage:  stateStorage,
		HTTPClient: &http.Client{
			Transport: &rewriteTransport{
				base:        server.Client().Transport,
				serverURL:   server.URL,
				originalURL: GmailAPIBaseURL,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	result, err := engine.IncrementalSync(context.Background(), tenantID, nil)
	if err != nil {
		t.Fatalf("IncrementalSync failed: %v", err)
	}

	// Should fall back to full sync.
	if result.SyncType != SyncTypeFull {
		t.Errorf("expected full sync fallback, got %s", result.SyncType)
	}
}

// TestCancelSync tests sync cancellation.
func TestCancelSync(t *testing.T) {
	ctx := context.Background()
	stateStorage := NewMemoryStateStorage()

	// Create an in-progress sync.
	state := &SyncState{
		SyncID:   "cancel-test",
		TenantID: "tenant-1",
		Status:   SyncStatusInProgress,
	}
	stateStorage.SaveState(ctx, state)

	oauthManager := setupTestOAuth(t, "tenant-1")
	engine, _ := NewEngine(&EngineConfig{
		OAuth2Manager: oauthManager,
		StateStorage:  stateStorage,
	})

	err := engine.CancelSync(ctx, "cancel-test")
	if err != nil {
		t.Fatalf("CancelSync failed: %v", err)
	}

	updated, _ := stateStorage.GetStateByID(ctx, "cancel-test")
	if updated.Status != SyncStatusCancelled {
		t.Errorf("expected cancelled status, got %s", updated.Status)
	}
}

// TestCancelCompletedSync tests that completed syncs cannot be cancelled.
func TestCancelCompletedSync(t *testing.T) {
	ctx := context.Background()
	stateStorage := NewMemoryStateStorage()

	state := &SyncState{
		SyncID:   "completed-sync",
		TenantID: "tenant-1",
		Status:   SyncStatusCompleted,
	}
	stateStorage.SaveState(ctx, state)

	oauthManager := setupTestOAuth(t, "tenant-1")
	engine, _ := NewEngine(&EngineConfig{
		OAuth2Manager: oauthManager,
		StateStorage:  stateStorage,
	})

	err := engine.CancelSync(ctx, "completed-sync")
	if err == nil {
		t.Error("expected error when cancelling completed sync")
	}
}

// TestParseMessage tests message parsing.
func TestParseMessage(t *testing.T) {
	msg := &Message{
		ID:           "test-msg",
		ThreadID:     "test-thread",
		LabelIDs:     []string{"INBOX", "STARRED"},
		Snippet:      "Test snippet",
		InternalDate: "1609459200000",
		SizeEstimate: 1024,
		HistoryID:    12345,
		Payload: &MessagePayload{
			MimeType: "multipart/mixed",
			Headers: []MessageHeader{
				{Name: "Subject", Value: "Test Subject"},
				{Name: "From", Value: "sender@example.com"},
				{Name: "To", Value: "recipient@example.com, another@example.com"},
				{Name: "Cc", Value: "cc@example.com"},
				{Name: "Message-ID", Value: "<abc123@example.com>"},
				{Name: "In-Reply-To", Value: "<parent@example.com>"},
			},
		},
	}

	parsed := ParseMessage(msg)

	if parsed.ID != "test-msg" {
		t.Errorf("ID mismatch: %s", parsed.ID)
	}
	if parsed.Subject != "Test Subject" {
		t.Errorf("Subject mismatch: %s", parsed.Subject)
	}
	if parsed.From != "sender@example.com" {
		t.Errorf("From mismatch: %s", parsed.From)
	}
	if len(parsed.To) != 2 {
		t.Errorf("expected 2 To addresses, got %d", len(parsed.To))
	}
	if len(parsed.CC) != 1 {
		t.Errorf("expected 1 CC address, got %d", len(parsed.CC))
	}
	if !parsed.IsStarred {
		t.Error("expected IsStarred to be true")
	}
	if !parsed.IsRead {
		t.Error("expected IsRead to be true (no UNREAD label)")
	}
	if parsed.MessageID != "<abc123@example.com>" {
		t.Errorf("MessageID mismatch: %s", parsed.MessageID)
	}
	if parsed.HistoryID != 12345 {
		t.Errorf("HistoryID mismatch: %d", parsed.HistoryID)
	}
}

// TestParseMessageUnread tests parsing unread messages.
func TestParseMessageUnread(t *testing.T) {
	msg := &Message{
		ID:       "unread-msg",
		LabelIDs: []string{"INBOX", "UNREAD"},
	}

	parsed := ParseMessage(msg)

	if parsed.IsRead {
		t.Error("expected IsRead to be false for UNREAD label")
	}
}

// TestAPIError tests APIError methods.
func TestAPIError(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		err := &APIError{
			Code:    404,
			Message: "Not Found",
			Status:  "NOT_FOUND",
		}
		expected := "Gmail API error 404 (NOT_FOUND): Not Found"
		if err.Error() != expected {
			t.Errorf("got %s, want %s", err.Error(), expected)
		}
	})

	t.Run("IsHistoryIDExpired", func(t *testing.T) {
		err := &APIError{
			Status:  "INVALID_ARGUMENT",
			Message: "Invalid historyId",
		}
		if !err.IsHistoryIDExpired() {
			t.Error("expected IsHistoryIDExpired to be true")
		}

		err2 := &APIError{
			Status:  "NOT_FOUND",
			Message: "Message not found",
		}
		if err2.IsHistoryIDExpired() {
			t.Error("expected IsHistoryIDExpired to be false")
		}
	})
}

// TestSyncMetrics tests metrics creation and registration.
func TestSyncMetrics(t *testing.T) {
	metrics := NewSyncMetrics("test", "gmail")

	if metrics.SyncsStarted == nil {
		t.Error("SyncsStarted is nil")
	}
	if metrics.SyncsCompleted == nil {
		t.Error("SyncsCompleted is nil")
	}
	if metrics.MessagesProcessed == nil {
		t.Error("MessagesProcessed is nil")
	}
	if metrics.APIRequests == nil {
		t.Error("APIRequests is nil")
	}
	if metrics.APILatency == nil {
		t.Error("APILatency is nil")
	}
}

// rewriteTransport rewrites request URLs for testing.
type rewriteTransport struct {
	base        http.RoundTripper
	serverURL   string
	originalURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the URL to point to our test server.
	newURL := t.serverURL + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}

	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header

	transport := t.base
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(newReq)
}

// TestContextCancellation tests that sync respects context cancellation.
func TestContextCancellation(t *testing.T) {
	tenantID := "test-tenant"

	// Create a server that delays responses.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow API.
		time.Sleep(500 * time.Millisecond)
		json.NewEncoder(w).Encode(Profile{
			EmailAddress:  "test@example.com",
			MessagesTotal: 1000,
			HistoryID:     1000,
		})
	}))
	defer server.Close()

	oauthManager := setupTestOAuth(t, tenantID)
	stateStorage := NewMemoryStateStorage()

	engine, _ := NewEngine(&EngineConfig{
		OAuth2Manager: oauthManager,
		StateStorage:  stateStorage,
		HTTPClient: &http.Client{
			Transport: &rewriteTransport{
				base:        server.Client().Transport,
				serverURL:   server.URL,
				originalURL: GmailAPIBaseURL,
			},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := engine.FullSync(ctx, tenantID, nil)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

// BenchmarkRateLimiter benchmarks the rate limiter.
func BenchmarkRateLimiter(b *testing.B) {
	limiter := NewRateLimiter(1000, 100)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Wait(ctx)
	}
}
