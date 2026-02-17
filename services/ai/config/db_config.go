package config

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ModelConfigQuerier defines the interface for querying model configuration from the database.
// This matches the Repository pattern from pkg/models/config_repository.go.
type ModelConfigQuerier interface {
	GetModelConfig(ctx context.Context, tenantID uuid.UUID, configKey string) (string, error)
}

// DBConfigResolver wraps the existing Config struct and adds DB-backed dynamic config resolution.
// It implements a 5-level priority chain for model resolution:
//
// 1. DB override: model_config where config_key = "stage.<stage>" for tenant
// 2. Per-stage env var: existing AI_MODEL_TRIAGE etc.
// 3. DB default: model_config where config_key = "default.llm" (or "default.embedding" for embedding stage)
// 4. Global env var default: existing AI_DEFAULT_LLM_MODEL / AI_DEFAULT_EMBEDDING_MODEL
// 5. Hardcoded fallback: DefaultLLMModel / DefaultEmbeddingModel constants
//
// Results are cached with a configurable TTL (default 30s) so config changes take effect without restart.
type DBConfigResolver struct {
	db       ModelConfigQuerier
	baseCfg  *Config
	tenantID uuid.UUID
	cache    sync.Map // configKey -> cachedEntry
	ttl      time.Duration
	logger   logging.Logger
}

// cachedEntry holds a cached model name with its fetch timestamp.
type cachedEntry struct {
	value     string
	fetchedAt time.Time
}

const (
	// DefaultCacheTTL is the default cache duration for DB config entries.
	DefaultCacheTTL = 30 * time.Second
)

// NewDBConfigResolver creates a DBConfigResolver with the default 30s cache TTL.
// If db is nil, it falls back to env-var-only resolution (backward compatible).
func NewDBConfigResolver(db ModelConfigQuerier, baseCfg *Config, tenantID uuid.UUID) *DBConfigResolver {
	return NewDBConfigResolverWithTTL(db, baseCfg, tenantID, DefaultCacheTTL)
}

// NewDBConfigResolverWithTTL creates a DBConfigResolver with a custom cache TTL.
// This is primarily for testing (shorter TTL for faster tests).
func NewDBConfigResolverWithTTL(db ModelConfigQuerier, baseCfg *Config, tenantID uuid.UUID, ttl time.Duration) *DBConfigResolver {
	resolver := &DBConfigResolver{
		db:       db,
		baseCfg:  baseCfg,
		tenantID: tenantID,
		ttl:      ttl,
	}

	// Try to get global logger, but don't panic if not initialized (e.g., in tests)
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Global logger not initialized, use nil (we'll check before logging)
				resolver.logger = nil
			}
		}()
		resolver.logger = logging.Global()
	}()

	return resolver
}

// ModelForStage implements the 5-level priority chain for model resolution.
// Resolution order for embedding stage:
// 1. DB override (stage.embedding)
// 2. Per-stage env var (AI_MODEL_EMBEDDING)
// 3. DB default (default.embedding)
// 4. Global env var (AI_DEFAULT_EMBEDDING_MODEL)
// 5. Hardcoded fallback (DefaultEmbeddingModel)
//
// Resolution order for other stages (triage, extract_entities, extract_assertions, deep_analyze):
// 1. DB override (stage.<stage>)
// 2. Per-stage env var (AI_MODEL_<STAGE>)
// 3. DB default (default.llm)
// 4. Global env var (AI_DEFAULT_LLM_MODEL)
// 5. Hardcoded fallback (DefaultLLMModel)
func (r *DBConfigResolver) ModelForStage(ctx context.Context, stage string) (string, error) {
	// Normalize stage name to lowercase
	stage = strings.ToLower(strings.TrimSpace(stage))

	// Level 1: Check DB override for stage.<stage>
	if r.db != nil {
		stageKey := fmt.Sprintf("stage.%s", stage)
		if model := r.getCachedOrFetch(ctx, stageKey); model != "" {
			return model, nil
		}
	}

	// Level 2: Check per-stage env var (existing behavior from baseCfg.StageModels)
	if model, ok := r.baseCfg.StageModels[stage]; ok && model != "" {
		return model, nil
	}

	// Level 3: Check DB default (default.llm for LLM stages, default.embedding for embedding stage)
	if r.db != nil {
		defaultKey := "default.llm"
		if stage == "embedding" {
			defaultKey = "default.embedding"
		}
		if model := r.getCachedOrFetch(ctx, defaultKey); model != "" {
			return model, nil
		}
	}

	// Level 4: Check global env var default
	if stage == "embedding" {
		if r.baseCfg.DefaultEmbeddingModel != "" {
			return r.baseCfg.DefaultEmbeddingModel, nil
		}
	} else {
		if r.baseCfg.DefaultLLMModel != "" {
			return r.baseCfg.DefaultLLMModel, nil
		}
	}

	// Level 5: Hardcoded fallback
	if stage == "embedding" {
		return DefaultEmbeddingModel, nil
	}
	return DefaultLLMModel, nil
}

// getCachedOrFetch checks cache first, then queries DB if cache is stale or missing.
// Returns empty string if DB query fails or config not found (caller should fall through to next level).
func (r *DBConfigResolver) getCachedOrFetch(ctx context.Context, configKey string) string {
	// Check cache
	if entry, ok := r.cache.Load(configKey); ok {
		cached := entry.(cachedEntry)
		if time.Since(cached.fetchedAt) < r.ttl {
			return cached.value
		}
		// Cache expired, will fetch fresh
	}

	// Query DB
	modelName, err := r.db.GetModelConfig(ctx, r.tenantID, configKey)
	if err != nil {
		// DB error or not found - log and fall through to next level
		// Only log at debug level to avoid spam for expected "not found" cases
		if r.logger != nil && !errors.Is(err, errors.New("model config not found")) && !strings.Contains(err.Error(), "not found") {
			r.logger.Debug("DB model config query failed, falling back",
				logging.F("config_key", configKey),
				logging.Err(err),
			)
		}
		return ""
	}

	// Cache the result
	r.cache.Store(configKey, cachedEntry{
		value:     modelName,
		fetchedAt: time.Now(),
	})

	return modelName
}

// Config returns the underlying base Config for accessing other configuration fields.
// This allows DBConfigResolver to be used as a drop-in replacement for Config.
func (r *DBConfigResolver) Config() *Config {
	return r.baseCfg
}
