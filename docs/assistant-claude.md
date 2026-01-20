# Penfold CLI Reference for Claude

This document provides the `penf` CLI command reference for Claude to execute directly when assisting users.

## Role Definition

You (Claude) are an assistant with access to the Penfold personal information system via the `penf` CLI.

**Key principle: Execute commands directly. Never suggest commands for the user to run.**

When the user asks for information:
1. Run the appropriate `penf` command yourself using Bash
2. Parse the output
3. Present the results in a helpful format

The user will never run CLI commands themselves. You have full access to execute them.

## Output Format

**Always use `--format json` for machine-parseable output:**

```bash
penf glossary list --format json
penf review questions list --format json
penf search "query" --format json
```

This gives structured data you can parse and present meaningfully.

## Command Reference

### Search

Find information in the knowledge base.

```bash
# Basic search
penf search "project status" --format json

# By content type
penf search "meeting notes" --type=meeting --format json

# Date range
penf search "budget" --after=2024-01-01 --before=2024-06-30 --format json

# Semantic search (conceptual similarity)
penf search "cost reduction strategies" --semantic --format json

# Limit results
penf search "customer feedback" --limit=20 --format json
```

Search modes: `hybrid` (default), `semantic`, `keyword`

### Glossary

Domain terminology and acronyms.

```bash
# List all terms
penf glossary list --format json

# Show specific term
penf glossary show TER --format json

# Search terms
penf glossary search "database" --format json

# Add a term
penf glossary add DRI "Directly Responsible Individual"

# Add with context
penf glossary add MTC "Major TikTok Contract" --context TikTok,Oracle

# Expand query (see how acronyms would be expanded)
penf glossary expand "DRI responsibilities" --format json

# Remove a term
penf glossary remove TER
```

### Review Questions

AI-generated questions needing human answers.

```bash
# Queue statistics
penf review questions stats --format json

# List pending questions
penf review questions list --format json

# Filter by priority
penf review questions list --priority high --format json

# Filter by type
penf review questions list --type acronym --format json

# Get next prioritized question
penf review questions next --format json

# Show specific question
penf review questions show 123 --format json

# Get source content for a question (to see more context)
penf review questions source 123 --format json
penf review questions source 123 --context 1000 --format json  # More context
penf review questions source 123 --context -1 --format json    # Full content

# Resolve a question (adds to glossary if acronym type)
penf review questions resolve 123 "Technical Execution Review"

# Dismiss a question
penf review questions dismiss 123 "Not relevant"

# Defer for later
penf review questions defer 123
```

Question types: `acronym`, `person`, `entity`, `duplicate`, `other`
Priority levels: `high`, `medium`, `low`

### System Status

```bash
# Connection status
penf status

# System health
penf health --format json
```

### Configuration

```bash
# Show current config
penf config show

# Current config is at ~/.penf/config.yaml
```

## Common Workflows

### When user asks about a topic

1. Run search: `penf search "topic" --format json --limit=10`
2. Parse results and summarize findings
3. If acronyms are unclear, check glossary: `penf glossary show TERM --format json`

### When user asks about pending questions

1. Get stats: `penf review questions stats --format json`
2. List questions: `penf review questions list --format json`
3. Present summary to user

### When user wants to answer a question

1. Show the question details: `penf review questions show ID --format json`
2. Get user's answer
3. Submit: `penf review questions resolve ID "user's answer"`

### When user provides an acronym definition

1. Add to glossary: `penf glossary add TERM "Expansion" --context relevant,tags`
2. Confirm addition to user

### When user asks about a term/acronym

1. Check glossary: `penf glossary show TERM --format json`
2. If not found, search for context: `penf search "TERM" --format json --limit=5`
3. Report findings

## Error Handling

If a command fails:
1. Check connection: `penf status`
2. Report the specific error to the user
3. Suggest what might be wrong (network, server down, etc.)

## Environment

- Server: `home-01.brown.chat:50051`
- Config: `~/.penf/config.yaml`
- Binary: `/usr/local/bin/penf` or user's PATH

## Notes

- All commands support `--format json` for structured output (prefer this)
- Text output includes ANSI color codes - JSON is cleaner for parsing
- Questions resolved as acronym type are automatically added to glossary
- Search uses hybrid mode by default (semantic + keyword)
