# Thin CLI Specification

## Overview

The `penf` CLI runs on the user's laptop and communicates with the API Gateway on dev01 via gRPC. It is intentionally thin - all business logic resides in the backend services.

## Design Principles

1. **Minimal Dependencies**: Only gRPC client, config, and terminal UI
2. **No Local State**: All state lives on the server
3. **Fast Startup**: Sub-100ms cold start
4. **Offline Graceful**: Clear errors when server unreachable
5. **Rich Output**: Tables, progress bars, colors via lipgloss/bubbletea

## Connection Model

```
┌─────────────┐         gRPC (TLS)           ┌──────────────┐
│  penf CLI   │ ──────────────────────────▶  │ API Gateway  │
│  (laptop)   │                              │   (dev01)    │
└─────────────┘                              └──────────────┘
      │
      │ Config: ~/.config/penf/config.yaml
      │   - server: dev01.local:8080
      │   - auth_token: <encrypted>
      │   - tenant: work|personal|family
```

## Configuration

```yaml
# ~/.config/penf/config.yaml
server:
  address: "dev01.local:8080"
  tls: true
  timeout: 30s

auth:
  token_file: "~/.config/penf/token"  # encrypted

defaults:
  tenant: "work"
  output: "table"  # table, json, yaml
```

## Commands

### Connection & Auth

```bash
penf connect <server>          # Connect to server, store config
penf auth login                # OAuth2 flow, store token
penf auth status               # Show auth state
penf auth logout               # Clear stored token
```

### Tenant Management

```bash
penf tenant                    # Show current tenant
penf tenant list               # List available tenants
penf tenant switch <name>      # Switch tenant context
```

### Search & Query

```bash
penf search "query"            # Full-text + semantic search
penf search "query" --type email|meeting|document
penf search "query" --from 2024-01-01 --to 2024-12-31
penf search "query" --person "Sarah Chen"
penf search "query" --project "Atlas"
penf search "query" --json     # JSON output for scripting

# Ask natural language questions
penf ask "What decisions were made about Atlas last week?"
penf ask "Who was involved in the budget discussion?"
```

### Email Ingestion

```bash
penf ingest <path>             # Ingest .eml files from path
penf ingest <path> --label "work/projects"
penf ingest --status           # Show active ingest jobs
penf ingest --watch <path>     # Watch folder for new .eml files
```

### Gmail Sync

```bash
penf gmail status              # Show sync status for all accounts
penf gmail sync                # Trigger incremental sync
penf gmail sync --full         # Full historical sync
penf gmail accounts            # List connected accounts
penf gmail connect             # Add new Gmail account (OAuth flow)
penf gmail disconnect <email>  # Remove account
```

### Review Workflows

```bash
penf review                    # Start interactive review session
penf review --daily            # Show daily digest
penf review --weekly           # Show weekly summary
penf review queue              # Show pending items
penf review approve <id>       # Approve item
penf review reject <id>        # Reject item
penf review skip <id>          # Skip for now
```

### Relationships

```bash
penf relationships             # Show relationship network
penf relationships --person "Sarah Chen"
penf relationships --project "Atlas"
penf relationships pending     # Show pending validations
penf relationships validate <id> --confirm|--reject
```

### Health & Admin

```bash
penf health                    # Check all service health
penf health --verbose          # Detailed health per service
penf status                    # System status overview
penf version                   # CLI and server versions
```

## gRPC Service Definition

```protobuf
// api/proto/penf/v1/cli.proto

syntax = "proto3";
package penf.v1;

service PenfCLI {
  // Auth
  rpc GetAuthURL(GetAuthURLRequest) returns (GetAuthURLResponse);
  rpc ExchangeToken(ExchangeTokenRequest) returns (ExchangeTokenResponse);

  // Tenant
  rpc ListTenants(ListTenantsRequest) returns (ListTenantsResponse);
  rpc SwitchTenant(SwitchTenantRequest) returns (SwitchTenantResponse);

  // Search
  rpc Search(SearchRequest) returns (SearchResponse);
  rpc Ask(AskRequest) returns (AskResponse);

  // Ingest
  rpc IngestFiles(stream IngestFileChunk) returns (IngestResponse);
  rpc GetIngestStatus(GetIngestStatusRequest) returns (GetIngestStatusResponse);

  // Gmail
  rpc GetGmailStatus(GetGmailStatusRequest) returns (GetGmailStatusResponse);
  rpc TriggerGmailSync(TriggerGmailSyncRequest) returns (TriggerGmailSyncResponse);

  // Review
  rpc GetReviewQueue(GetReviewQueueRequest) returns (GetReviewQueueResponse);
  rpc SubmitReview(SubmitReviewRequest) returns (SubmitReviewResponse);

  // Relationships
  rpc GetRelationships(GetRelationshipsRequest) returns (GetRelationshipsResponse);
  rpc ValidateRelationship(ValidateRelationshipRequest) returns (ValidateRelationshipResponse);

  // Health
  rpc GetHealth(GetHealthRequest) returns (GetHealthResponse);
}

message SearchRequest {
  string query = 1;
  string tenant_id = 2;
  SearchFilters filters = 3;
  int32 limit = 4;
  int32 offset = 5;
}

message SearchFilters {
  repeated string content_types = 1;  // email, meeting, document
  string date_from = 2;               // ISO8601
  string date_to = 3;
  repeated string person_ids = 4;
  repeated string project_ids = 5;
  repeated string labels = 6;
}

message SearchResponse {
  repeated SearchResult results = 1;
  int32 total_count = 2;
  SearchMetadata metadata = 3;
}

message SearchResult {
  string id = 1;
  string content_type = 2;
  string title = 3;
  string snippet = 4;
  float relevance_score = 5;
  string timestamp = 6;
  repeated string people = 7;
  repeated string projects = 8;
}
```

