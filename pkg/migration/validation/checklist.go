// Package validation provides migration validation checklist functionality.
package validation

import (
	"context"
	"sync"
	"time"
)

// CheckCategory represents a category of migration checks.
type CheckCategory string

const (
	// CategoryService represents service-level checks.
	CategoryService CheckCategory = "service"
	// CategoryEndpoint represents endpoint checks.
	CategoryEndpoint CheckCategory = "endpoint"
	// CategoryData represents data integrity checks.
	CategoryData CheckCategory = "data"
	// CategoryPerformance represents performance checks.
	CategoryPerformance CheckCategory = "performance"
)

// MigrationCheck represents a single migration checklist item.
type MigrationCheck struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Category    CheckCategory `json:"category"`
	Critical    bool          `json:"critical"`
	TargetMs    int64         `json:"target_ms,omitempty"`
	CheckFunc   CheckFunc     `json:"-"`
}

// CheckFunc is the function signature for checklist check implementations.
type CheckFunc func(ctx context.Context) ValidationResult

// Checklist manages the migration validation checklist.
type Checklist struct {
	mu     sync.RWMutex
	checks map[string]*MigrationCheck
	order  []string // maintains order of checks
}

// NewChecklist creates a new migration checklist with default checks.
func NewChecklist() *Checklist {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	// Register default service checks.
	cl.registerServiceChecks()

	// Register endpoint checks.
	cl.registerEndpointChecks()

	// Register data checks.
	cl.registerDataChecks()

	// Register performance checks.
	cl.registerPerformanceChecks()

	return cl
}

// registerServiceChecks adds service-level checks to the checklist.
func (cl *Checklist) registerServiceChecks() {
	services := []struct {
		name     string
		critical bool
	}{
		{"gateway", true},
		{"search", true},
		{"gmail", false},
		{"content", true},
		{"ai", true},
		{"review", false},
		{"relationship", false},
	}

	for _, svc := range services {
		check := &MigrationCheck{
			ID:          "svc_" + svc.name,
			Name:        svc.name + "_service",
			Description: "Verify " + svc.name + " service is running and responsive",
			Category:    CategoryService,
			Critical:    svc.critical,
		}
		cl.AddCheck(check)
	}
}

// registerEndpointChecks adds endpoint checks to the checklist.
func (cl *Checklist) registerEndpointChecks() {
	endpoints := []struct {
		id          string
		name        string
		description string
		critical    bool
	}{
		{"ep_gateway_status", "gateway_status", "Gateway GetStatus endpoint responds", true},
		{"ep_search_query", "search_query", "Search Query endpoint responds", true},
		{"ep_search_hybrid", "search_hybrid", "Search HybridSearch endpoint responds", true},
		{"ep_gmail_sync", "gmail_sync", "Gmail Sync endpoint responds", false},
		{"ep_gmail_list", "gmail_list", "Gmail ListMessages endpoint responds", false},
		{"ep_content_ingest", "content_ingest", "Content Ingest endpoint responds", true},
		{"ep_content_get", "content_get", "Content GetContent endpoint responds", true},
		{"ep_ai_generate", "ai_generate", "AI Generate endpoint responds", true},
		{"ep_ai_embed", "ai_embed", "AI CreateEmbedding endpoint responds", true},
		{"ep_review_daily", "review_daily", "Review GetDailyReview endpoint responds", false},
		{"ep_relationship_discover", "relationship_discover", "Relationship Discover endpoint responds", false},
	}

	for _, ep := range endpoints {
		check := &MigrationCheck{
			ID:          ep.id,
			Name:        ep.name,
			Description: ep.description,
			Category:    CategoryEndpoint,
			Critical:    ep.critical,
		}
		cl.AddCheck(check)
	}
}

// registerDataChecks adds data integrity checks to the checklist.
func (cl *Checklist) registerDataChecks() {
	dataChecks := []struct {
		id          string
		name        string
		description string
		critical    bool
	}{
		{"data_db_connect", "database_connection", "Database is accessible", true},
		{"data_db_schema", "database_schema", "Database schema is valid", true},
		{"data_db_vector", "vector_extension", "pgvector extension is installed", true},
		{"data_content_count", "content_count", "Content items are preserved", true},
		{"data_entity_count", "entity_count", "Entities are preserved", false},
		{"data_relationship_count", "relationship_count", "Relationships are preserved", false},
		{"data_embedding_integrity", "embedding_integrity", "Embeddings are valid", true},
	}

	for _, dc := range dataChecks {
		check := &MigrationCheck{
			ID:          dc.id,
			Name:        dc.name,
			Description: dc.description,
			Category:    CategoryData,
			Critical:    dc.critical,
		}
		cl.AddCheck(check)
	}
}

