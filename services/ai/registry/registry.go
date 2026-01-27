package registry

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ModelFilter defines criteria for filtering models.
type ModelFilter struct {
	// Provider filters by model provider.
	Provider Provider
	// Capability filters by a specific capability.
	Capability Capability
	// IsLocal filters by local vs cloud models.
	IsLocal *bool
	// HealthStatus filters by health status.
	HealthStatus HealthStatus
	// Tags filters by tags (model must have all specified tags).
	Tags []string
	// MinPriority filters by minimum priority.
	MinPriority int
}

// Match checks if a model matches the filter criteria.
func (f *ModelFilter) Match(m *ModelConfig) bool {
	if f.Provider != "" && m.Provider != f.Provider {
		return false
	}
	if f.Capability != "" && !m.SupportsCapability(f.Capability) {
		return false
	}
	if f.IsLocal != nil && m.IsLocal != *f.IsLocal {
		return false
	}
	if f.HealthStatus != "" && m.Health.Status != f.HealthStatus {
		return false
	}
	if len(f.Tags) > 0 {
		tagSet := make(map[string]bool)
		for _, t := range m.Tags {
			tagSet[t] = true
		}
		for _, t := range f.Tags {
			if !tagSet[t] {
				return false
			}
		}
	}
	if f.MinPriority > 0 && m.Priority < f.MinPriority {
		return false
	}
	return true
}

// RegistryConfig holds configuration for the model registry.
type RegistryConfig struct {
	// MLXEmbeddingsURL is the base URL for the MLX embeddings server.
	MLXEmbeddingsURL string
	// MLXLLMURL is the base URL for the MLX LLM server.
	MLXLLMURL string
	// GeminiEndpoint is the base URL for the Gemini API.
	GeminiEndpoint string
	// HealthCheckInterval is how often to check model health.
	HealthCheckInterval time.Duration
	// HealthCheckTimeout is the timeout for health check requests.
	HealthCheckTimeout time.Duration
	// EnableAutoDiscovery enables automatic model discovery (disabled for MLX).
	EnableAutoDiscovery bool
	// DiscoveryInterval is how often to run discovery (if enabled).
	DiscoveryInterval time.Duration
}

// DefaultRegistryConfig returns a configuration with sensible defaults.
func DefaultRegistryConfig() *RegistryConfig {
	return &RegistryConfig{
		MLXEmbeddingsURL:    "http://localhost:8081",
		MLXLLMURL:           "http://localhost:8080",
		GeminiEndpoint:      "https://generativelanguage.googleapis.com/v1",
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  5 * time.Second,
		EnableAutoDiscovery: false, // MLX models are pre-configured
		DiscoveryInterval:   5 * time.Minute,
	}
}

// ModelRegistry manages the registration and discovery of AI models.
type ModelRegistry struct {
	mu       sync.RWMutex
	models   map[string]*ModelConfig
	config   *RegistryConfig
	client   *http.Client
	closed   bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewRegistry creates a new model registry with the given configuration.
func NewRegistry(config *RegistryConfig) *ModelRegistry {
	if config == nil {
		config = DefaultRegistryConfig()
	}

	r := &ModelRegistry{
		models:   make(map[string]*ModelConfig),
		config:   config,
		client:   &http.Client{Timeout: config.HealthCheckTimeout},
		stopChan: make(chan struct{}),
	}

	return r
}

// Start begins background tasks like health checking and discovery.
func (r *ModelRegistry) Start(ctx context.Context) error {
	// Start health checker
	if r.config.HealthCheckInterval > 0 {
		r.wg.Add(1)
		go r.healthCheckLoop(ctx)
	}

	// Start auto-discovery if enabled
	if r.config.EnableAutoDiscovery && r.config.DiscoveryInterval > 0 {
		r.wg.Add(1)
		go r.discoveryLoop(ctx)
	}

	return nil
}

// Stop halts all background tasks.
func (r *ModelRegistry) Stop() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	close(r.stopChan)
	r.mu.Unlock()

	r.wg.Wait()
}

// Register adds a new model to the registry.
func (r *ModelRegistry) Register(model *ModelConfig) error {
	if err := model.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRegistryClosed
	}

	if _, exists := r.models[model.ID]; exists {
		return ErrModelAlreadyExists
	}

	// Set timestamps
	now := time.Now()
	model.CreatedAt = now
	model.UpdatedAt = now

	// Initialize health if not set
	if model.Health.Status == "" {
		model.Health.Status = HealthStatusUnknown
	}

	r.models[model.ID] = model.Clone()
	return nil
}

