// Package validation provides a comprehensive validation suite to verify
// the Go migration is complete and functional.
package validation

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// ServiceInfo contains information about a Go service to validate.
type ServiceInfo struct {
	Name     string
	GRPCAddr string
	HTTPAddr string
	Critical bool
}

// ValidationResult represents the result of a single validation check.
type ValidationResult struct {
	Name      string        `json:"name"`
	Status    CheckStatus   `json:"status"`
	Message   string        `json:"message,omitempty"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration_ms"`
	Timestamp time.Time     `json:"timestamp"`
	Details   interface{}   `json:"details,omitempty"`
}

// CheckStatus represents the status of a validation check.
type CheckStatus string

const (
	// StatusPass indicates the check passed successfully.
	StatusPass CheckStatus = "PASS"
	// StatusFail indicates the check failed.
	StatusFail CheckStatus = "FAIL"
	// StatusWarn indicates the check passed with warnings.
	StatusWarn CheckStatus = "WARN"
	// StatusSkip indicates the check was skipped.
	StatusSkip CheckStatus = "SKIP"
)

// Validator provides the main validation framework for migration validation.
type Validator struct {
	mu sync.RWMutex

	// Configuration.
	timeout     time.Duration
	parallelism int

	// Service registry.
	services map[string]ServiceInfo

	// Results storage.
	results []ValidationResult

	// HTTP client for health checks.
	httpClient *http.Client
}

// ValidatorOption configures a Validator.
type ValidatorOption func(*Validator)

// WithTimeout sets the timeout for validation checks.
func WithTimeout(timeout time.Duration) ValidatorOption {
	return func(v *Validator) {
		v.timeout = timeout
	}
}

// WithParallelism sets the maximum number of concurrent checks.
func WithParallelism(n int) ValidatorOption {
	return func(v *Validator) {
		v.parallelism = n
	}
}

// NewValidator creates a new validation framework instance.
func NewValidator(opts ...ValidatorOption) *Validator {
	v := &Validator{
		timeout:     30 * time.Second,
		parallelism: 10,
		services:    make(map[string]ServiceInfo),
		results:     make([]ValidationResult, 0),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(v)
	}

	// Register default Penfold services.
	v.registerDefaultServices()

	return v
}

// registerDefaultServices registers the standard Penfold Go services.
func (v *Validator) registerDefaultServices() {
	defaultServices := []ServiceInfo{
		{Name: "gateway", GRPCAddr: "localhost:50051", HTTPAddr: "localhost:8080", Critical: true},
		{Name: "search", GRPCAddr: "localhost:50052", HTTPAddr: "localhost:8082", Critical: true},
		{Name: "gmail", GRPCAddr: "localhost:50053", HTTPAddr: "localhost:8083", Critical: false},
		{Name: "content", GRPCAddr: "localhost:50054", HTTPAddr: "localhost:8084", Critical: true},
		{Name: "ai", GRPCAddr: "localhost:50055", HTTPAddr: "localhost:8085", Critical: true},
		{Name: "review", GRPCAddr: "localhost:50056", HTTPAddr: "localhost:8086", Critical: false},
		{Name: "relationship", GRPCAddr: "localhost:50057", HTTPAddr: "localhost:8087", Critical: false},
	}

	for _, svc := range defaultServices {
		v.services[svc.Name] = svc
	}
}

// RegisterService adds or updates a service in the registry.
func (v *Validator) RegisterService(svc ServiceInfo) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.services[svc.Name] = svc
}

// GetServices returns the registered services.
func (v *Validator) GetServices() map[string]ServiceInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	services := make(map[string]ServiceInfo)
	for k, svc := range v.services {
		services[k] = svc
	}
	return services
}

