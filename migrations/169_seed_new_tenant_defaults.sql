-- +goose Up

-- Migration 169: Seed default pipeline_definitions, pipeline_routing, and
-- pipeline_operational_config for tenants that were created before automatic seeding
-- was added to tenant.Repository.Create() (pf-40d500).
--
-- Safe to run on any tenant: every INSERT uses ON CONFLICT DO NOTHING.
-- Backfills m365-test (7078ab42-336e-48cc-8f8a-056487204546) specifically, plus
-- any other tenants that may be missing defaults.

DO $$
DECLARE
  t_id UUID;
BEGIN
  FOR t_id IN SELECT id FROM tenants WHERE NOT is_deleted LOOP

    -- ── pipeline_definitions ──────────────────────────────────────────────────

    -- standard
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, stage_kind, persist_key)
    VALUES
      (t_id, 'standard', 'parse',               10, true, false, false,  30, 'code_only',  NULL),
      (t_id, 'standard', 'triage',              20, true, false, false, 120, 'llm',        NULL),
      (t_id, 'standard', 'summarize',           25, true, true,  true,   60, 'llm',        NULL),
      (t_id, 'standard', 'extract_ner',         30, true, true,  false, 120, 'llm',        NULL),
      (t_id, 'standard', 'extract_assertions',  40, true, true,  false, 120, 'llm',        NULL),
      (t_id, 'standard', 'classify_project',    45, true, false, true,  120, 'llm',        NULL),
      (t_id, 'standard', 'instruction_evaluate',46, true, true,  true,  120, 'llm',        NULL),
      (t_id, 'standard', 'extract_semantic',    50, true, true,  false, 120, 'llm',        NULL),
      (t_id, 'standard', 'resolve',             60, true, true,  false,  60, 'code_only',  NULL),
      (t_id, 'standard', 'enrich_entities',     65, true, true,  true,  120, 'code_only',  NULL),
      (t_id, 'standard', 'analyze',             70, true, true,  true,  120, 'llm',        NULL),
      (t_id, 'standard', 'persist',             80, true, true,  false,  60, 'code_only',  NULL),
      (t_id, 'standard', 'embed',               90, true, false, false,  60, 'embedding',  NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- transcript
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, stage_kind, persist_key)
    VALUES
      (t_id, 'transcript', 'parse_transcript',   10, true, false, false,  30, 'code_only', NULL),
      (t_id, 'transcript', 'triage',             20, true, false, false, 120, 'llm',       NULL),
      (t_id, 'transcript', 'summarize',          25, true, true,  true,   60, 'llm',       NULL),
      (t_id, 'transcript', 'extract_ner',        30, true, false, false, 120, 'llm',       NULL),
      (t_id, 'transcript', 'extract_assertions', 40, true, false, false, 120, 'llm',       NULL),
      (t_id, 'transcript', 'classify_project',   45, true, false, true,  120, 'llm',       NULL),
      (t_id, 'transcript', 'instruction_evaluate',46,true, true,  true,  120, 'llm',       NULL),
      (t_id, 'transcript', 'extract_semantic',   50, true, false, false, 120, 'llm',       NULL),
      (t_id, 'transcript', 'resolve',            60, true, false, false, 120, 'code_only', NULL),
      (t_id, 'transcript', 'analyze',            70, true, false, false, 120, 'llm',       NULL),
      (t_id, 'transcript', 'persist',            80, true, false, false,  60, 'code_only', NULL),
      (t_id, 'transcript', 'embed',              90, true, false, false,  60, 'embedding', NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- attendees_only
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, stage_kind, persist_key)
    VALUES
      (t_id, 'attendees_only', 'parse',       0, true, false, false,  30, 'code_only', NULL),
      (t_id, 'attendees_only', 'triage',      1, true, false, false, 120, 'llm',       NULL),
      (t_id, 'attendees_only', 'extract_ner', 2, true, false, false, 120, 'llm',       NULL),
      (t_id, 'attendees_only', 'embed',       3, true, false, false,  60, 'embedding', NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- auto_reply
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, stage_kind, persist_key)
    VALUES
      (t_id, 'auto_reply', 'parse',     0, true, false, false,  30, 'code_only', NULL),
      (t_id, 'auto_reply', 'triage',   10, true, false, false, 120, 'llm',       NULL),
      (t_id, 'auto_reply', 'summarize',20, true, true,  true,   60, 'llm',       NULL),
      (t_id, 'auto_reply', 'embed',    30, true, false, false,  60, 'embedding', NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- newsletter
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, stage_kind, persist_key)
    VALUES
      (t_id, 'newsletter', 'parse',              0, true, false, false,  30, 'code_only',        NULL),
      (t_id, 'newsletter', 'triage',             1, true, false, false, 120, 'llm',              NULL),
      (t_id, 'newsletter', 'newsletter_extract', 2, true, false, false, 120, 'structured_extract', 'newsletter'),
      (t_id, 'newsletter', 'embed',              4, true, false, false,  60, 'embedding',        NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- newsletter_internal
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, stage_kind, persist_key)
    VALUES
      (t_id, 'newsletter_internal', 'parse',              0, true, false, false,  60, 'code_only',          NULL),
      (t_id, 'newsletter_internal', 'triage',             1, true, false, false, 120, 'llm',                NULL),
      (t_id, 'newsletter_internal', 'newsletter_extract', 2, true, false, false, 120, 'structured_extract', 'newsletter'),
      (t_id, 'newsletter_internal', 'embed',              3, true, false, false,  60, 'embedding',          NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- newsletter_digest
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, stage_kind, persist_key)
    VALUES
      (t_id, 'newsletter_digest', 'parse',              0, true, false, false,  60, 'code_only',          NULL),
      (t_id, 'newsletter_digest', 'triage',             1, true, false, false, 120, 'llm',                NULL),
      (t_id, 'newsletter_digest', 'newsletter_extract', 2, true, false, false, 120, 'structured_extract', 'newsletter'),
      (t_id, 'newsletter_digest', 'embed',              3, true, false, false,  60, 'embedding',          NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- notification
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, stage_kind, persist_key)
    VALUES
      (t_id, 'notification', 'parse',            0, true, false, false,  30, 'code_only', NULL),
      (t_id, 'notification', 'triage',          10, true, false, false, 120, 'llm',       NULL),
      (t_id, 'notification', 'summarize',       15, true, true,  false,  60, 'llm',       NULL),
      (t_id, 'notification', 'extract_ner',     20, true, false, false, 120, 'llm',       NULL),
      (t_id, 'notification', 'extract_semantic',25, true, true,  false, 120, 'llm',       NULL),
      (t_id, 'notification', 'persist',         50, true, false, false,  60, 'code_only', NULL),
      (t_id, 'notification', 'embed',           60, true, false, false,  60, 'embedding', NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- ── pipeline_routing ──────────────────────────────────────────────────────

    INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline, active)
    VALUES
      (t_id, 'EMAIL',    'HUMAN',               'standard',            true),
      (t_id, 'EMAIL',    'NOTIFICATION',        'notification',        true),
      (t_id, 'EMAIL',    'NEWSLETTER',          'newsletter',          true),
      (t_id, 'EMAIL',    'NEWSLETTER_INTERNAL', 'newsletter_internal', true),
      (t_id, 'EMAIL',    'NEWSLETTER_DIGEST',   'newsletter_digest',   true),
      (t_id, 'EMAIL',    'AUTO_REPLY',          'auto_reply',          true),
      (t_id, 'EMAIL',    'CALENDAR_UPDATE',     'attendees_only',      true),
      (t_id, 'EMAIL',    'CALENDAR_RESPONSE',   'attendees_only',      true),
      (t_id, 'CALENDAR', 'INVITE',              'attendees_only',      true),
      (t_id, 'CALENDAR', 'CANCELLATION',        'attendees_only',      true),
      (t_id, 'MEETING',  'TRANSCRIPT',          'standard',            true)
    ON CONFLICT (tenant_id, content_type, content_subtype, pipeline) DO NOTHING;

    -- ── pipeline_operational_config ───────────────────────────────────────────

    INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
    VALUES
      (t_id, 'embedding.chunk_max_tokens',              '400',  'Maximum tokens per embedding chunk'),
      (t_id, 'embedding.chunk_overlap_tokens',          '50',   'Overlap tokens between chunks'),
      (t_id, 'pipeline.kick_next_limit',                '0',    'KickNextPending limit (0 = fill all available slots)'),
      (t_id, 'model.concurrency.ollama',                '3',    'Max concurrent Ollama model calls'),
      (t_id, 'model.concurrency.gemini',                '50',   'Max concurrent Gemini model calls'),
      (t_id, 'model.concurrency.anthropic',             '20',   'Max concurrent Anthropic model calls'),
      (t_id, 'model.concurrency.mlx',                   '3',    'Max concurrent MLX model calls'),
      (t_id, 'model.concurrency.default',               '10',   'Default max concurrent model calls'),
      (t_id, 'queue.backpressure.tier1_threshold',      '50',   'Pending queue depth triggering tier1 timeout'),
      (t_id, 'queue.backpressure.tier1_timeout_hours',  '2',    'Workflow execution timeout (hours) at tier1'),
      (t_id, 'queue.backpressure.tier2_threshold',      '100',  'Pending queue depth triggering tier2 timeout'),
      (t_id, 'queue.backpressure.tier2_timeout_hours',  '4',    'Workflow execution timeout (hours) at tier2'),
      (t_id, 'queue.backpressure.default_timeout_hours','1',    'Default workflow execution timeout (hours)'),
      (t_id, 'attribution.method',                      'llm',  'Project attribution method: llm or keyword'),
      (t_id, 'automation.chain_depth_max',              '3',    'Maximum automation rule chain depth'),
      (t_id, 'automation.default_selector_limit',       '50',   'Default result limit for automation selectors'),
      (t_id, 'context.tenant_context.max_entries_per_email', '10',   'Max tenant context entries injected per email'),
      (t_id, 'context.tenant_context.content_scan_length',   '5000', 'Max content length scanned for context triggers')
    ON CONFLICT (tenant_id, key) DO NOTHING;

  END LOOP;
END $$;

-- +goose Down

-- No rollback: seeded config rows may have been intentionally modified by tenants.
-- To undo for a specific tenant, delete rows manually with appropriate tenant_id filter.
