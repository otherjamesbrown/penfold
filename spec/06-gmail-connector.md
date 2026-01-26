# Gmail Connector Specification

## Overview

The Gmail Connector handles OAuth2 authentication, email synchronization, and attachment processing for Gmail accounts. It supports multiple Gmail accounts per tenant with incremental sync, real-time push notifications, and comprehensive rate limiting.

## Status: Planned (Phase 3)

## Responsibilities

1. **OAuth2 Management**: Token storage, refresh, revocation with PKCE flow
2. **Email Sync**: Historical and incremental via Gmail History API
3. **Real-time**: Push notifications via Google Cloud Pub/Sub
4. **Attachments**: Download, extract content, deduplicate by hash
5. **Multi-account**: Support multiple Gmail accounts per tenant
6. **Rate Limiting**: Respect Gmail API quotas (250 req/sec)
7. **Privacy**: Label-based exclusion, sender filtering

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Gmail Connector                                  │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                      gRPC Server (:8082)                        │    │
│  └──────────────────────────┬─────────────────────────────────────┘    │
│                             │                                           │
│  ┌──────────────────────────┼──────────────────────────────────────┐   │
│  │                          ▼                                       │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │    OAuth     │  │    Sync      │  │  Attachment  │          │   │
│  │  │   Manager    │  │   Engine     │  │  Processor   │          │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │   │
│  │         │                 │                 │                   │   │
│  │         └─────────────────┼─────────────────┘                   │   │
│  │                           ▼                                      │   │
│  │  ┌─────────────────────────────────────────────────────────┐   │   │
│  │  │                  Gmail API Client                        │   │   │
│  │  │          (rate-limited, retry-aware)                    │   │   │
│  │  └─────────────────────────────────────────────────────────┘   │   │
│  └──────────────────────────┬──────────────────────────────────────┘   │
│                             │                                           │
│         ┌───────────────────┼───────────────────┐                      │
│         ▼                   ▼                   ▼                       │
│  ┌───────────┐       ┌───────────┐       ┌───────────┐                │
│  │PostgreSQL │       │   Redis   │       │ Gmail API │                │
│  │ (tokens,  │       │  (cache,  │       │           │                │
│  │  state)   │       │   queue)  │       │           │                │
│  └───────────┘       └───────────┘       └───────────┘                │
└─────────────────────────────────────────────────────────────────────────┘
```

## gRPC Service Definition

```protobuf
// api/proto/gmail/v1/gmail.proto

syntax = "proto3";
package gmail.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/empty.proto";

service GmailService {
  // Account management
  rpc GetAuthURL(GetAuthURLRequest) returns (GetAuthURLResponse);
  rpc ExchangeToken(ExchangeTokenRequest) returns (ExchangeTokenResponse);
  rpc ConnectAccount(ConnectAccountRequest) returns (ConnectAccountResponse);
  rpc DisconnectAccount(DisconnectAccountRequest) returns (google.protobuf.Empty);
  rpc ListAccounts(ListAccountsRequest) returns (ListAccountsResponse);
  rpc GetAccount(GetAccountRequest) returns (GmailAccount);

  // Sync operations
  rpc TriggerSync(TriggerSyncRequest) returns (TriggerSyncResponse);
  rpc GetSyncStatus(GetSyncStatusRequest) returns (GetSyncStatusResponse);
  rpc CancelSync(CancelSyncRequest) returns (google.protobuf.Empty);

  // Push notifications
  rpc RegisterWatch(RegisterWatchRequest) returns (RegisterWatchResponse);
  rpc HandlePushNotification(PushNotificationRequest) returns (google.protobuf.Empty);

  // Privacy
  rpc SetPrivacyRules(SetPrivacyRulesRequest) returns (google.protobuf.Empty);
  rpc GetPrivacyRules(GetPrivacyRulesRequest) returns (PrivacyRules);

  // Scheduler (intelligent scheduling)
  rpc GetSchedulerStatus(GetSchedulerStatusRequest) returns (GetSchedulerStatusResponse);
  rpc SetAccountPriority(SetAccountPriorityRequest) returns (google.protobuf.Empty);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}

message SetAccountPriorityRequest {
  string tenant_id = 1;
  string account_id = 2;
  int32 priority = 3;  // 1-10
}

// Auth messages
message GetAuthURLRequest {
  string tenant_id = 1;
  string redirect_uri = 2;
  repeated string scopes = 3;
}

message GetAuthURLResponse {
  string auth_url = 1;
  string state = 2;
  string code_verifier = 3;  // For PKCE - store securely client-side
}

message ExchangeTokenRequest {
  string tenant_id = 1;
  string code = 2;
  string state = 3;
  string code_verifier = 4;
  string redirect_uri = 5;
}

message ExchangeTokenResponse {
  bool success = 1;
  string account_email = 2;
  string error = 3;
}

message ConnectAccountRequest {
  string tenant_id = 1;
  string account_email = 2;
  bytes encrypted_token = 3;
  SyncConfig sync_config = 4;
}

message ConnectAccountResponse {
  string account_id = 1;
  bool success = 2;
}

message DisconnectAccountRequest {
  string tenant_id = 1;
  string account_id = 2;
  bool revoke_token = 3;  // Also revoke at Google
}

message ListAccountsRequest {
  string tenant_id = 1;
}

message ListAccountsResponse {
  repeated GmailAccount accounts = 1;
}

message GetAccountRequest {
  string tenant_id = 1;
  string account_id = 2;
}

message GmailAccount {
  string id = 1;
  string tenant_id = 2;
  string email = 3;
  AccountStatus status = 4;
  SyncState sync_state = 5;
  google.protobuf.Timestamp connected_at = 6;
  google.protobuf.Timestamp last_sync_at = 7;
  int64 message_count = 8;
  int64 thread_count = 9;
}

enum AccountStatus {
  ACCOUNT_STATUS_UNSPECIFIED = 0;
  ACCOUNT_STATUS_ACTIVE = 1;
  ACCOUNT_STATUS_TOKEN_EXPIRED = 2;
  ACCOUNT_STATUS_DISCONNECTED = 3;
  ACCOUNT_STATUS_ERROR = 4;
}

// Sync messages
message TriggerSyncRequest {
  string tenant_id = 1;
  string account_id = 2;
  SyncType sync_type = 3;
  SyncOptions options = 4;
}

