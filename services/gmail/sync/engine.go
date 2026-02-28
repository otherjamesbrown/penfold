// Package sync provides Gmail synchronization capabilities including
// full sync, incremental sync via History API, and resumable sync state.
package sync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/gmail/oauth"
	"github.com/prometheus/client_golang/prometheus"
)

// Gmail API constants.
const (
	GmailAPIBaseURL  = "https://gmail.googleapis.com/gmail/v1"
	DefaultBatchSize = 100
	MaxBatchSize     = 500

	// Rate limiting defaults (Gmail API allows 250 quota units per user per second).
	DefaultRateLimit       = 250   // requests per second
	DefaultBurstLimit      = 50    // burst allowance
	DefaultMaxRetries      = 5     // max retry attempts
	DefaultInitialBackoff  = 1 * time.Second
	DefaultMaxBackoff      = 60 * time.Second
	DefaultBackoffMultiple = 2.0
)

// Message represents a Gmail message from the API.
type Message struct {
	ID           string            `json:"id"`
	ThreadID     string            `json:"threadId"`
	LabelIDs     []string          `json:"labelIds"`
	Snippet      string            `json:"snippet"`
	InternalDate string            `json:"internalDate"`
	SizeEstimate int64             `json:"sizeEstimate"`
	Payload      *MessagePayload   `json:"payload"`
	Raw          string            `json:"raw,omitempty"`
	HistoryID    uint64            `json:"historyId,string,omitempty"`
}

// MessagePayload represents the payload of a Gmail message.
type MessagePayload struct {
	PartID   string           `json:"partId"`
	MimeType string           `json:"mimeType"`
	Filename string           `json:"filename"`
	Headers  []MessageHeader  `json:"headers"`
	Body     *MessageBody     `json:"body"`
	Parts    []*MessagePayload `json:"parts"`
}

// MessageHeader represents a message header.
type MessageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MessageBody represents a message body part.
type MessageBody struct {
	AttachmentID string `json:"attachmentId"`
	Size         int64  `json:"size"`
	Data         string `json:"data"`
}

// MessageList represents a list of messages from the API.
type MessageList struct {
	Messages           []MessageRef `json:"messages"`
	NextPageToken      string       `json:"nextPageToken"`
	ResultSizeEstimate int64        `json:"resultSizeEstimate"`
}

// MessageRef represents a message reference (ID and thread ID only).
type MessageRef struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

// HistoryList represents history changes from the API.
type HistoryList struct {
	History        []History `json:"history"`
	NextPageToken  string    `json:"nextPageToken"`
	HistoryID      uint64    `json:"historyId,string"`
}

// History represents a single history record.
type History struct {
	ID              uint64           `json:"id,string"`
	Messages        []Message        `json:"messages"`
	MessagesAdded   []HistoryMessage `json:"messagesAdded"`
	MessagesDeleted []HistoryMessage `json:"messagesDeleted"`
	LabelsAdded     []HistoryLabel   `json:"labelsAdded"`
	LabelsRemoved   []HistoryLabel   `json:"labelsRemoved"`
}

// HistoryMessage represents a message in a history record.
type HistoryMessage struct {
	Message Message `json:"message"`
}

// HistoryLabel represents label changes in a history record.
type HistoryLabel struct {
	Message  Message  `json:"message"`
	LabelIDs []string `json:"labelIds"`
}

// Label represents a Gmail label.
type Label struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Type                  string `json:"type"`
	MessagesTotal         int64  `json:"messagesTotal"`
	MessagesUnread        int64  `json:"messagesUnread"`
	ThreadsTotal          int64  `json:"threadsTotal"`
	ThreadsUnread         int64  `json:"threadsUnread"`
	MessageListVisibility string `json:"messageListVisibility"`
	LabelListVisibility   string `json:"labelListVisibility"`
	Color                 *LabelColor `json:"color,omitempty"`
}

// LabelColor represents Gmail label colors.
type LabelColor struct {
	TextColor       string `json:"textColor"`
	BackgroundColor string `json:"backgroundColor"`
}

// LabelList represents a list of labels.
type LabelList struct {
	Labels []Label `json:"labels"`
}

// Profile represents a Gmail user profile.
type Profile struct {
	EmailAddress  string `json:"emailAddress"`
	MessagesTotal int64  `json:"messagesTotal"`
	ThreadsTotal  int64  `json:"threadsTotal"`
	HistoryID     uint64 `json:"historyId,string"`
}

// MessageHandler is called for each message during sync.
type MessageHandler func(ctx context.Context, msg *Message) error

// LabelHandler is called for label updates during sync.
type LabelHandler func(ctx context.Context, labels []Label) error

// EngineConfig holds configuration for the sync engine.
type EngineConfig struct {
	// OAuth2Manager for token management.
	OAuth2Manager *oauth.OAuth2Manager

	// StateStorage for persisting sync state.
	StateStorage StateStorage

	// HTTPClient for making API requests.
	HTTPClient *http.Client

	// BatchSize is the number of messages to fetch per batch.
	BatchSize int

	// RateLimit is the maximum requests per second.
	RateLimit int

	// BurstLimit is the burst allowance for rate limiting.
	BurstLimit int

	// MaxRetries is the maximum retry attempts for failed requests.
	MaxRetries int

	// InitialBackoff is the initial backoff duration for retries.
	InitialBackoff time.Duration

	// MaxBackoff is the maximum backoff duration.
	MaxBackoff time.Duration

	// BackoffMultiplier is the multiplier for exponential backoff.
	BackoffMultiplier float64

	// Logger for logging.
	Logger logging.Logger
}

