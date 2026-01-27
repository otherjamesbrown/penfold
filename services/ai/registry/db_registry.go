package registry

import (
	"context"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// DBRegistry provides a database-backed model registry that wraps the in-memory
// ModelRegistry for caching while persisting changes to PostgreSQL.
type DBRegistry struct {
	*ModelRegistry        // Embedded for in-memory operations
	repo           Repository
	logger         logging.Logger
	mu             sync.RWMutex
	syncInterval   time.Duration
	stopSync       chan struct{}
	wg             sync.WaitGroup
}

// DBRegistryConfig holds configuration for the DB registry.
type DBRegistryConfig struct {
	// Registry configuration for the underlying in-memory registry.
	*RegistryConfig

	// SyncInterval is how often to sync health status to the database.
	// Default: 1 minute.
	SyncInterval time.Duration

	// Logger for logging operations. If nil, a no-op logger is used.
	Logger logging.Logger
}

// DefaultDBRegistryConfig returns a configuration with sensible defaults.
func DefaultDBRegistryConfig() *DBRegistryConfig {
	return &DBRegistryConfig{
		RegistryConfig: DefaultRegistryConfig(),
		SyncInterval:   1 * time.Minute,
	}
}

// NewDBRegistry creates a new database-backed model registry.
func NewDBRegistry(repo Repository, config *DBRegistryConfig) *DBRegistry {
	if config == nil {
		config = DefaultDBRegistryConfig()
	}

	reg := &DBRegistry{
		ModelRegistry: NewRegistry(config.RegistryConfig),
		repo:          repo,
		syncInterval:  config.SyncInterval,
		stopSync:      make(chan struct{}),
	}

	if config.Logger != nil {
		reg.logger = config.Logger.With(logging.F("component", "db_registry"))
	}

	return reg
}

// Start loads models from the database, runs discovery, and starts background tasks.
func (r *DBRegistry) Start(ctx context.Context) error {
	// Load models from database first
	if err := r.loadFromDB(ctx); err != nil {
		// Log but continue - we can still use discovered models
		// The DB might be empty on first run
	}

	// Start the underlying registry (health checks, discovery)
	if err := r.ModelRegistry.Start(ctx); err != nil {
		return err
	}

	// Start health sync loop if interval is configured
	if r.syncInterval > 0 {
		r.wg.Add(1)
		// Get stopSync channel under lock to prevent race
		r.mu.Lock()
		stopCh := r.stopSync
		r.mu.Unlock()
		go r.healthSyncLoop(ctx, stopCh)
	}

	return nil
}

// Stop halts all background tasks and syncs final state to DB.
func (r *DBRegistry) Stop() {
	// Stop health sync
	r.mu.Lock()
	if r.stopSync != nil {
		close(r.stopSync)
		r.stopSync = nil
	}
	r.mu.Unlock()

	// Wait for health sync to complete
	r.wg.Wait()

	// Stop underlying registry
	r.ModelRegistry.Stop()
}

// Register adds a model to both the database and in-memory registry.
func (r *DBRegistry) Register(model *ModelConfig) error {
	// Validate first
	if err := model.Validate(); err != nil {
		return err
	}

	// Register in memory first (sets timestamps)
	if err := r.ModelRegistry.Register(model); err != nil {
		return err
	}

	// Persist to database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.repo.CreateModel(ctx, model); err != nil {
		// Roll back in-memory registration
		_ = r.ModelRegistry.Unregister(model.ID)
		return err
	}

	return nil
}

// Update modifies a model in both the database and in-memory registry.
func (r *DBRegistry) Update(model *ModelConfig) error {
	// Get the current model for rollback
	current, err := r.ModelRegistry.Get(model.ID)
	if err != nil {
		return err
	}

	// Update in memory first
	if err := r.ModelRegistry.Update(model); err != nil {
		return err
	}

	// Persist to database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.repo.UpdateModel(ctx, model); err != nil {
		// Roll back in-memory update
		_ = r.ModelRegistry.Update(current)
		return err
	}

	return nil
}

// Unregister removes a model from both the database and in-memory registry.
func (r *DBRegistry) Unregister(modelID string) error {
	// Get the current model for rollback
	current, err := r.ModelRegistry.Get(modelID)
	if err != nil {
		return err
	}

	// Delete from database first (this cascades to health)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.repo.DeleteModel(ctx, modelID); err != nil {
		return err
	}

	// Remove from memory
	if err := r.ModelRegistry.Unregister(modelID); err != nil {
		// Try to restore to DB (best effort)
		_ = r.repo.CreateModel(ctx, current)
		return err
	}

	return nil
}

