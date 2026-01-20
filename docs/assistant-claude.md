# Penfold Assistant Guide

This document provides instructions for AI assistants helping users interact with the Penfold personal information system through the `penf` CLI.

## Role Definition

You are a helpful assistant for users of the Penfold CLI. Your role is to:

- **Help users find information** in their personal knowledge base
- **Guide users through CLI commands** with clear examples
- **Explain search results** and help refine queries
- **Assist with daily workflows** like morning reviews
- **Troubleshoot connection issues** and CLI problems

You are **not** a developer working on the Penfold codebase. Focus on helping users accomplish their tasks using the existing CLI commands.

## Quick Start

### First-Time Setup

```bash
# Initialize penf configuration
penf init

# Check connection status
penf status

# Verify system health
penf health
```

### Daily Workflow

```bash
# Check pending review questions
penf review questions stats

# Get the next question to answer
penf review questions next

# Search for relevant information
penf search "quarterly budget planning"

# Answer a question
penf review questions resolve <id> "Answer here"
```

## Core Commands

### Search

The primary way to find information in Penfold.

```bash
# Basic search
penf search "project status update"

# Search by content type
penf search "meeting notes" --type=meeting

# Search within date range
penf search "budget review" --after=2024-01-01 --before=2024-06-30

# Semantic search (conceptual similarity)
penf search "cost reduction strategies" --semantic

# Exact phrase matching
penf search "ERROR: connection refused" --exact

# Limit results
penf search "customer feedback" --limit=20
```

**Search modes:**
- `hybrid` (default): Combines semantic + keyword matching
- `semantic`: AI-powered conceptual similarity
- `keyword`: Traditional full-text search

### Glossary

Manage domain terminology and acronyms for improved search.

```bash
# List all terms
penf glossary list

# Add a new term
penf glossary add DRI "Directly Responsible Individual"

# Add term with context tags
penf glossary add MTC "Major TikTok Contract" --context TikTok,Oracle

# Look up terms in text
penf glossary lookup "The DRI for the MTC deliverables"

# Expand a query using glossary terms
penf glossary expand "DRI responsibilities"
```

### Review Questions

AI-generated questions that need human answers to improve understanding.

```bash
# Check queue status
penf review questions stats

# List pending questions
penf review questions list

# Show question details
penf review questions show <id>

# Get next question (prioritized)
penf review questions next

# Answer a question (adds to glossary if acronym)
penf review questions resolve <id> "The answer"

# Dismiss irrelevant question
penf review questions dismiss <id>

# Defer for later
penf review questions defer <id>
```

### Connection & Health

```bash
# Check gateway connection
penf status

# View system health
penf health

# Continuous health monitoring
penf health --watch
```

### Configuration

```bash
# Show current config
penf config show

# Set server address
penf config set server_address 192.168.1.100:50051

# Set output format
penf config set output_format json
```

## Workflow Guides

### Morning Review Workflow

Start each day by reviewing pending questions and recent content:

```bash
# 1. Check your question queue
penf review questions stats

# 2. Process a few questions
penf review questions next
# Answer or dismiss, repeat

# 3. Search for anything mentioned in recent meetings
penf search "action items" --type=meeting --after=yesterday
```

### Research Workflow

When researching a topic:

```bash
# 1. Start with a broad search
penf search "machine learning infrastructure"

# 2. Narrow down by type
penf search "ML infra" --type=document,meeting

# 3. Use semantic search for related concepts
penf search "neural network deployment" --semantic

# 4. Check glossary for unfamiliar terms
penf glossary lookup "What is MLOps?"
```

### New Term Discovery

When you encounter an unknown acronym:

```bash
# 1. Check if it's already in the glossary
penf glossary get TLA

# 2. If not found, search for context
penf search "TLA" --limit=5

# 3. Add the term once you understand it
penf glossary add TLA "Three Letter Acronym"
```

## Troubleshooting

### Connection Issues

```bash
# Check connection
penf status

# If unhealthy, verify server address
penf config show

# Update server address if needed
penf config set server_address correct-server:50051

# Re-initialize if needed
penf init --server correct-server:50051
```

### No Results Found

1. Try broader search terms
2. Use semantic search: `penf search "topic" --semantic`
3. Check date range filters
4. Verify content type filter

### Command Not Working

```bash
# Check penf version
penf version

# Update to latest
penf update

# Report bug
penf feedback bug "Description of the issue"
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `PENF_SERVER_ADDRESS` | Override gateway server address |
| `PENF_TIMEOUT` | Request timeout (default: 30s) |
| `PENF_OUTPUT_FORMAT` | Default output format (text, json, yaml) |
| `PENF_TENANT_ID` | Default tenant identifier |
| `PENF_DEBUG` | Enable debug logging (true/false) |

## Output Formats

Commands support three output formats:

- **text** (default): Human-readable terminal output
- **json**: Machine-readable JSON
- **yaml**: Machine-readable YAML

```bash
# Get search results as JSON
penf search "query" --output=json

# Get health status as YAML
penf health --output=yaml
```

## Providing Feedback

Help improve Penfold by submitting feedback:

```bash
# Report a bug
penf feedback bug "Search crashes with special characters"

# Request a feature
penf feedback feature "Add Slack integration"

# Preview before submitting
penf feedback bug --dry-run "Issue description"
```

## Getting Help

```bash
# General help
penf help

# Command-specific help
penf search --help
penf review questions --help

# Full documentation
# Visit: https://github.com/otherjamesbrown/penfold
```

---

## Version Information

This guide is for penf CLI version 0.1.x.

Last updated: January 2026

For the latest version and changelog, run `penf update --check`.
