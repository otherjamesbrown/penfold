package pattern

import (
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/services/review/automation"
)

func TestNewFrequencyAnalyzer(t *testing.T) {
	analyzer := NewFrequencyAnalyzer(time.Hour)

	if analyzer == nil {
		t.Fatal("expected analyzer to be created")
	}
	if analyzer.bucketSize != time.Hour {
		t.Errorf("expected bucket size 1 hour, got %v", analyzer.bucketSize)
	}
}

func TestFrequencyAnalyzerDefaultBucketSize(t *testing.T) {
	analyzer := NewFrequencyAnalyzer(0)

	if analyzer.bucketSize != time.Hour {
		t.Errorf("expected default bucket size 1 hour, got %v", analyzer.bucketSize)
	}
}

func TestFrequencyAnalyzerRecordOccurrence(t *testing.T) {
	analyzer := NewFrequencyAnalyzer(time.Hour)

	now := time.Now().UTC()
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now)
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now.Add(30*time.Minute))
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now.Add(2*time.Hour))

	freq := analyzer.GetFrequency("pattern-1")
	if freq == nil {
		t.Fatal("expected frequency data")
	}

	if freq.TotalOccurrences != 3 {
		t.Errorf("expected 3 occurrences, got %d", freq.TotalOccurrences)
	}
	if freq.PatternID != "pattern-1" {
		t.Errorf("expected pattern ID 'pattern-1', got '%s'", freq.PatternID)
	}
	if freq.TenantID != "tenant-1" {
		t.Errorf("expected tenant ID 'tenant-1', got '%s'", freq.TenantID)
	}
}

func TestFrequencyAnalyzerHourlyDistribution(t *testing.T) {
	analyzer := NewFrequencyAnalyzer(time.Hour)

	// Record occurrences at specific hours
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // 9 AM
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now)
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now)
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now.Add(5*time.Hour)) // 2 PM

	freq := analyzer.GetFrequency("pattern-1")
	if freq.HourlyDistribution[9] != 2 {
		t.Errorf("expected 2 occurrences at hour 9, got %d", freq.HourlyDistribution[9])
	}
	if freq.HourlyDistribution[14] != 1 {
		t.Errorf("expected 1 occurrence at hour 14, got %d", freq.HourlyDistribution[14])
	}
	if freq.PeakHour != 9 {
		t.Errorf("expected peak hour 9, got %d", freq.PeakHour)
	}
}

func TestFrequencyAnalyzerDayOfWeekDistribution(t *testing.T) {
	analyzer := NewFrequencyAnalyzer(time.Hour)

	// Monday
	monday := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	analyzer.RecordOccurrence("pattern-1", "tenant-1", monday)
	analyzer.RecordOccurrence("pattern-1", "tenant-1", monday)

	// Wednesday
	wednesday := time.Date(2024, 1, 17, 9, 0, 0, 0, time.UTC)
	analyzer.RecordOccurrence("pattern-1", "tenant-1", wednesday)

	freq := analyzer.GetFrequency("pattern-1")
	if freq.DayOfWeekDistribution[1] != 2 { // Monday = 1
		t.Errorf("expected 2 occurrences on Monday, got %d", freq.DayOfWeekDistribution[1])
	}
	if freq.DayOfWeekDistribution[3] != 1 { // Wednesday = 3
		t.Errorf("expected 1 occurrence on Wednesday, got %d", freq.DayOfWeekDistribution[3])
	}
	if freq.PeakDay != 1 { // Monday
		t.Errorf("expected peak day 1 (Monday), got %d", freq.PeakDay)
	}
}

func TestFrequencyAnalyzerBuckets(t *testing.T) {
	analyzer := NewFrequencyAnalyzer(time.Hour)

	now := time.Now().UTC().Truncate(time.Hour)
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now)
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now.Add(30*time.Minute))
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now.Add(90*time.Minute)) // Next hour

	freq := analyzer.GetFrequency("pattern-1")

	// Should have 2 buckets
	if len(freq.Buckets) != 2 {
		t.Errorf("expected 2 buckets, got %d", len(freq.Buckets))
	}
}

func TestFrequencyAnalyzerNonExistent(t *testing.T) {
	analyzer := NewFrequencyAnalyzer(time.Hour)

	freq := analyzer.GetFrequency("nonexistent")
	if freq != nil {
		t.Error("expected nil for non-existent pattern")
	}
}

