"""Create initial database schema with all core entities

Revision ID: db131899df87
Revises:
Create Date: 2026-01-12 21:04:36.293497

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

# Import pgvector if available
try:
    from pgvector.sqlalchemy import Vector
except ImportError:
    Vector = None

# revision identifiers, used by Alembic.
revision: str = 'db131899df87'
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade database schema."""
    # Create PostgreSQL extensions
    op.execute("CREATE EXTENSION IF NOT EXISTS vector")
    op.execute("CREATE EXTENSION IF NOT EXISTS ltree")
    op.execute("CREATE EXTENSION IF NOT EXISTS pg_trgm")

    # Create sources table
    op.create_table(
        'sources',
        sa.Column('id', postgresql.BIGINT(), primary_key=True),
        sa.Column('tenant_id', postgresql.UUID(as_uuid=True), nullable=False, index=True,
                  comment='Tenant ID for Row-Level Security'),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  nullable=False, comment='When this record was created'),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  onupdate=sa.func.now(), nullable=False, comment='When this record was last updated'),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True,
                  comment='When this record was soft deleted'),
        sa.Column('is_deleted', sa.Boolean(), default=False, nullable=False, index=True,
                  comment='Whether this record is soft deleted'),
        sa.Column('deleted_by', sa.String(100), nullable=True,
                  comment='User or system that performed the soft delete'),
        sa.Column('deletion_reason', sa.String(500), nullable=True,
                  comment='Reason for deletion for audit purposes'),
        sa.Column('source_system', sa.String(50), nullable=False,
                  comment='System that provided this content (gmail, slack, etc.)'),
        sa.Column('external_id', sa.String(255), nullable=False,
                  comment='ID from the source system'),
        sa.Column('content_hash', sa.String(64), nullable=False,
                  comment='SHA-256 hash of raw content for deduplication'),
        sa.Column('raw_content', sa.Text(), comment='Original content as received from source'),
        sa.Column('content_type', sa.String(100), comment='MIME type or content classification'),
        sa.Column('content_size', sa.Integer(), comment='Size of content in bytes'),
        sa.Column('ingestion_metadata', postgresql.JSONB(), default={},
                  comment='Metadata from ingestion process'),
        sa.Column('processing_status', sa.String(20), default='pending', nullable=False,
                  comment='Current processing status (pending, processing, completed, failed)'),
        sa.Column('source_timestamp', sa.DateTime(timezone=True),
                  comment='When this content was originally created in the source system'),

        # Constraints
        sa.CheckConstraint('length(source_system) > 0', name='ck_sources_source_system_not_empty'),
        sa.CheckConstraint('length(external_id) > 0', name='ck_sources_external_id_not_empty'),
        sa.CheckConstraint('length(content_hash) = 64', name='ck_sources_content_hash_length'),
        sa.CheckConstraint("content_hash ~ '^[a-fA-F0-9]{64}$'", name='ck_sources_content_hash_hex'),
        sa.CheckConstraint('content_size >= 0', name='ck_sources_content_size_non_negative'),
        sa.CheckConstraint(
            "processing_status IN ('pending', 'processing', 'completed', 'failed', 'retrying')",
            name='ck_sources_processing_status_valid'
        ),
        sa.CheckConstraint(
            "source_system IN ('gmail', 'slack', 'zoom', 'calendar', 'drive', 'confluence', 'jira', 'teams')",
            name='ck_sources_source_system_valid'
        ),
    )

    # Create assertions table
    op.create_table(
        'assertions',
        sa.Column('id', postgresql.BIGINT(), primary_key=True),
        sa.Column('tenant_id', postgresql.UUID(as_uuid=True), nullable=False, index=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  onupdate=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), default=False, nullable=False, index=True),
        sa.Column('deleted_by', sa.String(100), nullable=True),
        sa.Column('deletion_reason', sa.String(500), nullable=True),
        sa.Column('source_id', postgresql.BIGINT(), sa.ForeignKey('sources.id'), nullable=False,
                  comment='Reference to the source this assertion was extracted from'),
        sa.Column('assertion_type', sa.String(50), nullable=False,
                  comment='Type of assertion (decision, risk, commitment, milestone, etc.)'),
        sa.Column('content', sa.Text(), nullable=False, comment='The extracted assertion content'),
        sa.Column('context', sa.Text(), comment='Additional context around the assertion'),
        sa.Column('confidence_score', sa.DECIMAL(4, 3),
                  comment='AI confidence in this assertion (0.000-1.000)'),
        sa.Column('extraction_model', sa.String(100), comment='Model used to extract this assertion'),
        sa.Column('processing_metadata', postgresql.JSONB(), default={},
                  comment='Metadata from AI processing'),
        sa.Column('related_entities', postgresql.JSONB(), default={},
                  comment='References to people, projects, etc.'),
        sa.Column('tags', postgresql.ARRAY(sa.String()), comment='Tags for categorization'),
        sa.Column('user_validated', sa.Boolean(), default=False,
                  comment='Whether this assertion has been validated by a human'),
        sa.Column('validation_feedback', postgresql.JSONB(), default={},
                  comment='Human feedback on this assertion'),

        # Constraints
        sa.CheckConstraint('length(assertion_type) > 0', name='ck_assertions_type_not_empty'),
        sa.CheckConstraint('length(content) > 0', name='ck_assertions_content_not_empty'),
        sa.CheckConstraint('length(content) <= 10000', name='ck_assertions_content_max_length'),
        sa.CheckConstraint(
            'confidence_score IS NULL OR (confidence_score >= 0.000 AND confidence_score <= 1.000)',
            name='ck_assertions_confidence_range'
        ),
        sa.CheckConstraint(
            "assertion_type IN ('decision', 'risk', 'commitment', 'milestone', 'outcome', 'dependency', 'assumption', 'issue')",
            name='ck_assertions_type_valid'
        ),
    )

    # Create people table
    op.create_table(
        'people',
        sa.Column('id', postgresql.BIGINT(), primary_key=True),
        sa.Column('tenant_id', postgresql.UUID(as_uuid=True), nullable=False, index=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  onupdate=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), default=False, nullable=False, index=True),
        sa.Column('deleted_by', sa.String(100), nullable=True),
        sa.Column('deletion_reason', sa.String(500), nullable=True),
        sa.Column('canonical_name', sa.String(200), nullable=False, comment='Primary name for this person'),
        sa.Column('aliases', postgresql.ARRAY(sa.String()), default=[],
                  comment='Alternative names, nicknames, email addresses'),
        sa.Column('email_addresses', postgresql.ARRAY(sa.String()), comment='Known email addresses'),
        sa.Column('phone_numbers', postgresql.ARRAY(sa.String()), comment='Known phone numbers'),
        sa.Column('job_title', sa.String(200), comment='Current job title'),
        sa.Column('company', sa.String(200), comment='Current company'),
        sa.Column('department', sa.String(200), comment='Department within company'),
        sa.Column('entity_resolution_metadata', postgresql.JSONB(), default={},
                  comment='Metadata from entity resolution process'),
        sa.Column('confidence_score', sa.DECIMAL(4, 3), comment='Confidence in entity resolution'),

        # Constraints
        sa.CheckConstraint('length(canonical_name) > 0', name='ck_people_name_not_empty'),
        sa.CheckConstraint('length(canonical_name) <= 200', name='ck_people_name_max_length'),
        sa.CheckConstraint(
            'confidence_score IS NULL OR (confidence_score >= 0.000 AND confidence_score <= 1.000)',
            name='ck_people_confidence_range'
        ),
        sa.CheckConstraint(
            'job_title IS NULL OR length(job_title) <= 200',
            name='ck_people_job_title_max_length'
        ),
        sa.CheckConstraint(
            'company IS NULL OR length(company) <= 200',
            name='ck_people_company_max_length'
        ),
        sa.CheckConstraint(
            'department IS NULL OR length(department) <= 200',
            name='ck_people_department_max_length'
        ),
    )

    # Create projects table
    op.create_table(
        'projects',
        sa.Column('id', postgresql.BIGINT(), primary_key=True),
        sa.Column('tenant_id', postgresql.UUID(as_uuid=True), nullable=False, index=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  onupdate=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), default=False, nullable=False, index=True),
        sa.Column('deleted_by', sa.String(100), nullable=True),
        sa.Column('deletion_reason', sa.String(500), nullable=True),
        sa.Column('name', sa.String(200), nullable=False, comment='Project name'),
        sa.Column('description', sa.Text(), comment='Project description'),
        sa.Column('status', sa.String(50), default='active',
                  comment='Project status (active, completed, paused, cancelled)'),
        sa.Column('path', sa.String(500), comment='Hierarchical path using ltree notation'),
        sa.Column('parent_id', postgresql.BIGINT(), sa.ForeignKey('projects.id'), comment='Parent project ID'),
        sa.Column('start_date', sa.DateTime(timezone=True), comment='Project start date'),
        sa.Column('end_date', sa.DateTime(timezone=True), comment='Project end date'),
        sa.Column('target_completion', sa.DateTime(timezone=True), comment='Target completion date'),
        sa.Column('project_type', sa.String(100), comment='Type of project'),
        sa.Column('priority', sa.String(20), comment='Project priority (high, medium, low)'),
        sa.Column('budget_allocated', sa.DECIMAL(12, 2), comment='Budget allocated for project'),
        sa.Column('budget_spent', sa.DECIMAL(12, 2), comment='Budget spent so far'),
        sa.Column('artifacts', postgresql.JSONB(), default=[], comment='References to documents, files, links'),
        sa.Column('milestones', postgresql.JSONB(), default=[], comment='Project milestones'),
        sa.Column('team_members', postgresql.ARRAY(postgresql.BIGINT()),
                  comment='References to people on the team'),

        # Constraints
        sa.CheckConstraint('length(name) > 0', name='ck_projects_name_not_empty'),
        sa.CheckConstraint('length(name) <= 200', name='ck_projects_name_max_length'),
        sa.CheckConstraint(
            "status IN ('planning', 'active', 'on_hold', 'completed', 'cancelled', 'archived')",
            name='ck_projects_status_valid'
        ),
        sa.CheckConstraint(
            "priority IS NULL OR priority IN ('critical', 'high', 'medium', 'low')",
            name='ck_projects_priority_valid'
        ),
        sa.CheckConstraint(
            'budget_allocated IS NULL OR budget_allocated >= 0',
            name='ck_projects_budget_allocated_non_negative'
        ),
        sa.CheckConstraint(
            'budget_spent IS NULL OR budget_spent >= 0',
            name='ck_projects_budget_spent_non_negative'
        ),
        sa.CheckConstraint(
            'end_date IS NULL OR start_date IS NULL OR end_date >= start_date',
            name='ck_projects_end_after_start'
        ),
        sa.CheckConstraint(
            'parent_id IS NULL OR parent_id != id',
            name='ck_projects_no_self_reference'
        ),
    )

    # Create teams table
    op.create_table(
        'teams',
        sa.Column('id', postgresql.BIGINT(), primary_key=True),
        sa.Column('tenant_id', postgresql.UUID(as_uuid=True), nullable=False, index=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  onupdate=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), default=False, nullable=False, index=True),
        sa.Column('deleted_by', sa.String(100), nullable=True),
        sa.Column('deletion_reason', sa.String(500), nullable=True),
        sa.Column('name', sa.String(200), nullable=False, comment='Team name'),
        sa.Column('description', sa.Text(), comment='Team description'),
        sa.Column('team_type', sa.String(100),
                  comment='Type of team (department, project team, working group)'),
        sa.Column('parent_team_id', postgresql.BIGINT(), sa.ForeignKey('teams.id'), comment='Parent team ID'),
        sa.Column('lead_person_id', postgresql.BIGINT(), comment='Reference to team lead person'),
        sa.Column('member_person_ids', postgresql.ARRAY(postgresql.BIGINT()),
                  comment='References to team members'),
        sa.Column('associated_project_ids', postgresql.ARRAY(postgresql.BIGINT()),
                  comment='Associated project IDs'),
        sa.Column('location', sa.String(200), comment='Team location'),
        sa.Column('contact_info', postgresql.JSONB(), comment='Team contact information'),

        # Constraints
        sa.CheckConstraint('length(name) > 0', name='ck_teams_name_not_empty'),
        sa.CheckConstraint('length(name) <= 200', name='ck_teams_name_max_length'),
        sa.CheckConstraint(
            "team_type IS NULL OR team_type IN ('department', 'project_team', 'working_group', 'committee')",
            name='ck_teams_type_valid'
        ),
        sa.CheckConstraint(
            'parent_team_id IS NULL OR parent_team_id != id',
            name='ck_teams_no_self_reference'
        ),
        sa.CheckConstraint(
            'location IS NULL OR length(location) <= 200',
            name='ck_teams_location_max_length'
        ),
        sa.CheckConstraint(
            'lead_person_id IS NULL OR lead_person_id > 0',
            name='ck_teams_lead_id_positive'
        ),
    )

    # Create processing_events table
    op.create_table(
        'processing_events',
        sa.Column('id', postgresql.BIGINT(), primary_key=True),
        sa.Column('tenant_id', postgresql.UUID(as_uuid=True), nullable=False, index=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  onupdate=sa.func.now(), nullable=False),
        sa.Column('event_id', postgresql.UUID(as_uuid=True), unique=True, nullable=False,
                  comment='Unique event identifier'),
        sa.Column('event_type', sa.String(100), nullable=False,
                  comment='Type of event (content.ingested, etc.)'),
        sa.Column('payload', postgresql.JSONB(), nullable=False, comment='Event data'),
        sa.Column('payload_size', sa.Integer(), comment='Size of payload in bytes'),
        sa.Column('publisher', sa.String(100), comment='Service that published this event'),
        sa.Column('subscriber_count', sa.Integer(), default=0, comment='Number of subscribers'),
        sa.Column('processing_status', sa.String(20), default='published',
                  comment='Event processing status'),
        sa.Column('published_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('expires_at', sa.DateTime(timezone=True),
                  server_default=sa.text("NOW() + INTERVAL '30 days'"), nullable=False),

        # Constraints
        sa.CheckConstraint('length(event_type) > 0', name='ck_events_type_not_empty'),
        sa.CheckConstraint('payload_size >= 0', name='ck_events_payload_size_non_negative'),
        sa.CheckConstraint('subscriber_count >= 0', name='ck_events_subscriber_count_non_negative'),
        sa.CheckConstraint(
            "processing_status IN ('published', 'processing', 'completed', 'failed')",
            name='ck_events_processing_status_valid'
        ),
    )

    # Create processing_jobs table
    op.create_table(
        'processing_jobs',
        sa.Column('id', postgresql.BIGINT(), primary_key=True),
        sa.Column('tenant_id', postgresql.UUID(as_uuid=True), nullable=False, index=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  onupdate=sa.func.now(), nullable=False),
        sa.Column('job_id', postgresql.UUID(as_uuid=True), unique=True, nullable=False,
                  comment='Unique job identifier'),
        sa.Column('event_id', postgresql.UUID(as_uuid=True),
                  sa.ForeignKey('processing_events.event_id'), nullable=False,
                  comment='Reference to triggering event'),
        sa.Column('processor_name', sa.String(100), nullable=False,
                  comment='Name of the processor handling this job'),
        sa.Column('job_type', sa.String(100), nullable=False, comment='Type of processing job'),
        sa.Column('status', sa.String(20), default='queued', nullable=False,
                  comment='Job status (queued, in_progress, completed, failed, retrying)'),
        sa.Column('priority', sa.Integer(), default=100, comment='Job priority (lower = higher priority)'),
        sa.Column('input_data', postgresql.JSONB(), comment='Input data for processing'),
        sa.Column('processor_metadata', postgresql.JSONB(), comment='Processor-specific metadata'),
        sa.Column('started_at', sa.DateTime(timezone=True), comment='When job execution started'),
        sa.Column('completed_at', sa.DateTime(timezone=True), comment='When job execution completed'),
        sa.Column('retry_count', sa.Integer(), default=0, comment='Number of retry attempts'),
        sa.Column('max_retries', sa.Integer(), default=3, comment='Maximum number of retries'),
        sa.Column('error_message', sa.Text(), comment='Error message if job failed'),
        sa.Column('error_details', postgresql.JSONB(), comment='Detailed error information'),

        # Constraints
        sa.CheckConstraint('length(processor_name) > 0', name='ck_jobs_processor_name_not_empty'),
        sa.CheckConstraint('length(job_type) > 0', name='ck_jobs_type_not_empty'),
        sa.CheckConstraint('retry_count >= 0', name='ck_jobs_retry_count_non_negative'),
        sa.CheckConstraint('max_retries >= 0', name='ck_jobs_max_retries_non_negative'),
        sa.CheckConstraint(
            "status IN ('queued', 'in_progress', 'completed', 'failed', 'retrying', 'cancelled')",
            name='ck_jobs_status_valid'
        ),
    )

    # Create processing_results table
    op.create_table(
        'processing_results',
        sa.Column('id', postgresql.BIGINT(), primary_key=True),
        sa.Column('tenant_id', postgresql.UUID(as_uuid=True), nullable=False, index=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  onupdate=sa.func.now(), nullable=False),
        sa.Column('job_id', postgresql.UUID(as_uuid=True),
                  sa.ForeignKey('processing_jobs.job_id'), nullable=False,
                  comment='Reference to the processing job'),
        sa.Column('result_type', sa.String(100), nullable=False,
                  comment='Type of result (extraction, classification, etc.)'),
        sa.Column('result_data', postgresql.JSONB(), nullable=False, comment='The actual processing result'),
        sa.Column('confidence_score', sa.DECIMAL(4, 3), comment='Processor confidence in result'),
        sa.Column('model_name', sa.String(100), comment='Model used for processing'),
        sa.Column('model_version', sa.String(50), comment='Version of the model'),
        sa.Column('processor_version', sa.String(50), comment='Version of the processor'),
        sa.Column('processing_time_ms', sa.Integer(), comment='Processing time in milliseconds'),
        sa.Column('quality_score', sa.DECIMAL(4, 3), comment='Quality assessment of result'),
        sa.Column('compared_against', postgresql.ARRAY(postgresql.UUID()),
                  comment='IDs of results this was compared against'),
        sa.Column('validation_status', sa.String(20),
                  comment='Validation status (pending, approved, rejected)'),
        sa.Column('human_feedback', postgresql.JSONB(), comment='Human feedback on result quality'),
        sa.Column('expires_at', sa.DateTime(timezone=True),
                  server_default=sa.text("NOW() + INTERVAL '30 days'"), nullable=False),

        # Constraints
        sa.CheckConstraint('length(result_type) > 0', name='ck_results_type_not_empty'),
        sa.CheckConstraint(
            'confidence_score IS NULL OR (confidence_score >= 0.000 AND confidence_score <= 1.000)',
            name='ck_results_confidence_range'
        ),
        sa.CheckConstraint(
            'quality_score IS NULL OR (quality_score >= 0.000 AND quality_score <= 1.000)',
            name='ck_results_quality_range'
        ),
        sa.CheckConstraint('processing_time_ms >= 0', name='ck_results_processing_time_non_negative'),
        sa.CheckConstraint(
            "validation_status IS NULL OR validation_status IN ('pending', 'approved', 'rejected', 'needs_review')",
            name='ck_results_validation_status_valid'
        ),
    )

    # Create embeddings table
    op.create_table(
        'embeddings',
        sa.Column('id', postgresql.BIGINT(), primary_key=True),
        sa.Column('tenant_id', postgresql.UUID(as_uuid=True), nullable=False, index=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(),
                  onupdate=sa.func.now(), nullable=False),
        sa.Column('entity_type', sa.String(50), nullable=False,
                  comment='Type of entity (source, assertion, person, etc.)'),
        sa.Column('entity_id', postgresql.BIGINT(), nullable=False, comment='ID of the linked entity'),
        sa.Column('source_id', postgresql.BIGINT(), sa.ForeignKey('sources.id'),
                  comment='Reference to source if applicable'),
        sa.Column('assertion_id', postgresql.BIGINT(), sa.ForeignKey('assertions.id'),
                  comment='Reference to assertion if applicable'),
        sa.Column('person_id', postgresql.BIGINT(), sa.ForeignKey('people.id'),
                  comment='Reference to person if applicable'),
        sa.Column('project_id', postgresql.BIGINT(), sa.ForeignKey('projects.id'),
                  comment='Reference to project if applicable'),
        sa.Column('team_id', postgresql.BIGINT(), sa.ForeignKey('teams.id'),
                  comment='Reference to team if applicable'),
        sa.Column('embedding',
                  postgresql.ARRAY(sa.Float()) if not Vector else Vector(768), nullable=False,
                  comment='768-dimensional vector embedding'),
        sa.Column('embedding_model', sa.String(100), nullable=False,
                  comment='Model used to generate embedding'),
        sa.Column('model_version', sa.String(50), comment='Version of the embedding model'),
        sa.Column('text_content', sa.Text(), comment='Text content that was embedded'),
        sa.Column('content_hash', sa.String(64), comment='Hash of the text content'),
        sa.Column('generation_confidence', sa.DECIMAL(4, 3),
                  comment='Confidence in embedding generation'),
        sa.Column('search_count', sa.Integer(), default=0, comment='Number of times used in searches'),
        sa.Column('last_searched_at', sa.DateTime(timezone=True), comment='Last time used in search'),

        # Constraints
        sa.CheckConstraint('length(entity_type) > 0', name='ck_embeddings_entity_type_not_empty'),
        sa.CheckConstraint('entity_id > 0', name='ck_embeddings_entity_id_positive'),
        sa.CheckConstraint('length(embedding_model) > 0', name='ck_embeddings_model_not_empty'),
        sa.CheckConstraint('search_count >= 0', name='ck_embeddings_search_count_non_negative'),
        sa.CheckConstraint(
            'generation_confidence IS NULL OR (generation_confidence >= 0.000 AND generation_confidence <= 1.000)',
            name='ck_embeddings_confidence_range'
        ),
        sa.CheckConstraint(
            "entity_type IN ('source', 'assertion', 'person', 'project', 'team')",
            name='ck_embeddings_entity_type_valid'
        ),
        sa.CheckConstraint(
            'content_hash IS NULL OR length(content_hash) = 64',
            name='ck_embeddings_content_hash_length'
        ),
    )


def downgrade() -> None:
    """Downgrade database schema."""
    # Drop all tables in reverse order
    op.drop_table('embeddings')
    op.drop_table('processing_results')
    op.drop_table('processing_jobs')
    op.drop_table('processing_events')
    op.drop_table('teams')
    op.drop_table('projects')
    op.drop_table('people')
    op.drop_table('assertions')
    op.drop_table('sources')

    # Note: We don't drop extensions as they might be used by other databases