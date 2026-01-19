package registry

import "time"

// DefaultModels returns the pre-configured known models for Ollama and Gemini.
func DefaultModels(ollamaHost, geminiEndpoint string) []*ModelConfig {
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	if geminiEndpoint == "" {
		geminiEndpoint = "https://generativelanguage.googleapis.com/v1"
	}

	now := time.Now()

	return []*ModelConfig{
		// Ollama Models
		{
			ID:        "ollama/llama3",
			Name:      "Llama 3",
			Provider:  ProviderOllama,
			ModelName: "llama3",
			Endpoint:  ollamaHost,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilitySummarization, CapabilityExtraction},
				SupportsFunctionCalling: false,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   8192,
				MaxOutputTokens: 4096,
				RateLimitRPM:    0, // No rate limit for local
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0,
				OutputCostPer1K: 0,
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:       "3",
				ParameterSize: "8B",
				IsLatest:      true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   true,
			IsDefault: true,
			Priority:  100,
			Tags:      []string{"general", "fast", "local"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "ollama/llama3.1",
			Name:      "Llama 3.1",
			Provider:  ProviderOllama,
			ModelName: "llama3.1",
			Endpoint:  ollamaHost,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilitySummarization, CapabilityExtraction},
				SupportsFunctionCalling: true,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   131072,
				MaxOutputTokens: 4096,
				RateLimitRPM:    0,
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0,
				OutputCostPer1K: 0,
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:       "3.1",
				ParameterSize: "8B",
				IsLatest:      true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   true,
			IsDefault: false,
			Priority:  110,
			Tags:      []string{"general", "fast", "local", "long-context"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "ollama/llama3.2",
			Name:      "Llama 3.2",
			Provider:  ProviderOllama,
			ModelName: "llama3.2",
			Endpoint:  ollamaHost,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilitySummarization, CapabilityExtraction, CapabilityClassification},
				SupportsFunctionCalling: true,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   131072,
				MaxOutputTokens: 4096,
				RateLimitRPM:    0,
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0,
				OutputCostPer1K: 0,
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:       "3.2",
				ParameterSize: "3B",
				IsLatest:      true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   true,
			IsDefault: false,
			Priority:  120,
			Tags:      []string{"general", "fast", "local", "long-context", "lightweight"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "ollama/mistral",
			Name:      "Mistral",
			Provider:  ProviderOllama,
			ModelName: "mistral",
			Endpoint:  ollamaHost,
			Capabilities: ModelCapabilities{
				Capabilities:            []Capability{CapabilityChat, CapabilityCompletion, CapabilitySummarization},
				SupportsFunctionCalling: false,
				SupportsStreaming:       true,
				SupportsJSON:            true,
			},
			Limits: ModelLimits{
				ContextWindow:   32768,
				MaxOutputTokens: 4096,
				RateLimitRPM:    0,
				MaxBatchSize:    1,
			},
			Cost: ModelCost{
				InputCostPer1K:  0,
				OutputCostPer1K: 0,
				Currency:        "USD",
			},
			Version: ModelVersion{
				Version:       "0.3",
				ParameterSize: "7B",
				IsLatest:      true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   true,
			IsDefault: false,
			Priority:  90,
			Tags:      []string{"general", "fast", "local"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "ollama/nomic-embed-text",
			Name:      "Nomic Embed Text",
			Provider:  ProviderOllama,
			ModelName: "nomic-embed-text",
			Endpoint:  ollamaHost,
			Capabilities: ModelCapabilities{
				Capabilities:        []Capability{CapabilityEmbedding},
				EmbeddingDimensions: 768,
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
				Version:  "1.5",
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
			ID:        "gemini/gemini-1.5-pro",
			Name:      "Gemini 1.5 Pro",
			Provider:  ProviderGemini,
			ModelName: "gemini-1.5-pro",
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
				IsLatest: true,
			},
			Health: ModelHealth{
				Status: HealthStatusUnknown,
			},
			IsLocal:   false,
			IsDefault: true,
			Priority:  85,
			Tags:      []string{"general", "cloud", "google", "long-context", "vision"},
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
func RegisterDefaults(r *ModelRegistry, ollamaHost, geminiEndpoint string) error {
	models := DefaultModels(ollamaHost, geminiEndpoint)
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

	ollamaHost := ""
	geminiEndpoint := ""
	if config != nil {
		ollamaHost = config.OllamaHost
		geminiEndpoint = config.GeminiEndpoint
	}

	_ = RegisterDefaults(r, ollamaHost, geminiEndpoint)
	return r
}
