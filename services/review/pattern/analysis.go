// Package pattern provides pattern analysis capabilities for the review service.
package pattern

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/services/review/automation"
)

// FrequencyBucket represents a time-based frequency measurement.
type FrequencyBucket struct {
	// Timestamp is the start of this bucket.
	Timestamp time.Time `json:"timestamp"`

	// Count is the number of occurrences in this bucket.
	Count int `json:"count"`

	// Duration is the bucket duration.
	Duration time.Duration `json:"duration"`
}

// FrequencyData holds frequency analysis results.
type FrequencyData struct {
	// PatternID is the pattern being analyzed.
	PatternID string `json:"pattern_id"`

	// TenantID is the tenant this data belongs to.
	TenantID string `json:"tenant_id"`

	// TotalOccurrences is the total number of occurrences.
	TotalOccurrences int64 `json:"total_occurrences"`

	// DailyAverage is the average occurrences per day.
	DailyAverage float64 `json:"daily_average"`

	// HourlyDistribution shows occurrences by hour of day (0-23).
	HourlyDistribution [24]int `json:"hourly_distribution"`

	// DayOfWeekDistribution shows occurrences by day of week (0-6, Sunday=0).
	DayOfWeekDistribution [7]int `json:"day_of_week_distribution"`

	// Buckets contains time-series frequency data.
	Buckets []FrequencyBucket `json:"buckets"`

	// PeakHour is the hour with most occurrences.
	PeakHour int `json:"peak_hour"`

	// PeakDay is the day of week with most occurrences.
	PeakDay int `json:"peak_day"`

	// FirstSeen is when this pattern was first observed.
	FirstSeen time.Time `json:"first_seen"`

	// LastSeen is when this pattern was last observed.
	LastSeen time.Time `json:"last_seen"`
}

// FrequencyAnalyzer tracks pattern occurrence frequency over time.
type FrequencyAnalyzer struct {
	frequencies map[string]*FrequencyData // patternID -> FrequencyData
	bucketSize  time.Duration
	mu          sync.RWMutex
}

// NewFrequencyAnalyzer creates a new FrequencyAnalyzer.
func NewFrequencyAnalyzer(bucketSize time.Duration) *FrequencyAnalyzer {
	if bucketSize == 0 {
		bucketSize = time.Hour
	}
	return &FrequencyAnalyzer{
		frequencies: make(map[string]*FrequencyData),
		bucketSize:  bucketSize,
	}
}

// RecordOccurrence records a pattern occurrence.
func (a *FrequencyAnalyzer) RecordOccurrence(patternID, tenantID string, timestamp time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	data, exists := a.frequencies[patternID]
	if !exists {
		data = &FrequencyData{
			PatternID: patternID,
			TenantID:  tenantID,
			Buckets:   make([]FrequencyBucket, 0),
			FirstSeen: timestamp,
			LastSeen:  timestamp,
		}
		a.frequencies[patternID] = data
	}

	data.TotalOccurrences++
	data.HourlyDistribution[timestamp.Hour()]++
	data.DayOfWeekDistribution[int(timestamp.Weekday())]++
	data.LastSeen = timestamp

	// Update buckets
	bucketStart := timestamp.Truncate(a.bucketSize)
	updated := false
	for i := range data.Buckets {
		if data.Buckets[i].Timestamp.Equal(bucketStart) {
			data.Buckets[i].Count++
			updated = true
			break
		}
	}
	if !updated {
		data.Buckets = append(data.Buckets, FrequencyBucket{
			Timestamp: bucketStart,
			Count:     1,
			Duration:  a.bucketSize,
		})
	}

	// Update peak hour and day
	a.updatePeaks(data)
}