// DefaultEngineConfig returns an EngineConfig with sensible defaults.
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		BatchSize:         DefaultBatchSize,
		RateLimit:         DefaultRateLimit,
		BurstLimit:        DefaultBurstLimit,
		MaxRetries:        DefaultMaxRetries,
		InitialBackoff:    DefaultInitialBackoff,
		MaxBackoff:        DefaultMaxBackoff,
		BackoffMultiplier: DefaultBackoffMultiple,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Engine manages Gmail synchronization operations.
type Engine struct {
	config *EngineConfig
	mu     sync.RWMutex

	// Rate limiter state.
	rateLimiter *RateLimiter

	// Metrics for monitoring.
	metrics *SyncMetrics
}

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(ratePerSecond, burst int) *RateLimiter {
	return &RateLimiter{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: float64(ratePerSecond),
		lastRefill: time.Now(),
	}
}

// Wait waits until a token is available.
func (r *RateLimiter) Wait(ctx context.Context) error {
	for {
		r.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(r.lastRefill).Seconds()
		r.tokens = math.Min(r.maxTokens, r.tokens+elapsed*r.refillRate)
		r.lastRefill = now

		if r.tokens >= 1 {
			r.tokens--
			r.mu.Unlock()
			return nil
		}

		// Calculate wait time
		waitTime := time.Duration((1 - r.tokens) / r.refillRate * float64(time.Second))
		r.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue to next iteration
		}
	}
}

// SyncMetrics holds Prometheus metrics for sync operations.
type SyncMetrics struct {
	SyncsStarted       prometheus.Counter
	SyncsCompleted     prometheus.Counter
	SyncsFailed        prometheus.Counter
	MessagesProcessed  prometheus.Counter
	MessagesFailed     prometheus.Counter
	APIRequests        prometheus.Counter
	APIErrors          prometheus.Counter
	APILatency         prometheus.Histogram
	RateLimitHits      prometheus.Counter
	RetryAttempts      prometheus.Counter
}

// NewSyncMetrics creates and registers sync metrics.
func NewSyncMetrics(namespace, service string) *SyncMetrics {
	m := &SyncMetrics{
		SyncsStarted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_syncs_started_total",
			Help:        "Total number of sync operations started",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		SyncsCompleted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_syncs_completed_total",
			Help:        "Total number of sync operations completed successfully",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		SyncsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_syncs_failed_total",
			Help:        "Total number of sync operations that failed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		MessagesProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_messages_processed_total",
			Help:        "Total number of messages processed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		MessagesFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_messages_failed_total",
			Help:        "Total number of messages that failed to process",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		APIRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_api_requests_total",
			Help:        "Total number of Gmail API requests",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		APIErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_api_errors_total",
			Help:        "Total number of Gmail API errors",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		APILatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:   namespace,
			Name:        "gmail_api_latency_seconds",
			Help:        "Gmail API request latency in seconds",
			Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			ConstLabels: prometheus.Labels{"service": service},
		}),
		RateLimitHits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_rate_limit_hits_total",
			Help:        "Total number of rate limit hits",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		RetryAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_retry_attempts_total",
			Help:        "Total number of retry attempts",
			ConstLabels: prometheus.Labels{"service": service},
		}),
	}
	return m
}

