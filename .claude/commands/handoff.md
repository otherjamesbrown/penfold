# Session Handoff

Create a handoff shard to preserve context for resuming work in a new session.

## Arguments: $ARGUMENTS

Optional: Brief description of why you're handing off (e.g., "end of day", "context full", "switching agents")

## Instructions

### Step 1: Check for Existing Handoffs

First, check if there's already an open handoff for this branch:

```sql
-- Check for open handoff shards
SELECT id, title, status FROM shards
WHERE project = 'penfold'
  AND title LIKE '%Handoff%'
  AND status != 'closed';
```

**Connection:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

If existing handoff(s) found, ask the user whether to close them or update instead.

### Step 2: Gather Context

Collect all relevant information from the current session:

1. **What was the goal?** - Original task/problem being worked on
2. **What was done?** - Completed steps, commits made, files changed
3. **What's blocking/remaining?** - Unfinished work, blockers, next steps
4. **Key findings** - Root causes discovered, important decisions made
5. **Related shards** - Any shards created or worked on this session
6. **Agent domain** - Which agent domain this work belongs to (database-dev, ai-dev, etc.)
7. **Architectural decisions** - Any patterns discovered relevant to context/ARCHITECTURE.md

### Step 3: Create Handoff Shard

```sql
SELECT create_shard('penfold',
  'Handoff: $BRIEF_SUMMARY',
  '## Session Goal
<goal>

## Progress Made
<completed steps>

## Remaining Work
<next steps>

## Key Findings
<discoveries>

## Related Shards
- pf-xxx: description
- pf-yyy: description

## Architecture Notes
<any relevant patterns>',
  'task',
  'agent-penfdev');
```

### Step 4: Add Penfold-Specific Context

Include relevant Penfold context in the handoff content:

- **Current specification** (if working on specs/001-011)
- **Implementation phase** (SpecKit → Implementation → Consolidation)
- **Agent responsibilities** and domain boundaries
- **Architecture implications** of the work
- **Cross-agent dependencies** that might be affected

### Step 5: Output Summary

```
Handoff Shard:    pf-xxx
Branch:          $BRANCH_NAME
Agent Domain:    $AGENT_DOMAIN (if applicable)
Specification:   $CURRENT_SPEC (if applicable)

To resume in new session:
  /pickup

Or manually:
  SELECT * FROM shards WHERE title LIKE '%Handoff%' AND status != 'closed';
  SELECT * FROM shards WHERE id = 'pf-xxx';
```

### Step 6: Session Close Protocol

Remind about mandatory session close steps:

```
⚠️  BEFORE ENDING SESSION - MANDATORY:
   git status              # Check what changed
   git add <files>         # Stage changes
   git commit -m "..."     # Commit with shard reference [pf-xxx]
   git push                # MUST PUSH TO REMOTE
```

(No sync needed - shards are always live in Context-Palace)
