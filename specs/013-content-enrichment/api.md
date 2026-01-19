# API Surface

Part of [Content Enrichment Pipeline](spec.md)

---

## Overview

Enrichment data is exposed via:
1. **Shared query library** (`pkg/enrichment/query/`) - used by CLI and services
2. **Extended search service** - enrichment filters and data in search results
3. **CLI commands** - direct access to enrichment data

---

## Query Library

### People Queries

```go
// pkg/enrichment/query/people.go
type PeopleQuery interface {
    GetPerson(ctx context.Context, id string) (*Person, error)
    SearchPeople(ctx context.Context, query string, filters PeopleFilters) ([]*Person, error)
    GetPersonCommunications(ctx context.Context, personID string, timeRange TimeRange) ([]*Source, error)
}
```

### Thread Queries

```go
// pkg/enrichment/query/threads.go
type ThreadsQuery interface {
    GetThread(ctx context.Context, id string) (*Thread, error)
    GetThreadsForPerson(ctx context.Context, personID string, timeRange TimeRange) ([]*Thread, error)
    GetThreadsForProject(ctx context.Context, projectID string, timeRange TimeRange) ([]*Thread, error)
}
```

### Assertion Queries

```go
// pkg/enrichment/query/assertions.go
type AssertionsQuery interface {
    GetAssertions(ctx context.Context, filters AssertionFilters) ([]*Assertion, error)
    GetOpenActions(ctx context.Context, assigneeID string) ([]*Assertion, error)
    GetRecentDecisions(ctx context.Context, projectID string, days int) ([]*Assertion, error)
}
```

### Jira Queries

```go
// pkg/enrichment/query/jira.go
type JiraQuery interface {
    GetTicket(ctx context.Context, key string) (*JiraTicket, error)
    GetTicketReferences(ctx context.Context, key string) ([]*Source, error)
    GetTicketHistory(ctx context.Context, key string) ([]*TicketChange, error)
}
```

### Project Queries

```go
// pkg/enrichment/query/projects.go
type ProjectsQuery interface {
    GetProject(ctx context.Context, id string) (*Project, error)
    GetProjectActivity(ctx context.Context, id string, timeRange TimeRange) ([]*ActivityItem, error)
}
```

### Status Queries

```go
// pkg/enrichment/query/status.go
type StatusQuery interface {
    GetEnrichmentStatus(ctx context.Context, sourceID string) (*EnrichmentStatus, error)
    GetEnrichmentStats(ctx context.Context, tenantID string, timeRange TimeRange) (*EnrichmentStats, error)
}
```

---

## Filter Types

```go
type AssertionFilters struct {
    Type        string   // risk, action, issue, decision, commitment, question
    Status      string   // open, in_progress, completed, cancelled (for actions)
    AssigneeID  string
    OwnerID     string
    ProjectID   string
    TicketID    string
    DateRange   TimeRange
    IsCurrent   *bool    // Filter to current (not superseded) assertions
}

type PeopleFilters struct {
    AccountType  string   // person, bot, distribution, role_account
    IsInternal   *bool
    HasRecent    *bool    // Has communications in last 30 days
}

type TimeRange struct {
    Start time.Time
    End   time.Time
}
```

---

## Search Service Extension

### New Filter Fields

```protobuf
// api/proto/search.proto additions
message SearchRequest {
    // ... existing fields ...

    // Enrichment filters
    repeated string participant_ids = 10;      // Filter by person_id
    repeated string project_ids = 11;          // Filter by project
    repeated string content_types = 12;        // email, calendar, document
    repeated string content_subtypes = 13;     // thread, notification/jira
    bool has_actions = 14;                     // Has open actions
    bool has_decisions = 15;                   // Contains decisions
    string thread_id = 16;                     // Get all messages in thread
}
```

### Enriched Search Results

```protobuf
message SearchResult {
    // ... existing fields ...

    // Enrichment data
    EnrichmentSummary enrichment = 20;
}

message EnrichmentSummary {
    string content_type = 1;
    string content_subtype = 2;
    repeated ParticipantInfo participants = 3;
    string project_name = 4;
    string thread_id = 5;
    int32 thread_message_count = 6;
    int32 action_count = 7;
    int32 decision_count = 8;
}

message ParticipantInfo {
    string person_id = 1;
    string name = 2;
    string role = 3;           // from, to, cc
    bool is_internal = 4;
}
```

---

## CLI Commands

### People Commands

```bash
# List people
penf people list [--type person|bot|distribution] [--internal] [--needs-review]

# Show person details
penf people show <id|email>
# Output includes: aliases, recent threads, team memberships, project involvement

# Search people
penf people search "Rick"

# Communications
penf people comms <id> [--since 7d] [--limit 20]
```

### Thread Commands

```bash
# List threads
penf threads list [--person <id>] [--project <id>] [--since 7d]

# Show thread
penf threads show <id>
# Output: all messages, participants, decisions, actions, summary

# Thread summary
penf threads summary <id>
```

### Assertion Commands

```bash
# List assertions
penf assertions list [--type action|decision|risk] [--project <id>] [--open]

# Open actions
penf actions open [--assignee <id>]

# Recent decisions
penf decisions recent [--project <id>] [--days 30]
```

### Jira Commands

```bash
# Ticket details
penf jira show <key>
# Output: current state, history, related emails

# Ticket history
penf jira history <key>

# Emails referencing ticket
penf jira refs <key>
```

### Project Commands

```bash
# List projects
penf projects list

# Show project
penf projects show <id>
# Output: members, teams, jira tickets, recent activity

# Project activity
penf projects activity <id> [--since 30d]
```

### Enrichment Commands

```bash
# View enrichment status
penf enrichment status <source_id>

# Trace through pipeline
penf enrichment trace <source_id>

# Recent errors
penf enrichment errors [--last 1h]

# Queue status
penf enrichment queues

# Replay extraction
penf enrichment replay <source_id> [--template current|<version>]
```

---

## Functional Requirements

- **FR-700**: System MUST provide query library in `pkg/enrichment/query/` for reuse
- **FR-701**: System MUST extend search service with enrichment filters
- **FR-702**: System MUST include enrichment summary in search results
- **FR-703**: System MUST support filtering by participant, project, content type
- **FR-704**: System MUST support thread retrieval with all messages
- **FR-705**: System MUST support assertion queries with status/type/assignee filters
- **FR-706**: System MUST provide CLI commands for all query operations
- **FR-707**: System MUST support enrichment stats for monitoring
