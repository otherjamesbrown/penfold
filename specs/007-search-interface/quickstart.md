# Quickstart: Search and Query Interface

**Feature**: 007-search-interface
**Date**: 2026-01-15

## Overview

The Penfold search interface provides unified search across all indexed content using natural language queries, temporal filters, and cross-content correlation discovery.

## Prerequisites

- Penfold CLI installed (`penf` command available)
- PostgreSQL database running with indexed content
- Redis running for caching (optional but recommended)
- Tenant context set up

## CLI Commands

### Basic Search

Search across all content types:

```bash
# Simple natural language search
penf search "customer deployment issues"

# Search with limited results
penf search "Atlas project" --limit 10

# Search with verbose output
penf search "budget discussions" -v
```

### Temporal Queries

Search with time-based filters:

```bash
# Using relative expressions
penf search "team meetings" --since "last week"
penf search "project updates" --since "December"
penf search "deadlines" --before "next Monday"

# Using date ranges
penf search "quarterly review" --from 2026-01-01 --to 2026-03-31

# Timeline reconstruction
penf search timeline "Atlas project" --granularity week
```

### Content Type Filtering

Filter by specific content types:

```bash
# Search only emails
penf search "deployment" --type email

# Search meetings and documents
penf search "architecture decisions" --type meeting,document

# Exclude specific types
penf search "budget" --exclude slack
```

### Advanced Filtering

Apply multiple filters:

```bash
# Filter by participants
penf search "project status" --participant john@company.com

# Filter by project
penf search "deadlines" --project "Q1 Planning"

# Filter by confidence
penf search "decisions" --min-confidence 0.8

# Combine filters
penf search "budget review" \
  --type email,meeting \
  --participant finance@company.com \
  --since "last month" \
  --min-confidence 0.7
```

### Related Content Discovery

Find related discussions:

```bash
# Find content related to a specific email
penf search related source 12345

# Find content related to a person
penf search related person 678

# Find content related to a project
penf search related project 42
```

### Search Sessions

Manage search sessions for history tracking:

```bash
# Start a new search session
penf search session start

# View search history
penf search history

# View session details
penf search session show

# End current session
penf search session end
```

### Query Suggestions

Get search suggestions:

```bash
# Autocomplete suggestions
penf search suggest "deploy"

# Popular queries
penf search popular

# Recent queries
penf search recent
```

## Output Formats

### Default (Pretty)

```
Search Results for "customer deployment issues"

Found 15 results in 342ms (hybrid search, cache miss)

1. [EMAIL] Re: Deployment Issues - 2026-01-10 14:30
   From: john@company.com
   ...the customer reported issues with the deployment pipeline...
   Relevance: 0.92 | Confidence: 0.88
   Related: 3 emails, 1 meeting

2. [MEETING] Customer Support Standup - 2026-01-09 10:00
   Participants: john@company.com, jane@company.com
   ...discussed customer feedback on deployment delays...
   Relevance: 0.87 | Confidence: 0.91

[... more results ...]

Suggestions: "deployment pipeline", "customer feedback", "deployment timeline"
```

### JSON Output

```bash
# Get JSON output for programmatic use
penf search "deployment" --format json
```

```json
{
  "metadata": {
    "query_id": "550e8400-e29b-41d4-a716-446655440000",
    "execution_time_ms": 342,
    "total_results": 15,
    "returned_results": 15,
    "search_strategy": "hybrid",
    "cache_hit": false
  },
  "results": [
    {
      "result_id": "550e8400-e29b-41d4-a716-446655440001",
      "entity_type": "source",
      "entity_id": 12345,
      "content_type": "email",
      "preview": {
        "title": "Re: Deployment Issues",
        "snippet": "...the customer reported issues with the deployment pipeline..."
      },
      "relevance_score": 0.92,
      "confidence_score": 0.88,
      "timestamp": "2026-01-10T14:30:00Z",
      "participants": ["john@company.com"],
      "related_content_ids": ["email-123", "email-124", "meeting-456"]
    }
  ],
  "suggestions": ["deployment pipeline", "customer feedback"],
  "has_more": false
}
```

