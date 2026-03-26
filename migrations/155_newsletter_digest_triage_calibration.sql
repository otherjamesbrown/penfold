-- +goose Up

-- Migration 154: Newsletter digest triage calibration (pf-2193d8)
-- Adds triage prompt v4 specifically calibrated for automated social feed digests,
-- and wires it into the newsletter_digest pipeline triage stage.
--
-- Background: The newsletter_digest pipeline (Dynamic Signal) was using triage v3
-- (the general newsletter prompt). The general newsletter prompt rates "all-hands
-- recaps" and "product updates" as MEDIUM, causing the Dynamic Signal digest to be
-- rated MEDIUM when the expected rating for this low-signal automated feed is LOW.
--
-- Fix: triage v4 is calibrated specifically for automated digest feeds — it defaults
-- to LOW unless genuinely project-relevant content is found.

-- 1. Triage v4 — automated digest pipeline variant
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('triage', 4, $prompt$You are a content classifier for a business knowledge management system. This content is an automated digest from a social feed tool — not a newsletter written by a person.

Automated digests aggregate third-party content (social posts, reshared articles, app downloads, engagement metrics). They are typically low-signal noise.

Classify this content into exactly ONE category:
- FYI: general automated digest with no required action and no project-relevant content
- PROJECT_UPDATE: the digest contains an item that directly relates to a tracked project or product (rare)
- ACTION_REQUEST: the digest explicitly requires the recipient to take a specific action with a deadline
- OTHER: does not fit any category above

Rate importance: HIGH, MEDIUM, LOW

Calibrate importance aggressively LOW for automated digest content:
- LOW: routine automated digest, social feed aggregation, app downloads, engagement content, reshared articles — this is the default for automated digests
- MEDIUM: only if the digest contains an item that explicitly references a tracked project or product by name with meaningful context
- HIGH: only if the digest contains a time-sensitive action requirement or a critical business alert — this is extremely rare for automated feed digests

Key question: Is there any content in this digest that a business professional would need to act on or that directly relates to active projects? If no, rate LOW.

Respond ONLY with JSON:
{"category": "...", "importance": "...", "reason": "one sentence"}$prompt$,
'Triage prompt v4 — automated digest pipeline variant, calibrated LOW for social feed digests (pf-2193d8)',
false, 'agent-mycroft')
ON CONFLICT (stage, version) DO NOTHING;

-- 2. Update newsletter_digest triage stage to use prompt_override = 4
-- (previously set to NULL by register_newsletter_variant, or incorrectly set to 3 by pf-8f8510)
UPDATE pipeline_definitions
SET    prompt_override = 4
WHERE  pipeline = 'newsletter_digest'
  AND  stage = 'triage';

-- +goose Down

-- Remove triage v4 prompt
DELETE FROM prompt_templates WHERE stage = 'triage' AND version = 4;

-- Revert newsletter_digest triage stage to NULL (default active prompt)
UPDATE pipeline_definitions
SET    prompt_override = NULL
WHERE  pipeline = 'newsletter_digest'
  AND  stage = 'triage';