// registerPerformanceChecks adds performance checks to the checklist.
func (cl *Checklist) registerPerformanceChecks() {
	perfChecks := []struct {
		id          string
		name        string
		description string
		targetMs    int64
		critical    bool
	}{
		{"perf_search_latency", "search_latency", "Search query latency within bounds", 500, true},
		{"perf_embed_latency", "embedding_latency", "Embedding generation latency within bounds", 1000, false},
		{"perf_db_query_latency", "db_query_latency", "Database query latency within bounds", 100, true},
		{"perf_grpc_roundtrip", "grpc_roundtrip", "gRPC roundtrip latency within bounds", 50, true},
		{"perf_http_health", "http_health_latency", "HTTP health check latency within bounds", 100, false},
	}

	for _, pc := range perfChecks {
		check := &MigrationCheck{
			ID:          pc.id,
			Name:        pc.name,
			Description: pc.description,
			Category:    CategoryPerformance,
			TargetMs:    pc.targetMs,
			Critical:    pc.critical,
		}
		cl.AddCheck(check)
	}
}

// AddCheck adds a check to the checklist.
func (cl *Checklist) AddCheck(check *MigrationCheck) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	cl.checks[check.ID] = check
	cl.order = append(cl.order, check.ID)
}

// RemoveCheck removes a check from the checklist.
func (cl *Checklist) RemoveCheck(id string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	delete(cl.checks, id)

	// Remove from order slice.
	for i, oid := range cl.order {
		if oid == id {
			cl.order = append(cl.order[:i], cl.order[i+1:]...)
			break
		}
	}
}

// GetCheck retrieves a check by ID.
func (cl *Checklist) GetCheck(id string) (*MigrationCheck, bool) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	check, ok := cl.checks[id]
	return check, ok
}

// GetAllChecks returns all checks in order.
func (cl *Checklist) GetAllChecks() []*MigrationCheck {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	checks := make([]*MigrationCheck, 0, len(cl.order))
	for _, id := range cl.order {
		if check, ok := cl.checks[id]; ok {
			checks = append(checks, check)
		}
	}
	return checks
}

// GetChecksByCategory returns all checks in a specific category.
func (cl *Checklist) GetChecksByCategory(category CheckCategory) []*MigrationCheck {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	checks := make([]*MigrationCheck, 0)
	for _, id := range cl.order {
		if check, ok := cl.checks[id]; ok && check.Category == category {
			checks = append(checks, check)
		}
	}
	return checks
}

// GetCriticalChecks returns all critical checks.
func (cl *Checklist) GetCriticalChecks() []*MigrationCheck {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	checks := make([]*MigrationCheck, 0)
	for _, id := range cl.order {
		if check, ok := cl.checks[id]; ok && check.Critical {
			checks = append(checks, check)
		}
	}
	return checks
}

// ChecklistStatus represents the overall status of the checklist.
type ChecklistStatus struct {
	TotalChecks     int                       `json:"total_checks"`
	PassedChecks    int                       `json:"passed_checks"`
	FailedChecks    int                       `json:"failed_checks"`
	WarningChecks   int                       `json:"warning_checks"`
	SkippedChecks   int                       `json:"skipped_checks"`
	CriticalPassed  int                       `json:"critical_passed"`
	CriticalFailed  int                       `json:"critical_failed"`
	ByCategory      map[CheckCategory]*CategoryStatus `json:"by_category"`
	OverallStatus   CheckStatus               `json:"overall_status"`
	CompletedAt     time.Time                 `json:"completed_at"`
}

// CategoryStatus represents the status of checks in a category.
type CategoryStatus struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Warning int `json:"warning"`
	Skipped int `json:"skipped"`
}

