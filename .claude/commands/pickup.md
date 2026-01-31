# Session Pickup

Resume work from a previous session using a handoff shard.

## Instructions

### Step 1: Find Handoff Shards

Look for open handoff shards:

```sql
-- Check for open handoff shards
SELECT id, title, status, created_at FROM shards
WHERE project = 'penfold'
  AND title LIKE '%Handoff%'
  AND status != 'closed'
ORDER BY created_at DESC;
```

**Connection:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

If no handoffs found, check all open work:

```sql
SELECT * FROM tasks_for('penfold', 'agent-penfdev');
```

### Step 2: Display Handoff Context

For each handoff found, show the details:

```sql
SELECT * FROM shards WHERE id = 'pf-xxx';
```

Present a summary:

```
═══════════════════════════════════════════════════════════════
 PENFOLD HANDOFF FOUND
═══════════════════════════════════════════════════════════════

 Handoff Shard:    pf-xxx
 Title:           $TITLE
 Branch:          $BRANCH_NAME
 Agent Domain:    $AGENT_DOMAIN
 Specification:   $CURRENT_SPEC
 Created:         $CREATED_DATE

 ## Session Goal
 $GOAL_FROM_CONTENT

 ## Progress Made
 $COMPLETED_FROM_CONTENT

 ## Remaining Work
 $REMAINING_FROM_CONTENT

 ## Key Findings
 $FINDINGS_FROM_CONTENT

 ## Related Shards
 $RELATED_SHARDS

 ## Architecture Notes
 $ARCHITECTURE_IMPLICATIONS

═══════════════════════════════════════════════════════════════
```

### Step 3: Mark as In Progress

Update the handoff shard to show work has resumed:

```sql
SELECT claim_task('pf-xxx', 'agent-penfdev');
```

### Step 4: Load Penfold Context

Based on the handoff, load relevant Penfold context:

1. **Check current specification status** (if working on specs/001-011)
2. **Review agent domain context** - Load appropriate context/{domain}/agents.md
3. **Check architecture relevance** - Review context/ARCHITECTURE.md for related patterns
4. **Verify autonomous development rules** - Confirm behavior from CLAUDE.md
5. **Review related shards** - Check status of dependent/related work:
   ```sql
   SELECT id, title, status FROM shards WHERE id IN ('pf-xxx', 'pf-yyy');
   ```

### Step 5: Load Relevant Files

Load files mentioned in the handoff:
- Read key implementation files
- Check status of related shards: `SELECT * FROM shards WHERE id = 'pf-xxx';`
- Review recent git history: `git log --oneline -10`
- Check current working directory status: `git status`

### Step 6: Ask What to Do

Use AskUserQuestion to ask:

"What would you like to work on from this handoff?"

Options:
- Continue with remaining work
- Focus on a specific item from the list
- Start new related work
- Review and update the handoff context
- Something else (will ask for details)

### Step 7: Context Summary

Summarize what you've loaded and current project state:

```
═══════════════════════════════════════════════════════════════
 CONTEXT LOADED
═══════════════════════════════════════════════════════════════

 ✓ Agent Context:     $AGENT_DOMAIN_CONTEXT
 ✓ Architecture:      $RELEVANT_ARCHITECTURE_PATTERNS
 ✓ Current Branch:    $BRANCH_STATUS
 ✓ Related Shards:    $SHARD_STATUS_SUMMARY
 ✓ Files Status:      $FILE_CHANGES_SUMMARY

 Ready to continue autonomous development within:
 - Current specification: $SPEC_NAME
 - Agent domain: $AGENT_DOMAIN
 - Established patterns: $ARCHITECTURE_PATTERNS

═══════════════════════════════════════════════════════════════
```

Then proceed with the user's chosen direction while following autonomous development rules from CLAUDE.md.
