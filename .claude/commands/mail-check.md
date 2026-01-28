# Mail Check

Check inbox for new messages and propose actions.

## Instructions

### Step 1: Register Identity

Register with agent-mail as RusticDesert:

```
mcp__agent-mail__register_agent(
  project_key: "/Users/james/github/otherjamesbrown/penfold",
  name: "RusticDesert",
  program: "claude-code",
  model: <your model>,
  task_description: "Checking mail"
)
```

### Step 2: Fetch Inbox

Fetch recent messages:

```
mcp__agent-mail__fetch_inbox(
  project_key: "/Users/james/github/otherjamesbrown/penfold",
  agent_name: "RusticDesert",
  limit: 20,
  include_bodies: true
)
```

### Step 3: Categorize Messages

For each message, determine:

| Category | Description | Typical Action |
|----------|-------------|----------------|
| **Bug Report** | Something is broken | Create P1/P2 bead |
| **Feature Request** | New functionality needed | Create P2/P3 bead |
| **Question** | Needs information/clarification | Reply with answer |
| **Status Update** | FYI, completion notice | Acknowledge only |
| **Blocker** | Urgent, blocking work | Create P0 bead |

### Step 4: Build Summary

Present a summary table:

```
═══════════════════════════════════════════════════════════════
 INBOX SUMMARY - RusticDesert
═══════════════════════════════════════════════════════════════

 Messages: X new, Y total

 # | From      | Subject                    | Category   | Proposed Action
───┼───────────┼────────────────────────────┼────────────┼─────────────────
 1 | RedWolf   | Bug: login fails           | Bug        | Create bead P1
 2 | RedWolf   | Add dark mode              | Feature    | Create bead P3
 3 | JadeMeadow| Refactor complete          | Status     | Acknowledge
 4 | RedWolf   | How does X work?           | Question   | Reply with answer

═══════════════════════════════════════════════════════════════
```

### Step 5: Detail Proposed Actions

For each actionable message, provide details:

```
───────────────────────────────────────────────────────────────
MESSAGE #1: Bug: login fails
───────────────────────────────────────────────────────────────
From: RedWolf
Category: Bug Report
Priority: P1 (blocks user workflow)
Agent: cli-dev (work in cmd/penf/)

Summary: [1-2 sentence summary of the issue]

Proposed Action:
  → Create bead: "fix: login authentication failure"
  → Labels: bug, auth, cli
  → Assign to: cli-dev agent
  → Reply: "Created bead pe-XXXX, assigning to cli-dev agent."

───────────────────────────────────────────────────────────────
```

### Step 6: Ask for Approval

Use AskUserQuestion to get approval:

**Question:** "How should I proceed with these messages?"

**Options:**
1. **Process all** - Create beads and send replies (no execution)
2. **Process and execute** - Create beads, spawn agents, execute, reply when done
3. **Review one by one** - Confirm each action individually
4. **Skip for now** - Don't take any actions

### Step 7: Execute Based on Choice

---

## Option A: Process All (Create beads, reply, no execution)

For each actionable message:

1. **Create bead** using correct syntax:
   ```bash
   bd create "<type>: <title>" \
     --priority <P0|P1|P2|P3> \
     --label <label1> \
     --label <label2> \
     --description "<context from the message>"
   ```

2. **Reply to sender**:
   ```
   mcp__agent-mail__reply_message(
     project_key: "/Users/james/github/otherjamesbrown/penfold",
     message_id: <id>,
     sender_name: "RusticDesert",
     to: ["<sender_name>"],
     body_md: "## Acknowledged\n\n<summary>\n\n### Bead Created\n| Bead | Title | Priority |\n|------|-------|----------|\n| **pe-XXXX** | <title> | <priority> |\n\nWill update you when resolved."
   )
   ```

3. **Acknowledge message**:
   ```
   mcp__agent-mail__acknowledge_message(...)
   ```

---

## Option B: Process and Execute (Full automation)

For each actionable message:

1. **Create bead** (same as Option A)