enum SyncType {
  SYNC_TYPE_UNSPECIFIED = 0;
  SYNC_TYPE_INCREMENTAL = 1;
  SYNC_TYPE_FULL = 2;
  SYNC_TYPE_LABELS_ONLY = 3;
}

message SyncOptions {
  int32 max_messages = 1;      // Limit for full sync
  int32 history_days = 2;       // How far back to sync
  repeated string labels = 3;   // Only sync these labels
  bool include_attachments = 4;
}

message TriggerSyncResponse {
  string sync_id = 1;
  bool started = 2;
  string message = 3;
}

message GetSyncStatusRequest {
  string tenant_id = 1;
  string account_id = 2;
  string sync_id = 3;  // Optional, get latest if empty
}

message GetSyncStatusResponse {
  string sync_id = 1;
  SyncState state = 2;
  int64 messages_processed = 3;
  int64 messages_total = 4;
  int64 attachments_processed = 5;
  float progress_percent = 6;
  google.protobuf.Timestamp started_at = 7;
  google.protobuf.Timestamp completed_at = 8;
  string error = 9;
  SyncStats stats = 10;
}

message SyncState {
  string current_history_id = 1;
  google.protobuf.Timestamp last_sync = 2;
  bool is_syncing = 3;
  string last_error = 4;
}

message SyncStats {
  int64 new_messages = 1;
  int64 updated_messages = 2;
  int64 deleted_messages = 3;
  int64 new_threads = 4;
  int64 attachments_downloaded = 5;
  int64 bytes_downloaded = 6;
}

message CancelSyncRequest {
  string tenant_id = 1;
  string sync_id = 2;
}

// Push notification messages
message RegisterWatchRequest {
  string tenant_id = 1;
  string account_id = 2;
  string topic_name = 3;  // Google Cloud Pub/Sub topic
}

message RegisterWatchResponse {
  string watch_id = 1;
  google.protobuf.Timestamp expiration = 2;
}

message PushNotificationRequest {
  string message_data = 1;  // Base64-encoded push data
}

// Privacy messages
message SetPrivacyRulesRequest {
  string tenant_id = 1;
  string account_id = 2;
  PrivacyRules rules = 3;
}

message GetPrivacyRulesRequest {
  string tenant_id = 1;
  string account_id = 2;
}

message PrivacyRules {
  repeated string excluded_labels = 1;    // Don't sync these labels
  repeated string excluded_senders = 2;   // Don't sync from these
  repeated string excluded_domains = 3;   // Don't sync from these domains
  repeated string excluded_patterns = 4;  // Subject patterns to skip
  bool exclude_spam = 5;
  bool exclude_trash = 6;
  bool exclude_promotions = 7;
}

message SyncConfig {
  bool auto_sync = 1;
  int32 sync_interval_minutes = 2;
  int32 history_days = 3;
  bool enable_push = 4;
  PrivacyRules privacy_rules = 5;
  int32 priority = 6;                    // Account priority (1-10, higher = more frequent)
  bool adaptive_scheduling = 7;          // Enable activity-based scheduling
}

// Scheduler messages (for intelligent scheduling)
message GetSchedulerStatusRequest {
  string tenant_id = 1;
}

message GetSchedulerStatusResponse {
  repeated AccountScheduleInfo accounts = 1;
  int64 next_sync_in_seconds = 2;
}

message AccountScheduleInfo {
  string account_id = 1;
  string email = 2;
  int32 priority_score = 3;
  int32 current_interval_minutes = 4;
  int32 messages_per_day = 5;
  float activity_level = 6;             // 0.0 to 1.0
  google.protobuf.Timestamp next_sync_at = 7;
  google.protobuf.Timestamp last_activity = 8;
}

// Health
message HealthRequest {}
message HealthResponse {
  bool healthy = 1;
  map<string, ComponentHealth> components = 2;
}

message ComponentHealth {
  bool healthy = 1;
  string status = 2;
  int64 latency_ms = 3;
}
```

## OAuth2 Implementation

### PKCE Flow

```go
// internal/oauth/oauth.go

package oauth

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "time"

    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    "google.golang.org/api/gmail/v1"
)

type OAuthManager struct {
    config       *oauth2.Config
    tokenStore   TokenStore
    stateStore   StateStore
    encryptor    Encryptor
}

type OAuthConfig struct {
    ClientID     string
    ClientSecret string
    RedirectURI  string
    Scopes       []string
}

func NewOAuthManager(cfg *OAuthConfig, tokenStore TokenStore, stateStore StateStore, encryptor Encryptor) *OAuthManager {
    scopes := cfg.Scopes
    if len(scopes) == 0 {
        scopes = []string{
            gmail.GmailReadonlyScope,
            gmail.GmailLabelsReadonlyScope,
            "https://www.googleapis.com/auth/userinfo.email",
        }
    }

    return &OAuthManager{
        config: &oauth2.Config{
            ClientID:     cfg.ClientID,
            ClientSecret: cfg.ClientSecret,
            RedirectURL:  cfg.RedirectURI,
            Scopes:       scopes,
            Endpoint:     google.Endpoint,
        },
        tokenStore: tokenStore,
        stateStore: stateStore,
        encryptor:  encryptor,
    }
}

