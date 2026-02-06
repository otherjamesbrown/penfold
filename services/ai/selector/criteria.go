// Package selector provides AI model selection logic based on task requirements,
// cost optimization, and quality constraints. It supports local-first model selection
// with cloud fallback for complex tasks.
package selector

import (
	"time"
)

// TaskType represents the type of AI task to be performed.
type TaskType string

const (
	// TaskTypeEmbedding is for vector embedding generation.
	TaskTypeEmbedding TaskType = "embedding"

	// TaskTypeSummarization is for content summarization.
	TaskTypeSummarization TaskType = "summarization"

	// TaskTypeExtraction is for information/assertion extraction.
	TaskTypeExtraction TaskType = "extraction"

	// TaskTypeClassification is for content classification.
	TaskTypeClassification TaskType = "classification"
)

// OptimizationMode defines the selection priority strategy.
type OptimizationMode string

const (
	// OptimizationModeBalanced balances cost, quality, and latency.
	OptimizationModeBalanced OptimizationMode = "balanced"

	// OptimizationModeCost prioritizes lower cost over quality/latency.
	OptimizationModeCost OptimizationMode = "cost"

	// OptimizationModeQuality prioritizes quality over cost/latency.
	OptimizationModeQuality OptimizationMode = "quality"

	// OptimizationModeLatency prioritizes low latency over cost/quality.
	OptimizationModeLatency OptimizationMode = "latency"

	// OptimizationModePrivacy prioritizes local models for data privacy.
	OptimizationModePrivacy OptimizationMode = "privacy"
)

// ModelProvider identifies the source/provider of a model.
type ModelProvider string

const (
	// ModelProviderMLX is for local MLX models (vllm-mlx).
	ModelProviderMLX ModelProvider = "mlx"

	// ModelProviderGemini is for Google Gemini cloud models.
	ModelProviderGemini ModelProvider = "gemini"

	// ModelProviderOpenAI is for OpenAI cloud models.
	ModelProviderOpenAI ModelProvider = "openai"
)

// SelectionCriteria defines requirements and constraints for model selection.
type SelectionCriteria struct {
	// TaskType is the type of AI task (required).
	TaskType TaskType

	// ContentSize is the approximate size of the input content in characters.
	// Used to ensure the model can handle the input within its context window.
	ContentSize int

	// MaxLatency is the maximum acceptable latency for the request.
	// If zero, no latency constraint is applied.
	MaxLatency time.Duration

	// MaxCostPerRequest is the maximum acceptable cost per request in USD.
	// If zero, no cost constraint is applied.
	MaxCostPerRequest float64

	// MinQuality is the minimum acceptable quality score (0.0 to 1.0).
	// Quality is a normalized score based on model benchmarks.
	// If zero, defaults to 0.5.
	MinQuality float64

	// OptimizationMode determines the primary optimization target.
	// If empty, defaults to OptimizationModeBalanced.
	OptimizationMode OptimizationMode

	// RequireLocal restricts selection to local models only.
	// Overrides other optimization settings when true.
	RequireLocal bool

	// PreferredProvider is an optional preferred model provider.
	// If set, models from this provider get a selection bonus.
	PreferredProvider ModelProvider

	// PreferredModel is an optional preferred specific model.
	// If set and available, this model is selected directly.
	PreferredModel string
}

// ModelCapabilities describes what a model can do and its performance characteristics.
type ModelCapabilities struct {
	// ModelID is the unique identifier for the model (e.g., "llama3.2", "gemini-2.5-pro").
	ModelID string

	// Provider is the source of the model.
	Provider ModelProvider

	// IsLocal indicates whether the model runs locally.
	IsLocal bool

	// SupportedTasks lists the task types this model can handle.
	SupportedTasks []TaskType

	// ContextWindow is the maximum context length in tokens.
	ContextWindow int

	// EmbeddingDimensions is the output embedding size (for embedding models).
	EmbeddingDimensions int

	// AvgLatency is the typical latency for a standard request.
	AvgLatency time.Duration

	// CostPer1KTokens is the cost per 1000 tokens in USD.
	// For local models, this represents compute cost (typically 0 or very low).
	CostPer1KTokens float64

	// QualityScore is a normalized quality score (0.0 to 1.0) based on benchmarks.
	// Higher values indicate better performance on standard benchmarks.
	QualityScore float64

	// SpeedScore is a normalized speed score (0.0 to 1.0).
	// Higher values indicate faster inference.
	SpeedScore float64

	// IsAvailable indicates whether the model is currently available for use.
	IsAvailable bool

	// WarmupRequired indicates whether the model needs warmup before optimal performance.
	WarmupRequired bool

	// LastUsed tracks when the model was last used for warmup decisions.
	LastUsed time.Time
}

