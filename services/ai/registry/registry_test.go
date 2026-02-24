package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestModelCapabilities_HasCapability(t *testing.T) {
	caps := &ModelCapabilities{
		Capabilities: []Capability{CapabilityChat, CapabilityEmbedding},
	}

	if !caps.HasCapability(CapabilityChat) {
		t.Error("expected HasCapability(Chat) to return true")
	}
	if !caps.HasCapability(CapabilityEmbedding) {
		t.Error("expected HasCapability(Embedding) to return true")
	}
	if caps.HasCapability(CapabilityVision) {
		t.Error("expected HasCapability(Vision) to return false")
	}
}

func TestModelHealth_IsAvailable(t *testing.T) {
	tests := []struct {
		status   HealthStatus
		expected bool
	}{
		{HealthStatusHealthy, true},
		{HealthStatusDegraded, true},
		{HealthStatusUnhealthy, false},
		{HealthStatusUnknown, false},
		{HealthStatusLoading, false},
	}

	for _, tc := range tests {
		health := &ModelHealth{Status: tc.status}
		if health.IsAvailable() != tc.expected {
			t.Errorf("IsAvailable() for status %s: got %v, want %v", tc.status, health.IsAvailable(), tc.expected)
		}
	}
}

func TestModelConfig_Validate(t *testing.T) {
	validConfig := &ModelConfig{
		ID:        "test/model",
		Name:      "Test Model",
		Provider:  ProviderMLX,
		ModelName: "test-model",
		Capabilities: ModelCapabilities{
			Capabilities: []Capability{CapabilityChat},
		},
	}

	if err := validConfig.Validate(); err != nil {
		t.Errorf("valid config should not return error: %v", err)
	}

	// Test missing ID
	invalid := validConfig.Clone()
	invalid.ID = ""
	if err := invalid.Validate(); err != ErrInvalidModelID {
		t.Errorf("expected ErrInvalidModelID, got %v", err)
	}

	// Test missing name
	invalid = validConfig.Clone()
	invalid.Name = ""
	if err := invalid.Validate(); err != ErrInvalidModelName {
		t.Errorf("expected ErrInvalidModelName, got %v", err)
	}

	// Test missing provider
	invalid = validConfig.Clone()
	invalid.Provider = ""
	if err := invalid.Validate(); err != ErrInvalidProvider {
		t.Errorf("expected ErrInvalidProvider, got %v", err)
	}

	// Test missing model name
	invalid = validConfig.Clone()
	invalid.ModelName = ""
	if err := invalid.Validate(); err != ErrInvalidModelName {
		t.Errorf("expected ErrInvalidModelName, got %v", err)
	}

	// Test no capabilities
	invalid = validConfig.Clone()
	invalid.Capabilities.Capabilities = nil
	if err := invalid.Validate(); err != ErrNoCapabilities {
		t.Errorf("expected ErrNoCapabilities, got %v", err)
	}
}

func TestModelConfig_Clone(t *testing.T) {
	original := &ModelConfig{
		ID:        "test/model",
		Name:      "Test Model",
		Provider:  ProviderMLX,
		ModelName: "test-model",
		Capabilities: ModelCapabilities{
			Capabilities: []Capability{CapabilityChat, CapabilityEmbedding},
		},
		Tags:     []string{"tag1", "tag2"},
		Metadata: map[string]interface{}{"key": "value"},
	}

	clone := original.Clone()

	// Verify values are copied
	if clone.ID != original.ID {
		t.Error("ID not cloned correctly")
	}

	// Modify clone and verify original is unchanged
	clone.Tags[0] = "modified"
	if original.Tags[0] == "modified" {
		t.Error("Clone modified original tags")
	}

	clone.Capabilities.Capabilities[0] = CapabilityVision
	if original.Capabilities.Capabilities[0] == CapabilityVision {
		t.Error("Clone modified original capabilities")
	}

	clone.Metadata["key"] = "changed"
	if original.Metadata["key"] == "changed" {
		t.Error("Clone modified original metadata")
	}
}

