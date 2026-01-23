# Gmail Integration API Reference

This document provides detailed API documentation for all Gmail integration components in Penfold. The Go implementation follows idiomatic patterns with comprehensive interfaces, error handling, and extensibility.

## Service Architecture

The Gmail service is located at `services/gmail/` and consists of these packages:

| Package | Path | Description |
|---------|------|-------------|
| `oauth` | `services/gmail/oauth/` | OAuth2 PKCE authentication |
| `sync` | `services/gmail/sync/` | Email synchronization engine |
| `push` | `services/gmail/push/` | Push notification handling |
| `attachment` | `services/gmail/attachment/` | Attachment processing |
| `privacy` | `services/gmail/privacy/` | Privacy filtering and PII detection |
| `config` | `services/gmail/config/` | Service configuration |
| `server` | `services/gmail/server/` | gRPC service implementation |

---

## OAuth2 Package (`services/gmail/oauth/`)

### Token Type

Represents OAuth2 tokens with metadata for secure storage and management.

```go
// Token represents OAuth2 tokens with metadata.
type Token struct {
    // AccessToken is the token that authorizes API requests.
    AccessToken string `json:"access_token"`

    // RefreshToken allows obtaining new access tokens.
    RefreshToken string `json:"refresh_token"`

    // TokenType is typically "Bearer".
    TokenType string `json:"token_type"`

    // ExpiresAt is when the access token expires.
    ExpiresAt time.Time `json:"expires_at"`

    // Scopes are the granted OAuth scopes.
    Scopes []string `json:"scopes"`

    // TenantID identifies the account/tenant.
    TenantID string `json:"tenant_id"`

    // CreatedAt is when the token was created.
    CreatedAt time.Time `json:"created_at"`

    // UpdatedAt is when the token was last updated.
    UpdatedAt time.Time `json:"updated_at"`
}
```

### OAuth2Manager

Manages OAuth2 authentication flows with PKCE support.

```go
// OAuth2Manager manages OAuth2 authentication flows for Gmail.
type OAuth2Manager struct {
    config       *Config
    tokenStorage TokenStorage
    pendingFlows map[string]*AuthFlowState
    metrics      *OAuthMetrics
    mu           sync.RWMutex
}

// Config holds OAuth2 configuration.
type Config struct {
    // ClientID is the OAuth2 client ID from Google Cloud Console.
    ClientID string

    // ClientSecret is the OAuth2 client secret.
    ClientSecret string

    // RedirectURL is the callback URL for the OAuth flow.
    RedirectURL string

    // Scopes are the Gmail API scopes to request.
    Scopes []string

    // TokenStorage is the storage backend for tokens.
    TokenStorage TokenStorage

    // RefreshMargin is how early to refresh tokens before expiry.
    RefreshMargin time.Duration
}
```

#### Methods

```go
// NewOAuth2Manager creates a new OAuth2 manager.
func NewOAuth2Manager(config *Config) (*OAuth2Manager, error)

// StartAuthFlow initiates OAuth2 authorization with PKCE.
// Returns the authorization URL and state parameter.
func (m *OAuth2Manager) StartAuthFlow(ctx context.Context, tenantID string) (authURL, state string, err error)

// CompleteAuthFlow exchanges the authorization code for tokens.
func (m *OAuth2Manager) CompleteAuthFlow(ctx context.Context, state, code string) (*Token, error)

// GetValidToken returns a valid access token, refreshing if needed.
func (m *OAuth2Manager) GetValidToken(ctx context.Context, tenantID string) (string, error)

// RefreshToken explicitly refreshes the access token.
func (m *OAuth2Manager) RefreshToken(ctx context.Context, tenantID string) (*Token, error)

// RevokeToken revokes all tokens for a tenant.
func (m *OAuth2Manager) RevokeToken(ctx context.Context, tenantID string) error

// IsTokenValid checks if the current token is valid.
func (m *OAuth2Manager) IsTokenValid(ctx context.Context, tenantID string) (bool, error)
```

#### Example Usage

