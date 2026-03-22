-- +goose Up

-- Migration 147: Newsletter variant registration pattern (pf-5f200d)
-- Creates a reusable SQL function and operator view for registering newsletter variants.
-- Previously, each variant required a bespoke migration with 4 manual INSERT blocks.
-- This function encapsulates the full registration in a single idempotent call.

-- Function: register_newsletter_variant()
-- Registers a newsletter variant across all tenants by inserting into:
--   1. classification_rules + classification_match_conditions
--   2. pipeline_routing
--   3. pipeline_definitions (4 stages: parse, triage, newsletter_extract, embed)
--   4. prompt_templates
--
-- Fully idempotent — safe to call multiple times. Skips existing rows on conflict.
CREATE OR REPLACE FUNCTION register_newsletter_variant(
    p_variant_name        TEXT,
    p_content_subtype     TEXT,
    p_rule_name           TEXT,
    p_rule_priority       INT,
    p_match_field         TEXT,
    p_match_type          TEXT,
    p_match_value         TEXT,
    p_match_case_sensitive BOOLEAN DEFAULT false,
    p_prompt_version      INT DEFAULT NULL,
    p_prompt_content      TEXT DEFAULT NULL,
    p_prompt_description  TEXT DEFAULT ''
) RETURNS VOID AS $$
DECLARE
    t_rec         RECORD;
    t_id          UUID;
    rule_id       INT;
    has_stage_kind BOOLEAN;
BEGIN
    -- Detect whether pipeline_definitions has the stage_kind column (added in migration 144).
    -- This guard mirrors the pattern used in migration 146 to handle tracking mismatches.
    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'pipeline_definitions' AND column_name = 'stage_kind'
    ) INTO has_stage_kind;

    -- 1. Prompt template (global, not per-tenant)
    IF p_prompt_version IS NOT NULL AND p_prompt_content IS NOT NULL THEN
        INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
        VALUES (
            'newsletter_extract',
            p_prompt_version,
            p_prompt_content,
            p_prompt_description,
            false,
            'register_newsletter_variant'
        )
        ON CONFLICT (stage, version) DO NOTHING;
    END IF;

    FOR t_rec IN SELECT id FROM tenants LOOP
        t_id := t_rec.id;

        -- 2. Classification rule + match condition (skip if rule name already exists for tenant)
        IF NOT EXISTS (
            SELECT 1 FROM classification_rules
            WHERE tenant_id = t_id AND name = p_rule_name
        ) THEN
            INSERT INTO classification_rules (
                tenant_id, name, priority, content_type_scope,
                content_type, content_subtype, active
            )
            VALUES (
                t_id,
                p_rule_name,
                p_rule_priority,
                'EMAIL',
                'EMAIL',
                p_content_subtype,
                true
            )
            RETURNING id INTO rule_id;

            INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
            VALUES (rule_id, p_match_field, p_match_type, p_match_value, p_match_case_sensitive);
        END IF;

        -- 3. Pipeline routing: EMAIL/{content_subtype} → {variant_name}
        INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline, active)
        SELECT t_id, 'EMAIL', p_content_subtype, p_variant_name, true
        WHERE NOT EXISTS (
            SELECT 1 FROM pipeline_routing
            WHERE tenant_id = t_id
              AND content_type = 'EMAIL'
              AND content_subtype = p_content_subtype
        );

        -- 4. Pipeline definitions (4 stages)
        IF has_stage_kind THEN
            INSERT INTO pipeline_definitions (
                tenant_id, pipeline, stage, stage_order,
                enabled, skip_when_low, optional, timeout_seconds,
                model_override, prompt_override, stage_kind, persist_key
            )
            VALUES
                (t_id, p_variant_name, 'parse',              0, true, false, false,  60, NULL, NULL,             'code_only',          NULL),
                (t_id, p_variant_name, 'triage',             1, true, false, false, 120, NULL, NULL,             'llm',                NULL),
                (t_id, p_variant_name, 'newsletter_extract', 2, true, false, false, 120, NULL, p_prompt_version, 'structured_extract', 'newsletter'),
                (t_id, p_variant_name, 'embed',              3, true, false, false,  60, NULL, NULL,             'embedding',          NULL)
            ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;
        ELSE
            INSERT INTO pipeline_definitions (
                tenant_id, pipeline, stage, stage_order,
                enabled, skip_when_low, optional, timeout_seconds,
                model_override, prompt_override
            )
            VALUES
                (t_id, p_variant_name, 'parse',              0, true, false, false,  60, NULL, NULL),
                (t_id, p_variant_name, 'triage',             1, true, false, false, 120, NULL, NULL),
                (t_id, p_variant_name, 'newsletter_extract', 2, true, false, false, 120, NULL, p_prompt_version),
                (t_id, p_variant_name, 'embed',              3, true, false, false,  60, NULL, NULL)
            ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;
        END IF;

    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- View: newsletter_variant_overview
-- Joins classification_rules, pipeline_routing, pipeline_definitions, and prompt_templates
-- to provide operators a single query showing all configured newsletter variants.
CREATE OR REPLACE VIEW newsletter_variant_overview AS
SELECT
    pr.pipeline                                           AS variant_name,
    pr.content_subtype                                    AS content_subtype,
    cr.name                                               AS rule_name,
    cr.priority                                           AS rule_priority,
    cmc.value                                             AS match_value,
    pd_extract.prompt_override                            AS prompt_version,
    (SELECT COUNT(*) FROM pipeline_definitions pd
     WHERE pd.tenant_id = pr.tenant_id
       AND pd.pipeline = pr.pipeline)                     AS stage_count
FROM pipeline_routing pr
JOIN classification_rules cr
    ON cr.tenant_id = pr.tenant_id
   AND cr.content_subtype = pr.content_subtype
   AND cr.content_type = 'EMAIL'
LEFT JOIN classification_match_conditions cmc
    ON cmc.rule_id = cr.id
LEFT JOIN pipeline_definitions pd_extract
    ON pd_extract.tenant_id = pr.tenant_id
   AND pd_extract.pipeline = pr.pipeline
   AND pd_extract.stage = 'newsletter_extract'
WHERE pr.content_type = 'EMAIL'
  AND pr.content_subtype LIKE 'NEWSLETTER%';

-- Example: Register a "Tech Digest" variant
-- Copy and execute this block to add a second newsletter variant without writing a new migration.
--
-- SELECT register_newsletter_variant(
--   p_variant_name        := 'newsletter_tech_digest',
--   p_content_subtype     := 'NEWSLETTER_TECH',
--   p_rule_name           := 'newsletter_tech_digest',
--   p_rule_priority       := 80,
--   p_match_field         := 'subject',
--   p_match_type          := 'contains',
--   p_match_value         := 'Tech Digest',
--   p_match_case_sensitive := false,
--   p_prompt_version      := 4,
--   p_prompt_content      := 'You are extracting...(variant prompt)...',
--   p_prompt_description  := 'Newsletter extraction v4 for Tech Digest variant'
-- );

-- +goose Down

DROP VIEW IF EXISTS newsletter_variant_overview;
DROP FUNCTION IF EXISTS register_newsletter_variant(
    TEXT, TEXT, TEXT, INT, TEXT, TEXT, TEXT, BOOLEAN, INT, TEXT, TEXT
);
