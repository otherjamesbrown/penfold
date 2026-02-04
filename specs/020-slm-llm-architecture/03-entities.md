# Penfold: Entity Model

## Overview

Penfold is entity-centric. All content (emails, meetings, documents) is linked to entities through mention resolution, and entities are linked to each other through participation, ownership, and context. The entity model drives search expansion, knowledge discovery, and the review queue.

## Entity Types

### People

People are the most connected entity type. They appear as:
- Email senders/recipients (auto-created from headers)
- Meeting attendees and participants
- Names mentioned in content text
- Product team members
- Project members

**Key fields**: canonical_name, primary_email, account_type (person/role/distribution/bot/external_service), is_internal, confidence score, needs_review flag

**Seniority and Trust** (see `00-overview.md` — collaboration philosophy, `design.md` — concrete implementation):

People carry two weighting signals that affect how the system surfaces information. These are **separate signals used for different purposes** — they are never combined into a single numeric weight.

| Signal | Source | Purpose | Used by |
|--------|--------|---------|---------|
| **seniority_tier** (1-7) | Organizational fact (title/level) | Escalation detection: a senior person entering a previously junior discussion is an organizational signal. | Stage 4 context (visible to LLM), seniority escalation queries, briefing ordering (tier 3) |
| **trust_level** (0-5) | Human-assigned, subjective | "When this person says it's an issue, I believe them." | Briefing ordering (tier 2), Stage 4 participant context (visible to LLM) |
| **trust_domains** (text[]) | Human-assigned | Domain-specific trust: e.g. `['technical-risk', 'timeline']`. When set, trust applies only to matching assertion types. | Briefing ordering (refines trust_level applicability), Stage 4 context |

These are **different axes**. A trusted senior IC may carry more weight than an unfamiliar VP. A VP you don't trust joining a discussion is still an escalation signal. The system uses them separately:
- **Seniority** → escalation detection (who just got involved?), Stage 4 participant context (observable org fact)
- **Trust** → briefing ordering (which assertions surface first?), Stage 4 participant context (LLM can reference trust in observations)
- **Briefing priority tiers**: watched items → trusted source → senior source → everything else (see `design.md` for the full query)

**Aliases**: Each person can have multiple identifiers:
- Email addresses (multiple)
- Slack IDs
- Display names
- Nicknames ("Bob" → "Robert Smith")
- Typos ("Jmaes" → "James")

**Resolution priority**: Exact email match → Confirmed alias → Higher confidence → More recent activity

### Products

Products represent business products, services, or features. They support a 3-level hierarchy:

```
Product (top-level)
├── Sub-Product
│   ├── Feature
│   └── Feature
└── Sub-Product
    └── Feature
```

**Key fields**: name, product_type (product/sub_product/feature), parent_id, status (active/beta/sunset/deprecated/archived)

**Relationships**:
- Products have **team associations** with context labels
- Team members have **scoped roles** (e.g., DRI for PostgreSQL Support) with active date ranges
- Products have **timeline events** (decisions, milestones, risks, releases, competitor moves)
- Events link back to **source content** (which meeting/email is the evidence)

### Projects

Projects are time-bounded initiatives or programs.

**Key fields**: name, description, keywords (text array for matching), jira_projects (text array for integration), status (active/archived)

**Note:** The `projects` table currently has no `status` column in the schema. This needs to be added — see `07-session-bootstrap.md` for the migration. Archived projects are excluded from morning bootstrap and Stage 4 context but remain searchable and drillable on demand.

**Relationships**:
- Project members (people or teams) with roles
- Content is assigned to projects during enrichment
- Assertions (risks, actions, decisions) link to projects
- Entity-project affinity tracks how strongly an entity relates to a project

### Teams

Teams are groups of people.

**Key fields**: name, description, team_type (department/project_team/working_group/committee)

**Relationships**: Team members with optional roles; teams can be project members; teams can be associated with products

### Glossary Terms

Domain-specific acronyms and terminology used for search query expansion and content understanding.

**Key fields**: term, expansion, definition, context tags, aliases, expand_in_search flag, embedding (vector 1024)

**Multi-context support**: The same acronym can mean different things:
- "VIP" in sales = "Very Important Person"
- "VIP" in networking = "Virtual IP Address"
- Context tags disambiguate based on surrounding content

**Search integration**: When a user searches for "TER meeting notes", it expands to "(TER OR 'Technical Execution Review') meeting notes"

## Entity Relationships Map