```go
config := &oauth.Config{
    ClientID:      "your-client-id.apps.googleusercontent.com",
    ClientSecret:  "your-client-secret",
    RedirectURL:   "http://localhost:8080/callback",
    Scopes:        []string{"https://www.googleapis.com/auth/gmail.readonly"},
    TokenStorage:  storage,
    RefreshMargin: 5 * time.Minute,
}

manager, err := oauth.NewOAuth2Manager(config)
if err != nil {
    return err
}

// Start the authorization flow
authURL, state, err := manager.StartAuthFlow(ctx, tenantID)
if err != nil {
    return err
}

// User visits authURL and authorizes...
// Then redirect brings back the code

token, err := manager.CompleteAuthFlow(ctx, state, authCode)
if err != nil {
    return err
}

// Later, get a valid token for API requests
accessToken, err := manager.GetValidToken(ctx, tenantID)
if err != nil {
    return err
}
```

### TokenEncryptor

Provides AES-256-GCM encryption for secure token storage.

```go
// TokenEncryptor provides AES-256-GCM encryption for OAuth tokens.
type TokenEncryptor struct {
    gcm cipher.AEAD
}

// NewTokenEncryptor creates a new encryptor with a 32-byte key.
// The key must be exactly 32 bytes for AES-256.
func NewTokenEncryptor(key []byte) (*TokenEncryptor, error)

// Encrypt encrypts plaintext using AES-256-GCM.
// The nonce is prepended to the ciphertext.
func (e *TokenEncryptor) Encrypt(plaintext []byte) ([]byte, error)

// Decrypt decrypts ciphertext encrypted with Encrypt.
func (e *TokenEncryptor) Decrypt(ciphertext []byte) ([]byte, error)
```

#### Example Usage

```go
// Generate a 32-byte key (in production, use a secure key derivation)
key := make([]byte, 32)
if _, err := rand.Read(key); err != nil {
    return err
}

encryptor, err := oauth.NewTokenEncryptor(key)
if err != nil {
    return err
}

// Encrypt token data
tokenJSON, _ := json.Marshal(token)
encrypted, err := encryptor.Encrypt(tokenJSON)
if err != nil {
    return err
}

// Decrypt token data
decrypted, err := encryptor.Decrypt(encrypted)
if err != nil {
    return err
}
```

### TokenStorage Interface

Interface for token persistence backends.

```go
// TokenStorage defines the interface for token persistence.
type TokenStorage interface {
    // StoreToken persists an OAuth2 token.
    StoreToken(ctx context.Context, token *Token) error

    // GetToken retrieves a token by tenant ID.
    GetToken(ctx context.Context, tenantID string) (*Token, error)

    // DeleteToken removes a token.
    DeleteToken(ctx context.Context, tenantID string) error

    // ListTokens returns all stored tokens.
    ListTokens(ctx context.Context) ([]*Token, error)
}
```

---

## Sync Package (`services/gmail/sync/`)

### Engine

Orchestrates email synchronization with full and incremental modes.

```go
// Engine manages Gmail synchronization operations.
type Engine struct {
    config      *EngineConfig
    rateLimiter *RateLimiter
    metrics     *SyncMetrics
}

// EngineConfig holds configuration for the sync engine.
type EngineConfig struct {
    // OAuth2Manager for token management.
    OAuth2Manager *oauth.OAuth2Manager

    // StateStorage for persisting sync state.
    StateStorage StateStorage

    // HTTPClient for API requests (optional, uses default if nil).
    HTTPClient *http.Client

    // BatchSize is the number of messages to fetch per batch.
    BatchSize int

    // RateLimit is the max requests per second.
    RateLimit int

    // MaxRetries for transient failures.
    MaxRetries int

    // InitialBackoff for retry delays.
    InitialBackoff time.Duration

    // MaxBackoff caps exponential backoff.
    MaxBackoff time.Duration

    // BackoffMultiplier for exponential increase.
    BackoffMultiplier float64
}
```

#### Methods

