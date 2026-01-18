# API Gateway Specification

## Overview

The API Gateway is the single entry point for all client requests. It handles authentication, routing, rate limiting, and request/response transformation.

## Responsibilities

1. **Authentication**: Validate tokens, manage sessions
2. **Authorization**: Tenant isolation, permission checks
3. **Routing**: Direct requests to appropriate microservices
4. **Rate Limiting**: Per-tenant and per-endpoint limits
5. **Request Transformation**: Protocol bridging (REST ↔ gRPC)
6. **Response Aggregation**: Combine results from multiple services
7. **Observability**: Request logging, metrics, tracing

## Architecture

```
                              ┌─────────────────────────────────────┐
                              │           API Gateway               │
                              │                                     │
 CLI (gRPC) ─────────────────▶│  ┌─────────────────────────────┐   │
                              │  │     gRPC Server (:8080)     │   │
                              │  └─────────────┬───────────────┘   │
                              │                │                    │
 Web (REST) ─────────────────▶│  ┌─────────────┴───────────────┐   │
                              │  │      REST Server (:8088)    │   │
                              │  └─────────────┬───────────────┘   │
                              │                │                    │
                              │  ┌─────────────┴───────────────┐   │
                              │  │     Middleware Chain        │   │
                              │  │  ┌─────┐ ┌─────┐ ┌─────┐   │   │
                              │  │  │Auth │→│Rate │→│Log  │   │   │
                              │  │  └─────┘ └─────┘ └─────┘   │   │
                              │  └─────────────┬───────────────┘   │
                              │                │                    │
                              │  ┌─────────────┴───────────────┐   │
                              │  │       Service Router        │   │
                              │  └─────────────┬───────────────┘   │
                              └────────────────┼────────────────────┘
                                               │
           ┌───────────────┬───────────────────┼───────────────────┬───────────────┐
           ▼               ▼                   ▼                   ▼               ▼
    ┌───────────┐   ┌───────────┐       ┌───────────┐       ┌───────────┐   ┌───────────┐
    │  Search   │   │  Gmail    │       │  Content  │       │  Review   │   │  Event    │
    │  Service  │   │ Connector │       │ Processor │       │  Service  │   │  Router   │
    └───────────┘   └───────────┘       └───────────┘       └───────────┘   └───────────┘
```

## gRPC Service Definition

```protobuf
// api/proto/gateway/v1/gateway.proto

syntax = "proto3";
package gateway.v1;

import "google/protobuf/empty.proto";
import "penf/v1/cli.proto";

// Gateway service - implements penf.v1.PenfCLI
// and adds gateway-specific endpoints

service Gateway {
  // Health
  rpc Health(google.protobuf.Empty) returns (HealthResponse);
  rpc Ready(google.protobuf.Empty) returns (ReadyResponse);

  // Auth (gateway handles directly)
  rpc GetAuthURL(GetAuthURLRequest) returns (GetAuthURLResponse);
  rpc ExchangeToken(ExchangeTokenRequest) returns (ExchangeTokenResponse);
  rpc RefreshToken(RefreshTokenRequest) returns (RefreshTokenResponse);
  rpc RevokeToken(RevokeTokenRequest) returns (google.protobuf.Empty);

  // Forwarded to Search Service
  rpc Search(penf.v1.SearchRequest) returns (penf.v1.SearchResponse);
  rpc Ask(penf.v1.AskRequest) returns (penf.v1.AskResponse);

  // Forwarded to Gmail Connector
  rpc GetGmailStatus(penf.v1.GetGmailStatusRequest) returns (penf.v1.GetGmailStatusResponse);
  rpc TriggerGmailSync(penf.v1.TriggerGmailSyncRequest) returns (penf.v1.TriggerGmailSyncResponse);

  // Forwarded to Content Processor
  rpc IngestFiles(stream penf.v1.IngestFileChunk) returns (penf.v1.IngestResponse);
  rpc GetIngestStatus(penf.v1.GetIngestStatusRequest) returns (penf.v1.GetIngestStatusResponse);

  // Forwarded to Review Service
  rpc GetReviewQueue(penf.v1.GetReviewQueueRequest) returns (penf.v1.GetReviewQueueResponse);
  rpc SubmitReview(penf.v1.SubmitReviewRequest) returns (penf.v1.SubmitReviewResponse);

  // Forwarded to Relationship Discovery
  rpc GetRelationships(penf.v1.GetRelationshipsRequest) returns (penf.v1.GetRelationshipsResponse);
  rpc ValidateRelationship(penf.v1.ValidateRelationshipRequest) returns (penf.v1.ValidateRelationshipResponse);
}

message HealthResponse {
  bool healthy = 1;
  map<string, ServiceHealth> services = 2;
}

message ServiceHealth {
  bool healthy = 1;
  string status = 2;
  int64 latency_ms = 3;
}

message ReadyResponse {
  bool ready = 1;
  repeated string not_ready_services = 2;
}
```