// ValidateServices checks that all Go services are running and responsive.
func (v *Validator) ValidateServices(ctx context.Context) []ValidationResult {
	v.mu.RLock()
	services := make([]ServiceInfo, 0, len(v.services))
	for _, svc := range v.services {
		services = append(services, svc)
	}
	v.mu.RUnlock()

	results := make([]ValidationResult, len(services))
	var wg sync.WaitGroup

	// Use semaphore for parallelism control.
	sem := make(chan struct{}, v.parallelism)

	for i, svc := range services {
		wg.Add(1)
		go func(idx int, service ServiceInfo) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = v.validateService(ctx, service)
		}(i, svc)
	}

	wg.Wait()
	return results
}

// validateService validates a single service.
func (v *Validator) validateService(ctx context.Context, svc ServiceInfo) ValidationResult {
	start := time.Now()
	result := ValidationResult{
		Name:      fmt.Sprintf("service_%s", svc.Name),
		Timestamp: time.Now(),
	}

	// Check if gRPC port is listening.
	conn, err := net.DialTimeout("tcp", svc.GRPCAddr, 5*time.Second)
	if err != nil {
		result.Status = StatusFail
		result.Error = fmt.Sprintf("gRPC port not reachable: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	_ = conn.Close()

	// Check if HTTP health endpoint responds.
	healthURL := fmt.Sprintf("http://%s/health", svc.HTTPAddr)
	resp, err := v.httpClient.Get(healthURL)
	if err != nil {
		result.Status = StatusWarn
		result.Message = fmt.Sprintf("gRPC port reachable but HTTP health check failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 500 {
		result.Status = StatusFail
		result.Error = fmt.Sprintf("health endpoint returned status %d", resp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	if resp.StatusCode >= 400 {
		result.Status = StatusWarn
		result.Message = fmt.Sprintf("health endpoint returned status %d", resp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	result.Status = StatusPass
	result.Message = fmt.Sprintf("Service %s is healthy", svc.Name)
	result.Duration = time.Since(start)
	result.Details = map[string]interface{}{
		"grpc_addr": svc.GRPCAddr,
		"http_addr": svc.HTTPAddr,
		"critical":  svc.Critical,
	}

	return result
}

// ValidateEndpoints tests that all gRPC endpoints respond correctly.
func (v *Validator) ValidateEndpoints(ctx context.Context) []ValidationResult {
	v.mu.RLock()
	services := make([]ServiceInfo, 0, len(v.services))
	for _, svc := range v.services {
		services = append(services, svc)
	}
	v.mu.RUnlock()

	results := make([]ValidationResult, len(services))
	var wg sync.WaitGroup

	sem := make(chan struct{}, v.parallelism)

	for i, svc := range services {
		wg.Add(1)
		go func(idx int, service ServiceInfo) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = v.validateEndpoint(ctx, service)
		}(i, svc)
	}

	wg.Wait()
	return results
}

// validateEndpoint validates gRPC health endpoint for a service.
func (v *Validator) validateEndpoint(ctx context.Context, svc ServiceInfo) ValidationResult {
	start := time.Now()
	result := ValidationResult{
		Name:      fmt.Sprintf("endpoint_%s_grpc", svc.Name),
		Timestamp: time.Now(),
	}

	// Create gRPC connection.
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, svc.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		result.Status = StatusFail
		result.Error = fmt.Sprintf("failed to connect: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer conn.Close() //nolint:errcheck

	// Use standard gRPC health check protocol.
	healthClient := grpc_health_v1.NewHealthClient(conn)
	checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
	defer checkCancel()

	resp, err := healthClient.Check(checkCtx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		// gRPC health service might not be implemented yet.
		result.Status = StatusWarn
		result.Message = fmt.Sprintf("gRPC health check not implemented or failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		result.Status = StatusFail
		result.Error = fmt.Sprintf("service not serving: %v", resp.Status)
		result.Duration = time.Since(start)
		return result
	}

	result.Status = StatusPass
	result.Message = fmt.Sprintf("gRPC endpoint %s is serving", svc.Name)
	result.Duration = time.Since(start)
	result.Details = map[string]interface{}{
		"grpc_addr": svc.GRPCAddr,
		"status":    resp.Status.String(),
	}

	return result
}

// DatabaseValidationConfig holds database validation configuration.
type DatabaseValidationConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
}

// ValidateDatabase verifies database connectivity and schema.
func (v *Validator) ValidateDatabase(ctx context.Context, cfg DatabaseValidationConfig) []ValidationResult {
	results := make([]ValidationResult, 0)

	// Check 1: Connection test.
	connResult := v.validateDBConnection(ctx, cfg)
	results = append(results, connResult)

	// If connection failed, skip other DB checks.
	if connResult.Status == StatusFail {
		results = append(results, ValidationResult{
			Name:      "db_schema",
			Status:    StatusSkip,
			Message:   "Skipped due to connection failure",
			Timestamp: time.Now(),
		})
		results = append(results, ValidationResult{
			Name:      "db_vector_extension",
			Status:    StatusSkip,
			Message:   "Skipped due to connection failure",
			Timestamp: time.Now(),
		})
		return results
	}

	// Check 2: Schema validation.
	schemaResult := v.validateDBSchema(ctx, cfg)
	results = append(results, schemaResult)

	// Check 3: pgvector extension.
	vectorResult := v.validateDBVectorExtension(ctx, cfg)
	results = append(results, vectorResult)

	return results
}

// validateDBConnection tests basic database connectivity.
func (v *Validator) validateDBConnection(ctx context.Context, cfg DatabaseValidationConfig) ValidationResult {
	start := time.Now()
	result := ValidationResult{
		Name:      "db_connection",
		Timestamp: time.Now(),
	}

	// Test TCP connectivity to database port.
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		result.Status = StatusFail
		result.Error = fmt.Sprintf("cannot connect to database at %s: %v", addr, err)
		result.Duration = time.Since(start)
		return result
	}
	_ = conn.Close()

	result.Status = StatusPass
	result.Message = fmt.Sprintf("Database port reachable at %s", addr)
	result.Duration = time.Since(start)
	result.Details = map[string]interface{}{
		"host":     cfg.Host,
		"port":     cfg.Port,
		"database": cfg.Database,
	}

	return result
}

// validateDBSchema validates that required tables exist.
func (v *Validator) validateDBSchema(ctx context.Context, cfg DatabaseValidationConfig) ValidationResult {
	start := time.Now()
	result := ValidationResult{
		Name:      "db_schema",
		Timestamp: time.Now(),
	}

	// This is a placeholder - actual implementation would query the database.
	// For now, we mark it as a warning since we can't fully validate without
	// connecting to the database.
	result.Status = StatusWarn
	result.Message = "Schema validation requires database connection (implement with pgx)"
	result.Duration = time.Since(start)

	return result
}

// validateDBVectorExtension checks if pgvector extension is installed.
func (v *Validator) validateDBVectorExtension(ctx context.Context, cfg DatabaseValidationConfig) ValidationResult {
	start := time.Now()
	result := ValidationResult{
		Name:      "db_vector_extension",
		Timestamp: time.Now(),
	}

	// This is a placeholder - actual implementation would query the database.
	result.Status = StatusWarn
	result.Message = "Vector extension validation requires database connection"
	result.Duration = time.Since(start)

	return result
}

// FeatureParityConfig holds configuration for feature parity validation.
type FeatureParityConfig struct {
	PythonEndpoint string
	GoEndpoint     string
	TestCases      []ParityTestCase
}

// ParityTestCase represents a single test case for feature parity validation.
type ParityTestCase struct {
	Name        string
	Description string
	Endpoint    string
	Method      string
	Payload     interface{}
}

// ValidateFeatureParity compares Go vs Python functionality.
func (v *Validator) ValidateFeatureParity(ctx context.Context, cfg FeatureParityConfig) []ValidationResult {
	results := make([]ValidationResult, 0)

	// Check if Python endpoint is available.
	pythonResult := v.checkEndpointAvailable(ctx, "parity_python_available", cfg.PythonEndpoint)
	results = append(results, pythonResult)

	// Check if Go endpoint is available.
	goResult := v.checkEndpointAvailable(ctx, "parity_go_available", cfg.GoEndpoint)
	results = append(results, goResult)

	// If either endpoint is unavailable, skip parity tests.
	if pythonResult.Status == StatusFail || goResult.Status == StatusFail {
		result := ValidationResult{
			Name:      "parity_tests",
			Status:    StatusSkip,
			Message:   "Skipped: both Python and Go endpoints must be available",
			Timestamp: time.Now(),
		}
		results = append(results, result)
		return results
	}

	// Run parity tests for each test case.
	for _, tc := range cfg.TestCases {
		parityResult := v.runParityTest(ctx, cfg, tc)
		results = append(results, parityResult)
	}

	return results
}

// checkEndpointAvailable verifies an endpoint is reachable.
func (v *Validator) checkEndpointAvailable(ctx context.Context, name, endpoint string) ValidationResult {
	start := time.Now()
	result := ValidationResult{
		Name:      name,
		Timestamp: time.Now(),
	}

	if endpoint == "" {
		result.Status = StatusSkip
		result.Message = "Endpoint not configured"
		result.Duration = time.Since(start)
		return result
	}

	resp, err := v.httpClient.Get(endpoint)
	if err != nil {
		result.Status = StatusFail
		result.Error = fmt.Sprintf("endpoint not reachable: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	_ = resp.Body.Close()

	result.Status = StatusPass
	result.Message = fmt.Sprintf("Endpoint available: %s", endpoint)
	result.Duration = time.Since(start)

	return result
}

// runParityTest runs a single parity test case.
func (v *Validator) runParityTest(ctx context.Context, cfg FeatureParityConfig, tc ParityTestCase) ValidationResult {
	start := time.Now()
	result := ValidationResult{
		Name:      fmt.Sprintf("parity_%s", tc.Name),
		Timestamp: time.Now(),
	}

	// This is a placeholder for actual parity testing.
	// In a full implementation, this would:
	// 1. Make identical requests to both Python and Go endpoints.
	// 2. Compare response structures.
	// 3. Validate response values match within acceptable tolerances.

	result.Status = StatusWarn
	result.Message = fmt.Sprintf("Parity test '%s' requires implementation", tc.Name)
	result.Duration = time.Since(start)
	result.Details = map[string]interface{}{
		"description": tc.Description,
		"endpoint":    tc.Endpoint,
		"method":      tc.Method,
	}

	return result
}

// RunAll executes all validation checks and returns a comprehensive report.
func (v *Validator) RunAll(ctx context.Context) *ValidationReport {
	report := NewValidationReport()
	report.StartTime = time.Now()

	// Run service validations.
	serviceResults := v.ValidateServices(ctx)
	for _, r := range serviceResults {
		report.AddResult("services", r)
	}

	// Run endpoint validations.
	endpointResults := v.ValidateEndpoints(ctx)
	for _, r := range endpointResults {
		report.AddResult("endpoints", r)
	}

	// Run database validations with default config.
	dbConfig := DatabaseValidationConfig{
		Host:     "localhost",
		Port:     5432,
		Database: "penfold",
		User:     "penfold",
		SSLMode:  "disable",
	}
	dbResults := v.ValidateDatabase(ctx, dbConfig)
	for _, r := range dbResults {
		report.AddResult("database", r)
	}

	report.EndTime = time.Now()
	report.calculateSummary()

	return report
}

// GetResults returns all stored validation results.
func (v *Validator) GetResults() []ValidationResult {
	v.mu.RLock()
	defer v.mu.RUnlock()

	results := make([]ValidationResult, len(v.results))
	copy(results, v.results)
	return results
}

// ClearResults clears all stored validation results.
func (v *Validator) ClearResults() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.results = make([]ValidationResult, 0)
}