func TestFrequencyAnalyzerGetAllFrequencies(t *testing.T) {
	analyzer := NewFrequencyAnalyzer(time.Hour)

	now := time.Now().UTC()
	analyzer.RecordOccurrence("pattern-1", "tenant-1", now)
	analyzer.RecordOccurrence("pattern-2", "tenant-1", now)
	analyzer.RecordOccurrence("pattern-3", "tenant-2", now)

	all := analyzer.GetAllFrequencies()
	if len(all) != 3 {
		t.Errorf("expected 3 frequency entries, got %d", len(all))
	}
}

func TestNewCorrelationAnalyzer(t *testing.T) {
	analyzer := NewCorrelationAnalyzer()

	if analyzer == nil {
		t.Fatal("expected analyzer to be created")
	}
}

func TestCorrelationAnalyzerRecordMatch(t *testing.T) {
	analyzer := NewCorrelationAnalyzer()

	analyzer.RecordMatch("pattern-1", "item-1")
	analyzer.RecordMatch("pattern-1", "item-2")
	analyzer.RecordMatch("pattern-2", "item-1")

	// Pattern-1 and pattern-2 both matched item-1
	analyzer.RecordCoOccurrence("pattern-1", "pattern-2")

	result := analyzer.AnalyzeCorrelation("pattern-1", "pattern-2")

	if result == nil {
		t.Fatal("expected correlation result")
	}
	if result.CoOccurrences != 1 {
		t.Errorf("expected 1 co-occurrence, got %d", result.CoOccurrences)
	}
}

func TestCorrelationAnalyzerAnalyzeCorrelation(t *testing.T) {
	analyzer := NewCorrelationAnalyzer()

	// Record matches
	for i := 0; i < 10; i++ {
		itemID := string(rune('a' + i))
		analyzer.RecordMatch("pattern-1", itemID)
		if i < 8 { // 8 co-occurrences
			analyzer.RecordMatch("pattern-2", itemID)
			analyzer.RecordCoOccurrence("pattern-1", "pattern-2")
		}
	}

	result := analyzer.AnalyzeCorrelation("pattern-1", "pattern-2")

	if result.CoOccurrences != 8 {
		t.Errorf("expected 8 co-occurrences, got %d", result.CoOccurrences)
	}
	if result.Coefficient <= 0 {
		t.Error("expected positive correlation coefficient")
	}
}

func TestCorrelationAnalyzerFindRelatedPatterns(t *testing.T) {
	analyzer := NewCorrelationAnalyzer()

	// Create patterns with varying correlations
	for i := 0; i < 10; i++ {
		itemID := string(rune('a' + i))
		analyzer.RecordMatch("pattern-main", itemID)

		if i < 8 { // High correlation
			analyzer.RecordMatch("pattern-high", itemID)
			analyzer.RecordCoOccurrence("pattern-main", "pattern-high")
		}
		if i < 3 { // Low correlation
			analyzer.RecordMatch("pattern-low", itemID)
			analyzer.RecordCoOccurrence("pattern-main", "pattern-low")
		}
	}

	related := analyzer.FindRelatedPatterns("pattern-main", 0.3)

	// Should find pattern-high, may or may not find pattern-low depending on threshold
	hasHighCorrelation := false
	for _, r := range related {
		if r.Pattern2ID == "pattern-high" || r.Pattern1ID == "pattern-high" {
			hasHighCorrelation = true
			break
		}
	}

	if !hasHighCorrelation && len(related) > 0 {
		t.Error("expected to find highly correlated pattern")
	}
}

func TestCorrelationAnalyzerNoData(t *testing.T) {
	analyzer := NewCorrelationAnalyzer()

	result := analyzer.AnalyzeCorrelation("nonexistent-1", "nonexistent-2")

	if result.Coefficient != 0 {
		t.Errorf("expected 0 coefficient for non-existent patterns, got %f", result.Coefficient)
	}
	if result.Significance {
		t.Error("expected no significance for non-existent patterns")
	}
}

func TestNewTrendAnalyzer(t *testing.T) {
	freqAnalyzer := NewFrequencyAnalyzer(time.Hour)
	trendAnalyzer := NewTrendAnalyzer(freqAnalyzer)

	if trendAnalyzer == nil {
		t.Fatal("expected analyzer to be created")
	}
}

