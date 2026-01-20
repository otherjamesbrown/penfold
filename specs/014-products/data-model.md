# Data Model: Products & Organizational Entities

**Feature**: 014-products
**Date**: 2026-01-20
**Status**: Draft

## Entity Overview

```
Product (hierarchy via parent_id)
├── ProductTeam *──1 Team
│   └── ProductTeamRole *──1 Person (scoped roles)
└── ProductEvent (timeline)
    └── ProductEventLink (to meetings, emails, etc.)

Person (extended with country)
└── TeamMember *──1 Team
    └── ProductTeamRole (via product_team)

Navigation paths:
- Person → TeamMember → Team → ProductTeam → Product
- Product → ProductTeam → Team → TeamMember → Person
```

## Schema Changes to Existing Tables

### People Table Extension

Add country field to existing `people` table:

```sql
ALTER TABLE people ADD COLUMN IF NOT EXISTS country VARCHAR(100);
CREATE INDEX IF NOT EXISTS idx_people_country ON people (tenant_id, country) WHERE country IS NOT NULL;
```

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| country | VARCHAR(100) | NULL | ISO country name (e.g., "Poland", "United States") |

---

## New Entities

### Product

Business product with 3-level hierarchy support.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PK | Product identifier |
| tenant_id | VARCHAR(255) | NOT NULL | Tenant isolation |
| name | VARCHAR(255) | NOT NULL | Product name (e.g., "LKE Enterprise") |
| parent_id | BIGINT | FK(products.id), NULL | Parent product for hierarchy |
| product_type | VARCHAR(50) | NOT NULL, DEFAULT 'product' | product, sub_product, feature |
| description | TEXT | NULL | Product description |
| status | VARCHAR(50) | NOT NULL, DEFAULT 'active' | active, beta, sunset, deprecated |
| keywords | TEXT[] | DEFAULT '{}' | Search keywords |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update |

**Indexes**:
- `idx_products_tenant_name` UNIQUE ON (tenant_id, name)
- `idx_products_parent` ON (parent_id)
- `idx_products_type` ON (tenant_id, product_type)
- `idx_products_status` ON (tenant_id, status)
- `idx_products_keywords` USING GIN ON (keywords)

**Constraints**:
- CHECK: product_type IN ('product', 'sub_product', 'feature')
- CHECK: status IN ('active', 'beta', 'sunset', 'deprecated')
- CHECK: (parent_id IS NULL AND product_type = 'product') OR (parent_id IS NOT NULL AND product_type IN ('sub_product', 'feature'))

---

### ProductAlias