// Update modifies an existing model in the registry.
func (r *ModelRegistry) Update(model *ModelConfig) error {
	if err := model.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRegistryClosed
	}

	existing, exists := r.models[model.ID]
	if !exists {
		return ErrModelNotFound
	}

	// Preserve creation time
	model.CreatedAt = existing.CreatedAt
	model.UpdatedAt = time.Now()

	r.models[model.ID] = model.Clone()
	return nil
}

// Unregister removes a model from the registry.
func (r *ModelRegistry) Unregister(modelID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRegistryClosed
	}

	if _, exists := r.models[modelID]; !exists {
		return ErrModelNotFound
	}

	delete(r.models, modelID)
	return nil
}

// Get retrieves a model by its ID.
func (r *ModelRegistry) Get(modelID string) (*ModelConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrRegistryClosed
	}

	model, exists := r.models[modelID]
	if !exists {
		return nil, ErrModelNotFound
	}

	return model.Clone(), nil
}

// List returns all models matching the optional filter.
func (r *ModelRegistry) List(filter *ModelFilter) []*ModelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ModelConfig, 0, len(r.models))
	for _, model := range r.models {
		if filter == nil || filter.Match(model) {
			result = append(result, model.Clone())
		}
	}

	return result
}

// ListByCapability returns all models that support the given capability.
func (r *ModelRegistry) ListByCapability(cap Capability) []*ModelConfig {
	return r.List(&ModelFilter{Capability: cap})
}

// ListByProvider returns all models from the given provider.
func (r *ModelRegistry) ListByProvider(provider Provider) []*ModelConfig {
	return r.List(&ModelFilter{Provider: provider})
}

// ListAvailable returns all models that are healthy and available.
func (r *ModelRegistry) ListAvailable() []*ModelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ModelConfig, 0)
	for _, model := range r.models {
		if model.Health.IsAvailable() {
			result = append(result, model.Clone())
		}
	}

	return result
}

// GetDefaultModel returns the default model for a capability.
func (r *ModelRegistry) GetDefaultModel(cap Capability) (*ModelConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best *ModelConfig
	for _, model := range r.models {
		if !model.SupportsCapability(cap) {
			continue
		}
		if model.IsDefault && model.Health.IsAvailable() {
			return model.Clone(), nil
		}
		// Track highest priority available model as fallback
		if model.Health.IsAvailable() {
			if best == nil || model.Priority > best.Priority {
				best = model
			}
		}
	}

	if best != nil {
		return best.Clone(), nil
	}

	return nil, ErrModelNotFound
}

// SelectModel finds the best available model matching the criteria.
func (r *ModelRegistry) SelectModel(cap Capability, preferLocal bool) (*ModelConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best *ModelConfig
	for _, model := range r.models {
		if !model.SupportsCapability(cap) || !model.Health.IsAvailable() {
			continue
		}

		if best == nil {
			best = model
			continue
		}

		// Prefer local models if requested
		if preferLocal && model.IsLocal && !best.IsLocal {
			best = model
			continue
		}
		if preferLocal && !model.IsLocal && best.IsLocal {
			continue
		}

		// Otherwise prefer higher priority
		if model.Priority > best.Priority {
			best = model
		}
	}

	if best != nil {
		return best.Clone(), nil
	}

	return nil, ErrModelNotFound
}

// UpdateHealth updates the health status of a model.
func (r *ModelRegistry) UpdateHealth(modelID string, health ModelHealth) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	model, exists := r.models[modelID]
	if !exists {
		return ErrModelNotFound
	}

	model.Health = health
	model.UpdatedAt = time.Now()
	return nil
}

// RecordRequest records a successful or failed request for a model.
func (r *ModelRegistry) RecordRequest(modelID string, latencyMs float64, success bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	model, exists := r.models[modelID]
	if !exists {
		return ErrModelNotFound
	}

	model.Health.RequestCount++
	if !success {
		model.Health.ErrorCount++
	} else {
		// Update rolling average latency
		if model.Health.AvgLatencyMs == 0 {
			model.Health.AvgLatencyMs = latencyMs
		} else {
			// Exponential moving average
			model.Health.AvgLatencyMs = 0.9*model.Health.AvgLatencyMs + 0.1*latencyMs
		}
		model.Health.LastSuccess = time.Now()
	}
	model.UpdatedAt = time.Now()

	return nil
}