func TestModelFilter_Match(t *testing.T) {
	model := &ModelConfig{
		ID:       "mlx/qwen2.5-32b",
		Provider: ProviderMLX,
		Capabilities: ModelCapabilities{
			Capabilities: []Capability{CapabilityChat, CapabilitySummarization},
		},
		Health: ModelHealth{
			Status: HealthStatusHealthy,
		},
		IsLocal:  true,
		Priority: 100,
		Tags:     []string{"fast", "local"},
	}

	tests := []struct {
		name     string
		filter   *ModelFilter
		expected bool
	}{
		{"nil filter matches", nil, true},
		{"empty filter matches", &ModelFilter{}, true},
		{"matching provider", &ModelFilter{Provider: ProviderMLX}, true},
		{"non-matching provider", &ModelFilter{Provider: ProviderGemini}, false},
		{"matching capability", &ModelFilter{Capability: CapabilityChat}, true},
		{"non-matching capability", &ModelFilter{Capability: CapabilityVision}, false},
		{"matching local", &ModelFilter{IsLocal: boolPtr(true)}, true},
		{"non-matching local", &ModelFilter{IsLocal: boolPtr(false)}, false},
		{"matching health", &ModelFilter{HealthStatus: HealthStatusHealthy}, true},
		{"non-matching health", &ModelFilter{HealthStatus: HealthStatusUnhealthy}, false},
		{"matching tags", &ModelFilter{Tags: []string{"fast"}}, true},
		{"non-matching tags", &ModelFilter{Tags: []string{"cloud"}}, false},
		{"matching priority", &ModelFilter{MinPriority: 50}, true},
		{"non-matching priority", &ModelFilter{MinPriority: 150}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.filter == nil || tc.filter.Match(model)
			if result != tc.expected {
				t.Errorf("got %v, want %v", result, tc.expected)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry(nil)

	model := &ModelConfig{
		ID:        "test/model",
		Name:      "Test Model",
		Provider:  ProviderMLX,
		ModelName: "test-model",
		Capabilities: ModelCapabilities{
			Capabilities: []Capability{CapabilityChat},
		},
	}

	// Register should succeed
	if err := r.Register(model); err != nil {
		t.Errorf("Register failed: %v", err)
	}

	// Duplicate should fail
	if err := r.Register(model); err != ErrModelAlreadyExists {
		t.Errorf("expected ErrModelAlreadyExists, got %v", err)
	}

	// Verify timestamps were set
	stored, _ := r.Get(model.ID)
	if stored.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if stored.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}

	// Verify count
	if r.Count() != 1 {
		t.Errorf("expected count 1, got %d", r.Count())
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry(nil)

	model := &ModelConfig{
		ID:        "test/model",
		Name:      "Test Model",
		Provider:  ProviderMLX,
		ModelName: "test-model",
		Capabilities: ModelCapabilities{
			Capabilities: []Capability{CapabilityChat},
		},
	}
	_ = r.Register(model)

	// Get existing
	retrieved, err := r.Get("test/model")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if retrieved.ID != model.ID {
		t.Error("Retrieved model ID doesn't match")
	}

	// Get non-existing
	_, err = r.Get("nonexistent")
	if err != ErrModelNotFound {
		t.Errorf("expected ErrModelNotFound, got %v", err)
	}
}

func TestRegistry_Update(t *testing.T) {
	r := NewRegistry(nil)

	model := &ModelConfig{
		ID:        "test/model",
		Name:      "Test Model",
		Provider:  ProviderMLX,
		ModelName: "test-model",
		Capabilities: ModelCapabilities{
			Capabilities: []Capability{CapabilityChat},
		},
	}
	_ = r.Register(model)

	// Update
	updated := model.Clone()
	updated.Name = "Updated Name"
	if err := r.Update(updated); err != nil {
		t.Errorf("Update failed: %v", err)
	}

	// Verify update
	retrieved, _ := r.Get(model.ID)
	if retrieved.Name != "Updated Name" {
		t.Error("Update not applied")
	}

	// Update non-existing
	nonexistent := model.Clone()
	nonexistent.ID = "nonexistent"
	if err := r.Update(nonexistent); err != ErrModelNotFound {
		t.Errorf("expected ErrModelNotFound, got %v", err)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry(nil)

	model := &ModelConfig{
		ID:        "test/model",
		Name:      "Test Model",
		Provider:  ProviderMLX,
		ModelName: "test-model",
		Capabilities: ModelCapabilities{
			Capabilities: []Capability{CapabilityChat},
		},
	}
	_ = r.Register(model)

	// Unregister
	if err := r.Unregister(model.ID); err != nil {
		t.Errorf("Unregister failed: %v", err)
	}

	// Verify removed
	if r.Count() != 0 {
		t.Error("Model not removed")
	}

	// Unregister non-existing
	if err := r.Unregister("nonexistent"); err != ErrModelNotFound {
		t.Errorf("expected ErrModelNotFound, got %v", err)
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry(nil)

	// Register multiple models
	models := []*ModelConfig{
		{
			ID:        "mlx/qwen2.5-32b",
			Name:      "Llama 3",
			Provider:  ProviderMLX,
			ModelName: "llama3",
			Capabilities: ModelCapabilities{
				Capabilities: []Capability{CapabilityChat, CapabilitySummarization},
			},
			IsLocal: true,
		},
		{
			ID:        "mlx/embed",
			Name:      "Embed",
			Provider:  ProviderMLX,
			ModelName: "nomic-embed-text",
			Capabilities: ModelCapabilities{
				Capabilities: []Capability{CapabilityEmbedding},
			},
			IsLocal: true,
		},
		{
			ID:        "gemini/pro",
			Name:      "Gemini Pro",
			Provider:  ProviderGemini,
			ModelName: "gemini-pro",
			Capabilities: ModelCapabilities{
				Capabilities: []Capability{CapabilityChat, CapabilitySummarization},
			},
			IsLocal: false,
		},
	}

	for _, m := range models {
		_ = r.Register(m)
	}

	// List all
	all := r.List(nil)
	if len(all) != 3 {
		t.Errorf("expected 3 models, got %d", len(all))
	}

	// List by provider
	mlxModels := r.ListByProvider(ProviderMLX)
	if len(mlxModels) != 2 {
		t.Errorf("expected 2 MLX models, got %d", len(mlxModels))
	}

	// List by capability
	chatModels := r.ListByCapability(CapabilityChat)
	if len(chatModels) != 2 {
		t.Errorf("expected 2 chat models, got %d", len(chatModels))
	}

	embeddingModels := r.ListByCapability(CapabilityEmbedding)
	if len(embeddingModels) != 1 {
		t.Errorf("expected 1 embedding model, got %d", len(embeddingModels))
	}
}

func TestRegistry_ListAvailable(t *testing.T) {
	r := NewRegistry(nil)

	// Register models with different health statuses
	models := []*ModelConfig{
		{
			ID:           "healthy",
			Name:         "Healthy",
			Provider:     ProviderMLX,
			ModelName:    "healthy",
			Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
			Health:       ModelHealth{Status: HealthStatusHealthy},
		},
		{
			ID:           "degraded",
			Name:         "Degraded",
			Provider:     ProviderMLX,
			ModelName:    "degraded",
			Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
			Health:       ModelHealth{Status: HealthStatusDegraded},
		},
		{
			ID:           "unhealthy",
			Name:         "Unhealthy",
			Provider:     ProviderMLX,
			ModelName:    "unhealthy",
			Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
			Health:       ModelHealth{Status: HealthStatusUnhealthy},
		},
	}

	for _, m := range models {
		_ = r.Register(m)
	}

	available := r.ListAvailable()
	if len(available) != 2 {
		t.Errorf("expected 2 available models, got %d", len(available))
	}
}

func TestRegistry_GetDefaultModel(t *testing.T) {
	r := NewRegistry(nil)

	// Register models
	models := []*ModelConfig{
		{
			ID:           "model1",
			Name:         "Model 1",
			Provider:     ProviderMLX,
			ModelName:    "model1",
			Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
			Health:       ModelHealth{Status: HealthStatusHealthy},
			IsDefault:    false,
			Priority:     50,
		},
		{
			ID:           "model2",
			Name:         "Model 2",
			Provider:     ProviderMLX,
			ModelName:    "model2",
			Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
			Health:       ModelHealth{Status: HealthStatusHealthy},
			IsDefault:    true,
			Priority:     100,
		},
	}

	for _, m := range models {
		_ = r.Register(m)
	}

	// Should return the default model
	def, err := r.GetDefaultModel(CapabilityChat)
	if err != nil {
		t.Errorf("GetDefaultModel failed: %v", err)
	}
	if def.ID != "model2" {
		t.Errorf("expected model2, got %s", def.ID)
	}

	// Should return error for non-existing capability
	_, err = r.GetDefaultModel(CapabilityVision)
	if err != ErrModelNotFound {
		t.Errorf("expected ErrModelNotFound, got %v", err)
	}
}

func TestRegistry_SelectModel(t *testing.T) {
	r := NewRegistry(nil)

	// Register models
	models := []*ModelConfig{
		{
			ID:           "local",
			Name:         "Local",
			Provider:     ProviderMLX,
			ModelName:    "local",
			Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
			Health:       ModelHealth{Status: HealthStatusHealthy},
			IsLocal:      true,
			Priority:     50,
		},
		{
			ID:           "cloud",
			Name:         "Cloud",
			Provider:     ProviderGemini,
			ModelName:    "cloud",
			Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
			Health:       ModelHealth{Status: HealthStatusHealthy},
			IsLocal:      false,
			Priority:     100,
		},
	}

	for _, m := range models {
		_ = r.Register(m)
	}

	// Prefer local
	selected, err := r.SelectModel(CapabilityChat, true)
	if err != nil {
		t.Errorf("SelectModel failed: %v", err)
	}
	if !selected.IsLocal {
		t.Error("expected local model when preferLocal=true")
	}

	// Prefer cloud (higher priority)
	selected, err = r.SelectModel(CapabilityChat, false)
	if err != nil {
		t.Errorf("SelectModel failed: %v", err)
	}
	if selected.ID != "cloud" {
		t.Error("expected cloud model (higher priority) when preferLocal=false")
	}
}

func TestRegistry_UpdateHealth(t *testing.T) {
	r := NewRegistry(nil)

	model := &ModelConfig{
		ID:           "test",
		Name:         "Test",
		Provider:     ProviderMLX,
		ModelName:    "test",
		Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
	}
	_ = r.Register(model)

	health := ModelHealth{
		Status:      HealthStatusHealthy,
		LastCheck:   time.Now(),
		LastSuccess: time.Now(),
	}

	if err := r.UpdateHealth("test", health); err != nil {
		t.Errorf("UpdateHealth failed: %v", err)
	}

	retrieved, _ := r.Get("test")
	if retrieved.Health.Status != HealthStatusHealthy {
		t.Error("Health not updated")
	}

	// Update non-existing
	if err := r.UpdateHealth("nonexistent", health); err != ErrModelNotFound {
		t.Errorf("expected ErrModelNotFound, got %v", err)
	}
}

func TestRegistry_RecordRequest(t *testing.T) {
	r := NewRegistry(nil)

	model := &ModelConfig{
		ID:           "test",
		Name:         "Test",
		Provider:     ProviderMLX,
		ModelName:    "test",
		Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
	}
	_ = r.Register(model)

	// Record successful request
	if err := r.RecordRequest("test", 100.0, true); err != nil {
		t.Errorf("RecordRequest failed: %v", err)
	}

	retrieved, _ := r.Get("test")
	if retrieved.Health.RequestCount != 1 {
		t.Errorf("expected RequestCount 1, got %d", retrieved.Health.RequestCount)
	}
	if retrieved.Health.AvgLatencyMs != 100.0 {
		t.Errorf("expected AvgLatencyMs 100, got %f", retrieved.Health.AvgLatencyMs)
	}

	// Record failed request
	if err := r.RecordRequest("test", 200.0, false); err != nil {
		t.Errorf("RecordRequest failed: %v", err)
	}

	retrieved, _ = r.Get("test")
	if retrieved.Health.RequestCount != 2 {
		t.Errorf("expected RequestCount 2, got %d", retrieved.Health.RequestCount)
	}
	if retrieved.Health.ErrorCount != 1 {
		t.Errorf("expected ErrorCount 1, got %d", retrieved.Health.ErrorCount)
	}
}

func TestRegistry_ClosedOperations(t *testing.T) {
	r := NewRegistry(nil)
	r.Stop()

	model := &ModelConfig{
		ID:           "test",
		Name:         "Test",
		Provider:     ProviderMLX,
		ModelName:    "test",
		Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
	}

	if err := r.Register(model); err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed, got %v", err)
	}

	if _, err := r.Get("test"); err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed, got %v", err)
	}

	if err := r.Update(model); err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed, got %v", err)
	}

	if err := r.Unregister("test"); err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed, got %v", err)
	}
}

func TestDefaultModels(t *testing.T) {
	models := DefaultModels("http://localhost:8080", "http://localhost:8081", "https://api.example.com")

	if len(models) == 0 {
		t.Error("expected default models")
	}

	// Check for specific expected models
	modelIDs := make(map[string]bool)
	for _, m := range models {
		modelIDs[m.ID] = true
	}

	expectedIDs := []string{
		"mlx/Qwen2.5-7B-Instruct-4bit",
		"mlx/mxbai-embed-large",
		"gemini/gemini-pro",
		"gemini/gemini-2.5-pro",
		"gemini/gemini-2.5-flash",
	}

	for _, id := range expectedIDs {
		if !modelIDs[id] {
			t.Errorf("expected model %s not found", id)
		}
	}
}

func TestNewRegistryWithDefaults(t *testing.T) {
	r := NewRegistryWithDefaults(nil)

	if r.Count() == 0 {
		t.Error("expected registry to have default models")
	}

	// Verify we can get a default embedding model
	embeddingModels := r.ListByCapability(CapabilityEmbedding)
	if len(embeddingModels) == 0 {
		t.Error("expected embedding models")
	}

	// Verify we can get a default chat model
	chatModels := r.ListByCapability(CapabilityChat)
	if len(chatModels) == 0 {
		t.Error("expected chat models")
	}
}

func TestRegistry_MLXHealthCheck(t *testing.T) {
	// Create mock MLX server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	config := &RegistryConfig{
		MLXLLMURL:           server.URL,
		MLXEmbeddingsURL:    server.URL,
		EnableAutoDiscovery: false,
		HealthCheckInterval: 0,
		HealthCheckTimeout:  5 * time.Second,
	}

	r := NewRegistry(config)

	model := &ModelConfig{
		ID:           "mlx/test-model",
		Name:         "Test Model",
		Provider:     ProviderMLX,
		ModelName:    "test-model",
		Endpoint:     server.URL,
		Capabilities: ModelCapabilities{Capabilities: []Capability{CapabilityChat}},
	}
	_ = r.Register(model)

	// Trigger health check
	ctx := context.Background()
	r.checkAllHealth(ctx)

	// Verify health was updated
	checked, _ := r.Get("mlx/test-model")
	if checked.Health.Status != HealthStatusHealthy {
		t.Errorf("expected healthy status, got %s", checked.Health.Status)
	}
}

func TestRegistry_StartStop(t *testing.T) {
	config := &RegistryConfig{
		MLXLLMURL:           "http://localhost:8080",
		MLXEmbeddingsURL:    "http://localhost:8081",
		HealthCheckInterval: 100 * time.Millisecond,
		DiscoveryInterval:   100 * time.Millisecond,
		EnableAutoDiscovery: false, // Disable to avoid connection errors
	}

	r := NewRegistry(config)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := r.Start(ctx); err != nil {
		t.Errorf("Start failed: %v", err)
	}

	// Let it run briefly
	time.Sleep(200 * time.Millisecond)

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Stop timed out")
	}
}

func TestRegisterDefaults(t *testing.T) {
	r := NewRegistry(nil)

	err := RegisterDefaults(r, "http://localhost:8080", "http://localhost:8081", "https://api.example.com")
	if err != nil {
		t.Errorf("RegisterDefaults failed: %v", err)
	}

	// Calling again should not fail (idempotent)
	err = RegisterDefaults(r, "http://localhost:8080", "http://localhost:8081", "https://api.example.com")
	if err != nil {
		t.Errorf("Second RegisterDefaults failed: %v", err)
	}
}