Alternative names for products (e.g., "LK Enterprise" → "LKE Enterprise").

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PK | Alias identifier |
| product_id | BIGINT | FK(products.id), NOT NULL | Referenced product |
| alias | VARCHAR(255) | NOT NULL | Alias value |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_product_aliases_product` ON (product_id)
- `idx_product_aliases_lookup` UNIQUE ON (alias) -- aliases must be globally unique

**Constraints**:
- CHECK: length(trim(alias)) > 0

---

### ProductTeam

Associates a team with a product with context (Core Team, DRI Team, etc.).

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PK | Association identifier |
| tenant_id | VARCHAR(255) | NOT NULL | Tenant isolation |
| product_id | BIGINT | FK(products.id), NOT NULL | Associated product |
| team_id | BIGINT | FK(teams.id), NOT NULL | Associated team |
| context | VARCHAR(100) | NULL | Team context (e.g., "Core Team", "DRI Team", "Engineering") |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update |

**Indexes**:
- `idx_product_teams_product` ON (product_id)
- `idx_product_teams_team` ON (team_id)
- `idx_product_teams_unique` UNIQUE ON (product_id, team_id, context) -- same team can have multiple contexts

**Constraints**:
- At least one of product_id and team_id must be NOT NULL (implicit via FK)

---

### ProductTeamRole

Scoped role assignment: person has role X in context of product Y through team Z.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PK | Role assignment identifier |
| tenant_id | VARCHAR(255) | NOT NULL | Tenant isolation |
| product_team_id | BIGINT | FK(product_teams.id), NOT NULL | Product-team association |
| person_id | BIGINT | FK(people.id), NOT NULL | Person with the role |
| role | VARCHAR(100) | NOT NULL | Role name (e.g., "DRI", "Manager", "Lead", "Contributor") |
| scope | VARCHAR(100) | NULL | Role scope (e.g., "Networking", "Database", "Security") |
| is_active | BOOLEAN | NOT NULL, DEFAULT TRUE | Whether role is currently active |
| started_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Role start date |
| ended_at | TIMESTAMPTZ | NULL | Role end date (NULL if active) |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update |

**Indexes**:
- `idx_product_team_roles_product_team` ON (product_team_id)
- `idx_product_team_roles_person` ON (person_id)
- `idx_product_team_roles_active` ON (tenant_id, is_active) WHERE is_active = TRUE
- `idx_product_team_roles_lookup` ON (product_team_id, role, scope) WHERE is_active = TRUE

**Constraints**:
- CHECK: length(trim(role)) > 0
- CHECK: (is_active = TRUE AND ended_at IS NULL) OR (is_active = FALSE AND ended_at IS NOT NULL)

---

### ProductEvent

Timeline entry for product decisions, milestones, and risks.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PK | Event identifier |
| event_uuid | UUID | UNIQUE, NOT NULL | External reference |
| tenant_id | VARCHAR(255) | NOT NULL | Tenant isolation |
| product_id | BIGINT | FK(products.id), NOT NULL | Associated product |
| event_type | VARCHAR(50) | NOT NULL | decision, milestone, risk, note |
| title | VARCHAR(500) | NOT NULL | Event title/summary |
| description | TEXT | NULL | Detailed description |
| occurred_at | TIMESTAMPTZ | NOT NULL | When the event occurred |
| recorded_by | VARCHAR(255) | NULL | Who recorded this event |
| metadata | JSONB | DEFAULT '{}' | Additional structured data |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update |

**Indexes**:
- `idx_product_events_product` ON (product_id, occurred_at DESC)
- `idx_product_events_type` ON (tenant_id, event_type)
- `idx_product_events_occurred` ON (tenant_id, occurred_at DESC)
- `idx_product_events_uuid` ON (event_uuid)

**Constraints**:
- CHECK: event_type IN ('decision', 'milestone', 'risk', 'note')
- CHECK: length(trim(title)) > 0

---

### ProductEventLink

Links product events to other entities (meetings, emails, documents).

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PK | Link identifier |
| event_id | BIGINT | FK(product_events.id), NOT NULL | Parent event |
| linked_entity_type | VARCHAR(50) | NOT NULL | meeting, email, document, source |
| linked_entity_id | BIGINT | NOT NULL | ID of linked entity |
| link_type | VARCHAR(50) | NOT NULL | source, reference, follow_up |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_product_event_links_event` ON (event_id)
- `idx_product_event_links_entity` ON (linked_entity_type, linked_entity_id)

**Constraints**:
- CHECK: linked_entity_type IN ('meeting', 'email', 'document', 'source')
- CHECK: link_type IN ('source', 'reference', 'follow_up')
- UNIQUE (event_id, linked_entity_type, linked_entity_id)

---

## State Transitions

### Product Status

```
active -> beta (for new products in testing)
active -> sunset (deprecation announced)
sunset -> deprecated (no longer supported)
beta -> active (general availability)
```

### ProductTeamRole Active State

```
is_active=TRUE, ended_at=NULL (current assignment)
    -> is_active=FALSE, ended_at=NOW() (role ended)

Historical roles preserved for queries like "who was DRI for X in Q3"
```

---

## Relationships to Existing Entities

