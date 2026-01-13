# Feature Specification: Database Schema and Multi-Tenant Storage Layer

**Feature Branch**: `001-database-schema`
**Created**: 2026-01-11
**Updated**: 2026-01-13 (Added multi-tenancy architecture)
**Status**: Updated Draft
**Input**: User description: "Database Schema and Storage Layer with Multi-Tenant Support"

## Clarifications

### Session 2026-01-12

- Q: How should database structure be organized for feature separation while maintaining referential integrity? → A: Single database with logical schemas (e.g., core, events, vector) for organizational separation
- Q: How should entity deletion be handled for audit trails and data recovery? → A: Soft deletes with archive tables for audit and recovery
- Q: What messaging system should handle event distribution for the pub-sub framework? → A: Redis for pub-sub with PostgreSQL LISTEN/NOTIFY fallback
- Q: Which vector indexing algorithm should be used for 768-dimensional embeddings? → A: HNSW (Hierarchical Navigable Small World) for high-performance approximate search
- Q: How long should processing results be retained for debugging and quality validation? → A: 30 days processing result retention

### Session 2026-01-13 - Multi-Tenancy Design

- Q: How should the system handle separation between work and personal contexts? → A: Context-based multi-tenancy with 'work', 'personal', and optional 'family' tenants
- Q: Should people exist across tenants (e.g., user appears in both work and personal)? → A: Shared entity resolution - people can exist across tenants while other entities remain isolated
- Q: How should AI learning work across tenant boundaries? → A: Shared learning with privacy - AI improves across all data while maintaining strict tenant data isolation
- Q: What's the preferred interface for tenant switching? → A: Context switching model with persistent tenant selection (penf tenant switch work)
- Q: How should Row-Level Security be implemented? → A: PostgreSQL RLS policies with automatic tenant_id filtering and session-based tenant context

## User Scenarios & Testing *(mandatory)*

### User Story 0 - Multi-Tenant Architecture and Context Management (Priority: P0)

As a user, I need to separate work and personal contexts completely so that my professional data (emails, projects, meetings) is isolated from personal data (family projects, personal emails) while sharing common entities like people who exist in both contexts.

**Why this priority**: Multi-tenancy is foundational - all other features depend on proper tenant isolation. Must be implemented before any real data is stored.

**Independent Test**: Can be fully tested by creating tenants, storing data in different contexts, verifying isolation, and testing cross-tenant entity resolution.

**Acceptance Scenarios**:

1. **Given** tenant management system, **When** work and personal tenants are created, **Then** each tenant has isolated data storage with proper RLS policies
2. **Given** data in multiple tenants, **When** user switches contexts, **Then** only appropriate tenant data is accessible with zero cross-tenant data leakage
3. **Given** a person appearing in both contexts, **When** entity resolution runs, **Then** person is linked across tenants while maintaining context separation for all other data
4. **Given** AI processing on multi-tenant data, **When** models learn patterns, **Then** insights improve across all data while maintaining strict tenant data boundaries

---

### User Story 1 - Schema Definition and Migration (Priority: P1)

As a developer, I need to define the database schema for core entities (Source, Assertion, Person, Project, Team) so that the system has a structured foundation for storing and retrieving information with proper relationships and constraints.

**Why this priority**: This is the foundational requirement - without a properly defined schema, no data operations can occur. All other features depend on this.

**Independent Test**: Can be fully tested by creating schema migration scripts and verifying table creation with proper constraints, relationships, and indexes.

**Acceptance Scenarios**:

1. **Given** schema migration scripts are available, **When** database setup is executed, **Then** all core tables are created with correct structure
2. **Given** core tables exist, **When** test data is inserted, **Then** all constraints and relationships are enforced properly
3. **Given** schema is established, **When** migration rollback is performed, **Then** schema changes are reverted cleanly

---

### User Story 2 - Data Storage Operations (Priority: P1)

As an application component, I need to store and retrieve entity data (sources, assertions, people, projects) with proper validation and transaction support so that data integrity is maintained across all operations.

**Why this priority**: Core storage operations are essential for any functionality - without reliable CRUD operations, the system cannot function.

**Independent Test**: Can be fully tested by performing create, read, update, delete operations on each entity type and verifying data persistence and retrieval accuracy.

**Acceptance Scenarios**:

1. **Given** valid entity data, **When** storage operation is requested, **Then** data is persisted with generated IDs and timestamps
2. **Given** stored entity data, **When** retrieval is requested, **Then** complete and accurate data is returned
3. **Given** concurrent operations, **When** multiple writes occur simultaneously, **Then** data consistency is maintained through proper locking

---

