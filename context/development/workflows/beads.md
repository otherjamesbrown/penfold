# Beads Workflow

**NEVER start work without a bead.**

## Recognizing Bead References

**Bead IDs in this project use the format `pe-<xxx>`** (e.g., `pe-t3st`, `pe-0ilh`).

**`bd` is a standalone CLI tool** (not part of `penf`). Run it directly: `bd show`, not `penf bd` or `./penf bd`.

When someone asks you to "work on pe-xxx" or references a `pe-` ID:

1. **Get the bead details:** `bd show pe-xxx`
2. **Read the description** - understand what's being asked
3. **Check status** - is it already in progress?
4. **Start work** - `bd update pe-xxx --status=in_progress`

Do NOT search for files matching the bead ID. Beads are tracked in `.beads/` and accessed via the `bd` CLI.

---

## Essential Commands

```bash
bd ready                              # Find available work
bd create --title="..." --type=task   # Create new work
bd update <id> --status=in_progress   # Claim work
bd close <id> --reason="summary"      # Complete work
bd sync                               # Sync with git
```

## Critical Rules

1. **Find or create bead BEFORE writing code**
2. **Assign agent when creating**: `bd update <id> --assignee="cli-dev"`
3. **Update status when starting**: `bd update <id> --status=in_progress`
4. **Reference bead in commits**: `feat(component): description [pe-xxx]`
5. **Close with commit hash**: `bd close <id> --reason="commit <hash>: summary"`
6. **Run `bd sync` before ending session**

## Agent Assignment

When creating beads, always specify which agent should do the work:

```bash
# Create bead with agent assignment
bd create --title="Fix search help text" --type=task
bd update <id> --assignee="cli-dev"

# Or for investigation work
bd create --title="Investigate flaky test" --type=bug
bd update <id> --assignee="debugger"
```

| Work Type | Assign To |
|-----------|-----------|
| CLI commands, help text, CLI docs | `cli-dev` |
| Database, migrations, repositories | `data-dev` |
| AI/ML features, embeddings | `ai-dev` |
| Temporal workflows, activities | `worker-dev` |
| Gmail connector, OAuth | `gmail-dev` |
| Bug investigation, root cause | `debugger` |
| Test framework, fixtures | `testing-dev` |
| Feature specs, planning | `speckit-dev` |

## After Closing a Bead

When closing a bead that belongs to an epic, check if all sibling beads are also closed:

```bash
bd dep tree <epic-id>                 # Check if all children are closed
```

If all child beads are closed, suggest closing the parent epic to the user.

---

## Epic Management

**ALL beads must be associated with an epic to prevent proliferation.**

### Epic Structure

```bash
bd list --type=epic                   # Show all epics
bd dep add <bead-id> <epic-id>        # Associate bead with epic
bd dep tree <epic-id>                 # Show epic's bead tree
```

### Epic Creation Rules

**BEFORE creating any new bead:**
1. **Check existing epics** - Does it fit in current epic?
2. **Epic first** - Create epic before related beads
3. **Batch related work** - Group similar beads under same epic
4. **Link immediately** - `bd dep add <new-bead> <epic-id>`

### When to Create New Epics

- **Major feature areas** (5+ related beads expected)
- **Cross-cutting initiatives** (affects multiple systems)
- **Implementation phases** (setup → implement → polish)
- **Maintenance categories** (quarterly tasks, cleanup)

### Rare Non-Epic Exceptions

**ONLY create standalone beads for:**
- Immediate blockers preventing all work
- Emergency security issues
- Quick research tasks (<30 min, inform epic planning)
- Epic creation itself

### Epic Naming Convention

- `[EPIC] SpecKit: Complete All Feature Specifications`
- `[EPIC] Operationalization: Dev Agents and Documentation`
- `[EPIC] Integration: Cross-Cutting System Concerns`
- `[EPIC] Maintenance: Cleanup and Audits`
