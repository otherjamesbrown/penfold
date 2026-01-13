# Multi-Agent System Rules

> **Context**: This file is only loaded for multi-agent coordination work
> **Universal Rules**: See CLAUDE.md for autonomous development, beads workflow, session management
> **Last verified**: 2026-01-13

---

## When This Context Applies

This file is loaded when:
- Spawning specialized agents for complex work
- Coordinating work across multiple agent domains
- Managing handoffs between development specializations

For single-session work, CLAUDE.md contains all essential rules.

---

## Development Agent Domains

| Agent | Domain | Handoff To |
|-------|--------|------------|
| `database-dev` | Storage, migrations, performance, vector operations | ai-dev (events), search-dev (vector queries) |
| `ai-dev` | Model integration, pub-sub processing, event coordination | database-dev (storage), integration-dev (triggers) |
| `integration-dev` | Gmail, meeting pipelines, external system connectors | ai-dev (processing), database-dev (ingestion) |
| `search-dev` | Query interface, vector search, retrieval systems | database-dev (indexes), ai-dev (query processing) |
| `automation-dev` | Rule engine, workflow automation, daily review | ai-dev (logic), search-dev (queries) |
| `testing-dev` | Test framework, AI mocking, performance testing | All agents (test support) |
| `debugger` | Read-only investigation, root cause analysis | Domain agents (fixes) |

## Multi-Agent Specific Rules

**NEVER:**
- Work outside your agent domain without creating a handoff bead
- Exceed 30 minutes without documenting progress in bead (for crash recovery)
- **Modify ARCHITECTURE.md without user approval**
- **Create infrastructure that duplicates existing systems**

**ALWAYS:**
- Update beads with progress as you work (for crash/context recovery)
- Create handoff beads when work crosses domain boundaries
- Document what needs to be done and why in handoff beads

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

## Architecture Coordination in Multi-Agent Context

### **Cross-Agent Architecture Changes**

When multiple agents might add similar infrastructure:

```bash
# 1. Create architecture review bead
bd create --title="ARCH REVIEW: Add [component] for [purpose]" --type=review
bd update <id> --add-label="architecture-review"

# 2. Document what exists and what's needed
bd comments add <id> "Current state: [existing solutions]
Proposed: [new component]
Justification: [why needed]
Cross-agent impact: [what other agents affected]"

# 3. STOP and ask user for approval before proceeding
```

### **Multi-Agent Coordination Examples**

```markdown
❌ BAD - Agents work independently:
Database agent: "Adding Prometheus for DB monitoring"
AI agent: "Adding Prometheus for model monitoring"
(Result: Duplicate monitoring infrastructure)

✅ GOOD - Agents coordinate:
Database agent: "Need performance monitoring - checking if observability framework exists"
AI agent: "Same monitoring needs - let's use centralized observability (011)"
```

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