// GenerateCodeVerifier creates a PKCE code verifier
func GenerateCodeVerifier() (string, error) {
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateCodeChallenge creates PKCE code challenge from verifier
func GenerateCodeChallenge(verifier string) string {
    h := sha256.Sum256([]byte(verifier))
    return base64.RawURLEncoding.EncodeToString(h[:])
}

func (m *OAuthManager) GetAuthURL(ctx context.Context, tenantID string) (*AuthURLResponse, error) {
    // Generate state for CSRF protection
    state, err := generateSecureToken(32)
    if err != nil {
        return nil, fmt.Errorf("failed to generate state: %w", err)
    }

    // Generate PKCE verifier
    verifier, err := GenerateCodeVerifier()
    if err != nil {
        return nil, fmt.Errorf("failed to generate verifier: %w", err)
    }

    challenge := GenerateCodeChallenge(verifier)

    // Store state -> tenantID mapping (expires in 10 minutes)
    if err := m.stateStore.Set(ctx, state, &StateData{
        TenantID:  tenantID,
        CreatedAt: time.Now(),
    }, 10*time.Minute); err != nil {
        return nil, fmt.Errorf("failed to store state: %w", err)
    }

    url := m.config.AuthCodeURL(state,
        oauth2.AccessTypeOffline,
        oauth2.SetAuthURLParam("prompt", "consent"),
        oauth2.SetAuthURLParam("code_challenge", challenge),
        oauth2.SetAuthURLParam("code_challenge_method", "S256"),
    )

    return &AuthURLResponse{
        AuthURL:      url,
        State:        state,
        CodeVerifier: verifier,
    }, nil
}

func (m *OAuthManager) ExchangeToken(ctx context.Context, req *ExchangeRequest) (*ExchangeResponse, error) {
    // Verify state
    stateData, err := m.stateStore.Get(ctx, req.State)
    if err != nil {
        return nil, fmt.Errorf("invalid state: %w", err)
    }

    // Delete state (one-time use)
    m.stateStore.Delete(ctx, req.State)

    // Exchange code for token with PKCE verifier
    token, err := m.config.Exchange(ctx, req.Code,
        oauth2.SetAuthURLParam("code_verifier", req.CodeVerifier),
    )
    if err != nil {
        return nil, fmt.Errorf("token exchange failed: %w", err)
    }

    // Get user email from Gmail API
    client := m.config.Client(ctx, token)
    gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(client))
    if err != nil {
        return nil, fmt.Errorf("failed to create gmail service: %w", err)
    }

    profile, err := gmailSvc.Users.GetProfile("me").Do()
    if err != nil {
        return nil, fmt.Errorf("failed to get profile: %w", err)
    }

    // Encrypt and store token
    encryptedToken, err := m.encryptor.Encrypt(token)
    if err != nil {
        return nil, fmt.Errorf("failed to encrypt token: %w", err)
    }

    if err := m.tokenStore.Store(ctx, stateData.TenantID, profile.EmailAddress, encryptedToken); err != nil {
        return nil, fmt.Errorf("failed to store token: %w", err)
    }

    return &ExchangeResponse{
        Success:      true,
        AccountEmail: profile.EmailAddress,
        TenantID:     stateData.TenantID,
    }, nil
}

func (m *OAuthManager) GetClient(ctx context.Context, tenantID, accountEmail string) (*gmail.Service, error) {
    encryptedToken, err := m.tokenStore.Get(ctx, tenantID, accountEmail)
    if err != nil {
        return nil, fmt.Errorf("failed to get token: %w", err)
    }

    token, err := m.encryptor.Decrypt(encryptedToken)
    if err != nil {
        return nil, fmt.Errorf("failed to decrypt token: %w", err)
    }

    // Create token source that auto-refreshes
    tokenSource := m.config.TokenSource(ctx, token)

    // Wrap to detect refresh and persist new token
    persistingSource := &persistingTokenSource{
        source:     tokenSource,
        store:      m.tokenStore,
        encryptor:  m.encryptor,
        tenantID:   tenantID,
        email:      accountEmail,
        lastToken:  token,
    }

    client := oauth2.NewClient(ctx, persistingSource)
    return gmail.NewService(ctx, option.WithHTTPClient(client))
}

type persistingTokenSource struct {
    source    oauth2.TokenSource
    store     TokenStore
    encryptor Encryptor
    tenantID  string
    email     string
    lastToken *oauth2.Token
    mu        sync.Mutex
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    token, err := s.source.Token()
    if err != nil {
        return nil, err
    }

    // Check if token was refreshed
    if token.AccessToken != s.lastToken.AccessToken {
        encrypted, err := s.encryptor.Encrypt(token)
        if err != nil {
            slog.Warn("failed to encrypt refreshed token", "error", err)
        } else {
            if err := s.store.Store(context.Background(), s.tenantID, s.email, encrypted); err != nil {
                slog.Warn("failed to persist refreshed token", "error", err)
            }
        }
        s.lastToken = token
    }

    return token, nil
}
```

### Token Encryption

```go
// internal/oauth/encryption.go

package oauth

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/json"
    "fmt"
    "io"

    "golang.org/x/oauth2"
)

type AESEncryptor struct {
    key []byte  // 32 bytes for AES-256
}

func NewAESEncryptor(key []byte) (*AESEncryptor, error) {
    if len(key) != 32 {
        return nil, fmt.Errorf("key must be 32 bytes for AES-256")
    }
    return &AESEncryptor{key: key}, nil
}

func (e *AESEncryptor) Encrypt(token *oauth2.Token) ([]byte, error) {
    plaintext, err := json.Marshal(token)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal token: %w", err)
    }

    block, err := aes.NewCipher(e.key)
    if err != nil {
        return nil, fmt.Errorf("failed to create cipher: %w", err)
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, fmt.Errorf("failed to create GCM: %w", err)
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, fmt.Errorf("failed to generate nonce: %w", err)
    }

    ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
    return ciphertext, nil
}

func (e *AESEncryptor) Decrypt(ciphertext []byte) (*oauth2.Token, error) {
    block, err := aes.NewCipher(e.key)
    if err != nil {
        return nil, fmt.Errorf("failed to create cipher: %w", err)
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, fmt.Errorf("failed to create GCM: %w", err)
    }

    nonceSize := gcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return nil, fmt.Errorf("ciphertext too short")
    }

    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to decrypt: %w", err)
    }

    var token oauth2.Token
    if err := json.Unmarshal(plaintext, &token); err != nil {
        return nil, fmt.Errorf("failed to unmarshal token: %w", err)
    }

    return &token, nil
}
```

## Sync Engine

### Incremental Sync (History API)

```go
// internal/sync/engine.go

package sync

import (
    "context"
    "fmt"
    "log/slog"
    "sync"
    "time"

    "google.golang.org/api/gmail/v1"
)

type SyncEngine struct {
    oauth        *oauth.OAuthManager
    db           *pgxpool.Pool
    publisher    *events.Publisher
    rateLimit    *RateLimiter
    attachments  *AttachmentProcessor
    privacy      *PrivacyFilter
}

type SyncJob struct {
    ID          string
    TenantID    string
    AccountID   string
    Email       string
    Type        SyncType
    Options     SyncOptions
    State       SyncState
    Stats       SyncStats
    StartedAt   time.Time
    CompletedAt *time.Time
    Error       *string
    cancel      context.CancelFunc
}

