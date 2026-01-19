# Migration Integration Tests

This document explains the migration integration tests for the Penfold storage layer.

## Overview

The migration tests in `test_migrations.py` are designed to comprehensively test the Alembic migration system for Database Schema User Story 1. These tests follow Test-Driven Development (TDD) principles and will initially FAIL until migrations are properly generated.

## Test Categories

### 1. Migration Execution Tests (`TestMigrationExecution`)
- **test_migrations_can_create_all_tables_successfully**: Verifies all tables are created
- **test_required_postgresql_extensions_are_enabled**: Checks vector, ltree, pg_trgm extensions
- **test_hnsw_vector_indexes_are_created_correctly**: Validates HNSW vector indexes
- **test_all_model_indexes_are_created**: Verifies all model-defined indexes exist
- **test_database_schema_matches_model_definitions**: Compares DB schema to SQLAlchemy models
- **test_migrations_can_be_rolled_back_cleanly**: Tests rollback functionality
- **test_migration_handles_tenant_isolation_setup**: Validates tenant_id columns and RLS setup
- **test_migration_creates_proper_foreign_key_constraints**: Checks FK relationships

### 2. Migration Workflow Tests (`TestMigrationWorkflow`)
- **test_fresh_database_migration_workflow**: Tests complete fresh DB to migrated state
- **test_migration_idempotency**: Ensures migrations can run multiple times safely
- **test_migration_generates_correct_alembic_version_tracking**: Validates version tracking

### 3. Development Workflow Tests (`TestMigrationDevelopmentWorkflow`)
- **test_alembic_configuration_is_valid**: Validates Alembic config setup
- **test_alembic_can_detect_model_changes**: Tests autogenerate capability
- **test_database_initialization_without_migrations**: Direct model creation for development

## Running the Tests

### Prerequisites
Install required dependencies:
```bash
pip install pytest pytest-asyncio asyncpg psycopg2-binary pgvector alembic sqlalchemy pydantic-settings
```

### Running Tests
```bash
# Run all migration tests (with slow tests)
python -m pytest tests/integration/test_migrations.py --runslow -v

# Run specific test
python -m pytest tests/integration/test_migrations.py::TestMigrationExecution::test_migrations_can_create_all_tables_successfully --runslow -v

# Run configuration tests only (no database required)
python -m pytest tests/integration/test_migrations.py::TestMigrationDevelopmentWorkflow::test_alembic_configuration_is_valid -v
```

### Expected Behavior

**Before Migrations Are Generated:**
- Most tests will FAIL because no migration files exist yet
- Configuration tests should PASS
- This is expected TDD behavior - tests should fail first

**After Migrations Are Generated:**
- Run `alembic revision --autogenerate -m "Initial schema"`
- All tests should PASS
- Any failures indicate issues with migration generation

## Test Environment Setup

The tests use:
- **Separate test database**: Creates `penfold_test_migrations` database
- **Automatic cleanup**: Drops test database after each test module
- **Transaction isolation**: Each test runs in its own transaction
- **Tenant isolation**: Tests include tenant_id setup

## Key Testing Features

### 1. Extension Verification
Tests verify that PostgreSQL extensions are properly enabled:
- `vector`: For pgvector support
- `ltree`: For hierarchical data (project paths)
- `pg_trgm`: For trigram indexes on text search

### 2. Index Validation
Comprehensive index testing including:
- Standard B-tree indexes
- HNSW vector indexes for embeddings
- GIN indexes for arrays and JSONB
- Unique constraints

### 3. Schema Consistency
Validates that migrated database schema matches SQLAlchemy model definitions:
- Column types and constraints
- Foreign key relationships
- Index definitions
- Table structure

### 4. Migration Safety
Tests migration safety features:
- Rollback capability
- Idempotency (safe to run multiple times)
- Proper version tracking

## Troubleshooting

### Common Issues

**"Module not found: asyncpg"**
```bash
pip install asyncpg
```

**"Extension 'vector' not available"**
- Ensure PostgreSQL has pgvector extension installed
- For macOS: `brew install pgvector`

**"Database connection failed"**
- Ensure PostgreSQL is running
- Check environment variables (DB_HOST, DB_USER, DB_PASSWORD)
- Verify test database permissions

**"Tests pass but no migrations exist"**
- This is expected TDD behavior
- Generate migrations with: `alembic revision --autogenerate`

### Debug Mode

Enable verbose SQL logging:
```bash
PYTEST_VERBOSE_SQL=1 python -m pytest tests/integration/test_migrations.py --runslow -v -s
```

## Integration with Development Workflow

1. **Before implementing migrations**: Run tests to see what fails
2. **Generate migration**: `alembic revision --autogenerate -m "Initial schema"`
3. **Run tests again**: Should now pass if migration is correct
4. **Fix any issues**: Adjust migration files as needed
5. **Continuous validation**: Run tests after any model changes

This TDD approach ensures that:
- Migration requirements are clearly defined upfront
- Generated migrations meet all specifications
- Edge cases and error conditions are handled
- Database schema remains consistent with models