// Count returns the total number of registered models.
func (r *ModelRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.models)
}

// healthCheckLoop periodically checks the health of all models.
func (r *ModelRegistry) healthCheckLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.HealthCheckInterval)
	defer ticker.Stop()

	// Run initial check
	r.checkAllHealth(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopChan:
			return
		case <-ticker.C:
			r.checkAllHealth(ctx)
		}
	}
}

// checkAllHealth checks the health of all registered models.
func (r *ModelRegistry) checkAllHealth(ctx context.Context) {
	r.mu.RLock()
	models := make([]*ModelConfig, 0, len(r.models))
	for _, m := range r.models {
		models = append(models, m)
	}
	r.mu.RUnlock()

	for _, model := range models {
		select {
		case <-ctx.Done():
			return
		case <-r.stopChan:
			return
		default:
		}

		health := r.checkModelHealth(ctx, model)

		r.mu.Lock()
		if m, exists := r.models[model.ID]; exists {
			// Preserve counters
			health.RequestCount = m.Health.RequestCount
			health.ErrorCount = m.Health.ErrorCount
			if health.Status == HealthStatusHealthy {
				health.ConsecutiveFailures = 0
			} else {
				health.ConsecutiveFailures = m.Health.ConsecutiveFailures + 1
			}
			m.Health = health
			m.UpdatedAt = time.Now()
		}
		r.mu.Unlock()
	}
}

// checkModelHealth performs a health check on a single model.
func (r *ModelRegistry) checkModelHealth(ctx context.Context, model *ModelConfig) ModelHealth {
	health := ModelHealth{
		LastCheck: time.Now(),
		Status:    HealthStatusUnknown,
	}

	switch model.Provider {
	case ProviderMLX:
		health = r.checkMLXHealth(ctx, model)
	case ProviderGemini:
		// Gemini health is checked via API availability
		health.Status = HealthStatusHealthy
		health.LastCheck = time.Now()
		health.LastSuccess = time.Now()
	default:
		health.Status = HealthStatusUnknown
	}

	return health
}

// checkMLXHealth checks if an MLX model server is available.
func (r *ModelRegistry) checkMLXHealth(ctx context.Context, model *ModelConfig) ModelHealth {
	health := ModelHealth{
		LastCheck: time.Now(),
		Status:    HealthStatusUnknown,
	}

	endpoint := model.Endpoint
	if endpoint == "" {
		// Use appropriate MLX endpoint based on capability
		if model.SupportsCapability(CapabilityEmbedding) {
			endpoint = r.config.MLXEmbeddingsURL
		} else {
			endpoint = r.config.MLXLLMURL
		}
	}

	// Check MLX server health via /health or /v1/models endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"/health", nil)
	if err != nil {
		health.Status = HealthStatusUnhealthy
		health.ErrorMessage = err.Error()
		return health
	}

	start := time.Now()
	resp, err := r.client.Do(req)
	if err != nil {
		health.Status = HealthStatusUnhealthy
		health.ErrorMessage = err.Error()
		return health
	}
	defer func() { _ = resp.Body.Close() }()

	health.AvgLatencyMs = float64(time.Since(start).Milliseconds())

	if resp.StatusCode == http.StatusOK {
		health.Status = HealthStatusHealthy
		health.LastSuccess = time.Now()
		return health
	}

	health.Status = HealthStatusUnhealthy
	health.ErrorMessage = fmt.Sprintf("MLX server returned status code: %d", resp.StatusCode)
	return health
}

// discoveryLoop periodically discovers new models from providers.
// Note: MLX models are pre-configured, so discovery is a no-op for MLX.
func (r *ModelRegistry) discoveryLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopChan:
			return
		case <-ticker.C:
			// MLX models are pre-configured, no dynamic discovery needed
		}
	}
}

// DiscoverModels manually triggers model discovery.
// Note: MLX models are pre-configured, so this is a no-op.
func (r *ModelRegistry) DiscoverModels(_ context.Context) error {
	// MLX models are pre-configured via defaults, no dynamic discovery
	return nil
}
