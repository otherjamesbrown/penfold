"""Add search interface tables

Revision ID: 005
Revises: 004
Create Date: 2026-01-16 08:00:00.000000

Creates 4 tables for the search interface feature:
- search_sessions: User search sessions with state management
- search_query_records: Search history with query tracking
- query_suggestions: Autocomplete suggestions
- search_analytics: Aggregated metrics (P3)

Also adds GIN indexes to existing tables for full-text search:
- idx_sources_fulltext ON sources USING GIN(to_tsvector('english', raw_content))
- idx_assertions_fulltext ON assertions USING GIN(to_tsvector('english', content))

Based on specs/007-search-interface/data-model.md
"""

from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT, ARRAY
from sqlalchemy.sql import text


# revision identifiers, used by Alembic.
revision: str = '005'
down_revision: Union[str, None] = '004'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Create search interface tables."""

    # =========================================================================
    # 1. search_sessions - User search sessions
    # =========================================================================
    op.create_table(
        'search_sessions',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True),
        sa.Column('session_uuid', UUID(as_uuid=True), nullable=False, unique=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('user_email', sa.String(254), nullable=False),
        sa.Column('is_active', sa.Boolean(), nullable=False, server_default='true'),
        sa.Column('started_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('last_activity_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('expires_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=text("NOW() + INTERVAL '24 hours'")),
        sa.Column('session_context', JSONB(), nullable=False, server_default='{}'),
        sa.Column('total_queries', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('successful_searches', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "total_queries >= 0",
            name='ck_search_session_total_queries'
        ),
        sa.CheckConstraint(
            "successful_searches >= 0",
            name='ck_search_session_successful_searches'
        ),
        sa.CheckConstraint(
            "successful_searches <= total_queries",
            name='ck_search_session_successful_lte_total'
        ),

        comment="User search sessions with state management and progress tracking"
    )

    # Indexes for search_sessions
    op.create_index('idx_search_session_user', 'search_sessions',
                    ['tenant_id', 'user_email', 'is_active'])
    op.create_index('idx_search_session_activity', 'search_sessions',
                    ['last_activity_at'])
    op.create_index('idx_search_session_uuid', 'search_sessions', ['session_uuid'])
    op.create_index('idx_search_session_active', 'search_sessions',
                    ['is_active', 'expires_at'],
                    postgresql_where=text("is_active = TRUE"))

    # =========================================================================
    # 2. search_query_records - Search history
    # =========================================================================
    op.create_table(
        'search_query_records',
        sa.Column('id', UUID(as_uuid=True), primary_key=True, server_default=text('gen_random_uuid()')),
        sa.Column('session_id', BIGINT(), sa.ForeignKey('search_sessions.id'), nullable=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('query_text', sa.Text(), nullable=False),
        sa.Column('query_hash', sa.String(64), nullable=False),
        sa.Column('normalized_query', sa.Text(), nullable=True),
        sa.Column('query_vector_hash', sa.String(64), nullable=True),
        sa.Column('filters', JSONB(), nullable=False, server_default='{}'),
        sa.Column('temporal_constraint', JSONB(), nullable=True),
        sa.Column('execution_time_ms', sa.Integer(), nullable=True),
        sa.Column('result_count', sa.Integer(), nullable=True),
        sa.Column('cache_hit', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('search_strategy', sa.String(50), nullable=True),
        sa.Column('results_clicked', ARRAY(sa.String()), nullable=True),
        sa.Column('search_successful', sa.Boolean(), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "length(query_text) > 0 AND length(query_text) <= 500",
            name='ck_query_record_text_length'
        ),
        sa.CheckConstraint(
            "length(query_hash) = 64",
            name='ck_query_record_hash_length'
        ),
        sa.CheckConstraint(
            "query_vector_hash IS NULL OR length(query_vector_hash) = 64",
            name='ck_query_record_vector_hash_length'
        ),
        sa.CheckConstraint(
            "execution_time_ms IS NULL OR execution_time_ms >= 0",
            name='ck_query_record_execution_time'
        ),
        sa.CheckConstraint(
            "result_count IS NULL OR result_count >= 0",
            name='ck_query_record_result_count'
        ),
        sa.CheckConstraint(
            "search_strategy IS NULL OR search_strategy IN ('hybrid', 'full_text', 'semantic', 'full_text_fallback', 'no_results')",
            name='ck_query_record_strategy'
        ),

        comment="Persisted search query records for history and analytics"
    )

    # Indexes for search_query_records
    op.create_index('idx_query_record_session', 'search_query_records',
                    ['session_id', 'created_at'])
    op.create_index('idx_query_record_hash', 'search_query_records',
                    ['query_hash'])
    op.create_index('idx_query_record_tenant', 'search_query_records',
                    ['tenant_id', 'created_at'])
    op.create_index('idx_query_record_cache', 'search_query_records',
                    ['tenant_id', 'query_hash', 'cache_hit'])

    # =========================================================================
    # 3. query_suggestions - Autocomplete suggestions
    # =========================================================================
    op.create_table(
        'query_suggestions',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('suggestion_text', sa.String(500), nullable=False),
        sa.Column('suggestion_type', sa.String(50), nullable=True),
        sa.Column('frequency', sa.Integer(), nullable=False, server_default='1'),
        sa.Column('success_rate', sa.Numeric(4, 3), nullable=True),
        sa.Column('last_used_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('context_tags', ARRAY(sa.String()), nullable=True),
        sa.Column('content_types', ARRAY(sa.String()), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),

        # Unique constraint
        sa.UniqueConstraint('tenant_id', 'suggestion_text', name='uc_suggestion_per_tenant'),

        # Constraints
        sa.CheckConstraint(
            "length(suggestion_text) > 0 AND length(suggestion_text) <= 500",
            name='ck_suggestion_text_length'
        ),
        sa.CheckConstraint(
            "suggestion_type IS NULL OR suggestion_type IN ('popular', 'recent', 'contextual')",
            name='ck_suggestion_type'
        ),
        sa.CheckConstraint(
            "frequency >= 0",
            name='ck_suggestion_frequency'
        ),
        sa.CheckConstraint(
            "success_rate IS NULL OR (success_rate >= 0.000 AND success_rate <= 1.000)",
            name='ck_suggestion_success_rate'
        ),

        comment="Cached query suggestions for autocomplete"
    )

    # Indexes for query_suggestions
    op.create_index('idx_suggestion_text', 'query_suggestions',
                    ['tenant_id', 'suggestion_text'])
    op.create_index('idx_suggestion_frequency', 'query_suggestions',
                    ['frequency', 'success_rate'],
                    postgresql_ops={'frequency': 'DESC', 'success_rate': 'DESC'})
    op.create_index('idx_suggestion_type', 'query_suggestions',
                    ['tenant_id', 'suggestion_type'])

    # =========================================================================
    # 4. search_analytics - Aggregated metrics (P3)
    # =========================================================================
    op.create_table(
        'search_analytics',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('bucket_date', sa.DateTime(timezone=True), nullable=False),
        sa.Column('total_queries', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('unique_users', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('avg_execution_time_ms', sa.Float(), nullable=True),
        sa.Column('cache_hit_rate', sa.Numeric(4, 3), nullable=True),
        sa.Column('avg_result_count', sa.Float(), nullable=True),
        sa.Column('zero_result_queries', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('successful_search_rate', sa.Numeric(4, 3), nullable=True),
        sa.Column('content_type_distribution', JSONB(), nullable=True),
        sa.Column('popular_queries', JSONB(), nullable=True),
        sa.Column('p50_execution_time_ms', sa.Integer(), nullable=True),
        sa.Column('p95_execution_time_ms', sa.Integer(), nullable=True),
        sa.Column('p99_execution_time_ms', sa.Integer(), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),

        # Unique constraint
        sa.UniqueConstraint('tenant_id', 'bucket_date', name='uc_analytics_per_day'),

        # Constraints
        sa.CheckConstraint(
            "total_queries >= 0",
            name='ck_analytics_total_queries'
        ),
        sa.CheckConstraint(
            "unique_users >= 0",
            name='ck_analytics_unique_users'
        ),
        sa.CheckConstraint(
            "unique_users <= total_queries OR total_queries = 0",
            name='ck_analytics_users_lte_queries'
        ),
        sa.CheckConstraint(
            "zero_result_queries >= 0 AND zero_result_queries <= total_queries",
            name='ck_analytics_zero_results'
        ),
        sa.CheckConstraint(
            "cache_hit_rate IS NULL OR (cache_hit_rate >= 0.000 AND cache_hit_rate <= 1.000)",
            name='ck_analytics_cache_hit_rate'
        ),
        sa.CheckConstraint(
            "successful_search_rate IS NULL OR (successful_search_rate >= 0.000 AND successful_search_rate <= 1.000)",
            name='ck_analytics_success_rate'
        ),
        sa.CheckConstraint(
            "avg_execution_time_ms IS NULL OR avg_execution_time_ms >= 0",
            name='ck_analytics_avg_exec_time'
        ),
        sa.CheckConstraint(
            "p50_execution_time_ms IS NULL OR p50_execution_time_ms >= 0",
            name='ck_analytics_p50_exec_time'
        ),
        sa.CheckConstraint(
            "p95_execution_time_ms IS NULL OR p95_execution_time_ms >= 0",
            name='ck_analytics_p95_exec_time'
        ),
        sa.CheckConstraint(
            "p99_execution_time_ms IS NULL OR p99_execution_time_ms >= 0",
            name='ck_analytics_p99_exec_time'
        ),

        comment="Aggregated search analytics for P3 analytics feature"
    )

    # Indexes for search_analytics
    op.create_index('idx_analytics_bucket', 'search_analytics',
                    ['tenant_id', 'bucket_date'],
                    postgresql_ops={'bucket_date': 'DESC'})

    # =========================================================================
    # 5. Add GIN indexes to existing tables for full-text search
    # =========================================================================

    # GIN index on sources.raw_content for full-text search
    op.execute("""
        CREATE INDEX idx_sources_fulltext
        ON sources USING GIN(to_tsvector('english', COALESCE(raw_content, '')))
    """)

    # GIN index on assertions.content for full-text search
    op.execute("""
        CREATE INDEX idx_assertions_fulltext
        ON assertions USING GIN(to_tsvector('english', content))
    """)

    # =========================================================================
    # Enable Row-Level Security on new tables
    # =========================================================================
    search_tables = [
        'search_sessions', 'search_query_records',
        'query_suggestions', 'search_analytics'
    ]

    for table_name in search_tables:
        # Enable RLS
        op.execute(f"ALTER TABLE {table_name} ENABLE ROW LEVEL SECURITY")

        # Create policy for tenant isolation
        op.execute(f"""
            CREATE POLICY tenant_isolation_policy ON {table_name}
            FOR ALL
            TO PUBLIC
            USING (
                tenant_id = COALESCE(
                    current_setting('app.current_tenant_id', true)::uuid,
                    '00000000-0000-0000-0000-000000000000'::uuid
                )
            )
        """)

        # Create policy for superuser access
        op.execute(f"""
            CREATE POLICY superuser_access_policy ON {table_name}
            FOR ALL
            TO PUBLIC
            USING (
                current_setting('app.current_tenant_id', true) IS NULL
                OR current_setting('app.current_tenant_id', true) = ''
                OR current_user = 'postgres'
            )
        """)


def downgrade() -> None:
    """Remove search interface tables."""

    search_tables = [
        'search_analytics', 'query_suggestions',
        'search_query_records', 'search_sessions'
    ]

    # Remove RLS policies
    for table_name in search_tables:
        op.execute(f"DROP POLICY IF EXISTS tenant_isolation_policy ON {table_name}")
        op.execute(f"DROP POLICY IF EXISTS superuser_access_policy ON {table_name}")
        op.execute(f"ALTER TABLE {table_name} DISABLE ROW LEVEL SECURITY")

    # Drop GIN indexes from existing tables
    op.execute("DROP INDEX IF EXISTS idx_assertions_fulltext")
    op.execute("DROP INDEX IF EXISTS idx_sources_fulltext")

    # Drop tables in reverse order
    for table_name in search_tables:
        op.drop_table(table_name)
