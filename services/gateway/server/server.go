// Package server provides the gRPC server implementation for the API Gateway.
// It implements the GatewayService protobuf service definition.
package server

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pkghealth "github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/gateway/config"
	"github.com/otherjamesbrown/penfold/services/gateway/health"
)

// Version is the service version.
const Version = "0.1.0"

// GatewayServer implements the GatewayService gRPC service.
// It acts as the unified entry point for external API requests,
// routing them to appropriate internal services.
type GatewayServer struct {
	// UnimplementedGatewayServiceServer is embedded to ensure forward compatibility.
	// When new methods are added to the proto, this struct will automatically
	// return Unimplemented for those methods.
	// UnimplementedGatewayServiceServer

	config           *config.GatewayConfig
	logger           logging.Logger
	metrics          *metrics.Metrics
	startTime        time.Time
	healthAggregator *health.Aggregator
}

// NewGatewayServer creates a new GatewayServer instance.
func NewGatewayServer(cfg *config.GatewayConfig, logger logging.Logger, m *metrics.Metrics, healthAggregator *health.Aggregator) *GatewayServer {
	return &GatewayServer{
		config:           cfg,
		logger:           logger,
		metrics:          m,
		startTime:        time.Now(),
		healthAggregator: healthAggregator,
	}
}

// ProcessEmailRequest represents a request to process an email.
// This is a placeholder until proto generation is set up.
type ProcessEmailRequest struct {
	TenantID  string
	MessageID string
	ThreadID  string
	FromEmail string
	FromName  string
	Subject   string
	Body      string
	ToEmails  []string
	CCEmails  []string
	// EmailDate and Attachments omitted for now
}

// ProcessEmailResponse represents the response from processing an email.
type ProcessEmailResponse struct {
	JobID      string
	WorkflowID string
	SourceID   int64
	Status     string
	Message    string
}

// ProcessEmail ingests and processes an email through the AI pipeline.
// This is a placeholder implementation that returns Unimplemented.
func (s *GatewayServer) ProcessEmail(ctx context.Context, req *ProcessEmailRequest) (*ProcessEmailResponse, error) {
	s.logger.Info("ProcessEmail called",
		logging.F("tenant_id", req.TenantID),
		logging.F("message_id", req.MessageID),
	)

	// TODO: Implement actual email processing by calling the orchestrator service.
	return nil, status.Error(codes.Unimplemented, "ProcessEmail not yet implemented")
}

// SearchRequest represents a search request.
type SearchRequest struct {
	TenantID       string
	Query          string
	Mode           string
	SourceTypes    []string
	IncludeContent bool
}

// SearchResponse represents the response from a search.
type SearchResponse struct {
	Results     []SearchResult
	QueryTimeMs int64
	TotalCount  int64
}

// SearchResult represents a single search result.
type SearchResult struct {
	SourceID   string
	SourceType string
	Title      string
	Snippet    string
	Score      float32
}

// Search provides unified search across all indexed content.
// This is a placeholder implementation that returns Unimplemented.
func (s *GatewayServer) Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	s.logger.Info("Search called",
		logging.F("tenant_id", req.TenantID),
		logging.F("query", req.Query),
		logging.F("mode", req.Mode),
	)

	// TODO: Implement actual search by calling the search service.
	return nil, status.Error(codes.Unimplemented, "Search not yet implemented")
}

// GetDailyReviewRequest represents a request to get the daily review.
type GetDailyReviewRequest struct {
	TenantID       string
	Date           *time.Time
	UserID         string
	IncludeDetails bool
}

// GetDailyReviewResponse represents the response containing the daily review.
type GetDailyReviewResponse struct {
	Date        time.Time
	Summary     string
	Highlights  []ReviewHighlight
	ActionItems []ActionItem
	GeneratedAt time.Time
}

// ReviewHighlight represents a notable item from the day.
type ReviewHighlight struct {
	HighlightType string
	Title         string
	Summary       string
	Importance    float32
}

// ActionItem represents a task or follow-up extracted from content.
type ActionItem struct {
	Description string
	Priority    string
	Status      string
}

// GetDailyReview retrieves the daily digest/review for a user.
// This is a placeholder implementation that returns Unimplemented.
func (s *GatewayServer) GetDailyReview(ctx context.Context, req *GetDailyReviewRequest) (*GetDailyReviewResponse, error) {
	s.logger.Info("GetDailyReview called",
		logging.F("tenant_id", req.TenantID),
		logging.F("user_id", req.UserID),
	)

	// TODO: Implement actual daily review by calling the daily review service.
	return nil, status.Error(codes.Unimplemented, "GetDailyReview not yet implemented")
}

// HealthCheckRequest represents a health check request.
type HealthCheckRequest struct {
	IncludeDependencies bool
}

// HealthCheckResponse represents the response from a health check.
type HealthCheckResponse struct {
	Healthy       bool
	Message       string
	Dependencies  []DependencyHealth
	Version       string
	UptimeSeconds int64
	Timestamp     *timestamppb.Timestamp
}

// DependencyHealth represents the health status of a dependent service.
type DependencyHealth struct {
	Name      string
	Healthy   bool
	Message   string
	LatencyMs float64
}

// HealthCheck returns the health status of the gateway and its dependencies.
func (s *GatewayServer) HealthCheck(ctx context.Context, req *HealthCheckRequest) (*HealthCheckResponse, error) {
	s.logger.Debug("HealthCheck called",
		logging.F("include_dependencies", req.IncludeDependencies),
	)

	// Use the health aggregator to get aggregated health from all backend services.
	aggregatedHealth := s.healthAggregator.CheckAll(ctx)

	// Determine overall health based on aggregated status.
	healthy := aggregatedHealth.Status != pkghealth.StatusUnhealthy
	message := "Gateway is healthy"
	if aggregatedHealth.Status == pkghealth.StatusDegraded {
		message = "Gateway is degraded - some non-critical services are unavailable"
	} else if aggregatedHealth.Status == pkghealth.StatusUnhealthy {
		message = "Gateway is unhealthy - critical services are unavailable"
	}

	resp := &HealthCheckResponse{
		Healthy:       healthy,
		Message:       message,
		Version:       Version,
		UptimeSeconds: int64(time.Since(s.startTime).Seconds()),
		Timestamp:     timestamppb.Now(),
		Dependencies:  make([]DependencyHealth, 0),
	}

	if req.IncludeDependencies {
		// Include all backend service health status from the aggregator.
		for _, svc := range aggregatedHealth.Services {
			depHealth := DependencyHealth{
				Name:      svc.Name,
				Healthy:   svc.Status == pkghealth.StatusHealthy,
				Message:   svc.Message,
				LatencyMs: float64(svc.LatencyMs),
			}
			if svc.Error != "" {
				depHealth.Message = svc.Error
			}
			resp.Dependencies = append(resp.Dependencies, depHealth)
		}
	}

	return resp, nil
}

// Uptime returns the duration since the server started.
func (s *GatewayServer) Uptime() time.Duration {
	return time.Since(s.startTime)
}
