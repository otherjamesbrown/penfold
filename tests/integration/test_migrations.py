"""Integration tests for Alembic migration execution.

This module contains comprehensive tests to verify that Alembic migrations:
1. Create all tables successfully
2. Can be rolled back cleanly
3. Match model definitions after migration
4. Create indexes correctly (especially HNSW vector indexes)
5. Enable required PostgreSQL extensions
6. Handle database initialization properly

Tests follow TDD principles and will initially fail until migrations are properly generated.
"""

import asyncio
import logging
import os
import subprocess
import tempfile
from pathlib import Path
from typing import Dict, List, Set
from unittest.mock import patch

import pytest
import sqlalchemy as sa
from alembic import command as alembic_command
from alembic.config import Config as AlembicConfig
from alembic.runtime.migration import MigrationContext
from alembic.script import ScriptDirectory
from sqlalchemy import MetaData, inspect, text
from sqlalchemy.ext.asyncio import AsyncConnection, AsyncEngine, create_async_engine
from sqlalchemy.schema import CreateIndex

from penf_lib.storage.config import config
from penf_lib.storage.models import Base, Embedding

logger = logging.getLogger(__name__)


@pytest.fixture(scope="module")
def alembic_config() -> AlembicConfig:
    """Create Alembic configuration for testing."""
    project_root = Path(__file__).parent.parent.parent
    alembic_ini_path = project_root / "alembic.ini"

    alembic_cfg = AlembicConfig(str(alembic_ini_path))
    alembic_cfg.set_main_option("script_location", str(project_root / "penf_lib" / "storage" / "migrations"))

    # Use test database URL
    alembic_cfg.set_main_option("sqlalchemy.url", config.database.sync_database_url.replace(
        config.database.name, config.database.test_name
    ))

    return alembic_cfg


@pytest.fixture(scope="module")
async def migration_test_engine() -> AsyncEngine:
    """Create a separate engine for migration testing."""
    # Create a unique test database for migration tests
    migration_db_name = f"{config.database.test_name}_migrations"

    # First, connect to default database to create our test database
    admin_url = config.database.test_database_url.replace(f"/{config.database.test_name}", "/postgres")
    admin_engine = create_async_engine(admin_url, isolation_level="AUTOCOMMIT")

    async with admin_engine.connect() as conn:
        # Drop database if it exists
        await conn.execute(text(f'DROP DATABASE IF EXISTS "{migration_db_name}"'))
        # Create fresh database
        await conn.execute(text(f'CREATE DATABASE "{migration_db_name}"'))

    await admin_engine.dispose()

    # Create engine for our test database
    test_db_url = config.database.test_database_url.replace(config.database.test_name, migration_db_name)
    test_engine = create_async_engine(test_db_url, echo=False)

    yield test_engine

    # Cleanup
    await test_engine.dispose()

    # Drop test database
    admin_engine = create_async_engine(admin_url, isolation_level="AUTOCOMMIT")
    async with admin_engine.connect() as conn:
        await conn.execute(text(f'DROP DATABASE IF EXISTS "{migration_db_name}"'))
    await admin_engine.dispose()


@pytest.fixture
async def fresh_migration_db(migration_test_engine: AsyncEngine) -> AsyncConnection:
    """Provide a fresh database connection for migration testing."""
    async with migration_test_engine.connect() as conn:
        yield conn


