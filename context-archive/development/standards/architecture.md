# Architecture Standards

## CLI Architecture (MANDATORY)

**The CLI must use the Gateway service via gRPC. Never call the database directly.**

```
CLI (penf) → Gateway (gRPC) → Database
```

### Rules

- All CLI commands that need data must go through `services/gateway`
- Use gRPC clients to call Gateway services (see `cmd/penf/cmd/product.go` for pattern)
- File parsing and user interaction stay in CLI (hybrid approach)
- Database operations, duplicate detection, and business logic live in Gateway

### Key Locations

| Component | Location |
|-----------|----------|
| Proto definitions | `api/proto/<service>/v1/` |
| Gateway services | `services/gateway/<service>service/` |
| CLI commands | `cmd/penf/cmd/` |
| Generated protos | `api/proto/<service>/v1/*.pb.go` |

## Service Communication

- **gRPC + Protocol Buffers** for service communication
- **Temporal workflows** for durable execution
- **Test-driven development** workflow
- **Complete workflows** - specification to working, tested, committed code

## Current Architecture (Go)

| Component | Location | Purpose |
|-----------|----------|---------|
| CLI Tool | `cmd/penf` | Cobra-based CLI with all commands |
| Gateway | `services/gateway` | gRPC + HTTP gateway with auth |
| Gmail Connector | `services/gmail` | OAuth2 PKCE, sync, push notifications |
| Worker | `services/worker` | Temporal activities and workflows |
| Database | `pkg/db` | PostgreSQL + pgvector utilities |
| Protos | `api/proto` | gRPC service definitions |

## Technology Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.22+ |
| CLI | Cobra |
| API | gRPC + Protocol Buffers |
| Database | PostgreSQL 16+ with pgvector |
| Workflows | Temporal |
| Embeddings | MLX (Apple Silicon sidecar) |

## See Also

- `context/ARCHITECTURE.md` - Full system architecture
- `context/infrastructure.md` - Deployment topology, connection strings
