package ensemble

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// OrchestratorConfig holds configuration for the Orchestrator.
type OrchestratorConfig struct {
	// DefaultTimeout is the default timeout for requests.
	DefaultTimeout time.Duration

	// MaxConcurrentModels is the maximum models to query in parallel.
	MaxConcurrentModels int

	// RetryFailedModels enables retrying individual model failures.
	RetryFailedModels bool

	// MaxRetries is the maximum retries per model.
	MaxRetries int

	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration

	// BackoffMultiplier increases delay for each retry.
	BackoffMultiplier float64
}

// DefaultOrchestratorConfig returns a configuration with sensible defaults.
func DefaultOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		DefaultTimeout:      30 * time.Second,
		MaxConcurrentModels: 5,
		RetryFailedModels:   true,
		MaxRetries:          2,
		RetryDelay:          100 * time.Millisecond,
		BackoffMultiplier:   2.0,
	}
}

// Orchestrator handles request orchestration across multiple models.
type Orchestrator struct {
	mu     sync.RWMutex
	config *OrchestratorConfig
	logger logging.Logger

	// semaphore limits concurrent model requests
	semaphore chan struct{}
}

// NewOrchestrator creates a new Orchestrator with the given configuration.
func NewOrchestrator(cfg *OrchestratorConfig, logger logging.Logger) *Orchestrator {
	if cfg == nil {
		cfg = DefaultOrchestratorConfig()
	}

	if logger == nil {
		logger = logging.MustGlobal()
	}

	o := &Orchestrator{
		config:    cfg,
		logger:    logger.With(logging.F("component", "orchestrator")),
		semaphore: make(chan struct{}, cfg.MaxConcurrentModels),
	}

	return o
}