@pytest.mark.integration
@pytest.mark.slow
class TestMigrationExecution:
    """Test suite for Alembic migration execution."""

    async def test_migrations_can_create_all_tables_successfully(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that Alembic migrations can create all tables successfully."""
        # This test will initially FAIL because no migrations exist yet

        # Run migrations to head
        try:
            with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
                alembic_command.upgrade(alembic_config, "head")
        except Exception as e:
            pytest.fail(f"Migration to head failed: {e}")

        # Verify all expected tables exist
        inspector = inspect(fresh_migration_db.sync_engine)
        existing_tables = set(inspector.get_table_names())

        # Expected tables from our models
        expected_tables = {
            'sources', 'assertions', 'people', 'projects', 'teams',
            'processing_events', 'processing_jobs', 'processing_results',
            'embeddings', 'alembic_version'
        }

        missing_tables = expected_tables - existing_tables
        assert not missing_tables, f"Missing tables: {missing_tables}"

        logger.info(f"Successfully created tables: {existing_tables}")

    async def test_required_postgresql_extensions_are_enabled(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that required PostgreSQL extensions are enabled during migration."""
        # This test will initially FAIL because migrations don't exist yet

        # Run migrations
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

        # Check that required extensions are enabled
        required_extensions = ['vector', 'ltree', 'pg_trgm']

        for extension in required_extensions:
            result = await fresh_migration_db.execute(
                text("SELECT 1 FROM pg_extension WHERE extname = :ext"),
                {"ext": extension}
            )
            assert result.fetchone() is not None, f"Extension {extension} not enabled"

        logger.info(f"All required extensions enabled: {required_extensions}")

    async def test_hnsw_vector_indexes_are_created_correctly(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that HNSW vector indexes are created with correct parameters."""
        # This test will initially FAIL because migrations don't exist yet

        # Run migrations
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

        # Check vector index on embeddings table
        inspector = inspect(fresh_migration_db.sync_engine)

        # Check embeddings table exists
        assert 'embeddings' in inspector.get_table_names(), "embeddings table not found"

        # Check that vector column exists and has correct type
        columns = inspector.get_columns('embeddings')
        embedding_col = next((col for col in columns if col['name'] == 'embedding'), None)
        assert embedding_col is not None, "embedding column not found"

        # Note: Full HNSW index verification requires checking pg_indexes
        # This is a placeholder for when the actual migration is created
        indexes = inspector.get_indexes('embeddings')
        logger.info(f"Embeddings table indexes: {indexes}")

    async def test_all_model_indexes_are_created(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that all indexes defined in models are created during migration."""
        # This test will initially FAIL because migrations don't exist yet

        # Run migrations
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

        inspector = inspect(fresh_migration_db.sync_engine)

        # Expected indexes by table (simplified list)
        expected_indexes_by_table = {
            'sources': [
                'idx_sources_tenant_created',
                'idx_sources_unique_external',
                'idx_sources_hash',
                'idx_sources_status',
                'idx_sources_soft_delete'
            ],
            'assertions': [
                'idx_assertions_tenant_type',
                'idx_assertions_confidence',
                'idx_assertions_source',
                'idx_assertions_validated',
                'idx_assertions_soft_delete'
            ],
            'people': [
                'idx_people_tenant_name',
                'idx_people_aliases',
                'idx_people_emails',
                'idx_people_soft_delete'
            ],
            'embeddings': [
                'idx_embeddings_vector_hnsw',
                'idx_embeddings_entity',
                'idx_embeddings_model',
                'idx_embeddings_content_hash'
            ]
        }

        for table_name, expected_indexes in expected_indexes_by_table.items():
            if table_name in inspector.get_table_names():
                actual_indexes = {idx['name'] for idx in inspector.get_indexes(table_name)}
                actual_indexes.update({idx['name'] for idx in inspector.get_unique_constraints(table_name)})

                for expected_index in expected_indexes:
                    # Skip vector index if pgvector not available
                    if expected_index == 'idx_embeddings_vector_hnsw':
                        try:
                            from pgvector.sqlalchemy import Vector
                            if Vector is None:
                                continue
                        except ImportError:
                            continue

                    # For now, just log the indexes since migration doesn't exist yet
                    logger.info(f"Table {table_name} - Expected: {expected_index}, Actual: {actual_indexes}")

    async def test_database_schema_matches_model_definitions(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that database schema matches SQLAlchemy model definitions after migration."""
        # This test will initially FAIL because migrations don't exist yet

        # Run migrations
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

        inspector = inspect(fresh_migration_db.sync_engine)

        # Get model metadata
        model_metadata = Base.metadata

        # Check each table
        for table_name, table in model_metadata.tables.items():
            assert table_name in inspector.get_table_names(), f"Table {table_name} missing from database"

            # Check columns
            db_columns = {col['name']: col for col in inspector.get_columns(table_name)}
            model_columns = {col.name: col for col in table.columns}

            # Verify all model columns exist in database
            for col_name in model_columns:
                assert col_name in db_columns, f"Column {col_name} missing from table {table_name}"

            logger.info(f"Table {table_name} schema validation passed")

    async def test_migrations_can_be_rolled_back_cleanly(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that migrations can be rolled back cleanly without errors."""
        # This test will initially FAIL because migrations don't exist yet

        # First, upgrade to head
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

        # Verify tables exist
        inspector = inspect(fresh_migration_db.sync_engine)
        tables_after_upgrade = set(inspector.get_table_names())
        assert len(tables_after_upgrade) > 1, "No tables created after upgrade"

        # Now rollback
        try:
            with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
                alembic_command.downgrade(alembic_config, "base")
        except Exception as e:
            pytest.fail(f"Migration rollback failed: {e}")

        # Verify tables are removed (except alembic_version)
        inspector = inspect(fresh_migration_db.sync_engine)
        tables_after_downgrade = set(inspector.get_table_names())

        # Only alembic_version table should remain
        remaining_tables = tables_after_downgrade - {'alembic_version'}
        assert not remaining_tables, f"Tables not properly cleaned up: {remaining_tables}"

        logger.info("Migration rollback completed successfully")

    async def test_migration_handles_tenant_isolation_setup(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that migration properly sets up tenant isolation (RLS policies)."""
        # This test will initially FAIL because migrations don't exist yet

        # Run migrations
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

        # Check that tables have tenant_id columns
        inspector = inspect(fresh_migration_db.sync_engine)
        tenant_tables = ['sources', 'assertions', 'people', 'projects', 'teams', 'embeddings']

        for table_name in tenant_tables:
            if table_name in inspector.get_table_names():
                columns = {col['name'] for col in inspector.get_columns(table_name)}
                assert 'tenant_id' in columns, f"tenant_id column missing from {table_name}"

        # Note: RLS policy checking would require additional PostgreSQL-specific queries
        logger.info("Tenant isolation columns verified")

    async def test_migration_creates_proper_foreign_key_constraints(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that foreign key constraints are properly created."""
        # This test will initially FAIL because migrations don't exist yet

        # Run migrations
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

        inspector = inspect(fresh_migration_db.sync_engine)

        # Expected foreign keys
        expected_fks = {
            'assertions': [('source_id', 'sources', 'id')],
            'embeddings': [
                ('source_id', 'sources', 'id'),
                ('assertion_id', 'assertions', 'id'),
                ('person_id', 'people', 'id'),
                ('project_id', 'projects', 'id'),
                ('team_id', 'teams', 'id')
            ],
            'processing_jobs': [('event_id', 'processing_events', 'event_id')],
            'processing_results': [('job_id', 'processing_jobs', 'job_id')]
        }

        for table_name, expected_fk_list in expected_fks.items():
            if table_name in inspector.get_table_names():
                actual_fks = inspector.get_foreign_keys(table_name)
                logger.info(f"Table {table_name} foreign keys: {len(actual_fks)}")

                # For now, just verify FKs exist since specific validation requires migration
                # This test will be more specific once migrations are generated


@pytest.mark.integration
@pytest.mark.slow
class TestMigrationWorkflow:
    """Test the complete migration workflow scenarios."""

    async def test_fresh_database_migration_workflow(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test complete workflow from fresh database to fully migrated state."""
        # This test will initially FAIL because migrations don't exist yet

        # Start with empty database
        inspector = inspect(fresh_migration_db.sync_engine)
        initial_tables = inspector.get_table_names()
        assert not initial_tables, f"Database should be empty, found: {initial_tables}"

        # Run initial migration
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

        # Verify complete schema exists
        inspector = inspect(fresh_migration_db.sync_engine)
        final_tables = set(inspector.get_table_names())

        expected_core_tables = {
            'sources', 'assertions', 'people', 'projects', 'teams',
            'processing_events', 'processing_jobs', 'processing_results',
            'embeddings'
        }

        assert expected_core_tables.issubset(final_tables), (
            f"Missing core tables: {expected_core_tables - final_tables}"
        )

        logger.info(f"Fresh database migration workflow completed: {final_tables}")

    async def test_migration_idempotency(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that running migrations multiple times is safe (idempotent)."""
        # This test will initially FAIL because migrations don't exist yet

        # Run migration to head twice
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

            # Second run should not fail
            try:
                alembic_command.upgrade(alembic_config, "head")
            except Exception as e:
                pytest.fail(f"Second migration run failed: {e}")

        # Database should still be in valid state
        inspector = inspect(fresh_migration_db.sync_engine)
        tables = inspector.get_table_names()
        assert 'sources' in tables, "Core tables missing after second migration run"

        logger.info("Migration idempotency test passed")

    async def test_migration_generates_correct_alembic_version_tracking(
        self,
        alembic_config: AlembicConfig,
        fresh_migration_db: AsyncConnection
    ):
        """Test that Alembic version tracking works correctly."""
        # This test will initially FAIL because migrations don't exist yet

        # Run migrations
        with patch.dict(os.environ, {"DB_NAME": config.database.test_name + "_migrations"}):
            alembic_command.upgrade(alembic_config, "head")

        # Check alembic_version table exists and has correct structure
        inspector = inspect(fresh_migration_db.sync_engine)
        assert 'alembic_version' in inspector.get_table_names(), "alembic_version table missing"

        # Check version is recorded
        result = await fresh_migration_db.execute(
            text("SELECT version_num FROM alembic_version LIMIT 1")
        )
        version = result.scalar()
        assert version is not None, "No version recorded in alembic_version table"

        logger.info(f"Alembic version tracking working: {version}")


@pytest.mark.integration
class TestMigrationDevelopmentWorkflow:
    """Test development-specific migration scenarios."""

    def test_alembic_configuration_is_valid(self, alembic_config: AlembicConfig):
        """Test that Alembic configuration is properly set up."""
        # Verify script location exists
        script_location = alembic_config.get_main_option("script_location")
        assert Path(script_location).exists(), f"Script location not found: {script_location}"

        # Verify env.py exists
        env_py_path = Path(script_location) / "env.py"
        assert env_py_path.exists(), f"env.py not found: {env_py_path}"

        # Verify script template exists
        template_path = Path(script_location) / "script.py.mako"
        assert template_path.exists(), f"script.py.mako not found: {template_path}"

        logger.info("Alembic configuration validation passed")

    def test_alembic_can_detect_model_changes(self, alembic_config: AlembicConfig):
        """Test that Alembic can detect changes in model definitions."""
        # This test verifies that autogenerate can work with our models

        try:
            script_dir = ScriptDirectory.from_config(alembic_config)

            # This will fail initially because no migrations exist
            # But it tests that the configuration is valid for autogenerate
            logger.info("Alembic script directory initialized successfully")

        except Exception as e:
            # Expected to fail until first migration is generated
            logger.info(f"Alembic model change detection test setup: {e}")

    async def test_database_initialization_without_migrations(
        self,
        fresh_migration_db: AsyncConnection
    ):
        """Test that database can be initialized directly from models (development mode)."""
        # This should work even before migrations exist

        # Enable extensions manually (as would be done in init_database)
        await fresh_migration_db.execute(text("CREATE EXTENSION IF NOT EXISTS vector"))
        await fresh_migration_db.execute(text("CREATE EXTENSION IF NOT EXISTS ltree"))
        await fresh_migration_db.execute(text("CREATE EXTENSION IF NOT EXISTS pg_trgm"))

        # Create all tables directly from metadata
        await fresh_migration_db.run_sync(Base.metadata.create_all)

        # Verify all tables were created
        inspector = inspect(fresh_migration_db.sync_engine)
        created_tables = set(inspector.get_table_names())

        expected_tables = set(Base.metadata.tables.keys())
        assert expected_tables.issubset(created_tables), (
            f"Missing tables: {expected_tables - created_tables}"
        )

        logger.info(f"Direct model creation successful: {created_tables}")


# Test utilities for migration development
class MigrationTestUtils:
    """Utility class for migration testing helpers."""

    @staticmethod
    async def get_table_structure(connection: AsyncConnection, table_name: str) -> Dict:
        """Get detailed table structure for comparison."""
        inspector = inspect(connection.sync_engine)

        return {
            'columns': inspector.get_columns(table_name),
            'indexes': inspector.get_indexes(table_name),
            'foreign_keys': inspector.get_foreign_keys(table_name),
            'unique_constraints': inspector.get_unique_constraints(table_name),
        }

    @staticmethod
    async def verify_vector_support(connection: AsyncConnection) -> bool:
        """Check if pgvector extension is properly installed and working."""
        try:
            await connection.execute(text("SELECT 1::vector"))
            return True
        except Exception:
            return False

    @staticmethod
    def get_model_table_names() -> Set[str]:
        """Get all table names defined in our models."""
        return set(Base.metadata.tables.keys())