func (e *SyncEngine) StartSync(ctx context.Context, job *SyncJob) error {
    ctx, cancel := context.WithCancel(ctx)
    job.cancel = cancel

    // Get Gmail client
    gmailSvc, err := e.oauth.GetClient(ctx, job.TenantID, job.Email)
    if err != nil {
        return fmt.Errorf("failed to get gmail client: %w", err)
    }

    switch job.Type {
    case SyncTypeIncremental:
        return e.incrementalSync(ctx, gmailSvc, job)
    case SyncTypeFull:
        return e.fullSync(ctx, gmailSvc, job)
    case SyncTypeLabelsOnly:
        return e.syncLabels(ctx, gmailSvc, job)
    default:
        return fmt.Errorf("unknown sync type: %v", job.Type)
    }
}

func (e *SyncEngine) incrementalSync(ctx context.Context, svc *gmail.Service, job *SyncJob) error {
    // Get last known history ID
    lastHistoryID := job.State.CurrentHistoryID
    if lastHistoryID == "" {
        // No history, need full sync
        return e.fullSync(ctx, svc, job)
    }

    slog.Info("starting incremental sync",
        "account", job.Email,
        "history_id", lastHistoryID,
    )

    // Fetch history changes
    historyCall := svc.Users.History.List("me").
        StartHistoryId(parseHistoryID(lastHistoryID)).
        HistoryTypes("messageAdded", "messageDeleted", "labelAdded", "labelRemoved")

    var newHistoryID string
    var messagesAdded, messagesDeleted int64

    err := historyCall.Pages(ctx, func(resp *gmail.ListHistoryResponse) error {
        if err := e.rateLimit.Wait(ctx); err != nil {
            return err
        }

        newHistoryID = fmt.Sprintf("%d", resp.HistoryId)

        for _, history := range resp.History {
            // Process added messages
            for _, added := range history.MessagesAdded {
                if e.privacy.ShouldSkip(added.Message) {
                    continue
                }

                if err := e.processMessage(ctx, svc, job, added.Message.Id); err != nil {
                    slog.Error("failed to process message", "id", added.Message.Id, "error", err)
                    continue
                }
                messagesAdded++
                job.Stats.NewMessages++
            }

            // Process deleted messages
            for _, deleted := range history.MessagesDeleted {
                if err := e.markMessageDeleted(ctx, job, deleted.Message.Id); err != nil {
                    slog.Error("failed to mark deleted", "id", deleted.Message.Id, "error", err)
                    continue
                }
                messagesDeleted++
                job.Stats.DeletedMessages++
            }

            // Process label changes
            for _, labelAdded := range history.LabelsAdded {
                e.updateMessageLabels(ctx, job, labelAdded.Message.Id, labelAdded.LabelIds, nil)
            }
            for _, labelRemoved := range history.LabelsRemoved {
                e.updateMessageLabels(ctx, job, labelRemoved.Message.Id, nil, labelRemoved.LabelIds)
            }
        }

        return nil
    })

    if err != nil {
        // Check for history expired error
        if isHistoryExpired(err) {
            slog.Warn("history expired, falling back to full sync", "account", job.Email)
            return e.fullSync(ctx, svc, job)
        }
        return fmt.Errorf("history list failed: %w", err)
    }

    // Update history ID
    if newHistoryID != "" {
        job.State.CurrentHistoryID = newHistoryID
        if err := e.updateSyncState(ctx, job); err != nil {
            return fmt.Errorf("failed to update sync state: %w", err)
        }
    }

    slog.Info("incremental sync complete",
        "account", job.Email,
        "added", messagesAdded,
        "deleted", messagesDeleted,
        "new_history_id", newHistoryID,
    )

    return nil
}

func (e *SyncEngine) fullSync(ctx context.Context, svc *gmail.Service, job *SyncJob) error {
    slog.Info("starting full sync", "account", job.Email, "history_days", job.Options.HistoryDays)

    // Get current history ID first
    profile, err := svc.Users.GetProfile("me").Do()
    if err != nil {
        return fmt.Errorf("failed to get profile: %w", err)
    }
    currentHistoryID := fmt.Sprintf("%d", profile.HistoryId)

    // Build query for date range
    query := ""
    if job.Options.HistoryDays > 0 {
        after := time.Now().AddDate(0, 0, -job.Options.HistoryDays)
        query = fmt.Sprintf("after:%s", after.Format("2006/01/02"))
    }

    // Add label filter if specified
    if len(job.Options.Labels) > 0 {
        for _, label := range job.Options.Labels {
            query += fmt.Sprintf(" label:%s", label)
        }
    }

    listCall := svc.Users.Messages.List("me")
    if query != "" {
        listCall = listCall.Q(query)
    }

    var totalProcessed int64
    var messageIDs []string

    // First pass: collect message IDs
    err = listCall.Pages(ctx, func(resp *gmail.ListMessagesResponse) error {
        if err := e.rateLimit.Wait(ctx); err != nil {
            return err
        }

        for _, msg := range resp.Messages {
            messageIDs = append(messageIDs, msg.Id)
        }

        // Check limit
        if job.Options.MaxMessages > 0 && len(messageIDs) >= int(job.Options.MaxMessages) {
            messageIDs = messageIDs[:job.Options.MaxMessages]
            return fmt.Errorf("limit reached")  // Break pagination
        }

        return nil
    })

    // Ignore "limit reached" error
    if err != nil && err.Error() != "limit reached" {
        return fmt.Errorf("failed to list messages: %w", err)
    }

    job.Stats.MessagesTotal = int64(len(messageIDs))
    slog.Info("found messages to sync", "count", len(messageIDs))

    // Second pass: process messages in batches
    batchSize := 50
    for i := 0; i < len(messageIDs); i += batchSize {
        end := i + batchSize
        if end > len(messageIDs) {
            end = len(messageIDs)
        }

        batch := messageIDs[i:end]

        // Process batch concurrently
        var wg sync.WaitGroup
        errChan := make(chan error, len(batch))

        for _, msgID := range batch {
            wg.Add(1)
            go func(id string) {
                defer wg.Done()
                if err := e.rateLimit.Wait(ctx); err != nil {
                    errChan <- err
                    return
                }
                if err := e.processMessage(ctx, svc, job, id); err != nil {
                    errChan <- err
                }
            }(msgID)
        }

        wg.Wait()
        close(errChan)

        // Collect errors but continue
        for err := range errChan {
            slog.Error("batch processing error", "error", err)
        }

        totalProcessed += int64(len(batch))
        job.Stats.MessagesProcessed = totalProcessed

        // Update progress
        e.publishProgress(ctx, job)
    }

    // Update to current history ID
    job.State.CurrentHistoryID = currentHistoryID
    if err := e.updateSyncState(ctx, job); err != nil {
        return fmt.Errorf("failed to update sync state: %w", err)
    }

    slog.Info("full sync complete",
        "account", job.Email,
        "processed", totalProcessed,
        "history_id", currentHistoryID,
    )

    return nil
}

