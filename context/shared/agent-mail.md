# Agent Mail

> **Last updated:** 2026-01-26 (added reply_message `to` requirement)

## Overview

Agent Mail is an MCP (Model Context Protocol) server that enables asynchronous communication between Claude Code instances working on the Penfold project.

**Two Claude instances collaborate:**

| Role | Location | Purpose |
|------|----------|---------|
| **Dev Claude** | dev01 (Mac Mini) | Development, code changes, bug fixes |
| **Client Claude** | User's laptop | User assistance, bug reports, feature requests |

## MCP Configuration

Agent Mail runs on dev02 and is accessed via MCP tools prefixed with `mcp__agent-mail__`.

```json
{
  "mcpServers": {
    "agent-mail": {
      "command": "npx",
      "args": ["mcp-agentmail", "--host", "dev02.brown.chat", "--port", "8765"]
    }
  }
}
```

## Agent Identity

### Session Registration

Always start a session to register your agent identity:

```
mcp__agent-mail__macro_start_session(
  human_key: "/Users/james/github/otherjamesbrown/penfold",
  program: "claude-code",
  model: "opus-4.5",
  task_description: "Brief description of current work"
)
```

This returns your randomly-generated agent name (e.g., "RusticDesert", "RedWolf").

### Agent Names

- Names are **adjective+noun** combinations (e.g., "BlueLake", "GreenCastle")
- Names are auto-generated - do NOT use descriptive names like "BackendDev"
- Your name persists across sessions if you use the same `human_key`

### Current Known Agents

| Name | Role | Notes |
|------|------|-------|
| RedWolf | Client Claude | User's laptop instance |
| RusticDesert | Dev Claude | Development server instance |

## Project Key

Always use the **canonical project path** for the project key:

```
/Users/james/github/otherjamesbrown/penfold
```

This ensures all agents working on the same codebase share the same project context.

## Message Conventions

### Subject Lines

Use prefixed, searchable subject lines:

| Type | Format | Example |
|------|--------|---------|
| Bug report | `Bug: <brief description>` | `Bug: glossary_terms table missing linked_entity_type column` |
| Feature request | `Feature: <brief description>` | `Feature: Add batch import for glossary terms` |
| Question | `Question: <topic>` | `Question: How does tenant isolation work?` |
| Status update | `Status: <topic>` | `Status: Completed mention resolution refactor` |
| Review request | `Review: <what>` | `Review: PR #123 - Add glossary linking` |

### Thread IDs

Thread IDs group related messages. Use descriptive, prefixed IDs:

| Type | Format | Example |
|------|--------|---------|
| Bug | `bug-<topic>-<seq>` | `bug-schema-001` |
| Feature | `feat-<topic>-<seq>` | `feat-glossary-001` |
| Discussion | `disc-<topic>-<seq>` | `disc-architecture-001` |
| Bead reference | `pe-<id>` | `pe-j73v` |

**Important:** Thread IDs are NOT searchable via `search_messages`. Always include keywords in the subject/body.

### Message Body

Use markdown. Include:

1. **Context** - What were you doing when this came up?
2. **Details** - Specific error messages, file paths, code references
3. **Suggested action** - What do you think should happen?

## Searching Messages

### What Works

`search_messages` does **full-text search on subject and body only**.

```
# Good - searches words in subject/body
search_messages(query: "glossary")
search_messages(query: "missing column")
search_messages(query: "bug AND schema")
```

### What Doesn't Work

```
# Bad - thread_id is NOT indexed
search_messages(query: "bug-schema-001")  # Returns nothing

# Bad - agent names are NOT indexed
search_messages(query: "RedWolf")  # Returns nothing
```

### Finding by Thread ID

Use `summarize_thread` to fetch messages by thread ID:

```
mcp__agent-mail__summarize_thread(
  project_key: "/Users/james/github/otherjamesbrown/penfold",
  thread_id: "bug-schema-001",
  include_examples: true
)
```

## Issue Tracking Protocol

When bugs or feature requests come through Agent Mail, they must be tracked in the bead system.

### Dev: Receiving a Bug/Feature Request

1. **Create bead(s)** immediately upon receiving the report
   - One primary bead for the main issue
   - Sub-beads if the work naturally splits (e.g., fix + migration + tests)

2. **Reply ASAP** with acknowledgment and tracking info:
   ```markdown
   ## Acknowledged

   Created bead(s) to track this:

   | Bead | Description | Status |
   |------|-------------|--------|
   | pe-xxxx | Main issue description | open |
   | pe-yyyy | Sub-task if applicable | open |

   I'll update you when resolved.
   ```

3. **Reply again** when work is complete with resolution details

### Client: After Receiving Bead IDs

Once you have a bead ID, **use it for all future discussion** about that issue:

- Reference the bead in follow-up messages: "Regarding pe-xxxx..."
- Use the bead ID as thread_id for related messages: `thread_id="pe-xxxx"`
- Check bead status directly: `bd show pe-xxxx`

This keeps communication organized and traceable.

### Why This Matters

