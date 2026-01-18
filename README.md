# Penfold

Personal AI-powered contextual information system that aggregates and correlates information from communication channels (email, Slack, documents, meetings) into a queryable institutional memory.

## Architecture

Penfold is built with Go for high performance and reliability:

```
cmd/penf/           # CLI application
services/
├── gateway/        # API Gateway (gRPC + HTTP)
├── gmail/          # Gmail Connector (OAuth2, sync, push notifications)
└── worker/         # Temporal worker for background processing
pkg/
├── db/             # Database utilities (PostgreSQL + pgvector)
├── tracing/        # Distributed tracing
├── temporal/       # Temporal workflow SDK
└── embeddings/     # Vector embedding generation
api/proto/          # Protocol Buffer definitions
```

### Core Services

| Service | Description | Port |
|---------|-------------|------|
| Gateway | API gateway with auth, routing, rate limiting | 8080 (HTTP), 9090 (gRPC) |
| Gmail | OAuth2 PKCE, real-time sync, push notifications | - |
| Worker | Temporal activities and workflows | - |

### Technology Stack

- **Language**: Go 1.22+
- **Database**: PostgreSQL 16+ with pgvector extension
- **Workflows**: Temporal for durable execution
- **API**: gRPC with Protocol Buffers, HTTP gateway
- **Embeddings**: MLX on Apple Silicon (via sidecar)
- **Search**: Hybrid full-text + vector similarity

## Getting Started

### Prerequisites

- Go 1.22+
- PostgreSQL 16+ with pgvector
- Temporal server
- Redis (optional, for caching)

### Installation

```bash
# Clone the repository
git clone https://github.com/otherjamesbrown/penfold.git
cd penfold

# Build the CLI
go build -o penf ./cmd/penf

# Or install directly
go install ./cmd/penf
```

### Configuration

Copy the example environment file and configure:

```bash
cp .env.example .env
```

Key configuration:
- `DATABASE_URL`: PostgreSQL connection string
- `TEMPORAL_ADDRESS`: Temporal server address
- `GMAIL_CLIENT_ID`: Google OAuth2 client ID
- `GMAIL_CLIENT_SECRET`: Google OAuth2 client secret

### Running

```bash
# Start the gateway
./penf gateway start

# Start the worker
./penf worker start

# Or run services via docker-compose
docker-compose up -d
```

### CLI Usage

```bash
# Check system health
penf health

# Search content
penf search "project status meeting"

# Manage Gmail accounts
penf auth gmail add
penf auth gmail list

# View relationships
penf relationship list
penf relationship show <entity-id>

# Daily review
penf review pending
penf review process

# Manage tenants
penf tenant list
penf tenant create <name>
```

## Project Structure

```
.
├── api/proto/              # Protocol Buffer definitions
│   ├── gateway/v1/         # Gateway service
│   ├── search/v1/          # Search service
│   ├── review/v1/          # Review service
│   └── ...
├── cmd/penf/               # CLI application
│   ├── cmd/                # Command implementations
│   ├── client/             # gRPC client
│   └── config/             # CLI configuration
├── pkg/                    # Shared packages
│   ├── db/                 # Database utilities
│   ├── temporal/           # Temporal SDK helpers
│   └── tracing/            # Observability
├── services/               # Backend services
│   ├── gateway/            # API Gateway
│   ├── gmail/              # Gmail Connector
│   └── worker/             # Temporal Worker
├── specs/                  # Feature specifications
├── docs/                   # Documentation
└── penfold-go-pipeline/    # MLX embeddings sidecar
```

## Development

### Building

```bash
# Build all
go build ./...

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Generate protobuf code
buf generate
```

### Workflow Management

Penfold uses a bead-based workflow system for task tracking:

```bash
# Find available work
bd ready

# Claim a task
bd update <bead-id> --status=in_progress

# Complete a task
bd close <bead-id> --reason="Implementation complete"

# Sync with remote
bd sync
```

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Run `go vet` and `staticcheck` before commits
- Reference beads in commits: `feat(component): description [pe-xxxx]`

## Documentation

- [Architecture Patterns](context/ARCHITECTURE.md)
- [Gmail Integration](docs/gmail-integration/README.md)
- [Search Interface](docs/search/README.md)
- [Database Schema](docs/database-schema/README.md)
- [Feature Specifications](specs/)

## Contributing

1. Find or create a bead for your work: `bd ready`
2. Update status: `bd update <id> --status=in_progress`
3. Implement with tests
4. Reference bead in commit: `git commit -m "feat: description [pe-xxxx]"`
5. Close bead: `bd close <id> --reason="summary"`
6. Push changes: `git push`

## License

MIT