// UpdateConfig updates the orchestrator configuration.
func (o *Orchestrator) UpdateConfig(cfg *OrchestratorConfig) {
	if cfg == nil {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	o.config = cfg

	// Recreate semaphore if concurrency changed
	o.semaphore = make(chan struct{}, cfg.MaxConcurrentModels)
}

// ExecuteParallel executes the request across all models in parallel.
func (o *Orchestrator) ExecuteParallel(ctx context.Context, req *EnsembleRequest, models []ModelBackend) ([]ModelResult, error) {
	if len(models) == 0 {
		return nil, ErrNoModels
	}

	// Apply timeout from request or config
	timeout := req.Timeout
	if timeout <= 0 {
		o.mu.RLock()
		timeout = o.config.DefaultTimeout
		o.mu.RUnlock()
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create result channel
	resultChan := make(chan ModelResult, len(models))
	var wg sync.WaitGroup

	// Launch goroutines for each model
	for _, model := range models {
		wg.Add(1)
		go func(m ModelBackend) {
			defer wg.Done()
			result := o.executeWithRetry(ctx, req, m)
			resultChan <- result
		}(model)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	var results []ModelResult
	for result := range resultChan {
		results = append(results, result)
	}

	// Check if we got any results
	if len(results) == 0 {
		return nil, ErrNoResults
	}

	return results, nil
}

// ExecuteSingle executes the request on a single model.
func (o *Orchestrator) ExecuteSingle(ctx context.Context, req *EnsembleRequest, model ModelBackend) (*ModelResult, error) {
	// Apply timeout from request or config
	timeout := req.Timeout
	if timeout <= 0 {
		o.mu.RLock()
		timeout = o.config.DefaultTimeout
		o.mu.RUnlock()
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := o.executeWithRetry(ctx, req, model)
	return &result, result.Error
}

// ExecuteSequential executes the request on models in sequence.
func (o *Orchestrator) ExecuteSequential(ctx context.Context, req *EnsembleRequest, models []ModelBackend) ([]ModelResult, error) {
	if len(models) == 0 {
		return nil, ErrNoModels
	}

	var results []ModelResult

	for _, model := range models {
		select {
		case <-ctx.Done():
			return results, ErrContextCanceled
		default:
		}

		result := o.executeWithRetry(ctx, req, model)
		results = append(results, result)
	}

	return results, nil
}

// ExecuteWithLimit executes with a limit on concurrent models.
func (o *Orchestrator) ExecuteWithLimit(ctx context.Context, req *EnsembleRequest, models []ModelBackend, limit int) ([]ModelResult, error) {
	if len(models) == 0 {
		return nil, ErrNoModels
	}

	if limit <= 0 {
		o.mu.RLock()
		limit = o.config.MaxConcurrentModels
		o.mu.RUnlock()
	}

	// Create a semaphore for limiting concurrency
	sem := make(chan struct{}, limit)

	// Apply timeout
	timeout := req.Timeout
	if timeout <= 0 {
		o.mu.RLock()
		timeout = o.config.DefaultTimeout
		o.mu.RUnlock()
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultChan := make(chan ModelResult, len(models))
	var wg sync.WaitGroup

	for _, model := range models {
		wg.Add(1)
		go func(m ModelBackend) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultChan <- ModelResult{
					ModelID: m.Name(),
					Error:   ErrContextCanceled,
				}
				return
			}

			result := o.executeWithRetry(ctx, req, m)
			resultChan <- result
		}(model)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var results []ModelResult
	for result := range resultChan {
		results = append(results, result)
	}

	return results, nil
}

// ExecuteCostAware executes with cost constraints.
func (o *Orchestrator) ExecuteCostAware(ctx context.Context, req *EnsembleRequest, models []ModelBackend, maxCost float64) ([]ModelResult, error) {
	if len(models) == 0 {
		return nil, ErrNoModels
	}

	// Estimate tokens for cost calculation
	estimatedTokens := len(req.Content) / 4 // Rough estimate: 4 chars per token

	// Filter models by cost
	var affordableModels []ModelBackend
	totalCost := 0.0

	// Sort by cost (lowest first)
	sortedModels := make([]ModelBackend, len(models))
	copy(sortedModels, models)
	sort.Slice(sortedModels, func(i, j int) bool {
		return sortedModels[i].CostPer1KTokens() < sortedModels[j].CostPer1KTokens()
	})

	for _, m := range sortedModels {
		estimatedCost := float64(estimatedTokens) / 1000.0 * m.CostPer1KTokens()
		if totalCost+estimatedCost <= maxCost {
			affordableModels = append(affordableModels, m)
			totalCost += estimatedCost
		}
	}

	if len(affordableModels) == 0 {
		o.logger.Warn("No models within cost budget",
			logging.F("max_cost", maxCost),
			logging.F("estimated_tokens", estimatedTokens),
		)
		return nil, ErrNoModels
	}

	return o.ExecuteParallel(ctx, req, affordableModels)
}

// executeWithRetry executes a single model request with retry logic.
func (o *Orchestrator) executeWithRetry(ctx context.Context, req *EnsembleRequest, model ModelBackend) ModelResult {
	o.mu.RLock()
	retryEnabled := o.config.RetryFailedModels
	maxRetries := o.config.MaxRetries
	retryDelay := o.config.RetryDelay
	backoffMultiplier := o.config.BackoffMultiplier
	o.mu.RUnlock()

	var lastErr error
	attempts := 0

	for attempts <= maxRetries {
		select {
		case <-ctx.Done():
			return ModelResult{
				ModelID:  model.Name(),
				Provider: model.Provider(),
				Error:    ErrContextCanceled,
			}
		default:
		}

		// Acquire semaphore
		select {
		case o.semaphore <- struct{}{}:
		case <-ctx.Done():
			return ModelResult{
				ModelID:  model.Name(),
				Provider: model.Provider(),
				Error:    ErrContextCanceled,
			}
		}

		startTime := time.Now()
		result, err := model.Process(ctx, req.Content, req.Type, req.Options)

		// Release semaphore
		<-o.semaphore

		if err == nil && result != nil {
			result.ModelID = model.Name()
			result.Provider = model.Provider()
			result.ProcessingTime = time.Since(startTime)
			return *result
		}

		lastErr = err
		if err != nil {
			o.logger.Debug("Model request failed",
				logging.F("model_id", model.Name()),
				logging.F("attempt", attempts+1),
				logging.Err(err),
			)
		}

		// Don't retry if retries are disabled
		if !retryEnabled {
			break
		}

		attempts++

		// Don't wait before the last attempt (which won't happen)
		if attempts <= maxRetries {
			// Calculate backoff delay
			delay := retryDelay
			for i := 1; i < attempts; i++ {
				delay = time.Duration(float64(delay) * backoffMultiplier)
			}

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ModelResult{
					ModelID:  model.Name(),
					Provider: model.Provider(),
					Error:    ErrContextCanceled,
				}
			}
		}
	}

	// Return error result
	return ModelResult{
		ModelID:  model.Name(),
		Provider: model.Provider(),
		Error:    lastErr,
	}
}

// PartialResultHandler handles partial results when not all models respond.
type PartialResultHandler struct {
	minResponses int
	logger       logging.Logger
}

// NewPartialResultHandler creates a new partial result handler.
func NewPartialResultHandler(minResponses int, logger logging.Logger) *PartialResultHandler {
	return &PartialResultHandler{
		minResponses: minResponses,
		logger:       logger,
	}
}

// HandlePartialResults processes partial results when timeout occurs.
func (h *PartialResultHandler) HandlePartialResults(results []ModelResult, timeout time.Duration) ([]ModelResult, error) {
	successCount := 0
	for _, r := range results {
		if r.IsSuccess() {
			successCount++
		}
	}

	h.logger.Warn("Partial results received",
		logging.F("total_results", len(results)),
		logging.F("successful", successCount),
		logging.F("timeout", timeout),
	)

	if successCount < h.minResponses {
		return results, fmt.Errorf("%w: got %d successful, need %d", ErrInsufficientQuorum, successCount, h.minResponses)
	}

	return results, nil
}

// ModelPrioritizer prioritizes models for execution order.
type ModelPrioritizer struct {
	criteria ModelSortCriteria
}

// NewModelPrioritizer creates a new model prioritizer.
func NewModelPrioritizer(criteria ModelSortCriteria) *ModelPrioritizer {
	return &ModelPrioritizer{criteria: criteria}
}

// Prioritize sorts models according to the criteria.
func (p *ModelPrioritizer) Prioritize(models []ModelBackend) []ModelBackend {
	sorted := make([]ModelBackend, len(models))
	copy(sorted, models)

	switch p.criteria {
	case SortByQuality:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].QualityScore() > sorted[j].QualityScore()
		})
	case SortByCost:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].CostPer1KTokens() < sorted[j].CostPer1KTokens()
		})
	case SortByLocal:
		sort.Slice(sorted, func(i, j int) bool {
			// Local models first
			if sorted[i].IsLocal() != sorted[j].IsLocal() {
				return sorted[i].IsLocal()
			}
			return sorted[i].QualityScore() > sorted[j].QualityScore()
		})
	}

	return sorted
}

