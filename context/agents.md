# Penfold Agent Rules

> **Last verified**: 2026-01-13 | **Project**: Penfold AI Information System

---

## Development Agents Available

| Agent | Domain | Handoff To |
|-------|--------|------------|
| `database-dev` | Storage, migrations, performance, vector operations | ai-dev (events), search-dev (vector queries) |
| `ai-dev` | Model integration, pub-sub processing, event coordination | database-dev (storage), integration-dev (triggers) |
| `integration-dev` | Gmail, meeting pipelines, external system connectors | ai-dev (processing), database-dev (ingestion) |
| `search-dev` | Query interface, vector search, retrieval systems | database-dev (indexes), ai-dev (query processing) |
| `automation-dev` | Rule engine, workflow automation, daily review | ai-dev (logic), search-dev (queries) |
| `testing-dev` | Test framework, AI mocking, performance testing | All agents (test support) |
| `debugger` | Read-only investigation, root cause analysis | Domain agents (fixes) |

---

## 🚨 AUTONOMOUS DEVELOPMENT - CRITICAL

**This is 100% AI-coding assistant driven development. Continue implementing autonomously until completion or you need user clarification on business requirements.**

**CONTINUE AUTONOMOUSLY for:**
- Writing code and tests
- Making technical implementation decisions
- Running commands and tests
- Committing and pushing changes
- Following established patterns and specifications

**ONLY ASK USER when:**
- Business requirements are ambiguous
- Multiple valid approaches exist and user preference is needed
- You need external credentials or resources
- Technical blockers require user intervention

## Critical Rules

**NEVER:**
- Start work without a bead
- Close a bead with just "Done" - include commit hash and summary
- Work outside your agent domain without creating a handoff bead
- Ship new code without tests
- Exceed 30 minutes without documenting progress in bead
- Say "ready to push when you are" - YOU must push
- Stop for permission to write code, create files, or make technical decisions

**ALWAYS:**
- Create or find a bead BEFORE writing code
- Update bead status: `bd update <id> --status in_progress`
- Update beads with progress as you work (for crash/context recovery)
- Reference bead in commits: `fix(component): description [pe-xxx]`
- Close bead with details: `bd close <id> --reason "commit <hash>: <summary>"`
- Write tests for new functionality
- Run tests before closing bead
- Push work to remote before ending session

---

## Domain Boundaries

**Agents only write code for areas they are responsible for.**

If work is needed for another agent:
1. Create handoff bead: `bd create --title="Handoff: description" --type=task`
2. Assign to target agent: `bd update <id> --assignee=target-agent`
3. Add domain label: `bd update <id> --add-label="agent:database-dev"`
4. Document what needs to be done and why

**Never modify files outside your domain without explicit handoff.**

---

## Before Starting Work

```bash
# 1. Check for blockers to new work
bd list --status open --priority 0    # Any P0? Fix first!
bd list --status open --priority 1    # P1s >7 days? Address first.
bd list --status in_progress          # Already ≥3? Finish something.

# 2. Find existing bead or create new one
bd ready                    # Find unblocked tasks
bd list --status open       # All open issues
bd create --title="..." --type=task

# 3. Claim the work
bd update <id> --status in_progress
```

**Cannot start new work if:**
- Any P0 exists (fix it first)
- You have ≥3 independent work streams in_progress (finish something first)
- A P1 has been open >7 days (address it)

---

## While Working

1. **Stay in your domain** - see Agent Domains above
2. **Document progress** - add comments to bead for significant findings
3. **Follow project principles** - see ARCHITECTURE.md
4. **Update bead every 15-30 minutes** with progress notes

Example progress update:
```bash
bd comments add pe-abc "Working on database migration for multi-tenancy.
Created tenant table, now adding RLS policies.
Next: Update existing entities to include tenant_id."
```

---

## When Done

```bash
# 1. Run tests
pytest tests/ -v

# 2. Commit with bead reference
git commit -m "feat(database): add multi-tenant RLS policies [pe-abc]"

# 3. Close bead with commit hash
bd close pe-abc --reason "commit abc1234: implemented multi-tenant RLS policies for all core entities"

# 4. Create handoff beads if needed
bd create --title="Handoff: Update AI event processing for multi-tenancy" --type=task
bd update <new-id> --assignee=ai-dev --add-label="agent:ai-dev"
```

---

## Session Close Protocol

**CRITICAL**: Work is NOT complete until pushed to remote.

```bash
# MANDATORY before saying "done":
git status                  # Check what changed
git add <files>             # Stage changes
bd sync                     # Commit beads changes
git commit -m "..."         # Commit code
git pull --rebase           # Get any remote changes
git push                    # PUSH TO REMOTE
git status                  # MUST show "up to date with origin"
```

**Rules:**
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
- Create beads for any remaining work before ending

---

## Agent Context Loading

### Main Agent
1. Read CLAUDE.md (entry point)
2. Read context/agents.md (this file)
3. Read ARCHITECTURE.md (system design)

### Domain Agent
1. Read context/agents.md (always)
2. Read context/<domain>/agents.md (domain-specific rules)
3. Read relevant ARCHITECTURE.md sections

---

## Spawn Triggers

Create handoff bead and spawn agent when:

| Trigger | Spawn Agent |
|---------|-------------|
| Bug is complex or >30 min unresolved | `debugger` |
| Need root cause analysis before fix | `debugger` |
| Storage/migration issue | `database-dev` |
| AI model integration problem | `ai-dev` |
| External system integration issue | `integration-dev` |
| Search or query performance issue | `search-dev` |
| Automation rule or workflow issue | `automation-dev` |
| Test framework or mocking issue | `testing-dev` |

---

## Completion Checklist

Before reporting complete:
- [ ] Bead exists and is in_progress
- [ ] Tests written for new functionality
- [ ] Tests pass: `pytest tests/ -v`
- [ ] Commits reference bead ID
- [ ] Bead closed with commit hash and summary
- [ ] Handoff beads created if needed
- [ ] Work pushed to remote
- [ ] Git status shows "up to date with origin"

---

## Report Format

When completing work, report:

```markdown
**Bead**: pe-xxx (closed)

**Summary**: What was accomplished

**Commits**: `abc1234`: description [pe-xxx]

**Files Changed**: path/to/file.py

**Tests**: Added/updated (or "N/A - no new functionality")

**Handoffs**: Beads created for other agents (or "None")

**Domain**: Confirmed work stayed within <agent> domain
```

---

## Reference Documents

| What | Where |
|------|-------|
| System architecture | ARCHITECTURE.md |
| Beads workflow | context/beads.md |
| Database agent context | context/database-dev/agents.md |
| AI agent context | context/ai-dev/agents.md |
| Integration agent context | context/integration-dev/agents.md |
| Search agent context | context/search-dev/agents.md |
| Automation agent context | context/automation-dev/agents.md |
| Testing agent context | context/testing-dev/agents.md |
| Debugger agent context | context/debugger/agents.md |