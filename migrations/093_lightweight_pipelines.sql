-- Migration 093: Lightweight pipeline definitions and routing rules
-- Author: agent-mycroft
-- Date: 2026-02-28
-- Shard: pf-cd2269 (Phase 2: Pipeline definitions + routing rules)
--
-- Creates purpose-built pipelines for notification, newsletter, and auto-reply
-- content, and routes classified subtypes to them. This reduces processing cost
-- by running only the stages each content type needs.

BEGIN;

-- Seed lightweight pipeline definitions for ALL tenants.
DO $$
DECLARE
  t_id UUID;
  t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT id FROM tenants LOOP
    t_id := t_rec.id;

    -- notification pipeline (5 stages):
    -- For Jira, Aha, Google Docs, Smartsheet, IT, security notifications.
    -- Extract who/what is mentioned (NER), make searchable. Skip deep analysis.
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override)
    VALUES
      (t_id, 'notification', 'parse',       0, true, false, false,  30, NULL),
      (t_id, 'notification', 'triage',      1, true, false, false,  60, NULL),
      (t_id, 'notification', 'extract_ner', 2, true, false, false, 120, 'gemini-2.5-flash'),
      (t_id, 'notification', 'persist',     3, true, false, false,  60, NULL),
      (t_id, 'notification', 'embed',       4, true, false, false,  60, NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- newsletter pipeline (5 stages):
    -- For company newsletters, marketing, broadcasts.
    -- Extract key announcements (semantic). Skip NER (newsletter "participants" aren't meaningful).
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override)
    VALUES
      (t_id, 'newsletter', 'parse',            0, true, false, false,  30, NULL),
      (t_id, 'newsletter', 'triage',           1, true, false, false,  60, NULL),
      (t_id, 'newsletter', 'extract_semantic', 2, true, false, false, 120, 'gemini-2.5-flash'),
      (t_id, 'newsletter', 'persist',          3, true, false, false,  60, NULL),
      (t_id, 'newsletter', 'embed',            4, true, false, false,  60, NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- auto_reply pipeline (3 stages):
    -- For OOFs, automatic responses.
    -- Record existence, classify, don't extract.
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds)
    VALUES
      (t_id, 'auto_reply', 'parse',  0, true, false, false, 30),
      (t_id, 'auto_reply', 'triage', 1, true, false, false, 60),
      (t_id, 'auto_reply', 'embed',  2, true, false, false, 60)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- Update routing: EMAIL/NOTIFICATION → notification (was: standard)
    UPDATE pipeline_routing
    SET pipeline = 'notification'
    WHERE tenant_id = t_id
      AND content_type = 'EMAIL'
      AND content_subtype = 'NOTIFICATION'
      AND pipeline = 'standard';

    -- Insert routing: EMAIL/NEWSLETTER → newsletter
    INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline)
    SELECT t_id, 'EMAIL', 'NEWSLETTER', 'newsletter'
    WHERE NOT EXISTS (
      SELECT 1 FROM pipeline_routing
      WHERE tenant_id = t_id AND content_type = 'EMAIL' AND content_subtype = 'NEWSLETTER'
    );

    -- Insert routing: EMAIL/AUTO_REPLY → auto_reply
    INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline)
    SELECT t_id, 'EMAIL', 'AUTO_REPLY', 'auto_reply'
    WHERE NOT EXISTS (
      SELECT 1 FROM pipeline_routing
      WHERE tenant_id = t_id AND content_type = 'EMAIL' AND content_subtype = 'AUTO_REPLY'
    );

    -- Insert routing: EMAIL/CALENDAR_RESPONSE → attendees_only
    INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline)
    SELECT t_id, 'EMAIL', 'CALENDAR_RESPONSE', 'attendees_only'
    WHERE NOT EXISTS (
      SELECT 1 FROM pipeline_routing
      WHERE tenant_id = t_id AND content_type = 'EMAIL' AND content_subtype = 'CALENDAR_RESPONSE'
    );

    -- Insert routing: EMAIL/CALENDAR_UPDATE → attendees_only
    INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline)
    SELECT t_id, 'EMAIL', 'CALENDAR_UPDATE', 'attendees_only'
    WHERE NOT EXISTS (
      SELECT 1 FROM pipeline_routing
      WHERE tenant_id = t_id AND content_type = 'EMAIL' AND content_subtype = 'CALENDAR_UPDATE'
    );

    -- Insert routing: CALENDAR/CANCELLATION → attendees_only
    INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline)
    SELECT t_id, 'CALENDAR', 'CANCELLATION', 'attendees_only'
    WHERE NOT EXISTS (
      SELECT 1 FROM pipeline_routing
      WHERE tenant_id = t_id AND content_type = 'CALENDAR' AND content_subtype = 'CANCELLATION'
    );

  END LOOP;
END $$;

COMMIT;

-- Rollback (manual):
-- Revert routing UPDATE:
--   UPDATE pipeline_routing SET pipeline = 'standard'
--   WHERE content_type = 'EMAIL' AND content_subtype = 'NOTIFICATION' AND pipeline = 'notification';
--
-- Delete new routing INSERTs:
--   DELETE FROM pipeline_routing WHERE content_subtype IN ('NEWSLETTER', 'AUTO_REPLY', 'CALENDAR_RESPONSE', 'CALENDAR_UPDATE')
--     AND content_type = 'EMAIL';
--   DELETE FROM pipeline_routing WHERE content_type = 'CALENDAR' AND content_subtype = 'CANCELLATION';
--
-- Delete new pipeline_definitions:
--   DELETE FROM pipeline_definitions WHERE pipeline IN ('notification', 'newsletter', 'auto_reply');
