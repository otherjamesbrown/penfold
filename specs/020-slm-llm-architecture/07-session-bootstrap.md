# Penfold: Session Bootstrap

## Purpose

The morning briefing is how Claude reconstructs its working memory at the start of every session. This document specifies it as a first-class component.

## Design Philosophy

The morning briefing is a **playbook** — a markdown document that Claude reads and follows step by step, running specific `penf` commands as it goes. It is not a single monolithic command. Each step is a small, focused query. Claude uses judgment between steps: skip steps that return nothing interesting, flag things that need attention, ask the user where to go next.

The playbook is **user-editable through conversation**. The user says "can you add a standup recap to my morning briefing?" and Claude adds a step. The user says "make it top 3 projects instead of 5" and Claude edits the relevant step. The preferences _are_ the playbook — there's no separate config.

Think of it as a good chief of staff's morning checklist that evolves over time based on what the user actually cares about.

---

## The Playbook

The playbook is stored as a Context-Palace shard (type `briefing_playbook`). Claude reads it at the start of every session and follows it. Here is the default playbook:

```markdown
# Morning Briefing

Follow these steps in order. Run each command, read the output, and summarise
conversationally. Skip steps that return empty results. Don't dump raw data —
give me highlights and ask where I want to drill in.

## 1. Last session

Run: `penf session resume --last-closed --format json`

Remind me briefly what we covered last time. If no session exists, say so and
move on.

## 2. Overnight processing

Run: `penf pipeline status --since-last-session --format json`

Tell me how many emails and meetings were processed overnight. If there are
watch list changes or seniority escalations, flag them specifically.

## 3. My meetings yesterday

Run: `penf meetings list --participant me --since yesterday --format json`

Tell me how many meetings I had. If any have assertion changes (new risks,
decisions, actions), flag those by name. Offer to recap any of them or drill
into the associated projects.

## 4. Meetings I wasn't in

Run: `penf meetings list --not-participant me --has-changes --projects active --since yesterday --format json`

If any meetings come back, tell me about them — especially if senior people
attended or assertions changed. If none, skip this step entirely.

## 5. Reminders

Run: `penf reminders list --due --format json`

Surface any due reminders. If there are none, skip.

## 6. Projects

Show me the top 3 most active projects. Always include MTC 2026 even if
it's not in the top 3.

Run: `penf projects list --status active --sort activity --limit 3 --always-include "MTC 2026" --format json`

Tell me which have changes and which are quiet.

## 7. Daily standup recap

Run: `penf meetings recap --series "Daily Standup" --most-recent --format json`

Give me a brief recap of yesterday's standup so I remember what the team is
working on.

## 8. Upcoming meetings

Run: `penf calendar upcoming --hours 4 --format json`

If I have a meeting coming up, offer to prep me with context on the associated
product or project. If calendar integration isn't available, skip.

## 9. Ask me

Present a short summary of the highlights and ask: "Where do you want to start?"

Offer specific options based on what came up:
- Recap a specific meeting
- Drill into a project
- Follow up on a reminder
- Prep for an upcoming meeting
```

---

## How the Playbook Gets Modified

The user modifies the playbook through natural conversation. Claude reads the current playbook, applies the requested change, and writes it back.

### Example: changing project count

> **User:** "For my morning briefing, you currently give me the top 3 projects. Can you make that just the top 3 but always include the MTC project?"
>
> **Claude:** reads the playbook, edits step 6, writes it back. "Done — step 6 now shows the top 3 by activity, always including MTC."

### Example: adding a new step

> **User:** "Can you also give me a recap of the previous day's daily standup, so I can remember what we're working on?"
>
> **Claude:** reads the playbook, adds a new step 7, writes it back. "Done — I've added a daily standup recap to your morning briefing."

### Example: removing a step

> **User:** "I don't need the calendar check any more, I use my phone for that."
>
> **Claude:** reads the playbook, removes the calendar step, writes it back. "Done — removed the upcoming meetings check."

### Example: asking what's configured

> **User:** "What does my morning briefing include?"
>
> **Claude:** reads the playbook and explains it in natural language. "Your morning briefing runs through 9 steps: it checks your last session, overnight processing, your meetings, meetings you missed, reminders, top 3 projects (always including MTC), a standup recap, upcoming calendar, then asks where you want to start."

### Storage

```sql
-- Create the playbook
SELECT create_shard(
  'penfold',
  'Morning Briefing Playbook',
  '<the markdown content>',
  'briefing_playbook',
  'agent-mycroft'
);

-- Read the current playbook
SELECT content FROM shards
WHERE project = 'penfold'
  AND type = 'briefing_playbook'
  AND owner = 'agent-mycroft'
  AND status = 'open'
ORDER BY updated_at DESC
LIMIT 1;

-- Update the playbook
UPDATE shards SET content = '<updated markdown>'
WHERE id = 'pf-xxx';
```

Only one active playbook exists per agent. If none exists, Claude uses the default playbook above and offers to store it.

---

## The `penf` Commands Referenced in the Playbook

Each step runs a specific command. These are lightweight, focused queries — not monolithic endpoints.

| Command | What it returns |
|---------|----------------|
| `penf session resume --last-closed` | Most recent closed session: summary and checkpoints |
| `penf pipeline status --since-last-session` | Counts: emails/meetings processed, items needing attention, watch list changes, escalations |
| `penf meetings list --participant me --since yesterday` | List of meetings the user attended, with `has_changes` flag |
| `penf meetings list --not-participant me --has-changes --projects active --since yesterday` | Meetings on tracked projects the user wasn't in, filtered to those with assertion changes |
| `penf reminders list --due` | Reminders that are due now or overdue |
| `penf projects list --status active --sort activity --limit N` | Active projects sorted by recent activity, with change flag |
| `penf meetings recap --series "X" --most-recent` | Brief summary of the most recent instance of a meeting series |
| `penf calendar upcoming --hours N` | Upcoming calendar events (future — requires calendar integration) |

