// Package router provides request routing functionality for the API Gateway.
// It manages backend service connections, connection pooling, and routes
// requests to appropriate backend services based on request type.
package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/review/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
)

// Backend names for registered services.
const (
	BackendSearch       = "search"
	BackendGmail        = "gmail"
	BackendContent      = "content"
	BackendAI           = "ai"
	BackendRelationship = "relationship"
	BackendReview       = "review"
)

// Context keys for request metadata propagation.
type contextKey string

const (
	// TenantIDKey is the context key for tenant ID.
	TenantIDKey contextKey = "tenant_id"
	// TraceIDKey is the context key for trace ID.
	TraceIDKey contextKey = "trace_id"
	// RequestIDKey is the context key for request ID.
	RequestIDKey contextKey = "request_id"
)

// Metadata keys for gRPC propagation.
const (
	MetadataTenantID  = "x-tenant-id"
	MetadataTraceID   = "x-trace-id"
	MetadataRequestID = "x-request-id"
)

// BackendConfig holds configuration for a backend service connection.
type BackendConfig struct {
	// Address is the gRPC address of the backend service.
	Address string
	// MaxRetries is the maximum number of connection retry attempts.
	MaxRetries int
	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration
	// ConnectionTimeout is the timeout for establishing a connection.
	ConnectionTimeout time.Duration
	// RequestTimeout is the default timeout for requests to this backend.
	RequestTimeout time.Duration
}

// DefaultBackendConfig returns a BackendConfig with sensible defaults.
func DefaultBackendConfig(address string) *BackendConfig {
	return &BackendConfig{
		Address:           address,
		MaxRetries:        3,
		RetryDelay:        time.Second,
		ConnectionTimeout: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
	}
}

// Backend represents a connection to a backend service.
type Backend struct {
	name   string
	config *BackendConfig
	conn   *grpc.ClientConn
	mu     sync.RWMutex

	// Service clients (lazy initialized).
	searchClient       searchv1.SearchServiceClient
	gmailClient        gmailv1.GmailConnectorServiceClient
	contentClient      contentv1.ContentProcessorServiceClient
	aiClient           aiv1.AICoordinatorServiceClient
	relationshipClient relationshipv1.RelationshipServiceClient
	reviewClient       reviewv1.ReviewServiceClient
}

// BackendStatus represents the health status of a backend.
type BackendStatus struct {
	Name      string
	Address   string
	Connected bool
	State     connectivity.State
	LastError error
}

// Router manages connections to backend services and routes requests.
type Router struct {
	backends map[string]*Backend
	mu       sync.RWMutex
	logger   logging.Logger
	metrics  *metrics.Metrics

	// Default dial options for all backend connections.
	dialOpts []grpc.DialOption
}

// RouterOption is a function that configures a Router.
type RouterOption func(*Router)

// WithLogger sets the logger for the router.
func WithLogger(logger logging.Logger) RouterOption {
	return func(r *Router) {
		r.logger = logger
	}
}

// WithMetrics sets the metrics collector for the router.
func WithMetrics(m *metrics.Metrics) RouterOption {
	return func(r *Router) {
		r.metrics = m
	}
}

// WithDialOptions sets additional gRPC dial options.
func WithDialOptions(opts ...grpc.DialOption) RouterOption {
	return func(r *Router) {
		r.dialOpts = append(r.dialOpts, opts...)
	}
}

// NewRouter creates a new Router instance.
func NewRouter(opts ...RouterOption) *Router {
	r := &Router{
		backends: make(map[string]*Backend),
		dialOpts: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
	}

	for _, opt := range opts {
		opt(r)
	}

	// Use a no-op logger if none provided.
	if r.logger == nil {
		r.logger = logging.MustGlobal()
	}

	return r
}

// RegisterBackend registers a new backend service with the router.
// It establishes a gRPC connection to the backend and stores it for routing.
func (r *Router) RegisterBackend(name, address string) error {
	return r.RegisterBackendWithConfig(name, DefaultBackendConfig(address))
}

// RegisterBackendWithConfig registers a backend service with custom configuration.
func (r *Router) RegisterBackendWithConfig(name string, config *BackendConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.backends[name]; exists {
		return fmt.Errorf("backend %q already registered", name)
	}

	backend := &Backend{
		name:   name,
		config: config,
	}

	// Establish connection with retry logic.
	if err := r.connectBackend(backend); err != nil {
		r.logger.Warn("Failed to connect to backend on registration, will retry on demand",
			logging.F("backend", name),
			logging.F("address", config.Address),
			logging.Err(err),
		)
		// Don't return error - allow registration with deferred connection.
	}

	r.backends[name] = backend
	r.logger.Info("Backend registered",
		logging.F("name", name),
		logging.F("address", config.Address),
	)

	return nil
}

