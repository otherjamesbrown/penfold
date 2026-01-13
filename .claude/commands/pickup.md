# Session Pickup

Resume work from a previous session using a handoff bead.

## Instructions

### Step 1: Find Handoff Beads

Look for open handoff beads on the current branch:

```bash
BRANCH=$(git rev-parse --abbrev-ref HEAD)
bd list --label handoff --label $BRANCH --status open
bd list --label handoff --label $BRANCH --status in_progress
```

If no handoffs found for current branch, check all open handoffs:

```bash
bd list --label handoff --status open
bd list --label handoff --status in_progress
```

### Step 2: Display Handoff Context

For each handoff found, show the details:

```bash
bd show $HANDOFF_BEAD_ID
```

Present a summary:

```
═══════════════════════════════════════════════════════════════
 PENFOLD HANDOFF FOUND
═══════════════════════════════════════════════════════════════

 Handoff Bead:    $BEAD_ID
 Title:           $TITLE
 Branch:          $BRANCH_NAME
 Agent Domain:    $AGENT_DOMAIN
 Specification:   $CURRENT_SPEC
 Created:         $CREATED_DATE

 ## Session Goal
 $GOAL_FROM_DESCRIPTION

 ## Progress Made
 $COMPLETED_FROM_DESCRIPTION

 ## Remaining Work
 $REMAINING_FROM_DESCRIPTION

 ## Key Findings
 $FINDINGS_FROM_DESCRIPTION

 ## Related Beads
 $RELATED_BEADS

 ## Architecture Notes
 $ARCHITECTURE_IMPLICATIONS

═══════════════════════════════════════════════════════════════
```

### Step 3: Mark as In Progress

Update the handoff bead to show work has resumed:

```bash
bd update $HANDOFF_BEAD_ID --status in_progress
```

### Step 4: Load Penfold Context

Based on the handoff, load relevant Penfold context:

1. **Check current specification status** (if working on specs/001-011)
2. **Review agent domain context** - Load appropriate context/{domain}/agents.md
3. **Check architecture relevance** - Review context/ARCHITECTURE.md for related patterns
4. **Verify autonomous development rules** - Confirm behavior from CLAUDE.md
5. **Review related beads** - Check status of dependent/related work

### Step 5: Load Relevant Files

Load files mentioned in the handoff:
- Read key implementation files
- Check status of related beads: `bd show $RELATED_BEAD_ID`
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
 ✓ Related Beads:     $BEAD_STATUS_SUMMARY
 ✓ Files Status:      $FILE_CHANGES_SUMMARY

 Ready to continue autonomous development within:
 - Current specification: $SPEC_NAME
 - Agent domain: $AGENT_DOMAIN
 - Established patterns: $ARCHITECTURE_PATTERNS

═══════════════════════════════════════════════════════════════
```

Then proceed with the user's chosen direction while following autonomous development rules from CLAUDE.md.