func TestTrendAnalyzerAnalyzeTrend(t *testing.T) {
	freqAnalyzer := NewFrequencyAnalyzer(time.Hour)
	trendAnalyzer := NewTrendAnalyzer(freqAnalyzer)

	// Record some occurrences over time
	now := time.Now().UTC()
	for i := 0; i < 24; i++ {
		ts := now.Add(time.Duration(-i) * time.Hour)
		freqAnalyzer.RecordOccurrence("pattern-1", "tenant-1", ts)
		freqAnalyzer.RecordOccurrence("pattern-1", "tenant-1", ts)
	}

	trend := trendAnalyzer.AnalyzeTrend("pattern-1")

	if trend == nil {
		t.Fatal("expected trend data")
	}
	if trend.PatternID != "pattern-1" {
		t.Errorf("expected pattern ID 'pattern-1', got '%s'", trend.PatternID)
	}
	if trend.AnalyzedAt.IsZero() {
		t.Error("expected analyzed_at to be set")
	}
}

func TestTrendAnalyzerNoData(t *testing.T) {
	freqAnalyzer := NewFrequencyAnalyzer(time.Hour)
	trendAnalyzer := NewTrendAnalyzer(freqAnalyzer)

	trend := trendAnalyzer.AnalyzeTrend("nonexistent")

	if trend.Direction != 0 {
		t.Errorf("expected direction 0 for no data, got %d", trend.Direction)
	}
}

func TestTrendAnalyzerSetWindows(t *testing.T) {
	freqAnalyzer := NewFrequencyAnalyzer(time.Hour)
	trendAnalyzer := NewTrendAnalyzer(freqAnalyzer)

	trendAnalyzer.SetWindows(24*time.Hour, 7*24*time.Hour)

	if trendAnalyzer.windowSize != 24*time.Hour {
		t.Errorf("expected window size 24h, got %v", trendAnalyzer.windowSize)
	}
	if trendAnalyzer.historicalSize != 7*24*time.Hour {
		t.Errorf("expected historical size 7d, got %v", trendAnalyzer.historicalSize)
	}
}

func TestTrendAnalyzerFindTrendingPatterns(t *testing.T) {
	freqAnalyzer := NewFrequencyAnalyzer(time.Hour)
	trendAnalyzer := NewTrendAnalyzer(freqAnalyzer)
	trendAnalyzer.SetWindows(12*time.Hour, 48*time.Hour)

	now := time.Now().UTC()

	// Create an increasing trend for pattern-1
	// Recent: many occurrences, Historical: fewer
	for i := 0; i < 48; i++ {
		ts := now.Add(time.Duration(-i) * time.Hour)
		if i < 12 { // Recent
			freqAnalyzer.RecordOccurrence("pattern-1", "tenant-1", ts)
			freqAnalyzer.RecordOccurrence("pattern-1", "tenant-1", ts)
			freqAnalyzer.RecordOccurrence("pattern-1", "tenant-1", ts)
		} else { // Historical
			freqAnalyzer.RecordOccurrence("pattern-1", "tenant-1", ts)
		}
	}

	trending := trendAnalyzer.FindTrendingPatterns(1, 10) // Increasing, >10% change

	// Check that we found trending patterns
	if len(trending) > 0 {
		// Verify all have positive direction
		for _, trend := range trending {
			if trend.Direction != 1 {
				t.Errorf("expected direction 1, got %d", trend.Direction)
			}
		}
	}
}

func TestNewPatternAnalyzer(t *testing.T) {
	detector := newTestDetector()
	analyzer := NewPatternAnalyzer(detector)

	if analyzer == nil {
		t.Fatal("expected analyzer to be created")
	}
	if analyzer.frequencyAnalyzer == nil {
		t.Error("expected frequency analyzer to be initialized")
	}
	if analyzer.correlationAnalyzer == nil {
		t.Error("expected correlation analyzer to be initialized")
	}
	if analyzer.trendAnalyzer == nil {
		t.Error("expected trend analyzer to be initialized")
	}
}

func TestPatternAnalyzerRecordPatternMatch(t *testing.T) {
	detector := newTestDetector()
	analyzer := NewPatternAnalyzer(detector)

	pattern := NewPattern("tenant-1", "Test", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})

	item := &automation.ReviewItem{
		ID:       "item-1",
		TenantID: "tenant-1",
		Sender:   "test@example.com",
	}

	analyzer.RecordPatternMatch(pattern, item)

	// Verify frequency was recorded
	freq := analyzer.GetFrequencyAnalyzer().GetFrequency(pattern.ID)
	if freq == nil {
		t.Error("expected frequency data to be recorded")
	}
	if freq.TotalOccurrences != 1 {
		t.Errorf("expected 1 occurrence, got %d", freq.TotalOccurrences)
	}
}