Each command returns JSON with `--format json`. Claude reads the JSON and translates it into natural conversation. The commands themselves are simple — they don't need to know about the playbook or the briefing context.

### Commands available for drill-down (not part of the playbook, called on demand)

| User says | Claude calls |
|-----------|-------------|
| "Recap the TER Weekly" | `penf meetings recap --source-id src-m001` |
| "Tell me about MTC" | `penf context project --name "MTC 2026"` |
| "What's the VxLAN situation?" | `penf assertion briefing --root-id 101` |
| "What's Dan involved in?" | `penf context person --name "Dan Spataro"` |
| "What changed on my watch list?" | `penf context changes --watched-only` |
| "What about TitanMail?" | `penf context product --name "TitanMail"` |

---

## Failure Handling

Since the playbook runs multiple independent commands, failure is granular. If one command fails, Claude notes it and moves to the next step.

**Command fails:** Claude says "I couldn't load your meetings — the gateway might be down. Moving on." and continues with the next step.

**Gateway entirely unreachable:** Claude detects this on the first command that hits the gateway and says: "The gateway is unreachable. I can check your session notes and reminders (those are in Context-Palace), but pipeline data won't be available. Want me to retry or work with what I have?"

**Stale pipeline data:** The `penf pipeline status` command includes a `data_as_of` timestamp. If it's more than 12 hours old, Claude flags it: "Pipeline data looks stale — last update was 18 hours ago. Might want to check the worker."

**No previous session:** The `penf session resume --last-closed` command returns empty. Claude says "I don't have notes from a previous session" and moves on.

---

## Archived Projects and Products

Projects and products can be marked as `archived`. Archived items:
- **Remain in the database** — all assertions, references, and history are preserved
- **Are excluded from the playbook queries** — `--status active` filters them out
- **Are excluded from Stage 4 context** — the pipeline doesn't load their assertions as background
- **Are still searchable** — `penf search` finds content in archived projects
- **Can be drilled into on demand** — `penf context project --name "Old Project"` still works
- **Can be unarchived** — status change, not deletion

### Schema changes needed

The `projects` table currently has no `status` column. The `product_status` enum exists but lacks `archived`.

```sql
-- Add status to projects
CREATE TYPE project_status AS ENUM ('active', 'archived');
ALTER TABLE projects ADD COLUMN status project_status NOT NULL DEFAULT 'active';
CREATE INDEX idx_projects_status ON projects (status);

-- Add archived to product_status
ALTER TYPE product_status ADD VALUE 'archived';
```

The `--always-include` flag in `penf projects list` overrides the archive filter. If the playbook says `--always-include "MTC 2026"` and MTC is archived, it still appears (with an `archived` flag so Claude can mention it).

---

## Reminders Storage

Reminders are stored as Context-Palace shards with type `reminder`:

```sql
SELECT create_shard(
  'penfold',
  'Reminder: dig into MTC staffing risk',
  '{"text": "Dig into MTC staffing risk", "remind_after": "2026-02-04T07:00:00Z", "context_project": "MTC 2026"}',
  'reminder',
  'agent-mycroft'
);
```

The `penf reminders list --due` command queries:

```sql
SELECT id, title, content
FROM shards
WHERE project = 'penfold'
  AND type = 'reminder'
  AND status = 'open'
  AND (
    content::jsonb->>'remind_after' IS NULL
    OR (content::jsonb->>'remind_after')::timestamptz <= NOW()
  )
ORDER BY created_at ASC
LIMIT 10;
```

Once surfaced in the morning briefing, a reminder is marked `closed`:

```sql
SELECT close_task('pf-rem001', 'Surfaced in morning briefing 2026-02-04');
```

If the user wants to be reminded again, Claude creates a new reminder with an updated `remind_after`.

---

## Session Flow

```
1. Claude reads the briefing playbook from Context-Palace
2. Claude follows each step, running penf commands and summarising
3. Claude asks "Where do you want to start?"
4. User picks a direction → Claude drills in with follow-up commands
5. penf session start "Morning"  → Session tracking begins
6. ... interactive work ...
7. penf session end "Summary"    → Summary persisted for next briefing
```

---

## Size and Performance

Each playbook command returns a small, focused response. No single command needs to return more than ~500 tokens. The total data loaded across all steps is roughly what the old monolithic command returned (~1,250 tokens), but spread across multiple calls.

The tradeoff: multiple round-trips to the gateway instead of one. This is acceptable because:
- Each call is fast (simple queries, small results)
- Claude processes results between calls (natural conversation flow)
- The user sees progress as each step completes, not a single long wait
- Failed steps don't block the rest

---

## Implementation Notes

### What needs building

| Command | Exists? | Notes |
|---------|---------|-------|
| `penf session resume --last-closed` | Partial | `session resume` exists but loads active session, not last closed |
| `penf pipeline status --since-last-session` | No | New command — queries pipeline processing log |
| `penf meetings list` | No | New command — queries sources table with filters |
| `penf meetings recap` | No | New command — returns meeting summary and extracted assertions |
| `penf reminders list --due` | No | New command — queries Context-Palace shards |
| `penf projects list --status active --sort activity` | Partial | `projects` commands exist but need activity sorting and `--always-include` |
| `penf calendar upcoming` | No | Future — requires calendar integration |

### Playbook as CLAUDE.md alternative

The playbook could also live as a file in the repo (e.g., `context/morning-briefing.md`) rather than in Context-Palace. The advantage of Context-Palace is that Claude can update it during conversation without a git commit. The advantage of a file is visibility and version control. Either works — the format is the same.