// Register registers all metrics with the default Prometheus registry.
func (m *SyncMetrics) Register() error {
	collectors := []prometheus.Collector{
		m.SyncsStarted,
		m.SyncsCompleted,
		m.SyncsFailed,
		m.MessagesProcessed,
		m.MessagesFailed,
		m.APIRequests,
		m.APIErrors,
		m.APILatency,
		m.RateLimitHits,
		m.RetryAttempts,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// NewEngine creates a new sync engine.
func NewEngine(config *EngineConfig) (*Engine, error) {
	if config == nil {
		config = DefaultEngineConfig()
	}

	if config.OAuth2Manager == nil {
		return nil, fmt.Errorf("oauth2 manager is required")
	}

	if config.StateStorage == nil {
		return nil, fmt.Errorf("state storage is required")
	}

	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}

	if config.BatchSize <= 0 || config.BatchSize > MaxBatchSize {
		config.BatchSize = DefaultBatchSize
	}

	if config.RateLimit <= 0 {
		config.RateLimit = DefaultRateLimit
	}

	if config.BurstLimit <= 0 {
		config.BurstLimit = DefaultBurstLimit
	}

	if config.MaxRetries <= 0 {
		config.MaxRetries = DefaultMaxRetries
	}

	if config.InitialBackoff <= 0 {
		config.InitialBackoff = DefaultInitialBackoff
	}

	if config.MaxBackoff <= 0 {
		config.MaxBackoff = DefaultMaxBackoff
	}

	if config.BackoffMultiplier <= 0 {
		config.BackoffMultiplier = DefaultBackoffMultiple
	}

	return &Engine{
		config:      config,
		rateLimiter: NewRateLimiter(config.RateLimit, config.BurstLimit),
		metrics:     NewSyncMetrics("penfold", "gmail_connector"),
	}, nil
}

// SetMetrics sets custom metrics for the engine.
func (e *Engine) SetMetrics(metrics *SyncMetrics) {
	e.metrics = metrics
}

// SyncOptions holds options for sync operations.
type SyncOptions struct {
	// Labels to sync (empty means all).
	Labels []string

	// Query is the Gmail search query filter.
	Query string

	// MaxResults limits the total messages to sync.
	MaxResults int64

	// IncludeSpamTrash includes spam and trash folders.
	IncludeSpamTrash bool

	// ForceFullSync ignores the last history ID and does a full sync.
	ForceFullSync bool

	// MessageHandler is called for each message.
	MessageHandler MessageHandler

	// LabelHandler is called for label updates.
	LabelHandler LabelHandler
}

// SyncResult holds the result of a sync operation.
type SyncResult struct {
	// SyncID is the unique identifier for this sync.
	SyncID string

	// SyncType is the type of sync performed.
	SyncType SyncType

	// ProcessedCount is the number of messages processed.
	ProcessedCount int64

	// SuccessCount is the number of successfully synced messages.
	SuccessCount int64

	// ErrorCount is the number of errors encountered.
	ErrorCount int64

	// HistoryID is the new history ID after sync.
	HistoryID uint64

	// Duration is how long the sync took.
	Duration time.Duration

	// Errors encountered during sync.
	Errors []SyncErrorRecord
}

// FullSync performs a full mailbox sync for a tenant.
func (e *Engine) FullSync(ctx context.Context, tenantID string, opts *SyncOptions) (*SyncResult, error) {
	if opts == nil {
		opts = &SyncOptions{}
	}

	syncID := uuid.New().String()
	startTime := time.Now()

	if e.metrics != nil {
		e.metrics.SyncsStarted.Inc()
	}

	e.log().Info("starting full sync",
		logging.F("tenant_id", tenantID),
		logging.F("sync_id", syncID),
	)

	// Create initial sync state.
	state := &SyncState{
		TenantID:   tenantID,
		SyncID:     syncID,
		SyncType:   SyncTypeFull,
		Status:     SyncStatusInProgress,
		Labels:     opts.Labels,
		Query:      opts.Query,
		CreatedAt:  startTime,
		UpdatedAt:  startTime,
	}

	if err := e.config.StateStorage.SaveState(ctx, state); err != nil {
		return nil, fmt.Errorf("saving initial state: %w", err)
	}

	// Get access token.
	accessToken, err := e.config.OAuth2Manager.GetValidToken(ctx, tenantID)
	if err != nil {
		e.markSyncFailed(ctx, state, err)
		return nil, fmt.Errorf("getting access token: %w", err)
	}

	// Get profile to get the initial history ID and estimate.
	profile, err := e.getProfile(ctx, accessToken)
	if err != nil {
		e.markSyncFailed(ctx, state, err)
		return nil, fmt.Errorf("getting profile: %w", err)
	}

	state.TotalCount = profile.MessagesTotal
	state.HistoryID = profile.HistoryID

	// Sync labels first if handler provided.
	if opts.LabelHandler != nil {
		labels, err := e.listLabels(ctx, accessToken)
		if err != nil {
			e.log().Warn("failed to sync labels", logging.Err(err))
		} else if err := opts.LabelHandler(ctx, labels); err != nil {
			e.log().Warn("label handler error", logging.Err(err))
		}
	}

	// Perform the full message sync.
	result := &SyncResult{
		SyncID:   syncID,
		SyncType: SyncTypeFull,
	}

	pageToken := ""
	for {
		select {
		case <-ctx.Done():
			state.Status = SyncStatusInterrupted
			state.NextPageToken = pageToken
			e.config.StateStorage.SaveState(ctx, state)
			return nil, ctx.Err()
		default:
		}

		// List messages.
		msgList, err := e.listMessages(ctx, accessToken, opts, pageToken)
		if err != nil {
			state.Errors = append(state.Errors, SyncErrorRecord{
				Code:      "LIST_MESSAGES_ERROR",
				Message:   err.Error(),
				Timestamp: time.Now(),
				Retryable: true,
			})
			state.ErrorCount++
			e.config.StateStorage.SaveState(ctx, state)
			continue
		}

		// Fetch full messages in batches.
		for i := 0; i < len(msgList.Messages); i += e.config.BatchSize {
			end := i + e.config.BatchSize
			if end > len(msgList.Messages) {
				end = len(msgList.Messages)
			}

			batch := msgList.Messages[i:end]
			messages, errs := e.fetchMessagesBatch(ctx, accessToken, batch)

			for _, msg := range messages {
				if opts.MessageHandler != nil {
					if err := opts.MessageHandler(ctx, msg); err != nil {
						state.Errors = append(state.Errors, SyncErrorRecord{
							EmailID:   msg.ID,
							Code:      "HANDLER_ERROR",
							Message:   err.Error(),
							Timestamp: time.Now(),
							Retryable: false,
						})
						state.ErrorCount++
						result.ErrorCount++
						if e.metrics != nil {
							e.metrics.MessagesFailed.Inc()
						}
					} else {
						result.SuccessCount++
						if e.metrics != nil {
							e.metrics.MessagesProcessed.Inc()
						}
					}
				} else {
					result.SuccessCount++
					if e.metrics != nil {
						e.metrics.MessagesProcessed.Inc()
					}
				}
				state.ProcessedCount++
				result.ProcessedCount++
			}

			for _, syncErr := range errs {
				state.Errors = append(state.Errors, syncErr)
				state.ErrorCount++
				result.ErrorCount++
			}

			// Update state periodically.
			e.config.StateStorage.SaveState(ctx, state)

			// Check max results.
			if opts.MaxResults > 0 && state.ProcessedCount >= opts.MaxResults {
				break
			}
		}

		if msgList.NextPageToken == "" || (opts.MaxResults > 0 && state.ProcessedCount >= opts.MaxResults) {
			break
		}
		pageToken = msgList.NextPageToken
	}

	// Complete the sync.
	result.Duration = time.Since(startTime)
	result.HistoryID = state.HistoryID
	result.Errors = state.Errors

	state.LastSyncAt = time.Now()
	state.LastFullSyncAt = state.LastSyncAt
	if state.ErrorCount > 0 {
		state.Status = SyncStatusCompletedWithErrors
	} else {
		state.Status = SyncStatusCompleted
	}
	e.config.StateStorage.SaveState(ctx, state)

	if e.metrics != nil {
		e.metrics.SyncsCompleted.Inc()
	}

	e.log().Info("full sync completed",
		logging.F("tenant_id", tenantID),
		logging.F("sync_id", syncID),
		logging.F("processed", result.ProcessedCount),
		logging.F("success", result.SuccessCount),
		logging.F("errors", result.ErrorCount),
		logging.F("duration", result.Duration),
	)

	return result, nil
}

// IncrementalSync performs an incremental sync using the History API.
func (e *Engine) IncrementalSync(ctx context.Context, tenantID string, opts *SyncOptions) (*SyncResult, error) {
	if opts == nil {
		opts = &SyncOptions{}
	}

	// Get the last sync state to get the history ID.
	lastState, err := e.config.StateStorage.GetLatestState(ctx, tenantID)
	if err == ErrStateNotFound || opts.ForceFullSync {
		// No previous sync, do a full sync instead.
		return e.FullSync(ctx, tenantID, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("getting last sync state: %w", err)
	}

	if lastState.HistoryID == 0 {
		// No history ID, do a full sync.
		return e.FullSync(ctx, tenantID, opts)
	}

	syncID := uuid.New().String()
	startTime := time.Now()

	if e.metrics != nil {
		e.metrics.SyncsStarted.Inc()
	}

	e.log().Info("starting incremental sync",
		logging.F("tenant_id", tenantID),
		logging.F("sync_id", syncID),
		logging.F("from_history_id", lastState.HistoryID),
	)

	// Create sync state.
	state := &SyncState{
		TenantID:   tenantID,
		SyncID:     syncID,
		SyncType:   SyncTypeIncremental,
		Status:     SyncStatusInProgress,
		HistoryID:  lastState.HistoryID,
		Labels:     opts.Labels,
		Query:      opts.Query,
		CreatedAt:  startTime,
		UpdatedAt:  startTime,
	}

	if err := e.config.StateStorage.SaveState(ctx, state); err != nil {
		return nil, fmt.Errorf("saving initial state: %w", err)
	}

	// Get access token.
	accessToken, err := e.config.OAuth2Manager.GetValidToken(ctx, tenantID)
	if err != nil {
		e.markSyncFailed(ctx, state, err)
		return nil, fmt.Errorf("getting access token: %w", err)
	}

	result := &SyncResult{
		SyncID:   syncID,
		SyncType: SyncTypeIncremental,
	}

	// Track which messages need to be fetched/updated.
	messagesAdded := make(map[string]bool)
	messagesDeleted := make(map[string]bool)
	labelsChanged := make(map[string]bool)

	pageToken := ""
	for {
		select {
		case <-ctx.Done():
			state.Status = SyncStatusInterrupted
			state.NextPageToken = pageToken
			e.config.StateStorage.SaveState(ctx, state)
			return nil, ctx.Err()
		default:
		}

		historyList, err := e.listHistory(ctx, accessToken, lastState.HistoryID, opts.Labels, pageToken)
		if err != nil {
			// History ID might be too old, fall back to full sync.
			if isHistoryIDExpiredError(err) {
				e.log().Warn("history ID expired, falling back to full sync",
					logging.F("tenant_id", tenantID),
					logging.F("history_id", lastState.HistoryID),
				)
				return e.FullSync(ctx, tenantID, opts)
			}

			state.Errors = append(state.Errors, SyncErrorRecord{
				Code:      "LIST_HISTORY_ERROR",
				Message:   err.Error(),
				Timestamp: time.Now(),
				Retryable: true,
			})
			state.ErrorCount++
			e.config.StateStorage.SaveState(ctx, state)
			continue
		}

		// Update the history ID.
		if historyList.HistoryID > state.HistoryID {
			state.HistoryID = historyList.HistoryID
		}

		// Process history records.
		for _, history := range historyList.History {
			for _, added := range history.MessagesAdded {
				messagesAdded[added.Message.ID] = true
				delete(messagesDeleted, added.Message.ID)
			}
			for _, deleted := range history.MessagesDeleted {
				messagesDeleted[deleted.Message.ID] = true
				delete(messagesAdded, deleted.Message.ID)
			}
			for _, labelAdded := range history.LabelsAdded {
				labelsChanged[labelAdded.Message.ID] = true
			}
			for _, labelRemoved := range history.LabelsRemoved {
				labelsChanged[labelRemoved.Message.ID] = true
			}
		}

		if historyList.NextPageToken == "" {
			break
		}
		pageToken = historyList.NextPageToken
	}

	// Merge label-changed messages into added (they need refetching).
	for msgID := range labelsChanged {
		if !messagesDeleted[msgID] {
			messagesAdded[msgID] = true
		}
	}

	state.TotalCount = int64(len(messagesAdded) + len(messagesDeleted))

	// Fetch added/modified messages.
	var msgRefs []MessageRef
	for msgID := range messagesAdded {
		msgRefs = append(msgRefs, MessageRef{ID: msgID})
	}

	for i := 0; i < len(msgRefs); i += e.config.BatchSize {
		end := i + e.config.BatchSize
		if end > len(msgRefs) {
			end = len(msgRefs)
		}

		batch := msgRefs[i:end]
		messages, errs := e.fetchMessagesBatch(ctx, accessToken, batch)

		for _, msg := range messages {
			if opts.MessageHandler != nil {
				if err := opts.MessageHandler(ctx, msg); err != nil {
					state.Errors = append(state.Errors, SyncErrorRecord{
						EmailID:   msg.ID,
						Code:      "HANDLER_ERROR",
						Message:   err.Error(),
						Timestamp: time.Now(),
						Retryable: false,
					})
					state.ErrorCount++
					result.ErrorCount++
					if e.metrics != nil {
						e.metrics.MessagesFailed.Inc()
					}
				} else {
					result.SuccessCount++
					if e.metrics != nil {
						e.metrics.MessagesProcessed.Inc()
					}
				}
			} else {
				result.SuccessCount++
				if e.metrics != nil {
					e.metrics.MessagesProcessed.Inc()
				}
			}
			state.ProcessedCount++
			result.ProcessedCount++
		}

		for _, syncErr := range errs {
			state.Errors = append(state.Errors, syncErr)
			state.ErrorCount++
			result.ErrorCount++
		}

		e.config.StateStorage.SaveState(ctx, state)
	}

	// Complete the sync.
	result.Duration = time.Since(startTime)
	result.HistoryID = state.HistoryID
	result.Errors = state.Errors

	state.LastSyncAt = time.Now()
	if state.ErrorCount > 0 {
		state.Status = SyncStatusCompletedWithErrors
	} else {
		state.Status = SyncStatusCompleted
	}
	e.config.StateStorage.SaveState(ctx, state)

	if e.metrics != nil {
		e.metrics.SyncsCompleted.Inc()
	}

	e.log().Info("incremental sync completed",
		logging.F("tenant_id", tenantID),
		logging.F("sync_id", syncID),
		logging.F("processed", result.ProcessedCount),
		logging.F("success", result.SuccessCount),
		logging.F("errors", result.ErrorCount),
		logging.F("duration", result.Duration),
	)

	return result, nil
}

// ResumeSync resumes an interrupted sync operation.
func (e *Engine) ResumeSync(ctx context.Context, tenantID string, opts *SyncOptions) (*SyncResult, error) {
	if opts == nil {
		opts = &SyncOptions{}
	}

	// Get the interrupted sync state.
	state, err := e.config.StateStorage.GetState(ctx, tenantID)
	if err == ErrStateNotFound {
		// No interrupted sync, start a new incremental sync.
		return e.IncrementalSync(ctx, tenantID, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("getting sync state: %w", err)
	}

	if !state.CanResume() {
		// Can't resume, start a new sync.
		if state.SyncType == SyncTypeFull {
			return e.FullSync(ctx, tenantID, opts)
		}
		return e.IncrementalSync(ctx, tenantID, opts)
	}

	startTime := time.Now()

	if e.metrics != nil {
		e.metrics.SyncsStarted.Inc()
	}

	e.log().Info("resuming sync",
		logging.F("tenant_id", tenantID),
		logging.F("sync_id", state.SyncID),
		logging.F("from_page_token", state.NextPageToken),
	)

	// Update state to in progress.
	state.SyncType = SyncTypeResume
	state.Status = SyncStatusInProgress
	if err := e.config.StateStorage.SaveState(ctx, state); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}

	// Get access token.
	accessToken, err := e.config.OAuth2Manager.GetValidToken(ctx, tenantID)
	if err != nil {
		e.markSyncFailed(ctx, state, err)
		return nil, fmt.Errorf("getting access token: %w", err)
	}

	result := &SyncResult{
		SyncID:         state.SyncID,
		SyncType:       SyncTypeResume,
		ProcessedCount: state.ProcessedCount,
		SuccessCount:   state.ProcessedCount - state.ErrorCount,
		ErrorCount:     state.ErrorCount,
		Errors:         state.Errors,
	}

	pageToken := state.NextPageToken
	for {
		select {
		case <-ctx.Done():
			state.Status = SyncStatusInterrupted
			state.NextPageToken = pageToken
			e.config.StateStorage.SaveState(ctx, state)
			return nil, ctx.Err()
		default:
		}

		msgList, err := e.listMessages(ctx, accessToken, opts, pageToken)
		if err != nil {
			state.Errors = append(state.Errors, SyncErrorRecord{
				Code:      "LIST_MESSAGES_ERROR",
				Message:   err.Error(),
				Timestamp: time.Now(),
				Retryable: true,
			})
			state.ErrorCount++
			e.config.StateStorage.SaveState(ctx, state)
			continue
		}

		for i := 0; i < len(msgList.Messages); i += e.config.BatchSize {
			end := i + e.config.BatchSize
			if end > len(msgList.Messages) {
				end = len(msgList.Messages)
			}

			batch := msgList.Messages[i:end]
			messages, errs := e.fetchMessagesBatch(ctx, accessToken, batch)

			for _, msg := range messages {
				if opts.MessageHandler != nil {
					if err := opts.MessageHandler(ctx, msg); err != nil {
						state.Errors = append(state.Errors, SyncErrorRecord{
							EmailID:   msg.ID,
							Code:      "HANDLER_ERROR",
							Message:   err.Error(),
							Timestamp: time.Now(),
							Retryable: false,
						})
						state.ErrorCount++
						result.ErrorCount++
						if e.metrics != nil {
							e.metrics.MessagesFailed.Inc()
						}
					} else {
						result.SuccessCount++
						if e.metrics != nil {
							e.metrics.MessagesProcessed.Inc()
						}
					}
				} else {
					result.SuccessCount++
					if e.metrics != nil {
						e.metrics.MessagesProcessed.Inc()
					}
				}
				state.ProcessedCount++
				result.ProcessedCount++
			}

			for _, syncErr := range errs {
				state.Errors = append(state.Errors, syncErr)
				state.ErrorCount++
				result.ErrorCount++
			}

			e.config.StateStorage.SaveState(ctx, state)
		}

		if msgList.NextPageToken == "" {
			break
		}
		pageToken = msgList.NextPageToken
	}

	// Complete the sync.
	result.Duration = time.Since(startTime)
	result.HistoryID = state.HistoryID
	result.Errors = state.Errors

	state.LastSyncAt = time.Now()
	state.NextPageToken = ""
	if state.ErrorCount > 0 {
		state.Status = SyncStatusCompletedWithErrors
	} else {
		state.Status = SyncStatusCompleted
	}
	e.config.StateStorage.SaveState(ctx, state)

	if e.metrics != nil {
		e.metrics.SyncsCompleted.Inc()
	}

	e.log().Info("resumed sync completed",
		logging.F("tenant_id", tenantID),
		logging.F("sync_id", state.SyncID),
		logging.F("processed", result.ProcessedCount),
		logging.F("success", result.SuccessCount),
		logging.F("errors", result.ErrorCount),
		logging.F("duration", result.Duration),
	)

	return result, nil
}

// GetSyncStatus returns the current sync status for a tenant.
func (e *Engine) GetSyncStatus(ctx context.Context, tenantID string) (*SyncState, error) {
	return e.config.StateStorage.GetState(ctx, tenantID)
}

// GetSyncStatusByID returns a specific sync status by ID.
func (e *Engine) GetSyncStatusByID(ctx context.Context, syncID string) (*SyncState, error) {
	return e.config.StateStorage.GetStateByID(ctx, syncID)
}

// CancelSync cancels an in-progress sync operation.
func (e *Engine) CancelSync(ctx context.Context, syncID string) error {
	state, err := e.config.StateStorage.GetStateByID(ctx, syncID)
	if err != nil {
		return err
	}

	if state.IsComplete() {
		return fmt.Errorf("sync already completed")
	}

	state.Status = SyncStatusCancelled
	state.UpdatedAt = time.Now()
	return e.config.StateStorage.SaveState(ctx, state)
}

// API helper methods.

func (e *Engine) getProfile(ctx context.Context, accessToken string) (*Profile, error) {
	url := GmailAPIBaseURL + "/users/me/profile"

	var profile Profile
	if err := e.doAPIRequest(ctx, accessToken, "GET", url, nil, &profile); err != nil {
		return nil, err
	}

	return &profile, nil
}

func (e *Engine) listLabels(ctx context.Context, accessToken string) ([]Label, error) {
	url := GmailAPIBaseURL + "/users/me/labels"

	var labelList LabelList
	if err := e.doAPIRequest(ctx, accessToken, "GET", url, nil, &labelList); err != nil {
		return nil, err
	}

	return labelList.Labels, nil
}

func (e *Engine) listMessages(ctx context.Context, accessToken string, opts *SyncOptions, pageToken string) (*MessageList, error) {
	u, _ := url.Parse(GmailAPIBaseURL + "/users/me/messages")
	q := u.Query()

	if opts.Query != "" {
		q.Set("q", opts.Query)
	}
	if len(opts.Labels) > 0 {
		for _, label := range opts.Labels {
			q.Add("labelIds", label)
		}
	}
	if opts.IncludeSpamTrash {
		q.Set("includeSpamTrash", "true")
	}
	q.Set("maxResults", strconv.Itoa(e.config.BatchSize))
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}

	u.RawQuery = q.Encode()

	var msgList MessageList
	if err := e.doAPIRequest(ctx, accessToken, "GET", u.String(), nil, &msgList); err != nil {
		return nil, err
	}

	return &msgList, nil
}

func (e *Engine) listHistory(ctx context.Context, accessToken string, startHistoryID uint64, labels []string, pageToken string) (*HistoryList, error) {
	u, _ := url.Parse(GmailAPIBaseURL + "/users/me/history")
	q := u.Query()

	q.Set("startHistoryId", strconv.FormatUint(startHistoryID, 10))
	for _, label := range labels {
		q.Add("labelId", label)
	}
	q.Set("maxResults", "500")
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}

	u.RawQuery = q.Encode()

	var historyList HistoryList
	if err := e.doAPIRequest(ctx, accessToken, "GET", u.String(), nil, &historyList); err != nil {
		return nil, err
	}

	return &historyList, nil
}

