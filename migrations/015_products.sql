-- =====================================================
-- Products & Organizational Entities Migration
-- Created: 2026-01-20
-- Description: Tables for products, product-team associations, scoped roles, and timeline events
-- Specification: specs/014-products/data-model.md
-- Epic: pe-i68n
-- =====================================================

-- =====================================================
-- Extend People Table
-- =====================================================

-- Add country field to people table
ALTER TABLE people ADD COLUMN IF NOT EXISTS country VARCHAR(100);

-- Index for country-based queries
CREATE INDEX IF NOT EXISTS idx_people_country ON people (tenant_id, country) WHERE country IS NOT NULL;

COMMENT ON COLUMN people.country IS 'ISO country name for geographic queries (e.g., Poland, United States)';

-- =====================================================
-- Products Tables
-- =====================================================

-- Enum for product types
DO $$ BEGIN
    CREATE TYPE product_type AS ENUM ('product', 'sub_product', 'feature');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for product status
DO $$ BEGIN
    CREATE TYPE product_status AS ENUM ('active', 'beta', 'sunset', 'deprecated');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Main products table with hierarchy support
CREATE TABLE IF NOT EXISTS products (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Product identity
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Hierarchy
    parent_id BIGINT REFERENCES products(id) ON DELETE SET NULL,
    product_type product_type NOT NULL DEFAULT 'product',

    -- Status
    status product_status NOT NULL DEFAULT 'active',

    -- Search keywords
    keywords TEXT[] DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT products_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT products_tenant_name_unique UNIQUE (tenant_id, name),
    -- Top-level products must have no parent, sub-products/features must have parent
    CONSTRAINT products_hierarchy_valid CHECK (
        (parent_id IS NULL AND product_type = 'product') OR
        (parent_id IS NOT NULL AND product_type IN ('sub_product', 'feature'))
    )
);

