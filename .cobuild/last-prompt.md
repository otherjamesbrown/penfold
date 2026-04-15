# Task: Recurring Gmail sync — establish canonical scheduling path for james-personal (and CyberSentriq)

**Task ID:** pf-830415
**Agent:** 

## Task Content

## Goal

Establish a recurring (hourly) Gmail sync for tenant `b857823a-362d-469a-bf44-df3f16fbf9c1` (james-personal). Today no Gmail sync runs unless `penf ingest gmail sync` is triggered manually. Last sync was 2026-04-08; the Healthcare digest at 07:00 today produced near-empty output because there was no fresh email to summarise (separate symptom of the same root cause).

This needs to be solved before the CyberSentriq tenant is connected — auto-classification tuning depends on a steady stream of fresh content.

## Architectural decision required (do not skip)

Two candidate scheduling mechanisms exist; the chosen one becomes the canonical path going forward.

### Option 1 — Wrap `SyncEmails` in a thin Temporal workflow (RECOMMENDED)

- Create `GmailSyncTickerWorkflow` (or extend an existing one) that performs a single activity: invoke the existing gateway `GmailConnectorService.SyncEmails` RPC for the given tenant.
- Schedule it via `ScheduleService.CreateSchedule` (`services/gateway/scheduleservice/service.go`) with cron `0 * * * *`.
- Workflow params: `{"tenant_id": "..."}` only — matches `SyncEmailsRequest` shape.
- **Pros:** Reuses the production code path that we know works (`SLMPipelineWorkflow` per-message on `penfold-main`). Tenant-scoped OAuth (per migration 139) "just works" because that's what `SyncEmails` already does.

### Option 2 — Schedule `GmailSyncWorkflow` directly

- Already registered at `services/worker/workflows/register.go:143` on task queue `penfold-email`.
- **Blocker:** `GmailSyncInput` (`pkg/temporal/types.go:184`) requires `user_id`, but production OAuth is tenant-scoped (no per-user concept; see `gmail_sync_state` schema in migration 157). No CLI/DB discovery path for `user_id` — likely needs to be `"me"` against the tenant's OAuth principal, but this is untested.
- **Risk:** `GmailSyncWorkflow` has zero invocations in production today. The manual `penf ingest gmail sync` does not use it. Scheduling it would be the first real-world fire.

**Recommend Option 1.** If Option 2, the user_id semantics need to be resolved as part of this task and `GmailSyncWorkflow` needs an integration test against a real tenant.

## Prerequisites (must verify before any schedule fires)

1. **Worker not running on dev01.** `launchctl print gui/501/com.penfold.worker` returns "Could not find service". Plist exists at `~/Library/LaunchAgents/com.penfold.worker.plist`. No Temporal workflow executes without a worker — any schedule created now would queue indefinitely.
   - Fix: load via launchd; document in deploy/launchd why it isn't auto-loaded on dev01 boot if intentional.
2. **Gmail OAuth confirmed for james-personal.** `penf ingest gmail sync` works manually for this tenant, so OAuth is valid. Verify the operational config row exists in `pipeline_operational_config` for tenant_id `b857823a-362d-469a-bf44-df3f16fbf9c1` with `email.oauth_*` keys (migration 139 only seeds these for `c3170310-78bd-409c-b186-126f40bfa6ad` — different tenant).

## Deliverables

1. Decision recorded (Option 1 vs 2) with rationale.
2. Implementation:
   - If Option 1: new ticker workflow + registration + integration test.
   - Schedule created for james-personal at `0 * * * *` (hourly on the hour).
3. Worker confirmed loaded on dev01 (or fix surfaced as a blocker).
4. Operational visibility: at minimum `penf workflow list --type ingestion` should show recent runs. Bonus: implement the stubbed `GetCurrentGmailStatus` and `GetGmailSyncHistory` endpoints (they currently return "not yet implemented" — see CLI gaps below).
5. Pattern documented so adding the same schedule for new tenants (CyberSentriq) is a one-liner.

## Acceptance

- `penf schedule list` for james-personal shows a `gmail-sync-hourly` cron schedule, enabled.
- After waiting one cron cycle, `penf workflow list --type ingestion` shows a completed run.
- New emails landing in the Gmail inbox appear in `penf project content <name>` within ~1 hour of arrival, without manual intervention.
- Pattern script or `penf` subcommand for "enable Gmail sync schedule on tenant X" so this is repeatable.

## Related findings (filed separately, do not bundle)

- `pf-dbe0f9` — `penf rule history` 500 from delivery_status unmarshal.
- `pf-dd2ac6` — Skip Gmail TRASH on ingestion.
- `pf-7e0198` — `project content` should use email/meeting date, not ingest date.
- KB shard (newly created) "Gmail Sync — Scheduling & Architecture" captures the two-path split, scaffolding scheduler, OAuth tenancy model, worker deployment notes — read this before starting work.

## File:line evidence

- Production sync path: `services/gmail/server/server.go:279-306` (starts `SLMPipelineWorkflow` per message on `penfold-main`).
- Gmail proxy at gateway: `services/gateway/gmailproxyservice/service.go:32-41` (`SyncEmails`).
- Scheduling RPC: `services/gateway/scheduleservice/service.go` (`CreateSchedule`).
- `GmailSyncWorkflow` definition: `services/worker/workflows/gmail_sync.go:133-143`.
- Workflow registration: `services/worker/workflows/register.go:143-145` (task queue `penfold-email`).
- Workflow input: `pkg/temporal/types.go:184-194` (note `user_id` requirement).
- Tenant-scoped OAuth: `migrations/139_email_config.sql`.
- Sync state schema (no user_id): `migrations/157_gmail_sync_state.sql`.
- Scaffolding scheduler (do not use): `services/gmail/scheduler/scheduler.go:714` (`processTask` is a stub).
- CLI sync command: `penf ingest gmail sync --help` (this is what's run manually today).


## Instructions

**Fix this bug.**

Follow the fix-bug skill:
1. Read the bug report
2. Check escalation criteria — if any apply, add `needs-investigation` label and stop
3. Reproduce, investigate lightly, append findings to the bug
4. Implement the fix, run tests, build
5. Commit — the Stop hook will run `cobuild complete`
6. After the Stop hook completes, your work is done — stop. The dispatch runner cleans up the session itself; do NOT type `/exit` (it's a REPL-only command and will be rendered as text in your final message rather than terminating the process — see cb-e619cb / cb-eaef03).
