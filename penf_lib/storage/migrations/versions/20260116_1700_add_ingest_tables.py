"""Add manual ingest tables

Revision ID: 006
Revises: 005
Create Date: 2026-01-16 17:00:00.000000

Creates 4 tables for the manual content ingest feature (012):
- ingest_jobs: Batch upload job tracking with resume capability
- ingest_errors: Detailed error records for failed files
- email_attachments: Links emails to extracted documents
- archived_files: Encrypted original file storage

Based on specs/012-manual-ingest/data-model.md
"""

from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT
from sqlalchemy.sql import text


# revision identifiers, used by Alembic.
revision: str = '006'
down_revision: Union[str, None] = '005'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Create manual ingest tables."""

    # =========================================================================
    # 1. ingest_jobs - Batch upload job tracking
    # =========================================================================
    op.create_table(
        'ingest_jobs',
        sa.Column('id', UUID(as_uuid=True), primary_key=True,
                  server_default=text("gen_random_uuid()")),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),

        # Job configuration
        sa.Column('source_tag', sa.String(100), nullable=False,
                  comment="User-provided source identifier"),
        sa.Column('content_type', sa.String(50), nullable=False,
                  server_default='email',
                  comment="Content type being ingested"),

        # Status tracking
        sa.Column('status', sa.String(20), nullable=False,
                  server_default='pending',
                  comment="Job status: pending, in_progress, completed, failed, cancelled"),

        # Progress counters
        sa.Column('total_files', sa.Integer(), nullable=False,
                  comment="Total files in batch"),
        sa.Column('processed_count', sa.Integer(), nullable=False,
                  server_default='0',
                  comment="Files processed so far"),
        sa.Column('imported_count', sa.Integer(), nullable=False,
                  server_default='0',
                  comment="Successfully imported"),
        sa.Column('skipped_count', sa.Integer(), nullable=False,
                  server_default='0',
                  comment="Skipped (duplicates)"),
        sa.Column('failed_count', sa.Integer(), nullable=False,
                  server_default='0',
                  comment="Failed files"),

        # File tracking for resume
        sa.Column('file_manifest', JSONB(), nullable=False,
                  comment="List of all file paths in batch"),
        sa.Column('processed_files', JSONB(), nullable=False,
                  server_default='[]',
                  comment="Completed file paths (for resume)"),

        # Job options
        sa.Column('options', JSONB(), nullable=False,
                  server_default='{}',
                  comment="Job options (preserve_folders, skip_attachments, etc.)"),

        # Timing
        sa.Column('started_at', sa.DateTime(timezone=True), nullable=True,
                  comment="When processing started"),
        sa.Column('completed_at', sa.DateTime(timezone=True), nullable=True,
                  comment="When processing finished"),

        # Timestamps
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "status IN ('pending', 'in_progress', 'completed', 'failed', 'cancelled')",
            name='ck_ingest_job_status_valid'
        ),
        sa.CheckConstraint(
            "total_files > 0",
            name='ck_ingest_job_total_files_positive'
        ),
        sa.CheckConstraint(
            "processed_count >= 0 AND processed_count <= total_files",
            name='ck_ingest_job_processed_count_valid'
        ),

        comment="Batch upload job tracking with progress for resume capability"
    )

    # Indexes for ingest_jobs
    op.create_index('idx_ingest_jobs_tenant_status', 'ingest_jobs',
                    ['tenant_id', 'status'])
    op.create_index('idx_ingest_jobs_tenant_created', 'ingest_jobs',
                    ['tenant_id', 'created_at'])

    # =========================================================================
    # 2. ingest_errors - Detailed error records
    # =========================================================================
    op.create_table(
        'ingest_errors',
        sa.Column('id', UUID(as_uuid=True), primary_key=True,
                  server_default=text("gen_random_uuid()")),
        sa.Column('job_id', UUID(as_uuid=True),
                  sa.ForeignKey('ingest_jobs.id', ondelete='CASCADE'),
                  nullable=False),

        # Error details
        sa.Column('file_path', sa.String(500), nullable=False,
                  comment="Path to failed file"),
        sa.Column('error_type', sa.String(50), nullable=False,
                  comment="Error category"),
        sa.Column('error_message', sa.Text(), nullable=False,
                  comment="Human-readable error message"),
        sa.Column('error_details', JSONB(), nullable=True,
                  comment="Stack trace and additional context"),

        # Timestamps
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "error_type IN ('parse_error', 'encoding_error', 'io_error', 'validation_error', 'storage_error')",
            name='ck_ingest_error_type_valid'
        ),

        comment="Detailed error records for failed files in an ingest job"
    )

    # Indexes for ingest_errors
    op.create_index('idx_ingest_errors_job', 'ingest_errors', ['job_id'])

    # =========================================================================
    # 3. email_attachments - Links emails to extracted documents
    # =========================================================================
    op.create_table(
        'email_attachments',
        sa.Column('id', UUID(as_uuid=True), primary_key=True,
                  server_default=text("gen_random_uuid()")),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('source_id', BIGINT(),
                  sa.ForeignKey('sources.id', ondelete='CASCADE'),
                  nullable=False,
                  comment="Parent email source"),
        sa.Column('document_id', UUID(as_uuid=True), nullable=True,
                  comment="Extracted document record (future)"),

        # Attachment metadata
        sa.Column('filename', sa.String(255), nullable=False,
                  comment="Original filename"),
        sa.Column('mime_type', sa.String(100), nullable=False,
                  comment="MIME content type"),
        sa.Column('size_bytes', sa.Integer(), nullable=False,
                  comment="File size in bytes"),

        # Extraction status
        sa.Column('extraction_status', sa.String(20), nullable=False,
                  server_default='pending',
                  comment="Status: pending, completed, failed, skipped_oversized, skipped_unsupported"),
        sa.Column('extraction_error', sa.Text(), nullable=True,
                  comment="Error message if extraction failed"),

        # Timestamps
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "extraction_status IN ('pending', 'completed', 'failed', 'skipped_oversized', 'skipped_unsupported')",
            name='ck_attachment_status_valid'
        ),
        sa.CheckConstraint(
            "size_bytes >= 0",
            name='ck_attachment_size_non_negative'
        ),

        comment="Links email sources to extracted document records"
    )

    # Indexes for email_attachments
    op.create_index('idx_email_attachments_source', 'email_attachments', ['source_id'])
    op.create_index('idx_email_attachments_document', 'email_attachments',
                    ['document_id'],
                    postgresql_where=text("document_id IS NOT NULL"))

    # =========================================================================
    # 4. archived_files - Encrypted original file storage
    # =========================================================================
    op.create_table(
        'archived_files',
        sa.Column('id', UUID(as_uuid=True), primary_key=True,
                  server_default=text("gen_random_uuid()")),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('source_id', BIGINT(),
                  sa.ForeignKey('sources.id', ondelete='CASCADE'),
                  nullable=False, unique=True,
                  comment="Link to processed email"),

        # Encrypted content
        sa.Column('encrypted_content', sa.Text(), nullable=False,
                  comment="Base64-encoded AES-256-GCM encrypted file"),
        sa.Column('original_size', sa.Integer(), nullable=False,
                  comment="Unencrypted file size in bytes"),

        # Key management
        sa.Column('encryption_key_id', sa.String(100), nullable=False,
                  comment="Key identifier for rotation"),

        # Integrity
        sa.Column('content_hash', sa.String(64), nullable=False,
                  comment="SHA-256 of original content (pre-encryption)"),

        # Timestamps
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "original_size > 0",
            name='ck_archived_file_size_positive'
        ),
        sa.CheckConstraint(
            "length(content_hash) = 64",
            name='ck_archived_file_hash_length'
        ),

        comment="Encrypted storage of original .eml files for compliance retrieval"
    )

    # Indexes for archived_files
    op.create_index('idx_archived_files_source', 'archived_files',
                    ['source_id'], unique=True)


def downgrade() -> None:
    """Drop manual ingest tables."""

    # Drop tables in reverse order (dependencies first)
    op.drop_table('archived_files')
    op.drop_table('email_attachments')
    op.drop_table('ingest_errors')
    op.drop_table('ingest_jobs')
