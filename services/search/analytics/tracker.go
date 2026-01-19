// Package analytics provides search analytics tracking and aggregation.
package analytics

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TrackerConfig holds configuration for the analytics tracker.
type TrackerConfig struct {
	// BatchSize is the number of events to accumulate before flushing.
	BatchSize int

	// FlushInterval is how often to flush events even if batch size not reached.
	FlushInterval time.Duration

	// MaxTopResults is the number of top result IDs to store per query.
	MaxTopResults int

	// AsyncFlush enables asynchronous event flushing.
	AsyncFlush bool

	// RetryAttempts is the number of retries for failed flushes.
	RetryAttempts int

	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration
}

// DefaultTrackerConfig returns a TrackerConfig with sensible defaults.
func DefaultTrackerConfig() *TrackerConfig {
	return &TrackerConfig{
		BatchSize:     100,
		FlushInterval: 10 * time.Second,
		MaxTopResults: 10,
		AsyncFlush:    true,
		RetryAttempts: 3,
		RetryDelay:    time.Second,
	}
}

// Tracker provides methods for tracking search analytics events.
// It batches events for efficient storage and provides a fluent API.
type Tracker struct {
	store  Store
	config *TrackerConfig
	logger logging.Logger

	// Event buffers for batching
	queryBuffer      []QueryEvent
	clickBuffer      []ClickEvent
	conversionBuffer []ConversionEvent

	// Synchronization
	mu       sync.Mutex
	flushMu  sync.Mutex
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewTracker creates a new analytics tracker.
func NewTracker(store Store, config *TrackerConfig, logger logging.Logger) *Tracker {
	if config == nil {
		config = DefaultTrackerConfig()
	}

	t := &Tracker{
		store:            store,
		config:           config,
		logger:           logger.With(logging.F("component", "analytics_tracker")),
		queryBuffer:      make([]QueryEvent, 0, config.BatchSize),
		clickBuffer:      make([]ClickEvent, 0, config.BatchSize),
		conversionBuffer: make([]ConversionEvent, 0, config.BatchSize),
		stopChan:         make(chan struct{}),
	}

	// Start background flush goroutine
	if config.AsyncFlush {
		t.wg.Add(1)
		go t.flushLoop()
	}

	return t
}

// Query represents a search query for tracking purposes.
type Query struct {
	// Text is the original query text.
	Text string

	// TenantID is required for tenant isolation.
	TenantID string

	// UserID is the user who executed the query (optional).
	UserID string

	// SessionID tracks queries within a session (optional).
	SessionID string

	// SearchMode indicates hybrid, keyword, or semantic search.
	SearchMode string

	// Filters contains applied filter parameters.
	Filters QueryFilters

	// ABTestVariant indicates which A/B test variant was used.
	ABTestVariant string
}

// Result represents a search result for tracking purposes.
type Result struct {
	// ID is the result document ID.
	ID string

	// Score is the relevance score.
	Score float64

	// Position is the rank position (1-indexed).
	Position int
}

// TrackQuery records a search query event with its results and performance.
// Returns the generated query ID for click/conversion tracking.
func (t *Tracker) TrackQuery(ctx context.Context, query Query, results []Result, latency time.Duration) (string, error) {
	if query.TenantID == "" {
		return "", ErrInvalidTenantID
	}

	queryID := uuid.New().String()

	// Extract top result IDs for click correlation
	topResults := make([]string, 0, t.config.MaxTopResults)
	for i, r := range results {
		if i >= t.config.MaxTopResults {
			break
		}
		topResults = append(topResults, r.ID)
	}

	event := QueryEvent{
		ID:            queryID,
		QueryText:     query.Text,
		QueryHash:     HashQuery(query.Text),
		UserID:        query.UserID,
		TenantID:      query.TenantID,
		SessionID:     query.SessionID,
		Timestamp:     time.Now().UTC(),
		ResultCount:   len(results),
		TotalMatches:  int64(len(results)), // This should come from search response
		LatencyMs:     float64(latency.Microseconds()) / 1000.0,
		SearchMode:    query.SearchMode,
		Filters:       query.Filters,
		ABTestVariant: query.ABTestVariant,
		TopResults:    topResults,
	}

	if t.config.AsyncFlush {
		t.bufferQueryEvent(event)
	} else {
		if err := t.store.SaveQueryEvent(ctx, &event); err != nil {
			t.logger.Error("Failed to save query event",
				logging.F("query_id", queryID),
				logging.Err(err),
			)
			return queryID, err
		}
	}

	t.logger.Debug("Tracked query event",
		logging.F("query_id", queryID),
		logging.F("tenant_id", query.TenantID),
		logging.F("result_count", len(results)),
		logging.F("latency_ms", event.LatencyMs),
	)

	return queryID, nil
}

// TrackQueryWithTotalMatches records a query event with explicit total match count.
func (t *Tracker) TrackQueryWithTotalMatches(ctx context.Context, query Query, results []Result, totalMatches int64, latency time.Duration) (string, error) {
	if query.TenantID == "" {
		return "", ErrInvalidTenantID
	}

	queryID := uuid.New().String()

	topResults := make([]string, 0, t.config.MaxTopResults)
	for i, r := range results {
		if i >= t.config.MaxTopResults {
			break
		}
		topResults = append(topResults, r.ID)
	}

	event := QueryEvent{
		ID:            queryID,
		QueryText:     query.Text,
		QueryHash:     HashQuery(query.Text),
		UserID:        query.UserID,
		TenantID:      query.TenantID,
		SessionID:     query.SessionID,
		Timestamp:     time.Now().UTC(),
		ResultCount:   len(results),
		TotalMatches:  totalMatches,
		LatencyMs:     float64(latency.Microseconds()) / 1000.0,
		SearchMode:    query.SearchMode,
		Filters:       query.Filters,
		ABTestVariant: query.ABTestVariant,
		TopResults:    topResults,
	}

	if t.config.AsyncFlush {
		t.bufferQueryEvent(event)
	} else {
		if err := t.store.SaveQueryEvent(ctx, &event); err != nil {
			return queryID, err
		}
	}

	return queryID, nil
}

// TrackClick records a user clicking on a search result.
func (t *Tracker) TrackClick(ctx context.Context, tenantID, queryID, resultID string, position int) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if queryID == "" {
		return ErrInvalidQueryID
	}

	event := ClickEvent{
		ID:        uuid.New().String(),
		QueryID:   queryID,
		ResultID:  resultID,
		Position:  position,
		TenantID:  tenantID,
		Timestamp: time.Now().UTC(),
	}

	if t.config.AsyncFlush {
		t.bufferClickEvent(event)
	} else {
		if err := t.store.SaveClickEvent(ctx, &event); err != nil {
			t.logger.Error("Failed to save click event",
				logging.F("query_id", queryID),
				logging.F("result_id", resultID),
				logging.Err(err),
			)
			return err
		}
	}

	t.logger.Debug("Tracked click event",
		logging.F("query_id", queryID),
		logging.F("result_id", resultID),
		logging.F("position", position),
	)

	return nil
}