// SupportsTask checks if the model supports the given task type.
func (c *ModelCapabilities) SupportsTask(task TaskType) bool {
	for _, t := range c.SupportedTasks {
		if t == task {
			return true
		}
	}
	return false
}

// CanHandleContentSize checks if the model can handle content of the given character size.
// Assumes approximately 4 characters per token as a rough estimate.
func (c *ModelCapabilities) CanHandleContentSize(charSize int) bool {
	// Rough token estimate: ~4 characters per token
	estimatedTokens := charSize / 4
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}
	// Allow for some overhead (system prompt, output, etc.)
	return estimatedTokens < (c.ContextWindow * 3 / 4)
}

// EstimateCost estimates the cost for processing the given number of tokens.
func (c *ModelCapabilities) EstimateCost(inputTokens, outputTokens int) float64 {
	totalTokens := inputTokens + outputTokens
	return float64(totalTokens) / 1000.0 * c.CostPer1KTokens
}

// NeedsWarmup checks if the model needs warmup based on last usage time.
// Returns true if the model hasn't been used in the specified duration.
func (c *ModelCapabilities) NeedsWarmup(threshold time.Duration) bool {
	if !c.WarmupRequired {
		return false
	}
	if c.LastUsed.IsZero() {
		return true
	}
	return time.Since(c.LastUsed) > threshold
}

// ModelScore represents the fitness score of a model for a given criteria.
type ModelScore struct {
	// ModelID is the identifier of the scored model.
	ModelID string

	// TotalScore is the final weighted score (higher is better).
	TotalScore float64

	// ComponentScores breaks down the scoring by category.
	ComponentScores ScoreComponents

	// Penalties lists any penalties applied and their reasons.
	Penalties []ScorePenalty
}

// ScoreComponents contains individual scoring components.
type ScoreComponents struct {
	// LatencyScore rewards lower latency (0.0 to 1.0).
	LatencyScore float64

	// CostScore rewards lower cost (0.0 to 1.0).
	CostScore float64

	// QualityScore is the model's quality benchmark score (0.0 to 1.0).
	QualityScore float64

	// PrivacyScore rewards local models (0.0 or 1.0).
	PrivacyScore float64

	// AvailabilityScore considers model readiness (0.0 or 1.0).
	AvailabilityScore float64
}

// ScorePenalty represents a scoring penalty applied to a model.
type ScorePenalty struct {
	// Reason describes why the penalty was applied.
	Reason string

	// Amount is the penalty amount (subtracted from total score).
	Amount float64
}

// ScoreWeights defines the relative importance of each scoring component.
type ScoreWeights struct {
	Latency      float64
	Cost         float64
	Quality      float64
	Privacy      float64
	Availability float64
}

// DefaultBalancedWeights returns weights for balanced optimization.
func DefaultBalancedWeights() ScoreWeights {
	return ScoreWeights{
		Latency:      0.20,
		Cost:         0.20,
		Quality:      0.30,
		Privacy:      0.15,
		Availability: 0.15,
	}
}

// CostOptimizedWeights returns weights that prioritize cost.
func CostOptimizedWeights() ScoreWeights {
	return ScoreWeights{
		Latency:      0.10,
		Cost:         0.50,
		Quality:      0.15,
		Privacy:      0.10,
		Availability: 0.15,
	}
}

// QualityOptimizedWeights returns weights that prioritize quality.
func QualityOptimizedWeights() ScoreWeights {
	return ScoreWeights{
		Latency:      0.10,
		Cost:         0.10,
		Quality:      0.55,
		Privacy:      0.10,
		Availability: 0.15,
	}
}

// LatencyOptimizedWeights returns weights that prioritize latency.
func LatencyOptimizedWeights() ScoreWeights {
	return ScoreWeights{
		Latency:      0.50,
		Cost:         0.10,
		Quality:      0.15,
		Privacy:      0.10,
		Availability: 0.15,
	}
}

// PrivacyOptimizedWeights returns weights that prioritize privacy (local models).
func PrivacyOptimizedWeights() ScoreWeights {
	return ScoreWeights{
		Latency:      0.15,
		Cost:         0.15,
		Quality:      0.20,
		Privacy:      0.35,
		Availability: 0.15,
	}
}

