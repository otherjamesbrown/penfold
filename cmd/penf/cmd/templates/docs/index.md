# Penfold System Documentation

This documentation helps Claude understand Penfold's concepts and workflows.
Read this file first, then follow links to go deeper.

## Quick Navigation

**Understand the system:**
- What entities exist → [concepts/entities.md](concepts/entities.md)
- How glossary/acronyms work → [concepts/glossary.md](concepts/glossary.md)
- How people are resolved → [concepts/people.md](concepts/people.md)
- How mentions become entities → [concepts/mentions.md](concepts/mentions.md)
- Products and hierarchy → [concepts/products.md](concepts/products.md)

**Workflows (how to do things):**
- Seed entities before import → [workflows/init-entities.md](workflows/init-entities.md)
- Review after import → [workflows/onboarding.md](workflows/onboarding.md)
- Process unknown acronyms → [workflows/acronym-review.md](workflows/acronym-review.md)
- Resolve person mentions → [workflows/mention-review.md](workflows/mention-review.md)

## System Overview

Penfold aggregates content from email, meetings, and documents into a
searchable knowledge base with automatic entity resolution.

### Entity Types

| Type | Description | CLI Commands |
|------|-------------|--------------|
| People | Individuals with emails, aliases | `penf relationship entity` |
| Products | Business products, hierarchy | `penf product` |
| Projects | Initiatives, timelines | (coming soon) |
| Teams | Groups of people | (coming soon) |
| Glossary | Acronyms, terminology | `penf glossary` |

### Processing Flow

```
┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
│ INGEST  │ ──▶ │ EXTRACT │ ──▶ │ RESOLVE │ ──▶ │ REVIEW  │ ──▶ │ SEARCH  │
└─────────┘     └─────────┘     └─────────┘     └─────────┘     └─────────┘
Content enters   Entities &      Match to        Queue unknowns  Content is
the system       mentions ID'd   known entities  for human       queryable
```

1. **Ingest** - Content (email, meetings) enters via `penf ingest`
2. **Extract** - System identifies people, acronyms, mentions
3. **Resolve** - Mentions are matched to known entities in the database
4. **Review** - Unknown entities are queued for human review via Claude
5. **Search** - Content becomes searchable with entity context

### First-Time Setup Flow

```
penf init              # Configure CLI connection
penf init entities     # Seed known people, products, projects, glossary
penf ingest email ...  # Import content
penf process onboarding context  # Claude guides you through new entities
```

## Key Concepts

### Multi-Context Terms

The same acronym can mean different things in different contexts.
See [concepts/glossary.md](concepts/glossary.md) for details.

**Example:** "VIP" might mean:
- "Very Important Person" in sales context
- "Virtual IP Address" in networking context

### Entity Resolution

When content mentions "JB said...", Penfold tries to resolve "JB" to a
known person. See [concepts/mentions.md](concepts/mentions.md) for details.

Resolution uses:
- Email addresses (exact match)
- Name aliases (configured matches)
- LLM disambiguation (when multiple candidates exist)

### Auto-Created vs Confirmed Entities

Entities have two creation modes:
- **Auto-created**: Discovered from content, needs review
- **Confirmed**: Explicitly added or reviewed by user

Auto-created entities have `needs_review: true` and should be reviewed
via the onboarding workflow.

## CLI Quick Reference

```bash
# Status & Health
penf status                    # Check gateway connection
penf health                    # System health overview

# Search
penf search "query"            # Search content
penf search "query" -o json    # JSON output for Claude

# Glossary
penf glossary list             # List all terms
penf glossary add TERM "Expansion"  # Add term
penf glossary show TERM        # Show term details

# Entity Management
penf relationship entity list  # List entities
penf product list              # List products
penf product add "Name"        # Add product

# AI Workflows
penf process acronyms context  # Get acronym review context
penf process mentions context  # Get mention resolution context
penf process onboarding context  # Get post-import review context

# Review Queue
penf review questions list     # List pending questions
penf review questions resolve ID "answer"  # Answer question
```

## For More Information

Each concept and workflow document provides detailed explanations,
examples, and CLI command references. Follow the links in Quick Navigation above.