// updatePeaks calculates peak hour and day.
func (a *FrequencyAnalyzer) updatePeaks(data *FrequencyData) {
	maxHour := 0
	maxHourCount := 0
	for h, count := range data.HourlyDistribution {
		if count > maxHourCount {
			maxHourCount = count
			maxHour = h
		}
	}
	data.PeakHour = maxHour

	maxDay := 0
	maxDayCount := 0
	for d, count := range data.DayOfWeekDistribution {
		if count > maxDayCount {
			maxDayCount = count
			maxDay = d
		}
	}
	data.PeakDay = maxDay
}

// GetFrequency returns frequency data for a pattern.
func (a *FrequencyAnalyzer) GetFrequency(patternID string) *FrequencyData {
	a.mu.RLock()
	defer a.mu.RUnlock()

	data, exists := a.frequencies[patternID]
	if !exists {
		return nil
	}

	// Calculate daily average
	if !data.FirstSeen.IsZero() && !data.LastSeen.IsZero() {
		days := data.LastSeen.Sub(data.FirstSeen).Hours() / 24
		if days > 0 {
			data.DailyAverage = float64(data.TotalOccurrences) / days
		}
	}

	return data
}

// GetAllFrequencies returns frequency data for all patterns.
func (a *FrequencyAnalyzer) GetAllFrequencies() map[string]*FrequencyData {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]*FrequencyData, len(a.frequencies))
	for k, v := range a.frequencies {
		result[k] = v
	}
	return result
}

// CorrelationResult holds the result of correlation analysis.
type CorrelationResult struct {
	// Pattern1ID is the first pattern.
	Pattern1ID string `json:"pattern1_id"`

	// Pattern2ID is the second pattern.
	Pattern2ID string `json:"pattern2_id"`

	// Coefficient is the correlation coefficient (-1 to 1).
	Coefficient float64 `json:"coefficient"`

	// CoOccurrences is the number of times both patterns matched the same item.
	CoOccurrences int `json:"co_occurrences"`

	// Significance indicates statistical significance (p < 0.05).
	Significance bool `json:"significance"`
}

// CorrelationAnalyzer finds relationships between patterns.
type CorrelationAnalyzer struct {
	// coOccurrences tracks items where patterns co-occurred
	coOccurrences map[string]map[string]int // pattern1 -> pattern2 -> count
	// patternMatches tracks which items each pattern matched
	patternMatches map[string]map[string]bool // patternID -> itemID -> matched
	mu             sync.RWMutex
}

// NewCorrelationAnalyzer creates a new CorrelationAnalyzer.
func NewCorrelationAnalyzer() *CorrelationAnalyzer {
	return &CorrelationAnalyzer{
		coOccurrences:  make(map[string]map[string]int),
		patternMatches: make(map[string]map[string]bool),
	}
}

// RecordMatch records a pattern match on an item.
func (a *CorrelationAnalyzer) RecordMatch(patternID, itemID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.patternMatches[patternID]; !exists {
		a.patternMatches[patternID] = make(map[string]bool)
	}
	a.patternMatches[patternID][itemID] = true
}

// RecordCoOccurrence records when two patterns match the same item.
func (a *CorrelationAnalyzer) RecordCoOccurrence(pattern1ID, pattern2ID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Ensure consistent ordering
	if pattern1ID > pattern2ID {
		pattern1ID, pattern2ID = pattern2ID, pattern1ID
	}

	if _, exists := a.coOccurrences[pattern1ID]; !exists {
		a.coOccurrences[pattern1ID] = make(map[string]int)
	}
	a.coOccurrences[pattern1ID][pattern2ID]++
}

