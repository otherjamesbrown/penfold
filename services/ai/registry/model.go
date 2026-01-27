// Package registry provides model registration and discovery for the AI Coordinator.
// It manages configurations for both local (MLX) and cloud (Gemini) AI models.
package registry

import (
	"time"
)

// Provider identifies the AI model provider.
type Provider string

const (
	// ProviderMLX is the MLX local model provider (vllm-mlx).
	ProviderMLX Provider = "mlx"
	// ProviderGemini is the Google Gemini cloud provider.
	ProviderGemini Provider = "gemini"
	// ProviderOpenAI is the OpenAI cloud provider.
	ProviderOpenAI Provider = "openai"
	// ProviderAnthropic is the Anthropic cloud provider.
	ProviderAnthropic Provider = "anthropic"
)

// Capability represents a specific AI capability that a model supports.
type Capability string

const (
	// CapabilityEmbedding indicates the model can generate vector embeddings.
	CapabilityEmbedding Capability = "embedding"
	// CapabilityChat indicates the model supports conversational interactions.
	CapabilityChat Capability = "chat"
	// CapabilityCompletion indicates the model supports text completion.
	CapabilityCompletion Capability = "completion"
	// CapabilityVision indicates the model can process images.
	CapabilityVision Capability = "vision"
	// CapabilitySummarization indicates the model is optimized for summarization.
	CapabilitySummarization Capability = "summarization"
	// CapabilityExtraction indicates the model is optimized for information extraction.
	CapabilityExtraction Capability = "extraction"
	// CapabilityClassification indicates the model is optimized for classification tasks.
	CapabilityClassification Capability = "classification"
	// CapabilityCodeGeneration indicates the model supports code generation.
	CapabilityCodeGeneration Capability = "code_generation"
)

// HealthStatus represents the current health state of a model.
type HealthStatus string

const (
	// HealthStatusUnknown indicates the health status has not been checked.
	HealthStatusUnknown HealthStatus = "unknown"
	// HealthStatusHealthy indicates the model is available and responding.
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusDegraded indicates the model is responding but with issues.
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusUnhealthy indicates the model is not responding.
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	// HealthStatusLoading indicates the model is currently loading.
	HealthStatusLoading HealthStatus = "loading"
)

// ModelLimits defines operational constraints for a model.
type ModelLimits struct {
	// ContextWindow is the maximum number of tokens the model can process in a single request.
	ContextWindow int `json:"context_window"`

	// MaxOutputTokens is the maximum number of tokens the model can generate in a response.
	MaxOutputTokens int `json:"max_output_tokens"`

	// RateLimitRPM is the maximum requests per minute allowed.
	RateLimitRPM int `json:"rate_limit_rpm"`

	// RateLimitTPM is the maximum tokens per minute allowed.
	RateLimitTPM int `json:"rate_limit_tpm"`

	// MaxBatchSize is the maximum number of inputs that can be processed in a single batch.
	MaxBatchSize int `json:"max_batch_size"`
}

// ModelCost defines pricing information for a model.
type ModelCost struct {
	// InputCostPer1K is the cost per 1000 input tokens in USD.
	InputCostPer1K float64 `json:"input_cost_per_1k"`

	// OutputCostPer1K is the cost per 1000 output tokens in USD.
	OutputCostPer1K float64 `json:"output_cost_per_1k"`

	// EmbeddingCostPer1K is the cost per 1000 tokens for embeddings in USD.
	EmbeddingCostPer1K float64 `json:"embedding_cost_per_1k"`

	// Currency is the currency code (default: USD).
	Currency string `json:"currency"`
}

// ModelCapabilities holds the set of capabilities a model supports.
type ModelCapabilities struct {
	// Capabilities is the list of supported capabilities.
	Capabilities []Capability `json:"capabilities"`

	// EmbeddingDimensions is the dimension of embedding vectors (if embedding is supported).
	EmbeddingDimensions int `json:"embedding_dimensions,omitempty"`

	// SupportsFunctionCalling indicates if the model supports function/tool calling.
	SupportsFunctionCalling bool `json:"supports_function_calling"`

	// SupportsStreaming indicates if the model supports streaming responses.
	SupportsStreaming bool `json:"supports_streaming"`

	// SupportsJSON indicates if the model supports structured JSON output.
	SupportsJSON bool `json:"supports_json"`
}

