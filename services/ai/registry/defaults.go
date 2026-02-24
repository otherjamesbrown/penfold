package registry

import "time"

// DefaultModels returns the pre-configured known models for MLX and Gemini.
func DefaultModels(mlxLLMURL, mlxEmbeddingsURL, geminiEndpoint string) []*ModelConfig {
	if mlxLLMURL == "" {
		mlxLLMURL = "http://localhost:8080"
	}
	if mlxEmbeddingsURL == "" {
		mlxEmbeddingsURL = "http://localhost:11434"
	}
	if geminiEndpoint == "" {
		geminiEndpoint = "https://generativelanguage.googleapis.com/v1"
	}

	now := time.Now()

	return []*ModelConfig{
		// MLX Models (via vllm-mlx)
		// Note: MLX LLM is retained as a local fallback but is no longer the default.
		// Gemini 2.0 Flash is the default LLM for chat/completion operations.
		{
			ID:        "mlx/Qwen2.5-7B-Instruct-4bit",
			Name:      "Qwen 2.5 7B Instruct",
			Provider:  ProviderMLX,
			ModelName: "mlx-community/Qwen2.5-7B-Instruct-4bit",
			Endpoint:  mlxLLMURL,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilitySummarization, CapabilityExtraction, CapabilityClassification, CapabilityCodeGeneration},
				SupportsFunctionCalling: true,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   32768,
				MaxOutputTokens: 8192,
				RateLimitRPM:    0, // No rate limit for local
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0,
				OutputCostPer1K: 0,
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:       "2.5",
				ParameterSize: "32B",
				IsLatest:      true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   true,
			IsDefault: false,
			Priority:  50,
			Tags:      []string{"general", "local", "instruction-following", "fallback"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "mlx/mxbai-embed-large",
			Name:      "MixedBread Embed Large",
			Provider:  ProviderMLX,
			ModelName: "mxbai-embed-large",
			Endpoint:  mlxEmbeddingsURL,
			Capabilities: ModelCapabilities{
				Capabilities:        []Capability{CapabilityEmbedding},
				EmbeddingDimensions: 1024,
				SupportsStreaming:   false,
				SupportsJSON:        false,
			},
			Limits: ModelLimits{
				ContextWindow:   8192,
				MaxOutputTokens: 0, // Embedding model
				RateLimitRPM:    0,
				MaxBatchSize:    32,
			},
			Cost: ModelCost{
				EmbeddingCostPer1K: 0,
				Currency:           "USD",
			},
			Version: ModelVersion{
				Version:  "1",
				IsLatest: true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   true,
			IsDefault: true,
			Priority:  100,
			Tags:      []string{"embedding", "fast", "local"},
			CreatedAt: now,
			UpdatedAt: now,
		},

		// Gemini Models
		{
			ID:        "gemini/gemini-pro",
			Name:      "Gemini Pro",
			Provider:  ProviderGemini,
			ModelName: "gemini-pro",
			Endpoint:  geminiEndpoint,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilitySummarization, CapabilityExtraction, CapabilityClassification},
				SupportsFunctionCalling: true,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   32760,
				MaxOutputTokens: 8192,
				RateLimitRPM:    60,
				RateLimitTPM:    60000,
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0.00025, // $0.00025 per 1K input tokens
				OutputCostPer1K: 0.0005,  // $0.0005 per 1K output tokens
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:  "1.0",
				IsLatest: false,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   false,
			IsDefault: false,
			Priority:  80,
			Tags:      []string{"general", "cloud", "google"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "gemini/gemini-2.5-pro",
			Name:      "Gemini 2.5 Pro",
			Provider:  ProviderGemini,
			ModelName: "gemini-2.5-pro",
			Endpoint:  geminiEndpoint,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilitySummarization, CapabilityExtraction, CapabilityClassification, CapabilityVision},
				SupportsFunctionCalling: true,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   2097152, // 2M tokens
				MaxOutputTokens: 8192,
				RateLimitRPM:    60,
				RateLimitTPM:    60000,
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0.00125, // $0.00125 per 1K input tokens
				OutputCostPer1K: 0.005,   // $0.005 per 1K output tokens
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:  "1.5",
				IsLatest: false,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   false,
			IsDefault: false,
			Priority:  85,
			Tags:      []string{"general", "cloud", "google", "long-context", "vision"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "gemini/gemini-2.5-flash",
			Name:      "Gemini 2.5 Flash",
			Provider:  ProviderGemini,
			ModelName: "gemini-2.5-flash",
			Endpoint:  geminiEndpoint,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilitySummarization, CapabilityExtraction, CapabilityClassification, CapabilityCodeGeneration},
				SupportsFunctionCalling: true,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   1048576, // 1M tokens
				MaxOutputTokens: 8192,
				RateLimitRPM:    60,
				RateLimitTPM:    60000,
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0.00015, // $0.00015 per 1K input tokens
				OutputCostPer1K: 0.00060, // $0.00060 per 1K output tokens
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:  "2.5",
				IsLatest: true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   false,
			IsDefault: false,
			Priority:  92,
			Tags:      []string{"general", "cloud", "google", "fast", "structured-output", "long-context"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "gemini/gemini-2.0-flash",
			Name:      "Gemini 2.0 Flash",
			Provider:  ProviderGemini,
			ModelName: "gemini-2.0-flash",
			Endpoint:  geminiEndpoint,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilitySummarization, CapabilityExtraction, CapabilityClassification, CapabilityCodeGeneration},
				SupportsFunctionCalling: true,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   1048576, // 1M tokens
				MaxOutputTokens: 8192,
				RateLimitRPM:    60,
				RateLimitTPM:    60000,
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0.00010, // $0.00010 per 1K input tokens
				OutputCostPer1K: 0.00040, // $0.00040 per 1K output tokens
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:  "2.0",
				IsLatest: true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   false,
			IsDefault: true,
			Priority:  90,
			Tags:      []string{"general", "cloud", "google", "fast", "structured-output"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "gemini/gemini-pro-vision",
			Name:      "Gemini Pro Vision",
			Provider:  ProviderGemini,
			ModelName: "gemini-pro-vision",
			Endpoint:  geminiEndpoint,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilityVision},
				SupportsFunctionCalling: false,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   16384,
				MaxOutputTokens: 4096,
				RateLimitRPM:    60,
				RateLimitTPM:    60000,
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0.00025, // $0.00025 per 1K input tokens
				OutputCostPer1K: 0.0005,  // $0.0005 per 1K output tokens
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:      "1.0",
				IsLatest:     false,
				IsDeprecated: true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   false,
			IsDefault: false,
			Priority:  70,
			Tags:      []string{"vision", "cloud", "google", "deprecated"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "gemini/text-embedding-004",
			Name:      "Gemini Text Embedding",
			Provider:  ProviderGemini,
			ModelName: "text-embedding-004",
			Endpoint:  geminiEndpoint,
			Capabilities: ModelCapabilities{
				Capabilities:        []Capability{CapabilityEmbedding},
				EmbeddingDimensions: 768,
				SupportsStreaming:   false,
				SupportsJSON:        false,
			},
			Limits: ModelLimits{
				ContextWindow:   2048,
				MaxOutputTokens: 0,
				RateLimitRPM:    1500,
				MaxBatchSize:    100,
			},
			Cost: ModelCost{
				EmbeddingCostPer1K: 0.00001, // $0.00001 per 1K tokens
				Currency:           "USD",
			},
			Version: ModelVersion{
				Version:  "004",
				IsLatest: true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   false,
			IsDefault: false,
			Priority:  75,
			Tags:      []string{"embedding", "cloud", "google"},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

// RegisterDefaults registers all default models into the registry.
func RegisterDefaults(r *ModelRegistry, mlxLLMURL, mlxEmbeddingsURL, geminiEndpoint string) error {
	models := DefaultModels(mlxLLMURL, mlxEmbeddingsURL, geminiEndpoint)
	for _, m := range models {
		if err := r.Register(m); err != nil {
			// Ignore already exists errors for idempotency
			if err != ErrModelAlreadyExists {
				return err
			}
		}
	}
	return nil
}

// NewRegistryWithDefaults creates a new registry pre-populated with default models.
func NewRegistryWithDefaults(config *RegistryConfig) *ModelRegistry {
	r := NewRegistry(config)

	mlxLLMURL := ""
	mlxEmbeddingsURL := ""
	geminiEndpoint := ""
	if config != nil {
		mlxLLMURL = config.MLXLLMURL
		mlxEmbeddingsURL = config.MLXEmbeddingsURL
		geminiEndpoint = config.GeminiEndpoint
	}

	_ = RegisterDefaults(r, mlxLLMURL, mlxEmbeddingsURL, geminiEndpoint)
	return r
}