// connectBackend establishes a gRPC connection to a backend service.
func (r *Router) connectBackend(backend *Backend) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()

	// Close existing connection if any.
	if backend.conn != nil {
		backend.conn.Close()
		backend.conn = nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), backend.config.ConnectionTimeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= backend.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backend.config.RetryDelay)
		}

		conn, err := grpc.DialContext(ctx, backend.config.Address, r.dialOpts...)
		if err != nil {
			lastErr = err
			r.logger.Debug("Connection attempt failed",
				logging.F("backend", backend.name),
				logging.F("attempt", attempt+1),
				logging.Err(err),
			)
			continue
		}

		backend.conn = conn
		r.logger.Debug("Backend connected",
			logging.F("backend", backend.name),
			logging.F("address", backend.config.Address),
		)
		return nil
	}

	return fmt.Errorf("failed to connect to backend %q after %d attempts: %w",
		backend.name, backend.config.MaxRetries+1, lastErr)
}

// ensureConnection ensures the backend has an active connection.
// It will attempt to reconnect if the connection is not ready.
func (r *Router) ensureConnection(backend *Backend) error {
	backend.mu.RLock()
	if backend.conn != nil {
		state := backend.conn.GetState()
		if state == connectivity.Ready || state == connectivity.Idle {
			backend.mu.RUnlock()
			return nil
		}
	}
	backend.mu.RUnlock()

	// Need to reconnect.
	return r.connectBackend(backend)
}

// getBackend returns the backend for the given name.
func (r *Router) getBackend(name string) (*Backend, error) {
	r.mu.RLock()
	backend, exists := r.backends[name]
	r.mu.RUnlock()

	if !exists {
		return nil, status.Errorf(codes.NotFound, "backend %q not registered", name)
	}

	// Ensure connection is ready.
	if err := r.ensureConnection(backend); err != nil {
		return nil, status.Errorf(codes.Unavailable, "backend %q unavailable: %v", name, err)
	}

	return backend, nil
}

// PropagateContext creates a new context with metadata propagated from the input context.
// It copies tenant_id, trace_id, and request_id to gRPC metadata for backend calls.
func PropagateContext(ctx context.Context) context.Context {
	md := metadata.MD{}

	// Extract and propagate tenant_id.
	if tenantID, ok := ctx.Value(TenantIDKey).(string); ok && tenantID != "" {
		md.Set(MetadataTenantID, tenantID)
	}

	// Extract and propagate trace_id.
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		md.Set(MetadataTraceID, traceID)
	}

	// Extract and propagate request_id.
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok && requestID != "" {
		md.Set(MetadataRequestID, requestID)
	}

	// Also check for existing gRPC metadata and merge it.
	if existingMD, ok := metadata.FromIncomingContext(ctx); ok {
		for k, v := range existingMD {
			if _, exists := md[k]; !exists {
				md[k] = v
			}
		}
	}

	return metadata.NewOutgoingContext(ctx, md)
}

