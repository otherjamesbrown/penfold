# Feature Specification: Products & Organizational Entities

**Feature Branch**: `014-products`
**Created**: 2026-01-20
**Status**: Draft
**Bead**: pe-5csn (Epic: pe-i68n)
**Input**: User discussion on Products as knowledge hubs for SaaS company context

## Overview

Products are first-class entities representing business products (LKE, LKE Enterprise), sub-products, and features. They serve as knowledge hubs that aggregate decisions, research, team information, and cross-references to meetings, emails, and customers.

This extends the existing People/Teams/Projects model with:
- **Products** with 3-level hierarchy (Product → Sub-Product → Feature)
- **Product-Team associations** with scoped roles
- **Timeline/Decision log** for product events
- **Bidirectional navigation**: Person → Teams → Products and Products → Teams → People
- **Country field on People** for geographic queries

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Product Information Queries (Priority: P1)

As a product manager, I need to query information about my products so that I can quickly find decisions, feature status, and team assignments.

**Why this priority**: Core use case - products as knowledge hubs require basic CRUD and querying.

**Independent Test**: Can be fully tested by creating products, adding metadata, and verifying query results.

**Acceptance Scenarios**:

1. **Given** LKE and LKE Enterprise are defined as products, **When** user queries "what products do we have", **Then** system lists all products with hierarchy
2. **Given** a product has sub-products and features, **When** user queries product details, **Then** hierarchy is displayed (LKE → Managed Databases → PostgreSQL)
3. **Given** product has associated competitive research, **When** user asks about competitors, **Then** linked documents are retrievable

---

### User Story 2 - Team and Role Queries (Priority: P1)

As a manager, I need to find who owns what on which product so that I can direct questions to the right people.

**Why this priority**: Critical for organizational navigation - the "who is the DRI for X on Y" query pattern.

**Independent Test**: Can be fully tested by creating team associations with scoped roles and querying by role/product/domain.

**Acceptance Scenarios**:

1. **Given** James is DRI for networking on MTC, **When** user asks "who is the DRI for networking on MTC", **Then** system returns James with role context
2. **Given** same person has different roles on different products, **When** user asks "what are James's responsibilities", **Then** all role assignments are listed by product
3. **Given** MTC has a Core Team and DRI Team, **When** user asks "who is on the MTC team", **Then** system lists members grouped by team type

---

### User Story 3 - Geographic Team Queries (Priority: P1)

As a project lead, I need to find team members by location so that I can coordinate across time zones and regions.

**Why this priority**: Essential for distributed teams - "who is the engineer on MTC in Poland" is a common query pattern.

**Independent Test**: Can be fully tested by adding country to people records and querying by product + country combination.

**Acceptance Scenarios**:

1. **Given** engineers on MTC are in multiple countries, **When** user asks "who is on MTC in Poland", **Then** system returns Polish team members with their roles
2. **Given** person's country is known, **When** viewing person details, **Then** country is displayed
3. **Given** product team spans regions, **When** user asks "where is the MTC team located", **Then** system provides geographic breakdown

---

### User Story 4 - Decision Timeline (Priority: P2)

As a product owner, I need to track when decisions were made about my product so that I can understand historical context and revisit past choices.

**Why this priority**: Enables institutional memory - "when did we decide on pricing" requires temporal event tracking.

**Independent Test**: Can be fully tested by recording decisions with timestamps and querying by product + date range.

**Acceptance Scenarios**:

1. **Given** pricing decision was made for LKE Enterprise, **When** user asks "when did we decide on LKE Enterprise pricing", **Then** decision event is returned with date and context
2. **Given** multiple decisions over time, **When** user requests product timeline, **Then** events are displayed chronologically
3. **Given** decision was discussed in a meeting, **When** viewing decision, **Then** linked meeting is accessible

---

### User Story 5 - Cross-Cutting Queries (Priority: P2)

As an executive, I need to correlate information across products and time so that I can identify patterns and risks.

**Why this priority**: High-value insight generation - connects products to customers, meetings, and other entities.

**Independent Test**: Can be fully tested by linking meetings/emails to products and querying relationships.

**Acceptance Scenarios**:

1. **Given** meetings are tagged to products, **When** user asks "show me meetings about LKE in last 6 months", **Then** relevant meetings are listed
2. **Given** customer issues are tracked, **When** user asks "customers with LKE performance issues", **Then** relevant customer feedback is returned
3. **Given** MTC project features relate to LKE, **When** user asks "are MTC features on track", **Then** feature status for related products is shown

---

### User Story 6 - Bidirectional Navigation (Priority: P1)

