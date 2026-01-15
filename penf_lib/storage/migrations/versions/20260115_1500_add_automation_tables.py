"""Add automation rules engine tables

Revision ID: 004
Revises: 003
Create Date: 2026-01-15 15:00:00.000000

Creates all tables for the Automation Rules Engine feature:
- automation_rules: User-defined automation rules
- automation_rule_versions: Immutable version history
- confidence_thresholds: Per-user per-content-type thresholds
- automation_decisions: Audit trail for all decisions
- rule_effectiveness: Performance metrics
- automation_patterns: Detected behavior patterns
- rule_conflicts: Conflict resolution records
- progressive_settings: User automation preferences

Reference: specs/008-automation-engine/data-model.md
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT, ARRAY


# revision identifiers, used by Alembic.
revision: str = '004'
down_revision: Union[str, None] = '003'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Create automation rules engine tables."""

    # ==========================================================================
    # automation_rules - User-defined automation rules
    # ==========================================================================
    op.create_table(
        'automation_rules',
        sa.Column('id', UUID(as_uuid=True), nullable=False, server_default=sa.text('gen_random_uuid()')),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('user_id', sa.String(254), nullable=False),
        sa.Column('name', sa.String(200), nullable=False),
        sa.Column('description', sa.Text(), nullable=True),
        sa.Column('is_enabled', sa.Boolean(), nullable=False, server_default='true'),
        sa.Column('priority', sa.Integer(), nullable=False, server_default='5'),
        sa.Column('conditions', JSONB(), nullable=False),
        sa.Column('actions', JSONB(), nullable=False),
        sa.Column('current_version_id', BIGINT(), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('deleted_by', sa.String(100), nullable=True),
        sa.Column('deletion_reason', sa.String(500), nullable=True),
        sa.PrimaryKeyConstraint('id'),
        sa.CheckConstraint('priority >= 1 AND priority <= 10', name='ck_rules_priority_range'),
        sa.CheckConstraint("char_length(name) >= 1 AND char_length(name) <= 200", name='ck_rules_name_length'),
    )

    op.create_index('idx_rules_tenant_user', 'automation_rules', ['tenant_id', 'user_id', 'is_enabled'])
    op.create_index('idx_rules_conditions', 'automation_rules', ['conditions'], postgresql_using='gin')
    op.create_index('idx_rules_priority', 'automation_rules', ['priority', 'is_enabled'])
    op.create_index('idx_rules_soft_delete', 'automation_rules', ['is_deleted', 'deleted_at'])

    # ==========================================================================
    # automation_rule_versions - Immutable version history
    # ==========================================================================
    op.create_table(
        'automation_rule_versions',
        sa.Column('id', BIGINT(), autoincrement=True, nullable=False),
        sa.Column('rule_id', UUID(as_uuid=True), nullable=False),
        sa.Column('version_number', sa.Integer(), nullable=False),
        sa.Column('is_active', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('conditions', JSONB(), nullable=False),
        sa.Column('actions', JSONB(), nullable=False),
        sa.Column('change_description', sa.Text(), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('created_by', sa.String(254), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['rule_id'], ['automation_rules.id'], ondelete='CASCADE'),
        sa.UniqueConstraint('rule_id', 'version_number', name='uc_rule_version_number'),
    )

    op.create_index('idx_versions_rule_active', 'automation_rule_versions', ['rule_id', 'is_active'])
    op.create_index('idx_versions_rule_number', 'automation_rule_versions', ['rule_id', 'version_number'])

    # Add FK from automation_rules to automation_rule_versions (after version table created)
    op.create_foreign_key(
        'fk_rules_current_version',
        'automation_rules', 'automation_rule_versions',
        ['current_version_id'], ['id'],
        ondelete='SET NULL'
    )

    # ==========================================================================
    # confidence_thresholds - Per-user per-content-type thresholds
    # ==========================================================================
    op.create_table(
        'confidence_thresholds',
        sa.Column('id', BIGINT(), autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('user_id', sa.String(254), nullable=False),
        sa.Column('content_type', sa.String(50), nullable=False),
        sa.Column('threshold_value', sa.DECIMAL(precision=4, scale=3), nullable=False, server_default='0.850'),
        sa.Column('is_enabled', sa.Boolean(), nullable=False, server_default='true'),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('tenant_id', 'user_id', 'content_type', name='uc_threshold_user_content'),
        sa.CheckConstraint('threshold_value >= 0.000 AND threshold_value <= 1.000', name='ck_threshold_value_range'),
    )

    op.create_index('idx_thresholds_user_type', 'confidence_thresholds', ['tenant_id', 'user_id', 'content_type'])

    # ==========================================================================
    # automation_decisions - Audit trail for all decisions
    # ==========================================================================
    op.create_table(
        'automation_decisions',
        sa.Column('id', BIGINT(), autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('user_id', sa.String(254), nullable=False),
        sa.Column('content_id', sa.String(255), nullable=False),
        sa.Column('content_type', sa.String(50), nullable=False),
        sa.Column('rule_id', UUID(as_uuid=True), nullable=True),
        sa.Column('rule_version_id', BIGINT(), nullable=True),
        sa.Column('decision_type', sa.String(50), nullable=False),
        sa.Column('confidence_score', sa.DECIMAL(precision=4, scale=3), nullable=False),
        sa.Column('threshold_used', sa.DECIMAL(precision=4, scale=3), nullable=False),
        sa.Column('actions_taken', JSONB(), nullable=False),
        sa.Column('reasoning', sa.Text(), nullable=True),
        sa.Column('processing_time_ms', sa.Integer(), nullable=True),
        sa.Column('retry_count', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('error_details', JSONB(), nullable=True),
        sa.Column('user_override', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('user_feedback', JSONB(), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['rule_id'], ['automation_rules.id'], ondelete='SET NULL'),
        sa.ForeignKeyConstraint(['rule_version_id'], ['automation_rule_versions.id'], ondelete='SET NULL'),
        sa.CheckConstraint("decision_type IN ('auto_processed', 'queued_review', 'conflict_resolved')", name='ck_decision_type_valid'),
        sa.CheckConstraint('confidence_score >= 0.000 AND confidence_score <= 1.000', name='ck_decision_confidence_range'),
        sa.CheckConstraint('threshold_used >= 0.000 AND threshold_used <= 1.000', name='ck_decision_threshold_range'),
    )

    op.create_index('idx_decisions_user_date', 'automation_decisions', ['tenant_id', 'user_id', 'created_at'])
    op.create_index('idx_decisions_rule', 'automation_decisions', ['rule_id', 'created_at'])
    op.create_index('idx_decisions_content', 'automation_decisions', ['content_id', 'content_type'])
    op.create_index('idx_decisions_type', 'automation_decisions', ['decision_type', 'created_at'])

    # ==========================================================================
    # rule_effectiveness - Performance metrics
    # ==========================================================================
    op.create_table(
        'rule_effectiveness',
        sa.Column('id', BIGINT(), autoincrement=True, nullable=False),
        sa.Column('rule_id', UUID(as_uuid=True), nullable=False),
        sa.Column('period_start', sa.DateTime(timezone=True), nullable=False),
        sa.Column('period_end', sa.DateTime(timezone=True), nullable=False),
        sa.Column('total_matches', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('total_applied', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('correct_decisions', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('incorrect_decisions', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('accuracy_rate', sa.DECIMAL(precision=4, scale=3), nullable=True),
        sa.Column('avg_confidence', sa.DECIMAL(precision=4, scale=3), nullable=True),
        sa.Column('avg_processing_time_ms', sa.Integer(), nullable=True),
        sa.Column('conflict_count', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('conflict_win_count', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['rule_id'], ['automation_rules.id'], ondelete='CASCADE'),
        sa.UniqueConstraint('rule_id', 'period_start', 'period_end', name='uc_effectiveness_rule_period'),
    )

    op.create_index('idx_effectiveness_rule_period', 'rule_effectiveness', ['rule_id', 'period_start', 'period_end'])
    op.create_index('idx_effectiveness_accuracy', 'rule_effectiveness', ['accuracy_rate', 'total_applied'])

    # ==========================================================================
    # automation_patterns - Detected behavior patterns
    # ==========================================================================
    op.create_table(
        'automation_patterns',
        sa.Column('id', BIGINT(), autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('user_id', sa.String(254), nullable=False),
        sa.Column('pattern_signature', sa.String(64), nullable=False),
        sa.Column('detected_conditions', JSONB(), nullable=False),
        sa.Column('suggested_actions', JSONB(), nullable=False),
        sa.Column('occurrence_count', sa.Integer(), nullable=False),
        sa.Column('first_seen_at', sa.DateTime(timezone=True), nullable=False),
        sa.Column('last_seen_at', sa.DateTime(timezone=True), nullable=False),
        sa.Column('confidence_score', sa.DECIMAL(precision=4, scale=3), nullable=False),
        sa.Column('status', sa.String(20), nullable=False, server_default="'pending'"),
        sa.Column('converted_rule_id', UUID(as_uuid=True), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['converted_rule_id'], ['automation_rules.id'], ondelete='SET NULL'),
        sa.CheckConstraint("status IN ('pending', 'accepted', 'rejected', 'expired')", name='ck_pattern_status_valid'),
        sa.CheckConstraint('confidence_score >= 0.000 AND confidence_score <= 1.000', name='ck_pattern_confidence_range'),
    )

    op.create_index('idx_patterns_user_status', 'automation_patterns', ['tenant_id', 'user_id', 'status'])
    op.create_index('idx_patterns_signature', 'automation_patterns', ['pattern_signature'])
    op.create_index('idx_patterns_confidence', 'automation_patterns', ['confidence_score', 'occurrence_count'])

    # ==========================================================================
    # rule_conflicts - Conflict resolution records
    # ==========================================================================
    op.create_table(
        'rule_conflicts',
        sa.Column('id', BIGINT(), autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('content_id', sa.String(255), nullable=False),
        sa.Column('conflicting_rule_ids', ARRAY(UUID(as_uuid=True)), nullable=False),
        sa.Column('winning_rule_id', UUID(as_uuid=True), nullable=True),
        sa.Column('resolution_method', sa.String(50), nullable=False),
        sa.Column('resolution_reasoning', sa.Text(), nullable=True),
        sa.Column('confidence_scores', JSONB(), nullable=False),
        sa.Column('user_notified', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('user_resolution', UUID(as_uuid=True), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.ForeignKeyConstraint(['winning_rule_id'], ['automation_rules.id'], ondelete='SET NULL'),
        sa.ForeignKeyConstraint(['user_resolution'], ['automation_rules.id'], ondelete='SET NULL'),
        sa.CheckConstraint("resolution_method IN ('confidence', 'priority', 'user')", name='ck_conflict_resolution_valid'),
    )

    op.create_index('idx_conflicts_content', 'rule_conflicts', ['content_id', 'created_at'])
    op.create_index('idx_conflicts_rules', 'rule_conflicts', ['conflicting_rule_ids'], postgresql_using='gin')
    op.create_index('idx_conflicts_winner', 'rule_conflicts', ['winning_rule_id'])

    # ==========================================================================
    # progressive_settings - User automation preferences
    # ==========================================================================
    op.create_table(
        'progressive_settings',
        sa.Column('id', BIGINT(), autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),
        sa.Column('user_id', sa.String(254), nullable=False),
        sa.Column('aggressiveness_level', sa.String(20), nullable=False, server_default="'moderate'"),
        sa.Column('auto_threshold_adjustment', sa.Boolean(), nullable=False, server_default='true'),
        sa.Column('min_learning_period_days', sa.Integer(), nullable=False, server_default='30'),
        sa.Column('target_automation_rate', sa.DECIMAL(precision=4, scale=3), nullable=False, server_default='0.600'),
        sa.Column('accuracy_floor', sa.DECIMAL(precision=4, scale=3), nullable=False, server_default='0.950'),
        sa.Column('pattern_suggestion_enabled', sa.Boolean(), nullable=False, server_default='true'),
        sa.Column('notification_preferences', JSONB(), nullable=False, server_default='{}'),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('tenant_id', 'user_id', name='uc_settings_user'),
        sa.CheckConstraint("aggressiveness_level IN ('conservative', 'moderate', 'aggressive')", name='ck_settings_aggressiveness_valid'),
        sa.CheckConstraint('target_automation_rate >= 0.000 AND target_automation_rate <= 1.000', name='ck_settings_target_rate_range'),
        sa.CheckConstraint('accuracy_floor >= 0.800 AND accuracy_floor <= 1.000', name='ck_settings_accuracy_floor_range'),
        sa.CheckConstraint('min_learning_period_days >= 1', name='ck_settings_learning_period_positive'),
    )

    op.create_index('idx_settings_user', 'progressive_settings', ['tenant_id', 'user_id'])

    # ==========================================================================
    # Row-Level Security (RLS) for tenant isolation
    # ==========================================================================
    # Enable RLS on all automation tables
    for table in ['automation_rules', 'confidence_thresholds', 'automation_decisions',
                  'automation_patterns', 'rule_conflicts', 'progressive_settings']:
        op.execute(f'ALTER TABLE {table} ENABLE ROW LEVEL SECURITY')

        # Create RLS policy
        op.execute(f'''
            CREATE POLICY {table}_tenant_isolation ON {table}
            USING (tenant_id = current_setting('app.current_tenant')::uuid)
            WITH CHECK (tenant_id = current_setting('app.current_tenant')::uuid)
        ''')


def downgrade() -> None:
    """Drop automation rules engine tables."""

    # Drop RLS policies
    for table in ['automation_rules', 'confidence_thresholds', 'automation_decisions',
                  'automation_patterns', 'rule_conflicts', 'progressive_settings']:
        op.execute(f'DROP POLICY IF EXISTS {table}_tenant_isolation ON {table}')
        op.execute(f'ALTER TABLE {table} DISABLE ROW LEVEL SECURITY')

    # Drop foreign key from automation_rules to automation_rule_versions
    op.drop_constraint('fk_rules_current_version', 'automation_rules', type_='foreignkey')

    # Drop tables in reverse dependency order
    op.drop_table('progressive_settings')
    op.drop_table('rule_conflicts')
    op.drop_table('automation_patterns')
    op.drop_table('rule_effectiveness')
    op.drop_table('automation_decisions')
    op.drop_table('confidence_thresholds')
    op.drop_table('automation_rule_versions')
    op.drop_table('automation_rules')
