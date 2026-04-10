-- +goose Up

-- Migration 163: Seed classify_project prompt template, routing rule, feature flag
-- Parent: pf-2091d5 (LLM project classification design)
-- Task: pf-5b62f5

-- 1. Register classify_project in pipeline_stages
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'classify_project',
    'Classify Project',
    'LLM-based project attribution using gemini-flash. Replaces keyword-based attribute_project stage.',
    'llm',
    true,
    true,
    ARRAY['extract_assertions'],
    ARRAY['analyze']
) ON CONFLICT (stage) DO NOTHING;

-- 2. Prompt template
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('classify_project', 1, $$
You are classifying an email into James's personal projects.

Projects (id | name | description | keyword hints):
{{.ProjectList}}

Rules:
- Pick AT MOST 2 projects. Only pick a project if you are confident it is genuinely about that project's subject matter.
- If nothing fits well, return an empty list. DO NOT default to any project.
- Keyword hints are just hints — use judgement. An "appointment" email from webuyanycar is NOT healthcare; it's a car valuation.
- Return JSON: {"projects": [{"id": 123, "confidence": 0.8, "reason": "dental appointment at Oxford Smile Clinics"}]}

Email:
From: {{.From}}
Subject: {{.Subject}}
Body preview: {{.BodyPreview}}
$$, 'LLM-based project classification prompt — replaces keyword matching', true, 'design-pf-2091d5')
ON CONFLICT (stage, version) DO NOTHING;

-- 3. AI routing rule (task_type: classify_project, prefers gemini-flash)
INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES ('classify-project-default', 'classify_project',
        ARRAY['gemini/gemini-2.5-flash'], ARRAY['gemini/gemini-2.0-flash'],
        'latency', 0, true, '{}')
ON CONFLICT (name) DO NOTHING;

-- 4. Feature flag per tenant
INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'attribution.method', 'llm', 'Project attribution method: llm or keyword'
FROM tenants
ON CONFLICT (tenant_id, key) DO NOTHING;

-- +goose Down
DELETE FROM pipeline_operational_config WHERE key = 'attribution.method';
DELETE FROM ai_routing_rules WHERE name = 'classify-project-default';
DELETE FROM prompt_templates WHERE stage = 'classify_project';
DELETE FROM pipeline_stages WHERE stage = 'classify_project';