// UpdateHealth updates health status in both memory and database.
func (r *DBRegistry) UpdateHealth(modelID string, health ModelHealth) error {
	// Update in memory first
	if err := r.ModelRegistry.UpdateHealth(modelID, health); err != nil {
		return err
	}

	// Persist to database asynchronously (fire and forget for latency)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := r.repo.UpdateHealth(ctx, modelID, &health); err != nil {
			if r.logger != nil {
				r.logger.Error("Failed to sync model health to DB",
					logging.Err(err),
					logging.F("model_id", modelID))
			}
		}
	}()

	return nil
}

// GetRoutingRules retrieves routing rules from the database.
func (r *DBRegistry) GetRoutingRules(ctx context.Context) ([]*RoutingRule, error) {
	return r.repo.GetRoutingRules(ctx)
}

// GetRoutingRulesByTask retrieves routing rules for a specific task type.
func (r *DBRegistry) GetRoutingRulesByTask(ctx context.Context, taskType string) ([]*RoutingRule, error) {
	return r.repo.GetRoutingRulesByTask(ctx, taskType)
}

// GetRoutingRuleByName retrieves a routing rule by its unique name.
func (r *DBRegistry) GetRoutingRuleByName(ctx context.Context, name string) (*RoutingRule, error) {
	return r.repo.GetRoutingRuleByName(ctx, name)
}

// CreateRoutingRule creates a new routing rule in the database.
func (r *DBRegistry) CreateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	return r.repo.CreateRoutingRule(ctx, rule)
}

// UpdateRoutingRule updates an existing routing rule in the database.
func (r *DBRegistry) UpdateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	return r.repo.UpdateRoutingRule(ctx, rule)
}

// DeleteRoutingRule removes a routing rule from the database.
func (r *DBRegistry) DeleteRoutingRule(ctx context.Context, ruleID string) error {
	return r.repo.DeleteRoutingRule(ctx, ruleID)
}

// loadFromDB loads all models and their health from the database.
func (r *DBRegistry) loadFromDB(ctx context.Context) error {
	models, err := r.repo.ListModels(ctx, nil)
	if err != nil {
		return err
	}

	for _, model := range models {
		// Load health for this model
		health, err := r.repo.GetHealth(ctx, model.ID)
		if err == nil && health != nil {
			model.Health = *health
		}

		// Register in memory (ignore already exists errors from discovery)
		if err := r.ModelRegistry.Register(model); err != nil && err != ErrModelAlreadyExists {
			// Log but continue loading other models
			continue
		}
	}

	return nil
}

// healthSyncLoop periodically syncs health status to the database.
func (r *DBRegistry) healthSyncLoop(ctx context.Context, stopCh <-chan struct{}) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.syncHealthToDB(context.Background())
			return
		case <-stopCh:
			r.syncHealthToDB(context.Background())
			return
		case <-ticker.C:
			r.syncHealthToDB(ctx)
		}
	}
}

// syncHealthToDB writes all in-memory health status to the database.
func (r *DBRegistry) syncHealthToDB(ctx context.Context) {
	models := r.ModelRegistry.List(nil)

	for _, model := range models {
		syncCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = r.repo.UpdateHealth(syncCtx, model.ID, &model.Health)
		cancel()
	}
}

// MergeDiscoveredModels adds auto-discovered models to the database.
// This should be called after model discovery to persist new models.
func (r *DBRegistry) MergeDiscoveredModels(ctx context.Context) error {
	// Get all in-memory models
	models := r.ModelRegistry.List(nil)

	for _, model := range models {
		// Try to create in DB (will fail if already exists)
		err := r.repo.CreateModel(ctx, model)
		if err != nil {
			// Ignore errors - model likely already exists
			continue
		}
	}

	return nil
}

// Reload refreshes the in-memory registry from the database.
func (r *DBRegistry) Reload(ctx context.Context) error {
	// Clear current models
	models := r.ModelRegistry.List(nil)
	for _, m := range models {
		_ = r.ModelRegistry.Unregister(m.ID)
	}

	// Reload from DB
	return r.loadFromDB(ctx)
}