### Compact Output

```bash
# One-line-per-result for scripting
penf search "deployment" --format compact
```

```
source:12345 | EMAIL | 2026-01-10 | 0.92 | Re: Deployment Issues
source:12340 | MEETING | 2026-01-09 | 0.87 | Customer Support Standup
```

## Library Usage

For programmatic access, use the search library:

```python
from penf_lib.search import SearchEngine, SearchQuery, TemporalConstraint

# Initialize search engine
engine = SearchEngine(session=db_session)

# Build a search query
query = SearchQuery(
    query_text="customer deployment issues",
    content_types=["email", "meeting"],
    temporal=TemporalConstraint(relative_expression="last week"),
    limit=25,
)

# Execute search
response = await engine.search(query)

# Process results
for result in response.results:
    print(f"{result.content_type}: {result.preview.title}")
    print(f"  Relevance: {result.relevance_score:.2f}")
    print(f"  Related: {len(result.related_content_ids)} items")
```

### Timeline Reconstruction

```python
from penf_lib.search import TimelineQuery

timeline_query = TimelineQuery(
    topic="Atlas project",
    start_date=datetime(2026, 1, 1),
    end_date=datetime(2026, 3, 31),
    granularity="week",
)

timeline = await engine.build_timeline(timeline_query)

for entry in timeline.timeline_entries:
    print(f"\n{entry.date.strftime('%Y-%m-%d')}")
    print(f"  Evolution: {entry.evolution_note}")
    for content in entry.content:
        print(f"  - {content.preview.title}")
```

### Cross-Content Correlation

```python
from penf_lib.search import CorrelationDiscovery

correlations = CorrelationDiscovery(session=db_session)

# Find related content
related = await correlations.find_related_content(
    entity_type="source",
    entity_id=12345,
    limit=10,
)

for item in related:
    print(f"{item.relationship_type}: {item.result.preview.title}")
    print(f"  Strength: {item.strength:.2f}")
```

## Common Use Cases

### 1. Find All Discussions About a Decision

```bash
penf search "Atlas architecture decision" --since "last quarter"
```

### 2. Trace Who Was Involved in a Project

```bash
penf search "Atlas project" --type meeting --limit 50 --format json \
  | jq '.results[].participants' | sort | uniq -c | sort -rn
```

### 3. Build Context for a Meeting

```bash
# Before a meeting, gather relevant background
penf search "Q1 Planning" --since "2 weeks ago" --participant attendee@company.com
```

### 4. Find Follow-ups to a Decision

```bash
# Start from a known decision email
penf search related source 12345 --limit 20
```

### 5. Weekly Summary of Project Activity

```bash
penf search "My Project" --since "last Monday" --to "last Friday" --format json \
  | jq '.results | group_by(.content_type) | map({type: .[0].content_type, count: length})'
```

## Performance Tips

1. **Use temporal filters**: Narrowing the date range significantly improves performance
2. **Limit result count**: Start with smaller limits and paginate
3. **Use content type filters**: Searching specific types is faster than "all"
4. **Let Redis cache work**: Repeated queries are much faster
5. **Use sessions**: Session context improves suggestion quality

## Troubleshooting

### No Results Found

```bash
# Check if content is indexed
penf status index

# Try broader search terms
penf search "deploy" instead of "deployment configuration yaml update"

# Check temporal filter is not too narrow
penf search "deploy" --since "last year"
```

### Slow Searches

```bash
# Check performance metrics
penf search "query" -v  # Shows execution time breakdown

# Enable Redis caching
export PENFOLD_REDIS_URL=redis://localhost:6379

# Check index health
penf status search
```

### Unexpected Results

```bash
# View search explanation
penf search "query" --explain

# Check which strategy was used
penf search "query" -v  # Shows "hybrid", "full_text", or "semantic"
```

## Next Steps

- Explore [Search API Reference](./contracts/search-api.yaml) for full API details
- Review [Data Model](./data-model.md) for entity structures
- See [Research](./research.md) for technical decisions
