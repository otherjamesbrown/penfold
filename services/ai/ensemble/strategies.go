package ensemble

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// StrategyType identifies the ensemble strategy to use.
type StrategyType string

const (
	// StrategyMajorityVoting selects the most common answer across models.
	StrategyMajorityVoting StrategyType = "majority_voting"

	// StrategyWeightedVoting weights responses by model confidence and quality.
	StrategyWeightedVoting StrategyType = "weighted_voting"

	// StrategyCascade tries models in order until one succeeds with sufficient confidence.
	StrategyCascade StrategyType = "cascade"

	// StrategyParallel queries all models in parallel and combines results.
	StrategyParallel StrategyType = "parallel"

	// StrategyDebate has models critique each other's responses iteratively.
	StrategyDebate StrategyType = "debate"
)

// Strategy defines the interface for ensemble strategies.
type Strategy interface {
	// Name returns the strategy name.
	Name() StrategyType

	// Execute runs the ensemble strategy and returns the result.
	Execute(ctx context.Context, req *EnsembleRequest, models []ModelBackend,
		orchestrator *Orchestrator, aggregator *Aggregator) (*EnsembleResult, error)
}

// MajorityVotingStrategy selects the response that appears most frequently.
type MajorityVotingStrategy struct{}

// NewMajorityVotingStrategy creates a new majority voting strategy.
func NewMajorityVotingStrategy() *MajorityVotingStrategy {
	return &MajorityVotingStrategy{}
}

// Name returns the strategy name.
func (s *MajorityVotingStrategy) Name() StrategyType {
	return StrategyMajorityVoting
}

// Execute implements the majority voting strategy.
func (s *MajorityVotingStrategy) Execute(ctx context.Context, req *EnsembleRequest,
	models []ModelBackend, orchestrator *Orchestrator, aggregator *Aggregator) (*EnsembleResult, error) {

	// Query all models in parallel
	results, err := orchestrator.ExecuteParallel(ctx, req, models)
	if err != nil {
		return nil, err
	}

	// Filter successful results
	successResults := filterSuccessfulResults(results)
	if len(successResults) < req.MinResponses {
		return nil, fmt.Errorf("%w: got %d, need %d", ErrInsufficientQuorum, len(successResults), req.MinResponses)
	}

	// Aggregate using majority voting
	result := aggregator.AggregateMajorityVoting(successResults)
	result.IndividualResults = results

	// Check consensus requirement
	if req.RequireConsensus && result.ConsensusScore < req.ConsensusThreshold {
		result.Explanation = fmt.Sprintf("Consensus not reached: %.2f < %.2f threshold",
			result.ConsensusScore, req.ConsensusThreshold)
	}

	return result, nil
}

// WeightedVotingStrategy weights responses by confidence and model quality.
type WeightedVotingStrategy struct{}

// NewWeightedVotingStrategy creates a new weighted voting strategy.
func NewWeightedVotingStrategy() *WeightedVotingStrategy {
	return &WeightedVotingStrategy{}
}

// Name returns the strategy name.
func (s *WeightedVotingStrategy) Name() StrategyType {
	return StrategyWeightedVoting
}

// Execute implements the weighted voting strategy.
func (s *WeightedVotingStrategy) Execute(ctx context.Context, req *EnsembleRequest,
	models []ModelBackend, orchestrator *Orchestrator, aggregator *Aggregator) (*EnsembleResult, error) {

	// Query all models in parallel
	results, err := orchestrator.ExecuteParallel(ctx, req, models)
	if err != nil {
		return nil, err
	}

	// Filter successful results
	successResults := filterSuccessfulResults(results)
	if len(successResults) < req.MinResponses {
		return nil, fmt.Errorf("%w: got %d, need %d", ErrInsufficientQuorum, len(successResults), req.MinResponses)
	}

	// Build model quality map
	modelQualities := make(map[string]float64)
	for _, m := range models {
		modelQualities[m.Name()] = m.QualityScore()
	}

	// Aggregate using weighted voting
	result := aggregator.AggregateWeightedVoting(successResults, modelQualities)
	result.IndividualResults = results

	return result, nil
}

// CascadeStrategy tries models in priority order until success.
type CascadeStrategy struct{}

// NewCascadeStrategy creates a new cascade strategy.
func NewCascadeStrategy() *CascadeStrategy {
	return &CascadeStrategy{}
}

// Name returns the strategy name.
func (s *CascadeStrategy) Name() StrategyType {
	return StrategyCascade
}