func (e *SyncEngine) processMessage(ctx context.Context, svc *gmail.Service, job *SyncJob, messageID string) error {
    // Get full message
    msg, err := svc.Users.Messages.Get("me", messageID).
        Format("full").
        Do()
    if err != nil {
        return fmt.Errorf("failed to get message: %w", err)
    }

    // Apply privacy filter
    if e.privacy.ShouldSkip(msg) {
        return nil
    }

    // Parse message
    parsed, err := parseMessage(msg)
    if err != nil {
        return fmt.Errorf("failed to parse message: %w", err)
    }

    // Store in database
    if err := e.storeMessage(ctx, job, parsed); err != nil {
        return fmt.Errorf("failed to store message: %w", err)
    }

    // Process attachments if enabled
    if job.Options.IncludeAttachments && len(msg.Payload.Parts) > 0 {
        for _, part := range msg.Payload.Parts {
            if part.Body.AttachmentId != "" {
                if err := e.attachments.Process(ctx, svc, job, messageID, part); err != nil {
                    slog.Warn("failed to process attachment",
                        "message_id", messageID,
                        "attachment_id", part.Body.AttachmentId,
                        "error", err,
                    )
                }
            }
        }
    }

    // Publish event
    e.publisher.Publish(ctx, "email.ingested", &events.EmailIngestedEvent{
        TenantID:     job.TenantID,
        MessageID:    messageID,
        ThreadID:     msg.ThreadId,
        AccountEmail: job.Email,
        Subject:      parsed.Subject,
        Snippet:      msg.Snippet,
        Labels:       msg.LabelIds,
        ReceivedAt:   parsed.Date,
    })

    return nil
}
```

### Rate Limiter

```go
// internal/sync/ratelimit.go

package sync

import (
    "context"
    "sync"
    "time"

    "golang.org/x/time/rate"
)

type RateLimiter struct {
    limiter    *rate.Limiter
    burstLimit int
    mu         sync.Mutex
}

func NewRateLimiter(requestsPerSecond float64, burstSize int) *RateLimiter {
    return &RateLimiter{
        limiter:    rate.NewLimiter(rate.Limit(requestsPerSecond), burstSize),
        burstLimit: burstSize,
    }
}

func (r *RateLimiter) Wait(ctx context.Context) error {
    return r.limiter.Wait(ctx)
}

func (r *RateLimiter) Allow() bool {
    return r.limiter.Allow()
}

// AdaptiveRateLimiter adjusts rate based on API responses
type AdaptiveRateLimiter struct {
    base       *RateLimiter
    minRate    float64
    maxRate    float64
    currentRate float64
    mu         sync.Mutex
}

func (r *AdaptiveRateLimiter) OnSuccess() {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Gradually increase rate on success
    r.currentRate = min(r.currentRate*1.1, r.maxRate)
    r.base.limiter.SetLimit(rate.Limit(r.currentRate))
}

func (r *AdaptiveRateLimiter) OnRateLimit() {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Quickly reduce rate on rate limit error
    r.currentRate = max(r.currentRate*0.5, r.minRate)
    r.base.limiter.SetLimit(rate.Limit(r.currentRate))
}
```

## Intelligent Scheduler

```go
// internal/scheduler/scheduler.go

package scheduler

import (
    "context"
    "log/slog"
    "sort"
    "sync"
    "time"
)

// IntelligentScheduler manages sync timing based on account activity
type IntelligentScheduler struct {
    db          *pgxpool.Pool
    syncEngine  *sync.SyncEngine
    accounts    map[string]*AccountState
    mu          sync.RWMutex
    stopChan    chan struct{}

    // Configuration
    minInterval     time.Duration  // Minimum sync interval (e.g., 5 min)
    maxInterval     time.Duration  // Maximum sync interval (e.g., 60 min)
    baseInterval    time.Duration  // Default interval (e.g., 15 min)
    activityWindow  time.Duration  // Window for activity calculation (e.g., 7 days)
}

type AccountState struct {
    AccountID       string
    TenantID        string
    Email           string
    Priority        int           // User-set priority (1-10)
    ActivityLevel   float64       // Calculated activity (0.0 to 1.0)
    MessagesPerDay  float64       // Average messages per day
    LastSyncAt      time.Time
    LastActivityAt  time.Time
    NextSyncAt      time.Time
    CurrentInterval time.Duration
    ConsecutiveIdle int           // Count of syncs with no new messages
}

type SchedulerConfig struct {
    MinIntervalMinutes  int
    MaxIntervalMinutes  int
    BaseIntervalMinutes int
    ActivityWindowDays  int
    HighActivityThreshold float64  // e.g., 0.7
    LowActivityThreshold  float64  // e.g., 0.3
}

var DefaultSchedulerConfig = SchedulerConfig{
    MinIntervalMinutes:    5,
    MaxIntervalMinutes:    60,
    BaseIntervalMinutes:   15,
    ActivityWindowDays:    7,
    HighActivityThreshold: 0.7,
    LowActivityThreshold:  0.3,
}

func NewIntelligentScheduler(db *pgxpool.Pool, syncEngine *sync.SyncEngine, cfg SchedulerConfig) *IntelligentScheduler {
    return &IntelligentScheduler{
        db:              db,
        syncEngine:      syncEngine,
        accounts:        make(map[string]*AccountState),
        stopChan:        make(chan struct{}),
        minInterval:     time.Duration(cfg.MinIntervalMinutes) * time.Minute,
        maxInterval:     time.Duration(cfg.MaxIntervalMinutes) * time.Minute,
        baseInterval:    time.Duration(cfg.BaseIntervalMinutes) * time.Minute,
        activityWindow:  time.Duration(cfg.ActivityWindowDays) * 24 * time.Hour,
    }
}