- **Traceability** - Every issue has a trackable ID
- **Async-friendly** - Either agent can check status without messaging
- **History** - Beads persist; chat context doesn't
- **Handoffs** - Work can transfer between sessions via bead

## Common Workflows

### Client: Report a Bug

```python
# 1. Start session
session = macro_start_session(
  human_key="/Users/james/github/otherjamesbrown/penfold",
  program="claude-code",
  model="opus-4.5",
  task_description="Reporting bug"
)

# 2. Send bug report
send_message(
  project_key="/Users/james/github/otherjamesbrown/penfold",
  sender_name=session.agent.name,  # e.g., "RedWolf"
  to=["RusticDesert"],  # Dev Claude
  subject="Bug: <description>",
  body_md="## Context\n...\n## Error\n...\n## Expected\n...",
  thread_id="bug-<topic>-001",
  importance="high"
)
```

### Dev: Check for Bug Reports

```python
# 1. Start session
session = macro_start_session(...)

# 2. Check inbox
inbox = fetch_inbox(
  project_key="/Users/james/github/otherjamesbrown/penfold",
  agent_name=session.agent.name,
  include_bodies=true
)

# 3. Or search for bugs
bugs = search_messages(
  project_key="/Users/james/github/otherjamesbrown/penfold",
  query="Bug"
)
```

### Dev: Reply to Bug Report

**Important:** Follow the [Issue Tracking Protocol](#issue-tracking-protocol) - create beads first, then reply with bead IDs.

```python
# 1. Create bead(s) first
# bd create --title="Fix: <description>" --type=bug

# 2. Reply with bead ID(s)
# IMPORTANT: Always specify `to` explicitly - see Troubleshooting
reply_message(
  project_key="/Users/james/github/otherjamesbrown/penfold",
  message_id=<original_message_id>,
  sender_name="RusticDesert",
  to=["RedWolf"],  # Always specify recipient explicitly!
  body_md="## Acknowledged\n\nCreated bead **pe-xxxx** to track this.\n\n| Bead | Description |\n|------|-------------|\n| pe-xxxx | Fix description |\n\nWill update when resolved."
)
```

## Message Templates

### Bug Report Template

```markdown
## Context
What I was doing when this occurred.

## Error
Exact error message or unexpected behavior.

## Steps to Reproduce
1. Step one
2. Step two
3. ...

## Expected Behavior
What should have happened.

## Code References
- `path/to/file.go:123`
- Related bead: pe-xxxx

## Suggested Fix
If you have ideas about what might be wrong.
```

### Feature Request Template

```markdown
## Use Case
Why this feature is needed.

## Proposed Behavior
What the feature should do.

## Alternatives Considered
Other approaches and why they don't fit.

## Priority
How urgent is this? (nice-to-have / important / blocking)
```

### Status Update Template

```markdown
## Completed
- Item 1
- Item 2

## In Progress
- Item 3 (blocked by X)

## Next Steps
- Item 4

## Beads Updated
- pe-xxxx: closed
- pe-yyyy: in_progress
```

## Best Practices

### Do

- **Always register** with `macro_start_session` at session start
- **Create beads immediately** for bugs/features - reply with bead IDs ASAP
- **Use bead IDs** in follow-up discussions (e.g., "Regarding pe-xxxx...")
- **Use searchable subjects** - include keywords that describe the issue
- **Include bead IDs** in messages when referencing tracked work
- **Use thread IDs** for related conversations (use bead ID as thread_id after acknowledgment)
- **Mark messages read** with `mark_message_read` after processing
- **Acknowledge** messages with `ack_required=true` using `acknowledge_message`

### Don't

- Don't leave bug/feature requests untracked - always create a bead
- Don't reply without bead IDs - client needs them for follow-up
- Don't omit `to` in `reply_message` - always specify recipient explicitly (see Troubleshooting)
- Don't search for thread IDs (they're not indexed)
- Don't use descriptive agent names (use auto-generated ones)
- Don't assume your inbox has all messages (you only see messages TO you)
- Don't forget to check inbox at session start

## Troubleshooting

### "Inbox is empty but I know there are messages"

Messages only appear in your inbox if you're a recipient. Use `search_messages` with keywords to find messages not addressed to you.

### "Reply went to wrong recipient" / "Reply went to myself"

**Known issue:** `reply_message` does not reliably default `to` to the original sender. The message may go to yourself instead of the intended recipient.

**Always specify `to` explicitly:**

```python
reply_message(
  project_key="...",
  message_id=123,
  sender_name="RusticDesert",
  to=["RedWolf"],  # REQUIRED - don't rely on default!
  body_md="..."
)
```

If you forget `to`, your reply may end up in your own inbox instead of the recipient's.

### "search_messages returns nothing for thread ID"

Thread IDs are not searchable. Use `summarize_thread(thread_id="...")` instead.

### "Can't find agent by name"

Use `whois(agent_name="...")` to look up agent details, or check the agents list for the project.

## See Also

- [infrastructure.md](../infrastructure.md) - Agent Mail server setup on dev02
- [root-agent.md](../root-agent.md) - How dev agents are organized
- [client/assistant-rules.md](../client/assistant-rules.md) - Client Claude's Agent Mail usage