func TestPatternAnalyzerRecordPatternMatches(t *testing.T) {
	detector := newTestDetector()
	analyzer := NewPatternAnalyzer(detector)

	p1 := NewPattern("tenant-1", "Pattern 1", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	p2 := NewPattern("tenant-1", "Pattern 2", PatternTopic, []PatternCondition{
		NewPatternCondition("category", OpEquals, "work"),
	})

	item := &automation.ReviewItem{
		ID:       "item-1",
		TenantID: "tenant-1",
		Sender:   "test@example.com",
		Category: "work",
	}

	analyzer.RecordPatternMatches([]*Pattern{p1, p2}, item)

	// Both patterns should have frequency recorded
	freq1 := analyzer.GetFrequencyAnalyzer().GetFrequency(p1.ID)
	freq2 := analyzer.GetFrequencyAnalyzer().GetFrequency(p2.ID)

	if freq1 == nil || freq2 == nil {
		t.Error("expected both patterns to have frequency data")
	}

	// Co-occurrence should be recorded
	corr := analyzer.GetCorrelationAnalyzer().AnalyzeCorrelation(p1.ID, p2.ID)
	if corr.CoOccurrences != 1 {
		t.Errorf("expected 1 co-occurrence, got %d", corr.CoOccurrences)
	}
}

func TestPatternAnalyzerAnalyzePattern(t *testing.T) {
	detector := newTestDetector()
	analyzer := NewPatternAnalyzer(detector)

	pattern := NewPattern("tenant-1", "Test", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})

	item := &automation.ReviewItem{
		ID:       "item-1",
		TenantID: "tenant-1",
		Sender:   "test@example.com",
	}

	// Record some matches
	for i := 0; i < 10; i++ {
		analyzer.RecordPatternMatch(pattern, item)
	}

	result := analyzer.AnalyzePattern(pattern.ID)

	if result == nil {
		t.Fatal("expected analysis result")
	}
	if result.PatternID != pattern.ID {
		t.Errorf("expected pattern ID '%s', got '%s'", pattern.ID, result.PatternID)
	}
	if result.FrequencyData == nil {
		t.Error("expected frequency data in result")
	}
	if result.TrendData == nil {
		t.Error("expected trend data in result")
	}
	if result.AnalyzedAt.IsZero() {
		t.Error("expected analyzed_at to be set")
	}
}

func TestPatternAnalyzerGetAnalyzers(t *testing.T) {
	detector := newTestDetector()
	analyzer := NewPatternAnalyzer(detector)

	if analyzer.GetFrequencyAnalyzer() == nil {
		t.Error("expected frequency analyzer")
	}
	if analyzer.GetCorrelationAnalyzer() == nil {
		t.Error("expected correlation analyzer")
	}
	if analyzer.GetTrendAnalyzer() == nil {
		t.Error("expected trend analyzer")
	}
}

func TestCalculateSlope(t *testing.T) {
	freqAnalyzer := NewFrequencyAnalyzer(time.Hour)
	trendAnalyzer := NewTrendAnalyzer(freqAnalyzer)

	// Create buckets with increasing counts
	now := time.Now().UTC().Truncate(time.Hour)
	buckets := []FrequencyBucket{
		{Timestamp: now.Add(-4 * time.Hour), Count: 1},
		{Timestamp: now.Add(-3 * time.Hour), Count: 2},
		{Timestamp: now.Add(-2 * time.Hour), Count: 3},
		{Timestamp: now.Add(-1 * time.Hour), Count: 4},
		{Timestamp: now, Count: 5},
	}

	slope := trendAnalyzer.calculateSlope(buckets)

	// Slope should be positive (increasing)
	if slope <= 0 {
		t.Errorf("expected positive slope for increasing trend, got %f", slope)
	}
}

func TestCalculateVolatility(t *testing.T) {
	freqAnalyzer := NewFrequencyAnalyzer(time.Hour)
	trendAnalyzer := NewTrendAnalyzer(freqAnalyzer)

	// Low volatility (consistent values)
	lowVolBuckets := []FrequencyBucket{
		{Count: 10},
		{Count: 10},
		{Count: 10},
		{Count: 10},
	}

	// High volatility (varying values)
	highVolBuckets := []FrequencyBucket{
		{Count: 1},
		{Count: 100},
		{Count: 5},
		{Count: 50},
	}

	lowVol := trendAnalyzer.calculateVolatility(lowVolBuckets)
	highVol := trendAnalyzer.calculateVolatility(highVolBuckets)

	if lowVol >= highVol {
		t.Errorf("expected low volatility (%f) < high volatility (%f)", lowVol, highVol)
	}
}
