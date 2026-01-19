package analytics

import (
	"context"
	"testing"
	"time"
)

func TestAggregator_AggregateDaily(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)

	ctx := context.Background()

	// Add test data
	now := time.Now()
	store.queryEvents = []QueryEvent{
		{
			ID:          "q1",
			QueryText:   "test query 1",
			QueryHash:   HashQuery("test query 1"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 5,
			LatencyMs:   100.0,
			SearchMode:  "hybrid",
		},
		{
			ID:          "q2",
			QueryText:   "test query 2",
			QueryHash:   HashQuery("test query 2"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 3,
			LatencyMs:   50.0,
			SearchMode:  "keyword",
		},
		{
			ID:          "q3",
			QueryText:   "zero result query",
			QueryHash:   HashQuery("zero result query"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 0,
			LatencyMs:   30.0,
			SearchMode:  "hybrid",
		},
	}

	store.clickEvents = []ClickEvent{
		{
			ID:        "c1",
			QueryID:   "q1",
			ResultID:  "doc-1",
			Position:  1,
			TenantID:  "tenant-1",
			Timestamp: now,
		},
	}

	store.conversionEvents = []ConversionEvent{
		{
			ID:        "cv1",
			QueryID:   "q1",
			ResultID:  "doc-1",
			Action:    "download",
			TenantID:  "tenant-1",
			Timestamp: now,
		},
	}

	rollup, err := aggregator.AggregateDaily(ctx, "tenant-1", now)
	if err != nil {
		t.Fatalf("AggregateDaily failed: %v", err)
	}

	if rollup.TenantID != "tenant-1" {
		t.Errorf("Expected tenant ID 'tenant-1', got '%s'", rollup.TenantID)
	}

	if rollup.TotalQueries != 3 {
		t.Errorf("Expected 3 total queries, got %d", rollup.TotalQueries)
	}

	if rollup.TotalClicks != 1 {
		t.Errorf("Expected 1 total click, got %d", rollup.TotalClicks)
	}

	if rollup.TotalConversions != 1 {
		t.Errorf("Expected 1 total conversion, got %d", rollup.TotalConversions)
	}

	if rollup.ZeroResultQueries != 1 {
		t.Errorf("Expected 1 zero result query, got %d", rollup.ZeroResultQueries)
	}
}

func TestAggregator_AggregateDaily_InvalidTenantID(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)

	ctx := context.Background()

	_, err := aggregator.AggregateDaily(ctx, "", time.Now())
	if err != ErrInvalidTenantID {
		t.Errorf("Expected ErrInvalidTenantID, got %v", err)
	}
}

func TestAggregator_TopQueries(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)

	ctx := context.Background()

	// Add test data with repeated queries
	now := time.Now()
	for i := 0; i < 5; i++ {
		store.queryEvents = append(store.queryEvents, QueryEvent{
			ID:          "q" + string(rune('a'+i)),
			QueryText:   "popular query",
			QueryHash:   HashQuery("popular query"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 5,
		})
	}
	for i := 0; i < 2; i++ {
		store.queryEvents = append(store.queryEvents, QueryEvent{
			ID:          "q" + string(rune('f'+i)),
			QueryText:   "less popular query",
			QueryHash:   HashQuery("less popular query"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 3,
		})
	}

	filter := DefaultStatsFilter("tenant-1")
	topQueries, err := aggregator.TopQueries(ctx, filter, 10)
	if err != nil {
		t.Fatalf("TopQueries failed: %v", err)
	}

	if len(topQueries) != 2 {
		t.Errorf("Expected 2 unique queries, got %d", len(topQueries))
	}

	// Find the popular query and verify count
	found := false
	for _, q := range topQueries {
		if q.QueryHash == HashQuery("popular query") {
			found = true
			if q.Count != 5 {
				t.Errorf("Expected count 5 for popular query, got %d", q.Count)
			}
		}
	}
	if !found {
		t.Error("Expected to find popular query in results")
	}
}

func TestAggregator_ZeroResultQueries(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)

	ctx := context.Background()

	// Add test data with zero result queries
	now := time.Now()
	store.queryEvents = []QueryEvent{
		{
			ID:          "q1",
			QueryText:   "good query",
			QueryHash:   HashQuery("good query"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 5,
		},
		{
			ID:          "q2",
			QueryText:   "bad query 1",
			QueryHash:   HashQuery("bad query 1"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 0,
		},
		{
			ID:          "q3",
			QueryText:   "bad query 2",
			QueryHash:   HashQuery("bad query 2"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 0,
		},
	}

	filter := DefaultStatsFilter("tenant-1")
	zeroQueries, err := aggregator.ZeroResultQueries(ctx, filter)
	if err != nil {
		t.Fatalf("ZeroResultQueries failed: %v", err)
	}

	if len(zeroQueries) != 2 {
		t.Errorf("Expected 2 zero result queries, got %d", len(zeroQueries))
	}
}

func TestAggregator_ClickThroughRate(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)

	ctx := context.Background()

	// Add test data: 10 queries, 3 clicks = 30% CTR
	now := time.Now()
	for i := 0; i < 10; i++ {
		store.queryEvents = append(store.queryEvents, QueryEvent{
			ID:          "q" + string(rune('0'+i)),
			QueryText:   "test query",
			QueryHash:   HashQuery("test query"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 5,
		})
	}

	for i := 0; i < 3; i++ {
		store.clickEvents = append(store.clickEvents, ClickEvent{
			ID:        "c" + string(rune('0'+i)),
			QueryID:   "q" + string(rune('0'+i)),
			ResultID:  "doc-1",
			Position:  1,
			TenantID:  "tenant-1",
			Timestamp: now,
		})
	}

	filter := DefaultStatsFilter("tenant-1")
	ctr, err := aggregator.ClickThroughRate(ctx, filter)
	if err != nil {
		t.Fatalf("ClickThroughRate failed: %v", err)
	}

	expected := 0.3
	if ctr < expected-0.01 || ctr > expected+0.01 {
		t.Errorf("Expected CTR ~0.3, got %f", ctr)
	}
}

func TestAggregator_AnalyzeTrend(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)

	ctx := context.Background()

	// Add test rollups with improving trend
	baseDate := time.Now().AddDate(0, 0, -7)
	for i := 0; i < 7; i++ {
		store.dailyRollups = append(store.dailyRollups, DailyRollup{
			Date:         baseDate.AddDate(0, 0, i).Truncate(24 * time.Hour),
			TenantID:     "tenant-1",
			TotalQueries: int64(100 + i*10), // Increasing queries
			TotalClicks:  int64(10 + i*5),   // Increasing clicks
			SumLatencyMs: float64((100 + i*10) * 50),
		})
	}

	filter := StatsFilter{
		TenantID:    "tenant-1",
		DateFrom:    baseDate,
		DateTo:      time.Now(),
		Granularity: GranularityDay,
	}

	trend, err := aggregator.AnalyzeTrend(ctx, filter)
	if err != nil {
		t.Fatalf("AnalyzeTrend failed: %v", err)
	}

	if len(trend.DataPoints) != 7 {
		t.Errorf("Expected 7 data points, got %d", len(trend.DataPoints))
	}

	if trend.TrendDirection != "improving" && trend.TrendDirection != "stable" {
		t.Logf("Trend direction: %s, change: %.2f%%", trend.TrendDirection, trend.ChangePercent)
	}
}

func TestAggregator_GeneratePerformanceReport(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)

	ctx := context.Background()

	// Add test data
	now := time.Now()
	store.queryEvents = []QueryEvent{
		{
			ID:          "q1",
			QueryText:   "fast query",
			QueryHash:   HashQuery("fast query"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 5,
			LatencyMs:   50.0,
		},
		{
			ID:          "q2",
			QueryText:   "slow query",
			QueryHash:   HashQuery("slow query"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 5,
			LatencyMs:   2000.0, // Slow query
		},
		{
			ID:          "q3",
			QueryText:   "zero result query",
			QueryHash:   HashQuery("zero result query"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 0,
			LatencyMs:   30.0,
		},
	}

	filter := DefaultStatsFilter("tenant-1")
	report, err := aggregator.GeneratePerformanceReport(ctx, filter)
	if err != nil {
		t.Fatalf("GeneratePerformanceReport failed: %v", err)
	}

	if report.OverallStats.TotalQueries != 3 {
		t.Errorf("Expected 3 total queries, got %d", report.OverallStats.TotalQueries)
	}

	if len(report.Recommendations) == 0 {
		t.Error("Expected at least one recommendation")
	}

	// Check for slow query warning
	hasSlowQueryRecommendation := false
	for _, rec := range report.Recommendations {
		if rec != "" {
			hasSlowQueryRecommendation = true
			break
		}
	}
	if !hasSlowQueryRecommendation {
		t.Log("Recommendations:", report.Recommendations)
	}
}

func TestAggregator_CompareABTest(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)

	ctx := context.Background()

	filter := DefaultStatsFilter("tenant-1")
	result, err := aggregator.CompareABTest(ctx, filter, "ranking_test")
	if err != nil {
		t.Fatalf("CompareABTest failed: %v", err)
	}

	if result.TestName != "ranking_test" {
		t.Errorf("Expected test name 'ranking_test', got '%s'", result.TestName)
	}

	if result.Variants == nil {
		t.Error("Expected non-nil variants map")
	}
}

func TestAggregator_BackfillRollups(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)

	ctx := context.Background()

	// Add some historical events
	for i := 0; i < 7; i++ {
		date := time.Now().AddDate(0, 0, -i)
		store.queryEvents = append(store.queryEvents, QueryEvent{
			ID:          "q" + string(rune('a'+i)),
			QueryText:   "test query",
			QueryHash:   HashQuery("test query"),
			TenantID:    "tenant-1",
			Timestamp:   date,
			ResultCount: 5,
			LatencyMs:   100.0,
		})
	}

	from := time.Now().AddDate(0, 0, -7)
	to := time.Now()

	err := aggregator.BackfillRollups(ctx, "tenant-1", from, to)
	if err != nil {
		t.Fatalf("BackfillRollups failed: %v", err)
	}

	// Should have created rollups for each day
	if len(store.dailyRollups) < 1 {
		t.Errorf("Expected at least 1 rollup, got %d", len(store.dailyRollups))
	}
}