As a user exploring the organization, I need to navigate from people to their teams and products, and from products to their teams and people.

**Why this priority**: Core navigation - understanding organizational structure requires traversing relationships in both directions.

**Independent Test**: Can be fully tested by creating complete relationship chains and verifying navigation paths.

**Acceptance Scenarios**:

1. **Given** person is on multiple teams, **When** viewing person, **Then** all team memberships with products are shown
2. **Given** product has multiple teams, **When** viewing product, **Then** all teams and their members are listed
3. **Given** team works on multiple products, **When** viewing team, **Then** all product associations are shown

---

### Edge Cases

- What happens when a product is renamed? (Historical references preserved)
- How are products handled when sunset/deprecated?
- What happens when a person changes teams but historical assignments matter?
- How to handle temporary team assignments (interim DRI)?
- What if the same team name exists for different products?
- How to handle contested/unclear role assignments?
- What happens when querying products with no team assignments yet?

## Requirements *(mandatory)*

### Functional Requirements

**Products**
- **FR-001**: System MUST support 3-level product hierarchy: Product → Sub-Product → Feature
- **FR-002**: System MUST allow products to have name, description, status (active, beta, sunset, deprecated)
- **FR-003**: System MUST support product aliases for common abbreviations (LK Enterprise → LKE Enterprise)
- **FR-004**: System MUST track product creation and modification timestamps

**Teams & Roles**
- **FR-005**: System MUST support associating teams with products (MTC has Networking Team)
- **FR-006**: System MUST support scoped roles: person has role X in context of product Y + team Z
- **FR-007**: System MUST support team context labels (Core Team, DRI Team, Engineering)
- **FR-008**: System MUST preserve historical role assignments when changes occur

**People**
- **FR-009**: System MUST add country field to person records
- **FR-010**: System MUST support querying people by country + product + role combination
- **FR-011**: System MUST support person → teams → products navigation
- **FR-012**: System MUST support products → teams → people navigation

**Timeline**
- **FR-013**: System MUST record product events (decisions, milestones, risk creation)
- **FR-014**: System MUST link events to meetings/emails when applicable
- **FR-015**: System MUST support querying events by product and date range
- **FR-016**: System MUST capture event metadata (who, what, when, context)

**CLI Interface**
- **FR-017**: System MUST provide `penf product` command group for product management
- **FR-018**: System MUST provide `penf product add` for creating products with hierarchy
- **FR-019**: System MUST provide `penf product team` for managing team associations
- **FR-020**: System MUST provide `penf product info` for viewing product details
- **FR-021**: System MUST provide `penf product timeline` for viewing product events
- **FR-022**: System MUST integrate products with search and AI queries

### Key Entities

- **Product**: Business product with hierarchy (parent_id), status, and metadata
- **ProductTeam**: Association between product and team with context label
- **ProductTeamRole**: Scoped role assignment (person + role + product + team)
- **ProductEvent**: Timeline entry (decision, milestone, risk) with metadata
- **Person.Country**: Geographic location for person (extension to existing entity)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Product hierarchy queries return accurate results within 100ms
- **SC-002**: "Who is the DRI for X on Y" queries resolve correctly 100% of the time when data exists
- **SC-003**: Geographic team queries (person + product + country) work correctly
- **SC-004**: Timeline queries return events in correct chronological order
- **SC-005**: Bidirectional navigation (person ↔ teams ↔ products) works in both directions
- **SC-006**: Product search integrates with existing search infrastructure
- **SC-007**: CLI commands complete with clear output and error handling
- **SC-008**: Historical role assignments preserved when team memberships change

## Dependencies

- Entity resolution system from [003-entity-resolution migration](../migrations/003_entity_resolution.sql) for People/Teams/Projects
- Search service from [007-search-interface](../007-search-interface/spec.md) for product search integration
- Meeting pipeline from [005-meeting-pipeline](../005-meeting-pipeline/spec.md) for timeline meeting links
- AI coordination from [003-ai-coordination](../003-ai-coordination/spec.md) for natural language product queries

## Assumptions

- Products are long-lived entities that rarely change hierarchy after creation
- Team-product associations are relatively stable (changes monthly, not daily)
- Role assignments can change but historical context matters
- Country is sufficient for geographic queries (not city-level granularity)
- Timeline events are manually recorded or linked from meetings/decisions
- Product hierarchy is shallow (3 levels sufficient for v1)
- One person can have multiple roles on the same product through different teams

## Non-Goals (v1)

- Competitor entity management (use documents for now)
- Usage metrics/adoption data (future enhancement)
- Integration with external product management tools (Jira, Productboard)
- Automated product detection from content
- Product financial data (revenue, costs)
