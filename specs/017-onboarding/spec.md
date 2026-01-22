# Spec 017: Entity Onboarding & Documentation Architecture

## Overview

This specification defines two closely related features:

1. **Entity Onboarding Workflow** - CLI commands for seeding entities before import and guided review after import
2. **Hierarchical Documentation Architecture** - Self-documenting system for Claude agents

Both features enable a smooth first-time user experience and ongoing AI-assisted entity management.

## Problem Statement

When a new user imports 10-20 emails into Penfold:

1. The system encounters unknown entities (people, acronyms, products, projects)
2. There's no way to pre-seed known entities before import
3. After import, there's no guided experience to review what was discovered
4. Claude agents lack structured documentation to understand system concepts

**Current gaps:**
- No `penf init entities` to seed before import
- No `penf process onboarding context` to guide post-import review
- Workflow docs (`acronym-review.md`) exist but deeper concept docs don't
- Claude must load everything into context or guess

## Solution

### Phase 1: Entity Seeding (`penf init entities`)

Interactive wizard or JSON import to seed known entities before importing content.

```
penf init entities [--from-json <file>]
```

**Interactive mode:**
```
$ penf init entities

Entity Seeding Wizard
=====================

Let's set up your known entities before importing content.
This helps Penfold match mentions to the right people/products.

1. PEOPLE - Who will appear in your emails?
   Add key colleagues, customers, partners.

   Add person (or 'done' to continue):
   > Name: John Smith
   > Email: john.smith@company.com
   > Company: Acme Corp
   > Title (optional): VP Engineering

   Added: John Smith <john.smith@company.com>
   Add another? (y/n): n

2. PRODUCTS - What products/services does your org work on?

   Add product (or 'done' to continue):
   > Name: DBaaS
   > Description: Database as a Service platform
   > Aliases (comma-separated): Database Service, Managed DB

   Added: DBaaS
   Add another? (y/n): n

3. PROJECTS - What active initiatives should Penfold know about?

   Add project (or 'done' to continue):
   > Name: MTC
   > Description: Major TikTok Contract migration
   > Keywords (comma-separated): TikTok, migration, Oracle

   Added: MTC
   Add another? (y/n): n

4. GLOSSARY - What domain-specific acronyms should Penfold know?

   Add term (or 'done' to continue):
   > Term: TER
   > Expansion: Technical Execution Review
   > Context (comma-separated, optional): MTC, meetings

   Added: TER = Technical Execution Review
   Add another? (y/n): n

Summary:
  People:   3 added
  Products: 2 added
  Projects: 1 added
  Glossary: 5 added

Ready to import content! Run: penf ingest email <files>
```

**JSON import mode:**
```bash
penf init entities --from-json entities.json
```

```json
{
  "people": [
    {
      "name": "John Smith",
      "email": "john.smith@company.com",
      "company": "Acme Corp",
      "title": "VP Engineering"
    }
  ],
  "products": [
    {
      "name": "DBaaS",
      "description": "Database as a Service platform",
      "aliases": ["Database Service", "Managed DB"]
    }
  ],
  "projects": [
    {
      "name": "MTC",
      "description": "Major TikTok Contract migration",
      "keywords": ["TikTok", "migration", "Oracle"]
    }
  ],
  "glossary": [
    {
      "term": "TER",
      "expansion": "Technical Execution Review",
      "context": ["MTC", "meetings"]
    },
    {
      "term": "VIP",
      "expansion": "Very Important Person",
      "context": ["sales"]
    },
    {
      "term": "VIP",
      "expansion": "Virtual IP Address",
      "context": ["networking", "MTC"]
    }
  ]
}
```

**Multi-context glossary terms:**
The same acronym can have different meanings in different contexts. When resolving mentions, Penfold uses context (project, surrounding text) to pick the right expansion.

### Phase 2: Post-Import Onboarding (`penf process onboarding`)

After importing emails, Claude guides the user through reviewing discovered entities.

```bash
penf process onboarding context --output json
```