// Execute implements the cascade strategy.
func (s *CascadeStrategy) Execute(ctx context.Context, req *EnsembleRequest,
	models []ModelBackend, orchestrator *Orchestrator, aggregator *Aggregator) (*EnsembleResult, error) {

	// Sort models by quality score (highest first)
	sortedModels := make([]ModelBackend, len(models))
	copy(sortedModels, models)
	sort.Slice(sortedModels, func(i, j int) bool {
		return sortedModels[i].QualityScore() > sortedModels[j].QualityScore()
	})

	// Get minimum confidence threshold from options
	minConfidence := 0.7
	if req.Options != nil {
		if mc, ok := req.Options["min_confidence"].(float64); ok {
			minConfidence = mc
		}
	}

	var allResults []ModelResult

	// Try models in order until we get a confident result
	for _, model := range sortedModels {
		select {
		case <-ctx.Done():
			return nil, ErrContextCanceled
		default:
		}

		result, err := orchestrator.ExecuteSingle(ctx, req, model)
		allResults = append(allResults, *result)

		if err != nil {
			continue
		}

		if result.IsSuccess() && result.Confidence >= minConfidence {
			return &EnsembleResult{
				FinalResponse:     result.Response,
				SelectedModel:     result.ModelID,
				ConsensusScore:    1.0, // Single model, full confidence
				IndividualResults: allResults,
				TotalTokensUsed:   result.TokensUsed,
				Explanation:       fmt.Sprintf("Model %s selected via cascade (confidence: %.2f)", result.ModelID, result.Confidence),
			}, nil
		}
	}

	// No model met the threshold, use the best available result
	if len(allResults) > 0 {
		bestResult := selectBestResult(allResults)
		if bestResult != nil && bestResult.IsSuccess() {
			return &EnsembleResult{
				FinalResponse:     bestResult.Response,
				SelectedModel:     bestResult.ModelID,
				ConsensusScore:    bestResult.Confidence,
				IndividualResults: allResults,
				TotalTokensUsed:   bestResult.TokensUsed,
				Explanation:       fmt.Sprintf("Best available model %s (confidence: %.2f, below threshold)", bestResult.ModelID, bestResult.Confidence),
			}, nil
		}
	}

	return nil, ErrNoResults
}

// ParallelStrategy queries all models in parallel and combines results.
type ParallelStrategy struct{}

// NewParallelStrategy creates a new parallel strategy.
func NewParallelStrategy() *ParallelStrategy {
	return &ParallelStrategy{}
}

// Name returns the strategy name.
func (s *ParallelStrategy) Name() StrategyType {
	return StrategyParallel
}

// Execute implements the parallel strategy.
func (s *ParallelStrategy) Execute(ctx context.Context, req *EnsembleRequest,
	models []ModelBackend, orchestrator *Orchestrator, aggregator *Aggregator) (*EnsembleResult, error) {

	// Query all models in parallel
	results, err := orchestrator.ExecuteParallel(ctx, req, models)
	if err != nil {
		return nil, err
	}

	// Filter successful results
	successResults := filterSuccessfulResults(results)
	if len(successResults) < req.MinResponses {
		return nil, fmt.Errorf("%w: got %d, need %d", ErrInsufficientQuorum, len(successResults), req.MinResponses)
	}

	// Build model quality map
	modelQualities := make(map[string]float64)
	for _, m := range models {
		modelQualities[m.Name()] = m.QualityScore()
	}

	// Aggregate results - use weighted voting for parallel
	result := aggregator.AggregateParallel(successResults, modelQualities)
	result.IndividualResults = results

	return result, nil
}

// DebateStrategy has models critique each other's responses iteratively.
type DebateStrategy struct {
	logger    logging.Logger
	maxRounds int
}

// NewDebateStrategy creates a new debate strategy.
func NewDebateStrategy(logger logging.Logger) *DebateStrategy {
	return &DebateStrategy{
		logger:    logger,
		maxRounds: 3,
	}
}

// Name returns the strategy name.
func (s *DebateStrategy) Name() StrategyType {
	return StrategyDebate
}

// Execute implements the debate strategy.
func (s *DebateStrategy) Execute(ctx context.Context, req *EnsembleRequest,
	models []ModelBackend, orchestrator *Orchestrator, aggregator *Aggregator) (*EnsembleResult, error) {

	if len(models) < 2 {
		// Need at least 2 models for a debate
		return nil, errors.New("debate strategy requires at least 2 models")
	}

	// Get debate configuration from options
	maxRounds := s.maxRounds
	if req.Options != nil {
		if mr, ok := req.Options["max_rounds"].(int); ok && mr > 0 {
			maxRounds = mr
		}
	}

	// Initial round: all models provide their response
	initialResults, err := orchestrator.ExecuteParallel(ctx, req, models)
	if err != nil {
		return nil, err
	}

	successResults := filterSuccessfulResults(initialResults)
	if len(successResults) < 2 {
		return nil, fmt.Errorf("%w: debate requires at least 2 successful responses", ErrInsufficientQuorum)
	}

	// Track all results across rounds
	allResults := make([]ModelResult, len(initialResults))
	copy(allResults, initialResults)

	// Current round results
	currentResults := successResults

	// Debate rounds: models critique and potentially revise
	for round := 1; round <= maxRounds; round++ {
		select {
		case <-ctx.Done():
			return nil, ErrContextCanceled
		default:
		}

		// Check for convergence - if all models agree, stop
		consensusScore := aggregator.CalculateConsensusScore(currentResults)
		if consensusScore >= req.ConsensusThreshold {
			s.logger.Debug("Debate reached consensus",
				logging.F("round", round),
				logging.F("consensus_score", consensusScore),
			)
			break
		}

		// Create critique requests
		critiqueReq := &EnsembleRequest{
			ID:       fmt.Sprintf("%s-debate-r%d", req.ID, round),
			Type:     req.Type,
			Content:  buildCritiquePrompt(req.Content, currentResults),
			Timeout:  req.Timeout,
			Options:  req.Options,
			Metadata: req.Metadata,
		}

		// Get critiques from all models
		critiqueResults, err := orchestrator.ExecuteParallel(ctx, critiqueReq, models)
		if err != nil {
			s.logger.Warn("Debate round failed",
				logging.F("round", round),
				logging.Err(err),
			)
			break
		}

		// Update current results with successful critiques
		successCritiques := filterSuccessfulResults(critiqueResults)
		if len(successCritiques) >= 2 {
			currentResults = successCritiques
			allResults = append(allResults, critiqueResults...)
		}
	}

	// Build model quality map
	modelQualities := make(map[string]float64)
	for _, m := range models {
		modelQualities[m.Name()] = m.QualityScore()
	}

	// Final aggregation
	result := aggregator.AggregateDebate(currentResults, modelQualities)
	result.IndividualResults = allResults
	result.Explanation = fmt.Sprintf("Debate completed with %d models over %d rounds",
		len(models), min(maxRounds, len(allResults)/len(models)))

	return result, nil
}