2. **Determine agent** based on domain:

   | Domain | Agent | Signs |
   |--------|-------|-------|
   | CLI commands, help text | `cli-dev` | `cmd/penf/`, CLI errors |
   | Database, migrations | `data-dev` | Schema, SQL, `pkg/` repos |
   | Search, embeddings, LLM | `ai-dev` | AI/ML, embeddings |
   | Temporal workflows | `worker-dev` | Background jobs, workflows |
   | Gmail connector, OAuth | `gmail-dev` | Email sync |
   | Gateway services | `gateway-dev` | gRPC, `services/gateway/` |
   | Complex investigation | `debugger` | Unknown cause, >30 min |

3. **Reply with assignment**:
   ```
   mcp__agent-mail__reply_message(
     ...
     body_md: "## In Progress\n\nCreated bead **pe-XXXX** and assigned to **<agent>** agent.\n\nWill update you when complete."
   )
   ```

4. **Spawn agent** using Task tool:
   ```
   Task(
     subagent_type: "general-purpose",
     prompt: "You are the <agent-name> agent. Read context/agents/<agent-name>.md first.

     Work on bead pe-XXXX: <title>

     Context from client (RedWolf):
     <paste message body>

     Requirements:
     1. Claim the bead: bd update pe-XXXX --status=in_progress
     2. Implement the fix/feature
     3. Test your changes
     4. Close the bead: bd close pe-XXXX
     5. Return a summary of what you did and any client actions needed

     If CLI changes: bump version in cmd/penf/VERSION and push to trigger release.",
     description: "<agent>: pe-XXXX"
   )
   ```

5. **After agent completes**, reply to client with results:
   ```
   mcp__agent-mail__reply_message(
     ...
     body_md: "## Resolved: <title>\n\n### What Was Done\n<agent summary>\n\n### Client Action\n<if needed: 'Run `penf update` to get the fix' or 'Just retry the command'>\n\n### Bead\n**pe-XXXX** - Closed"
   )
   ```

---

## Bead Creation Reference

**IMPORTANT: Use `bd create`, NOT `bd add`**

### Syntax
```bash
bd create "<type>: <title>" \
  --priority <priority> \
  --label <label> \
  --description "<description>"
```

### Type Prefixes
| Type | Usage |
|------|-------|
| `fix:` | Bug fixes |
| `feat:` | New features |
| `refactor:` | Code improvements |
| `docs:` | Documentation |
| `chore:` | Maintenance tasks |

### Priority Mapping
| Mail Importance | Bead Priority | Meaning |
|-----------------|---------------|---------|
| urgent | P0 | Drop everything |
| high | P1 | Do today |
| normal | P2 | This week |
| low | P3 | Backlog |

### Example
```bash
bd create "fix: ingest job tenant_id UUID resolution" \
  --priority P1 \
  --label bug \
  --label ingest \
  --label gateway \
  --description "Ingest job creation passes tenant slug to UUID column. Apply same tenant resolution pattern used in ProjectService."
```

---

## Final Summary

After processing, show what was done:

```
═══════════════════════════════════════════════════════════════
 MAIL PROCESSING COMPLETE
═══════════════════════════════════════════════════════════════

 Processed: X messages
 Beads created: Y
 Agents spawned: Z (if execute mode)
 Replies sent: W
 Acknowledged: V

 New Beads:
   pe-XXXX  fix: <title>                              P1  [cli-dev]
   pe-YYYY  feat: <title>                             P2  [data-dev]

 Agent Results: (if execute mode)
   ✓ cli-dev completed pe-XXXX - client action: run `penf update`
   ✓ data-dev completed pe-YYYY - no client action needed

═══════════════════════════════════════════════════════════════
```

---

## Notes

- **Thread awareness**: If multiple messages are in the same thread, summarize the thread context before proposing action
- **Don't duplicate**: Check if a bead already exists for the issue before proposing a new one (`bd search "<keywords>"`)
- **Importance mapping**: Mail importance → bead priority (urgent=P0, high=P1, normal=P2, low=P3)
- **Status updates**: Messages that are just "completed" or "FYI" only need acknowledgment, not beads
- **Always specify `to`**: When using reply_message, always include explicit `to: ["<recipient>"]`
- **Version bumps**: After CLI changes, bump `cmd/penf/VERSION` and push to trigger release
- **Client notifications**: After releases, tell client to run `penf update`