```go
// NewEngine creates a new sync engine.
func NewEngine(config *EngineConfig) (*Engine, error)

// FullSync performs a complete mailbox synchronization.
func (e *Engine) FullSync(ctx context.Context, tenantID string, opts *SyncOptions) (*SyncResult, error)

// IncrementalSync performs sync using the Gmail History API.
func (e *Engine) IncrementalSync(ctx context.Context, tenantID string, opts *SyncOptions) (*SyncResult, error)

// ResumeSync resumes an interrupted sync operation.
func (e *Engine) ResumeSync(ctx context.Context, tenantID string, opts *SyncOptions) (*SyncResult, error)

// CancelSync cancels an in-progress sync operation.
func (e *Engine) CancelSync(ctx context.Context, tenantID string) error

// GetSyncState returns the current sync state for a tenant.
func (e *Engine) GetSyncState(ctx context.Context, tenantID string) (*SyncState, error)
```

### SyncOptions

Configuration for sync operations.

```go
// SyncOptions configures sync behavior.
type SyncOptions struct {
    // LabelIDs filters messages to specific labels.
    LabelIDs []string

    // Query is a Gmail search query to filter messages.
    Query string

    // After limits sync to messages after this time.
    After time.Time

    // Before limits sync to messages before this time.
    Before time.Time

    // IncludeAttachments controls attachment processing.
    IncludeAttachments bool

    // MaxMessages limits total messages processed.
    MaxMessages int64

    // PrivacyFilter is the filter to apply to messages.
    PrivacyFilter *privacy.PrivacyFilter

    // OnProgress is called with progress updates.
    OnProgress func(*SyncProgress)
}
```

### SyncResult

Result of a sync operation.

```go
// SyncResult contains the outcome of a sync operation.
type SyncResult struct {
    // SyncID is the unique identifier for this sync.
    SyncID string

    // TenantID identifies the synced account.
    TenantID string

    // SyncType is the type of sync performed.
    SyncType SyncType

    // Status is the final status.
    Status SyncStatus

    // ProcessedCount is total messages processed.
    ProcessedCount int64

    // SuccessCount is messages successfully synced.
    SuccessCount int64

    // ErrorCount is messages that failed.
    ErrorCount int64

    // SkippedCount is messages filtered out.
    SkippedCount int64

    // NewHistoryID is the latest history ID after sync.
    NewHistoryID uint64

    // Duration is how long the sync took.
    Duration time.Duration

    // Errors contains error details if any.
    Errors []SyncErrorRecord

    // CompletedAt is when the sync finished.
    CompletedAt time.Time
}
```

#### Example Usage

```go
config := &sync.EngineConfig{
    OAuth2Manager:     oauthManager,
    StateStorage:      stateStorage,
    BatchSize:         100,
    RateLimit:         250,
    MaxRetries:        5,
    InitialBackoff:    time.Second,
    MaxBackoff:        60 * time.Second,
    BackoffMultiplier: 2.0,
}

engine, err := sync.NewEngine(config)
if err != nil {
    return err
}

opts := &sync.SyncOptions{
    LabelIDs:           []string{"INBOX"},
    After:              time.Now().AddDate(0, -1, 0), // Last month
    IncludeAttachments: true,
    PrivacyFilter:      privacyFilter,
    OnProgress: func(p *sync.SyncProgress) {
        log.Printf("Progress: %d/%d", p.Processed, p.Total)
    },
}

result, err := engine.FullSync(ctx, tenantID, opts)
if err != nil {
    return err
}

log.Printf("Synced %d messages in %v", result.SuccessCount, result.Duration)
```

---

## Push Package (`services/gmail/push/`)

### Handler

Processes incoming Gmail push notifications from Cloud Pub/Sub.

```go
// Handler processes Gmail push notifications.
type Handler struct {
    config *HandlerConfig
}

// HandlerConfig holds configuration for the handler.
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
```

#### Types

```go
// PushNotification represents a Gmail push notification from Pub/Sub.
type PushNotification struct {
    // Message is the Pub/Sub message.
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

// GmailNotificationData represents the decoded notification payload.
type GmailNotificationData struct {
    // EmailAddress is the Gmail account that received the notification.
    EmailAddress string `json:"emailAddress"`

    // HistoryID is the Gmail history ID for incremental sync.
    HistoryID uint64 `json:"historyId"`
}
```

