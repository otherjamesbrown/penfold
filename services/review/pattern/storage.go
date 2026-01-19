// Package pattern provides pattern persistence for the review service.
package pattern

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// PatternStore defines the interface for pattern persistence.
type PatternStore interface {
	// Save persists a pattern.
	Save(ctx context.Context, pattern *Pattern) error

	// Get retrieves a pattern by ID.
	Get(ctx context.Context, tenantID, patternID string) (*Pattern, error)

	// List retrieves patterns for a tenant with pagination.
	List(ctx context.Context, tenantID string, offset, limit int) ([]*Pattern, error)

	// Delete removes a pattern.
	Delete(ctx context.Context, tenantID, patternID string) error

	// Update updates an existing pattern.
	Update(ctx context.Context, pattern *Pattern) error

	// GetByType retrieves patterns of a specific type.
	GetByType(ctx context.Context, tenantID string, patternType PatternType) ([]*Pattern, error)

	// GetEnabled retrieves all enabled patterns for a tenant.
	GetEnabled(ctx context.Context, tenantID string) ([]*Pattern, error)

	// GetHotPatterns retrieves frequently matched patterns (for caching).
	GetHotPatterns(ctx context.Context, tenantID string, minMatches int64) ([]*Pattern, error)
}

// InMemoryPatternStore implements PatternStore for testing and development.
type InMemoryPatternStore struct {
	patterns map[string]map[string]*Pattern // tenantID -> patternID -> Pattern
	mu       sync.RWMutex
}

// NewInMemoryPatternStore creates a new in-memory pattern store.
func NewInMemoryPatternStore() *InMemoryPatternStore {
	return &InMemoryPatternStore{
		patterns: make(map[string]map[string]*Pattern),
	}
}