| New Entity | Existing Entity | Relationship | Notes |
|------------|-----------------|--------------|-------|
| Product | Glossary | Conceptual | Product names may be in glossary as terms |
| ProductTeam | Team | Many-to-One | Each association references one team |
| ProductTeamRole | Person | Many-to-One | Each role assignment references one person |
| ProductEventLink | Source | Many-to-One | Events can link to sources (meetings, emails) |
| Person.country | Person | Extension | New column on existing table |

---

## Query Patterns

### "Who is the DRI for networking on MTC?"

```sql
SELECT p.canonical_name, p.primary_email, ptr.role, ptr.scope
FROM product_team_roles ptr
JOIN product_teams pt ON ptr.product_team_id = pt.id
JOIN products prod ON pt.product_id = prod.id
JOIN people p ON ptr.person_id = p.id
WHERE prod.name = 'MTC'
  AND ptr.scope ILIKE '%networking%'
  AND ptr.role = 'DRI'
  AND ptr.is_active = TRUE;
```

### "Who is on MTC in Poland?"

```sql
SELECT DISTINCT p.canonical_name, p.primary_email, p.country, ptr.role, ptr.scope
FROM people p
JOIN product_team_roles ptr ON p.id = ptr.person_id
JOIN product_teams pt ON ptr.product_team_id = pt.id
JOIN products prod ON pt.product_id = prod.id
WHERE prod.name = 'MTC'
  AND p.country = 'Poland'
  AND ptr.is_active = TRUE;
```

### "What products does James work on?"

```sql
SELECT prod.name, prod.product_type, t.name as team_name, pt.context, ptr.role, ptr.scope
FROM products prod
JOIN product_teams pt ON prod.id = pt.product_id
JOIN teams t ON pt.team_id = t.id
JOIN product_team_roles ptr ON pt.id = ptr.product_team_id
JOIN people p ON ptr.person_id = p.id
WHERE p.canonical_name ILIKE '%james%'
  AND ptr.is_active = TRUE
ORDER BY prod.name, t.name;
```

### "Show me LKE Enterprise timeline"

```sql
SELECT pe.event_type, pe.title, pe.description, pe.occurred_at, pe.recorded_by
FROM product_events pe
JOIN products prod ON pe.product_id = prod.id
WHERE prod.name = 'LKE Enterprise'
ORDER BY pe.occurred_at DESC;
```

### "Product hierarchy for LKE"

```sql
WITH RECURSIVE product_tree AS (
  SELECT id, name, parent_id, product_type, 0 as depth
  FROM products
  WHERE name = 'LKE' AND parent_id IS NULL

  UNION ALL

  SELECT p.id, p.name, p.parent_id, p.product_type, pt.depth + 1
  FROM products p
  JOIN product_tree pt ON p.parent_id = pt.id
)
SELECT * FROM product_tree ORDER BY depth, name;
```

---

## Migration Strategy

1. **Phase 1**: Extend people table with country column
2. **Phase 2**: Create products table with hierarchy
3. **Phase 3**: Create product_aliases table
4. **Phase 4**: Create product_teams table
5. **Phase 5**: Create product_team_roles table
6. **Phase 6**: Create product_events and product_event_links tables
7. **Phase 7**: Add indexes and constraints
8. **Phase 8**: Create CLI commands for product management

---

## CLI Command Structure

```
penf product                    # List all products
penf product add <name>         # Add new product
  --parent <product>            # Parent product (for sub-product/feature)
  --type <type>                 # product, sub_product, feature
  --description <text>          # Description

penf product info <name>        # Show product details with teams
penf product hierarchy <name>   # Show product hierarchy tree

penf product team <product>     # List teams for product
penf product team add <product> <team> [--context <ctx>]  # Add team
penf product team role <product> <team> <person> <role>   # Add role
  --scope <scope>               # Optional scope (networking, database)

penf product timeline <name>    # Show product timeline
penf product event <product>    # Add timeline event
  --type <type>                 # decision, milestone, risk, note
  --title <title>
  --description <text>
  --occurred <date>
```