#### Methods

```go
// NewHandler creates a new push notification handler.
func NewHandler(config *HandlerConfig) (*Handler, error)

// HandlePush processes an incoming push notification.
func (h *Handler) HandlePush(ctx context.Context, notification *PushNotification) error

// ValidateNotification validates and parses raw notification data.
func (h *Handler) ValidateNotification(data []byte) (*PushNotification, error)

// ProcessHistoryChange processes a history change for a specific account.
func (h *Handler) ProcessHistoryChange(ctx context.Context, tenantID string, historyID uint64) error
```

#### Example Usage

```go
config := &push.HandlerConfig{
    SubscriptionStore:     subscriptionStore,
    NotificationProcessor: processor,
    Logger:                logger,
    Metrics:               metrics,
}

handler, err := push.NewHandler(config)
if err != nil {
    return err
}

// HTTP handler for webhook endpoint
http.HandleFunc("/webhooks/gmail", func(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Bad request", http.StatusBadRequest)
        return
    }

    notification, err := handler.ValidateNotification(body)
    if err != nil {
        http.Error(w, "Invalid notification", http.StatusBadRequest)
        return
    }

    if err := handler.HandlePush(r.Context(), notification); err != nil {
        http.Error(w, "Processing failed", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
})
```

---

## Attachment Package (`services/gmail/attachment/`)

### AttachmentProcessor

Processes email attachments for content extraction.

```go
// AttachmentProcessor processes email attachments.
type AttachmentProcessor struct {
    config    *ProcessorConfig
    semaphore chan struct{}
    metrics   *ProcessorMetrics
}

// ProcessorConfig holds configuration.
type ProcessorConfig struct {
    // MaxAttachmentSize is the maximum size to process (bytes).
    MaxAttachmentSize int64

    // ProcessTimeout is the timeout for processing a single attachment.
    ProcessTimeout time.Duration

    // ConcurrentLimit is the max concurrent processing operations.
    ConcurrentLimit int

    // ExtractorRegistry maps MIME types to extractors.
    ExtractorRegistry map[string]TextExtractor

    // AttachmentDownloader downloads attachment content.
    AttachmentDownloader AttachmentDownloader
}

// Attachment represents an email attachment.
type Attachment struct {
    // ID is the Gmail attachment ID.
    ID string `json:"id"`

    // MessageID is the parent message ID.
    MessageID string `json:"message_id"`

    // Filename is the original filename.
    Filename string `json:"filename"`

    // MimeType is the MIME type.
    MimeType string `json:"mime_type"`

    // Size is the attachment size in bytes.
    Size int64 `json:"size"`

    // ContentHash is the SHA-256 hash (set after processing).
    ContentHash string `json:"content_hash,omitempty"`
}

// ProcessedAttachment represents a processed attachment.
type ProcessedAttachment struct {
    // Attachment is the original attachment.
    Attachment *Attachment `json:"attachment"`

    // ExtractedText is the text extracted from the attachment.
    ExtractedText string `json:"extracted_text,omitempty"`

    // Classification is the type classification.
    Classification AttachmentClassification `json:"classification"`

    // ProcessedAt is when the attachment was processed.
    ProcessedAt time.Time `json:"processed_at"`

    // ProcessingDuration is how long processing took.
    ProcessingDuration time.Duration `json:"processing_duration_ns"`

    // Error is set if processing failed.
    Error string `json:"error,omitempty"`

    // Metadata contains additional extracted metadata.
    Metadata map[string]string `json:"metadata,omitempty"`
}
```

#### Methods