// Save persists a pattern.
func (s *InMemoryPatternStore) Save(ctx context.Context, pattern *Pattern) error {
	if pattern == nil {
		return ErrInvalidPattern
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.patterns[pattern.TenantID]; !exists {
		s.patterns[pattern.TenantID] = make(map[string]*Pattern)
	}

	// Check for duplicate
	if _, exists := s.patterns[pattern.TenantID][pattern.ID]; exists {
		return ErrPatternExists
	}

	s.patterns[pattern.TenantID][pattern.ID] = pattern.Clone()
	return nil
}

// Get retrieves a pattern by ID.
func (s *InMemoryPatternStore) Get(ctx context.Context, tenantID, patternID string) (*Pattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantPatterns, exists := s.patterns[tenantID]
	if !exists {
		return nil, ErrPatternNotFound
	}

	pattern, exists := tenantPatterns[patternID]
	if !exists {
		return nil, ErrPatternNotFound
	}

	return pattern.Clone(), nil
}

// List retrieves patterns for a tenant with pagination.
func (s *InMemoryPatternStore) List(ctx context.Context, tenantID string, offset, limit int) ([]*Pattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantPatterns, exists := s.patterns[tenantID]
	if !exists {
		return []*Pattern{}, nil
	}

	// Convert map to slice
	all := make([]*Pattern, 0, len(tenantPatterns))
	for _, p := range tenantPatterns {
		all = append(all, p.Clone())
	}

	// Apply pagination
	if offset >= len(all) {
		return []*Pattern{}, nil
	}

	end := offset + limit
	if end > len(all) || limit <= 0 {
		end = len(all)
	}

	return all[offset:end], nil
}

// Delete removes a pattern.
func (s *InMemoryPatternStore) Delete(ctx context.Context, tenantID, patternID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tenantPatterns, exists := s.patterns[tenantID]
	if !exists {
		return ErrPatternNotFound
	}

	if _, exists := tenantPatterns[patternID]; !exists {
		return ErrPatternNotFound
	}

	delete(tenantPatterns, patternID)
	return nil
}

// Update updates an existing pattern.
func (s *InMemoryPatternStore) Update(ctx context.Context, pattern *Pattern) error {
	if pattern == nil {
		return ErrInvalidPattern
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tenantPatterns, exists := s.patterns[pattern.TenantID]
	if !exists {
		return ErrPatternNotFound
	}

	if _, exists := tenantPatterns[pattern.ID]; !exists {
		return ErrPatternNotFound
	}

	pattern.UpdatedAt = time.Now().UTC()
	tenantPatterns[pattern.ID] = pattern.Clone()
	return nil
}

// GetByType retrieves patterns of a specific type.
func (s *InMemoryPatternStore) GetByType(ctx context.Context, tenantID string, patternType PatternType) ([]*Pattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantPatterns, exists := s.patterns[tenantID]
	if !exists {
		return []*Pattern{}, nil
	}

	result := make([]*Pattern, 0)
	for _, p := range tenantPatterns {
		if p.Type == patternType {
			result = append(result, p.Clone())
		}
	}

	return result, nil
}

// GetEnabled retrieves all enabled patterns for a tenant.
func (s *InMemoryPatternStore) GetEnabled(ctx context.Context, tenantID string) ([]*Pattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantPatterns, exists := s.patterns[tenantID]
	if !exists {
		return []*Pattern{}, nil
	}

	result := make([]*Pattern, 0)
	for _, p := range tenantPatterns {
		if p.Enabled {
			result = append(result, p.Clone())
		}
	}

	return result, nil
}

// GetHotPatterns retrieves frequently matched patterns.
func (s *InMemoryPatternStore) GetHotPatterns(ctx context.Context, tenantID string, minMatches int64) ([]*Pattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantPatterns, exists := s.patterns[tenantID]
	if !exists {
		return []*Pattern{}, nil
	}

	result := make([]*Pattern, 0)
	for _, p := range tenantPatterns {
		if p.Stats.TotalMatches >= minMatches {
			result = append(result, p.Clone())
		}
	}

	return result, nil
}

// PostgresPatternStore implements PatternStore using PostgreSQL.
type PostgresPatternStore struct {
	db *sql.DB
}

// NewPostgresPatternStore creates a new PostgreSQL pattern store.
func NewPostgresPatternStore(db *sql.DB) *PostgresPatternStore {
	return &PostgresPatternStore{db: db}
}

// Save persists a pattern.
func (s *PostgresPatternStore) Save(ctx context.Context, pattern *Pattern) error {
	if pattern == nil {
		return ErrInvalidPattern
	}

	conditionsJSON, err := json.Marshal(pattern.Conditions)
	if err != nil {
		return err
	}

	examplesJSON, err := json.Marshal(pattern.Examples)
	if err != nil {
		return err
	}

	statsJSON, err := json.Marshal(pattern.Stats)
	if err != nil {
		return err
	}

	tagsJSON, err := json.Marshal(pattern.Tags)
	if err != nil {
		return err
	}

	metadataJSON, err := json.Marshal(pattern.Metadata)
	if err != nil {
		return err
	}

	relatedJSON, err := json.Marshal(pattern.RelatedPatterns)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO patterns (
			id, tenant_id, name, description, type, conditions, frequency, confidence,
			suggested_action, examples, stats, enabled, auto_generated, source, priority,
			tags, related_patterns, metadata, created_at, updated_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`

	_, err = s.db.ExecContext(ctx, query,
		pattern.ID,
		pattern.TenantID,
		pattern.Name,
		pattern.Description,
		pattern.Type,
		conditionsJSON,
		pattern.Frequency,
		pattern.Confidence,
		pattern.SuggestedAction,
		examplesJSON,
		statsJSON,
		pattern.Enabled,
		pattern.AutoGenerated,
		pattern.Source,
		pattern.Priority,
		tagsJSON,
		relatedJSON,
		metadataJSON,
		pattern.CreatedAt,
		pattern.UpdatedAt,
		pattern.CreatedBy,
	)

	if err != nil {
		// Check for unique constraint violation
		if isUniqueViolation(err) {
			return ErrPatternExists
		}
		return err
	}

	return nil
}

// Get retrieves a pattern by ID.
func (s *PostgresPatternStore) Get(ctx context.Context, tenantID, patternID string) (*Pattern, error) {
	query := `
		SELECT id, tenant_id, name, description, type, conditions, frequency, confidence,
			   suggested_action, examples, stats, enabled, auto_generated, source, priority,
			   tags, related_patterns, metadata, created_at, updated_at, created_by
		FROM patterns
		WHERE tenant_id = $1 AND id = $2
	`

	row := s.db.QueryRowContext(ctx, query, tenantID, patternID)
	return s.scanPattern(row)
}

// List retrieves patterns for a tenant with pagination.
func (s *PostgresPatternStore) List(ctx context.Context, tenantID string, offset, limit int) ([]*Pattern, error) {
	query := `
		SELECT id, tenant_id, name, description, type, conditions, frequency, confidence,
			   suggested_action, examples, stats, enabled, auto_generated, source, priority,
			   tags, related_patterns, metadata, created_at, updated_at, created_by
		FROM patterns
		WHERE tenant_id = $1
		ORDER BY priority DESC, created_at DESC
		OFFSET $2 LIMIT $3
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanPatterns(rows)
}

// Delete removes a pattern.
func (s *PostgresPatternStore) Delete(ctx context.Context, tenantID, patternID string) error {
	query := `DELETE FROM patterns WHERE tenant_id = $1 AND id = $2`

	result, err := s.db.ExecContext(ctx, query, tenantID, patternID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrPatternNotFound
	}

	return nil
}

// Update updates an existing pattern.
func (s *PostgresPatternStore) Update(ctx context.Context, pattern *Pattern) error {
	if pattern == nil {
		return ErrInvalidPattern
	}

	pattern.UpdatedAt = time.Now().UTC()

	conditionsJSON, err := json.Marshal(pattern.Conditions)
	if err != nil {
		return err
	}

	examplesJSON, err := json.Marshal(pattern.Examples)
	if err != nil {
		return err
	}

	statsJSON, err := json.Marshal(pattern.Stats)
	if err != nil {
		return err
	}

	tagsJSON, err := json.Marshal(pattern.Tags)
	if err != nil {
		return err
	}

	metadataJSON, err := json.Marshal(pattern.Metadata)
	if err != nil {
		return err
	}

	relatedJSON, err := json.Marshal(pattern.RelatedPatterns)
	if err != nil {
		return err
	}

	query := `
		UPDATE patterns SET
			name = $3,
			description = $4,
			type = $5,
			conditions = $6,
			frequency = $7,
			confidence = $8,
			suggested_action = $9,
			examples = $10,
			stats = $11,
			enabled = $12,
			auto_generated = $13,
			source = $14,
			priority = $15,
			tags = $16,
			related_patterns = $17,
			metadata = $18,
			updated_at = $19
		WHERE tenant_id = $1 AND id = $2
	`

	result, err := s.db.ExecContext(ctx, query,
		pattern.TenantID,
		pattern.ID,
		pattern.Name,
		pattern.Description,
		pattern.Type,
		conditionsJSON,
		pattern.Frequency,
		pattern.Confidence,
		pattern.SuggestedAction,
		examplesJSON,
		statsJSON,
		pattern.Enabled,
		pattern.AutoGenerated,
		pattern.Source,
		pattern.Priority,
		tagsJSON,
		relatedJSON,
		metadataJSON,
		pattern.UpdatedAt,
	)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrPatternNotFound
	}

	return nil
}

// GetByType retrieves patterns of a specific type.
func (s *PostgresPatternStore) GetByType(ctx context.Context, tenantID string, patternType PatternType) ([]*Pattern, error) {
	query := `
		SELECT id, tenant_id, name, description, type, conditions, frequency, confidence,
			   suggested_action, examples, stats, enabled, auto_generated, source, priority,
			   tags, related_patterns, metadata, created_at, updated_at, created_by
		FROM patterns
		WHERE tenant_id = $1 AND type = $2
		ORDER BY priority DESC, created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, patternType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanPatterns(rows)
}

// GetEnabled retrieves all enabled patterns for a tenant.
func (s *PostgresPatternStore) GetEnabled(ctx context.Context, tenantID string) ([]*Pattern, error) {
	query := `
		SELECT id, tenant_id, name, description, type, conditions, frequency, confidence,
			   suggested_action, examples, stats, enabled, auto_generated, source, priority,
			   tags, related_patterns, metadata, created_at, updated_at, created_by
		FROM patterns
		WHERE tenant_id = $1 AND enabled = true
		ORDER BY priority DESC, created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanPatterns(rows)
}

// GetHotPatterns retrieves frequently matched patterns.
func (s *PostgresPatternStore) GetHotPatterns(ctx context.Context, tenantID string, minMatches int64) ([]*Pattern, error) {
	query := `
		SELECT id, tenant_id, name, description, type, conditions, frequency, confidence,
			   suggested_action, examples, stats, enabled, auto_generated, source, priority,
			   tags, related_patterns, metadata, created_at, updated_at, created_by
		FROM patterns
		WHERE tenant_id = $1
			AND enabled = true
			AND (stats->>'total_matches')::bigint >= $2
		ORDER BY (stats->>'total_matches')::bigint DESC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, minMatches)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanPatterns(rows)
}

// scanPattern scans a single row into a Pattern.
func (s *PostgresPatternStore) scanPattern(row *sql.Row) (*Pattern, error) {
	var pattern Pattern
	var conditionsJSON, examplesJSON, statsJSON, tagsJSON, relatedJSON, metadataJSON []byte
	var description, suggestedAction, source, createdBy sql.NullString

	err := row.Scan(
		&pattern.ID,
		&pattern.TenantID,
		&pattern.Name,
		&description,
		&pattern.Type,
		&conditionsJSON,
		&pattern.Frequency,
		&pattern.Confidence,
		&suggestedAction,
		&examplesJSON,
		&statsJSON,
		&pattern.Enabled,
		&pattern.AutoGenerated,
		&source,
		&pattern.Priority,
		&tagsJSON,
		&relatedJSON,
		&metadataJSON,
		&pattern.CreatedAt,
		&pattern.UpdatedAt,
		&createdBy,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPatternNotFound
		}
		return nil, err
	}

	if description.Valid {
		pattern.Description = description.String
	}
	if suggestedAction.Valid {
		pattern.SuggestedAction = suggestedAction.String
	}
	if source.Valid {
		pattern.Source = source.String
	}
	if createdBy.Valid {
		pattern.CreatedBy = createdBy.String
	}

	if err := json.Unmarshal(conditionsJSON, &pattern.Conditions); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(examplesJSON, &pattern.Examples); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(statsJSON, &pattern.Stats); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(tagsJSON, &pattern.Tags); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(relatedJSON, &pattern.RelatedPatterns); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(metadataJSON, &pattern.Metadata); err != nil {
		return nil, err
	}

	return &pattern, nil
}

// scanPatterns scans multiple rows into Patterns.
func (s *PostgresPatternStore) scanPatterns(rows *sql.Rows) ([]*Pattern, error) {
	patterns := make([]*Pattern, 0)

	for rows.Next() {
		var pattern Pattern
		var conditionsJSON, examplesJSON, statsJSON, tagsJSON, relatedJSON, metadataJSON []byte
		var description, suggestedAction, source, createdBy sql.NullString

		err := rows.Scan(
			&pattern.ID,
			&pattern.TenantID,
			&pattern.Name,
			&description,
			&pattern.Type,
			&conditionsJSON,
			&pattern.Frequency,
			&pattern.Confidence,
			&suggestedAction,
			&examplesJSON,
			&statsJSON,
			&pattern.Enabled,
			&pattern.AutoGenerated,
			&source,
			&pattern.Priority,
			&tagsJSON,
			&relatedJSON,
			&metadataJSON,
			&pattern.CreatedAt,
			&pattern.UpdatedAt,
			&createdBy,
		)

		if err != nil {
			return nil, err
		}

		if description.Valid {
			pattern.Description = description.String
		}
		if suggestedAction.Valid {
			pattern.SuggestedAction = suggestedAction.String
		}
		if source.Valid {
			pattern.Source = source.String
		}
		if createdBy.Valid {
			pattern.CreatedBy = createdBy.String
		}

		if err := json.Unmarshal(conditionsJSON, &pattern.Conditions); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(examplesJSON, &pattern.Examples); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(statsJSON, &pattern.Stats); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tagsJSON, &pattern.Tags); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(relatedJSON, &pattern.RelatedPatterns); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadataJSON, &pattern.Metadata); err != nil {
			return nil, err
		}

		patterns = append(patterns, &pattern)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return patterns, nil
}

// isUniqueViolation checks if an error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL unique violation error code is 23505
	return containsString(err.Error(), "23505") || containsString(err.Error(), "unique")
}

// containsString is a simple string contains check.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

// findSubstring finds a substring in a string.
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CachedPatternStore wraps a PatternStore with caching for hot patterns.
type CachedPatternStore struct {
	store      PatternStore
	cache      map[string]*Pattern // patternID -> Pattern
	tenantHot  map[string][]string // tenantID -> list of hot pattern IDs
	ttl        time.Duration
	lastUpdate time.Time
	mu         sync.RWMutex
}

// NewCachedPatternStore creates a new cached pattern store.
func NewCachedPatternStore(store PatternStore, ttl time.Duration) *CachedPatternStore {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	return &CachedPatternStore{
		store:     store,
		cache:     make(map[string]*Pattern),
		tenantHot: make(map[string][]string),
		ttl:       ttl,
	}
}

// Save persists a pattern and invalidates cache.
func (c *CachedPatternStore) Save(ctx context.Context, pattern *Pattern) error {
	err := c.store.Save(ctx, pattern)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.cache[pattern.ID] = pattern.Clone()
	c.mu.Unlock()

	return nil
}

// Get retrieves a pattern, checking cache first.
func (c *CachedPatternStore) Get(ctx context.Context, tenantID, patternID string) (*Pattern, error) {
	c.mu.RLock()
	cached, exists := c.cache[patternID]
	c.mu.RUnlock()

	if exists && cached.TenantID == tenantID {
		return cached.Clone(), nil
	}

	pattern, err := c.store.Get(ctx, tenantID, patternID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cache[patternID] = pattern.Clone()
	c.mu.Unlock()

	return pattern, nil
}

// List retrieves patterns from the underlying store.
func (c *CachedPatternStore) List(ctx context.Context, tenantID string, offset, limit int) ([]*Pattern, error) {
	return c.store.List(ctx, tenantID, offset, limit)
}

// Delete removes a pattern and invalidates cache.
func (c *CachedPatternStore) Delete(ctx context.Context, tenantID, patternID string) error {
	err := c.store.Delete(ctx, tenantID, patternID)
	if err != nil {
		return err
	}

	c.mu.Lock()
	delete(c.cache, patternID)
	c.mu.Unlock()

	return nil
}

// Update updates a pattern and updates cache.
func (c *CachedPatternStore) Update(ctx context.Context, pattern *Pattern) error {
	err := c.store.Update(ctx, pattern)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.cache[pattern.ID] = pattern.Clone()
	c.mu.Unlock()

	return nil
}

// GetByType retrieves patterns of a specific type.
func (c *CachedPatternStore) GetByType(ctx context.Context, tenantID string, patternType PatternType) ([]*Pattern, error) {
	return c.store.GetByType(ctx, tenantID, patternType)
}

// GetEnabled retrieves all enabled patterns.
func (c *CachedPatternStore) GetEnabled(ctx context.Context, tenantID string) ([]*Pattern, error) {
	return c.store.GetEnabled(ctx, tenantID)
}

// GetHotPatterns retrieves hot patterns, using cache when possible.
func (c *CachedPatternStore) GetHotPatterns(ctx context.Context, tenantID string, minMatches int64) ([]*Pattern, error) {
	c.mu.RLock()
	needsRefresh := time.Since(c.lastUpdate) > c.ttl
	hotIDs := c.tenantHot[tenantID]
	c.mu.RUnlock()

	if !needsRefresh && len(hotIDs) > 0 {
		// Return from cache
		patterns := make([]*Pattern, 0, len(hotIDs))
		c.mu.RLock()
		for _, id := range hotIDs {
			if p, exists := c.cache[id]; exists {
				patterns = append(patterns, p.Clone())
			}
		}
		c.mu.RUnlock()
		return patterns, nil
	}

	// Fetch from store
	patterns, err := c.store.GetHotPatterns(ctx, tenantID, minMatches)
	if err != nil {
		return nil, err
	}

	// Update cache
	c.mu.Lock()
	hotIDs = make([]string, 0, len(patterns))
	for _, p := range patterns {
		c.cache[p.ID] = p.Clone()
		hotIDs = append(hotIDs, p.ID)
	}
	c.tenantHot[tenantID] = hotIDs
	c.lastUpdate = time.Now()
	c.mu.Unlock()

	return patterns, nil
}

// InvalidateCache clears the cache.
func (c *CachedPatternStore) InvalidateCache() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*Pattern)
	c.tenantHot = make(map[string][]string)
	c.lastUpdate = time.Time{}
}
