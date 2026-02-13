-- =====================================================
-- Entity Resolution Tables Migration
-- Created: 2026-01-19
-- Description: Tables for people, teams, and projects entity resolution
-- Specification: specs/013-content-enrichment/entity-resolution.md
-- =====================================================

-- Enum for account types
DO $$ BEGIN
    CREATE TYPE account_type AS ENUM ('person', 'role', 'distribution', 'bot', 'external_service');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for alias types
DO $$ BEGIN
    CREATE TYPE alias_type AS ENUM ('email', 'slack_id', 'name', 'display_name');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- =====================================================
-- People Tables
-- =====================================================

-- Main people table
CREATE TABLE IF NOT EXISTS people (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Identity
    canonical_name VARCHAR(500) NOT NULL,
    primary_email VARCHAR(500) NOT NULL,

    -- Metadata
    job_title VARCHAR(255),
    department VARCHAR(255),

    -- Classification
    is_internal BOOLEAN DEFAULT FALSE,
    account_type account_type NOT NULL DEFAULT 'person',

    -- Review status
    confidence REAL NOT NULL DEFAULT 0.6 CHECK (confidence >= 0.0 AND confidence <= 1.0),
    needs_review BOOLEAN DEFAULT TRUE,
    auto_created BOOLEAN DEFAULT TRUE,
    reviewed_at TIMESTAMPTZ,
    reviewed_by VARCHAR(255),

    -- Duplicate tracking
    potential_duplicates BIGINT[] DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT people_primary_email_not_empty CHECK (length(trim(primary_email)) > 0),
    CONSTRAINT people_canonical_name_not_empty CHECK (length(trim(canonical_name)) > 0),
    CONSTRAINT people_tenant_email_unique UNIQUE (tenant_id, primary_email)
);

-- Indexes for people
CREATE INDEX IF NOT EXISTS idx_people_tenant_id ON people (tenant_id);
CREATE INDEX IF NOT EXISTS idx_people_primary_email ON people (primary_email);
CREATE INDEX IF NOT EXISTS idx_people_canonical_name ON people (tenant_id, canonical_name);
CREATE INDEX IF NOT EXISTS idx_people_needs_review ON people (tenant_id, needs_review) WHERE needs_review = TRUE;
CREATE INDEX IF NOT EXISTS idx_people_account_type ON people (tenant_id, account_type);
CREATE INDEX IF NOT EXISTS idx_people_is_internal ON people (tenant_id, is_internal);

-- Person aliases table
CREATE TABLE IF NOT EXISTS person_aliases (
    id BIGSERIAL PRIMARY KEY,
    person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,

    -- Alias data
    alias_type alias_type NOT NULL,
    alias_value VARCHAR(500) NOT NULL,

    -- Confidence and provenance
    confidence REAL NOT NULL DEFAULT 1.0 CHECK (confidence >= 0.0 AND confidence <= 1.0),
    source VARCHAR(100) NOT NULL DEFAULT 'auto_created',

    -- Timestamps
    discovered_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT person_aliases_value_not_empty CHECK (length(trim(alias_value)) > 0)
);

-- Indexes for person_aliases
CREATE INDEX IF NOT EXISTS idx_person_aliases_person_id ON person_aliases (person_id);
CREATE INDEX IF NOT EXISTS idx_person_aliases_lookup ON person_aliases (alias_type, alias_value);
CREATE INDEX IF NOT EXISTS idx_person_aliases_value ON person_aliases (alias_value);

-- =====================================================
-- Teams Tables
-- =====================================================

-- Teams table
CREATE TABLE IF NOT EXISTS teams (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Team info
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT teams_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT teams_tenant_name_unique UNIQUE (tenant_id, name)
);

-- Indexes for teams
CREATE INDEX IF NOT EXISTS idx_teams_tenant_id ON teams (tenant_id);
CREATE INDEX IF NOT EXISTS idx_teams_name ON teams (tenant_id, name);

-- Team members table
CREATE TABLE IF NOT EXISTS team_members (
    id BIGSERIAL PRIMARY KEY,
    team_id BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,

    -- Role
    role VARCHAR(100) DEFAULT 'member',

    -- Timestamps
    joined_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT team_members_unique UNIQUE (team_id, person_id)
);

-- Indexes for team_members
CREATE INDEX IF NOT EXISTS idx_team_members_team_id ON team_members (team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_person_id ON team_members (person_id);

-- =====================================================
-- Projects Tables
-- =====================================================

-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Project info
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Matching configuration
    keywords TEXT[] DEFAULT '{}',
    jira_projects TEXT[] DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT projects_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT projects_tenant_name_unique UNIQUE (tenant_id, name)
);

-- Indexes for projects
CREATE INDEX IF NOT EXISTS idx_projects_tenant_id ON projects (tenant_id);
CREATE INDEX IF NOT EXISTS idx_projects_name ON projects (tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_projects_jira ON projects USING GIN (jira_projects);
CREATE INDEX IF NOT EXISTS idx_projects_keywords ON projects USING GIN (keywords);

-- Project members table (individuals)
CREATE TABLE IF NOT EXISTS project_members (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    person_id BIGINT REFERENCES people(id) ON DELETE CASCADE,
    team_id BIGINT REFERENCES teams(id) ON DELETE CASCADE,

    -- Role
    role VARCHAR(100) DEFAULT 'contributor',

    -- Timestamps
    added_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints: must have either person_id or team_id
    CONSTRAINT project_members_has_member CHECK (person_id IS NOT NULL OR team_id IS NOT NULL),
    CONSTRAINT project_members_unique_person UNIQUE (project_id, person_id),
    CONSTRAINT project_members_unique_team UNIQUE (project_id, team_id)
);

-- Indexes for project_members
CREATE INDEX IF NOT EXISTS idx_project_members_project_id ON project_members (project_id);
CREATE INDEX IF NOT EXISTS idx_project_members_person_id ON project_members (person_id);
CREATE INDEX IF NOT EXISTS idx_project_members_team_id ON project_members (team_id);

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE people IS 'Canonical person records resolved from various identifiers';
COMMENT ON COLUMN people.canonical_name IS 'Normalized display name (Last, First → First Last)';
COMMENT ON COLUMN people.is_internal IS 'True if person is internal based on email domain matching';
COMMENT ON COLUMN people.account_type IS 'Type of account: person, role, distribution, bot, external_service';
COMMENT ON COLUMN people.confidence IS 'Confidence score 0.0-1.0, higher = more trusted';
COMMENT ON COLUMN people.needs_review IS 'True if person record should be reviewed by user';
COMMENT ON COLUMN people.potential_duplicates IS 'Array of person IDs that may be duplicates of this person';

COMMENT ON TABLE person_aliases IS 'Aliases (email, slack_id, names) that resolve to a person';
COMMENT ON COLUMN person_aliases.alias_type IS 'Type of alias: email, slack_id, name, display_name';
COMMENT ON COLUMN person_aliases.source IS 'Where this alias was discovered (auto_created, gmail_api, manual)';

COMMENT ON TABLE teams IS 'Manually defined teams for grouping people';
COMMENT ON TABLE team_members IS 'Team membership with optional role';

COMMENT ON TABLE projects IS 'Projects with keywords and Jira integration for auto-tagging';
COMMENT ON COLUMN projects.keywords IS 'Keywords for matching content to this project';
COMMENT ON COLUMN projects.jira_projects IS 'Jira project keys linked to this project';
COMMENT ON TABLE project_members IS 'Project membership - can be individuals or teams';
