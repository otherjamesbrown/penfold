# Implementation Plan: Products & Organizational Entities

**Feature**: 014-products
**Date**: 2026-01-20
**Epic**: pe-i68n

## Implementation Phases

### Phase 1: Database Foundation

**1.1 Products Migration** (pe-1bc7)
- Create `migrations/004_products.sql`
- Tables: products, product_aliases, product_teams, product_team_roles, product_events, product_event_links
- All indexes and constraints from data-model.md
- Extended event types: decision, milestone, risk, release, competitor, org_change, market, note
- Event visibility and source_type fields

**1.2 Person Country Extension** (pe-on1n)
- Add country column to people table
- Add index for country queries
- Update Person struct in Go

---

### Phase 2: Go Types & Repository

**2.1 Product Types** (part of pe-nisy)
- `pkg/products/types.go`:
  - Product struct with hierarchy support
  - ProductAlias struct
  - ProductType enum (product, sub_product, feature)
  - ProductStatus enum (active, beta, sunset, deprecated)

**2.2 Product Repository** (part of pe-nisy)
- `pkg/products/repository.go`:
  - Create, Get, Update, Delete product
  - GetByName, GetByAlias (resolve aliases)
  - GetHierarchy (recursive CTE)
  - ListByParent, ListTopLevel
  - AddAlias, RemoveAlias

**2.3 ProductTeam Types & Repository** (new bead)
- `pkg/products/team_types.go`:
  - ProductTeam struct
  - ProductTeamRole struct with scoped roles
- `pkg/products/team_repository.go`:
  - AssociateTeam, DissociateTeam
  - AddRole, UpdateRole, EndRole (set ended_at)
  - GetTeamsForProduct
  - GetProductsForTeam
  - GetRolesForPerson
  - FindByRole (the "who is DRI for X on Y" query)

**2.4 ProductEvent Types & Repository** (new bead)
- `pkg/products/event_types.go`:
  - ProductEvent struct with extended types
  - ProductEventLink struct
  - EventType enum
  - EventVisibility enum
  - EventSourceType enum
- `pkg/products/event_repository.go`:
  - CreateEvent, GetEvent, UpdateEvent
  - ListByProduct (with filters: type, visibility, date range)
  - LinkToSource, UnlinkFromSource
  - GetContextWindow (events around a date)

**2.5 Person Repository Update** (part of pe-on1n)
- Update `pkg/enrichment/entities/repository.go`:
  - Add country to Create/Update
  - Add FindByCountry query
  - Update existing tests

---

### Phase 3: CLI Implementation

**3.1 Product CLI - Core** (pe-o230)
- `cmd/penf/cmd/product.go`:
  - `penf product` - list all products
  - `penf product add <name>` - create product
  - `penf product info <name>` - show details
  - `penf product hierarchy <name>` - show tree
  - Flags: --parent, --type, --description, --status

**3.2 Product CLI - Teams** (pe-qs6z)
- `cmd/penf/cmd/product_team.go`:
  - `penf product team <product>` - list teams
  - `penf product team add <product> <team>` - associate team
  - `penf product team remove <product> <team>` - dissociate
  - `penf product team role add <product> <team> <person> <role>` - add role
  - `penf product team role remove <product> <team> <person>` - end role
  - Flags: --context, --scope

**3.3 Product CLI - Timeline** (pe-dhs4)
- `cmd/penf/cmd/product_timeline.go`:
  - `penf product timeline <product>` - show timeline
  - `penf product event add <product>` - add event
  - Flags: --type, --visibility, --title, --description, --occurred, --since, --until, --link

---

### Phase 4: Integration

**4.1 Search Integration** (pe-lzkf)
- Index product names, aliases, keywords
- Add product filter to search
- Return product context in search results

**4.2 AI Query Integration** (pe-lzkf)
- Enable natural language queries:
  - "Who is the DRI for networking on MTC?"
  - "Show me the LKE timeline"
  - "What decisions were made about pricing?"
- Provide event context for AI reasoning

---

### Phase 5: Testing

**5.1 Repository Unit Tests**
- `pkg/products/repository_test.go`:
  - TestCreateProduct, TestGetProduct, TestUpdateProduct
  - TestProductHierarchy
  - TestProductAliases
- `pkg/products/team_repository_test.go`:
  - TestAssociateTeam
  - TestScopedRoles
  - TestFindByRole (DRI query)
- `pkg/products/event_repository_test.go`:
  - TestCreateEvent (all types)
  - TestEventFiltering
  - TestContextWindow

**5.2 CLI Integration Tests**
- Test each CLI command with real database
- Verify output formatting
- Test error cases

**5.3 Query Pattern Tests**
- Test all query patterns from data-model.md:
  - "Who is the DRI for networking on MTC?"
  - "Who is on MTC in Poland?"
  - "What products does James work on?"
  - "Show me LKE Enterprise timeline"
  - "What was happening around the pricing decision?"

**5.4 Navigation Tests**
- Test Person → Teams → Products path
- Test Products → Teams → People path
- Verify bidirectional consistency

---

## Dependency Graph

```
Phase 1 (Foundation)
├── 1.1 Migration ──────────────────────┐
└── 1.2 Person Country ─────────────────┤
                                        │
Phase 2 (Types & Repository)            │
├── 2.1 Product Types ◄─────────────────┤
├── 2.2 Product Repository ◄────────────┼── depends on 2.1
├── 2.3 Team Types & Repo ◄─────────────┼── depends on 2.1, 2.2
├── 2.4 Event Types & Repo ◄────────────┼── depends on 2.1, 2.2
└── 2.5 Person Repo Update ◄────────────┘
                                        │
Phase 3 (CLI)                           │
├── 3.1 Product CLI Core ◄──────────────┼── depends on 2.2
├── 3.2 Product CLI Teams ◄─────────────┼── depends on 2.3, 3.1
└── 3.3 Product CLI Timeline ◄──────────┼── depends on 2.4, 3.1
                                        │
Phase 4 (Integration)                   │
└── 4.1/4.2 Search & AI ◄───────────────┘── depends on 2.2, 2.4

Phase 5 (Testing) - parallel with each phase
```

---

## Estimated Scope

| Phase | Beads | Complexity |
|-------|-------|------------|
| Phase 1 | 2 | Low |
| Phase 2 | 4 | Medium |
| Phase 3 | 3 | Medium |
| Phase 4 | 1 | Medium |
| Phase 5 | 4 | Medium |
| **Total** | **14** | |

---

## Implementation Order

1. **pe-1bc7** - Database migration (all tables)
2. **pe-on1n** - Person country extension
3. **pe-nisy** - Product types and repository
4. **NEW** - ProductTeam types and repository
5. **NEW** - ProductEvent types and repository
6. **pe-o230** - CLI product commands
7. **pe-qs6z** - CLI product team commands
8. **pe-dhs4** - CLI product timeline commands
9. **pe-lzkf** - Search and AI integration
10. **NEW** - Repository unit tests
11. **NEW** - CLI integration tests
12. **NEW** - Query pattern tests
13. **NEW** - Navigation tests