## UI Components

Using [bubbletea](https://github.com/charmbracelet/bubbletea) and [lipgloss](https://github.com/charmbracelet/lipgloss):

### Search Results Table
```
┌──────────────────────────────────────────────────────────────────┐
│ Search: "Atlas project timeline"                          3 results │
├──────────────────────────────────────────────────────────────────┤
│ # │ Type  │ Title                        │ Date       │ Score   │
├───┼───────┼──────────────────────────────┼────────────┼─────────┤
│ 1 │ email │ Re: Atlas Timeline Update    │ 2024-01-15 │ 0.94    │
│ 2 │ email │ Atlas Project Kickoff        │ 2024-01-10 │ 0.87    │
│ 3 │ meet  │ Atlas Planning Session       │ 2024-01-08 │ 0.82    │
└──────────────────────────────────────────────────────────────────┘
[↑/↓] Navigate  [Enter] View  [q] Quit
```

### Review Session
```
┌──────────────────────────────────────────────────────────────────┐
│ Daily Review                                        3/15 reviewed │
├──────────────────────────────────────────────────────────────────┤
│ Email: Re: Budget Approval for Q2                                │
│ From: finance@company.com                                        │
│ Date: 2024-01-16 09:30                                          │
├──────────────────────────────────────────────────────────────────┤
│ AI Classification: project_update (confidence: 0.87)             │
│ Suggested Projects: Budget Planning, Q2 Initiative               │
│ Extracted People: Sarah Chen, James Brown                        │
├──────────────────────────────────────────────────────────────────┤
│ [a] Approve  [r] Reject  [e] Edit  [s] Skip  [q] Quit           │
└──────────────────────────────────────────────────────────────────┘
```

## Error Handling

```go
// Graceful connection errors
func (c *Client) Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
    resp, err := c.grpcClient.Search(ctx, req)
    if err != nil {
        if status.Code(err) == codes.Unavailable {
            return nil, fmt.Errorf("server unreachable at %s - is dev01 running?", c.serverAddr)
        }
        if status.Code(err) == codes.Unauthenticated {
            return nil, fmt.Errorf("authentication required - run 'penf auth login'")
        }
        return nil, fmt.Errorf("search failed: %w", err)
    }
    return resp, nil
}
```

## Implementation Structure

```
cmd/penf/
├── main.go                 # Entry point
├── cmd/
│   ├── root.go            # Root command, config loading
│   ├── connect.go         # Connection management
│   ├── auth.go            # Authentication commands
│   ├── tenant.go          # Tenant switching
│   ├── search.go          # Search and ask
│   ├── ingest.go          # File ingestion
│   ├── gmail.go           # Gmail commands
│   ├── review.go          # Review workflows
│   ├── relationships.go   # Relationship commands
│   └── health.go          # Health checks
├── internal/
│   ├── client/
│   │   └── grpc.go        # gRPC client wrapper
│   ├── config/
│   │   └── config.go      # Config loading
│   ├── ui/
│   │   ├── table.go       # Table rendering
│   │   ├── progress.go    # Progress bars
│   │   └── review.go      # Review TUI
│   └── auth/
│       └── token.go       # Token storage
└── go.mod
```

## Dependencies

```go
// go.mod
module github.com/otherjamesbrown/penfold/cmd/penf

require (
    google.golang.org/grpc v1.60.0
    google.golang.org/protobuf v1.32.0
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.0
    github.com/charmbracelet/bubbletea v0.25.0
    github.com/charmbracelet/lipgloss v0.9.0
    github.com/charmbracelet/bubbles v0.17.0
)
```

## Build & Distribution

```bash
# Build for macOS (laptop)
GOOS=darwin GOARCH=arm64 go build -o penf ./cmd/penf

# Install
go install ./cmd/penf

# Or use Homebrew tap (future)
brew install otherjamesbrown/tap/penf
```