// TrackClickWithContext records a click with additional context.
func (t *Tracker) TrackClickWithContext(ctx context.Context, tenantID, queryID, resultID, userID, sessionID string, position int) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if queryID == "" {
		return ErrInvalidQueryID
	}

	event := ClickEvent{
		ID:        uuid.New().String(),
		QueryID:   queryID,
		ResultID:  resultID,
		Position:  position,
		TenantID:  tenantID,
		UserID:    userID,
		SessionID: sessionID,
		Timestamp: time.Now().UTC(),
	}

	if t.config.AsyncFlush {
		t.bufferClickEvent(event)
	} else {
		if err := t.store.SaveClickEvent(ctx, &event); err != nil {
			return err
		}
	}

	return nil
}

// TrackConversion records a user taking a meaningful action on a search result.
func (t *Tracker) TrackConversion(ctx context.Context, tenantID, queryID, resultID, action string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if queryID == "" {
		return ErrInvalidQueryID
	}

	event := ConversionEvent{
		ID:        uuid.New().String(),
		QueryID:   queryID,
		ResultID:  resultID,
		Action:    action,
		TenantID:  tenantID,
		Timestamp: time.Now().UTC(),
	}

	if t.config.AsyncFlush {
		t.bufferConversionEvent(event)
	} else {
		if err := t.store.SaveConversionEvent(ctx, &event); err != nil {
			t.logger.Error("Failed to save conversion event",
				logging.F("query_id", queryID),
				logging.F("result_id", resultID),
				logging.F("action", action),
				logging.Err(err),
			)
			return err
		}
	}

	t.logger.Debug("Tracked conversion event",
		logging.F("query_id", queryID),
		logging.F("result_id", resultID),
		logging.F("action", action),
	)

	return nil
}