func (s *IntelligentScheduler) Start(ctx context.Context) {
    // Load all accounts and initialize state
    s.loadAccounts(ctx)

    // Start scheduler loop
    go s.run(ctx)

    // Start activity calculator
    go s.calculateActivityLoop(ctx)

    slog.Info("intelligent scheduler started", "accounts", len(s.accounts))
}

func (s *IntelligentScheduler) Stop() {
    close(s.stopChan)
}

func (s *IntelligentScheduler) run(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)  // Check every 30 seconds
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-s.stopChan:
            return
        case <-ticker.C:
            s.processScheduledSyncs(ctx)
        }
    }
}

func (s *IntelligentScheduler) processScheduledSyncs(ctx context.Context) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    now := time.Now()

    // Get accounts due for sync, sorted by priority
    var dueSyncs []*AccountState
    for _, account := range s.accounts {
        if now.After(account.NextSyncAt) || now.Equal(account.NextSyncAt) {
            dueSyncs = append(dueSyncs, account)
        }
    }

    // Sort by priority score (higher first)
    sort.Slice(dueSyncs, func(i, j int) bool {
        return s.calculatePriorityScore(dueSyncs[i]) > s.calculatePriorityScore(dueSyncs[j])
    })

    // Process top N accounts to avoid overwhelming the system
    maxConcurrent := 3
    for i, account := range dueSyncs {
        if i >= maxConcurrent {
            break
        }

        go s.triggerSync(ctx, account)
    }
}

func (s *IntelligentScheduler) calculatePriorityScore(account *AccountState) float64 {
    // Combine user priority and activity level
    // Priority: 1-10, normalized to 0.1-1.0
    userPriority := float64(account.Priority) / 10.0

    // Activity level: 0.0-1.0
    activityBonus := account.ActivityLevel * 0.5

    // Overdue bonus: accounts past their sync time get priority
    overdueMinutes := time.Since(account.NextSyncAt).Minutes()
    overdueBonus := min(overdueMinutes/30.0, 0.3)  // Max 0.3 bonus

    return userPriority*0.4 + activityBonus + overdueBonus
}

func (s *IntelligentScheduler) triggerSync(ctx context.Context, account *AccountState) {
    job := &sync.SyncJob{
        ID:        generateID(),
        TenantID:  account.TenantID,
        AccountID: account.AccountID,
        Email:     account.Email,
        Type:      sync.SyncTypeIncremental,
        StartedAt: time.Now(),
    }

    err := s.syncEngine.StartSync(ctx, job)

    // Update account state based on result
    s.mu.Lock()
    defer s.mu.Unlock()

    account.LastSyncAt = time.Now()

    if err != nil {
        slog.Error("scheduled sync failed", "account", account.Email, "error", err)
        // On failure, don't change interval much
        account.NextSyncAt = time.Now().Add(account.CurrentInterval)
        return
    }

    // Adjust interval based on sync results
    s.adjustInterval(account, job.Stats.NewMessages)
}

func (s *IntelligentScheduler) adjustInterval(account *AccountState, newMessages int64) {
    if newMessages > 0 {
        // Activity! Decrease interval (more frequent syncs)
        account.ConsecutiveIdle = 0
        account.LastActivityAt = time.Now()

        // Reduce interval by 20%, but not below minimum
        newInterval := time.Duration(float64(account.CurrentInterval) * 0.8)
        account.CurrentInterval = max(newInterval, s.minInterval)

        slog.Debug("increased sync frequency",
            "account", account.Email,
            "new_messages", newMessages,
            "interval", account.CurrentInterval,
        )
    } else {
        // No new messages
        account.ConsecutiveIdle++

        // After 3 consecutive idle syncs, start backing off
        if account.ConsecutiveIdle >= 3 {
            // Increase interval by 25%, but not above maximum
            newInterval := time.Duration(float64(account.CurrentInterval) * 1.25)
            account.CurrentInterval = min(newInterval, s.maxInterval)

            slog.Debug("decreased sync frequency",
                "account", account.Email,
                "consecutive_idle", account.ConsecutiveIdle,
                "interval", account.CurrentInterval,
            )
        }
    }

    // Apply priority modifier
    priorityModifier := 1.0 + (float64(10-account.Priority) * 0.1)  // Priority 10 = 1.0x, Priority 1 = 1.9x
    adjustedInterval := time.Duration(float64(account.CurrentInterval) * priorityModifier)

    account.NextSyncAt = time.Now().Add(adjustedInterval)
}

func (s *IntelligentScheduler) calculateActivityLoop(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour)  // Recalculate hourly
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-s.stopChan:
            return
        case <-ticker.C:
            s.recalculateActivity(ctx)
        }
    }
}

func (s *IntelligentScheduler) recalculateActivity(ctx context.Context) {
    s.mu.Lock()
    defer s.mu.Unlock()

    for _, account := range s.accounts {
        // Query message count in activity window
        query := `
            SELECT COUNT(*),
                   MAX(received_at) as last_activity
            FROM emails
            WHERE account_id = $1
              AND received_at > $2
        `

        windowStart := time.Now().Add(-s.activityWindow)
        var count int64
        var lastActivity *time.Time

        err := s.db.QueryRow(ctx, query, account.AccountID, windowStart).Scan(&count, &lastActivity)
        if err != nil {
            continue
        }

        // Calculate messages per day
        days := s.activityWindow.Hours() / 24
        account.MessagesPerDay = float64(count) / days

        // Calculate activity level (normalized 0-1)
        // Assume 50 messages/day is high activity
        account.ActivityLevel = min(account.MessagesPerDay/50.0, 1.0)

        if lastActivity != nil {
            account.LastActivityAt = *lastActivity
        }
    }
}

func (s *IntelligentScheduler) GetStatus(tenantID string) *GetSchedulerStatusResponse {
    s.mu.RLock()
    defer s.mu.RUnlock()

    var infos []*AccountScheduleInfo
    var nextSync time.Time

    for _, account := range s.accounts {
        if account.TenantID != tenantID {
            continue
        }

        info := &AccountScheduleInfo{
            AccountId:              account.AccountID,
            Email:                  account.Email,
            PriorityScore:          int32(s.calculatePriorityScore(account) * 100),
            CurrentIntervalMinutes: int32(account.CurrentInterval.Minutes()),
            MessagesPerDay:         int32(account.MessagesPerDay),
            ActivityLevel:          float32(account.ActivityLevel),
            NextSyncAt:             timestamppb.New(account.NextSyncAt),
            LastActivity:           timestamppb.New(account.LastActivityAt),
        }
        infos = append(infos, info)

        if nextSync.IsZero() || account.NextSyncAt.Before(nextSync) {
            nextSync = account.NextSyncAt
        }
    }

    return &GetSchedulerStatusResponse{
        Accounts:          infos,
        NextSyncInSeconds: int64(time.Until(nextSync).Seconds()),
    }
}

