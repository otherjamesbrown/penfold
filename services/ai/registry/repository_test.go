package registry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockRepository implements Repository for testing.
type mockRepository struct {
	mu            sync.RWMutex
	models        map[string]*ModelConfig
	health        map[string]*ModelHealth
	routingRules  []*RoutingRule
	createErr     error
	getErr        error
	updateErr     error
	deleteErr     error
	listErr       error
	healthGetErr  error
	healthSetErr  error
	routingErr    error
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		models:       make(map[string]*ModelConfig),
		health:       make(map[string]*ModelHealth),
		routingRules: []*RoutingRule{},
	}
}

func (m *mockRepository) CreateModel(ctx context.Context, model *ModelConfig) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.models[model.ID]; exists {
		return ErrModelAlreadyExists
	}
	m.models[model.ID] = model.Clone()
	return nil
}

func (m *mockRepository) GetModel(ctx context.Context, modelID string) (*ModelConfig, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	model, exists := m.models[modelID]
	if !exists {
		return nil, ErrModelNotFound
	}
	return model.Clone(), nil
}

func (m *mockRepository) UpdateModel(ctx context.Context, model *ModelConfig) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.models[model.ID]; !exists {
		return ErrModelNotFound
	}
	m.models[model.ID] = model.Clone()
	return nil
}

func (m *mockRepository) DeleteModel(ctx context.Context, modelID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.models[modelID]; !exists {
		return ErrModelNotFound
	}
	delete(m.models, modelID)
	delete(m.health, modelID)
	return nil
}

func (m *mockRepository) ListModels(ctx context.Context, filter *ModelFilter) ([]*ModelConfig, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*ModelConfig
	for _, model := range m.models {
		if filter == nil || filter.Match(model) {
			result = append(result, model.Clone())
		}
	}
	return result, nil
}

