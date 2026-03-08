-- +goose Up

-- Migration 140: Notification enrichment pipeline
-- Adds notification_extract stage to replace the generic summarize/extract_ner/extract_semantic
-- stages for the notification pipeline. The new stage produces structured JSON stored in
-- content_enrichment.extracted_data['notification'].
-- Post-migration notification pipeline: triage(10) → notification_extract(25) → persist(50) → embed(60)

BEGIN;

-- Register notification_extract pipeline stage.
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES ('notification_extract', 'Notification Extract', 'Structured extraction for notification emails.', 'llm', true, true, ARRAY[]::text[], ARRAY[]::text[])
ON CONFLICT (stage) DO NOTHING;

-- Prompt template for notification_extract stage.
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('notification_extract', 1,
  E'You are extracting structured data from an email notification.\n\nAnalyse the following notification email and extract structured information.\n\n**Email content:**\n{{.Content}}\n\nReturn a JSON object with these fields:\n- source_system (string): The system that sent this notification (e.g., "GitHub", "Jira", "Google Docs", "Google Calendar", "Slack", "Linear", "AWS", "Datadog"). Infer from sender address and content.\n- notification_type (string): Category of notification (e.g., "PR review", "issue update", "document share", "calendar invite", "deployment alert", "mention")\n- summary (string): One-sentence summary of what happened, written for a human reading a daily digest\n- action_required (boolean): Whether this notification requires James to do something\n- action (string or null): If action_required is true, what specific action is needed\n- urgency (string): "high" (needs attention today), "medium" (this week), "low" (FYI only)\n- people (array): People mentioned, each with "name" and "role" (e.g., "author", "reviewer", "assignee")\n- project (string or null): Project or repository name if identifiable\n\nReturn ONLY valid JSON. No markdown, no explanation.',
  'Notification structured extraction v1',
  true, 'agent-mycroft')
ON CONFLICT (stage, version) DO NOTHING;

-- AI routing rule for notification_extract stage.
INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES ('notification-extract-default', 'notification_extract', '{gemini/gemini-2.5-flash}', '{gemini/gemini-2.0-flash}', 'cost', 0, true, '{}')
ON CONFLICT (name) DO NOTHING;

-- Remove old generic stages from notification pipeline.
DELETE FROM pipeline_definitions
WHERE pipeline = 'notification' AND stage IN ('summarize', 'extract_ner', 'extract_semantic');

-- Add notification_extract stage for all tenants that have the notification pipeline.
DO $$
DECLARE t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT DISTINCT tenant_id FROM pipeline_definitions WHERE pipeline = 'notification' LOOP
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override)
    VALUES (t_rec.tenant_id, 'notification', 'notification_extract', 25, true, false, false, 120, 'gemini-2.5-flash')
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;
  END LOOP;
END $$;

-- After migration, notification pipeline is: triage(10) → notification_extract(25) → persist(50) → embed(60)

COMMIT;

-- +goose Down

BEGIN;

DELETE FROM pipeline_definitions
WHERE pipeline = 'notification' AND stage = 'notification_extract';

-- Restore removed stages for all tenants that have the notification pipeline.
DO $$
DECLARE t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT DISTINCT tenant_id FROM pipeline_definitions WHERE pipeline = 'notification' LOOP
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override)
    VALUES
      (t_rec.tenant_id, 'notification', 'summarize',       15, true, false, false,  60, NULL),
      (t_rec.tenant_id, 'notification', 'extract_ner',     20, true, false, false,  60, NULL),
      (t_rec.tenant_id, 'notification', 'extract_semantic', 25, true, false, false, 120, 'gemini-2.5-flash')
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;
  END LOOP;
END $$;

DELETE FROM ai_routing_rules WHERE name = 'notification-extract-default';

DELETE FROM prompt_templates WHERE stage = 'notification_extract' AND version = 1;

DELETE FROM pipeline_stages WHERE stage = 'notification_extract';

COMMIT;