```go
// NewAttachmentProcessor creates a new processor.
func NewAttachmentProcessor(config *ProcessorConfig) (*AttachmentProcessor, error)

// ProcessAttachment processes a single attachment.
func (p *AttachmentProcessor) ProcessAttachment(ctx context.Context, attachment *Attachment) (*ProcessedAttachment, error)

// ProcessAttachments processes multiple attachments concurrently.
func (p *AttachmentProcessor) ProcessAttachments(ctx context.Context, attachments []*Attachment) ([]*ProcessedAttachment, error)

// RegisterExtractor registers a text extractor for a MIME type.
func (p *AttachmentProcessor) RegisterExtractor(mimeType string, extractor TextExtractor)

// SetDownloader sets the attachment downloader.
func (p *AttachmentProcessor) SetDownloader(downloader AttachmentDownloader)

// DownloadAttachment downloads an attachment from Gmail.
func (p *AttachmentProcessor) DownloadAttachment(ctx context.Context, messageID, attachmentID string) ([]byte, error)

// ExtractText extracts text content based on MIME type.
func (p *AttachmentProcessor) ExtractText(content []byte, mimeType string) (string, error)
```

### TextExtractor Interface

Interface for implementing content extractors.

```go
// TextExtractor extracts text from attachment content.
type TextExtractor interface {
    Extract(content []byte) (string, error)
}

// AttachmentDownloader downloads attachment content.
type AttachmentDownloader interface {
    Download(ctx context.Context, messageID, attachmentID string) ([]byte, error)
}
```

#### Example Usage

```go
config := &attachment.ProcessorConfig{
    MaxAttachmentSize: 25 * 1024 * 1024, // 25MB
    ProcessTimeout:    5 * time.Minute,
    ConcurrentLimit:   10,
    ExtractorRegistry: make(map[string]attachment.TextExtractor),
}

processor, err := attachment.NewAttachmentProcessor(config)
if err != nil {
    return err
}

processor.SetDownloader(gmailDownloader)

attachments := []*attachment.Attachment{
    {
        ID:        "att123",
        MessageID: "msg456",
        Filename:  "document.pdf",
        MimeType:  "application/pdf",
        Size:      1024000,
    },
}

results, err := processor.ProcessAttachments(ctx, attachments)
if err != nil {
    return err
}

for _, result := range results {
    if result.Error == "" {
        log.Printf("Extracted %d chars from %s",
            len(result.ExtractedText), result.Attachment.Filename)
    }
}
```

---

## Privacy Package (`services/gmail/privacy/`)

### PrivacyFilter

Implements configurable privacy controls with PII detection.

```go
// PrivacyFilter processes messages for PII detection and redaction.
type PrivacyFilter struct {
    config            *FilterConfig
    rules             []Rule
    blockedSendersMap map[string]bool
    blockedDomainsMap map[string]bool
    allowedSendersMap map[string]bool
    allowedDomainsMap map[string]bool
    metrics           *FilterMetrics
}

// FilterConfig holds configuration.
type FilterConfig struct {
    // SensitivityLevel controls filtering aggressiveness.
    SensitivityLevel SensitivityLevel

    // RedactionPlaceholder replaces detected PII.
    RedactionPlaceholder string

    // BlockedSenders is a list of email addresses to exclude.
    BlockedSenders []string

    // BlockedDomains is a list of domains to exclude.
    BlockedDomains []string

    // AllowedSenders bypasses filtering.
    AllowedSenders []string

    // AllowedDomains bypasses filtering.
    AllowedDomains []string

    // CustomRules adds rules beyond defaults.
    CustomRules []Rule

    // AuditLogger records filtering actions.
    AuditLogger AuditLogger
}

// SensitivityLevel defines filtering aggressiveness.
type SensitivityLevel int

const (
    SensitivityLow    SensitivityLevel = iota  // Blocklist only
    SensitivityMedium                           // PII detection + redaction
    SensitivityHigh                             // Full content filtering
)
```

#### Types