func (m *mockRepository) GetHealth(ctx context.Context, modelID string) (*ModelHealth, error) {
	if m.healthGetErr != nil {
		return nil, m.healthGetErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	health, exists := m.health[modelID]
	if !exists {
		return &ModelHealth{Status: HealthStatusUnknown}, nil
	}
	h := *health
	return &h, nil
}

func (m *mockRepository) UpdateHealth(ctx context.Context, modelID string, health *ModelHealth) error {
	if m.healthSetErr != nil {
		return m.healthSetErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	h := *health
	m.health[modelID] = &h
	return nil
}

func (m *mockRepository) GetRoutingRules(ctx context.Context) ([]*RoutingRule, error) {
	if m.routingErr != nil {
		return nil, m.routingErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*RoutingRule
	for _, rule := range m.routingRules {
		if rule.IsEnabled {
			result = append(result, rule.Clone())
		}
	}
	return result, nil
}

func (m *mockRepository) GetRoutingRulesByTask(ctx context.Context, taskType string) ([]*RoutingRule, error) {
	if m.routingErr != nil {
		return nil, m.routingErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*RoutingRule
	for _, rule := range m.routingRules {
		if rule.TaskType == taskType && rule.IsEnabled {
			result = append(result, rule.Clone())
		}
	}
	return result, nil
}

func (m *mockRepository) GetRoutingRuleByName(ctx context.Context, name string) (*RoutingRule, error) {
	if m.routingErr != nil {
		return nil, m.routingErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, rule := range m.routingRules {
		if rule.Name == name {
			return rule.Clone(), nil
		}
	}
	return nil, ErrRoutingRuleNotFound
}

func (m *mockRepository) CreateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	if m.routingErr != nil {
		return m.routingErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rule.ID = "mock-rule-id"
	m.routingRules = append(m.routingRules, rule.Clone())
	return nil
}

func (m *mockRepository) UpdateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	if m.routingErr != nil {
		return m.routingErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.routingRules {
		if r.ID == rule.ID {
			m.routingRules[i] = rule.Clone()
			return nil
		}
	}
	return ErrRoutingRuleNotFound
}

func (m *mockRepository) DeleteRoutingRule(ctx context.Context, ruleID string) error {
	if m.routingErr != nil {
		return m.routingErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.routingRules {
		if r.ID == ruleID {
			m.routingRules = append(m.routingRules[:i], m.routingRules[i+1:]...)
			return nil
		}
	}
	return ErrRoutingRuleNotFound
}

// Test helpers
func createTestModel(id string) *ModelConfig {
	return &ModelConfig{
		ID:        id,
		Name:      "Test Model " + id,
		Provider:  ProviderMLX,
		ModelName: id,
		Capabilities: ModelCapabilities{
			Capabilities: []Capability{CapabilityChat},
		},
		Limits: ModelLimits{
			ContextWindow:   4096,
			MaxOutputTokens: 2048,
		},
		Priority: 50,
		IsLocal:  true,
	}
}

func createTestRoutingRule(name, taskType string) *RoutingRule {
	return &RoutingRule{
		Name:             name,
		TaskType:         taskType,
		PreferredModels:  []string{"model-1"},
		FallbackModels:   []string{"model-2"},
		OptimizationMode: OptimizationModeBalanced,
		Priority:         10,
		IsEnabled:        true,
	}
}

// Tests for RoutingRule
func TestRoutingRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    *RoutingRule
		wantErr bool
	}{
		{
			name:    "valid rule",
			rule:    createTestRoutingRule("test", TaskTypeEmbedding),
			wantErr: false,
		},
		{
			name: "missing name",
			rule: &RoutingRule{
				TaskType:        TaskTypeEmbedding,
				PreferredModels: []string{"model-1"},
			},
			wantErr: true,
		},
		{
			name: "missing task type",
			rule: &RoutingRule{
				Name:            "test",
				PreferredModels: []string{"model-1"},
			},
			wantErr: true,
		},
		{
			name: "invalid task type",
			rule: &RoutingRule{
				Name:            "test",
				TaskType:        "invalid",
				PreferredModels: []string{"model-1"},
			},
			wantErr: true,
		},
		{
			name: "no preferred models",
			rule: &RoutingRule{
				Name:     "test",
				TaskType: TaskTypeEmbedding,
			},
			wantErr: true,
		},
		{
			name: "invalid optimization mode",
			rule: &RoutingRule{
				Name:             "test",
				TaskType:         TaskTypeEmbedding,
				PreferredModels:  []string{"model-1"},
				OptimizationMode: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestRoutingRule_Clone(t *testing.T) {
	cost := 0.05
	original := &RoutingRule{
		ID:                "rule-1",
		Name:              "test",
		TaskType:          TaskTypeEmbedding,
		PreferredModels:   []string{"model-1", "model-2"},
		FallbackModels:    []string{"model-3"},
		MaxCostPerRequest: &cost,
	}

	clone := original.Clone()

	// Modify clone
	clone.PreferredModels[0] = "modified"
	clone.FallbackModels[0] = "modified"
	*clone.MaxCostPerRequest = 1.0

	// Original should be unchanged
	if original.PreferredModels[0] == "modified" {
		t.Error("Clone modified original PreferredModels")
	}
	if original.FallbackModels[0] == "modified" {
		t.Error("Clone modified original FallbackModels")
	}
	if *original.MaxCostPerRequest == 1.0 {
		t.Error("Clone modified original MaxCostPerRequest")
	}
}

// Tests for DBRegistry
func TestDBRegistry_Register(t *testing.T) {
	repo := newMockRepository()
	config := &DBRegistryConfig{
		RegistryConfig: &RegistryConfig{
			HealthCheckInterval: 0,
			EnableAutoDiscovery: false,
		},
		SyncInterval: 0,
	}
	dbReg := NewDBRegistry(repo, config)

	model := createTestModel("test/model")

	// Register should succeed
	if err := dbReg.Register(model); err != nil {
		t.Errorf("Register failed: %v", err)
	}

	// Verify in memory
	retrieved, err := dbReg.Get("test/model")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if retrieved.ID != model.ID {
		t.Error("Model not found in memory")
	}

	// Verify in database
	dbModel, err := repo.GetModel(context.Background(), "test/model")
	if err != nil {
		t.Errorf("DB GetModel failed: %v", err)
	}
	if dbModel.ID != model.ID {
		t.Error("Model not found in database")
	}

	// Duplicate should fail
	if err := dbReg.Register(model); err != ErrModelAlreadyExists {
		t.Errorf("expected ErrModelAlreadyExists, got %v", err)
	}
}

func TestDBRegistry_RegisterDBError(t *testing.T) {
	repo := newMockRepository()
	repo.createErr = errors.New("db error")

	config := &DBRegistryConfig{
		RegistryConfig: &RegistryConfig{
			HealthCheckInterval: 0,
			EnableAutoDiscovery: false,
		},
		SyncInterval: 0,
	}
	dbReg := NewDBRegistry(repo, config)

	model := createTestModel("test/model")

	// Register should fail and rollback
	if err := dbReg.Register(model); err == nil {
		t.Error("expected error from DB")
	}

	// Should not be in memory after rollback
	if _, err := dbReg.Get("test/model"); err != ErrModelNotFound {
		t.Error("model should not exist after DB error rollback")
	}
}

func TestDBRegistry_Update(t *testing.T) {
	repo := newMockRepository()
	config := &DBRegistryConfig{
		RegistryConfig: &RegistryConfig{
			HealthCheckInterval: 0,
			EnableAutoDiscovery: false,
		},
		SyncInterval: 0,
	}
	dbReg := NewDBRegistry(repo, config)

	model := createTestModel("test/model")
	_ = dbReg.Register(model)

	// Update
	updated := model.Clone()
	updated.Name = "Updated Name"
	if err := dbReg.Update(updated); err != nil {
		t.Errorf("Update failed: %v", err)
	}

	// Verify in memory
	retrieved, _ := dbReg.Get("test/model")
	if retrieved.Name != "Updated Name" {
		t.Error("Update not reflected in memory")
	}

	// Verify in database
	dbModel, _ := repo.GetModel(context.Background(), "test/model")
	if dbModel.Name != "Updated Name" {
		t.Error("Update not reflected in database")
	}
}

func TestDBRegistry_Unregister(t *testing.T) {
	repo := newMockRepository()
	config := &DBRegistryConfig{
		RegistryConfig: &RegistryConfig{
			HealthCheckInterval: 0,
			EnableAutoDiscovery: false,
		},
		SyncInterval: 0,
	}
	dbReg := NewDBRegistry(repo, config)

	model := createTestModel("test/model")
	_ = dbReg.Register(model)

	// Unregister
	if err := dbReg.Unregister("test/model"); err != nil {
		t.Errorf("Unregister failed: %v", err)
	}

	// Verify removed from memory
	if _, err := dbReg.Get("test/model"); err != ErrModelNotFound {
		t.Error("Model still in memory after unregister")
	}

	// Verify removed from database
	if _, err := repo.GetModel(context.Background(), "test/model"); err != ErrModelNotFound {
		t.Error("Model still in database after unregister")
	}
}

func TestDBRegistry_UpdateHealth(t *testing.T) {
	repo := newMockRepository()
	config := &DBRegistryConfig{
		RegistryConfig: &RegistryConfig{
			HealthCheckInterval: 0,
			EnableAutoDiscovery: false,
		},
		SyncInterval: 0,
	}
	dbReg := NewDBRegistry(repo, config)

	model := createTestModel("test/model")
	_ = dbReg.Register(model)

	health := ModelHealth{
		Status:      HealthStatusHealthy,
		LastCheck:   time.Now(),
		LastSuccess: time.Now(),
	}

	if err := dbReg.UpdateHealth("test/model", health); err != nil {
		t.Errorf("UpdateHealth failed: %v", err)
	}

	// Verify in memory
	retrieved, _ := dbReg.Get("test/model")
	if retrieved.Health.Status != HealthStatusHealthy {
		t.Error("Health not updated in memory")
	}

	// Give async write time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify in database (health is written asynchronously)
	dbHealth, _ := repo.GetHealth(context.Background(), "test/model")
	if dbHealth.Status != HealthStatusHealthy {
		t.Error("Health not updated in database")
	}
}

func TestDBRegistry_LoadFromDB(t *testing.T) {
	repo := newMockRepository()

	// Pre-populate database
	ctx := context.Background()
	model1 := createTestModel("model-1")
	model2 := createTestModel("model-2")
	_ = repo.CreateModel(ctx, model1)
	_ = repo.CreateModel(ctx, model2)
	_ = repo.UpdateHealth(ctx, "model-1", &ModelHealth{Status: HealthStatusHealthy})

	config := &DBRegistryConfig{
		RegistryConfig: &RegistryConfig{
			HealthCheckInterval: 0,
			EnableAutoDiscovery: false,
		},
		SyncInterval: 0,
	}
	dbReg := NewDBRegistry(repo, config)

	// Start should load from DB
	if err := dbReg.Start(ctx); err != nil {
		t.Errorf("Start failed: %v", err)
	}
	defer dbReg.Stop()

	// Verify models loaded
	if dbReg.Count() != 2 {
		t.Errorf("expected 2 models, got %d", dbReg.Count())
	}

	// Verify health loaded
	m1, _ := dbReg.Get("model-1")
	if m1.Health.Status != HealthStatusHealthy {
		t.Error("Health not loaded from DB")
	}
}

func TestDBRegistry_RoutingRules(t *testing.T) {
	repo := newMockRepository()
	config := &DBRegistryConfig{
		RegistryConfig: &RegistryConfig{
			HealthCheckInterval: 0,
			EnableAutoDiscovery: false,
		},
		SyncInterval: 0,
	}
	dbReg := NewDBRegistry(repo, config)

	ctx := context.Background()

	// Create routing rule
	rule := createTestRoutingRule("test-rule", TaskTypeEmbedding)
	if err := dbReg.CreateRoutingRule(ctx, rule); err != nil {
		t.Errorf("CreateRoutingRule failed: %v", err)
	}

	// Get all rules
	rules, err := dbReg.GetRoutingRules(ctx)
	if err != nil {
		t.Errorf("GetRoutingRules failed: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(rules))
	}

	// Get by task type
	embeddingRules, err := dbReg.GetRoutingRulesByTask(ctx, TaskTypeEmbedding)
	if err != nil {
		t.Errorf("GetRoutingRulesByTask failed: %v", err)
	}
	if len(embeddingRules) != 1 {
		t.Errorf("expected 1 embedding rule, got %d", len(embeddingRules))
	}

	// No rules for other task types
	summaryRules, _ := dbReg.GetRoutingRulesByTask(ctx, TaskTypeSummarization)
	if len(summaryRules) != 0 {
		t.Errorf("expected 0 summarization rules, got %d", len(summaryRules))
	}
}

func TestDBRegistry_StartStop(t *testing.T) {
	repo := newMockRepository()
	config := &DBRegistryConfig{
		RegistryConfig: &RegistryConfig{
			HealthCheckInterval: 100 * time.Millisecond,
			EnableAutoDiscovery: false,
		},
		SyncInterval: 100 * time.Millisecond,
	}
	dbReg := NewDBRegistry(repo, config)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := dbReg.Start(ctx); err != nil {
		t.Errorf("Start failed: %v", err)
	}

	// Register a model
	model := createTestModel("test/model")
	_ = dbReg.Register(model)

	// Update health
	_ = dbReg.UpdateHealth("test/model", ModelHealth{Status: HealthStatusHealthy})

	// Let it run
	time.Sleep(200 * time.Millisecond)

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		dbReg.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Stop timed out")
	}
}

func TestDBRegistry_Reload(t *testing.T) {
	repo := newMockRepository()
	config := &DBRegistryConfig{
		RegistryConfig: &RegistryConfig{
			HealthCheckInterval: 0,
			EnableAutoDiscovery: false,
		},
		SyncInterval: 0,
	}
	dbReg := NewDBRegistry(repo, config)

	ctx := context.Background()

	// Register initial model
	model1 := createTestModel("model-1")
	_ = dbReg.Register(model1)

	if dbReg.Count() != 1 {
		t.Errorf("expected 1 model, got %d", dbReg.Count())
	}

	// Add model directly to DB
	model2 := createTestModel("model-2")
	_ = repo.CreateModel(ctx, model2)

	// Reload
	if err := dbReg.Reload(ctx); err != nil {
		t.Errorf("Reload failed: %v", err)
	}

	// Should now have 2 models
	if dbReg.Count() != 2 {
		t.Errorf("expected 2 models after reload, got %d", dbReg.Count())
	}
}

// Ensure mockRepository implements Repository interface
var _ Repository = (*mockRepository)(nil)