## Authentication

### Token-Based Auth

```go
// internal/auth/middleware.go

type AuthMiddleware struct {
    tokenValidator TokenValidator
    tenantResolver TenantResolver
}

func (m *AuthMiddleware) UnaryInterceptor(
    ctx context.Context,
    req interface{},
    info *grpc.UnaryServerInfo,
    handler grpc.UnaryHandler,
) (interface{}, error) {
    // Skip auth for public endpoints
    if isPublicEndpoint(info.FullMethod) {
        return handler(ctx, req)
    }

    // Extract token from metadata
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return nil, status.Error(codes.Unauthenticated, "missing metadata")
    }

    token := extractToken(md)
    if token == "" {
        return nil, status.Error(codes.Unauthenticated, "missing auth token")
    }

    // Validate token
    claims, err := m.tokenValidator.Validate(ctx, token)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, "invalid token")
    }

    // Add claims to context
    ctx = context.WithValue(ctx, claimsKey, claims)
    ctx = context.WithValue(ctx, tenantKey, claims.TenantID)

    return handler(ctx, req)
}
```

### OAuth2 Flow (for Gmail)

```go
// internal/auth/oauth.go

type OAuthHandler struct {
    config   *oauth2.Config
    stateGen StateGenerator
    db       *sql.DB
}

func (h *OAuthHandler) GetAuthURL(ctx context.Context, req *GetAuthURLRequest) (*GetAuthURLResponse, error) {
    state := h.stateGen.Generate()

    // Store state for CSRF protection
    if err := h.storeState(ctx, state, req.RedirectUri); err != nil {
        return nil, err
    }

    url := h.config.AuthCodeURL(state,
        oauth2.SetAuthURLParam("access_type", "offline"),
        oauth2.SetAuthURLParam("prompt", "consent"),
    )

    return &GetAuthURLResponse{
        AuthUrl: url,
        State:   state,
    }, nil
}

func (h *OAuthHandler) ExchangeToken(ctx context.Context, req *ExchangeTokenRequest) (*ExchangeTokenResponse, error) {
    // Verify state
    if !h.verifyState(ctx, req.State) {
        return nil, status.Error(codes.InvalidArgument, "invalid state")
    }

    // Exchange code for token
    token, err := h.config.Exchange(ctx, req.Code)
    if err != nil {
        return nil, status.Error(codes.Internal, "token exchange failed")
    }

    // Store encrypted token
    encrypted, err := h.encryptToken(token)
    if err != nil {
        return nil, err
    }

    if err := h.storeToken(ctx, req.AccountEmail, encrypted); err != nil {
        return nil, err
    }

    return &ExchangeTokenResponse{
        Success: true,
    }, nil
}
```

## Rate Limiting

```go
// internal/ratelimit/limiter.go

type RateLimiter struct {
    redis *redis.Client
    limits map[string]Limit
}

type Limit struct {
    Requests int
    Window   time.Duration
}

var defaultLimits = map[string]Limit{
    "/penf.v1.PenfCLI/Search":        {Requests: 100, Window: time.Minute},
    "/penf.v1.PenfCLI/Ask":           {Requests: 30, Window: time.Minute},
    "/penf.v1.PenfCLI/IngestFiles":   {Requests: 10, Window: time.Minute},
    "/penf.v1.PenfCLI/TriggerGmailSync": {Requests: 5, Window: time.Minute},
    "default":                        {Requests: 1000, Window: time.Minute},
}

func (r *RateLimiter) Allow(ctx context.Context, tenantID, method string) (bool, error) {
    limit := r.getLimit(method)
    key := fmt.Sprintf("ratelimit:%s:%s", tenantID, method)

    count, err := r.redis.Incr(ctx, key).Result()
    if err != nil {
        return false, err
    }

    if count == 1 {
        r.redis.Expire(ctx, key, limit.Window)
    }

    return count <= int64(limit.Requests), nil
}
```

## Service Routing

