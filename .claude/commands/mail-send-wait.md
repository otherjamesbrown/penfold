# Mail Send Wait

Send a synchronous message and wait for response using Context-Palace.

## Arguments

- `$ARGUMENTS` - Format: `<recipient> <subject>` (e.g., `agent-penf "Bug: Search timeout"`)

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
POLL_INTERVAL: 5  # seconds
MAX_DURATION: 3600  # 60 minutes
WARN_DURATION: 1800  # 30 minutes
```

## Instructions

### Step 1: Parse Arguments

Extract from `$ARGUMENTS`:
- `RECIPIENT`: First word (agent name)
- `SUBJECT`: Remaining text (quoted string)

If arguments missing, use AskUserQuestion to get them.

### Step 2: Generate Session ID

```bash
SESSION_ID=$(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8)
```

This creates a unique conversation identifier like `sync:session-a1b2c3d4`.

### Step 3: Compose Message

Build your message with JSON frontmatter:

```markdown
{
  "poll_hint": "continue",
  "type": "<bug|question|request|feature>",
  "session": "<SESSION_ID>"
}

<Your message content in markdown>

-- <AGENT_NAME>
```

### Step 4: Send Message

```sql
SELECT send_message(
  'penfold',
  '<AGENT_NAME>',
  ARRAY['<RECIPIENT>'],
  '<SUBJECT>',
  $body$
{
  "poll_hint": "continue",
  "type": "<type>",
  "session": "<SESSION_ID>"
}

<message content>

-- <AGENT_NAME>
$body$,
  NULL,
  '<type>',
  NULL
);
```

Add session label:
```sql
SELECT add_labels('<NEW_ID>', ARRAY['sync:true', 'sync:session-<SESSION_ID>']);
```

### Step 5: Poll for Response

```bash
START_TIME=$(date +%s)

while true; do
  # Check for new messages
  RESULT=$(psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -t -c \
    "SELECT id FROM unread_for('penfold', '<AGENT_NAME>')
     WHERE id IN (SELECT shard_id FROM labels WHERE label = 'sync:session-<SESSION_ID>');")

  if [ -n "$(echo "$RESULT" | tr -d '[:space:]')" ]; then
    echo "Response received: $RESULT"
    break
  fi

  # Check duration
  ELAPSED=$(($(date +%s) - START_TIME))
  if [ $ELAPSED -gt <MAX_DURATION> ]; then
    echo "TIMEOUT: Max duration reached"
    # Send timeout message
    break
  fi
  if [ $ELAPSED -gt <WARN_DURATION> ]; then
    echo "WARNING: Conversation running long ($ELAPSED seconds)"
  fi

  sleep <POLL_INTERVAL>
done
```

### Step 6: Process Response

When response arrives:

1. **Read the message**:
```sql
SELECT id, title, content, creator FROM shards WHERE id = '<RESPONSE_ID>';
SELECT mark_read(ARRAY['<RESPONSE_ID>'], '<AGENT_NAME>');
```

2. **Parse poll_hint** from JSON frontmatter:
   - Extract the JSON block at the start of content
   - Get `poll_hint` value

3. **Handle based on poll_hint**:

| poll_hint | Action |
|-----------|--------|
| `continue` | Process message, optionally reply, continue polling |
| `done` | Process message, **EXIT command entirely** |
| `pause` | Sleep for `resume_in` seconds, then resume polling |
| `typing` | Reset timeout, continue polling |

**IMPORTANT:** When you receive `poll_hint: done`, the conversation is over. Mark messages as read and EXIT the command immediately. Do NOT continue polling.

### Step 7: Reply (if needed)

If you need to respond:

```sql
SELECT send_message(
  'penfold',
  '<AGENT_NAME>',
  ARRAY['<RECIPIENT>'],
  'Re: <SUBJECT>',
  $body$
{
  "poll_hint": "<continue|done>",
  "type": "response",
  "session": "<SESSION_ID>"
}

<your reply>

-- <AGENT_NAME>
$body$,
  NULL,
  'response',
  '<ORIGINAL_MESSAGE_ID>'
);

SELECT add_labels('<NEW_ID>', ARRAY['sync:true', 'sync:session-<SESSION_ID>']);
```

### Step 8: End Conversation

To end the conversation, send a final message with `poll_hint: done`:

```sql
SELECT send_message(
  'penfold',
  '<AGENT_NAME>',
  ARRAY['<RECIPIENT>'],
  'Resolved: <SUBJECT>',
  $body$
{
  "poll_hint": "done",
  "type": "resolution",
  "session": "<SESSION_ID>"
}

## Summary

<what was resolved>

-- <AGENT_NAME>
$body$,
  NULL,
  'resolution',
  '<LAST_MESSAGE_ID>'
);
```

### Step 9: Final Summary

```
═══════════════════════════════════════════════════════════════
 CONVERSATION COMPLETE
═══════════════════════════════════════════════════════════════

 Session: sync:session-<SESSION_ID>
 With: <RECIPIENT>
 Subject: <SUBJECT>
 Duration: <elapsed time>
 Messages exchanged: <count>

 Outcome: <resolved|timeout|error>

═══════════════════════════════════════════════════════════════
```

---

## Timeout Handling

| Elapsed | Action |
|---------|--------|
| 5 min no response | Send reminder: `{poll_hint: "continue", type: "ping"}` |
| 15 min no response | Ask "Are you still there?" |
| 30 min | Warning to user |
| 60 min | Send `{poll_hint: "done", type: "timeout"}`, end conversation |

---

## Example Usage

User runs: `/mail-send-wait agent-penf "Bug: Search command timing out"`

1. You compose the bug report with JSON frontmatter
2. Send via `send_message()`
3. Poll every 5 seconds
4. agent-penf responds with `poll_hint: continue` asking for more info
5. You reply with details, `poll_hint: continue`
6. agent-penf responds with `poll_hint: done` saying it's fixed
7. Conversation ends, summary displayed

---

## Notes

- Always include `sync:session-<ID>` label on all messages in conversation
- Parse JSON frontmatter to get poll_hint (first `{...}` block in content)
- Use `<>` instead of `!=` in SQL to avoid bash escaping issues
- Prefer dollar-quoting (`$body$...$body$`) for message content
