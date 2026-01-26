# Agent Mail Integration Plan

**Goal:** Enable two-way communication between Client Claude (laptop) and Dev Claude (server) for feedback, bug reports, and feature discussions.

**Status:** Planning

---

## Overview

Replace the current `penf feedback` GitHub Issues approach with a beads + MCP Agent Mail architecture that enables proper conversations between client and development agents.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        AGENT COMMUNICATION FLOW                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   CLIENT (dev01/laptop)              SERVER (dev02)                     │
│   ─────────────────────              ─────────────                      │
│                                                                         │
│   penf feedback bug "..."            bd list --label=client-feedback    │
│     │                                  │                                │
│     ├─→ bd create pe-fb-xxx            └─→ sees new bead               │
│     │     --label=client-feedback                                       │
│     │                                  am fetch-inbox                   │
│     └─→ am send                          │                              │
│           --thread=pe-fb-xxx             └─→ sees message               │
│           "Bug: Search broken"                                          │
│                                        am reply --thread=pe-fb-xxx      │
│   [later]                                "What query caused it?"        │
│   penf feedback check                                                   │
│     │                                  [investigates, fixes]            │
│     └─→ am fetch-inbox                                                  │
│           → "What query caused it?"    bd close pe-fb-xxx               │
│                                        am send --thread=pe-fb-xxx       │
│   penf feedback reply pe-fb-xxx          "Fixed in commit abc123"       │
│     "Query was 'TER meetings'"                                          │
│                                                                         │
│           ↑                                      ↑                      │
│           └──────── MCP Agent Mail Server ───────┘                      │
│                     (HTTP on dev02:8765)                                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Infrastructure Setup

### 1.1 Install MCP Agent Mail on dev02

```bash
# On dev02
curl -fsSL "https://raw.githubusercontent.com/Dicklesworthstone/mcp_agent_mail/main/scripts/install.sh" | bash -s -- --yes
```

This installs:
- MCP Agent Mail server (port 8765)
- Beads Rust (`br` command)
- Beads Viewer (`bv` for analytics)

### 1.2 Configure Agent Mail Server

```bash
# Set port (if needed)
mcp-agent-mail cli config set-port 8765

# Start server
am  # or: mcp-agent-mail serve
```

### 1.3 Configure Network Access

Agent Mail server needs to be accessible from dev01 (where client runs):

```bash
# On dev02 - ensure port 8765 is accessible
# May need firewall rule if not on same network
```

### 1.4 Verify Beads Worktree Setup ✅ DONE

```bash
bd config set sync.branch beads-sync
bd sync
```

---

## Phase 2: Agent Registration

### 2.1 Register Dev Agents

Each development agent needs an identity:

```bash
# On dev02 (or wherever dev Claude runs)
am register-agent --project=/path/to/penfold --program="Claude Code" --model="claude-sonnet"
# Returns: Agent name like "BlueCastle"
```

### 2.2 Register Client Agent

```bash
# On dev01 (or laptop)
am register-agent --project=/path/to/penfold --program="Penfold Client" --model="claude-sonnet"
# Returns: Agent name like "GreenMountain"
```

### 2.3 Store Agent Identities

Add to `~/.penf/config.yaml`:

```yaml
agent_mail:
  server: "http://dev02.brown.chat:8765"
  client_agent: "GreenMountain"  # Client's agent name
```

---

## Phase 3: Update penf feedback Commands

### 3.1 Replace GitHub Issues with Beads + Agent Mail

**Current:** `penf feedback bug "desc"` → Creates GitHub issue

**New:** `penf feedback bug "desc"` → Creates bead + sends Agent Mail message

```go
// cmd/penf/cmd/feedback.go changes

func runFeedback(feedbackType, description string) error {
    // 1. Create bead
    beadID := createBead(feedbackType, description)

    // 2. Send Agent Mail message with bead as thread_id
    sendAgentMail(AgentMailMessage{
        ThreadID: beadID,
        Subject:  fmt.Sprintf("[%s] %s: %s", beadID, feedbackType, truncate(description, 50)),
        Body:     buildFeedbackBody(feedbackType, description),
    })

    return nil
}
```

### 3.2 Add New Commands

| Command | Action |
|---------|--------|
| `penf feedback bug "desc"` | Create bug bead + send message |
| `penf feedback feature "desc"` | Create feature bead + send message |
| `penf feedback check` | Check inbox for responses |
| `penf feedback reply <bead-id> "msg"` | Reply to a thread |
| `penf feedback list` | List all my feedback beads with status |
| `penf feedback show <bead-id>` | Show bead + full conversation thread |

### 3.3 Integrate with Agent Mail CLI