// AnalyzeCorrelation calculates correlation between two patterns.
func (a *CorrelationAnalyzer) AnalyzeCorrelation(pattern1ID, pattern2ID string) *CorrelationResult {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Ensure consistent ordering for lookup
	p1, p2 := pattern1ID, pattern2ID
	if p1 > p2 {
		p1, p2 = p2, p1
	}

	coCount := 0
	if matches, exists := a.coOccurrences[p1]; exists {
		coCount = matches[p2]
	}

	matches1 := len(a.patternMatches[pattern1ID])
	matches2 := len(a.patternMatches[pattern2ID])

	if matches1 == 0 || matches2 == 0 {
		return &CorrelationResult{
			Pattern1ID:    pattern1ID,
			Pattern2ID:    pattern2ID,
			Coefficient:   0,
			CoOccurrences: coCount,
			Significance:  false,
		}
	}

	// Calculate Jaccard similarity as a correlation proxy
	union := matches1 + matches2 - coCount
	if union == 0 {
		return &CorrelationResult{
			Pattern1ID:    pattern1ID,
			Pattern2ID:    pattern2ID,
			Coefficient:   0,
			CoOccurrences: coCount,
			Significance:  false,
		}
	}

	coefficient := float64(coCount) / float64(union)

	// Simple significance test (co-occurrences > threshold)
	significance := coCount >= 5 && coefficient > 0.3

	return &CorrelationResult{
		Pattern1ID:    pattern1ID,
		Pattern2ID:    pattern2ID,
		Coefficient:   coefficient,
		CoOccurrences: coCount,
		Significance:  significance,
	}
}

// FindRelatedPatterns finds patterns correlated with a given pattern.
func (a *CorrelationAnalyzer) FindRelatedPatterns(patternID string, minCoefficient float64) []*CorrelationResult {
	a.mu.RLock()
	patterns := make([]string, 0, len(a.patternMatches))
	for pid := range a.patternMatches {
		if pid != patternID {
			patterns = append(patterns, pid)
		}
	}
	a.mu.RUnlock()

	results := make([]*CorrelationResult, 0)
	for _, pid := range patterns {
		result := a.AnalyzeCorrelation(patternID, pid)
		if result.Coefficient >= minCoefficient {
			results = append(results, result)
		}
	}

	// Sort by coefficient descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Coefficient > results[j].Coefficient
	})

	return results
}

// TrendData holds trend analysis results.
type TrendData struct {
	// PatternID is the pattern being analyzed.
	PatternID string `json:"pattern_id"`

	// Direction indicates trend direction: 1=increasing, 0=stable, -1=decreasing.
	Direction int `json:"direction"`

	// Slope is the rate of change in occurrences per day.
	Slope float64 `json:"slope"`

	// Volatility measures how much the frequency varies.
	Volatility float64 `json:"volatility"`

	// RecentAverage is the average frequency over the recent period.
	RecentAverage float64 `json:"recent_average"`

	// HistoricalAverage is the average frequency over the historical period.
	HistoricalAverage float64 `json:"historical_average"`

	// ChangePercent is the percentage change from historical to recent.
	ChangePercent float64 `json:"change_percent"`

	// IsAnomaly indicates if recent activity is anomalous.
	IsAnomaly bool `json:"is_anomaly"`

	// AnalyzedAt is when this analysis was performed.
	AnalyzedAt time.Time `json:"analyzed_at"`
}

// TrendAnalyzer detects pattern changes over time.
type TrendAnalyzer struct {
	frequencyAnalyzer *FrequencyAnalyzer
	windowSize        time.Duration // Recent window for comparison
	historicalSize    time.Duration // Historical window for baseline
}

// NewTrendAnalyzer creates a new TrendAnalyzer.
func NewTrendAnalyzer(frequencyAnalyzer *FrequencyAnalyzer) *TrendAnalyzer {
	return &TrendAnalyzer{
		frequencyAnalyzer: frequencyAnalyzer,
		windowSize:        7 * 24 * time.Hour,  // 1 week recent
		historicalSize:    30 * 24 * time.Hour, // 30 days historical
	}
}

// SetWindows sets the analysis windows.
func (a *TrendAnalyzer) SetWindows(recent, historical time.Duration) {
	a.windowSize = recent
	a.historicalSize = historical
}