### User Story 3 - Event-Driven Processing Framework (Priority: P2)

As the processing system, I need to store and manage events, processing jobs, and results from multiple AI models so that the pub-sub framework can coordinate local and cloud-based processing workflows with proper state tracking and result aggregation.

**Why this priority**: Enables the flexible multi-model processing architecture that allows hybrid local/cloud AI workflows, but can be implemented after basic storage is working.

**Independent Test**: Can be fully tested by publishing processing events, subscribing multiple processors, and verifying proper job state management and result storage.

**Acceptance Scenarios**:

1. **Given** new content to process, **When** ingestion event is published, **Then** event is stored with proper subscriber tracking
2. **Given** multiple AI processors subscribed to an event, **When** processing completes, **Then** all results are stored with attribution and timestamps
3. **Given** processing jobs in various states, **When** status queries are performed, **Then** accurate job progress and completion status is returned

---

### User Story 4 - Vector Storage for Semantic Search (Priority: P2)

As the search system, I need to store and query high-dimensional embeddings for semantic search capabilities so that users can find relevant information through natural language queries rather than exact keyword matches.

**Why this priority**: Enables the core semantic search functionality that differentiates this system from basic keyword search, but can be implemented after basic storage is working.

**Independent Test**: Can be fully tested by storing embeddings and performing similarity searches with known query vectors to verify accurate nearest-neighbor retrieval.

**Acceptance Scenarios**:

1. **Given** text content with generated embeddings, **When** vector storage is requested, **Then** embeddings are stored with proper indexing for efficient retrieval
2. **Given** stored embeddings, **When** similarity search is performed, **Then** most relevant results are returned in ranked order
3. **Given** large embedding datasets, **When** search queries are executed, **Then** response times remain under acceptable thresholds

---

### User Story 5 - Performance Monitoring and Optimization (Priority: P3)

As a system administrator, I need database performance monitoring and query optimization capabilities so that I can ensure the system scales effectively as data volume grows.

**Why this priority**: Important for production deployment and long-term maintenance, but not required for initial functionality.

**Independent Test**: Can be fully tested by executing performance benchmarks and monitoring query execution plans with various data loads.

**Acceptance Scenarios**:

1. **Given** database operations under load, **When** centralized observability monitoring is active, **Then** query execution metrics are captured via observability framework (see specs/011-observability-framework)
2. **Given** slow-performing queries, **When** optimization analysis is run, **Then** specific improvement recommendations are provided
3. **Given** growing data volumes, **When** performance degrades, **Then** centralized alerting framework notifies administrators of scaling needs

---

### Edge Cases

- What happens when storage volume approaches disk capacity limits?
- How does the system handle corrupted data or schema inconsistencies?
- What occurs during concurrent schema migrations and data operations?
- How are orphaned records handled when parent entities are deleted?
- What happens when vector embedding dimensions change between model versions?

## Requirements *(mandatory)*

### Functional Requirements

#### Multi-Tenancy Requirements

- **FR-001**: System MUST provide multi-tenant architecture with complete data isolation between tenants (work, personal, family contexts)
- **FR-002**: System MUST implement PostgreSQL Row-Level Security (RLS) policies to automatically filter data by tenant_id
- **FR-003**: System MUST support tenant management operations (create, switch, list, configure tenants)
- **FR-004**: System MUST provide cross-tenant entity resolution for people while maintaining isolation for all other entities
- **FR-005**: System MUST track active tenant context in user sessions with persistent tenant selection

#### Storage and Schema Requirements

- **FR-006**: System MUST provide PostgreSQL database with pgvector extension for hybrid relational and vector storage, organized with logical schemas for feature separation (core, events, vector)
- **FR-007**: System MUST define schema for core entities: Source, Assertion, Person, Project, Team with proper relationships and constraints
- **FR-008**: System MUST support ACID transactions for data consistency across related entity operations
- **FR-009**: System MUST provide vector storage and indexing capabilities for embeddings up to 768 dimensions (optimized for nomic-embed-text model)
- **FR-010**: System MUST implement foreign key constraints and cascading rules for data integrity
- **FR-011**: System MUST support database migrations with rollback capabilities for schema evolution
- **FR-012**: System MUST provide indexing strategies for both relational queries and vector similarity search using HNSW algorithm for 768-dimensional embeddings with parameters M=16, ef_construction=200 for optimal performance/memory balance
- **FR-013**: System MUST handle concurrent read/write operations without data corruption
- **FR-014**: System MUST log all schema changes and critical data operations for audit trails, implementing soft deletes with archive tables for entity removal
- **FR-015**: System MUST support backup and restore operations for data protection
- **FR-016**: System MUST enforce data validation rules at the database level for critical fields
- **FR-017**: System MUST provide connection pooling and resource management for efficient database access
- **FR-018**: System MUST support event-driven processing with event storage, subscription management, and job state tracking
- **FR-019**: System MUST store processing results from multiple AI models with attribution, confidence scores, and comparison capabilities, retaining results for 30 days for debugging and quality validation
- **FR-020**: System MUST provide event publishing and subscription mechanisms for pub-sub processing workflows
- **FR-021**: System MUST track processing job states (queued, in_progress, completed, failed) with proper error handling and retry logic
- **FR-022**: System MUST support result aggregation and comparison between different AI processors for quality validation

