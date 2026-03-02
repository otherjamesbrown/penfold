-- Migration 100: Entity-Content Role Associations (Phase 1)
-- Adds participation roles to content mentions and group membership model.
-- Part of pf-400b05 (Entity-Content Role Associations).

-- Add participation_role column to content_mentions table.
-- Values map to ParticipationRole enum:
--   0=UNSPECIFIED, 1=FROM, 2=TO, 3=CC, 4=BCC, 5=MENTIONED,
--   6=ORGANIZER, 7=ATTENDEE, 8=SPEAKER, 9=GROUP_MEMBER
ALTER TABLE content_mentions ADD COLUMN IF NOT EXISTS participation_role smallint NOT NULL DEFAULT 0;

-- Add via_group_entity_id for group membership attribution.
-- When participation_role=GROUP_MEMBER(9), this points to the distribution list
-- entity that caused this mention to be created.
ALTER TABLE content_mentions ADD COLUMN IF NOT EXISTS via_group_entity_id bigint REFERENCES people(id);

-- Index for role-based queries (e.g. "find all FROM mentions for tenant")
CREATE INDEX IF NOT EXISTS idx_content_mentions_participation_role
  ON content_mentions(tenant_id, participation_role);

-- Composite index for "find all content where entity X is a participant with role Y"
CREATE INDEX IF NOT EXISTS idx_content_mentions_entity_role
  ON content_mentions(tenant_id, resolved_entity_id, participation_role)
  WHERE resolved_entity_id IS NOT NULL;

-- Group membership table: tracks which entities belong to distribution lists.
-- Separate from team_members (org structure) — this is for email routing attribution.
CREATE TABLE IF NOT EXISTS entity_group_members (
  id bigserial PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  group_entity_id bigint NOT NULL REFERENCES people(id),
  member_entity_id bigint NOT NULL REFERENCES people(id),
  added_at timestamptz NOT NULL DEFAULT now(),
  removed_at timestamptz,
  source text,
  UNIQUE(tenant_id, group_entity_id, member_entity_id, added_at)
);

-- Active members of a group
CREATE INDEX IF NOT EXISTS idx_group_members_group
  ON entity_group_members(tenant_id, group_entity_id)
  WHERE removed_at IS NULL;

-- Groups that a member belongs to
CREATE INDEX IF NOT EXISTS idx_group_members_member
  ON entity_group_members(tenant_id, member_entity_id)
  WHERE removed_at IS NULL;