// AnalyzeTrend analyzes the trend for a specific pattern.
func (a *TrendAnalyzer) AnalyzeTrend(patternID string) *TrendData {
	freqData := a.frequencyAnalyzer.GetFrequency(patternID)
	if freqData == nil || len(freqData.Buckets) < 2 {
		return &TrendData{
			PatternID:  patternID,
			Direction:  0,
			AnalyzedAt: time.Now().UTC(),
		}
	}

	now := time.Now().UTC()
	recentStart := now.Add(-a.windowSize)
	historicalStart := now.Add(-a.historicalSize)

	var recentSum, historicalSum int
	var recentCount, historicalCount int

	for _, bucket := range freqData.Buckets {
		if bucket.Timestamp.After(recentStart) {
			recentSum += bucket.Count
			recentCount++
		} else if bucket.Timestamp.After(historicalStart) {
			historicalSum += bucket.Count
			historicalCount++
		}
	}

	var recentAvg, historicalAvg float64
	if recentCount > 0 {
		recentAvg = float64(recentSum) / float64(recentCount)
	}
	if historicalCount > 0 {
		historicalAvg = float64(historicalSum) / float64(historicalCount)
	}

	// Calculate change percent
	var changePercent float64
	if historicalAvg > 0 {
		changePercent = ((recentAvg - historicalAvg) / historicalAvg) * 100
	}

	// Determine direction
	direction := 0
	if changePercent > 20 {
		direction = 1 // Increasing
	} else if changePercent < -20 {
		direction = -1 // Decreasing
	}

	// Calculate slope using linear regression on buckets
	slope := a.calculateSlope(freqData.Buckets)

	// Calculate volatility (standard deviation)
	volatility := a.calculateVolatility(freqData.Buckets)

	// Detect anomaly (recent avg > 2 standard deviations from historical)
	isAnomaly := historicalAvg > 0 && volatility > 0 &&
		math.Abs(recentAvg-historicalAvg) > 2*volatility

	return &TrendData{
		PatternID:         patternID,
		Direction:         direction,
		Slope:             slope,
		Volatility:        volatility,
		RecentAverage:     recentAvg,
		HistoricalAverage: historicalAvg,
		ChangePercent:     changePercent,
		IsAnomaly:         isAnomaly,
		AnalyzedAt:        now,
	}
}

// calculateSlope performs simple linear regression on frequency buckets.
func (a *TrendAnalyzer) calculateSlope(buckets []FrequencyBucket) float64 {
	n := len(buckets)
	if n < 2 {
		return 0
	}

	// Sort buckets by timestamp
	sorted := make([]FrequencyBucket, len(buckets))
	copy(sorted, buckets)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Calculate slope using least squares
	var sumX, sumY, sumXY, sumX2 float64
	for i, bucket := range sorted {
		x := float64(i)
		y := float64(bucket.Count)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	nf := float64(n)
	denominator := nf*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0
	}

	return (nf*sumXY - sumX*sumY) / denominator
}

// calculateVolatility calculates standard deviation of bucket counts.
func (a *TrendAnalyzer) calculateVolatility(buckets []FrequencyBucket) float64 {
	n := len(buckets)
	if n < 2 {
		return 0
	}

	var sum float64
	for _, b := range buckets {
		sum += float64(b.Count)
	}
	mean := sum / float64(n)

	var variance float64
	for _, b := range buckets {
		diff := float64(b.Count) - mean
		variance += diff * diff
	}
	variance /= float64(n - 1)

	return math.Sqrt(variance)
}

// FindTrendingPatterns returns patterns with significant trend changes.
func (a *TrendAnalyzer) FindTrendingPatterns(direction int, minChangePercent float64) []*TrendData {
	allFreq := a.frequencyAnalyzer.GetAllFrequencies()
	results := make([]*TrendData, 0)

	for patternID := range allFreq {
		trend := a.AnalyzeTrend(patternID)
		if trend.Direction == direction && math.Abs(trend.ChangePercent) >= minChangePercent {
			results = append(results, trend)
		}
	}

	// Sort by change percent
	sort.Slice(results, func(i, j int) bool {
		return math.Abs(results[i].ChangePercent) > math.Abs(results[j].ChangePercent)
	})

	return results
}