Returns:
```json
{
  "summary": {
    "new_people": 12,
    "new_acronyms": 15,
    "unresolved_mentions": 8,
    "potential_duplicates": 3
  },
  "new_people": [
    {
      "id": 45,
      "canonical_name": "Sarah Johnson",
      "email_addresses": ["sarah.j@external.com"],
      "auto_created": true,
      "needs_review": true,
      "source_count": 5,
      "first_seen": "2024-01-15T10:30:00Z"
    }
  ],
  "new_acronyms": [
    {
      "id": 123,
      "term": "PLD",
      "context": "...discussed in the PLD review yesterday...",
      "source_reference": "email-2024-01-15-001"
    }
  ],
  "unresolved_mentions": [
    {
      "id": 201,
      "mentioned_text": "JB",
      "context_snippet": "JB mentioned that...",
      "candidates": [
        {"person_id": 5, "name": "James Brown", "score": 0.85},
        {"person_id": 12, "name": "John Baker", "score": 0.72}
      ]
    }
  ],
  "potential_duplicates": [
    {
      "person_id": 45,
      "canonical_name": "Sarah Johnson",
      "potential_match": {
        "person_id": 3,
        "canonical_name": "Sarah J.",
        "similarity": 0.91
      }
    }
  ],
  "workflow": {
    "recommended_order": ["duplicates", "people", "acronyms", "mentions"],
    "commands": {
      "review_people": "penf process onboarding people",
      "review_acronyms": "penf process acronyms context",
      "review_mentions": "penf process mentions context",
      "merge_duplicate": "penf relationship entity merge <id1> <id2>"
    }
  }
}
```

**Claude's guided flow:**
```
After importing 15 emails, here's what Penfold discovered:

📊 Summary:
  - 12 new people (from email headers)
  - 15 unknown acronyms
  - 8 unresolved person mentions
  - 3 potential duplicate people

Let's review these together. I'll start with potential duplicates
since fixing those first prevents duplicate work.

──────────────────────────────────────────────────────────────────
POTENTIAL DUPLICATES (3)
──────────────────────────────────────────────────────────────────

1. "Sarah Johnson" <sarah.j@external.com> (new)
   Might be same as: "Sarah J." <sarah.johnson@external.com> (existing)

   → Merge these? (y/n/skip)

[... continues through each category ...]
```

**Batch processing:**
```bash
penf process onboarding batch '{
  "merge_people": [
    {"keep_id": 3, "merge_id": 45}
  ],
  "confirm_people": [12, 14, 16],
  "acronym_resolutions": [
    {"id": 123, "expansion": "Product Launch Date"}
  ],
  "mention_resolutions": [
    {"mention_id": 201, "person_id": 5}
  ]
}'
```

### Phase 3: Hierarchical Documentation

Documentation that ships with the CLI, installed to user's working directory.

**Source (embedded in binary):**
```
cmd/penf/cmd/templates/docs/
├── index.md                    # ROOT - Claude reads first
├── concepts/
│   ├── entities.md             # What are entities?
│   ├── glossary.md             # Multi-context terms
│   ├── people.md               # Person resolution
│   ├── products.md             # Product hierarchy
│   └── mentions.md             # Mention resolution
└── workflows/
    ├── init-entities.md        # Seeding workflow
    ├── onboarding.md           # Post-import review
    ├── acronym-review.md       # Processing acronyms
    └── mention-review.md       # Resolving mentions
```

**Installed location:**
```
./docs/                         # In user's working directory
├── index.md
├── concepts/...
└── workflows/...
```

**Root document (`index.md`):**
```markdown
# Penfold System Documentation

This documentation helps Claude understand Penfold's concepts and workflows.

## Quick Navigation

**Start here if you need to understand:**
- What entities exist → [concepts/entities.md](concepts/entities.md)
- How glossary/acronyms work → [concepts/glossary.md](concepts/glossary.md)
- How people are resolved → [concepts/people.md](concepts/people.md)
- How mentions become entities → [concepts/mentions.md](concepts/mentions.md)

**Workflows (how to do things):**
- Seed entities before import → [workflows/init-entities.md](workflows/init-entities.md)
- Review after import → [workflows/onboarding.md](workflows/onboarding.md)
- Process unknown acronyms → [workflows/acronym-review.md](workflows/acronym-review.md)
- Resolve person mentions → [workflows/mention-review.md](workflows/mention-review.md)

## System Overview

Penfold aggregates content from email, meetings, and documents into a
searchable knowledge base with entity resolution.

### Entity Types

| Type | Description | CLI Commands |
|------|-------------|--------------|
| People | Individuals with emails, aliases | `penf relationship entity` |
| Products | Business products, hierarchy | `penf product` |
| Projects | Initiatives, timelines | `penf project` |
| Teams | Groups of people | `penf team` |
| Glossary | Acronyms, terminology | `penf glossary` |

### Processing Flow

1. **Ingest** - Content enters the system
2. **Extract** - Entities and mentions are identified
3. **Resolve** - Mentions are matched to known entities
4. **Review** - Unknowns are queued for human review
5. **Search** - Content becomes queryable with entity context

For detailed explanations, follow the links above.
```