func (s *IntelligentScheduler) SetPriority(accountID string, priority int) {
    s.mu.Lock()
    defer s.mu.Unlock()

    if account, ok := s.accounts[accountID]; ok {
        account.Priority = priority
        // Recalculate next sync with new priority
        s.adjustInterval(account, 0)
    }
}
```

## Attachment Processor

```go
// internal/attachment/processor.go

package attachment

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "os"
    "path/filepath"

    "google.golang.org/api/gmail/v1"
)

type AttachmentProcessor struct {
    db          *pgxpool.Pool
    storagePath string
    maxSizeMB   int
    publisher   *events.Publisher
}

type ProcessedAttachment struct {
    ID           string
    MessageID    string
    Filename     string
    MimeType     string
    Size         int64
    SHA256       string
    StoragePath  string
    ExtractedText string
    IsDuplicate  bool
}

func (p *AttachmentProcessor) Process(
    ctx context.Context,
    svc *gmail.Service,
    job *SyncJob,
    messageID string,
    part *gmail.MessagePart,
) error {
    // Get attachment
    attachment, err := svc.Users.Messages.Attachments.Get("me", messageID, part.Body.AttachmentId).Do()
    if err != nil {
        return fmt.Errorf("failed to get attachment: %w", err)
    }

    // Decode data
    data, err := base64.URLEncoding.DecodeString(attachment.Data)
    if err != nil {
        return fmt.Errorf("failed to decode attachment: %w", err)
    }

    // Check size limit
    sizeMB := len(data) / (1024 * 1024)
    if sizeMB > p.maxSizeMB {
        slog.Warn("attachment too large, skipping",
            "message_id", messageID,
            "filename", part.Filename,
            "size_mb", sizeMB,
        )
        return nil
    }

    // Calculate hash
    hash := sha256.Sum256(data)
    hashStr := hex.EncodeToString(hash[:])

    // Check for duplicate
    existing, err := p.findByHash(ctx, job.TenantID, hashStr)
    if err == nil && existing != nil {
        // Duplicate found, just link to existing
        if err := p.linkAttachment(ctx, messageID, existing.ID); err != nil {
            return fmt.Errorf("failed to link duplicate: %w", err)
        }
        return nil
    }

    // Store file
    storagePath := filepath.Join(
        p.storagePath,
        job.TenantID,
        messageID[:2],  // Shard by first 2 chars
        fmt.Sprintf("%s_%s", messageID, part.Filename),
    )

    if err := os.MkdirAll(filepath.Dir(storagePath), 0755); err != nil {
        return fmt.Errorf("failed to create directory: %w", err)
    }

    if err := os.WriteFile(storagePath, data, 0644); err != nil {
        return fmt.Errorf("failed to write file: %w", err)
    }

    // Extract text if possible
    extractedText := ""
    if isTextExtractable(part.MimeType) {
        extractedText, _ = p.extractText(data, part.MimeType)
    }

    // Store metadata
    processed := &ProcessedAttachment{
        ID:            generateID(),
        MessageID:     messageID,
        Filename:      part.Filename,
        MimeType:      part.MimeType,
        Size:          int64(len(data)),
        SHA256:        hashStr,
        StoragePath:   storagePath,
        ExtractedText: extractedText,
    }

    if err := p.store(ctx, job.TenantID, processed); err != nil {
        return fmt.Errorf("failed to store attachment: %w", err)
    }

    // Publish event
    p.publisher.Publish(ctx, "email.attachment_ingested", &events.AttachmentIngestedEvent{
        TenantID:    job.TenantID,
        MessageID:   messageID,
        Filename:    part.Filename,
        MimeType:    part.MimeType,
        Size:        int64(len(data)),
        HasText:     extractedText != "",
    })

    job.Stats.AttachmentsDownloaded++
    job.Stats.BytesDownloaded += int64(len(data))

    return nil
}

func isTextExtractable(mimeType string) bool {
    extractable := map[string]bool{
        "text/plain":                             true,
        "text/html":                              true,
        "application/pdf":                        true,
        "application/msword":                     true,
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
    }
    return extractable[mimeType]
}
```

## Privacy Filter

```go
// internal/privacy/filter.go

package privacy

import (
    "regexp"
    "strings"

    "google.golang.org/api/gmail/v1"
)

type PrivacyFilter struct {
    rules *PrivacyRules
}

type PrivacyRules struct {
    ExcludedLabels   []string
    ExcludedSenders  []string
    ExcludedDomains  []string
    ExcludedPatterns []*regexp.Regexp
    ExcludeSpam      bool
    ExcludeTrash     bool
    ExcludePromotions bool
}

func NewPrivacyFilter(rules *PrivacyRules) *PrivacyFilter {
    return &PrivacyFilter{rules: rules}
}

func (f *PrivacyFilter) ShouldSkip(msg *gmail.Message) bool {
    if f.rules == nil {
        return false
    }

    // Check system labels
    for _, labelID := range msg.LabelIds {
        if f.rules.ExcludeSpam && labelID == "SPAM" {
            return true
        }
        if f.rules.ExcludeTrash && labelID == "TRASH" {
            return true
        }
        if f.rules.ExcludePromotions && labelID == "CATEGORY_PROMOTIONS" {
            return true
        }

        // Check custom excluded labels
        for _, excluded := range f.rules.ExcludedLabels {
            if strings.EqualFold(labelID, excluded) {
                return true
            }
        }
    }

    // Check sender
    from := extractHeader(msg, "From")
    if from != "" {
        email := extractEmail(from)
        domain := extractDomain(email)

        for _, excluded := range f.rules.ExcludedSenders {
            if strings.EqualFold(email, excluded) {
                return true
            }
        }

        for _, excluded := range f.rules.ExcludedDomains {
            if strings.EqualFold(domain, excluded) {
                return true
            }
        }
    }

    // Check subject patterns
    subject := extractHeader(msg, "Subject")
    for _, pattern := range f.rules.ExcludedPatterns {
        if pattern.MatchString(subject) {
            return true
        }
    }

    return false
}