// PatternAnalyzer provides comprehensive pattern analysis capabilities.
type PatternAnalyzer struct {
	detector            *PatternDetector
	frequencyAnalyzer   *FrequencyAnalyzer
	correlationAnalyzer *CorrelationAnalyzer
	trendAnalyzer       *TrendAnalyzer
}

// NewPatternAnalyzer creates a new PatternAnalyzer.
func NewPatternAnalyzer(detector *PatternDetector) *PatternAnalyzer {
	freqAnalyzer := NewFrequencyAnalyzer(time.Hour)
	return &PatternAnalyzer{
		detector:            detector,
		frequencyAnalyzer:   freqAnalyzer,
		correlationAnalyzer: NewCorrelationAnalyzer(),
		trendAnalyzer:       NewTrendAnalyzer(freqAnalyzer),
	}
}

// RecordPatternMatch records a pattern match for analysis.
func (a *PatternAnalyzer) RecordPatternMatch(pattern *Pattern, item *automation.ReviewItem) {
	timestamp := time.Now().UTC()

	// Record frequency
	a.frequencyAnalyzer.RecordOccurrence(pattern.ID, pattern.TenantID, timestamp)

	// Record for correlation analysis
	a.correlationAnalyzer.RecordMatch(pattern.ID, item.ID)
}

// RecordPatternMatches records multiple pattern matches for the same item.
func (a *PatternAnalyzer) RecordPatternMatches(patterns []*Pattern, item *automation.ReviewItem) {
	timestamp := time.Now().UTC()

	for _, pattern := range patterns {
		a.frequencyAnalyzer.RecordOccurrence(pattern.ID, pattern.TenantID, timestamp)
		a.correlationAnalyzer.RecordMatch(pattern.ID, item.ID)
	}

	// Record co-occurrences
	for i := 0; i < len(patterns); i++ {
		for j := i + 1; j < len(patterns); j++ {
			a.correlationAnalyzer.RecordCoOccurrence(patterns[i].ID, patterns[j].ID)
		}
	}
}

// GetFrequencyAnalyzer returns the frequency analyzer.
func (a *PatternAnalyzer) GetFrequencyAnalyzer() *FrequencyAnalyzer {
	return a.frequencyAnalyzer
}

// GetCorrelationAnalyzer returns the correlation analyzer.
func (a *PatternAnalyzer) GetCorrelationAnalyzer() *CorrelationAnalyzer {
	return a.correlationAnalyzer
}

// GetTrendAnalyzer returns the trend analyzer.
func (a *PatternAnalyzer) GetTrendAnalyzer() *TrendAnalyzer {
	return a.trendAnalyzer
}

// AnalyzePattern performs comprehensive analysis of a pattern.
func (a *PatternAnalyzer) AnalyzePattern(patternID string) *PatternAnalysisResult {
	freq := a.frequencyAnalyzer.GetFrequency(patternID)
	trend := a.trendAnalyzer.AnalyzeTrend(patternID)
	related := a.correlationAnalyzer.FindRelatedPatterns(patternID, 0.2)

	return &PatternAnalysisResult{
		PatternID:       patternID,
		FrequencyData:   freq,
		TrendData:       trend,
		RelatedPatterns: related,
		AnalyzedAt:      time.Now().UTC(),
	}
}

// PatternAnalysisResult holds complete analysis results for a pattern.
type PatternAnalysisResult struct {
	// PatternID is the analyzed pattern.
	PatternID string `json:"pattern_id"`

	// FrequencyData holds frequency analysis.
	FrequencyData *FrequencyData `json:"frequency_data,omitempty"`

	// TrendData holds trend analysis.
	TrendData *TrendData `json:"trend_data,omitempty"`

	// RelatedPatterns holds correlated patterns.
	RelatedPatterns []*CorrelationResult `json:"related_patterns,omitempty"`

	// AnalyzedAt is when the analysis was performed.
	AnalyzedAt time.Time `json:"analyzed_at"`
}