// TrackConversionWithMetadata records a conversion with additional metadata.
func (t *Tracker) TrackConversionWithMetadata(ctx context.Context, tenantID, queryID, resultID, action, userID, sessionID string, metadata map[string]string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if queryID == "" {
		return ErrInvalidQueryID
	}

	event := ConversionEvent{
		ID:        uuid.New().String(),
		QueryID:   queryID,
		ResultID:  resultID,
		Action:    action,
		TenantID:  tenantID,
		UserID:    userID,
		SessionID: sessionID,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}

	if t.config.AsyncFlush {
		t.bufferConversionEvent(event)
	} else {
		if err := t.store.SaveConversionEvent(ctx, &event); err != nil {
			return err
		}
	}

	return nil
}

// GetQueryStats retrieves aggregated query statistics.
func (t *Tracker) GetQueryStats(ctx context.Context, filter StatsFilter) (*QueryStats, error) {
	// Flush any pending events first for accurate stats
	if err := t.Flush(ctx); err != nil {
		t.logger.Warn("Failed to flush before getting stats", logging.Err(err))
	}

	return t.store.GetQueryStats(ctx, filter)
}

// bufferQueryEvent adds a query event to the buffer.
func (t *Tracker) bufferQueryEvent(event QueryEvent) {
	t.mu.Lock()
	t.queryBuffer = append(t.queryBuffer, event)
	shouldFlush := len(t.queryBuffer) >= t.config.BatchSize
	t.mu.Unlock()

	if shouldFlush {
		go t.flushQueries(context.Background())
	}
}

// bufferClickEvent adds a click event to the buffer.
func (t *Tracker) bufferClickEvent(event ClickEvent) {
	t.mu.Lock()
	t.clickBuffer = append(t.clickBuffer, event)
	shouldFlush := len(t.clickBuffer) >= t.config.BatchSize
	t.mu.Unlock()

	if shouldFlush {
		go t.flushClicks(context.Background())
	}
}

// bufferConversionEvent adds a conversion event to the buffer.
func (t *Tracker) bufferConversionEvent(event ConversionEvent) {
	t.mu.Lock()
	t.conversionBuffer = append(t.conversionBuffer, event)
	shouldFlush := len(t.conversionBuffer) >= t.config.BatchSize
	t.mu.Unlock()

	if shouldFlush {
		go t.flushConversions(context.Background())
	}
}

// flushLoop periodically flushes buffered events.
func (t *Tracker) flushLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := t.Flush(ctx); err != nil {
				t.logger.Error("Periodic flush failed", logging.Err(err))
			}
			cancel()
		case <-t.stopChan:
			// Final flush before stopping
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := t.Flush(ctx); err != nil {
				t.logger.Error("Final flush failed", logging.Err(err))
			}
			cancel()
			return
		}
	}
}