```go
// Message represents an email message to be filtered.
type Message struct {
    ID          string            `json:"id"`
    ThreadID    string            `json:"thread_id,omitempty"`
    From        string            `json:"from"`
    To          []string          `json:"to"`
    Cc          []string          `json:"cc,omitempty"`
    Subject     string            `json:"subject"`
    Body        string            `json:"body"`
    Labels      []string          `json:"labels,omitempty"`
    ReceivedAt  time.Time         `json:"received_at"`
    Attachments []AttachmentInfo  `json:"attachments,omitempty"`
}

// FilterResult contains the filtering outcome.
type FilterResult struct {
    // Message is the filtered message (may have redacted content).
    Message *Message `json:"message"`

    // Excluded indicates the message should not be stored.
    Excluded bool `json:"excluded"`

    // ExclusionReason explains why excluded.
    ExclusionReason string `json:"exclusion_reason,omitempty"`

    // PIIDetected contains all PII locations found.
    PIIDetected []PIILocation `json:"pii_detected,omitempty"`

    // RedactionCount is the number of redactions applied.
    RedactionCount int `json:"redaction_count"`

    // ProcessingTime is how long filtering took.
    ProcessingTime time.Duration `json:"processing_time"`
}

// PIILocation identifies detected PII in text.
type PIILocation struct {
    Start      int     `json:"start"`
    End        int     `json:"end"`
    Type       string  `json:"type"`
    Value      string  `json:"value"`
    Confidence float64 `json:"confidence"`
}
```

#### Methods

```go
// NewPrivacyFilter creates a new privacy filter.
func NewPrivacyFilter(config *FilterConfig) (*PrivacyFilter, error)

// FilterMessage processes a message according to sensitivity level.
func (f *PrivacyFilter) FilterMessage(ctx context.Context, msg *Message, tenantID string) (*FilterResult, error)

// ShouldExclude checks if a message should be excluded.
func (f *PrivacyFilter) ShouldExclude(msg *Message) (bool, string)

// DetectPII runs all enabled rules against the text.
func (f *PrivacyFilter) DetectPII(text string) []PIILocation

// RedactPII replaces PII at specified locations.
func (f *PrivacyFilter) RedactPII(text string, locations []PIILocation) string

// AddRule adds a custom rule to the filter.
func (f *PrivacyFilter) AddRule(rule Rule)

// RemoveRule removes a rule by name.
func (f *PrivacyFilter) RemoveRule(name string) bool

// UpdateBlocklist updates the sender and domain blocklists.
func (f *PrivacyFilter) UpdateBlocklist(senders, domains []string)

// UpdateAllowlist updates the sender and domain allowlists.
func (f *PrivacyFilter) UpdateAllowlist(senders, domains []string)

// SetSensitivityLevel updates the sensitivity level.
func (f *PrivacyFilter) SetSensitivityLevel(level SensitivityLevel)
```

### Rule Interface

Interface for implementing custom PII detection rules.

```go
// Rule defines the interface for PII detection rules.
type Rule interface {
    // Name returns the rule identifier.
    Name() string

    // Enabled returns whether the rule is active.
    Enabled() bool

    // Detect finds PII locations in text.
    Detect(text string) []PIILocation

    // Redact replaces PII at specified locations.
    Redact(text string, locations []PIILocation, placeholder string) string
}
```

#### Example Usage

```go
config := &privacy.FilterConfig{
    SensitivityLevel:     privacy.SensitivityMedium,
    RedactionPlaceholder: "[REDACTED]",
    BlockedSenders:       []string{"spam@example.com"},
    BlockedDomains:       []string{"malware.com"},
    AllowedDomains:       []string{"company.com"},
}

filter, err := privacy.NewPrivacyFilter(config)
if err != nil {
    return err
}

msg := &privacy.Message{
    ID:         "msg123",
    From:       "sender@example.com",
    Subject:    "Regarding account 123-45-6789",
    Body:       "Please call me at 555-123-4567 about your SSN 123-45-6789.",
    ReceivedAt: time.Now(),
}

result, err := filter.FilterMessage(ctx, msg, tenantID)
if err != nil {
    return err
}

if result.Excluded {
    log.Printf("Message excluded: %s", result.ExclusionReason)
} else {
    log.Printf("Filtered message, %d redactions applied", result.RedactionCount)
    // result.Message.Body: "Please call me at [REDACTED] about your SSN [REDACTED]."
}
```

---

## Config Package (`services/gmail/config/`)

### Config

Service-specific configuration.

