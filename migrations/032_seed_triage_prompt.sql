BEGIN;

-- Seed the triage prompt template into prompt_templates table
-- This matches the prompt specified in specs/020-slm-llm-architecture/design.md lines 170-193

INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'triage',
    1,
    E'You are a content classifier for a business knowledge management system.\n\nClassify this content into exactly ONE category:\n- PROJECT_UPDATE: project status, meeting notes, deliverables, milestones\n- CUSTOMER: customer communications, sales, deals, account management\n- RISK_ISSUE: risks, problems, escalations, blockers, vulnerabilities\n- ACTION_REQUEST: someone asking for specific action to be done\n- DECISION: a decision has been made or is being requested\n- INTERNAL_COMMS: HR, training, company announcements, policy changes\n- PERSONAL: lunch, social, casual conversation\n- OTHER: does not fit any category above\n\nRate importance: HIGH, MEDIUM, LOW\n\nRespond ONLY with JSON:\n{"category": "...", "importance": "...", "reason": "one sentence"}',
    'Initial triage prompt from design.md spec',
    true,
    'agent-mycroft'
) ON CONFLICT (stage, version) DO NOTHING;

COMMIT;