**Example concept doc (`concepts/glossary.md`):**
```markdown
# Glossary Terms

The glossary stores acronyms, abbreviations, and domain terminology.

## Multi-Context Terms

The same acronym can mean different things in different contexts.

**Example:** "VIP"
- In `sales` context: "Very Important Person" (customer tier)
- In `networking` context: "Virtual IP Address"
- In `MTC` project: "Virtual IP Address" (infrastructure term)

### How Context Resolution Works

When Penfold encounters "VIP" in content:
1. Looks at surrounding context (project, topic, keywords)
2. Checks the `context` tags on glossary entries
3. Picks the entry with best context match
4. Falls back to most common meaning if no context match

### Adding Multi-Context Terms

```bash
# Add VIP for sales context
penf glossary add VIP "Very Important Person" --context sales,customers

# Add VIP for networking context
penf glossary add VIP "Virtual IP Address" --context networking,MTC
```

### CLI Commands

| Command | Description |
|---------|-------------|
| `penf glossary list` | List all terms |
| `penf glossary list --context MTC` | Filter by context |
| `penf glossary add <term> <expansion>` | Add new term |
| `penf glossary show <term>` | Show term details |
| `penf glossary expand "<query>"` | Test query expansion |

### Related

- [Entity types overview](entities.md)
- [Acronym review workflow](../workflows/acronym-review.md)
```

## Implementation Plan

### Phase 1: Entity Seeding (pe-i6zf)

1. **Add CLI command structure**
   - `cmd/penf/cmd/init_entities.go`
   - Subcommand of `init` or standalone `penf init entities`

2. **Implement interactive wizard**
   - Prompt for each entity type
   - Validation and confirmation

3. **Implement JSON import**
   - `--from-json <file>` flag
   - Schema validation

4. **Backend support**
   - Bulk insert endpoints for people, products, projects
   - Gateway service methods

### Phase 2: Post-Import Onboarding (pe-i6zf)

1. **Add CLI commands**
   - `penf process onboarding context`
   - `penf process onboarding batch`

2. **Implement context aggregation**
   - Query new entities since last review
   - Identify potential duplicates
   - Gather unresolved mentions

3. **Implement batch processing**
   - Merge duplicates
   - Confirm people
   - Resolve acronyms and mentions

### Phase 3: Documentation Architecture (pe-slyu)

1. **Create docs templates**
   - `cmd/penf/cmd/templates/docs/` hierarchy
   - All markdown files

2. **Embed in binary**
   - `//go:embed` directives
   - File copy logic

3. **Update `penf init`**
   - Install docs to `./docs/`
   - Overwrite on `penf update`

4. **Update existing workflows**
   - Move `processes/acronym-review.md` content
   - Add cross-references

## Data Model Changes

### No schema changes required

All entity tables already exist:
- `people` - Has `auto_created`, `needs_review` flags
- `products` - Full hierarchy support
- `projects` - With keywords, status
- `glossary` - With context array
- `teams` - With member references

### New tracking (optional)

Could add `onboarding_sessions` table to track review progress:
```sql
CREATE TABLE onboarding_sessions (
  id BIGSERIAL PRIMARY KEY,
  tenant_id UUID NOT NULL,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  stats JSONB,  -- {people_reviewed: 5, acronyms_resolved: 10, ...}
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Testing Strategy

1. **Unit tests**
   - JSON schema validation
   - Entity creation logic

2. **Integration tests**
   - Full wizard flow (mocked stdin)
   - Batch processing

3. **E2E tests**
   - Import emails → run onboarding → verify entities

## Success Criteria

1. User can seed known entities before first import
2. After import, Claude can guide through discovered entities
3. Claude agents can navigate docs to answer system questions
4. Multi-context glossary terms are explained and work correctly