```go
// Config holds Gmail Connector service configuration.
type Config struct {
    // Base embeds the shared Penfold configuration.
    Base *pkgconfig.Config

    // GRPCPort is the port for gRPC connections.
    GRPCPort int

    // HTTPPort is the port for health checks and metrics.
    HTTPPort int

    // OAuthCredentialsPath is the path to OAuth credentials JSON.
    OAuthCredentialsPath string

    // TokenStorePath is the path for OAuth token storage.
    TokenStorePath string

    // MaxSyncBatchSize is the max emails per batch.
    MaxSyncBatchSize int

    // SyncTimeoutSeconds is the sync operation timeout.
    SyncTimeoutSeconds int
}

// Default values.
const (
    DefaultGRPCPort           = 50051
    DefaultHTTPPort           = 8081
    DefaultMaxSyncBatchSize   = 500
    DefaultSyncTimeoutSeconds = 300
)
```

#### Methods

```go
// LoadConfig loads the Gmail Connector service configuration.
func LoadConfig() (*Config, error)

// Validate checks that the configuration is valid.
func (c *Config) Validate() error

// GRPCAddress returns the gRPC server listen address.
func (c *Config) GRPCAddress() string

// HTTPAddress returns the HTTP server listen address.
func (c *Config) HTTPAddress() string
```

#### Example Usage

```go
cfg, err := config.LoadConfig()
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}

log.Printf("Starting gRPC server on %s", cfg.GRPCAddress())
log.Printf("Starting HTTP server on %s", cfg.HTTPAddress())
```

---

## CLI Commands

The `penf` CLI provides Gmail management commands.

### Connection Commands

```bash
# Connect a Gmail account (starts OAuth2 flow)
penf gmail connect

# Connect specific account
penf gmail connect --account user@gmail.com

# Disconnect account
penf gmail disconnect --account user@gmail.com

# Refresh authentication
penf gmail refresh-auth --account user@gmail.com
```

### Sync Commands

```bash
# Start historical import
penf gmail import

# Import with date range
penf gmail import --from-date 2025-01-01 --to-date 2025-12-31

# Dry run to preview import
penf gmail import --dry-run

# Manual sync
penf gmail sync

# Sync specific account
penf gmail sync --account user@gmail.com

# Force full sync
penf gmail sync --full
```

### Status Commands

```bash
# Show connection status
penf gmail status

# Verbose status with details
penf gmail status --verbose

# List all accounts
penf gmail accounts

# Show sync history
penf gmail status --history
```

### Configuration Commands

```bash
# List current configuration
penf gmail config --list

# Update batch size
penf gmail config --batch-size 100

# Update privacy level
penf gmail config --privacy-level medium

# Block sender
penf gmail config --block-sender spam@example.com

# Block domain
penf gmail config --block-domain spam.example.com

# Allow domain
penf gmail config --allow-domain company.com

# Configure polling fallback
penf gmail config --polling-fallback true --polling-interval 300
```

### Monitoring Commands

```bash
# Watch real-time status
penf gmail monitor

# Test real-time notification
penf gmail monitor --test-email

# View logs
penf gmail logs --tail
penf gmail logs --level debug

# Export diagnostic report
penf gmail diagnostic --export /tmp/gmail-diagnostic.zip
```

---

## Error Types

Common errors returned by the Gmail integration packages.

```go
// OAuth errors
var (
    ErrInvalidState       = errors.New("oauth: invalid state parameter")
    ErrFlowExpired        = errors.New("oauth: authorization flow expired")
    ErrTokenExpired       = errors.New("oauth: token expired")
    ErrTokenNotFound      = errors.New("oauth: token not found")
    ErrRefreshFailed      = errors.New("oauth: token refresh failed")
)

// Sync errors
var (
    ErrSyncInProgress     = errors.New("sync: sync already in progress")
    ErrSyncNotFound       = errors.New("sync: sync operation not found")
    ErrRateLimitExceeded  = errors.New("sync: rate limit exceeded")
    ErrHistoryExpired     = errors.New("sync: history ID expired, full sync required")
)

// Attachment errors
var (
    ErrAttachmentTooLarge  = errors.New("attachment: exceeds maximum size limit")
    ErrUnsupportedMimeType = errors.New("attachment: unsupported MIME type")
    ErrDownloadFailed      = errors.New("attachment: download failed")
    ErrExtractionFailed    = errors.New("attachment: text extraction failed")
    ErrProcessingTimeout   = errors.New("attachment: processing timeout")
    ErrNoDownloader        = errors.New("attachment: no downloader configured")
)

// Privacy errors
var (
    ErrFilterConfigInvalid = errors.New("privacy: invalid filter configuration")
)
```