func (e *Engine) fetchMessage(ctx context.Context, accessToken, messageID, format string) (*Message, error) {
	if format == "" {
		format = "full"
	}

	u := fmt.Sprintf("%s/users/me/messages/%s?format=%s", GmailAPIBaseURL, messageID, format)

	var msg Message
	if err := e.doAPIRequest(ctx, accessToken, "GET", u, nil, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (e *Engine) fetchMessagesBatch(ctx context.Context, accessToken string, msgRefs []MessageRef) ([]*Message, []SyncErrorRecord) {
	var messages []*Message
	var errors []SyncErrorRecord

	// Process in parallel with limited concurrency.
	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)
	results := make(chan struct {
		msg *Message
		err SyncErrorRecord
	}, len(msgRefs))

	var wg sync.WaitGroup
	for _, ref := range msgRefs {
		wg.Add(1)
		go func(msgRef MessageRef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			msg, err := e.fetchMessage(ctx, accessToken, msgRef.ID, "full")
			if err != nil {
				results <- struct {
					msg *Message
					err SyncErrorRecord
				}{
					err: SyncErrorRecord{
						EmailID:   msgRef.ID,
						Code:      "FETCH_ERROR",
						Message:   err.Error(),
						Timestamp: time.Now(),
						Retryable: true,
					},
				}
				return
			}
			results <- struct {
				msg *Message
				err SyncErrorRecord
			}{msg: msg}
		}(ref)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if result.msg != nil {
			messages = append(messages, result.msg)
		} else {
			errors = append(errors, result.err)
		}
	}

	return messages, errors
}

// doAPIRequest performs an API request with rate limiting and retries.
func (e *Engine) doAPIRequest(ctx context.Context, accessToken, method, url string, body io.Reader, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		// Wait for rate limiter.
		if err := e.rateLimiter.Wait(ctx); err != nil {
			return err
		}

		if e.metrics != nil {
			e.metrics.APIRequests.Inc()
		}

		start := time.Now()

		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")

		resp, err := e.config.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			if e.metrics != nil {
				e.metrics.APIErrors.Inc()
				e.metrics.RetryAttempts.Inc()
			}
			continue
		}

		if e.metrics != nil {
			e.metrics.APILatency.Observe(time.Since(start).Seconds())
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("reading response: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			if result != nil {
				if err := json.Unmarshal(respBody, result); err != nil {
					return fmt.Errorf("parsing response: %w", err)
				}
			}
			return nil
		}

		// Handle rate limiting.
		if resp.StatusCode == http.StatusTooManyRequests {
			if e.metrics != nil {
				e.metrics.RateLimitHits.Inc()
				e.metrics.APIErrors.Inc()
			}

			// Get retry-after header.
			retryAfter := resp.Header.Get("Retry-After")
			var backoff time.Duration
			if retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					backoff = time.Duration(seconds) * time.Second
				}
			}
			if backoff == 0 {
				backoff = e.calculateBackoff(attempt)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			if e.metrics != nil {
				e.metrics.RetryAttempts.Inc()
			}
			continue
		}

		// Handle other errors.
		if resp.StatusCode >= 400 {
			var apiErr struct {
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
					Status  string `json:"status"`
				} `json:"error"`
			}
			if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Error.Message != "" {
				lastErr = &APIError{
					Code:       apiErr.Error.Code,
					Message:    apiErr.Error.Message,
					Status:     apiErr.Error.Status,
					StatusCode: resp.StatusCode,
				}
			} else {
				lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
			}

			if e.metrics != nil {
				e.metrics.APIErrors.Inc()
			}

			// Retry on server errors.
			if resp.StatusCode >= 500 {
				backoff := e.calculateBackoff(attempt)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
				if e.metrics != nil {
					e.metrics.RetryAttempts.Inc()
				}
				continue
			}

			// Don't retry client errors.
			return lastErr
		}
	}

	if lastErr != nil {
		return fmt.Errorf("max retries exceeded: %w", lastErr)
	}
	return fmt.Errorf("max retries exceeded")
}