// GetWeightsForMode returns the appropriate weights for the given optimization mode.
func GetWeightsForMode(mode OptimizationMode) ScoreWeights {
	switch mode {
	case OptimizationModeCost:
		return CostOptimizedWeights()
	case OptimizationModeQuality:
		return QualityOptimizedWeights()
	case OptimizationModeLatency:
		return LatencyOptimizedWeights()
	case OptimizationModePrivacy:
		return PrivacyOptimizedWeights()
	default:
		return DefaultBalancedWeights()
	}
}

// CalculateScore computes the fitness score for a model given criteria and weights.
func CalculateScore(
	model *ModelCapabilities,
	criteria *SelectionCriteria,
	weights ScoreWeights,
	maxLatencyRef time.Duration,
	maxCostRef float64,
) ModelScore {
	score := ModelScore{
		ModelID:   model.ModelID,
		Penalties: []ScorePenalty{},
	}

	// Initialize component scores
	components := ScoreComponents{}

	// Latency score: lower is better
	if maxLatencyRef > 0 {
		latencyRatio := float64(model.AvgLatency) / float64(maxLatencyRef)
		components.LatencyScore = clamp(1.0-latencyRatio, 0, 1)
	} else {
		// Default scoring based on speedScore
		components.LatencyScore = model.SpeedScore
	}

	// Cost score: lower is better
	if maxCostRef > 0 {
		// Estimate cost for typical request (assume 500 input tokens, 100 output tokens)
		estimatedCost := model.EstimateCost(500, 100)
		costRatio := estimatedCost / maxCostRef
		components.CostScore = clamp(1.0-costRatio, 0, 1)
	} else {
		// For local models, cost is effectively 0
		if model.IsLocal {
			components.CostScore = 1.0
		} else {
			// Normalize based on typical cloud pricing
			components.CostScore = clamp(1.0-(model.CostPer1KTokens/0.01), 0, 1)
		}
	}

	// Quality score: direct from model capabilities
	components.QualityScore = model.QualityScore

	// Privacy score: 1.0 for local, 0.0 for cloud
	if model.IsLocal {
		components.PrivacyScore = 1.0
	} else {
		components.PrivacyScore = 0.0
	}

	// Availability score: 1.0 if available, 0.0 if not
	if model.IsAvailable {
		components.AvailabilityScore = 1.0
	} else {
		components.AvailabilityScore = 0.0
	}

	score.ComponentScores = components

	// Calculate weighted total
	total := components.LatencyScore * weights.Latency
	total += components.CostScore * weights.Cost
	total += components.QualityScore * weights.Quality
	total += components.PrivacyScore * weights.Privacy
	total += components.AvailabilityScore * weights.Availability

	// Apply penalties
	// Penalty for not supporting required task (severe)
	if !model.SupportsTask(criteria.TaskType) {
		penalty := ScorePenalty{Reason: "does not support task type", Amount: 10.0}
		score.Penalties = append(score.Penalties, penalty)
		total -= penalty.Amount
	}

	// Penalty for insufficient context window
	if criteria.ContentSize > 0 && !model.CanHandleContentSize(criteria.ContentSize) {
		penalty := ScorePenalty{Reason: "insufficient context window", Amount: 5.0}
		score.Penalties = append(score.Penalties, penalty)
		total -= penalty.Amount
	}

	// Penalty for exceeding max latency constraint
	if criteria.MaxLatency > 0 && model.AvgLatency > criteria.MaxLatency {
		penalty := ScorePenalty{Reason: "exceeds max latency", Amount: 3.0}
		score.Penalties = append(score.Penalties, penalty)
		total -= penalty.Amount
	}

	// Penalty for not being local when required
	if criteria.RequireLocal && !model.IsLocal {
		penalty := ScorePenalty{Reason: "local model required", Amount: 10.0}
		score.Penalties = append(score.Penalties, penalty)
		total -= penalty.Amount
	}

	// Penalty for quality below minimum
	if criteria.MinQuality > 0 && model.QualityScore < criteria.MinQuality {
		penalty := ScorePenalty{Reason: "quality below minimum", Amount: 2.0}
		score.Penalties = append(score.Penalties, penalty)
		total -= penalty.Amount
	}

	// Penalty for warmup required (minor)
	if model.NeedsWarmup(5 * time.Minute) {
		penalty := ScorePenalty{Reason: "warmup required", Amount: 0.1}
		score.Penalties = append(score.Penalties, penalty)
		total -= penalty.Amount
	}

	score.TotalScore = total
	return score
}

// clamp restricts a value to the range [min, max].
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