// Flush saves all buffered events to storage.
func (t *Tracker) Flush(ctx context.Context) error {
	t.flushMu.Lock()
	defer t.flushMu.Unlock()

	var firstErr error

	if err := t.flushQueries(ctx); err != nil && firstErr == nil {
		firstErr = err
	}

	if err := t.flushClicks(ctx); err != nil && firstErr == nil {
		firstErr = err
	}

	if err := t.flushConversions(ctx); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

// flushQueries saves buffered query events.
func (t *Tracker) flushQueries(ctx context.Context) error {
	t.mu.Lock()
	if len(t.queryBuffer) == 0 {
		t.mu.Unlock()
		return nil
	}
	events := t.queryBuffer
	t.queryBuffer = make([]QueryEvent, 0, t.config.BatchSize)
	t.mu.Unlock()

	var lastErr error
	for i := range events {
		if err := t.store.SaveQueryEvent(ctx, &events[i]); err != nil {
			lastErr = err
			t.logger.Error("Failed to save query event",
				logging.F("query_id", events[i].ID),
				logging.Err(err),
			)
		}
	}

	if lastErr != nil {
		t.logger.Warn("Some query events failed to save",
			logging.F("count", len(events)),
			logging.Err(lastErr),
		)
	} else {
		t.logger.Debug("Flushed query events", logging.F("count", len(events)))
	}

	return lastErr
}

// flushClicks saves buffered click events.
func (t *Tracker) flushClicks(ctx context.Context) error {
	t.mu.Lock()
	if len(t.clickBuffer) == 0 {
		t.mu.Unlock()
		return nil
	}
	events := t.clickBuffer
	t.clickBuffer = make([]ClickEvent, 0, t.config.BatchSize)
	t.mu.Unlock()

	var lastErr error
	for i := range events {
		if err := t.store.SaveClickEvent(ctx, &events[i]); err != nil {
			lastErr = err
			t.logger.Error("Failed to save click event",
				logging.F("click_id", events[i].ID),
				logging.Err(err),
			)
		}
	}

	if lastErr != nil {
		t.logger.Warn("Some click events failed to save",
			logging.F("count", len(events)),
		)
	} else {
		t.logger.Debug("Flushed click events", logging.F("count", len(events)))
	}

	return lastErr
}

// flushConversions saves buffered conversion events.
func (t *Tracker) flushConversions(ctx context.Context) error {
	t.mu.Lock()
	if len(t.conversionBuffer) == 0 {
		t.mu.Unlock()
		return nil
	}
	events := t.conversionBuffer
	t.conversionBuffer = make([]ConversionEvent, 0, t.config.BatchSize)
	t.mu.Unlock()

	var lastErr error
	for i := range events {
		if err := t.store.SaveConversionEvent(ctx, &events[i]); err != nil {
			lastErr = err
			t.logger.Error("Failed to save conversion event",
				logging.F("conversion_id", events[i].ID),
				logging.Err(err),
			)
		}
	}

	if lastErr != nil {
		t.logger.Warn("Some conversion events failed to save",
			logging.F("count", len(events)),
		)
	} else {
		t.logger.Debug("Flushed conversion events", logging.F("count", len(events)))
	}

	return lastErr
}

// Close stops the tracker and flushes remaining events.
func (t *Tracker) Close() error {
	close(t.stopChan)
	t.wg.Wait()
	return nil
}

// BufferStats returns current buffer sizes for monitoring.
type BufferStats struct {
	QueryCount      int
	ClickCount      int
	ConversionCount int
}

// GetBufferStats returns the current buffer sizes.
func (t *Tracker) GetBufferStats() BufferStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	return BufferStats{
		QueryCount:      len(t.queryBuffer),
		ClickCount:      len(t.clickBuffer),
		ConversionCount: len(t.conversionBuffer),
	}
}