### Key Entities

#### Multi-Tenancy Entities

- **Tenant**: Context definition and configuration (work, personal, family) with metadata, settings, and RLS policies
- **TenantSession**: Active tenant context tracking for user sessions with timestamp and configuration preferences
- **CrossTenantPersonLink**: Links person entities that represent the same individual across different tenants while maintaining data isolation

#### Core Business Entities

- **Source**: Raw content from external systems with metadata (type, ingestion timestamp, source system identifier, content hash) - tenant-isolated
- **Assertion**: Extracted meaningful information with confidence scores, relationships to sources, and categorization - tenant-isolated
- **Person**: Canonical person records with aliases, contact methods, and organizational relationships - can be linked across tenants
- **Project**: Hierarchical project structure with timeline, artifacts, team membership, and status tracking - tenant-isolated
- **Team**: Organizational units with member relationships, reporting structures, and project associations - tenant-isolated
- **Embedding**: Vector representations linked to content with model version, dimensions, and similarity indexes - tenant-isolated
- **ProcessingEvent**: Published events for content processing with event type, payload, and subscriber tracking - tenant-aware
- **ProcessingJob**: Individual processing tasks with state, processor identity, input/output references, and execution metadata - tenant-aware
- **ProcessingResult**: Output from AI processors with confidence scores, processing time, model version, and result comparison data - tenant-aware
- **Subscription**: Configuration for which processors handle which event types with filtering criteria and processing preferences - tenant-aware

## Success Criteria *(mandatory)*

### Measurable Outcomes

#### Multi-Tenancy Criteria

- **SC-001**: Tenant creation and RLS policy setup completes in under 30 seconds with proper data isolation verification
- **SC-002**: Tenant context switching completes in under 100ms with session persistence across CLI operations
- **SC-003**: Cross-tenant data leakage tests show zero unauthorized data access across 1000+ test scenarios
- **SC-004**: Cross-tenant person linking completes in under 200ms while maintaining strict isolation for all other entities

#### Storage and Performance Criteria

- **SC-005**: Database setup completes successfully in under 5 minutes with all tables, constraints, and indexes properly created
- **SC-006**: Basic CRUD operations on any entity complete in under 100ms for datasets up to 10,000 records per tenant
- **SC-007**: Vector similarity searches return results in under 500ms for embedding collections up to 100,000 vectors per tenant
- **SC-008**: Database maintains 99.9% uptime during normal operations with proper error recovery
- **SC-009**: Schema migrations execute without data loss and complete rollback capability in under 15 minutes
- **SC-010**: Concurrent operations support at least 50 simultaneous connections without performance degradation
- **SC-011**: Database storage efficiency achieves at least 80% utilization of allocated disk space
- **SC-012**: All database constraints prevent invalid data insertion with clear error messages for validation failures
- **SC-013**: Event publishing and subscription operations complete in under 50ms for real-time processing workflows
- **SC-014**: Processing job state transitions are atomic and trackable with 100% accuracy across concurrent operations
- **SC-015**: Multi-model processing results can be compared and aggregated within 200ms for quality validation workflows

## Dependencies

- PostgreSQL 16+ installation and configuration
- pgvector extension availability and setup
- Database administration credentials and access
- Storage infrastructure with adequate capacity and performance characteristics
- Redis message queue system for event distribution with PostgreSQL LISTEN/NOTIFY fallback
- Multi-model AI processing infrastructure (local and cloud processors)

## Assumptions

- PostgreSQL is chosen as the primary database system based on project requirements
- pgvector extension will be used for embedding storage and similarity search
- Standard relational database patterns are sufficient for entity relationships
- Database will initially run on a single instance without distributed architecture requirements
- Backup and disaster recovery procedures will follow standard PostgreSQL practices
- Event-driven processing architecture will start with simple pub-sub and evolve to support complex multi-model workflows
- Processing results from multiple AI models will be stored for comparison and quality validation
- Job state management will handle both synchronous and asynchronous processing patterns