func (e *Engine) calculateBackoff(attempt int) time.Duration {
	backoff := float64(e.config.InitialBackoff) * math.Pow(e.config.BackoffMultiplier, float64(attempt))
	if backoff > float64(e.config.MaxBackoff) {
		backoff = float64(e.config.MaxBackoff)
	}
	return time.Duration(backoff)
}

func (e *Engine) markSyncFailed(ctx context.Context, state *SyncState, err error) {
	state.Status = SyncStatusFailed
	state.Errors = append(state.Errors, SyncErrorRecord{
		Code:      "SYNC_FAILED",
		Message:   err.Error(),
		Timestamp: time.Now(),
		Retryable: false,
	})
	state.ErrorCount++
	e.config.StateStorage.SaveState(ctx, state)

	if e.metrics != nil {
		e.metrics.SyncsFailed.Inc()
	}
}

func (e *Engine) log() logging.Logger {
	if e.config.Logger != nil {
		return e.config.Logger
	}
	return logging.MustGlobal()
}

// APIError represents a Gmail API error.
type APIError struct {
	Code       int
	Message    string
	Status     string
	StatusCode int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Gmail API error %d (%s): %s", e.Code, e.Status, e.Message)
}

// IsHistoryIDExpired returns true if the error indicates the history ID is expired.
func (e *APIError) IsHistoryIDExpired() bool {
	return e.Status == "INVALID_ARGUMENT" && strings.Contains(e.Message, "historyId")
}