// ExecutionPlan represents a planned execution order.
type ExecutionPlan struct {
	// Phases are groups of models to execute together.
	Phases []ExecutionPhase

	// TotalModels is the total number of models in the plan.
	TotalModels int

	// EstimatedCost is the estimated total cost.
	EstimatedCost float64

	// EstimatedDuration is the estimated total duration.
	EstimatedDuration time.Duration
}

// ExecutionPhase represents a phase of model execution.
type ExecutionPhase struct {
	// Models to execute in this phase.
	Models []ModelBackend

	// Parallel indicates if models should run in parallel.
	Parallel bool

	// StopOnSuccess stops the plan if any model succeeds.
	StopOnSuccess bool

	// MinSuccess is the minimum successful responses needed to proceed.
	MinSuccess int
}

// ExecutionPlanner creates execution plans for ensemble requests.
type ExecutionPlanner struct {
	logger logging.Logger
}

// NewExecutionPlanner creates a new execution planner.
func NewExecutionPlanner(logger logging.Logger) *ExecutionPlanner {
	return &ExecutionPlanner{logger: logger}
}

// PlanExecution creates an execution plan for the request.
func (p *ExecutionPlanner) PlanExecution(req *EnsembleRequest, models []ModelBackend, strategy StrategyType) *ExecutionPlan {
	plan := &ExecutionPlan{
		TotalModels: len(models),
	}

	estimatedTokens := len(req.Content) / 4

	switch strategy {
	case StrategyCascade:
		// Cascade: sequential phases, stop on success
		prioritizer := NewModelPrioritizer(SortByQuality)
		sorted := prioritizer.Prioritize(models)

		for _, m := range sorted {
			plan.Phases = append(plan.Phases, ExecutionPhase{
				Models:        []ModelBackend{m},
				Parallel:      false,
				StopOnSuccess: true,
				MinSuccess:    1,
			})
			plan.EstimatedCost += float64(estimatedTokens) / 1000.0 * m.CostPer1KTokens()
		}

	case StrategyParallel, StrategyMajorityVoting, StrategyWeightedVoting:
		// Parallel: all models at once
		plan.Phases = append(plan.Phases, ExecutionPhase{
			Models:        models,
			Parallel:      true,
			StopOnSuccess: false,
			MinSuccess:    req.MinResponses,
		})

		for _, m := range models {
			plan.EstimatedCost += float64(estimatedTokens) / 1000.0 * m.CostPer1KTokens()
		}

	case StrategyDebate:
		// Debate: multiple parallel rounds
		numRounds := 3
		if req.Options != nil {
			if mr, ok := req.Options["max_rounds"].(int); ok {
				numRounds = mr
			}
		}

		for round := 0; round < numRounds; round++ {
			plan.Phases = append(plan.Phases, ExecutionPhase{
				Models:        models,
				Parallel:      true,
				StopOnSuccess: false,
				MinSuccess:    2,
			})
		}

		for _, m := range models {
			plan.EstimatedCost += float64(estimatedTokens*numRounds) / 1000.0 * m.CostPer1KTokens()
		}
	}

	// Estimate duration based on phases
	// This is a rough estimate; actual duration depends on model latencies
	for _, phase := range plan.Phases {
		if phase.Parallel {
			plan.EstimatedDuration += 2 * time.Second // Parallel phase estimate
		} else {
			plan.EstimatedDuration += time.Duration(len(phase.Models)) * 2 * time.Second
		}
	}

	return plan
}

