-- Migration 115: Digests table and daily digest prompt template
-- Stores generated daily/weekly/journal digest records

-- 1. Register digest_daily_generate pipeline stage (required by prompt_templates FK)
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'digest_daily_generate',
    'Daily Digest Generation',
    'Generates a structured daily digest for a project from attributed content, assertions, instruction matches, and ledger entries.',
    'llm', true, true,
    ARRAY[]::text[],
    ARRAY[]::text[]
) ON CONFLICT (stage) DO NOTHING;

-- 2. Digests table
CREATE TABLE IF NOT EXISTS digests (
    id              TEXT PRIMARY KEY DEFAULT 'dg-' || left(replace(gen_random_uuid()::text, '-', ''), 12),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    project_id      BIGINT NOT NULL REFERENCES projects(id),
    digest_type     VARCHAR(20) NOT NULL DEFAULT 'daily',
    period_start    DATE NOT NULL,
    period_end      DATE NOT NULL,
    body            JSONB NOT NULL,
    model_used      VARCHAR(100),
    prompt_template_id BIGINT,
    input_token_count  INT DEFAULT 0,
    output_token_count INT DEFAULT 0,
    source_content_ids JSONB DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_digests_tenant_project ON digests(tenant_id, project_id);
CREATE INDEX IF NOT EXISTS idx_digests_tenant_project_type_period ON digests(tenant_id, project_id, digest_type, period_start);
CREATE UNIQUE INDEX IF NOT EXISTS idx_digests_unique_daily ON digests(tenant_id, project_id, digest_type, period_start)
    WHERE digest_type = 'daily';

-- 3. Prompt template for daily digest generation
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'digest_daily_generate',
    1,
    E'You are a knowledge management assistant generating a daily project digest.\n\nProject: {{.ProjectName}}\nDate: {{.Date}}\n\nContent items attributed to this project today:\n{{.ContentSummaries}}\n\nExtracted assertions (risks, actions, decisions):\n{{.Assertions}}\n\nInstruction matches:\n{{.InstructionMatches}}\n\nSession ledger entries:\n{{.LedgerEntries}}\n\nGenerate a structured JSON digest with the following format:\n{\n  \"summary\": \"One paragraph narrative of the day''s key events\",\n  \"sections\": {\n    \"instruction_matches\": [{\"instruction_name\": \"...\", \"matches\": [...], \"summary\": \"...\"}],\n    \"assertions\": {\n      \"risks\": [{\"text\": \"...\", \"source_content_id\": \"...\"}],\n      \"actions\": [...],\n      \"decisions\": [...]\n    },\n    \"themes_updated\": [],\n    \"ledger_entries\": []\n  }\n}\n\nBe factual and concise. Reference specific people, dates, and content when available. If there are no items for a section, use an empty array.',
    'Daily digest generation — single-pass Fast tier',
    true,
    'agent-penfold'
)
ON CONFLICT (stage, version) DO NOTHING;
