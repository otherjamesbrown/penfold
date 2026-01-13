"""Initial schema with core entities

Revision ID: 001
Revises:
Create Date: 2026-01-12 23:00:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT, ARRAY
from sqlalchemy.sql import text


# revision identifiers, used by Alembic.
revision: str = '001'
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade database schema."""

    # Enable required PostgreSQL extensions
    op.execute("CREATE EXTENSION IF NOT EXISTS vector")
    op.execute("CREATE EXTENSION IF NOT EXISTS ltree")
    op.execute("CREATE EXTENSION IF NOT EXISTS pg_trgm")

    # Create sources table
    op.create_table(
        'sources',
        sa.Column('id', BIGINT(), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('source_system', sa.String(length=50), nullable=False),
        sa.Column('external_id', sa.String(length=255), nullable=False),
        sa.Column('content_hash', sa.String(length=64), nullable=False),
        sa.Column('raw_content', sa.Text(), nullable=True),
        sa.Column('content_type', sa.String(length=100), nullable=True),
        sa.Column('content_size', sa.Integer(), nullable=True),
        sa.Column('ingestion_metadata', JSONB(), nullable=True),
        sa.Column('processing_status', sa.String(length=20), nullable=False, server_default='pending'),
        sa.Column('source_timestamp', sa.DateTime(timezone=True), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('deleted_by', sa.String(length=100), nullable=True),
        sa.Column('deletion_reason', sa.String(length=500), nullable=True),
        sa.PrimaryKeyConstraint('id'),
        sa.CheckConstraint("length(source_system) > 0", name="ck_sources_source_system_not_empty"),
        sa.CheckConstraint("length(external_id) > 0", name="ck_sources_external_id_not_empty"),
        sa.CheckConstraint("length(content_hash) = 64", name="ck_sources_content_hash_length"),
        sa.CheckConstraint("content_hash ~ '^[a-fA-F0-9]{64}$'", name="ck_sources_content_hash_hex"),
        sa.CheckConstraint("content_size IS NULL OR content_size >= 0", name="ck_sources_content_size_non_negative"),
        sa.CheckConstraint("processing_status IN ('pending', 'processing', 'completed', 'failed', 'retrying')", name="ck_sources_processing_status_valid"),
        sa.CheckConstraint("source_system IN ('gmail', 'slack', 'zoom', 'calendar', 'drive', 'confluence', 'jira', 'teams')", name="ck_sources_source_system_valid"),
        sa.UniqueConstraint('tenant_id', 'source_system', 'external_id', name='idx_sources_unique_external')
    )

    # Create assertions table
    op.create_table(
        'assertions',
        sa.Column('id', BIGINT(), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('source_id', BIGINT(), nullable=False),
        sa.Column('assertion_type', sa.String(length=50), nullable=False),
        sa.Column('content', sa.Text(), nullable=False),
        sa.Column('context', sa.Text(), nullable=True),
        sa.Column('confidence_score', sa.DECIMAL(precision=4, scale=3), nullable=True),
        sa.Column('extraction_model', sa.String(length=100), nullable=True),
        sa.Column('processing_metadata', JSONB(), nullable=True),
        sa.Column('related_entities', JSONB(), nullable=True),
        sa.Column('tags', ARRAY(sa.String()), nullable=True),
        sa.Column('user_validated', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('validation_feedback', JSONB(), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('deleted_by', sa.String(length=100), nullable=True),
        sa.Column('deletion_reason', sa.String(length=500), nullable=True),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['source_id'], ['sources.id']),
        sa.CheckConstraint("length(assertion_type) > 0", name="ck_assertions_type_not_empty"),
        sa.CheckConstraint("length(content) > 0", name="ck_assertions_content_not_empty"),
        sa.CheckConstraint("length(content) <= 10000", name="ck_assertions_content_max_length"),
        sa.CheckConstraint("confidence_score IS NULL OR (confidence_score >= 0.000 AND confidence_score <= 1.000)", name="ck_assertions_confidence_range"),
        sa.CheckConstraint("assertion_type IN ('decision', 'risk', 'commitment', 'milestone', 'outcome', 'dependency', 'assumption', 'issue')", name="ck_assertions_type_valid")
    )

    # Create people table
    op.create_table(
        'people',
        sa.Column('id', BIGINT(), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('canonical_name', sa.String(length=200), nullable=False),
        sa.Column('aliases', ARRAY(sa.String()), nullable=True),
        sa.Column('email_addresses', ARRAY(sa.String()), nullable=True),
        sa.Column('phone_numbers', ARRAY(sa.String()), nullable=True),
        sa.Column('job_title', sa.String(length=200), nullable=True),
        sa.Column('company', sa.String(length=200), nullable=True),
        sa.Column('department', sa.String(length=200), nullable=True),
        sa.Column('entity_resolution_metadata', JSONB(), nullable=True),
        sa.Column('confidence_score', sa.DECIMAL(precision=4, scale=3), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('deleted_by', sa.String(length=100), nullable=True),
        sa.Column('deletion_reason', sa.String(length=500), nullable=True),
        sa.PrimaryKeyConstraint('id'),
        sa.CheckConstraint("length(canonical_name) > 0", name="ck_people_name_not_empty"),
        sa.CheckConstraint("length(canonical_name) <= 200", name="ck_people_name_max_length"),
        sa.CheckConstraint("confidence_score IS NULL OR (confidence_score >= 0.000 AND confidence_score <= 1.000)", name="ck_people_confidence_range"),
        sa.CheckConstraint("job_title IS NULL OR length(job_title) <= 200", name="ck_people_job_title_max_length"),
        sa.CheckConstraint("company IS NULL OR length(company) <= 200", name="ck_people_company_max_length"),
        sa.CheckConstraint("department IS NULL OR length(department) <= 200", name="ck_people_department_max_length")
    )

    # Create projects table
    op.create_table(
        'projects',
        sa.Column('id', BIGINT(), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('name', sa.String(length=200), nullable=False),
        sa.Column('description', sa.Text(), nullable=True),
        sa.Column('status', sa.String(length=50), nullable=False, server_default='active'),
        sa.Column('path', sa.String(length=500), nullable=True),
        sa.Column('parent_id', BIGINT(), nullable=True),
        sa.Column('start_date', sa.DateTime(timezone=True), nullable=True),
        sa.Column('end_date', sa.DateTime(timezone=True), nullable=True),
        sa.Column('target_completion', sa.DateTime(timezone=True), nullable=True),
        sa.Column('project_type', sa.String(length=100), nullable=True),
        sa.Column('priority', sa.String(length=20), nullable=True),
        sa.Column('budget_allocated', sa.DECIMAL(precision=12, scale=2), nullable=True),
        sa.Column('budget_spent', sa.DECIMAL(precision=12, scale=2), nullable=True),
        sa.Column('artifacts', JSONB(), nullable=True),
        sa.Column('milestones', JSONB(), nullable=True),
        sa.Column('team_members', ARRAY(BIGINT()), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('deleted_by', sa.String(length=100), nullable=True),
        sa.Column('deletion_reason', sa.String(length=500), nullable=True),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['parent_id'], ['projects.id']),
        sa.CheckConstraint("length(name) > 0", name="ck_projects_name_not_empty"),
        sa.CheckConstraint("length(name) <= 200", name="ck_projects_name_max_length"),
        sa.CheckConstraint("status IN ('planning', 'active', 'on_hold', 'completed', 'cancelled', 'archived')", name="ck_projects_status_valid"),
        sa.CheckConstraint("priority IS NULL OR priority IN ('critical', 'high', 'medium', 'low')", name="ck_projects_priority_valid"),
        sa.CheckConstraint("budget_allocated IS NULL OR budget_allocated >= 0", name="ck_projects_budget_allocated_non_negative"),
        sa.CheckConstraint("budget_spent IS NULL OR budget_spent >= 0", name="ck_projects_budget_spent_non_negative"),
        sa.CheckConstraint("end_date IS NULL OR start_date IS NULL OR end_date >= start_date", name="ck_projects_end_after_start"),
        sa.CheckConstraint("parent_id IS NULL OR parent_id != id", name="ck_projects_no_self_reference")
    )

    # Create teams table
    op.create_table(
        'teams',
        sa.Column('id', BIGINT(), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('name', sa.String(length=200), nullable=False),
        sa.Column('description', sa.Text(), nullable=True),
        sa.Column('team_type', sa.String(length=100), nullable=True),
        sa.Column('parent_team_id', BIGINT(), nullable=True),
        sa.Column('lead_person_id', BIGINT(), nullable=True),
        sa.Column('member_person_ids', ARRAY(BIGINT()), nullable=True),
        sa.Column('associated_project_ids', ARRAY(BIGINT()), nullable=True),
        sa.Column('location', sa.String(length=200), nullable=True),
        sa.Column('contact_info', JSONB(), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('deleted_by', sa.String(length=100), nullable=True),
        sa.Column('deletion_reason', sa.String(length=500), nullable=True),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['parent_team_id'], ['teams.id']),
        sa.CheckConstraint("length(name) > 0", name="ck_teams_name_not_empty"),
        sa.CheckConstraint("length(name) <= 200", name="ck_teams_name_max_length"),
        sa.CheckConstraint("team_type IS NULL OR team_type IN ('department', 'project_team', 'working_group', 'committee')", name="ck_teams_type_valid"),
        sa.CheckConstraint("location IS NULL OR length(location) <= 200", name="ck_teams_location_max_length"),
        sa.CheckConstraint("lead_person_id IS NULL OR lead_person_id > 0", name="ck_teams_lead_id_positive"),
        sa.CheckConstraint("parent_team_id IS NULL OR parent_team_id != id", name="ck_teams_no_self_reference")
    )

    # Create processing_events table
    op.create_table(
        'processing_events',
        sa.Column('id', BIGINT(), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('event_id', UUID(as_uuid=True), nullable=False),
        sa.Column('event_type', sa.String(length=100), nullable=False),
        sa.Column('payload', JSONB(), nullable=False),
        sa.Column('payload_size', sa.Integer(), nullable=True),
        sa.Column('publisher', sa.String(length=100), nullable=True),
        sa.Column('subscriber_count', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('processing_status', sa.String(length=20), nullable=False, server_default='published'),
        sa.Column('published_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('expires_at', sa.DateTime(timezone=True), server_default=text("NOW() + INTERVAL '30 days'"), nullable=False),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('event_id')
    )

    # Create processing_jobs table
    op.create_table(
        'processing_jobs',
        sa.Column('id', BIGINT(), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('job_id', UUID(as_uuid=True), nullable=False),
        sa.Column('event_id', UUID(as_uuid=True), nullable=False),
        sa.Column('processor_name', sa.String(length=100), nullable=False),
        sa.Column('job_type', sa.String(length=100), nullable=False),
        sa.Column('status', sa.String(length=20), nullable=False, server_default='queued'),
        sa.Column('priority', sa.Integer(), nullable=False, server_default='100'),
        sa.Column('input_data', JSONB(), nullable=True),
        sa.Column('processor_metadata', JSONB(), nullable=True),
        sa.Column('started_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('completed_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('retry_count', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('max_retries', sa.Integer(), nullable=False, server_default='3'),
        sa.Column('error_message', sa.Text(), nullable=True),
        sa.Column('error_details', JSONB(), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['event_id'], ['processing_events.event_id']),
        sa.UniqueConstraint('job_id')
    )

    # Create processing_results table
    op.create_table(
        'processing_results',
        sa.Column('id', BIGINT(), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('job_id', UUID(as_uuid=True), nullable=False),
        sa.Column('result_type', sa.String(length=100), nullable=False),
        sa.Column('result_data', JSONB(), nullable=False),
        sa.Column('confidence_score', sa.DECIMAL(precision=4, scale=3), nullable=True),
        sa.Column('model_name', sa.String(length=100), nullable=True),
        sa.Column('model_version', sa.String(length=50), nullable=True),
        sa.Column('processor_version', sa.String(length=50), nullable=True),
        sa.Column('processing_time_ms', sa.Integer(), nullable=True),
        sa.Column('quality_score', sa.DECIMAL(precision=4, scale=3), nullable=True),
        sa.Column('compared_against', ARRAY(UUID(as_uuid=True)), nullable=True),
        sa.Column('validation_status', sa.String(length=20), nullable=True),
        sa.Column('human_feedback', JSONB(), nullable=True),
        sa.Column('expires_at', sa.DateTime(timezone=True), server_default=text("NOW() + INTERVAL '30 days'"), nullable=False),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['job_id'], ['processing_jobs.job_id'])
    )

    # Create embeddings table
    op.create_table(
        'embeddings',
        sa.Column('id', BIGINT(), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('entity_type', sa.String(length=50), nullable=False),
        sa.Column('entity_id', BIGINT(), nullable=False),
        sa.Column('source_id', BIGINT(), nullable=True),
        sa.Column('assertion_id', BIGINT(), nullable=True),
        sa.Column('person_id', BIGINT(), nullable=True),
        sa.Column('project_id', BIGINT(), nullable=True),
        sa.Column('team_id', BIGINT(), nullable=True),
        # Note: Vector column will be added if pgvector is available
        sa.Column('embedding_model', sa.String(length=100), nullable=False),
        sa.Column('model_version', sa.String(length=50), nullable=True),
        sa.Column('text_content', sa.Text(), nullable=True),
        sa.Column('content_hash', sa.String(length=64), nullable=True),
        sa.Column('generation_confidence', sa.DECIMAL(precision=4, scale=3), nullable=True),
        sa.Column('search_count', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('last_searched_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['source_id'], ['sources.id']),
        sa.ForeignKeyConstraint(['assertion_id'], ['assertions.id']),
        sa.ForeignKeyConstraint(['person_id'], ['people.id']),
        sa.ForeignKeyConstraint(['project_id'], ['projects.id']),
        sa.ForeignKeyConstraint(['team_id'], ['teams.id'])
    )

    # Try to add vector column if pgvector is available
    try:
        # Check if vector extension is available
        result = op.get_bind().execute(text("SELECT 1 FROM pg_available_extensions WHERE name = 'vector' AND installed_version IS NOT NULL"))
        if result.fetchone():
            # Vector extension is available, add the vector column
            op.add_column('embeddings', sa.Column('embedding', sa.Text(), nullable=False, comment='768-dimensional vector (stored as vector type if pgvector available)'))
    except Exception:
        # Fallback: use ARRAY(REAL) if vector extension not available
        op.add_column('embeddings', sa.Column('embedding', ARRAY(sa.Float()), nullable=False, comment='768-dimensional vector array'))

    # Create indexes
    # Sources indexes
    op.create_index('idx_sources_tenant_created', 'sources', ['tenant_id', 'created_at'])
    op.create_index('idx_sources_hash', 'sources', ['content_hash'])
    op.create_index('idx_sources_status', 'sources', ['processing_status'])
    op.create_index('idx_sources_soft_delete', 'sources', ['is_deleted', 'deleted_at'])
    op.create_index('idx_sources_deleted_by', 'sources', ['deleted_by'])

    # Assertions indexes
    op.create_index('idx_assertions_tenant_type', 'assertions', ['tenant_id', 'assertion_type', 'created_at'])
    op.create_index('idx_assertions_confidence', 'assertions', ['confidence_score'])
    op.create_index('idx_assertions_source', 'assertions', ['source_id', 'created_at'])
    op.create_index('idx_assertions_validated', 'assertions', ['user_validated'])
    op.create_index('idx_assertions_soft_delete', 'assertions', ['is_deleted', 'deleted_at'])
    op.create_index('idx_assertions_deleted_by', 'assertions', ['deleted_by'])
    op.create_index('idx_assertions_tags', 'assertions', ['tags'], postgresql_using='gin')

    # People indexes
    op.create_index('idx_people_tenant_name', 'people', ['tenant_id', 'canonical_name'])
    op.create_index('idx_people_aliases', 'people', ['aliases'], postgresql_using='gin')
    op.create_index('idx_people_emails', 'people', ['email_addresses'], postgresql_using='gin')
    op.create_index('idx_people_phones', 'people', ['phone_numbers'], postgresql_using='gin')
    op.create_index('idx_people_company', 'people', ['company'])
    op.create_index('idx_people_soft_delete', 'people', ['is_deleted', 'deleted_at'])
    op.create_index('idx_people_deleted_by', 'people', ['deleted_by'])

    # Projects indexes
    op.create_index('idx_projects_tenant_name', 'projects', ['tenant_id', 'name'])
    op.create_index('idx_projects_status', 'projects', ['status'])
    op.create_index('idx_projects_hierarchy', 'projects', ['path'], postgresql_using='gist')
    op.create_index('idx_projects_timeline', 'projects', ['start_date', 'end_date'])
    op.create_index('idx_projects_priority', 'projects', ['priority'])
    op.create_index('idx_projects_budget', 'projects', ['budget_allocated', 'budget_spent'])
    op.create_index('idx_projects_soft_delete', 'projects', ['is_deleted', 'deleted_at'])
    op.create_index('idx_projects_deleted_by', 'projects', ['deleted_by'])

    # Teams indexes
    op.create_index('idx_teams_tenant_name', 'teams', ['tenant_id', 'name'])
    op.create_index('idx_teams_type', 'teams', ['team_type'])
    op.create_index('idx_teams_lead', 'teams', ['lead_person_id'])
    op.create_index('idx_teams_members', 'teams', ['member_person_ids'], postgresql_using='gin')
    op.create_index('idx_teams_projects', 'teams', ['associated_project_ids'], postgresql_using='gin')
    op.create_index('idx_teams_location', 'teams', ['location'])
    op.create_index('idx_teams_soft_delete', 'teams', ['is_deleted', 'deleted_at'])
    op.create_index('idx_teams_deleted_by', 'teams', ['deleted_by'])

    # Processing events indexes
    op.create_index('idx_events_tenant_type', 'processing_events', ['tenant_id', 'event_type', 'published_at'])
    op.create_index('idx_events_expiry', 'processing_events', ['expires_at'])
    op.create_index('idx_events_status', 'processing_events', ['processing_status'])

    # Processing jobs indexes
    op.create_index('idx_jobs_tenant_status', 'processing_jobs', ['tenant_id', 'status', 'created_at'])
    op.create_index('idx_jobs_processor', 'processing_jobs', ['processor_name', 'status'])
    op.create_index('idx_jobs_priority', 'processing_jobs', ['priority', 'created_at'])
    op.create_index('idx_jobs_retry', 'processing_jobs', ['status', 'retry_count'])

    # Processing results indexes
    op.create_index('idx_results_tenant_type', 'processing_results', ['tenant_id', 'result_type', 'created_at'])
    op.create_index('idx_results_job', 'processing_results', ['job_id'])
    op.create_index('idx_results_model', 'processing_results', ['model_name', 'model_version'])
    op.create_index('idx_results_expiry', 'processing_results', ['expires_at'])
    op.create_index('idx_results_validation', 'processing_results', ['validation_status'])

    # Embeddings indexes
    op.create_index('idx_embeddings_entity', 'embeddings', ['tenant_id', 'entity_type', 'entity_id'])
    op.create_index('idx_embeddings_model', 'embeddings', ['embedding_model', 'model_version'])
    op.create_index('idx_embeddings_content_hash', 'embeddings', ['content_hash'])

    # Try to create HNSW vector index if vector extension is available
    try:
        result = op.get_bind().execute(text("SELECT 1 FROM pg_available_extensions WHERE name = 'vector' AND installed_version IS NOT NULL"))
        if result.fetchone():
            # Create HNSW vector index with optimized parameters (M=16, ef_construction=200)
            op.execute("CREATE INDEX idx_embeddings_vector_hnsw ON embeddings USING hnsw (embedding vector_l2_ops) WITH (m = 16, ef_construction = 200)")
    except Exception:
        # Vector extension not available, skip vector index
        pass


def downgrade() -> None:
    """Downgrade database schema."""

    # Drop indexes first
    op.drop_index('idx_embeddings_content_hash', table_name='embeddings')
    op.drop_index('idx_embeddings_model', table_name='embeddings')
    op.drop_index('idx_embeddings_entity', table_name='embeddings')

    # Try to drop vector index if it exists
    try:
        op.drop_index('idx_embeddings_vector_hnsw', table_name='embeddings')
    except Exception:
        pass

    op.drop_index('idx_results_validation', table_name='processing_results')
    op.drop_index('idx_results_expiry', table_name='processing_results')
    op.drop_index('idx_results_model', table_name='processing_results')
    op.drop_index('idx_results_job', table_name='processing_results')
    op.drop_index('idx_results_tenant_type', table_name='processing_results')

    op.drop_index('idx_jobs_retry', table_name='processing_jobs')
    op.drop_index('idx_jobs_priority', table_name='processing_jobs')
    op.drop_index('idx_jobs_processor', table_name='processing_jobs')
    op.drop_index('idx_jobs_tenant_status', table_name='processing_jobs')

    op.drop_index('idx_events_status', table_name='processing_events')
    op.drop_index('idx_events_expiry', table_name='processing_events')
    op.drop_index('idx_events_tenant_type', table_name='processing_events')

    op.drop_index('idx_teams_deleted_by', table_name='teams')
    op.drop_index('idx_teams_soft_delete', table_name='teams')
    op.drop_index('idx_teams_location', table_name='teams')
    op.drop_index('idx_teams_projects', table_name='teams')
    op.drop_index('idx_teams_members', table_name='teams')
    op.drop_index('idx_teams_lead', table_name='teams')
    op.drop_index('idx_teams_type', table_name='teams')
    op.drop_index('idx_teams_tenant_name', table_name='teams')

    op.drop_index('idx_projects_deleted_by', table_name='projects')
    op.drop_index('idx_projects_soft_delete', table_name='projects')
    op.drop_index('idx_projects_budget', table_name='projects')
    op.drop_index('idx_projects_priority', table_name='projects')
    op.drop_index('idx_projects_timeline', table_name='projects')
    op.drop_index('idx_projects_hierarchy', table_name='projects')
    op.drop_index('idx_projects_status', table_name='projects')
    op.drop_index('idx_projects_tenant_name', table_name='projects')

    op.drop_index('idx_people_deleted_by', table_name='people')
    op.drop_index('idx_people_soft_delete', table_name='people')
    op.drop_index('idx_people_company', table_name='people')
    op.drop_index('idx_people_phones', table_name='people')
    op.drop_index('idx_people_emails', table_name='people')
    op.drop_index('idx_people_aliases', table_name='people')
    op.drop_index('idx_people_tenant_name', table_name='people')

    op.drop_index('idx_assertions_deleted_by', table_name='assertions')
    op.drop_index('idx_assertions_tags', table_name='assertions')
    op.drop_index('idx_assertions_soft_delete', table_name='assertions')
    op.drop_index('idx_assertions_validated', table_name='assertions')
    op.drop_index('idx_assertions_source', table_name='assertions')
    op.drop_index('idx_assertions_confidence', table_name='assertions')
    op.drop_index('idx_assertions_tenant_type', table_name='assertions')

    op.drop_index('idx_sources_deleted_by', table_name='sources')
    op.drop_index('idx_sources_soft_delete', table_name='sources')
    op.drop_index('idx_sources_status', table_name='sources')
    op.drop_index('idx_sources_hash', table_name='sources')
    op.drop_index('idx_sources_tenant_created', table_name='sources')

    # Drop tables in reverse dependency order
    op.drop_table('embeddings')
    op.drop_table('processing_results')
    op.drop_table('processing_jobs')
    op.drop_table('processing_events')
    op.drop_table('teams')
    op.drop_table('projects')
    op.drop_table('people')
    op.drop_table('assertions')
    op.drop_table('sources')

    # Extensions are left in place as they might be used by other applications