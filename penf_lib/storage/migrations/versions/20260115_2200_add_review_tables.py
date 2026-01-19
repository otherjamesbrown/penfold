"""Add daily review workflow tables

Revision ID: 003
Revises: 002
Create Date: 2026-01-15 22:00:00.000000

Creates 6 tables for the daily review workflow:
- review_sessions: User review workflow sessions
- review_items: Individual items in review queue
- user_feedback: User feedback on AI suggestions
- learning_rules: Automated decision patterns
- batch_operations: Batch operation records
- review_analytics: Aggregated metrics (P3)

Based on specs/006-daily-review/data-model.md
"""

from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT
from sqlalchemy.sql import text


# revision identifiers, used by Alembic.
revision: str = '003'
down_revision: Union[str, None] = '002'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Create review workflow tables."""

    # =========================================================================
    # 1. review_sessions - User review workflow sessions
    # =========================================================================
    op.create_table(
        'review_sessions',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True),
        sa.Column('session_uuid', UUID(as_uuid=True), nullable=False, unique=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('user_email', sa.String(254), nullable=False),
        sa.Column('status', sa.String(20), nullable=False, server_default='active'),
        sa.Column('started_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('last_activity_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('completed_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('expires_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=text("NOW() + INTERVAL '24 hours'")),
        sa.Column('current_position', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('total_items', sa.Integer(), nullable=False),
        sa.Column('items_reviewed', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('items_accepted', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('items_rejected', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('items_modified', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('items_skipped', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('review_mode', sa.String(30), nullable=False, server_default='standard'),
        sa.Column('priority_mode', sa.String(30), nullable=False, server_default='mixed'),
        sa.Column('filter_criteria', JSONB(), nullable=False, server_default='{}'),
        sa.Column('session_metadata', JSONB(), nullable=False, server_default='{}'),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "status IN ('active', 'paused', 'completed', 'abandoned')",
            name='ck_review_session_status'
        ),
        sa.CheckConstraint(
            "review_mode IN ('quick', 'standard', 'detailed')",
            name='ck_review_session_mode'
        ),
        sa.CheckConstraint(
            "priority_mode IN ('confidence', 'importance', 'recency', 'mixed')",
            name='ck_review_session_priority_mode'
        ),
        sa.CheckConstraint(
            "items_reviewed = items_accepted + items_rejected + items_modified + items_skipped",
            name='ck_review_session_counts'
        ),

        comment="User review workflow sessions with state management and progress tracking"
    )

    # Indexes for review_sessions
    op.create_index('idx_review_session_tenant_user', 'review_sessions',
                    ['tenant_id', 'user_email', 'status'])
    op.create_index('idx_review_session_active', 'review_sessions',
                    ['status', 'expires_at'],
                    postgresql_where=text("status = 'active'"))
    op.create_index('idx_review_session_uuid', 'review_sessions', ['session_uuid'])

    # =========================================================================
    # 2. review_items - Individual items in review queue
    # =========================================================================
    op.create_table(
        'review_items',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('session_id', BIGINT(), sa.ForeignKey('review_sessions.id'), nullable=True),
        sa.Column('source_id', BIGINT(), sa.ForeignKey('sources.id'), nullable=False),
        sa.Column('processing_result_id', UUID(as_uuid=True), nullable=True),
        sa.Column('queue_position', sa.Integer(), nullable=True),
        sa.Column('status', sa.String(20), nullable=False, server_default='pending'),
        sa.Column('content_type', sa.String(50), nullable=False),
        sa.Column('content_preview', sa.Text(), nullable=False),
        sa.Column('ai_suggestion', JSONB(), nullable=False),
        sa.Column('ai_confidence', sa.Numeric(4, 3), nullable=False),
        sa.Column('ai_model', sa.String(100), nullable=False),
        sa.Column('business_importance', sa.Integer(), nullable=False, server_default='5'),
        sa.Column('source_timestamp', sa.DateTime(timezone=True), nullable=False),
        sa.Column('reviewed_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('review_duration_ms', sa.Integer(), nullable=True),
        sa.Column('user_decision', sa.String(20), nullable=True),
        sa.Column('user_correction', JSONB(), nullable=True),
        sa.Column('batch_id', UUID(as_uuid=True), nullable=True),
        sa.Column('undo_eligible', sa.Boolean(), nullable=False, server_default='true'),
        sa.Column('undo_deadline', sa.DateTime(timezone=True), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "status IN ('pending', 'in_review', 'accepted', 'rejected', 'modified', 'skipped')",
            name='ck_review_item_status'
        ),
        sa.CheckConstraint(
            "content_type IN ('email', 'meeting', 'document')",
            name='ck_review_item_content_type'
        ),
        sa.CheckConstraint(
            "ai_confidence >= 0.000 AND ai_confidence <= 1.000",
            name='ck_review_item_confidence'
        ),
        sa.CheckConstraint(
            "business_importance >= 1 AND business_importance <= 10",
            name='ck_review_item_importance'
        ),
        sa.CheckConstraint(
            "user_decision IS NULL OR user_decision IN ('accept', 'reject', 'modify', 'skip')",
            name='ck_review_item_decision'
        ),

        comment="Individual items in the review queue linking AI processing results to review workflow"
    )

    # Indexes for review_items
    op.create_index('idx_review_item_session', 'review_items',
                    ['session_id', 'queue_position'])
    op.create_index('idx_review_item_status', 'review_items',
                    ['tenant_id', 'status', 'ai_confidence'])
    op.create_index('idx_review_item_source', 'review_items', ['source_id'])
    op.create_index('idx_review_item_batch', 'review_items', ['batch_id'],
                    postgresql_where=text("batch_id IS NOT NULL"))
    op.create_index('idx_review_item_undo', 'review_items',
                    ['undo_eligible', 'undo_deadline'],
                    postgresql_where=text("undo_eligible = TRUE"))

    # =========================================================================
    # 3. user_feedback - User feedback on AI suggestions
    # =========================================================================
    op.create_table(
        'user_feedback',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True),
        sa.Column('feedback_uuid', UUID(as_uuid=True), nullable=False, unique=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('review_item_id', BIGINT(), sa.ForeignKey('review_items.id'), nullable=False),
        sa.Column('session_id', BIGINT(), sa.ForeignKey('review_sessions.id'), nullable=False),
        sa.Column('decision_type', sa.String(20), nullable=False),
        sa.Column('original_suggestion', JSONB(), nullable=False),
        sa.Column('user_correction', JSONB(), nullable=True),
        sa.Column('correction_reason', sa.String(100), nullable=True),
        sa.Column('correction_notes', sa.Text(), nullable=True),
        sa.Column('confidence_feedback', sa.String(20), nullable=True),
        sa.Column('time_spent_ms', sa.Integer(), nullable=False),
        sa.Column('was_batch_decision', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('batch_id', UUID(as_uuid=True), nullable=True),
        sa.Column('learning_rule_id', BIGINT(), nullable=True),  # FK added after learning_rules created
        sa.Column('learning_signal_quality', sa.Numeric(3, 2), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "decision_type IN ('accept', 'reject', 'modify')",
            name='ck_user_feedback_decision_type'
        ),
        sa.CheckConstraint(
            "confidence_feedback IS NULL OR confidence_feedback IN ('too_high', 'accurate', 'too_low')",
            name='ck_user_feedback_confidence'
        ),
        sa.CheckConstraint(
            "learning_signal_quality IS NULL OR (learning_signal_quality >= 0.00 AND learning_signal_quality <= 1.00)",
            name='ck_user_feedback_signal_quality'
        ),

        comment="User validation decisions for learning system integration"
    )

    # Indexes for user_feedback
    op.create_index('idx_user_feedback_session', 'user_feedback',
                    ['session_id', 'created_at'])
    op.create_index('idx_user_feedback_item', 'user_feedback', ['review_item_id'])
    op.create_index('idx_user_feedback_learning', 'user_feedback',
                    ['tenant_id', 'decision_type', 'correction_reason'])
    op.create_index('idx_user_feedback_batch', 'user_feedback', ['batch_id'],
                    postgresql_where=text("batch_id IS NOT NULL"))

    # =========================================================================
    # 4. learning_rules - Automated decision patterns
    # =========================================================================
    op.create_table(
        'learning_rules',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True),
        sa.Column('rule_uuid', UUID(as_uuid=True), nullable=False, unique=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('name', sa.String(200), nullable=False),
        sa.Column('description', sa.Text(), nullable=True),
        sa.Column('status', sa.String(20), nullable=False, server_default='active'),
        sa.Column('rule_type', sa.String(50), nullable=False),
        sa.Column('match_criteria', JSONB(), nullable=False),
        sa.Column('action', JSONB(), nullable=False),
        sa.Column('confidence_threshold', sa.Numeric(4, 3), nullable=False, server_default='0.800'),
        sa.Column('priority', sa.Integer(), nullable=False, server_default='100'),
        sa.Column('derived_from_count', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('times_applied', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('times_overridden', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('effectiveness_score', sa.Numeric(4, 3), nullable=True),
        sa.Column('last_applied_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('expires_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('created_by', sa.String(254), nullable=False),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('is_deleted', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),

        # Constraints
        sa.CheckConstraint(
            "status IN ('active', 'disabled', 'expired')",
            name='ck_learning_rule_status'
        ),
        sa.CheckConstraint(
            "rule_type IN ('sender_match', 'content_pattern', 'category_override', 'participant_mapping')",
            name='ck_learning_rule_type'
        ),
        sa.CheckConstraint(
            "confidence_threshold >= 0.000 AND confidence_threshold <= 1.000",
            name='ck_learning_rule_confidence_threshold'
        ),
        sa.CheckConstraint(
            "priority >= 1 AND priority <= 1000",
            name='ck_learning_rule_priority'
        ),
        sa.CheckConstraint(
            "effectiveness_score IS NULL OR (effectiveness_score >= 0.000 AND effectiveness_score <= 1.000)",
            name='ck_learning_rule_effectiveness'
        ),

        comment="Automated decision patterns derived from user feedback"
    )

    # Indexes for learning_rules
    op.create_index('idx_learning_rule_tenant_status', 'learning_rules',
                    ['tenant_id', 'status', 'priority'])
    op.create_index('idx_learning_rule_type', 'learning_rules',
                    ['rule_type', 'status'])
    op.create_index('idx_learning_rule_effectiveness', 'learning_rules',
                    ['effectiveness_score'],
                    postgresql_ops={'effectiveness_score': 'DESC'},
                    postgresql_where=text("status = 'active'"))
    op.create_index('idx_learning_rule_soft_delete', 'learning_rules',
                    ['is_deleted', 'deleted_at'])

    # Add FK from user_feedback to learning_rules now that learning_rules exists
    op.create_foreign_key(
        'fk_user_feedback_learning_rule',
        'user_feedback', 'learning_rules',
        ['learning_rule_id'], ['id']
    )

    # =========================================================================
    # 5. batch_operations - Batch operation records
    # =========================================================================
    op.create_table(
        'batch_operations',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True),
        sa.Column('batch_uuid', UUID(as_uuid=True), nullable=False, unique=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('session_id', BIGINT(), sa.ForeignKey('review_sessions.id'), nullable=False),
        sa.Column('batch_type', sa.String(50), nullable=False),
        sa.Column('group_criteria', JSONB(), nullable=False),
        sa.Column('action_type', sa.String(20), nullable=False),
        sa.Column('action_details', JSONB(), nullable=False),
        sa.Column('item_count', sa.Integer(), nullable=False),
        sa.Column('status', sa.String(20), nullable=False, server_default='pending'),
        sa.Column('confirmed_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('applied_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('undone_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('undo_eligible', sa.Boolean(), nullable=False, server_default='true'),
        sa.Column('undo_deadline', sa.DateTime(timezone=True), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "batch_type IN ('thread', 'sender', 'category', 'time_window', 'custom')",
            name='ck_batch_operation_type'
        ),
        sa.CheckConstraint(
            "action_type IN ('accept_all', 'reject_all', 'apply_category', 'skip_all')",
            name='ck_batch_operation_action_type'
        ),
        sa.CheckConstraint(
            "status IN ('pending', 'confirmed', 'applied', 'undone')",
            name='ck_batch_operation_status'
        ),
        sa.CheckConstraint(
            "item_count > 0",
            name='ck_batch_operation_item_count'
        ),

        comment="Groups related review items for bulk actions"
    )

    # Indexes for batch_operations
    op.create_index('idx_batch_session', 'batch_operations',
                    ['session_id', 'created_at'])
    op.create_index('idx_batch_status', 'batch_operations',
                    ['tenant_id', 'status', 'created_at'])
    op.create_index('idx_batch_undo', 'batch_operations',
                    ['undo_eligible', 'undo_deadline'],
                    postgresql_where=text("undo_eligible = TRUE"))

    # =========================================================================
    # 6. review_analytics - Aggregated metrics (P3)
    # =========================================================================
    op.create_table(
        'review_analytics',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id'), nullable=False),
        sa.Column('period_start', sa.Date(), nullable=False),
        sa.Column('period_end', sa.Date(), nullable=False),
        sa.Column('period_type', sa.String(20), nullable=False),
        sa.Column('total_sessions', sa.Integer(), nullable=False),
        sa.Column('total_items_reviewed', sa.Integer(), nullable=False),
        sa.Column('avg_session_duration_min', sa.Numeric(6, 2), nullable=False),
        sa.Column('avg_items_per_minute', sa.Numeric(5, 2), nullable=False),
        sa.Column('acceptance_rate', sa.Numeric(5, 4), nullable=False),
        sa.Column('modification_rate', sa.Numeric(5, 4), nullable=False),
        sa.Column('rejection_rate', sa.Numeric(5, 4), nullable=False),
        sa.Column('skip_rate', sa.Numeric(5, 4), nullable=False),
        sa.Column('avg_ai_confidence', sa.Numeric(4, 3), nullable=False),
        sa.Column('ai_accuracy', sa.Numeric(5, 4), nullable=True),
        sa.Column('batch_usage_rate', sa.Numeric(5, 4), nullable=False),
        sa.Column('learning_rules_created', sa.Integer(), nullable=False),
        sa.Column('learning_rules_applied', sa.Integer(), nullable=False),
        sa.Column('content_type_breakdown', JSONB(), nullable=False),
        sa.Column('correction_type_breakdown', JSONB(), nullable=False),
        sa.Column('created_at', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),

        # Constraints
        sa.CheckConstraint(
            "period_type IN ('daily', 'weekly', 'monthly')",
            name='ck_review_analytics_period_type'
        ),
        sa.CheckConstraint(
            "period_end >= period_start",
            name='ck_review_analytics_period'
        ),

        comment="Aggregated metrics for review patterns and system learning progress (P3)"
    )

    # Indexes for review_analytics
    op.create_index('idx_analytics_tenant_period', 'review_analytics',
                    ['tenant_id', 'period_type', 'period_start'],
                    postgresql_ops={'period_start': 'DESC'})

    # =========================================================================
    # Enable Row-Level Security on new tables
    # =========================================================================
    review_tables = [
        'review_sessions', 'review_items', 'user_feedback',
        'learning_rules', 'batch_operations', 'review_analytics'
    ]

    for table_name in review_tables:
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
    """Remove review workflow tables."""

    review_tables = [
        'review_analytics', 'batch_operations', 'learning_rules',
        'user_feedback', 'review_items', 'review_sessions'
    ]

    # Remove RLS policies
    for table_name in review_tables:
        op.execute(f"DROP POLICY IF EXISTS tenant_isolation_policy ON {table_name}")
        op.execute(f"DROP POLICY IF EXISTS superuser_access_policy ON {table_name}")
        op.execute(f"ALTER TABLE {table_name} DISABLE ROW LEVEL SECURITY")

    # Drop FK constraint before dropping tables
    op.drop_constraint('fk_user_feedback_learning_rule', 'user_feedback', type_='foreignkey')

    # Drop tables in reverse order
    for table_name in review_tables:
        op.drop_table(table_name)