Option A: Shell out to `am` commands
```go
exec.Command("am", "send", "--thread", beadID, "--subject", subject, body)
```

Option B: Use Agent Mail HTTP API directly
```go
resp, err := http.Post("http://dev02:8765/tools/send_message", "application/json", payload)
```

---

## Phase 4: Client Startup Behavior

### 4.1 Update assistant-rules.md

Add to Memory System section:

```markdown
### Feedback & Communication

At session start, also check for responses to your feedback:
- Run `penf feedback check` to see if dev has questions
- Respond to any pending questions before starting new work

When you notice issues:
- Use `penf feedback bug "description"` instead of just noting it
- Dev Claude will see it and can ask clarifying questions
- Check back later with `penf feedback check`
```

### 4.2 Session Startup Checklist

```markdown
## Session Startup

1. Read memory files (memory/YYYY-MM-DD.md)
2. Read preferences.md
3. Check feedback inbox: `penf feedback check`
4. Respond to any pending questions
5. Continue with user's request
```

---

## Phase 5: Dev-Side Integration

### 5.1 Monitor Client Feedback

```bash
# List all client feedback beads
bd list --label=client-feedback --status=open

# Check Agent Mail for details
am fetch-inbox --project=/path/to/penfold
```

### 5.2 Respond to Feedback

```bash
# Read the full thread
am thread pe-fb-001

# Ask a clarifying question
am reply --thread=pe-fb-001 "What search query triggered this? Can you paste the error?"

# When fixed
bd close pe-fb-001 --reason="Fixed in commit abc123"
am send --thread=pe-fb-001 "Fixed in commit abc123. Update with 'penf update' to get the fix."
```

---

## Phase 6: Sync Architecture

### 6.1 Git-Based Sync

Both beads and Agent Mail use Git for persistence:

```
.beads/
├── beads.db          # SQLite (local, gitignored)
├── issues.jsonl      # Git-tracked, synced via beads-sync branch
├── config.yaml       # Git-tracked configuration
└── bd.sock           # Daemon socket (local)

.agent-mail/          # If using Git-backed Agent Mail
├── projects/
│   └── penfold/
│       ├── messages/
│       └── reservations/
└── agents.db
```

### 6.2 Sync Triggers

| Event | Action |
|-------|--------|
| `penf feedback *` | Auto-syncs bead + sends message |
| `penf update` | Pulls latest beads + could check inbox |
| `bd sync` | Syncs beads to beads-sync branch |
| Dev pushes to main | Client can pull to get fixes |

---

## Implementation Order

### Milestone 1: Basic Integration
- [ ] Install Agent Mail on dev02
- [ ] Configure network access
- [ ] Register agents (client + dev)
- [ ] Test basic messaging: `am send` / `am fetch-inbox`

### Milestone 2: CLI Integration
- [ ] Update `penf feedback bug` to create beads
- [ ] Update `penf feedback bug` to send Agent Mail
- [ ] Add `penf feedback check` command
- [ ] Add `penf feedback reply` command
- [ ] Add `penf feedback list` command

### Milestone 3: Documentation & Workflow
- [ ] Update assistant-rules.md with feedback workflow
- [ ] Update CLAUDE.md with agent mail instructions
- [ ] Add feedback check to session startup
- [ ] Document dev-side workflow

### Milestone 4: Polish
- [ ] Add `penf feedback show` for full thread view
- [ ] Add notification hints (e.g., "You have 2 pending responses")
- [ ] Consider file reservations for coordinated edits
- [ ] Add feedback metrics/stats

---

## Configuration Reference

### Client Config (~/.penf/config.yaml)

```yaml
server_address: "dev02.brown.chat:50051"
agent_mail:
  server: "http://dev02.brown.chat:8765"
  agent_name: "PenfoldClient"  # Set after registration
  project: "/Users/james/github/otherjamesbrown/penfold"
```

### Dev Config (beads)

```yaml
# .beads/config.yaml
issue-prefix: "pe"
sync-branch: "beads-sync"
```

---

## Open Questions

1. **Where to run Agent Mail server?**
   - Option A: dev02 (centralized, always on)
   - Option B: Shared Git repo only (no server, just Git sync)

2. **Authentication?**
   - Agent Mail supports JWT - do we need it for local network?

3. **File reservations?**
   - Do we want client to reserve files when reporting bugs about specific areas?

4. **Integration with existing beads workflow?**
   - Should `bd ready` show client-feedback items?
   - Should client-feedback have its own priority?

---

## Related Documents

- [assistant-rules.md](../client/assistant-rules.md) - Client behavior rules
- [infrastructure.md](../infrastructure.md) - Server topology
- [CLAUDE.md](../../CLAUDE.md) - Dev-side agent instructions