func isHistoryIDExpiredError(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.IsHistoryIDExpired()
	}
	return false
}

// ParsedMessage holds parsed email data from a Gmail message.
type ParsedMessage struct {
	ID           string
	ThreadID     string
	Subject      string
	From         string
	To           []string
	CC           []string
	BCC          []string
	Date         time.Time
	PlainText    string
	HTML         string
	Labels       []string
	IsRead       bool
	IsStarred    bool
	HasAttachments bool
	Attachments  []AttachmentInfo
	InternalDate int64
	SizeEstimate int64
	Snippet      string
	MessageID    string
	InReplyTo    string
	References   []string
	HistoryID    uint64
}

// AttachmentInfo holds attachment metadata.
type AttachmentInfo struct {
	ID       string
	Filename string
	MimeType string
	Size     int64
}

// ParseMessage extracts readable data from a Gmail message.
func ParseMessage(msg *Message) *ParsedMessage {
	parsed := &ParsedMessage{
		ID:           msg.ID,
		ThreadID:     msg.ThreadID,
		Labels:       msg.LabelIDs,
		Snippet:      msg.Snippet,
		SizeEstimate: msg.SizeEstimate,
		HistoryID:    msg.HistoryID,
	}

	// Parse internal date.
	if msg.InternalDate != "" {
		if ms, err := strconv.ParseInt(msg.InternalDate, 10, 64); err == nil {
			parsed.InternalDate = ms
			parsed.Date = time.UnixMilli(ms)
		}
	}

	// Check labels for read/starred status.
	for _, label := range msg.LabelIDs {
		switch label {
		case "UNREAD":
			parsed.IsRead = false
		case "STARRED":
			parsed.IsStarred = true
		}
	}
	// Default to read if UNREAD not present.
	hasUnread := false
	for _, label := range msg.LabelIDs {
		if label == "UNREAD" {
			hasUnread = true
			break
		}
	}
	parsed.IsRead = !hasUnread

	// Parse headers and body.
	if msg.Payload != nil {
		parsed.parsePayload(msg.Payload)
	}

	return parsed
}

