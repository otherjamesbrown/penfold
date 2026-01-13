# Session Handoff

Create a handoff bead to preserve context for resuming work in a new session.

## Arguments: $ARGUMENTS

Optional: Brief description of why you're handing off (e.g., "end of day", "context full", "switching agents")

## Instructions

### Step 1: Check for Existing Handoffs

First, check if there's already an open handoff for this branch:

```bash
BRANCH=$(git rev-parse --abbrev-ref HEAD)
bd list --label handoff --label $BRANCH --status open
bd list --label handoff --label $BRANCH --status in_progress
```

If existing handoff(s) found, ask the user whether to close them or update instead.

### Step 2: Gather Context

Collect all relevant information from the current session:

1. **What was the goal?** - Original task/problem being worked on
2. **What was done?** - Completed steps, commits made, files changed
3. **What's blocking/remaining?** - Unfinished work, blockers, next steps
4. **Key findings** - Root causes discovered, important decisions made
5. **Related beads** - Any beads created or worked on this session
6. **Agent domain** - Which agent domain this work belongs to (database-dev, ai-dev, etc.)
7. **Architectural decisions** - Any patterns discovered relevant to context/ARCHITECTURE.md

### Step 3: Create Handoff Bead

```bash
bd create \
  --title "Handoff: $BRIEF_SUMMARY" \
  --type task \
  --priority 1 \
  --labels "handoff,$BRANCH_NAME"
```

Then update the description with full context using bd update or bd comments.

### Step 4: Add Penfold-Specific Context

Include relevant Penfold context in the handoff description:

- **Current specification** (if working on specs/001-011)
- **Implementation phase** (SpecKit → Implementation → Consolidation)
- **Agent responsibilities** and domain boundaries
- **Architecture implications** of the work
- **Cross-agent dependencies** that might be affected

### Step 5: Output Summary

```
Handoff Bead:    $HANDOFF_BEAD_ID
Branch:          $BRANCH_NAME
Agent Domain:    $AGENT_DOMAIN (if applicable)
Specification:   $CURRENT_SPEC (if applicable)

To resume in new session:
  /pickup

Or manually:
  bd list --label handoff --label $BRANCH_NAME
  bd show $HANDOFF_BEAD_ID
```

### Step 6: Session Close Protocol

Remind about mandatory session close steps:

```
⚠️  BEFORE ENDING SESSION - MANDATORY:
   git status              # Check what changed
   git add <files>         # Stage changes
   bd sync                 # Sync beads
   git commit -m "..."     # Commit code
   git push                # MUST PUSH TO REMOTE
```