// Run executes all checks in the checklist and returns the status.
func (cl *Checklist) Run(ctx context.Context, validator *Validator) (*ChecklistStatus, map[string]ValidationResult) {
	results := make(map[string]ValidationResult)
	status := &ChecklistStatus{
		ByCategory: make(map[CheckCategory]*CategoryStatus),
	}

	// Initialize category statuses.
	for _, cat := range []CheckCategory{CategoryService, CategoryEndpoint, CategoryData, CategoryPerformance} {
		status.ByCategory[cat] = &CategoryStatus{}
	}

	checks := cl.GetAllChecks()
	status.TotalChecks = len(checks)

	// Run each check.
	for _, check := range checks {
		var result ValidationResult

		// If check has a custom function, use it.
		if check.CheckFunc != nil {
			result = check.CheckFunc(ctx)
		} else {
			// Use default validation based on category.
			result = cl.runDefaultCheck(ctx, validator, check)
		}

		result.Name = check.ID
		results[check.ID] = result

		// Update status counts.
		catStatus := status.ByCategory[check.Category]
		catStatus.Total++

		switch result.Status {
		case StatusPass:
			status.PassedChecks++
			catStatus.Passed++
			if check.Critical {
				status.CriticalPassed++
			}
		case StatusFail:
			status.FailedChecks++
			catStatus.Failed++
			if check.Critical {
				status.CriticalFailed++
			}
		case StatusWarn:
			status.WarningChecks++
			catStatus.Warning++
		case StatusSkip:
			status.SkippedChecks++
			catStatus.Skipped++
		}
	}

	// Determine overall status.
	if status.CriticalFailed > 0 {
		status.OverallStatus = StatusFail
	} else if status.FailedChecks > 0 || status.WarningChecks > 0 {
		status.OverallStatus = StatusWarn
	} else {
		status.OverallStatus = StatusPass
	}

	status.CompletedAt = time.Now()

	return status, results
}

// runDefaultCheck runs the default check based on category.
func (cl *Checklist) runDefaultCheck(ctx context.Context, validator *Validator, check *MigrationCheck) ValidationResult {
	start := time.Now()
	result := ValidationResult{
		Name:      check.ID,
		Timestamp: time.Now(),
	}

	switch check.Category {
	case CategoryService:
		// Service checks are handled by the validator's ValidateServices.
		// Return a placeholder result.
		result.Status = StatusWarn
		result.Message = "Service check should be run through validator.ValidateServices()"

	case CategoryEndpoint:
		// Endpoint checks are handled by the validator's ValidateEndpoints.
		result.Status = StatusWarn
		result.Message = "Endpoint check should be run through validator.ValidateEndpoints()"

	case CategoryData:
		// Data checks are handled by the validator's ValidateDatabase.
		result.Status = StatusWarn
		result.Message = "Data check should be run through validator.ValidateDatabase()"

	case CategoryPerformance:
		// Performance checks need actual measurements.
		result.Status = StatusWarn
		result.Message = "Performance check requires implementation"
		if check.TargetMs > 0 {
			result.Details = map[string]interface{}{
				"target_ms": check.TargetMs,
			}
		}

	default:
		result.Status = StatusSkip
		result.Message = "Unknown check category"
	}

	result.Duration = time.Since(start)
	return result
}

// ServiceChecks returns a summary of service-level checks.
func ServiceChecks() []string {
	return []string{
		"Gateway service running and accepting connections",
		"Search service running and accepting connections",
		"Gmail service running and accepting connections",
		"Content service running and accepting connections",
		"AI service running and accepting connections",
		"Review service running and accepting connections",
		"Relationship service running and accepting connections",
	}
}

// EndpointChecks returns a summary of endpoint checks.
func EndpointChecks() []string {
	return []string{
		"Gateway: GetStatus, GetHealth endpoints respond",
		"Search: Query, HybridSearch, GetDocument endpoints respond",
		"Gmail: Sync, ListMessages, GetMessage endpoints respond",
		"Content: Ingest, GetContent, ListContent endpoints respond",
		"AI: Generate, CreateEmbedding, Summarize endpoints respond",
		"Review: GetDailyReview, GenerateReview endpoints respond",
		"Relationship: Discover, GetRelationships, Validate endpoints respond",
	}
}

// DataChecks returns a summary of data integrity checks.
func DataChecks() []string {
	return []string{
		"Database connection successful",
		"Database schema matches expected version",
		"pgvector extension installed and functional",
		"Content item counts match between Python and Go",
		"Entity counts preserved",
		"Relationship graph integrity maintained",
		"Embedding vectors valid and searchable",
	}
}

// PerformanceChecks returns a summary of performance checks.
func PerformanceChecks() []string {
	return []string{
		"Search query latency < 500ms (p95)",
		"Embedding generation latency < 1000ms",
		"Database query latency < 100ms (p95)",
		"gRPC roundtrip latency < 50ms",
		"HTTP health check latency < 100ms",
	}
}
