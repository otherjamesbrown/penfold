-- +goose Up
-- Pipeline routing table: data-driven routing from classification to pipeline stages.
-- Items with no matching route skip all pipelines (zero processing cost).
-- Part of epic pf-778226, task pf-47a0bb.

CREATE TABLE pipeline_routing (
  id SERIAL PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  content_type VARCHAR(20),          -- NULL = any content type
  content_subtype VARCHAR(20),       -- NULL = any subtype
  pipeline VARCHAR(50) NOT NULL,     -- pipeline identifier (e.g. "standard", "attendees_only")
  active BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_pipeline_routing_tenant ON pipeline_routing (tenant_id, active);
CREATE INDEX idx_pipeline_routing_lookup ON pipeline_routing (tenant_id, content_type, content_subtype, active);

-- Seed default routing for the first tenant.
DO $$
DECLARE
  t_id UUID;
BEGIN
  SELECT id INTO t_id FROM tenants LIMIT 1;
  IF t_id IS NULL THEN
    RAISE EXCEPTION 'No tenant found for seeding pipeline routing';
  END IF;

  -- Human emails → standard pipeline (full processing)
  INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline)
  VALUES (t_id, 'EMAIL', 'HUMAN', 'standard');

  -- Notification emails → standard pipeline
  INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline)
  VALUES (t_id, 'EMAIL', 'NOTIFICATION', 'standard');

  -- Calendar invites → attendees_only pipeline (extract attendees, no triage LLM)
  INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline)
  VALUES (t_id, 'CALENDAR', 'INVITE', 'attendees_only');

  -- Meeting transcripts → standard pipeline
  INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline)
  VALUES (t_id, 'MEETING', 'TRANSCRIPT', 'standard');

  -- No rows for: EMAIL/AUTO_REPLY, CALENDAR/CANCELLATION, CALENDAR/UPDATE, CALENDAR/RESPONSE
  -- These are skipped (no pipeline processing).
END $$;

-- +goose Down
DROP TABLE IF EXISTS pipeline_routing;