// buildCritiquePrompt creates a prompt for the critique round.
func buildCritiquePrompt(originalContent string, previousResults []ModelResult) string {
	prompt := fmt.Sprintf("Original question/content:\n%s\n\nPrevious responses from different models:\n", originalContent)

	for i, r := range previousResults {
		if r.IsSuccess() {
			prompt += fmt.Sprintf("\nModel %d response:\n%v\n", i+1, r.Response)
		}
	}

	prompt += "\nPlease review these responses and provide your best answer, considering any points of disagreement."

	return prompt
}

// filterSuccessfulResults returns only results with no errors.
func filterSuccessfulResults(results []ModelResult) []ModelResult {
	var successful []ModelResult
	for _, r := range results {
		if r.IsSuccess() {
			successful = append(successful, r)
		}
	}
	return successful
}

// selectBestResult returns the result with highest confidence.
func selectBestResult(results []ModelResult) *ModelResult {
	var best *ModelResult
	bestConfidence := -1.0

	for i := range results {
		if results[i].IsSuccess() && results[i].Confidence > bestConfidence {
			best = &results[i]
			bestConfidence = results[i].Confidence
		}
	}

	return best
}

// CostAwareStrategy selects models based on cost constraints.
type CostAwareStrategy struct {
	maxCost float64
}

// NewCostAwareStrategy creates a cost-aware strategy wrapper.
func NewCostAwareStrategy(maxCost float64) *CostAwareStrategy {
	return &CostAwareStrategy{maxCost: maxCost}
}

// SelectModels filters models based on cost constraints.
func (s *CostAwareStrategy) SelectModels(models []ModelBackend, estimatedTokens int) []ModelBackend {
	if s.maxCost <= 0 {
		return models
	}

	var selected []ModelBackend
	totalCost := 0.0

	// Sort by cost (lowest first)
	sortedModels := make([]ModelBackend, len(models))
	copy(sortedModels, models)
	sort.Slice(sortedModels, func(i, j int) bool {
		return sortedModels[i].CostPer1KTokens() < sortedModels[j].CostPer1KTokens()
	})

	for _, m := range sortedModels {
		estimatedCost := float64(estimatedTokens) / 1000.0 * m.CostPer1KTokens()
		if totalCost+estimatedCost <= s.maxCost {
			selected = append(selected, m)
			totalCost += estimatedCost
		}
	}

	return selected
}

// StrategyChain allows composing multiple strategies.
type StrategyChain struct {
	strategies []Strategy
	mu         sync.RWMutex
}

// NewStrategyChain creates a new strategy chain.
func NewStrategyChain(strategies ...Strategy) *StrategyChain {
	return &StrategyChain{
		strategies: strategies,
	}
}

// Name returns a composite name.
func (s *StrategyChain) Name() StrategyType {
	return StrategyType("chain")
}

// Execute runs strategies in sequence, using fallback on failure.
func (s *StrategyChain) Execute(ctx context.Context, req *EnsembleRequest,
	models []ModelBackend, orchestrator *Orchestrator, aggregator *Aggregator) (*EnsembleResult, error) {

	s.mu.RLock()
	strategies := make([]Strategy, len(s.strategies))
	copy(strategies, s.strategies)
	s.mu.RUnlock()

	var lastErr error

	for _, strategy := range strategies {
		select {
		case <-ctx.Done():
			return nil, ErrContextCanceled
		default:
		}

		result, err := strategy.Execute(ctx, req, models, orchestrator, aggregator)
		if err == nil {
			return result, nil
		}

		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all strategies failed: %w", lastErr)
	}

	return nil, ErrNoResults
}

// AddStrategy adds a strategy to the chain.
func (s *StrategyChain) AddStrategy(strategy Strategy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strategies = append(s.strategies, strategy)
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
