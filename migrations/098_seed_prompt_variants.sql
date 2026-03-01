-- pf-14f931: Seed pipeline-specific prompt variants for notification and newsletter pipelines.
-- New prompts are inserted as is_active = false — they are accessed exclusively through
-- prompt_override set on pipeline_definitions (see migration 099).
--
-- Version map after this migration:
--   triage:           1 (active, standard), 2 (inactive, notification), 3 (inactive, newsletter)
--   summary:          1 (active, standard), 2 (inactive, notification)
--   extract_semantic: 1 (active, standard), 2 (inactive, notification), 3 (inactive, newsletter)
--
-- Versions 2 and 3 of extract_semantic previously existed as inactive old drafts.
-- ON CONFLICT intentionally overwrites them with the new pipeline-specific content.

-- 1. Triage v2 — notification pipeline
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('triage', 2, $prompt$You are a content classifier for a business knowledge management system. This content is an automated notification from a system or tool — not a message written by a person.

Classify this content into exactly ONE category:
- ACTION_REQUEST: the notification requires a human to take a specific action (e.g. approve a request, resolve an alert, respond to a ticket)
- RISK_ISSUE: the notification describes a security breach, system failure, compliance violation, or high-severity alert requiring immediate attention
- FYI: informational status updates, delivery confirmations, build results, routine system events
- INTERNAL_COMMS: policy notices, compliance reminders, or IT communications sent to users
- PROJECT_UPDATE: CI/CD pipeline results, deployment notifications, issue tracker updates, PR notifications
- OTHER: does not fit any category above

Rate importance: HIGH, MEDIUM, LOW

Calibrate importance for automated content:
- LOW: routine device check-ins, MDM heartbeats, successful build notifications, non-critical status pings
- MEDIUM: deployment completions, ticket assignments, review requests, service degradation warnings
- HIGH: security breach alerts, critical system failures, compliance violations, approval requests with deadlines

Key question to answer: Is this telling me about a routine automated event, or does a human need to act on this?

Assess content_contribution (how much NEW information the message body contributes):
- HIGH: Contains specific incident details, breach data, action requirements, or non-routine status
- MEDIUM: Contains structured system data mixed with boilerplate template text
- LOW: Template-heavy with only minor dynamic values (e.g. a device serial number in a check-in ping)
- NONE: Fully automated notifications with no human-written content and no required action

Respond ONLY with JSON:
{"category": "...", "importance": "...", "reason": "one sentence", "content_contribution": "...", "contribution_reason": "one sentence"}$prompt$,
'Triage prompt v2 — notification pipeline variant, calibrated for automated system notifications (pf-14f931)',
false, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content     = EXCLUDED.content,
        description = EXCLUDED.description,
        is_active   = EXCLUDED.is_active;

-- 2. Summary v2 — notification pipeline
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('summary', 2, $prompt$You are a summarization assistant. {style_instruction}{length_instruction}

Summarize this automated notification concisely in 1-2 sentences. Focus on: what system generated it, what happened, and whether any action is required.

Skip boilerplate headers, footers, unsubscribe links, and template filler text. Extract only the meaningful signal from the notification.

After the summary, provide 3-5 key points as a JSON array.

Format your response as:
SUMMARY:
[Your summary here]

KEY_POINTS:
["point 1", "point 2", "point 3"]$prompt$,
'Summary prompt v2 — notification pipeline variant, concise automated notification summaries (pf-14f931)',
false, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content     = EXCLUDED.content,
        description = EXCLUDED.description,
        is_active   = EXCLUDED.is_active;

-- 3. Extract_semantic v2 — notification pipeline
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('extract_semantic', 2, $prompt$Extract the following from this automated notification. Only include information that is explicitly stated — do not infer or guess.

Frame your extraction around what the notification is signalling, not around strategic business intelligence. A routine MDM device check-in has no action items, decisions, or risks — do not manufacture them.

1. Required actions: any explicit step a human must take in response to this notification (e.g. approve a request, remediate an alert, update a configuration). Only include if the notification directly requires human action.
2. Decisions or status changes: system-level state changes announced by the notification (e.g. a deployment was promoted, a service was restarted, a ticket was closed). Use the decisions field for these.
3. Risks or alerts: security incidents, system failures, threshold breaches, or compliance violations flagged by the notification. Only include genuine alerts — not routine status messages.

Respond ONLY with JSON:
{
  "action_items": [{"assignee": "...", "action": "...", "due": "..."}],
  "decisions": ["..."],
  "risks": ["..."]
}

If a field has no matches, use an empty array.

---
%s$prompt$,
'Extract_semantic prompt v2 — notification pipeline variant, action/alert/status-focused extraction (pf-14f931)',
false, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content     = EXCLUDED.content,
        description = EXCLUDED.description,
        is_active   = EXCLUDED.is_active;

-- 4. Triage v3 — newsletter pipeline
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('triage', 3, $prompt$You are a content classifier for a business knowledge management system. This content is a newsletter, company broadcast, or mass communication — not a direct personal message.

Classify this content into exactly ONE category:
- INTERNAL_COMMS: company-wide announcements, HR communications, policy changes, compliance notices, leadership updates
- PROJECT_UPDATE: product launch announcements, engineering updates, roadmap communications, feature releases
- FYI: general informational newsletters, industry news digests, event recaps with no required action
- ACTION_REQUEST: newsletters that explicitly require the recipient to take a specific action (e.g. complete a survey, register for an event by a deadline, acknowledge a policy)
- PERSONAL: wellness newsletters, social event invitations, casual internal community content
- OTHER: does not fit any category above

Rate importance: HIGH, MEDIUM, LOW

Calibrate importance for mass communications:
- LOW: wellness newsletters, social event announcements, general interest digests
- MEDIUM: company all-hands recaps, product launch announcements, team updates
- HIGH: policy change announcements with compliance deadlines, critical company-wide notices, mandatory training with hard deadlines

Assess content_contribution (how much NEW information the message body contributes):
- HIGH: Contains policy changes, significant announcements, or mandatory actions with deadlines
- MEDIUM: Contains a mix of new announcements and standard newsletter filler
- LOW: Mostly template structure with limited new information
- NONE: Boilerplate-only with no meaningful content (e.g. an empty digest)

Respond ONLY with JSON:
{"category": "...", "importance": "...", "reason": "one sentence", "content_contribution": "...", "contribution_reason": "one sentence"}$prompt$,
'Triage prompt v3 — newsletter pipeline variant, calibrated for mass communications and broadcasts (pf-14f931)',
false, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content     = EXCLUDED.content,
        description = EXCLUDED.description,
        is_active   = EXCLUDED.is_active;

-- 5. Extract_semantic v3 — newsletter pipeline
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('extract_semantic', 3, $prompt$Extract the following from this newsletter or company broadcast. Only include information that is explicitly stated — do not infer or guess.

Frame your extraction around what the newsletter is communicating to its recipients, not around strategic business intelligence. A company bike ride event is not a risk — it belongs in decisions (an event the company has organised). A compliance policy change with a deadline is an action item for recipients.

1. Recipient action items: anything the newsletter explicitly asks recipients to do (e.g. register for an event, complete a survey, acknowledge a policy, meet a compliance deadline). Capture the deadline in the due field where stated.
2. Announcements and decisions: significant company decisions, event announcements, product launches, policy changes, or programme updates communicated in the newsletter. Use the decisions field for these.
3. Risks or deadlines: compliance deadlines, mandatory requirements with consequences for non-action, or urgent notices flagged in the newsletter. Do not extract general business topics as risks.

Respond ONLY with JSON:
{
  "action_items": [{"assignee": "...", "action": "...", "due": "..."}],
  "decisions": ["..."],
  "risks": ["..."]
}

If a field has no matches, use an empty array.

---
%s$prompt$,
'Extract_semantic prompt v3 — newsletter pipeline variant, announcement/event/deadline-focused extraction (pf-14f931)',
false, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content     = EXCLUDED.content,
        description = EXCLUDED.description,
        is_active   = EXCLUDED.is_active;
