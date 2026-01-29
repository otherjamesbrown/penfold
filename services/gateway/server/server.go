// Package server provides the gRPC server implementation for the API Gateway.
// It implements the GatewayService protobuf service definition.
package server

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	gatewaypb "github.com/otherjamesbrown/penfold/api/proto/core/v1/gatewaypb"
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
	gatewaypb.UnimplementedGatewayServiceServer

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

// ProcessEmail ingests and processes an email through the AI pipeline.
// This is a placeholder implementation that returns Unimplemented.
func (s *GatewayServer) ProcessEmail(ctx context.Context, req *gatewaypb.ProcessEmailRequest) (*gatewaypb.ProcessEmailResponse, error) {
	s.logger.Info("ProcessEmail called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("message_id", req.GetMessageId()),
	)

	// STUB: Returns Unimplemented until orchestrator service integration.
	return nil, status.Error(codes.Unimplemented, "ProcessEmail not yet implemented")
}

// Search provides unified search across all indexed content.
// This is a placeholder implementation that returns Unimplemented.
func (s *GatewayServer) Search(ctx context.Context, req *gatewaypb.SearchRequest) (*gatewaypb.SearchResponse, error) {
	s.logger.Info("Search called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("query", req.GetQuery()),
		logging.F("mode", req.GetMode().String()),
	)

	// STUB: Returns Unimplemented until search service integration.
	return nil, status.Error(codes.Unimplemented, "Search not yet implemented")
}

// GetDailyReview retrieves the daily digest/review for a user.
// This is a placeholder implementation that returns Unimplemented.
func (s *GatewayServer) GetDailyReview(ctx context.Context, req *gatewaypb.GetDailyReviewRequest) (*gatewaypb.GetDailyReviewResponse, error) {
	s.logger.Info("GetDailyReview called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("user_id", req.GetUserId()),
	)

	// STUB: Returns Unimplemented until daily review service integration.
	return nil, status.Error(codes.Unimplemented, "GetDailyReview not yet implemented")
}

// HealthCheck returns the health status of the gateway and its dependencies.
func (s *GatewayServer) HealthCheck(ctx context.Context, req *gatewaypb.HealthCheckRequest) (*gatewaypb.HealthCheckResponse, error) {
	s.logger.Debug("HealthCheck called",
		logging.F("include_dependencies", req.GetIncludeDependencies()),
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

	resp := &gatewaypb.HealthCheckResponse{
		Healthy:       healthy,
		Message:       message,
		Version:       Version,
		UptimeSeconds: int64(time.Since(s.startTime).Seconds()),
		Timestamp:     timestamppb.Now(),
		Dependencies:  make([]*gatewaypb.DependencyHealth, 0),
	}

	if req.GetIncludeDependencies() {
		// Include all backend service health status from the aggregator.
		for _, svc := range aggregatedHealth.Services {
			latencyMs := float64(svc.LatencyMs)
			depHealth := &gatewaypb.DependencyHealth{
				Name:      svc.Name,
				Healthy:   svc.Status == pkghealth.StatusHealthy,
				Message:   svc.Message,
				LatencyMs: &latencyMs,
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