---

## Metrics

Prometheus metrics exported by each package.

### OAuth Metrics

```go
type OAuthMetrics struct {
    AuthFlowsStarted   prometheus.Counter   // Total auth flows initiated
    AuthFlowsCompleted prometheus.Counter   // Successfully completed flows
    AuthFlowsFailed    prometheus.Counter   // Failed auth flows
    TokenRefreshes     prometheus.Counter   // Token refresh operations
    TokenRefreshErrors prometheus.Counter   // Failed refreshes
    TokenValidations   prometheus.Counter   // Token validation checks
}
```

### Sync Metrics

```go
type SyncMetrics struct {
    SyncsStarted      prometheus.Counter   // Sync operations started
    SyncsCompleted    prometheus.Counter   // Successfully completed syncs
    SyncsFailed       prometheus.Counter   // Failed syncs
    MessagesProcessed prometheus.Counter   // Total messages processed
    MessagesFailed    prometheus.Counter   // Messages that failed processing
    APIRequests       prometheus.Counter   // Gmail API requests
    APIErrors         prometheus.Counter   // API errors
    APILatency        prometheus.Histogram // API request latency
    RateLimitHits     prometheus.Counter   // Rate limit encounters
    RetryAttempts     prometheus.Counter   // Retry attempts
}
```

### Attachment Metrics

```go
type ProcessorMetrics struct {
    AttachmentsProcessed prometheus.Counter    // Successfully processed
    AttachmentsFailed    prometheus.Counter    // Failed processing
    AttachmentsSkipped   prometheus.Counter    // Skipped (too large, unsupported)
    ProcessingDuration   prometheus.Histogram  // Processing time
    BytesProcessed       prometheus.Counter    // Total bytes processed
    TextBytesExtracted   prometheus.Counter    // Text bytes extracted
    ClassificationCounts *prometheus.CounterVec // By classification type
}
```

### Privacy Metrics

```go
type FilterMetrics struct {
    MessagesProcessed prometheus.Counter    // Messages processed
    MessagesExcluded  prometheus.Counter    // Messages excluded
    PIIDetections     *prometheus.CounterVec // By PII type
    RedactionsApplied prometheus.Counter    // Redactions applied
    ProcessingTime    prometheus.Histogram  // Processing time
    FilterErrors      prometheus.Counter    // Filter errors
}
```

---

## Testing Utilities

Mock implementations for testing.

```go
// MockDownloader for attachment processing tests.
type MockDownloader struct {
    Content map[string][]byte
    Err     error
}

func (m *MockDownloader) Download(ctx context.Context, messageID, attachmentID string) ([]byte, error) {
    if m.Err != nil {
        return nil, m.Err
    }
    key := messageID + ":" + attachmentID
    if content, exists := m.Content[key]; exists {
        return content, nil
    }
    return nil, fmt.Errorf("attachment not found: %s", key)
}

// MemoryAuditLogger for privacy filter tests.
type MemoryAuditLogger struct {
    entries []AuditEntry
    mu      sync.Mutex
}

func NewMemoryAuditLogger() *MemoryAuditLogger {
    return &MemoryAuditLogger{entries: make([]AuditEntry, 0)}
}

func (l *MemoryAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.entries = append(l.entries, *entry)
    return nil
}

func (l *MemoryAuditLogger) GetEntries() []AuditEntry {
    l.mu.Lock()
    defer l.mu.Unlock()
    entries := make([]AuditEntry, len(l.entries))
    copy(entries, l.entries)
    return entries
}
```

This API reference provides comprehensive documentation for all Gmail integration components. For usage examples and integration patterns, see the [Architecture Guide](./architecture.md) and [Setup Guide](./setup-guide.md).