func (p *ParsedMessage) parsePayload(payload *MessagePayload) {
	// Parse headers.
	for _, h := range payload.Headers {
		switch strings.ToLower(h.Name) {
		case "subject":
			p.Subject = h.Value
		case "from":
			p.From = h.Value
		case "to":
			p.To = parseAddressList(h.Value)
		case "cc":
			p.CC = parseAddressList(h.Value)
		case "bcc":
			p.BCC = parseAddressList(h.Value)
		case "message-id":
			p.MessageID = h.Value
		case "in-reply-to":
			p.InReplyTo = h.Value
		case "references":
			p.References = strings.Fields(h.Value)
		case "date":
			if t, err := time.Parse(time.RFC1123Z, h.Value); err == nil {
				p.Date = t
			} else if t, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", h.Value); err == nil {
				p.Date = t
			}
		}
	}

	// Parse body.
	p.parseBodyParts(payload)
}

func (p *ParsedMessage) parseBodyParts(payload *MessagePayload) {
	if payload.Body != nil && payload.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			switch payload.MimeType {
			case "text/plain":
				p.PlainText = string(data)
			case "text/html":
				p.HTML = string(data)
			case "text/calendar":
				// Extract calendar event metadata (title, time, attendees)
				// and append to PlainText so downstream pipeline stages see it.
				if summary := parseICSMetadata(string(data)); summary != "" {
					if p.PlainText != "" {
						p.PlainText += "\n\n"
					}
					p.PlainText += summary
				}
			}
		}
	}

	// Check for attachments.
	if payload.Filename != "" && payload.Body != nil && payload.Body.AttachmentID != "" {
		p.HasAttachments = true
		p.Attachments = append(p.Attachments, AttachmentInfo{
			ID:       payload.Body.AttachmentID,
			Filename: payload.Filename,
			MimeType: payload.MimeType,
			Size:     payload.Body.Size,
		})
	}

	// Recurse into parts.
	for _, part := range payload.Parts {
		p.parseBodyParts(part)
	}
}