-- Indexes for products
CREATE INDEX IF NOT EXISTS idx_products_tenant_id ON products (tenant_id);
CREATE INDEX IF NOT EXISTS idx_products_parent ON products (parent_id);
CREATE INDEX IF NOT EXISTS idx_products_type ON products (tenant_id, product_type);
CREATE INDEX IF NOT EXISTS idx_products_status ON products (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_products_keywords ON products USING GIN (keywords);
CREATE INDEX IF NOT EXISTS idx_products_name_lower ON products (tenant_id, LOWER(name));

-- =====================================================
-- Product Aliases Table
-- =====================================================

-- Alternative names for products (e.g., "LK Enterprise" -> "LKE Enterprise")
CREATE TABLE IF NOT EXISTS product_aliases (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,

    -- Alias value
    alias VARCHAR(255) NOT NULL,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT product_aliases_not_empty CHECK (length(trim(alias)) > 0)
);

-- Indexes for product_aliases
CREATE INDEX IF NOT EXISTS idx_product_aliases_product ON product_aliases (product_id);
-- Aliases must be globally unique to avoid ambiguous resolution
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_aliases_lookup ON product_aliases (LOWER(alias));

-- =====================================================
-- Product Teams Table
-- =====================================================

-- Associates teams with products with context labels
CREATE TABLE IF NOT EXISTS product_teams (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    team_id BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- Team context (e.g., "Core Team", "DRI Team", "Engineering")
    context VARCHAR(100),

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for product_teams
CREATE INDEX IF NOT EXISTS idx_product_teams_product ON product_teams (product_id);
CREATE INDEX IF NOT EXISTS idx_product_teams_team ON product_teams (team_id);
CREATE INDEX IF NOT EXISTS idx_product_teams_tenant ON product_teams (tenant_id);
-- Same team can have multiple contexts on same product
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_teams_unique ON product_teams (product_id, team_id, COALESCE(context, ''));

-- =====================================================
-- Product Team Roles Table
-- =====================================================

-- Scoped role assignments: person has role X in context of product Y through team Z
CREATE TABLE IF NOT EXISTS product_team_roles (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    product_team_id BIGINT NOT NULL REFERENCES product_teams(id) ON DELETE CASCADE,
    person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,

    -- Role information
    role VARCHAR(100) NOT NULL,
    scope VARCHAR(100), -- e.g., "Networking", "Database", "Security"

    -- Active status with history
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT product_team_roles_role_not_empty CHECK (length(trim(role)) > 0),
    CONSTRAINT product_team_roles_active_consistency CHECK (
        (is_active = TRUE AND ended_at IS NULL) OR
        (is_active = FALSE AND ended_at IS NOT NULL)
    )
);

-- Indexes for product_team_roles
CREATE INDEX IF NOT EXISTS idx_product_team_roles_product_team ON product_team_roles (product_team_id);
CREATE INDEX IF NOT EXISTS idx_product_team_roles_person ON product_team_roles (person_id);
CREATE INDEX IF NOT EXISTS idx_product_team_roles_tenant ON product_team_roles (tenant_id);
CREATE INDEX IF NOT EXISTS idx_product_team_roles_active ON product_team_roles (tenant_id, is_active) WHERE is_active = TRUE;
CREATE INDEX IF NOT EXISTS idx_product_team_roles_lookup ON product_team_roles (product_team_id, role, scope) WHERE is_active = TRUE;
-- Index for "who is DRI for X" queries
CREATE INDEX IF NOT EXISTS idx_product_team_roles_role ON product_team_roles (tenant_id, role) WHERE is_active = TRUE;

-- =====================================================
-- Product Events Table (Timeline)
-- =====================================================

-- Enum for event types
DO $$ BEGIN
    CREATE TYPE product_event_type AS ENUM (
        'decision',     -- Internal decision
        'milestone',    -- Project milestone
        'risk',         -- Risk identified
        'release',      -- Product release
        'competitor',   -- Competitor action
        'org_change',   -- Organizational change
        'market',       -- Market/geopolitical event
        'note'          -- General note
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for event visibility
DO $$ BEGIN
    CREATE TYPE event_visibility AS ENUM ('internal', 'external');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for event source type
DO $$ BEGIN
    CREATE TYPE event_source_type AS ENUM ('manual', 'derived');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Timeline events for products
CREATE TABLE IF NOT EXISTS product_events (
    id BIGSERIAL PRIMARY KEY,
    event_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(255) NOT NULL,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,

    -- Event classification
    event_type product_event_type NOT NULL,
    visibility event_visibility NOT NULL DEFAULT 'internal',
    source_type event_source_type NOT NULL DEFAULT 'manual',

    -- Event content
    title VARCHAR(500) NOT NULL,
    description TEXT,

    -- Timing
    occurred_at TIMESTAMPTZ NOT NULL,

    -- Provenance
    recorded_by VARCHAR(255),

    -- Additional structured data
    metadata JSONB DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT product_events_title_not_empty CHECK (length(trim(title)) > 0),
    CONSTRAINT product_events_uuid_unique UNIQUE (event_uuid)
);

-- Indexes for product_events
CREATE INDEX IF NOT EXISTS idx_product_events_product ON product_events (product_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_product_events_tenant ON product_events (tenant_id);
CREATE INDEX IF NOT EXISTS idx_product_events_type ON product_events (tenant_id, event_type);
CREATE INDEX IF NOT EXISTS idx_product_events_visibility ON product_events (tenant_id, visibility);
CREATE INDEX IF NOT EXISTS idx_product_events_occurred ON product_events (tenant_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_product_events_uuid ON product_events (event_uuid);
-- Index for "external events" queries
CREATE INDEX IF NOT EXISTS idx_product_events_external ON product_events (tenant_id, product_id, occurred_at DESC) WHERE visibility = 'external';

-- =====================================================
-- Product Event Links Table
-- =====================================================

-- Links product events to other entities (meetings, emails, documents)
CREATE TABLE IF NOT EXISTS product_event_links (
    id BIGSERIAL PRIMARY KEY,
    event_id BIGINT NOT NULL REFERENCES product_events(id) ON DELETE CASCADE,

    -- Linked entity reference
    linked_entity_type VARCHAR(50) NOT NULL,
    linked_entity_id BIGINT NOT NULL,

    -- Link type
    link_type VARCHAR(50) NOT NULL,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT product_event_links_entity_type_valid CHECK (
        linked_entity_type IN ('meeting', 'email', 'document', 'source')
    ),
    CONSTRAINT product_event_links_link_type_valid CHECK (
        link_type IN ('source', 'reference', 'follow_up')
    ),
    -- Each entity can only be linked once per event
    CONSTRAINT product_event_links_unique UNIQUE (event_id, linked_entity_type, linked_entity_id)
);

-- Indexes for product_event_links
CREATE INDEX IF NOT EXISTS idx_product_event_links_event ON product_event_links (event_id);
CREATE INDEX IF NOT EXISTS idx_product_event_links_entity ON product_event_links (linked_entity_type, linked_entity_id);

-- =====================================================
-- Triggers for updated_at
-- =====================================================

-- Products updated_at trigger
CREATE OR REPLACE FUNCTION update_products_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_products_updated_at ON products;
CREATE TRIGGER trg_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW
    EXECUTE FUNCTION update_products_updated_at();

-- Product teams updated_at trigger
DROP TRIGGER IF EXISTS trg_product_teams_updated_at ON product_teams;
CREATE TRIGGER trg_product_teams_updated_at
    BEFORE UPDATE ON product_teams
    FOR EACH ROW
    EXECUTE FUNCTION update_products_updated_at();

-- Product team roles updated_at trigger
DROP TRIGGER IF EXISTS trg_product_team_roles_updated_at ON product_team_roles;
CREATE TRIGGER trg_product_team_roles_updated_at
    BEFORE UPDATE ON product_team_roles
    FOR EACH ROW
    EXECUTE FUNCTION update_products_updated_at();

-- Product events updated_at trigger
DROP TRIGGER IF EXISTS trg_product_events_updated_at ON product_events;
CREATE TRIGGER trg_product_events_updated_at
    BEFORE UPDATE ON product_events
    FOR EACH ROW
    EXECUTE FUNCTION update_products_updated_at();

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE products IS 'Business products with 3-level hierarchy (product -> sub_product -> feature)';
COMMENT ON COLUMN products.name IS 'Product name (e.g., LKE Enterprise)';
COMMENT ON COLUMN products.parent_id IS 'Parent product for hierarchy (NULL for top-level products)';
COMMENT ON COLUMN products.product_type IS 'Type: product (top-level), sub_product, or feature';
COMMENT ON COLUMN products.status IS 'Lifecycle status: active, beta, sunset, deprecated';
COMMENT ON COLUMN products.keywords IS 'Search keywords for product discovery';

COMMENT ON TABLE product_aliases IS 'Alternative names for products (e.g., LK Enterprise -> LKE Enterprise)';
COMMENT ON COLUMN product_aliases.alias IS 'Alternative name that resolves to the product';

COMMENT ON TABLE product_teams IS 'Associates teams with products with optional context labels';
COMMENT ON COLUMN product_teams.context IS 'Team context (e.g., Core Team, DRI Team, Engineering)';

COMMENT ON TABLE product_team_roles IS 'Scoped role assignments: person has role in context of product through team';
COMMENT ON COLUMN product_team_roles.role IS 'Role name (e.g., DRI, Manager, Lead, Contributor)';
COMMENT ON COLUMN product_team_roles.scope IS 'Role scope within product (e.g., Networking, Database, Security)';
COMMENT ON COLUMN product_team_roles.is_active IS 'Whether role is currently active (FALSE preserves history)';

COMMENT ON TABLE product_events IS 'Timeline events for products: decisions, milestones, competitor moves, etc.';
COMMENT ON COLUMN product_events.event_type IS 'Event type: decision, milestone, risk, release, competitor, org_change, market, note';
COMMENT ON COLUMN product_events.visibility IS 'internal (our events) or external (competitor/market events)';
COMMENT ON COLUMN product_events.source_type IS 'manual (user entered) or derived (extracted from content)';
COMMENT ON COLUMN product_events.occurred_at IS 'When the event actually occurred (not when it was recorded)';
COMMENT ON COLUMN product_events.metadata IS 'Additional structured data (JSON)';

COMMENT ON TABLE product_event_links IS 'Links product events to source content (meetings, emails, documents)';
COMMENT ON COLUMN product_event_links.linked_entity_type IS 'Type of linked entity: meeting, email, document, source';
COMMENT ON COLUMN product_event_links.link_type IS 'Relationship: source (event came from this), reference, follow_up';
