-- pf-1c083d: Update triage prompt v2 (notification pipeline) to correctly handle hybrid notifications.
--
-- Prompt v2 (from migration 098) incorrectly states: "This content is an automated notification
-- from a system or tool — not a message written by a person."
--
-- This framing is wrong for hybrid notification sources (Google Docs comments, GitHub PR reviews,
-- GitHub issue comments) where the notification body contains verbatim human-written content.
-- A Google Docs comment notification IS written by a person — it just arrives via notification
-- infrastructure. Framing it as "not written by a person" causes the LLM to undervalue the
-- content and misclassify it.
--
-- This migration rewrites prompt v2 to acknowledge both cases: pure system notifications
-- (Jira state changes, MDM heartbeats) and hybrid notifications (human-authored content
-- delivered via notification wrapper). The classifier is instructed to identify which type
-- it is seeing and calibrate accordingly.
--
-- Prompt v2 remains is_active = false — it is accessed via prompt_override=2 on the
-- notification pipeline definitions (set in migration 099).

UPDATE prompt_templates
SET
    content = $prompt$You are a content classifier for a business knowledge management system. This content arrived as an automated notification from a system or tool.

Automated notifications fall into two categories — identify which you are seeing:

1. PURE SYSTEM NOTIFICATIONS: machine-generated state-change events with no human-written content body.
   Examples: Jira ticket status changes, MDM device check-ins, build pipeline results, Aha! roadmap updates,
   deployment confirmations. The body contains structured system data and boilerplate — no person wrote it.

2. HYBRID NOTIFICATIONS: a human wrote the substantive content, but it was delivered via notification
   infrastructure. Examples: Google Docs comment notifications (the commenter's words appear verbatim),
   GitHub issue/PR review comments (the reviewer's analysis), Slack message notifications.
   The wrapper is automated, but the body IS written by a person.

Classify this content into exactly ONE category:
- ACTION_REQUEST: requires a human to take a specific action (approve a request, resolve an alert, respond to a review)
- RISK_ISSUE: describes a security breach, system failure, compliance violation, or high-severity alert
- FYI: informational status updates, delivery confirmations, build results, routine system events
- INTERNAL_COMMS: policy notices, compliance reminders, or IT communications sent to users
- PROJECT_UPDATE: CI/CD results, deployment notifications, issue tracker updates, PR notifications, code review comments
- OTHER: does not fit any category above

Rate importance: HIGH, MEDIUM, LOW

For PURE SYSTEM NOTIFICATIONS:
- LOW: routine device check-ins, MDM heartbeats, successful build notifications, non-critical status pings
- MEDIUM: deployment completions, ticket assignments, review requests, service degradation warnings
- HIGH: security breach alerts, critical system failures, compliance violations, approval requests with deadlines

For HYBRID NOTIFICATIONS:
- LOW: brief acknowledgments, minor comments with no decision or action content
- MEDIUM: substantive questions, suggestions, or discussion that provides context but requires no action
- HIGH: decisions, action requests, risk flags, or substantive project discussion requiring follow-up

Assess content_contribution (how much NEW information the message body contributes):

For PURE SYSTEM NOTIFICATIONS:
- HIGH: specific incident details, breach data, action requirements, or non-routine status
- MEDIUM: structured system data mixed with boilerplate template text
- LOW: template-heavy with only minor dynamic values (e.g. a device serial number in a check-in ping)
- NONE: fully automated notifications with no human-written content and no required action

For HYBRID NOTIFICATIONS (human-written body delivered via notification wrapper):
- Assess content_contribution based on the HUMAN-WRITTEN CONTENT, not the notification wrapper
- HIGH: substantive comment, question, decision, risk flag, or action request from the author
- MEDIUM: relevant but lightweight comment providing context without requiring action
- LOW: brief acknowledgment, "+1", or minor note with minimal information value
- NONE: empty or fully boilerplate (rare for human-written notifications)

Key question: Is this telling me about a routine automated event, or does a human need to act on this?
If human-written content is present, evaluate it on its own merits.

Respond ONLY with JSON:
{"category": "...", "importance": "...", "reason": "one sentence", "content_contribution": "...", "contribution_reason": "one sentence"}$prompt$,
    description = 'Triage prompt v2 — notification pipeline variant, updated (pf-1c083d): correct framing for hybrid notifications (Google Docs comments, GitHub reviews) that embed human-written content within notification wrappers'
WHERE stage = 'triage'
  AND version = 2;