```go
// internal/router/router.go

type ServiceRouter struct {
    searchClient      searchpb.SearchServiceClient
    gmailClient       gmailpb.GmailServiceClient
    contentClient     contentpb.ContentServiceClient
    reviewClient      reviewpb.ReviewServiceClient
    relationshipClient relationshippb.RelationshipServiceClient
}

func NewServiceRouter(cfg *config.Config) (*ServiceRouter, error) {
    // Connection pool for each service
    searchConn, err := grpc.Dial(cfg.SearchServiceAddr, grpc.WithInsecure())
    if err != nil {
        return nil, fmt.Errorf("failed to connect to search service: %w", err)
    }

    gmailConn, err := grpc.Dial(cfg.GmailServiceAddr, grpc.WithInsecure())
    if err != nil {
        return nil, fmt.Errorf("failed to connect to gmail service: %w", err)
    }

    // ... etc

    return &ServiceRouter{
        searchClient:      searchpb.NewSearchServiceClient(searchConn),
        gmailClient:       gmailpb.NewGmailServiceClient(gmailConn),
        contentClient:     contentpb.NewContentServiceClient(contentConn),
        reviewClient:      reviewpb.NewReviewServiceClient(reviewConn),
        relationshipClient: relationshippb.NewRelationshipServiceClient(relationshipConn),
    }, nil
}

// Forward search request to Search Service
func (r *ServiceRouter) Search(ctx context.Context, req *penfpb.SearchRequest) (*penfpb.SearchResponse, error) {
    // Extract tenant from context (set by auth middleware)
    tenantID := ctx.Value(tenantKey).(string)

    // Forward with tenant context
    return r.searchClient.Search(ctx, &searchpb.SearchRequest{
        TenantId: tenantID,
        Query:    req.Query,
        Filters:  convertFilters(req.Filters),
        Limit:    req.Limit,
        Offset:   req.Offset,
    })
}
```

## Configuration

```yaml
# config/gateway.yaml

server:
  grpc_port: 8080
  rest_port: 8088
  host: "0.0.0.0"

tls:
  enabled: true
  cert_file: "/etc/penfold/certs/server.crt"
  key_file: "/etc/penfold/certs/server.key"

auth:
  token_secret: "${TOKEN_SECRET}"
  token_expiry: "24h"
  refresh_expiry: "7d"

services:
  search:
    address: "localhost:8081"
    timeout: "30s"
  gmail:
    address: "localhost:8082"
    timeout: "60s"
  content:
    address: "localhost:8083"
    timeout: "120s"
  review:
    address: "localhost:8084"
    timeout: "30s"
  relationship:
    address: "localhost:8086"
    timeout: "30s"
  event_router:
    address: "localhost:8090"
    timeout: "10s"

rate_limit:
  enabled: true
  redis_address: "home-01:6379"

logging:
  level: "info"
  format: "json"

metrics:
  enabled: true
  port: 9090
```

## Observability

### Logging

```go
// internal/logging/logger.go

func RequestLogInterceptor(
    ctx context.Context,
    req interface{},
    info *grpc.UnaryServerInfo,
    handler grpc.UnaryHandler,
) (interface{}, error) {
    start := time.Now()

    // Extract request ID
    requestID := uuid.New().String()
    ctx = context.WithValue(ctx, requestIDKey, requestID)

    // Call handler
    resp, err := handler(ctx, req)

    // Log request
    duration := time.Since(start)
    tenantID, _ := ctx.Value(tenantKey).(string)

    slog.Info("request",
        "request_id", requestID,
        "method", info.FullMethod,
        "tenant_id", tenantID,
        "duration_ms", duration.Milliseconds(),
        "error", err != nil,
    )

    return resp, err
}
```

### Metrics

```go
// internal/metrics/metrics.go

var (
    requestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_requests_total",
            Help: "Total number of requests",
        },
        []string{"method", "status"},
    )

    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "gateway_request_duration_seconds",
            Help:    "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method"},
    )

    activeConnections = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "gateway_active_connections",
            Help: "Number of active connections",
        },
    )
)
```

## Implementation Structure

```
services/gateway/
├── cmd/
│   └── gateway/
│       └── main.go
├── internal/
│   ├── auth/
│   │   ├── middleware.go
│   │   ├── oauth.go
│   │   └── token.go
│   ├── router/
│   │   └── router.go
│   ├── ratelimit/
│   │   └── limiter.go
│   ├── logging/
│   │   └── logger.go
│   ├── metrics/
│   │   └── metrics.go
│   └── config/
│       └── config.go
├── api/
│   └── proto/
│       └── gateway/
│           └── v1/
│               └── gateway.proto
└── go.mod
```

## Health Checks

```go
// internal/health/health.go

type HealthChecker struct {
    services map[string]grpc.ClientConnInterface
}

func (h *HealthChecker) Check(ctx context.Context) *HealthResponse {
    response := &HealthResponse{
        Healthy:  true,
        Services: make(map[string]*ServiceHealth),
    }

    var wg sync.WaitGroup
    var mu sync.Mutex

    for name, conn := range h.services {
        wg.Add(1)
        go func(name string, conn grpc.ClientConnInterface) {
            defer wg.Done()

            start := time.Now()
            state := conn.GetState()
            latency := time.Since(start)

            health := &ServiceHealth{
                Healthy:   state == connectivity.Ready,
                Status:    state.String(),
                LatencyMs: latency.Milliseconds(),
            }

            mu.Lock()
            response.Services[name] = health
            if !health.Healthy {
                response.Healthy = false
            }
            mu.Unlock()
        }(name, conn)
    }

    wg.Wait()
    return response
}
```