// ExecutePlan executes an execution plan.
func (o *Orchestrator) ExecutePlan(ctx context.Context, req *EnsembleRequest, plan *ExecutionPlan) ([]ModelResult, error) {
	var allResults []ModelResult

	for i, phase := range plan.Phases {
		select {
		case <-ctx.Done():
			return allResults, ErrContextCanceled
		default:
		}

		o.logger.Debug("Executing phase",
			logging.F("phase", i+1),
			logging.F("models", len(phase.Models)),
			logging.F("parallel", phase.Parallel),
		)

		var phaseResults []ModelResult
		var err error

		if phase.Parallel {
			phaseResults, err = o.ExecuteParallel(ctx, req, phase.Models)
		} else {
			phaseResults, err = o.ExecuteSequential(ctx, req, phase.Models)
		}

		if err != nil && len(phaseResults) == 0 {
			return allResults, err
		}

		allResults = append(allResults, phaseResults...)

		// Check stop conditions
		if phase.StopOnSuccess {
			for _, r := range phaseResults {
				if r.IsSuccess() {
					return allResults, nil
				}
			}
		}

		// Check minimum success requirement
		successCount := 0
		for _, r := range phaseResults {
			if r.IsSuccess() {
				successCount++
			}
		}

		if successCount < phase.MinSuccess {
			o.logger.Warn("Phase did not meet minimum success requirement",
				logging.F("phase", i+1),
				logging.F("success_count", successCount),
				logging.F("required", phase.MinSuccess),
			)
		}
	}

	return allResults, nil
}
