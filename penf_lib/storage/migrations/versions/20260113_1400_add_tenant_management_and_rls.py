"""Add tenant management and RLS policies

Revision ID: 002
Revises: 001
Create Date: 2026-01-13 14:00:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT
from sqlalchemy.sql import text


# revision identifiers, used by Alembic.
revision: str = '002'
down_revision: Union[str, None] = '001'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade database schema with tenant management and RLS policies."""

    # Create tenants table
    op.create_table(
        'tenants',
        sa.Column('id', UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column('name', sa.String(length=50), nullable=False, unique=True),
        sa.Column('display_name', sa.String(length=100), nullable=False),
        sa.Column('description', sa.Text(), nullable=True),
        sa.Column('settings', JSONB(), nullable=True, server_default='{}'),
        sa.Column('is_active', sa.Boolean(), nullable=False, server_default='true'),
        sa.Column('owner_email', sa.String(length=254), nullable=False),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('is_deleted', sa.Boolean(), nullable=False, server_default='false'),
        sa.Column('deleted_by', sa.String(length=100), nullable=True),
        sa.Column('deletion_reason', sa.String(length=500), nullable=True),

        sa.CheckConstraint("name IN ('work', 'personal', 'family')", name='ck_tenant_valid_names'),
        sa.CheckConstraint("char_length(name) >= 3", name='ck_tenant_name_length'),
        sa.CheckConstraint("char_length(display_name) >= 1", name='ck_tenant_display_name_not_empty'),

        comment="Tenant context definition and configuration"
    )

    # Create indexes for tenants
    op.create_index('idx_tenants_name', 'tenants', ['name'])
    op.create_index('idx_tenants_owner', 'tenants', ['owner_email'])
    op.create_index('idx_tenants_active', 'tenants', ['is_active', 'created_at'])

    # Create tenant_sessions table
    op.create_table(
        'tenant_sessions',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True, nullable=False),
        sa.Column('session_id', sa.String(length=255), nullable=False, unique=True),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id', ondelete='CASCADE'), nullable=False),
        sa.Column('user_email', sa.String(length=254), nullable=False),
        sa.Column('last_activity', sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column('expires_at', sa.DateTime(timezone=True), nullable=False,
                  server_default=text("NOW() + INTERVAL '24 hours'")),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),

        comment="Active tenant context tracking for user sessions"
    )

    # Create indexes for tenant_sessions
    op.create_index('idx_session_tenant_user', 'tenant_sessions', ['tenant_id', 'user_email'])
    op.create_index('idx_session_activity', 'tenant_sessions', ['last_activity'])
    op.create_index('idx_session_expires', 'tenant_sessions', ['expires_at'])

    # Create cross_tenant_person_links table
    op.create_table(
        'cross_tenant_person_links',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True, nullable=False),
        sa.Column('canonical_person_id', sa.String(length=100), nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), sa.ForeignKey('tenants.id', ondelete='CASCADE'), nullable=False),
        sa.Column('person_id', BIGINT(), sa.ForeignKey('people.id', ondelete='CASCADE'), nullable=False),
        sa.Column('link_confidence', sa.DECIMAL(precision=4, scale=3), nullable=False, server_default='1.000'),
        sa.Column('link_method', sa.String(length=50), nullable=False, server_default='manual'),
        sa.Column('validated_by', sa.String(length=254), nullable=True),
        sa.Column('validated_at', sa.DateTime(timezone=True), nullable=True),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),

        sa.UniqueConstraint('tenant_id', 'person_id', name='uc_tenant_person_link'),
        sa.CheckConstraint("link_confidence >= 0.000 AND link_confidence <= 1.000", name='ck_link_confidence_range'),
        sa.CheckConstraint("link_method IN ('manual', 'email_match', 'ai_suggested', 'name_similarity')",
                          name='ck_link_method_valid'),

        comment="Links person entities across different tenants"
    )

    # Create indexes for cross_tenant_person_links
    op.create_index('idx_person_links_canonical', 'cross_tenant_person_links', ['canonical_person_id'])
    op.create_index('idx_person_links_tenant_person', 'cross_tenant_person_links', ['tenant_id', 'person_id'])
    op.create_index('idx_person_links_confidence', 'cross_tenant_person_links', ['link_confidence'])

    # Enable Row-Level Security on all tenant-aware tables
    tenant_tables = [
        'sources', 'assertions', 'people', 'projects', 'teams',
        'processing_events', 'processing_jobs', 'processing_results', 'embeddings'
    ]

    for table_name in tenant_tables:
        # Enable RLS
        op.execute(f"ALTER TABLE {table_name} ENABLE ROW LEVEL SECURITY")

        # Create policy for tenant isolation
        # This policy allows users to see only their tenant's data when app.current_tenant_id is set
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

        # Create policy for superuser access (when no tenant context is set)
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

    # Create function to set tenant context
    op.execute("""
        CREATE OR REPLACE FUNCTION set_tenant_context(tenant_uuid UUID)
        RETURNS void AS $$
        BEGIN
            PERFORM set_config('app.current_tenant_id', tenant_uuid::text, false);
        END;
        $$ LANGUAGE plpgsql SECURITY DEFINER;
    """)

    # Create function to clear tenant context
    op.execute("""
        CREATE OR REPLACE FUNCTION clear_tenant_context()
        RETURNS void AS $$
        BEGIN
            PERFORM set_config('app.current_tenant_id', '', false);
        END;
        $$ LANGUAGE plpgsql SECURITY DEFINER;
    """)

    # Create function to get current tenant context
    op.execute("""
        CREATE OR REPLACE FUNCTION get_current_tenant_id()
        RETURNS UUID AS $$
        BEGIN
            RETURN COALESCE(
                current_setting('app.current_tenant_id', true)::uuid,
                NULL
            );
        END;
        $$ LANGUAGE plpgsql SECURITY DEFINER;
    """)

    # Create helper function for safe tenant switching
    op.execute("""
        CREATE OR REPLACE FUNCTION switch_tenant_safely(
            new_tenant_uuid UUID,
            user_email TEXT
        )
        RETURNS boolean AS $$
        DECLARE
            tenant_exists boolean;
        BEGIN
            -- Check if tenant exists and is active
            SELECT EXISTS(
                SELECT 1 FROM tenants
                WHERE id = new_tenant_uuid
                AND is_active = true
                AND is_deleted = false
            ) INTO tenant_exists;

            IF NOT tenant_exists THEN
                RAISE EXCEPTION 'Tenant % does not exist or is not active', new_tenant_uuid;
                RETURN false;
            END IF;

            -- Set tenant context
            PERFORM set_tenant_context(new_tenant_uuid);

            -- Update or create session record
            INSERT INTO tenant_sessions (session_id, tenant_id, user_email, last_activity, expires_at)
            VALUES (
                COALESCE(current_setting('app.session_id', true), gen_random_uuid()::text),
                new_tenant_uuid,
                user_email,
                NOW(),
                NOW() + INTERVAL '24 hours'
            )
            ON CONFLICT (session_id) DO UPDATE SET
                tenant_id = EXCLUDED.tenant_id,
                last_activity = EXCLUDED.last_activity,
                expires_at = EXCLUDED.expires_at,
                updated_at = NOW();

            RETURN true;
        END;
        $$ LANGUAGE plpgsql SECURITY DEFINER;
    """)

    # Create default tenants for initial setup
    op.execute("""
        INSERT INTO tenants (id, name, display_name, description, owner_email) VALUES
        ('00000001-0000-0000-0000-000000000001', 'work', 'Work', 'Professional work context', 'user@example.com'),
        ('00000002-0000-0000-0000-000000000002', 'personal', 'Personal', 'Personal life context', 'user@example.com')
        ON CONFLICT (name) DO NOTHING;
    """)

    # Grant necessary permissions
    op.execute("GRANT EXECUTE ON FUNCTION set_tenant_context(UUID) TO PUBLIC")
    op.execute("GRANT EXECUTE ON FUNCTION clear_tenant_context() TO PUBLIC")
    op.execute("GRANT EXECUTE ON FUNCTION get_current_tenant_id() TO PUBLIC")
    op.execute("GRANT EXECUTE ON FUNCTION switch_tenant_safely(UUID, TEXT) TO PUBLIC")


def downgrade() -> None:
    """Downgrade database schema by removing tenant management and RLS policies."""

    # Remove RLS policies and disable RLS
    tenant_tables = [
        'sources', 'assertions', 'people', 'projects', 'teams',
        'processing_events', 'processing_jobs', 'processing_results', 'embeddings'
    ]

    for table_name in tenant_tables:
        op.execute(f"DROP POLICY IF EXISTS tenant_isolation_policy ON {table_name}")
        op.execute(f"DROP POLICY IF EXISTS superuser_access_policy ON {table_name}")
        op.execute(f"ALTER TABLE {table_name} DISABLE ROW LEVEL SECURITY")

    # Drop functions
    op.execute("DROP FUNCTION IF EXISTS switch_tenant_safely(UUID, TEXT)")
    op.execute("DROP FUNCTION IF EXISTS get_current_tenant_id()")
    op.execute("DROP FUNCTION IF EXISTS clear_tenant_context()")
    op.execute("DROP FUNCTION IF EXISTS set_tenant_context(UUID)")

    # Drop tenant management tables
    op.drop_table('cross_tenant_person_links')
    op.drop_table('tenant_sessions')
    op.drop_table('tenants')