```
People ──────────────────────────────┐
  │                                   │
  ├── member_of ──▶ Teams            │
  ├── member_of ──▶ Projects         │
  ├── scoped_role_in ──▶ Products    │
  ├── mentioned_in ──▶ Content       │
  ├── author_of ──▶ Content          │
  ├── attendee_of ──▶ Meetings       │
  ├── owner_of ──▶ Assertions        │
  └── assignee_of ──▶ Assertions     │
                                      │
Teams ────────────────────────────────┤
  ├── member_of ──▶ Projects         │
  └── associated_with ──▶ Products   │
                                      │
Products ─────────────────────────────┤
  ├── parent_of ──▶ Products         │
  ├── has_events ──▶ Product Events  │
  └── linked_to ──▶ Glossary Terms   │
                                      │
Projects ─────────────────────────────┤
  ├── has_assertions ──▶ Assertions  │
  └── has_content ──▶ Content        │
                                      │
Glossary ─────────────────────────────┘
  └── linked_to ──▶ Products/Projects
```

## Mention Resolution Pipeline

When content is ingested, Penfold extracts and resolves mentions through a 4-stage pipeline:

### Stage 1: Extraction
Find potential mentions in text:
- NER (Named Entity Recognition) for person names
- Pattern matching for emails, @mentions
- Acronym detection for uppercase sequences
- Product/project name matching

### Stage 2: Candidate Generation
Find possible matches for each mention:
- Exact email match (highest confidence)
- Alias lookup (configured aliases)
- Fuzzy name matching (for typos)
- Pattern database lookup (learned resolutions)
- Project context matching

### Stage 3: Disambiguation
Pick the right match:
- Single high-confidence candidate → auto-resolve
- Multiple candidates → LLM disambiguation using context
- No candidates → queue for human review

### Stage 4: Outcome
- **RESOLVED** (auto_resolved or llm_resolved): Linked to entity
- **PENDING**: Queued for human review
- **DISMISSED**: Not an entity mention

### Pattern Learning
When a mention is resolved (by human or AI), Penfold can create a resolution pattern:
```
"JB" → resolved to James Brown → pattern created
Next time "JB" appears → auto-resolves without LLM call
```

Patterns can be:
- Global (always resolve this way)
- Project-scoped ("JB" in MTC context → James Brown; "JB" in Sales → John Baker)
- Permanent or frequency-based

### Candidate Scoring

Resolution is **project-scoped first**. When processing content about project MTC, candidates with high affinity to MTC rank above candidates from other projects. This prevents cross-project ambiguity (e.g., "JB" in an MTC email resolves to the JB most associated with MTC, not the most globally frequent JB).

| Factor | Weight | Description |
|--------|--------|-------------|
| Project affinity | Highest | Candidate's affinity to the content's resolved project |
| Historical pattern | High | Previously resolved same way |
| Alias match | High | Direct alias match |
| Name similarity | Medium | Fuzzy match to canonical name |
| Context match | Medium | Related to current project/topic |
| Recency | Medium | Recently mentioned people score higher |

## Entity Lifecycle

### Auto-Creation
Entities are auto-created during content processing:
- Email headers → Person entities (auto_created=true, needs_review=true)
- Unknown acronyms → Review queue items
- New names → Resolved or queued

### Review & Confirmation
Auto-created entities move through:
1. Created with low confidence
2. Queued for review (appears in review_queue)
3. Claude Code processes review batch (batch processing pattern)
4. Human confirms, merges duplicates, or dismisses
5. Confirmed entities get higher trust in future resolution

### Duplicate Detection
The system tracks potential duplicates:
```
Person A: "Sarah Johnson" <sarah.j@company.com>
Person B: "Sarah J." <sarah.johnson@company.com>
→ Potential duplicate detected (0.91 similarity)
```
Merging combines email addresses, aliases, and updates all references.

## Entity-Project Affinity

The `entity_project_affinity` table tracks how strongly entities relate to projects:
- mention_count: How often they co-occur
- affinity_score (0.0-1.0): Computed strength
- is_member: Whether explicitly added as member
- role: If they have a formal role

This powers contextual resolution: when "JB" appears in MTC project content, the system checks which "JB" has highest affinity to MTC.

## Integration Points

### Glossary ↔ Search
- Query expansion at both index time and query time
- Semantic matching via glossary embeddings
- Context-aware disambiguation

### People ↔ Content
- Author/sender tracking
- Participant resolution
- Mention extraction and linking
- Expertise inference (future)

### Products ↔ Assertions
- Risks, decisions, actions linked to products
- Timeline events provide product history
- Source content evidence chain

### Projects ↔ Everything
- Content assigned to projects during classification
- Assertions grounded to projects
- People affiliated via membership or mention affinity
- Glossary terms scoped to project context