// WithTenantID returns a context with the tenant ID set.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// WithTraceID returns a context with the trace ID set.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// WithRequestID returns a context with the request ID set.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetTenantID extracts the tenant ID from a context.
func GetTenantID(ctx context.Context) string {
	if tenantID, ok := ctx.Value(TenantIDKey).(string); ok {
		return tenantID
	}
	// Check gRPC metadata as fallback.
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(MetadataTenantID); len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// GetTraceID extracts the trace ID from a context.
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	// Check gRPC metadata as fallback.
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(MetadataTraceID); len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// SearchClient returns the Search service client.
func (r *Router) SearchClient() (searchv1.SearchServiceClient, error) {
	backend, err := r.getBackend(BackendSearch)
	if err != nil {
		return nil, err
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()

	if backend.searchClient == nil {
		backend.searchClient = searchv1.NewSearchServiceClient(backend.conn)
	}
	return backend.searchClient, nil
}

// GmailClient returns the Gmail service client.
func (r *Router) GmailClient() (gmailv1.GmailConnectorServiceClient, error) {
	backend, err := r.getBackend(BackendGmail)
	if err != nil {
		return nil, err
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()

	if backend.gmailClient == nil {
		backend.gmailClient = gmailv1.NewGmailConnectorServiceClient(backend.conn)
	}
	return backend.gmailClient, nil
}

// ContentClient returns the Content Processor service client.
func (r *Router) ContentClient() (contentv1.ContentProcessorServiceClient, error) {
	backend, err := r.getBackend(BackendContent)
	if err != nil {
		return nil, err
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()

	if backend.contentClient == nil {
		backend.contentClient = contentv1.NewContentProcessorServiceClient(backend.conn)
	}
	return backend.contentClient, nil
}

// AIClient returns the AI Coordinator service client.
func (r *Router) AIClient() (aiv1.AICoordinatorServiceClient, error) {
	backend, err := r.getBackend(BackendAI)
	if err != nil {
		return nil, err
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()

	if backend.aiClient == nil {
		backend.aiClient = aiv1.NewAICoordinatorServiceClient(backend.conn)
	}
	return backend.aiClient, nil
}

// RelationshipClient returns the Relationship service client.
func (r *Router) RelationshipClient() (relationshipv1.RelationshipServiceClient, error) {
	backend, err := r.getBackend(BackendRelationship)
	if err != nil {
		return nil, err
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()

	if backend.relationshipClient == nil {
		backend.relationshipClient = relationshipv1.NewRelationshipServiceClient(backend.conn)
	}
	return backend.relationshipClient, nil
}

// ReviewClient returns the Review service client.
func (r *Router) ReviewClient() (reviewv1.ReviewServiceClient, error) {
	backend, err := r.getBackend(BackendReview)
	if err != nil {
		return nil, err
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()

	if backend.reviewClient == nil {
		backend.reviewClient = reviewv1.NewReviewServiceClient(backend.conn)
	}
	return backend.reviewClient, nil
}

// RouteSearch routes a search request to the Search backend.
func (r *Router) RouteSearch(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	client, err := r.SearchClient()
	if err != nil {
		r.recordRoutingError(BackendSearch, err)
		return nil, r.handleUnavailable(BackendSearch, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.Search(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendSearch, err)
		return nil, err
	}

	return resp, nil
}

// RouteSemanticSearch routes a semantic search request to the Search backend.
func (r *Router) RouteSemanticSearch(ctx context.Context, req *searchv1.SemanticSearchRequest) (*searchv1.SearchResponse, error) {
	client, err := r.SearchClient()
	if err != nil {
		r.recordRoutingError(BackendSearch, err)
		return nil, r.handleUnavailable(BackendSearch, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.SemanticSearch(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendSearch, err)
		return nil, err
	}

	return resp, nil
}

// RouteGmailSync routes a Gmail sync request to the Gmail backend.
func (r *Router) RouteGmailSync(ctx context.Context, req *gmailv1.SyncEmailsRequest) (*gmailv1.SyncEmailsResponse, error) {
	client, err := r.GmailClient()
	if err != nil {
		r.recordRoutingError(BackendGmail, err)
		return nil, r.handleUnavailable(BackendGmail, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.SyncEmails(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendGmail, err)
		return nil, err
	}

	return resp, nil
}

// RouteGmailGetEmail routes a Gmail get email request to the Gmail backend.
func (r *Router) RouteGmailGetEmail(ctx context.Context, req *gmailv1.GetEmailRequest) (*gmailv1.Email, error) {
	client, err := r.GmailClient()
	if err != nil {
		r.recordRoutingError(BackendGmail, err)
		return nil, r.handleUnavailable(BackendGmail, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.GetEmail(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendGmail, err)
		return nil, err
	}

	return resp, nil
}

// RouteContentProcess routes a content processing request to the Content backend.
func (r *Router) RouteContentProcess(ctx context.Context, req *contentv1.ProcessContentRequest) (*contentv1.ProcessContentResponse, error) {
	client, err := r.ContentClient()
	if err != nil {
		r.recordRoutingError(BackendContent, err)
		return nil, r.handleUnavailable(BackendContent, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.ProcessContent(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendContent, err)
		return nil, err
	}

	return resp, nil
}

// RouteContentGet routes a content get request to the Content backend.
func (r *Router) RouteContentGet(ctx context.Context, req *contentv1.GetContentItemRequest) (*contentv1.ContentItem, error) {
	client, err := r.ContentClient()
	if err != nil {
		r.recordRoutingError(BackendContent, err)
		return nil, r.handleUnavailable(BackendContent, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.GetContentItem(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendContent, err)
		return nil, err
	}

	return resp, nil
}

// RouteAIGenerateEmbedding routes an embedding request to the AI backend.
func (r *Router) RouteAIGenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	client, err := r.AIClient()
	if err != nil {
		r.recordRoutingError(BackendAI, err)
		return nil, r.handleUnavailable(BackendAI, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.GenerateEmbedding(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendAI, err)
		return nil, err
	}

	return resp, nil
}

// RouteAIGenerateSummary routes a summary request to the AI backend.
func (r *Router) RouteAIGenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	client, err := r.AIClient()
	if err != nil {
		r.recordRoutingError(BackendAI, err)
		return nil, r.handleUnavailable(BackendAI, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.GenerateSummary(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendAI, err)
		return nil, err
	}

	return resp, nil
}

// RouteRelationshipDiscover routes a relationship discovery request to the Relationship backend.
func (r *Router) RouteRelationshipDiscover(ctx context.Context, req *relationshipv1.DiscoverRelationshipsRequest) (*relationshipv1.DiscoverRelationshipsResponse, error) {
	client, err := r.RelationshipClient()
	if err != nil {
		r.recordRoutingError(BackendRelationship, err)
		return nil, r.handleUnavailable(BackendRelationship, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.DiscoverRelationships(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendRelationship, err)
		return nil, err
	}

	return resp, nil
}

// RouteRelationshipList routes a list relationships request to the Relationship backend.
func (r *Router) RouteRelationshipList(ctx context.Context, req *relationshipv1.ListRelationshipsRequest) (*relationshipv1.ListRelationshipsResponse, error) {
	client, err := r.RelationshipClient()
	if err != nil {
		r.recordRoutingError(BackendRelationship, err)
		return nil, r.handleUnavailable(BackendRelationship, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.ListRelationships(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendRelationship, err)
		return nil, err
	}

	return resp, nil
}

// RouteReviewGetDaily routes a daily review request to the Review backend.
func (r *Router) RouteReviewGetDaily(ctx context.Context, req *reviewv1.GetDailyReviewRequest) (*reviewv1.GetDailyReviewResponse, error) {
	client, err := r.ReviewClient()
	if err != nil {
		r.recordRoutingError(BackendReview, err)
		return nil, r.handleUnavailable(BackendReview, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.GetDailyReview(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendReview, err)
		return nil, err
	}

	return resp, nil
}

// RouteReviewApprove routes an approve item request to the Review backend.
func (r *Router) RouteReviewApprove(ctx context.Context, req *reviewv1.ApproveItemRequest) (*reviewv1.ApproveItemResponse, error) {
	client, err := r.ReviewClient()
	if err != nil {
		r.recordRoutingError(BackendReview, err)
		return nil, r.handleUnavailable(BackendReview, err)
	}

	ctx = PropagateContext(ctx)
	resp, err := client.ApproveItem(ctx, req)
	if err != nil {
		r.recordRoutingError(BackendReview, err)
		return nil, err
	}

	return resp, nil
}

// handleUnavailable handles backend unavailability with graceful degradation.
// It logs the error and returns an appropriate gRPC status.
func (r *Router) handleUnavailable(backend string, err error) error {
	r.logger.Warn("Backend unavailable",
		logging.F("backend", backend),
		logging.Err(err),
	)

	// Return a proper gRPC unavailable status.
	return status.Errorf(codes.Unavailable,
		"service temporarily unavailable: backend %q is not reachable", backend)
}

// recordRoutingError records a routing error in metrics.
func (r *Router) recordRoutingError(backend string, err error) {
	if r.metrics != nil {
		r.metrics.RecordError(fmt.Sprintf("routing_%s", backend))
	}
	r.logger.Debug("Routing error",
		logging.F("backend", backend),
		logging.Err(err),
	)
}

// BackendStatuses returns the status of all registered backends.
func (r *Router) BackendStatuses() []BackendStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statuses := make([]BackendStatus, 0, len(r.backends))
	for name, backend := range r.backends {
		backend.mu.RLock()
		status := BackendStatus{
			Name:    name,
			Address: backend.config.Address,
		}
		if backend.conn != nil {
			status.Connected = true
			status.State = backend.conn.GetState()
		}
		backend.mu.RUnlock()
		statuses = append(statuses, status)
	}

	return statuses
}

// IsBackendHealthy checks if a specific backend is healthy.
func (r *Router) IsBackendHealthy(name string) bool {
	r.mu.RLock()
	backend, exists := r.backends[name]
	r.mu.RUnlock()

	if !exists {
		return false
	}

	backend.mu.RLock()
	defer backend.mu.RUnlock()

	if backend.conn == nil {
		return false
	}

	state := backend.conn.GetState()
	return state == connectivity.Ready || state == connectivity.Idle
}

// Close closes all backend connections and releases resources.
func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, backend := range r.backends {
		backend.mu.Lock()
		if backend.conn != nil {
			if err := backend.conn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("closing backend %q: %w", name, err))
			}
			backend.conn = nil
		}
		backend.mu.Unlock()
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing backends: %v", errs)
	}
	return nil
}

// UnregisterBackend removes a backend from the router.
func (r *Router) UnregisterBackend(name string) error {
	r.mu.Lock()
	backend, exists := r.backends[name]
	if !exists {
		r.mu.Unlock()
		return fmt.Errorf("backend %q not registered", name)
	}
	delete(r.backends, name)
	r.mu.Unlock()

	// Close the connection outside the lock.
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.conn != nil {
		if err := backend.conn.Close(); err != nil {
			return fmt.Errorf("closing backend %q: %w", name, err)
		}
	}

	r.logger.Info("Backend unregistered", logging.F("name", name))
	return nil
}