// HasCapability checks if the model has a specific capability.
func (mc *ModelCapabilities) HasCapability(cap Capability) bool {
	for _, c := range mc.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// ModelVersion represents version information for a model.
type ModelVersion struct {
	// Version is the semantic version string (e.g., "3.1", "1.5").
	Version string `json:"version"`

	// ParameterSize describes the model size (e.g., "8B", "70B", "1.5-pro").
	ParameterSize string `json:"parameter_size,omitempty"`

	// ReleaseDate is when this version was released.
	ReleaseDate time.Time `json:"release_date,omitempty"`

	// IsLatest indicates if this is the latest stable version.
	IsLatest bool `json:"is_latest"`

	// IsDeprecated indicates if this version is deprecated.
	IsDeprecated bool `json:"is_deprecated"`
}

// ModelHealth tracks the health status of a model.
type ModelHealth struct {
	// Status is the current health status.
	Status HealthStatus `json:"status"`

	// LastCheck is when the health was last checked.
	LastCheck time.Time `json:"last_check"`

	// LastSuccess is when the model last responded successfully.
	LastSuccess time.Time `json:"last_success,omitempty"`

	// ErrorMessage contains details if the model is unhealthy.
	ErrorMessage string `json:"error_message,omitempty"`

	// AvgLatencyMs is the average response latency in milliseconds.
	AvgLatencyMs float64 `json:"avg_latency_ms,omitempty"`

	// RequestCount is the number of requests made since tracking started.
	RequestCount int64 `json:"request_count"`

	// ErrorCount is the number of errors since tracking started.
	ErrorCount int64 `json:"error_count"`

	// ConsecutiveFailures is the number of consecutive failed health checks.
	ConsecutiveFailures int `json:"consecutive_failures"`
}

// IsAvailable returns true if the model is healthy enough to accept requests.
func (mh *ModelHealth) IsAvailable() bool {
	return mh.Status == HealthStatusHealthy || mh.Status == HealthStatusDegraded
}

// ModelConfig represents the complete configuration for an AI model.
type ModelConfig struct {
	// ID is the unique identifier for this model configuration.
	// Format: provider/model-name (e.g., "mlx/Qwen2.5-32B", "gemini/gemini-pro").
	ID string `json:"id"`

	// Name is the human-readable display name.
	Name string `json:"name"`

	// Provider is the model provider (mlx, gemini, etc.).
	Provider Provider `json:"provider"`

	// ModelName is the provider-specific model identifier.
	// This is what gets passed to the provider's API.
	ModelName string `json:"model_name"`

	// Endpoint is the API endpoint URL for cloud providers.
	// For local providers like MLX, this is the base URL.
	Endpoint string `json:"endpoint,omitempty"`

	// Capabilities describes what the model can do.
	Capabilities ModelCapabilities `json:"capabilities"`

	// Limits defines operational constraints.
	Limits ModelLimits `json:"limits"`

	// Cost defines pricing information.
	Cost ModelCost `json:"cost,omitempty"`

	// Version contains version information.
	Version ModelVersion `json:"version"`

	// Health contains current health status.
	Health ModelHealth `json:"health"`

	// IsLocal indicates if this is a locally-hosted model.
	IsLocal bool `json:"is_local"`

	// IsDefault indicates if this is a default model for its capability.
	IsDefault bool `json:"is_default"`

	// Priority is used for model selection; higher priority models are preferred.
	Priority int `json:"priority"`

	// Tags are arbitrary labels for filtering and organization.
	Tags []string `json:"tags,omitempty"`

	// Metadata contains provider-specific configuration.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// CreatedAt is when this configuration was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this configuration was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate checks if the model configuration is valid.
func (mc *ModelConfig) Validate() error {
	if mc.ID == "" {
		return ErrInvalidModelID
	}
	if mc.Name == "" {
		return ErrInvalidModelName
	}
	if mc.Provider == "" {
		return ErrInvalidProvider
	}
	if mc.ModelName == "" {
		return ErrInvalidModelName
	}
	if len(mc.Capabilities.Capabilities) == 0 {
		return ErrNoCapabilities
	}
	return nil
}

// SupportsCapability checks if the model supports a specific capability.
func (mc *ModelConfig) SupportsCapability(cap Capability) bool {
	return mc.Capabilities.HasCapability(cap)
}

// Clone creates a deep copy of the model configuration.
func (mc *ModelConfig) Clone() *ModelConfig {
	clone := *mc
	clone.Capabilities.Capabilities = make([]Capability, len(mc.Capabilities.Capabilities))
	copy(clone.Capabilities.Capabilities, mc.Capabilities.Capabilities)
	clone.Tags = make([]string, len(mc.Tags))
	copy(clone.Tags, mc.Tags)
	if mc.Metadata != nil {
		clone.Metadata = make(map[string]interface{})
		for k, v := range mc.Metadata {
			clone.Metadata[k] = v
		}
	}
	return &clone
}
