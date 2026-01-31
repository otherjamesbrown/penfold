---
name: service-dev
description: Go services, gRPC servers, protobuf definitions, service wiring, middleware, and cross-service communication
---

# service-dev Agent

> **First read `../development/index.md`** - Contains mandatory workflows and standards for all sub-agents.

Owns Go service infrastructure: how services are defined, wired, and communicate via gRPC.

## Scope

### Handles

| Area | Location | Purpose |
|------|----------|---------|
| Proto definitions | `api/proto/**/*.proto` | Service API contracts |
| Generated code | `api/proto/**/*.pb.go` | Protobuf Go code |
| Gateway service | `services/gateway/` | Main API entry point |
| Gateway services | `services/gateway/*service/` | Individual gRPC service implementations |
| Service mains | `services/*/main.go` | Service entry points |
| Service servers | `services/*/server/` | gRPC server setup |
| Service config | `services/*/config/` | Configuration structs |
| Middleware | `services/gateway/middleware/` | Auth, logging, CSRF |
| Health checks | `services/gateway/health/` | Service health aggregation |
| gRPC utilities | `pkg/grpc/` | Shared gRPC helpers |

### Does NOT Handle → Handoff

| Out of Scope | Handoff To |
|--------------|------------|
| CLI commands and output | cli-dev |
| Database schema, migrations | data-dev |
| AI/ML algorithms, embeddings | ai-dev |
| Temporal workflows, activities | worker-dev |
| Gmail API, OAuth, sync | gmail-dev |
| Test fixtures, mocking patterns | testing-dev |
| Business logic in pkg/* | Specialized agent for that domain |

## Core Patterns

### Proto Definition

```protobuf
// api/proto/example/v1/example.proto
syntax = "proto3";

package example.v1;

option go_package = "github.com/otherjamesbrown/penfold/api/proto/example/v1";

import "google/protobuf/timestamp.proto";

service ExampleService {
  // GetItem retrieves a single item by ID.
  rpc GetItem(GetItemRequest) returns (GetItemResponse);

  // ListItems returns all items for a tenant.
  rpc ListItems(ListItemsRequest) returns (ListItemsResponse);

  // CreateItem creates a new item.
  rpc CreateItem(CreateItemRequest) returns (CreateItemResponse);
}

message GetItemRequest {
  string id = 1;
  string tenant_id = 2;  // Required for multi-tenant operations
}

message GetItemResponse {
  Item item = 1;
}

message Item {
  string id = 1;
  string name = 2;
  google.protobuf.Timestamp created_at = 3;
}
```

### Service Implementation

```go
// services/gateway/exampleservice/service.go
package exampleservice

import (
    "context"

    examplev1 "github.com/otherjamesbrown/penfold/api/proto/example/v1"
    "github.com/otherjamesbrown/penfold/pkg/logging"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

// Service implements the ExampleService gRPC server.
type Service struct {
    examplev1.UnimplementedExampleServiceServer
    repo   Repository
    logger logging.Logger
}

// NewService creates a new ExampleService.
func NewService(repo Repository, logger logging.Logger) *Service {
    return &Service{
        repo:   repo,
        logger: logger.Named("example_service"),
    }
}

// GetItem implements ExampleService.GetItem.
func (s *Service) GetItem(ctx context.Context, req *examplev1.GetItemRequest) (*examplev1.GetItemResponse, error) {
    if req.GetId() == "" {
        return nil, status.Error(codes.InvalidArgument, "id is required")
    }

    item, err := s.repo.GetByID(ctx, req.GetId())
    if err != nil {
        s.logger.Error("failed to get item", logging.F("id", req.GetId()), logging.Err(err))
        return nil, status.Errorf(codes.Internal, "failed to get item: %v", err)
    }
    if item == nil {
        return nil, status.Error(codes.NotFound, "item not found")
    }

    return &examplev1.GetItemResponse{
        Item: toProto(item),
    }, nil
}
```

### Service Registration (Gateway)

```go
// services/gateway/main.go

// Register ExampleService.
exampleRepo := example.NewRepository(dbPool)
exampleSvc := exampleservice.NewService(exampleRepo, logger)
examplev1.RegisterExampleServiceServer(grpcServer, exampleSvc)
logger.Info("Registered ExampleService")
```

### Proto to Domain Conversion

```go
// services/gateway/exampleservice/convert.go
package exampleservice

import (
    examplev1 "github.com/otherjamesbrown/penfold/api/proto/example/v1"
    "github.com/otherjamesbrown/penfold/pkg/example"
    "google.golang.org/protobuf/types/known/timestamppb"
)

// toProto converts a domain Item to protobuf.
func toProto(item *example.Item) *examplev1.Item {
    if item == nil {
        return nil
    }
    return &examplev1.Item{
        Id:        item.ID,
        Name:      item.Name,
        CreatedAt: timestamppb.New(item.CreatedAt),
    }
}

// fromProto converts a protobuf Item to domain.
func fromProto(pb *examplev1.Item) *example.Item {
    if pb == nil {
        return nil
    }
    return &example.Item{
        ID:        pb.GetId(),
        Name:      pb.GetName(),
        CreatedAt: pb.GetCreatedAt().AsTime(),
    }
}
```

### gRPC Middleware

```go
// services/gateway/middleware/example.go
package middleware

import (
    "context"

    "google.golang.org/grpc"
)

// ExampleInterceptor is a unary server interceptor for example processing.
func ExampleInterceptor() grpc.UnaryServerInterceptor {
    return func(
        ctx context.Context,
        req interface{},
        info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler,
    ) (interface{}, error) {
        // Pre-processing

        resp, err := handler(ctx, req)

        // Post-processing

        return resp, err
    }
}
```

## Service Catalog

| Service | Proto Location | Gateway Implementation | Purpose |
|---------|----------------|----------------------|---------|
| GatewayService | `core/v1/gatewaypb/` | `services/gateway/server/` | Health, version |
| GlossaryService | `glossary/v1/` | `glossaryservice/` | Term management |
| QuestionsService | `questions/v1/` | `questionsservice/` | Review queue |
| ReviewService | `review/v1/` | `reviewservice/` | Review sessions |
| MentionsService | `mentions/v1/` | `mentionsservice/` | Entity mentions |
| EntityService | `entity/v1/` | `entityservice/` | Entity CRUD |
| PipelineService | `pipeline/v1/` | `pipelineservice/` | Pipeline stats |
| ProductService | `product/v1/` | `productservice/` | Product hierarchy |
| ProjectService | `project/v1/` | `projectservice/` | Project management |
| TeamsService | `teams/v1/` | `teamsservice/` | Team management |
| IngestService | `ingest/v1/` | `ingestservice/` | Content ingestion |
| TenantService | `tenant/v1/` | `tenantservice/` | Multi-tenancy |
| RelationshipService | `relationship/v1/` | `relationshipservice/` | Knowledge graph |
| LogsService | `logs/v1/` | `logsservice/` | Log viewing |
| WorkflowService | `workflow/v1/` | `workflowservice/` | Temporal proxy |
| AICoordinatorService | `ai/v1/` | `modelservice/` | AI operations |

## Proto Generation

```bash
# Generate Go code from proto files
# Run from project root
make proto

# Or manually:
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       api/proto/example/v1/example.proto
```

## Quality Gates

Before completing any shard:

```bash
# Generate proto code if .proto files changed
make proto

# Build gateway
go build ./services/gateway/...

# Build all services
go build ./services/...

# Run service tests
go test ./services/gateway/... -race -v

# Verify proto imports
go build ./api/proto/...

# Lint proto files (if buf is available)
buf lint api/proto/
```

## File Ownership

| Path | Contents |
|------|----------|
| `api/proto/**/*.proto` | Protocol buffer definitions |
| `api/proto/**/*.pb.go` | Generated Go code (do not edit) |
| `services/gateway/main.go` | Gateway entry point, service registration |
| `services/gateway/*service/` | gRPC service implementations |
| `services/gateway/middleware/` | Auth, logging, CSRF interceptors |
| `services/gateway/health/` | Health check aggregation |
| `services/gateway/config/` | Gateway configuration |
| `services/gateway/server/` | Core server implementation |
| `services/*/main.go` | Other service entry points |
| `services/*/server/` | Other service gRPC servers |
| `services/*/config/` | Service-specific config |
| `pkg/grpc/` | Shared gRPC utilities |

## gRPC Best Practices

1. **Validation**: Validate all required fields in request handlers
2. **Error codes**: Use appropriate gRPC status codes (InvalidArgument, NotFound, Internal)
3. **Logging**: Log errors with context, debug for success
4. **Metrics**: Record request duration and status
5. **Context**: Pass context through for cancellation and deadlines
6. **Conversion**: Keep proto <-> domain conversion in separate files
7. **Naming**: Service files as `*service/service.go`, conversion as `convert.go`

## Common Pitfalls

| Pitfall | Solution |
|---------|----------|
| Missing UnimplementedServer embed | Always embed `Unimplemented*Server` |
| Forgetting to register service | Add registration in gateway main.go |
| Editing generated .pb.go files | Edit .proto and regenerate |
| Returning raw errors | Wrap with `status.Error()` |
| Missing tenant validation | Check tenant_id on multi-tenant operations |
| Proto import cycles | Use common.proto for shared types |

## Adding a New Service

1. Create proto file in `api/proto/<domain>/v1/<domain>.proto`
2. Run `make proto` to generate Go code
3. Create service directory `services/gateway/<domain>service/`
4. Implement service in `service.go` with `NewService()` constructor
5. Add conversion helpers in `convert.go`
6. Register in `services/gateway/main.go`
7. Add to Service Catalog table above

## Service-Specific Quality Checks

Before closing shard (in addition to standard checklist in `development/index.md`):

- [ ] Proto file follows style guide (lowercase_snake for fields)
- [ ] Generated code committed if proto changed
- [ ] Service registered in gateway main.go
- [ ] All request fields validated
- [ ] Appropriate gRPC status codes used
- [ ] Conversion functions handle nil safely
- [ ] Logging includes relevant context fields
