package analytics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockStore implements Store for testing.
type mockStore struct {
	mu sync.Mutex

	queryEvents      []QueryEvent
	clickEvents      []ClickEvent
	conversionEvents []ConversionEvent
	dailyRollups     []DailyRollup

	// Error injection
	saveQueryErr      error
	saveClickErr      error
	saveConversionErr error
}

func newMockStore() *mockStore {
	return &mockStore{
		queryEvents:      make([]QueryEvent, 0),
		clickEvents:      make([]ClickEvent, 0),
		conversionEvents: make([]ConversionEvent, 0),
		dailyRollups:     make([]DailyRollup, 0),
	}
}

func (m *mockStore) SaveQueryEvent(ctx context.Context, event *QueryEvent) error {
	if m.saveQueryErr != nil {
		return m.saveQueryErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queryEvents = append(m.queryEvents, *event)
	return nil
}

func (m *mockStore) GetQueryEvent(ctx context.Context, tenantID, queryID string) (*QueryEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.queryEvents {
		if e.ID == queryID && e.TenantID == tenantID {
			return &e, nil
		}
	}
	return nil, ErrEventNotFound
}

func (m *mockStore) ListQueryEvents(ctx context.Context, filter StatsFilter) ([]QueryEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []QueryEvent
	for _, e := range m.queryEvents {
		if e.TenantID == filter.TenantID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockStore) SaveClickEvent(ctx context.Context, event *ClickEvent) error {
	if m.saveClickErr != nil {
		return m.saveClickErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clickEvents = append(m.clickEvents, *event)
	return nil
}

func (m *mockStore) GetClickEvent(ctx context.Context, tenantID, clickID string) (*ClickEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.clickEvents {
		if e.ID == clickID && e.TenantID == tenantID {
			return &e, nil
		}
	}
	return nil, ErrEventNotFound
}

func (m *mockStore) ListClickEvents(ctx context.Context, filter StatsFilter) ([]ClickEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []ClickEvent
	for _, e := range m.clickEvents {
		if e.TenantID == filter.TenantID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockStore) GetClicksForQuery(ctx context.Context, tenantID, queryID string) ([]ClickEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []ClickEvent
	for _, e := range m.clickEvents {
		if e.QueryID == queryID && e.TenantID == tenantID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockStore) SaveConversionEvent(ctx context.Context, event *ConversionEvent) error {
	if m.saveConversionErr != nil {
		return m.saveConversionErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conversionEvents = append(m.conversionEvents, *event)
	return nil
}

func (m *mockStore) GetConversionEvent(ctx context.Context, tenantID, conversionID string) (*ConversionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.conversionEvents {
		if e.ID == conversionID && e.TenantID == tenantID {
			return &e, nil
		}
	}
	return nil, ErrEventNotFound
}

func (m *mockStore) ListConversionEvents(ctx context.Context, filter StatsFilter) ([]ConversionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []ConversionEvent
	for _, e := range m.conversionEvents {
		if e.TenantID == filter.TenantID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockStore) GetConversionsForQuery(ctx context.Context, tenantID, queryID string) ([]ConversionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []ConversionEvent
	for _, e := range m.conversionEvents {
		if e.QueryID == queryID && e.TenantID == tenantID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockStore) GetQueryStats(ctx context.Context, filter StatsFilter) (*QueryStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var totalLatency float64
	var totalResults int64
	var zeroResults int64
	for _, e := range m.queryEvents {
		if e.TenantID == filter.TenantID {
			totalLatency += e.LatencyMs
			totalResults += int64(e.ResultCount)
			if e.ResultCount == 0 {
				zeroResults++
			}
		}
	}

	count := int64(len(m.queryEvents))
	var avgLatency, avgResults, zeroRate float64
	if count > 0 {
		avgLatency = totalLatency / float64(count)
		avgResults = float64(totalResults) / float64(count)
		zeroRate = float64(zeroResults) / float64(count)
	}

	var ctr float64
	if count > 0 {
		ctr = float64(len(m.clickEvents)) / float64(count)
	}

	return &QueryStats{
		TotalQueries:     count,
		AvgLatencyMs:     avgLatency,
		AvgResultCount:   avgResults,
		ZeroResultRate:   zeroRate,
		ZeroResultCount:  zeroResults,
		ClickThroughRate: ctr,
		Period: TimePeriod{
			Start:       filter.DateFrom,
			End:         filter.DateTo,
			Granularity: filter.Granularity,
		},
	}, nil
}

func (m *mockStore) GetTopQueries(ctx context.Context, filter StatsFilter, limit int) ([]QueryFrequency, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	counts := make(map[string]int64)
	for _, e := range m.queryEvents {
		if e.TenantID == filter.TenantID {
			counts[e.QueryHash]++
		}
	}

	var result []QueryFrequency
	for hash, count := range counts {
		result = append(result, QueryFrequency{
			QueryHash: hash,
			Count:     count,
		})
	}

	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

func (m *mockStore) GetZeroResultQueries(ctx context.Context, filter StatsFilter, limit int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []string
	seen := make(map[string]bool)
	for _, e := range m.queryEvents {
		if e.TenantID == filter.TenantID && e.ResultCount == 0 && !seen[e.QueryText] {
			result = append(result, e.QueryText)
			seen[e.QueryText] = true
		}
	}

	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

func (m *mockStore) GetClickThroughRate(ctx context.Context, filter StatsFilter) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queryCount := 0
	for _, e := range m.queryEvents {
		if e.TenantID == filter.TenantID {
			queryCount++
		}
	}

	clickCount := 0
	for _, e := range m.clickEvents {
		if e.TenantID == filter.TenantID {
			clickCount++
		}
	}

	if queryCount == 0 {
		return 0, nil
	}

	return float64(clickCount) / float64(queryCount), nil
}

func (m *mockStore) SaveDailyRollup(ctx context.Context, rollup *DailyRollup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dailyRollups = append(m.dailyRollups, *rollup)
	return nil
}

func (m *mockStore) GetDailyRollup(ctx context.Context, tenantID string, date time.Time) (*DailyRollup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.dailyRollups {
		if r.TenantID == tenantID && r.Date.Equal(date.Truncate(24*time.Hour)) {
			return &r, nil
		}
	}
	return nil, ErrEventNotFound
}

func (m *mockStore) ListDailyRollups(ctx context.Context, filter StatsFilter) ([]DailyRollup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []DailyRollup
	for _, r := range m.dailyRollups {
		if r.TenantID == filter.TenantID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *mockStore) ComputeDailyRollup(ctx context.Context, tenantID string, date time.Time) (*DailyRollup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rollup := &DailyRollup{
		Date:     date.Truncate(24 * time.Hour),
		TenantID: tenantID,
	}

	for _, e := range m.queryEvents {
		if e.TenantID == tenantID {
			rollup.TotalQueries++
			rollup.SumLatencyMs += e.LatencyMs
			rollup.SumResultCount += int64(e.ResultCount)
			if e.ResultCount == 0 {
				rollup.ZeroResultQueries++
			}
		}
	}

	for _, e := range m.clickEvents {
		if e.TenantID == tenantID {
			rollup.TotalClicks++
		}
	}

	for _, e := range m.conversionEvents {
		if e.TenantID == tenantID {
			rollup.TotalConversions++
		}
	}

	return rollup, nil
}

func (m *mockStore) GetABTestResults(ctx context.Context, filter StatsFilter, testName string) (*ABTestResult, error) {
	return &ABTestResult{
		TestName: testName,
		Variants: make(map[string]VariantStats),
		Period: TimePeriod{
			Start:       filter.DateFrom,
			End:         filter.DateTo,
			Granularity: filter.Granularity,
		},
	}, nil
}

func (m *mockStore) DeleteOldEvents(ctx context.Context, tenantID string, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var deleted int64
	var newQueryEvents []QueryEvent
	for _, e := range m.queryEvents {
		if e.TenantID == tenantID && e.Timestamp.Before(before) {
			deleted++
		} else {
			newQueryEvents = append(newQueryEvents, e)
		}
	}
	m.queryEvents = newQueryEvents

	return deleted, nil
}

func (m *mockStore) Close() error {
	return nil
}

// Test helper to create a test logger.
func testLogger() logging.Logger {
	cfg := &logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  false,
	}
	return logging.NewLogger(cfg)
}

func TestTracker_TrackQuery(t *testing.T) {
	store := newMockStore()
	logger := testLogger()

	config := &TrackerConfig{
		BatchSize:     10,
		FlushInterval: time.Hour, // Long interval so we can control flushing
		MaxTopResults: 5,
		AsyncFlush:    false, // Disable async for synchronous testing
	}

	tracker := NewTracker(store, config, logger)
	defer tracker.Close()

	ctx := context.Background()
	query := Query{
		Text:       "test query",
		TenantID:   "tenant-1",
		UserID:     "user-1",
		SessionID:  "session-1",
		SearchMode: "hybrid",
	}

	results := []Result{
		{ID: "doc-1", Score: 0.9, Position: 1},
		{ID: "doc-2", Score: 0.8, Position: 2},
		{ID: "doc-3", Score: 0.7, Position: 3},
	}

	queryID, err := tracker.TrackQuery(ctx, query, results, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("TrackQuery failed: %v", err)
	}

	if queryID == "" {
		t.Error("Expected non-empty query ID")
	}

	// Check the event was stored
	if len(store.queryEvents) != 1 {
		t.Errorf("Expected 1 query event, got %d", len(store.queryEvents))
	}

	event := store.queryEvents[0]
	if event.QueryText != "test query" {
		t.Errorf("Expected query text 'test query', got '%s'", event.QueryText)
	}
	if event.TenantID != "tenant-1" {
		t.Errorf("Expected tenant ID 'tenant-1', got '%s'", event.TenantID)
	}
	if event.ResultCount != 3 {
		t.Errorf("Expected result count 3, got %d", event.ResultCount)
	}
	if len(event.TopResults) != 3 {
		t.Errorf("Expected 3 top results, got %d", len(event.TopResults))
	}
}

func TestTracker_TrackQuery_MissingTenantID(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	config := DefaultTrackerConfig()
	config.AsyncFlush = false

	tracker := NewTracker(store, config, logger)
	defer tracker.Close()

	ctx := context.Background()
	query := Query{
		Text: "test query",
		// TenantID is missing
	}

	_, err := tracker.TrackQuery(ctx, query, nil, 100*time.Millisecond)
	if err != ErrInvalidTenantID {
		t.Errorf("Expected ErrInvalidTenantID, got %v", err)
	}
}

func TestTracker_TrackClick(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	config := DefaultTrackerConfig()
	config.AsyncFlush = false

	tracker := NewTracker(store, config, logger)
	defer tracker.Close()

	ctx := context.Background()

	err := tracker.TrackClick(ctx, "tenant-1", "query-1", "result-1", 1)
	if err != nil {
		t.Fatalf("TrackClick failed: %v", err)
	}

	if len(store.clickEvents) != 1 {
		t.Errorf("Expected 1 click event, got %d", len(store.clickEvents))
	}

	event := store.clickEvents[0]
	if event.QueryID != "query-1" {
		t.Errorf("Expected query ID 'query-1', got '%s'", event.QueryID)
	}
	if event.ResultID != "result-1" {
		t.Errorf("Expected result ID 'result-1', got '%s'", event.ResultID)
	}
	if event.Position != 1 {
		t.Errorf("Expected position 1, got %d", event.Position)
	}
}

func TestTracker_TrackClick_MissingQueryID(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	config := DefaultTrackerConfig()
	config.AsyncFlush = false

	tracker := NewTracker(store, config, logger)
	defer tracker.Close()

	ctx := context.Background()

	err := tracker.TrackClick(ctx, "tenant-1", "", "result-1", 1)
	if err != ErrInvalidQueryID {
		t.Errorf("Expected ErrInvalidQueryID, got %v", err)
	}
}

func TestTracker_TrackConversion(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	config := DefaultTrackerConfig()
	config.AsyncFlush = false

	tracker := NewTracker(store, config, logger)
	defer tracker.Close()

	ctx := context.Background()

	err := tracker.TrackConversion(ctx, "tenant-1", "query-1", "result-1", "download")
	if err != nil {
		t.Fatalf("TrackConversion failed: %v", err)
	}

	if len(store.conversionEvents) != 1 {
		t.Errorf("Expected 1 conversion event, got %d", len(store.conversionEvents))
	}

	event := store.conversionEvents[0]
	if event.Action != "download" {
		t.Errorf("Expected action 'download', got '%s'", event.Action)
	}
}

func TestTracker_Flush(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	config := &TrackerConfig{
		BatchSize:     100, // Large batch size so we flush manually
		FlushInterval: time.Hour,
		MaxTopResults: 5,
		AsyncFlush:    true,
	}

	tracker := NewTracker(store, config, logger)
	defer tracker.Close()

	ctx := context.Background()

	// Add some events
	for i := 0; i < 5; i++ {
		query := Query{
			Text:       "test query",
			TenantID:   "tenant-1",
			SearchMode: "hybrid",
		}
		tracker.TrackQuery(ctx, query, nil, 100*time.Millisecond)
	}

	// Events should be buffered
	stats := tracker.GetBufferStats()
	if stats.QueryCount != 5 {
		t.Errorf("Expected 5 buffered queries, got %d", stats.QueryCount)
	}

	// Flush
	err := tracker.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Buffer should be empty
	stats = tracker.GetBufferStats()
	if stats.QueryCount != 0 {
		t.Errorf("Expected 0 buffered queries after flush, got %d", stats.QueryCount)
	}

	// Store should have the events
	if len(store.queryEvents) != 5 {
		t.Errorf("Expected 5 stored query events, got %d", len(store.queryEvents))
	}
}

func TestTracker_GetQueryStats(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	config := DefaultTrackerConfig()
	config.AsyncFlush = false

	tracker := NewTracker(store, config, logger)
	defer tracker.Close()

	ctx := context.Background()

	// Add some query events
	for i := 0; i < 10; i++ {
		query := Query{
			Text:       "test query",
			TenantID:   "tenant-1",
			SearchMode: "hybrid",
		}
		results := []Result{{ID: "doc-1", Score: 0.9, Position: 1}}
		tracker.TrackQuery(ctx, query, results, time.Duration(i*10)*time.Millisecond)
	}

	// Add some clicks
	for i := 0; i < 3; i++ {
		tracker.TrackClick(ctx, "tenant-1", "query-"+string(rune('0'+i)), "result-1", 1)
	}

	filter := DefaultStatsFilter("tenant-1")
	stats, err := tracker.GetQueryStats(ctx, filter)
	if err != nil {
		t.Fatalf("GetQueryStats failed: %v", err)
	}

	if stats.TotalQueries != 10 {
		t.Errorf("Expected 10 total queries, got %d", stats.TotalQueries)
	}

	if stats.ClickThroughRate == 0 {
		t.Error("Expected non-zero CTR")
	}
}

func TestHashQuery(t *testing.T) {
	tests := []struct {
		name   string
		query1 string
		query2 string
		same   bool
	}{
		{
			name:   "same query",
			query1: "hello world",
			query2: "hello world",
			same:   true,
		},
		{
			name:   "case insensitive",
			query1: "Hello World",
			query2: "hello world",
			same:   true,
		},
		{
			name:   "whitespace normalized",
			query1: "  hello   world  ",
			query2: "hello world",
			same:   true,
		},
		{
			name:   "different queries",
			query1: "hello world",
			query2: "goodbye world",
			same:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := HashQuery(tt.query1)
			hash2 := HashQuery(tt.query2)

			if tt.same && hash1 != hash2 {
				t.Errorf("Expected same hash, got %s and %s", hash1, hash2)
			}
			if !tt.same && hash1 == hash2 {
				t.Errorf("Expected different hashes, got same: %s", hash1)
			}
		})
	}
}

func TestDefaultStatsFilter(t *testing.T) {
	filter := DefaultStatsFilter("tenant-1")

	if filter.TenantID != "tenant-1" {
		t.Errorf("Expected tenant ID 'tenant-1', got '%s'", filter.TenantID)
	}

	if filter.Limit != 100 {
		t.Errorf("Expected limit 100, got %d", filter.Limit)
	}

	if filter.Granularity != GranularityDay {
		t.Errorf("Expected granularity 'day', got '%s'", filter.Granularity)
	}

	// Should be 30 days range
	expectedDuration := 30 * 24 * time.Hour
	actualDuration := filter.DateTo.Sub(filter.DateFrom)
	if actualDuration < expectedDuration-time.Hour || actualDuration > expectedDuration+time.Hour {
		t.Errorf("Expected ~30 day range, got %v", actualDuration)
	}
}
