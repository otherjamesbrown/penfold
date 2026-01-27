# Autonomy Guidelines

**This is 100% AI-driven development. James is not writing code, not reviewing code, not running commands. You do the work.**

---

## The Two Rules

### Rule 1: Plan First, Then Execute

For **significant changes** (new features, refactors, redesigns):

1. **Propose** - Share your high-level plan (bullet points, not code)
2. **Align** - Discuss and refine with James
3. **Execute** - Once aligned, complete the work without interruption

**Example - Good:**
```
I'd suggest these changes to /review-arch:
1. Reduce from 8 passes to 5 (consolidate)
2. Add quick mode for weekly checks
3. Add regression detection vs previous reviews
4. Make it Penfold-specific

Does this approach work?
```

**Example - Bad:**
```
Let me rewrite it: [immediately starts coding]
```

### Rule 2: Execute Autonomously

For **known work** (implementing an agreed plan, bug fixes, routine tasks):

- Don't ask "Should I continue to phase 2?" - Just continue.
- Don't ask "Do you want to review the code?" - Complete the work.
- Don't ask "I've finished X, should I proceed?" - Move to the next step.

**Just execute.** Once the plan is agreed, complete it end-to-end.

---

## When to Propose vs Execute

| Propose First | Execute Directly |
|---------------|------------------|
| Redesigning a command or feature | Implementing an agreed spec |
| Multiple valid approaches exist | Clear single path forward |
| Significant refactoring | Bug fixes, small changes |
| New patterns or architecture | Following established patterns |
| "Can we improve X?" requests | "Fix the bug in X" requests |
| Unclear or open-ended tasks | Well-defined tasks with beads |

**Rule of thumb:** If you're about to write "Let me rewrite/redesign this", pause and propose first.

---

## What Goes in a Proposal

Keep it brief - bullet points, not essays:

1. **What** - The key changes (3-6 bullets)
2. **Why** - Brief rationale for each
3. **Impact** - What improves, what changes

Don't include:
- Actual code
- Detailed implementation steps
- Long explanations

James will either approve, adjust, or ask questions. Then execute.

---

## Continue Autonomously For

Once aligned (or for routine work):

- Writing code and tests
- Running commands, tests, builds
- Making implementation decisions
- Following established patterns
- Committing and pushing changes
- Bug fixes within current work
- Moving between phases of a feature
- Spawning sub-agents to do work

---

## Architecture Changes

**DO ask before:**
- Adding new infrastructure components (auth, caching, queues, etc.)
- Modifying ARCHITECTURE.md
- Creating systems that might duplicate existing ones

**Search first** - if similar infrastructure exists, use it or integrate with it.

---

## Multi-Agent Coordination

When work spans domains:

1. **Create handoff beads** - Don't just mention it, create the bead
2. **Stay in your domain** - Don't modify files outside your responsibility
3. **Document in the bead** - What needs doing and why

```bash
bd create --title="Handoff: description" --type=task
bd update <id> --assignee=target-agent
```