// parseICSMetadata extracts key calendar event fields from an iCalendar (ICS)
// text/calendar body and returns a human-readable summary string.
// Returns "" if no VEVENT is found.
func parseICSMetadata(icsData string) string {
	var summary, dtStart, dtEnd, organizer string
	var attendees []string
	inEvent := false

	for _, line := range strings.Split(icsData, "\n") {
		line = strings.TrimRight(line, "\r")

		if line == "BEGIN:VEVENT" {
			inEvent = true
			continue
		}
		if line == "END:VEVENT" {
			break
		}
		if !inEvent {
			continue
		}

		// Handle key-value pairs (ICS uses KEY;PARAMS:VALUE or KEY:VALUE)
		if strings.HasPrefix(line, "SUMMARY:") {
			summary = strings.TrimPrefix(line, "SUMMARY:")
		} else if strings.HasPrefix(line, "DTSTART") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				dtStart = line[idx+1:]
			}
		} else if strings.HasPrefix(line, "DTEND") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				dtEnd = line[idx+1:]
			}
		} else if strings.HasPrefix(line, "ORGANIZER") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				val := line[idx+1:]
				organizer = strings.TrimPrefix(val, "mailto:")
			}
		} else if strings.HasPrefix(line, "ATTENDEE") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				val := line[idx+1:]
				attendees = append(attendees, strings.TrimPrefix(val, "mailto:"))
			}
		}
	}

	if summary == "" && organizer == "" && len(attendees) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, "[Calendar Event]")
	if summary != "" {
		parts = append(parts, "Title: "+summary)
	}
	if dtStart != "" {
		parts = append(parts, "Start: "+dtStart)
	}
	if dtEnd != "" {
		parts = append(parts, "End: "+dtEnd)
	}
	if organizer != "" {
		parts = append(parts, "Organizer: "+organizer)
	}
	if len(attendees) > 0 {
		parts = append(parts, "Attendees: "+strings.Join(attendees, ", "))
	}

	return strings.Join(parts, "\n")
}

func parseAddressList(s string) []string {
	// Simple split by comma - real implementation would use mail.ParseAddressList.
	var addresses []string
	for _, addr := range strings.Split(s, ",") {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			addresses = append(addresses, addr)
		}
	}
	return addresses
}
