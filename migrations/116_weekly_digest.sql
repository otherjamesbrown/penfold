-- Migration 116: Weekly digest pipeline stages and prompt templates
-- Adds digest_weekly_synthesize and digest_theme_context_update stages

-- 1. Register digest_weekly_synthesize pipeline stage
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'digest_weekly_synthesize',
    'Weekly Digest Synthesis',
    'Synthesizes a weekly digest rollup from daily digests, previous weekly rollup, and theme running contexts.',
    'llm', true, true,
    ARRAY[]::text[],
    ARRAY[]::text[]
) ON CONFLICT (stage) DO NOTHING;

-- 2. Register digest_theme_context_update pipeline stage
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'digest_theme_context_update',
    'Theme Context Update',
    'Updates a topic running context by integrating weekly digest insights with existing context.',
    'llm', true, true,
    ARRAY[]::text[],
    ARRAY[]::text[]
) ON CONFLICT (stage) DO NOTHING;

-- 3. Prompt template for weekly digest synthesis
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'digest_weekly_synthesize',
    1,
    E'You are a knowledge management assistant generating a weekly project digest rollup.\n\nProject: {{.ProjectName}}\nWeek: {{.WeekStart}} to {{.WeekEnd}}\n\nDaily digest summaries for this week:\n{{.DailyDigests}}\n\nPrevious weekly rollup:\n{{.PreviousRollup}}\n\nCurrent theme running contexts:\n{{.ThemeContexts}}\n\nGenerate a structured JSON weekly digest with the following format:\n{\n  "summary": "One paragraph narrative of the week''s key themes and developments",\n  "sections": {\n    "trends": ["trend observations"],\n    "key_decisions": ["decisions made"],\n    "risks": ["emerging risks"],\n    "themes": [{"name": "theme_name", "context_update": "new running context text"}]\n  }\n}\n\nBe factual and concise. Reference specific people, dates, and decisions when available. For each active theme, provide an updated context_update that integrates the week''s developments. If there are no items for a section, use an empty array.',
    'Weekly digest synthesis — rolls up daily digests into a weekly narrative with theme updates',
    true,
    'agent-penfold'
)
ON CONFLICT (stage, version) DO NOTHING;

-- 4. Prompt template for theme context update
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'digest_theme_context_update',
    1,
    E'You are a knowledge management assistant updating a running context for a project theme.\n\nTopic: {{.TopicName}}\n\nExisting running context:\n{{.CurrentContext}}\n\nWeekly digest summary:\n{{.WeeklySummary}}\n\nTheme-specific update from weekly digest:\n{{.ThemeUpdate}}\n\nWrite a concise updated running context paragraph (2-4 sentences) that integrates the existing context with the new weekly insights. Preserve important historical context while incorporating the latest developments. Output only the updated context paragraph — no JSON, no preamble.',
    'Theme context update — integrates weekly digest insights into topic running context',
    true,
    'agent-penfold'
)
ON CONFLICT (stage, version) DO NOTHING;