func extractHeader(msg *gmail.Message, name string) string {
    if msg.Payload == nil {
        return ""
    }
    for _, header := range msg.Payload.Headers {
        if strings.EqualFold(header.Name, name) {
            return header.Value
        }
    }
    return ""
}
```

## Push Notifications

```go
// internal/push/handler.go

package push

import (
    "context"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "time"

    "google.golang.org/api/gmail/v1"
)

type PushHandler struct {
    db         *pgxpool.Pool
    syncEngine *sync.SyncEngine
}

type GmailPushNotification struct {
    EmailAddress string `json:"emailAddress"`
    HistoryID    uint64 `json:"historyId"`
}

func (h *PushHandler) HandleNotification(ctx context.Context, data string) error {
    // Decode base64 message
    decoded, err := base64.StdEncoding.DecodeString(data)
    if err != nil {
        return fmt.Errorf("failed to decode push data: %w", err)
    }

    var notification GmailPushNotification
    if err := json.Unmarshal(decoded, &notification); err != nil {
        return fmt.Errorf("failed to unmarshal notification: %w", err)
    }

    // Look up account
    account, err := h.findAccountByEmail(ctx, notification.EmailAddress)
    if err != nil {
        return fmt.Errorf("account not found: %w", err)
    }

    // Trigger incremental sync
    job := &sync.SyncJob{
        ID:        generateID(),
        TenantID:  account.TenantID,
        AccountID: account.ID,
        Email:     account.Email,
        Type:      sync.SyncTypeIncremental,
        StartedAt: time.Now(),
    }

    go h.syncEngine.StartSync(context.Background(), job)

    return nil
}

func (h *PushHandler) RegisterWatch(ctx context.Context, svc *gmail.Service, topicName string) (*gmail.WatchResponse, error) {
    watchReq := &gmail.WatchRequest{
        TopicName:      topicName,
        LabelIds:       []string{"INBOX"},
        LabelFilterAction: "include",
    }

    return svc.Users.Watch("me", watchReq).Do()
}
```

## Configuration

```yaml
# config/gmail-connector.yaml

server:
  grpc_port: 8082
  metrics_port: 9082

oauth:
  client_id: "${GMAIL_CLIENT_ID}"
  client_secret: "${GMAIL_CLIENT_SECRET}"
  redirect_uri: "http://localhost:8088/oauth/callback"
  scopes:
    - "https://www.googleapis.com/auth/gmail.readonly"
    - "https://www.googleapis.com/auth/gmail.labels"
    - "https://www.googleapis.com/auth/userinfo.email"

encryption:
  key: "${TOKEN_ENCRYPTION_KEY}"  # 32 bytes, base64 encoded

sync:
  batch_size: 100
  max_concurrent: 5
  history_days: 365
  default_interval_minutes: 15

scheduler:
  enabled: true
  min_interval_minutes: 5
  max_interval_minutes: 60
  base_interval_minutes: 15
  activity_window_days: 7
  high_activity_threshold: 0.7
  low_activity_threshold: 0.3
  max_concurrent_syncs: 3

rate_limit:
  requests_per_second: 200
  burst_size: 50

attachments:
  enabled: true
  max_size_mb: 25
  storage_path: "/data/attachments"
  extract_text: true

push:
  enabled: true
  topic: "projects/penfold/topics/gmail-notifications"
  watch_expiry_days: 7

database:
  host: "dev02"
  port: 5432
  database: "penfold"
  user: "penfold"
  password: "${DB_PASSWORD}"
  pool_size: 20

redis:
  address: "dev02:6379"

logging:
  level: "info"
  format: "json"
```

## Database Schema

```sql
-- Gmail accounts
CREATE TABLE gmail_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    email VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    encrypted_token BYTEA NOT NULL,
    history_id VARCHAR(50),
    last_sync_at TIMESTAMPTZ,
    sync_config JSONB DEFAULT '{}',
    privacy_rules JSONB DEFAULT '{}',
    connected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, email)
);

-- Sync jobs
CREATE TABLE gmail_sync_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    account_id UUID NOT NULL REFERENCES gmail_accounts(id),
    sync_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    options JSONB DEFAULT '{}',
    stats JSONB DEFAULT '{}',
    error TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- OAuth state (temporary)
CREATE TABLE oauth_states (
    state VARCHAR(64) PRIMARY KEY,
    tenant_id UUID NOT NULL,
    code_verifier VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

-- Index for cleanup
CREATE INDEX idx_oauth_states_expires ON oauth_states(expires_at);

-- Watch registrations
CREATE TABLE gmail_watches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES gmail_accounts(id),
    watch_id VARCHAR(255),
    topic_name VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## Implementation Structure

```
services/gmail-connector/
├── cmd/
│   └── gmail-connector/
│       └── main.go
├── internal/
│   ├── oauth/
│   │   ├── manager.go
│   │   ├── encryption.go
│   │   ├── store.go
│   │   └── state.go
│   ├── sync/
│   │   ├── engine.go
│   │   ├── incremental.go
│   │   ├── full.go
│   │   ├── ratelimit.go
│   │   └── job.go
│   ├── attachment/
│   │   ├── processor.go
│   │   ├── extractor.go
│   │   └── storage.go
│   ├── privacy/
│   │   └── filter.go
│   ├── push/
│   │   ├── handler.go
│   │   └── watch.go
│   ├── service/
│   │   └── grpc.go
│   └── config/
│       └── config.go
├── api/
│   └── proto/
│       └── gmail/
│           └── v1/
│               └── gmail.proto
└── go.mod
```

## Events Published

| Event | Trigger | Payload |
|-------|---------|---------|
| `email.ingested` | New email processed | MessageID, ThreadID, Subject, Snippet |
| `email.thread_ingested` | Thread updated | ThreadID, MessageCount |
| `email.attachment_ingested` | Attachment processed | Filename, MimeType, Size |
| `sync.progress` | Sync progress update | Processed, Total, Percent |
| `sync.completed` | Sync finished | Stats, Duration |
| `sync.failed` | Sync error | Error, AccountID |
