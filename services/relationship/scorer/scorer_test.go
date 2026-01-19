package scorer

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockLogger implements logging.Logger for testing.
type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...logging.Field) {}
func (m *mockLogger) Info(msg string, fields ...logging.Field)  {}
func (m *mockLogger) Warn(msg string, fields ...logging.Field)  {}
func (m *mockLogger) Error(msg string, fields ...logging.Field) {}
func (m *mockLogger) With(fields ...logging.Field) logging.Logger {
	return m
}
func (m *mockLogger) WithContext(ctx context.Context) logging.Logger {
	return m
}

// fixedTimeNow returns a function that always returns the same time.
func fixedTimeNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// ============================================================================
// Factor Tests
// ============================================================================

func TestFrequencyFactor_Score(t *testing.T) {
	factor := NewFrequencyFactor(0.25, 1, 10)
	ctx := context.Background()
	rel := &Relationship{ID: "test-rel"}

	tests := []struct {
		name          string
		evidenceCount int
		wantMin       float64
		wantMax       float64
	}{
		{
			name:          "no evidence",
			evidenceCount: 0,
			wantMin:       0.0,
			wantMax:       0.0,
		},
		{
			name:          "single evidence",
			evidenceCount: 1,
			wantMin:       0.2,
			wantMax:       0.4,
		},
		{
			name:          "moderate evidence",
			evidenceCount: 5,
			wantMin:       0.6,
			wantMax:       0.85,
		},
		{
			name:          "optimal evidence",
			evidenceCount: 10,
			wantMin:       0.95,
			wantMax:       1.0,
		},
		{
			name:          "above optimal",
			evidenceCount: 20,
			wantMin:       1.0,
			wantMax:       1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := make([]Evidence, tt.evidenceCount)
			for i := range evidence {
				evidence[i] = Evidence{ID: "e" + string(rune(i))}
			}

			score := factor.Score(ctx, rel, evidence)

			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("FrequencyFactor.Score() = %v, want between %v and %v",
					score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestFrequencyFactor_Explanation(t *testing.T) {
	factor := DefaultFrequencyFactor()

	tests := []struct {
		score    float64
		contains string
	}{
		{0.95, "Very frequent"},
		{0.75, "Frequent"},
		{0.55, "Moderate"},
		{0.35, "Occasional"},
		{0.1, "Rare"},
		{0.0, "Insufficient"},
	}

	for _, tt := range tests {
		t.Run(tt.contains, func(t *testing.T) {
			explanation := factor.Explanation(tt.score)
			if !containsString(explanation, tt.contains) {
				t.Errorf("Explanation(%v) = %q, should contain %q",
					tt.score, explanation, tt.contains)
			}
		})
	}
}

func TestRecencyFactor_Score(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	factor := NewRecencyFactor(0.25, 90*24*time.Hour, 0.1)
	factor.Now = fixedTimeNow(now)

	ctx := context.Background()
	rel := &Relationship{ID: "test-rel"}

	tests := []struct {
		name    string
		age     time.Duration
		wantMin float64
		wantMax float64
	}{
		{
			name:    "very recent (1 day)",
			age:     24 * time.Hour,
			wantMin: 0.98,
			wantMax: 1.0,
		},
		{
			name:    "half-life (90 days)",
			age:     90 * 24 * time.Hour,
			wantMin: 0.48,
			wantMax: 0.52,
		},
		{
			name:    "double half-life (180 days)",
			age:     180 * 24 * time.Hour,
			wantMin: 0.24,
			wantMax: 0.26,
		},
		{
			name:    "very old (1 year)",
			age:     365 * 24 * time.Hour,
			wantMin: 0.1,
			wantMax: 0.15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := []Evidence{
				{
					ID:           "e1",
					DiscoveredAt: now.Add(-tt.age),
				},
			}

			score := factor.Score(ctx, rel, evidence)

			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("RecencyFactor.Score() = %v, want between %v and %v",
					score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestRecencyFactor_UseMostRecent(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	factor := NewRecencyFactor(0.25, 90*24*time.Hour, 0.1)
	factor.Now = fixedTimeNow(now)

	ctx := context.Background()
	rel := &Relationship{ID: "test-rel"}

	// Mix of old and recent evidence
	evidence := []Evidence{
		{ID: "e1", DiscoveredAt: now.Add(-365 * 24 * time.Hour)}, // 1 year old
		{ID: "e2", DiscoveredAt: now.Add(-30 * 24 * time.Hour)},  // 30 days old
		{ID: "e3", DiscoveredAt: now.Add(-180 * 24 * time.Hour)}, // 6 months old
	}

	score := factor.Score(ctx, rel, evidence)

	// Should use most recent (30 days) - score should be high
	if score < 0.75 {
		t.Errorf("RecencyFactor should use most recent evidence, got score %v", score)
	}
}

func TestSourceQualityFactor_Score(t *testing.T) {
	factor := DefaultSourceQualityFactor()
	ctx := context.Background()
	rel := &Relationship{ID: "test-rel"}

	tests := []struct {
		name       string
		sourceType string
		wantMin    float64
		wantMax    float64
	}{
		{
			name:       "high quality - meeting",
			sourceType: "meeting",
			wantMin:    0.93,
			wantMax:    0.97,
		},
		{
			name:       "high quality - email",
			sourceType: "email",
			wantMin:    0.88,
			wantMax:    0.92,
		},
		{
			name:       "medium quality - document",
			sourceType: "document",
			wantMin:    0.78,
			wantMax:    0.82,
		},
		{
			name:       "low quality - web page",
			sourceType: "web_page",
			wantMin:    0.48,
			wantMax:    0.52,
		},
		{
			name:       "unknown type - uses default",
			sourceType: "unknown_type",
			wantMin:    0.48,
			wantMax:    0.52,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := []Evidence{
				{ID: "e1", SourceType: tt.sourceType},
			}

			score := factor.Score(ctx, rel, evidence)

			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("SourceQualityFactor.Score() = %v, want between %v and %v",
					score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSourceQualityFactor_WeightedAverage(t *testing.T) {
	factor := DefaultSourceQualityFactor()
	ctx := context.Background()
	rel := &Relationship{ID: "test-rel"}

	// Mix of high and low quality sources
	evidence := []Evidence{
		{ID: "e1", SourceType: "meeting", ConfidenceContribution: 1.0},  // 0.95
		{ID: "e2", SourceType: "web_page", ConfidenceContribution: 1.0}, // 0.5
	}

	score := factor.Score(ctx, rel, evidence)

	// Should be average of 0.95 and 0.5 = 0.725
	if score < 0.7 || score > 0.75 {
		t.Errorf("SourceQualityFactor should average sources, got %v", score)
	}
}

func TestConsistencyFactor_Score(t *testing.T) {
	factor := NewConsistencyFactor(0.25, 2, 5)
	ctx := context.Background()
	rel := &Relationship{ID: "test-rel"}

	tests := []struct {
		name      string
		evidence  []Evidence
		wantMin   float64
		wantMax   float64
	}{
		{
			name:      "no evidence",
			evidence:  []Evidence{},
			wantMin:   0.0,
			wantMax:   0.0,
		},
		{
			name: "single source",
			evidence: []Evidence{
				{ID: "e1", SourceID: "s1", SourceType: "email"},
			},
			wantMin: 0.48,
			wantMax: 0.52,
		},
		{
			name: "two sources same type",
			evidence: []Evidence{
				{ID: "e1", SourceID: "s1", SourceType: "email"},
				{ID: "e2", SourceID: "s2", SourceType: "email"},
			},
			wantMin: 0.48,
			wantMax: 0.52,
		},
		{
			name: "two sources different types",
			evidence: []Evidence{
				{ID: "e1", SourceID: "s1", SourceType: "email"},
				{ID: "e2", SourceID: "s2", SourceType: "meeting"},
			},
			wantMin: 0.6,
			wantMax: 0.8,
		},
		{
			name: "many diverse sources",
			evidence: []Evidence{
				{ID: "e1", SourceID: "s1", SourceType: "email"},
				{ID: "e2", SourceID: "s2", SourceType: "meeting"},
				{ID: "e3", SourceID: "s3", SourceType: "document"},
				{ID: "e4", SourceID: "s4", SourceType: "slack_channel"},
				{ID: "e5", SourceID: "s5", SourceType: "calendar_invite"},
			},
			wantMin: 0.95,
			wantMax: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := factor.Score(ctx, rel, tt.evidence)

			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("ConsistencyFactor.Score() = %v, want between %v and %v",
					score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestValidationBoostFactor_Score(t *testing.T) {
	factor := NewValidationBoostFactor(1.2, 0.1)
	ctx := context.Background()
	evidence := []Evidence{}

	tests := []struct {
		name   string
		status string
		want   float64
	}{
		{
			name:   "confirmed",
			status: "confirmed",
			want:   1.2,
		},
		{
			name:   "CONFIRMED uppercase",
			status: "CONFIRMED",
			want:   1.2,
		},
		{
			name:   "rejected",
			status: "rejected",
			want:   0.1,
		},
		{
			name:   "pending",
			status: "pending",
			want:   1.0,
		},
		{
			name:   "discovered",
			status: "discovered",
			want:   1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := &Relationship{ID: "test-rel", Status: tt.status}
			score := factor.Score(ctx, rel, evidence)

			if math.Abs(score-tt.want) > 0.01 {
				t.Errorf("ValidationBoostFactor.Score() = %v, want %v", score, tt.want)
			}
		})
	}
}

// ============================================================================
// ConfidenceScorer Tests
// ============================================================================

func TestConfidenceScorer_Score_NilRelationship(t *testing.T) {
	scorer := NewConfidenceScorer(nil, &mockLogger{})

	_, err := scorer.Score(context.Background(), nil)
	if err == nil {
		t.Error("Score() with nil relationship should return error")
	}
}

func TestConfidenceScorer_Score_BasicScoring(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	cfg := DefaultScorerConfig()
	cfg.DecayEnabled = false // Disable decay for predictable testing
	cfg.ValidationBoostEnabled = false

	// Set fixed time for recency factor
	for _, f := range cfg.Factors {
		if rf, ok := f.(*RecencyFactor); ok {
			rf.Now = fixedTimeNow(now)
		}
	}

	scorer := NewConfidenceScorer(cfg, &mockLogger{})
	scorer.now = fixedTimeNow(now)

	rel := &Relationship{
		ID:        "test-rel",
		UpdatedAt: now,
	}

	// Provide good evidence
	evidence := []Evidence{
		{ID: "e1", SourceID: "s1", SourceType: "email", DiscoveredAt: now.Add(-24 * time.Hour)},
		{ID: "e2", SourceID: "s2", SourceType: "meeting", DiscoveredAt: now.Add(-48 * time.Hour)},
		{ID: "e3", SourceID: "s3", SourceType: "document", DiscoveredAt: now.Add(-72 * time.Hour)},
	}

	result, err := scorer.ScoreWithEvidence(context.Background(), rel, evidence)
	if err != nil {
		t.Fatalf("ScoreWithEvidence() error = %v", err)
	}

	// With multiple recent high-quality diverse sources, score should be good
	if result.Confidence < 0.5 {
		t.Errorf("Expected confidence > 0.5 for good evidence, got %v", result.Confidence)
	}

	// Check all factors were scored
	expectedFactors := []string{"frequency", "recency", "source_quality", "consistency"}
	for _, name := range expectedFactors {
		if _, ok := result.FactorScores[name]; !ok {
			t.Errorf("Missing factor score for %s", name)
		}
	}

	// Check explanation is generated
	if result.Explanation == "" {
		t.Error("Explanation should not be empty")
	}
}

func TestConfidenceScorer_Score_WithDecay(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	cfg := DefaultScorerConfig()
	cfg.DecayEnabled = true
	cfg.DecayHalfLife = 90 * 24 * time.Hour
	cfg.DecayMinimum = 0.1
	cfg.ValidationBoostEnabled = false

	// Set fixed time for recency factor
	for _, f := range cfg.Factors {
		if rf, ok := f.(*RecencyFactor); ok {
			rf.Now = fixedTimeNow(now)
		}
	}

	scorer := NewConfidenceScorer(cfg, &mockLogger{})
	scorer.now = fixedTimeNow(now)

	// Relationship updated 90 days ago (half-life)
	rel := &Relationship{
		ID:        "test-rel",
		UpdatedAt: now.Add(-90 * 24 * time.Hour),
	}

	evidence := []Evidence{
		{ID: "e1", SourceID: "s1", SourceType: "email", DiscoveredAt: now.Add(-90 * 24 * time.Hour)},
	}

	result, err := scorer.ScoreWithEvidence(context.Background(), rel, evidence)
	if err != nil {
		t.Fatalf("ScoreWithEvidence() error = %v", err)
	}

	// Score should be decayed
	if result.DecayedFrom == nil {
		t.Error("DecayedFrom should be set when decay is applied")
	}

	// Decayed score should be roughly half the original
	if result.DecayedFrom != nil && result.Confidence > *result.DecayedFrom*0.6 {
		t.Errorf("Decayed score %v should be significantly less than original %v",
			result.Confidence, *result.DecayedFrom)
	}
}

func TestConfidenceScorer_Score_WithValidationBoost(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	cfg := DefaultScorerConfig()
	cfg.DecayEnabled = false
	cfg.ValidationBoostEnabled = true

	// Set fixed time
	for _, f := range cfg.Factors {
		if rf, ok := f.(*RecencyFactor); ok {
			rf.Now = fixedTimeNow(now)
		}
	}

	scorer := NewConfidenceScorer(cfg, &mockLogger{})
	scorer.now = fixedTimeNow(now)

	evidence := []Evidence{
		{ID: "e1", SourceID: "s1", SourceType: "email", DiscoveredAt: now},
	}

	// Score unconfirmed relationship
	relUnconfirmed := &Relationship{ID: "rel1", Status: "discovered", UpdatedAt: now}
	resultUnconfirmed, _ := scorer.ScoreWithEvidence(context.Background(), relUnconfirmed, evidence)

	// Score confirmed relationship
	relConfirmed := &Relationship{ID: "rel2", Status: "confirmed", UpdatedAt: now}
	resultConfirmed, _ := scorer.ScoreWithEvidence(context.Background(), relConfirmed, evidence)

	// Confirmed should have higher score
	if resultConfirmed.Confidence <= resultUnconfirmed.Confidence {
		t.Errorf("Confirmed score %v should be higher than unconfirmed %v",
			resultConfirmed.Confidence, resultUnconfirmed.Confidence)
	}
}

func TestConfidenceScorer_AggregateScores(t *testing.T) {
	scorer := NewConfidenceScorer(nil, &mockLogger{})

	tests := []struct {
		name    string
		scores  []float64
		wantMin float64
		wantMax float64
	}{
		{
			name:    "empty scores",
			scores:  []float64{},
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "single score",
			scores:  []float64{0.7},
			wantMin: 0.7,
			wantMax: 0.7,
		},
		{
			name:    "two equal scores",
			scores:  []float64{0.5, 0.5},
			wantMin: 0.74, // 0.5 + 0.5 - 0.25 = 0.75
			wantMax: 0.76,
		},
		{
			name:    "multiple scores",
			scores:  []float64{0.3, 0.3, 0.3},
			wantMin: 0.6,
			wantMax: 0.7,
		},
		{
			name:    "high scores",
			scores:  []float64{0.9, 0.9},
			wantMin: 0.98,
			wantMax: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scorer.AggregateScores(tt.scores)

			if result < tt.wantMin || result > tt.wantMax {
				t.Errorf("AggregateScores(%v) = %v, want between %v and %v",
					tt.scores, result, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestConfidenceScorer_FilterByThreshold(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.MinimumConfidence = 0.5

	scorer := NewConfidenceScorer(cfg, &mockLogger{})

	results := []*ScoreResult{
		{Confidence: 0.3, PassesThreshold: false},
		{Confidence: 0.5, PassesThreshold: true},
		{Confidence: 0.7, PassesThreshold: true},
		{Confidence: 0.4, PassesThreshold: false},
		{Confidence: 0.9, PassesThreshold: true},
	}

	filtered := scorer.FilterByThreshold(results)

	if len(filtered) != 3 {
		t.Errorf("FilterByThreshold() returned %d results, want 3", len(filtered))
	}

	for _, r := range filtered {
		if r.Confidence < 0.5 {
			t.Errorf("Filtered result has confidence %v, should be >= 0.5", r.Confidence)
		}
	}
}

func TestConfidenceScorer_FilterByConfidence(t *testing.T) {
	scorer := NewConfidenceScorer(nil, &mockLogger{})

	results := []*ScoreResult{
		{Confidence: 0.3},
		{Confidence: 0.5},
		{Confidence: 0.7},
		{Confidence: 0.4},
		{Confidence: 0.9},
	}

	filtered := scorer.FilterByConfidence(results, 0.6)

	if len(filtered) != 2 {
		t.Errorf("FilterByConfidence() returned %d results, want 2", len(filtered))
	}

	for _, r := range filtered {
		if r.Confidence < 0.6 {
			t.Errorf("Filtered result has confidence %v, should be >= 0.6", r.Confidence)
		}
	}
}

func TestConfidenceScorer_ScoreBatch(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	cfg := DefaultScorerConfig()
	cfg.DecayEnabled = false
	cfg.ValidationBoostEnabled = false

	for _, f := range cfg.Factors {
		if rf, ok := f.(*RecencyFactor); ok {
			rf.Now = fixedTimeNow(now)
		}
	}

	scorer := NewConfidenceScorer(cfg, &mockLogger{})
	scorer.now = fixedTimeNow(now)

	relationships := []*Relationship{
		{ID: "rel1", UpdatedAt: now},
		{ID: "rel2", UpdatedAt: now},
		{ID: "rel3", UpdatedAt: now},
	}

	evidenceMap := map[string][]Evidence{
		"rel1": {{ID: "e1", SourceID: "s1", SourceType: "email", DiscoveredAt: now}},
		"rel2": {
			{ID: "e2", SourceID: "s2", SourceType: "email", DiscoveredAt: now},
			{ID: "e3", SourceID: "s3", SourceType: "meeting", DiscoveredAt: now},
		},
		// rel3 has no evidence
	}

	results, err := scorer.ScoreBatch(context.Background(), relationships, evidenceMap)
	if err != nil {
		t.Fatalf("ScoreBatch() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("ScoreBatch() returned %d results, want 3", len(results))
	}

	// rel2 with more diverse evidence should score higher than rel1
	var rel1Score, rel2Score float64
	for i, rel := range relationships {
		if rel.ID == "rel1" {
			rel1Score = results[i].Confidence
		}
		if rel.ID == "rel2" {
			rel2Score = results[i].Confidence
		}
	}

	if rel2Score <= rel1Score {
		t.Errorf("rel2 score %v should be higher than rel1 score %v due to more diverse evidence",
			rel2Score, rel1Score)
	}
}

func TestConfidenceScorer_GetSetMinimumConfidence(t *testing.T) {
	scorer := NewConfidenceScorer(nil, &mockLogger{})

	initial := scorer.GetMinimumConfidence()
	if initial != 0.3 { // Default from DefaultScorerConfig
		t.Errorf("Initial minimum confidence = %v, want 0.3", initial)
	}

	scorer.SetMinimumConfidence(0.6)
	if scorer.GetMinimumConfidence() != 0.6 {
		t.Errorf("Minimum confidence after set = %v, want 0.6", scorer.GetMinimumConfidence())
	}

	// Test clamping
	scorer.SetMinimumConfidence(-0.5)
	if scorer.GetMinimumConfidence() != 0 {
		t.Error("Negative threshold should be clamped to 0")
	}

	scorer.SetMinimumConfidence(1.5)
	if scorer.GetMinimumConfidence() != 1.0 {
		t.Error("Threshold > 1 should be clamped to 1")
	}
}

func TestConfidenceScorer_GetFactorWeights(t *testing.T) {
	scorer := NewConfidenceScorer(nil, &mockLogger{})

	weights := scorer.GetFactorWeights()

	expectedFactors := []string{"frequency", "recency", "source_quality", "consistency"}
	for _, name := range expectedFactors {
		weight, ok := weights[name]
		if !ok {
			t.Errorf("Missing weight for factor %s", name)
		}
		if weight != 0.25 { // Default weight
			t.Errorf("Factor %s weight = %v, want 0.25", name, weight)
		}
	}
}

func TestConfidenceScorer_NormalizationMethods(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	// Create evidence with mixed quality
	evidence := []Evidence{
		{ID: "e1", SourceID: "s1", SourceType: "email", DiscoveredAt: now},
		{ID: "e2", SourceID: "s2", SourceType: "meeting", DiscoveredAt: now},
	}

	rel := &Relationship{ID: "test-rel", UpdatedAt: now}

	tests := []struct {
		name   string
		method NormalizationMethod
	}{
		{"weighted_average", NormalizationWeightedAverage},
		{"geometric_mean", NormalizationGeometricMean},
		{"minimum", NormalizationMinimum},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultScorerConfig()
			cfg.DecayEnabled = false
			cfg.ValidationBoostEnabled = false
			cfg.NormalizationMethod = tt.method

			for _, f := range cfg.Factors {
				if rf, ok := f.(*RecencyFactor); ok {
					rf.Now = fixedTimeNow(now)
				}
			}

			scorer := NewConfidenceScorer(cfg, &mockLogger{})
			scorer.now = fixedTimeNow(now)

			result, err := scorer.ScoreWithEvidence(context.Background(), rel, evidence)
			if err != nil {
				t.Fatalf("ScoreWithEvidence() error = %v", err)
			}

			// All methods should produce valid scores
			if result.Confidence < 0 || result.Confidence > 1 {
				t.Errorf("Confidence %v out of valid range [0, 1]", result.Confidence)
			}
		})
	}
}

func TestScoreResult_PassesThreshold(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	cfg := DefaultScorerConfig()
	cfg.MinimumConfidence = 0.5
	cfg.DecayEnabled = false
	cfg.ValidationBoostEnabled = false

	for _, f := range cfg.Factors {
		if rf, ok := f.(*RecencyFactor); ok {
			rf.Now = fixedTimeNow(now)
		}
	}

	scorer := NewConfidenceScorer(cfg, &mockLogger{})
	scorer.now = fixedTimeNow(now)

	// Good evidence - should pass threshold
	relGood := &Relationship{ID: "good-rel", UpdatedAt: now}
	evidenceGood := []Evidence{
		{ID: "e1", SourceID: "s1", SourceType: "meeting", DiscoveredAt: now},
		{ID: "e2", SourceID: "s2", SourceType: "email", DiscoveredAt: now},
		{ID: "e3", SourceID: "s3", SourceType: "document", DiscoveredAt: now},
	}

	resultGood, _ := scorer.ScoreWithEvidence(context.Background(), relGood, evidenceGood)
	if !resultGood.PassesThreshold {
		t.Errorf("Good evidence should pass threshold, got confidence %v", resultGood.Confidence)
	}

	// Poor evidence - should fail threshold
	relPoor := &Relationship{ID: "poor-rel", UpdatedAt: now}
	evidencePoor := []Evidence{} // No evidence

	resultPoor, _ := scorer.ScoreWithEvidence(context.Background(), relPoor, evidencePoor)
	if resultPoor.PassesThreshold {
		t.Errorf("No evidence should fail threshold, got confidence %v", resultPoor.Confidence)
	}
}

// ============================================================================
// Helper functions
// ============================================================================

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// Factor Default Tests
// ============================================================================

func TestDefaultFactors(t *testing.T) {
	t.Run("DefaultFrequencyFactor", func(t *testing.T) {
		f := DefaultFrequencyFactor()
		if f.Name() != "frequency" {
			t.Errorf("Name() = %s, want frequency", f.Name())
		}
		if f.Weight() != 0.25 {
			t.Errorf("Weight() = %v, want 0.25", f.Weight())
		}
	})

	t.Run("DefaultRecencyFactor", func(t *testing.T) {
		f := DefaultRecencyFactor()
		if f.Name() != "recency" {
			t.Errorf("Name() = %s, want recency", f.Name())
		}
		if f.Weight() != 0.25 {
			t.Errorf("Weight() = %v, want 0.25", f.Weight())
		}
	})

	t.Run("DefaultSourceQualityFactor", func(t *testing.T) {
		f := DefaultSourceQualityFactor()
		if f.Name() != "source_quality" {
			t.Errorf("Name() = %s, want source_quality", f.Name())
		}
		if f.Weight() != 0.25 {
			t.Errorf("Weight() = %v, want 0.25", f.Weight())
		}
	})

	t.Run("DefaultConsistencyFactor", func(t *testing.T) {
		f := DefaultConsistencyFactor()
		if f.Name() != "consistency" {
			t.Errorf("Name() = %s, want consistency", f.Name())
		}
		if f.Weight() != 0.25 {
			t.Errorf("Weight() = %v, want 0.25", f.Weight())
		}
	})

	t.Run("DefaultValidationBoostFactor", func(t *testing.T) {
		f := DefaultValidationBoostFactor()
		if f.Name() != "validation_boost" {
			t.Errorf("Name() = %s, want validation_boost", f.Name())
		}
		if f.ConfirmedBoost != 1.2 {
			t.Errorf("ConfirmedBoost = %v, want 1.2", f.ConfirmedBoost)
		}
		if f.RejectedPenalty != 0.1 {
			t.Errorf("RejectedPenalty = %v, want 0.1", f.RejectedPenalty)
		}
	})
}

func TestTimeSinceLastInteractionFactor(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	factor := NewTimeSinceLastInteractionFactor(0.2, 30*24*time.Hour, 180*24*time.Hour)
	factor.Now = fixedTimeNow(now)

	ctx := context.Background()
	rel := &Relationship{ID: "test-rel"}

	tests := []struct {
		name    string
		age     time.Duration
		wantMin float64
		wantMax float64
	}{
		{
			name:    "within active period",
			age:     7 * 24 * time.Hour,
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "at active period boundary",
			age:     30 * 24 * time.Hour,
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "halfway to inactive",
			age:     105 * 24 * time.Hour, // 30 + 75 days
			wantMin: 0.5,
			wantMax: 0.6,
		},
		{
			name:    "at inactive period",
			age:     180 * 24 * time.Hour,
			wantMin: 0.1,
			wantMax: 0.1,
		},
		{
			name:    "beyond inactive period",
			age:     365 * 24 * time.Hour,
			wantMin: 0.1,
			wantMax: 0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := []Evidence{
				{ID: "e1", DiscoveredAt: now.Add(-tt.age)},
			}

			score := factor.Score(ctx, rel, evidence)

			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("TimeSinceLastInteractionFactor.Score() = %v, want between %v and %v",
					score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestNewFactorsWithInvalidParams(t *testing.T) {
	t.Run("FrequencyFactor with invalid min", func(t *testing.T) {
		f := NewFrequencyFactor(0.25, -1, 10)
		if f.MinOccurrences < 1 {
			t.Error("MinOccurrences should be at least 1")
		}
	})

	t.Run("RecencyFactor with invalid half-life", func(t *testing.T) {
		f := NewRecencyFactor(0.25, 0, 0.1)
		if f.HalfLife <= 0 {
			t.Error("HalfLife should have default value")
		}
	})

	t.Run("ConsistencyFactor with invalid min sources", func(t *testing.T) {
		f := NewConsistencyFactor(0.25, 0, 5)
		if f.MinSourcesForBonus < 2 {
			t.Error("MinSourcesForBonus should be at least 2")
		}
	})

	t.Run("ValidationBoostFactor with extreme values", func(t *testing.T) {
		f := NewValidationBoostFactor(3.0, -0.5)
		if f.ConfirmedBoost > 2.0 {
			t.Error("ConfirmedBoost should be capped at 2.0")
		}
		if f.RejectedPenalty < 0 {
			t.Error("RejectedPenalty should not be negative")
		}
	})
}
