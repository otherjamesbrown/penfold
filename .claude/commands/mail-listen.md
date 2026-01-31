# Mail Listen

Listen for incoming synchronous messages and respond using Context-Palace.

## Configuration

```yaml
AGENT_NAME: agent-penfdev
PROJECT: penfold
POLL_INTERVAL: 5  # seconds
MAX_DURATION: 3600  # 60 minutes
WARN_DURATION: 1800  # 30 minutes
```

## Instructions

### Step 1: Start Listening

Announce that you're listening:

```
═══════════════════════════════════════════════════════════════
 LISTENING FOR MESSAGES - <AGENT_NAME>
═══════════════════════════════════════════════════════════════

 Project: <PROJECT>
 Poll interval: <POLL_INTERVAL>s
 Waiting for incoming sync messages...

═══════════════════════════════════════════════════════════════
```

### Step 2: Poll for Messages

```bash
START_TIME=$(date +%s)

while true; do
  # Check for new sync messages
  RESULT=$(psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -t -c \
    "SELECT s.id FROM unread_for('penfold', '<AGENT_NAME>') s
     JOIN labels l ON l.shard_id = s.id
     WHERE l.label = 'sync:true';")

  if [ -n "$(echo "$RESULT" | tr -d '[:space:]')" ]; then
    echo "Message received: $RESULT"
    break
  fi

  # Check duration
  ELAPSED=$(($(date +%s) - START_TIME))
  if [ $ELAPSED -gt <MAX_DURATION> ]; then
    echo "TIMEOUT: Max listen duration reached"
    break
  fi

  echo "Polling... ($ELAPSED seconds elapsed)"
  sleep <POLL_INTERVAL>
done
```

### Step 3: Read Incoming Message

```sql
SELECT id, title, content, creator FROM shards WHERE id = '<MESSAGE_ID>';
SELECT mark_read(ARRAY['<MESSAGE_ID>'], '<AGENT_NAME>');
```

### Step 4: Parse Message

Extract from the message:

1. **JSON frontmatter** (first `{...}` block):
   - `poll_hint`: continue|done|pause|typing
   - `type`: bug|question|request|feature|response
   - `session`: Session ID for this conversation

2. **Content**: Everything after the JSON block

3. **Metadata**:
   - `sender`: creator field
   - `subject`: title field

### Step 5: Handle Based on poll_hint

| poll_hint | Action |
|-----------|--------|
| `continue` | Sender is waiting - process and respond |
| `done` | Conversation ended by sender - acknowledge and stop |
| `pause` | Sender needs time - sleep `resume_in` seconds, continue polling |
| `typing` | Sender is composing - reset timeout, continue polling |

### Step 6: Process the Request

Based on message `type`:

#### Bug Report
1. Investigate the issue
2. Create task if needed:
```sql
SELECT create_task_from(
  'penfold',
  '<AGENT_NAME>',
  '<MESSAGE_ID>',
  'fix: <bug title>',
  '<investigation notes>',
  1,  -- priority from severity
  '<AGENT_NAME>'
);
```
3. Reply with findings

#### Question
1. Research the answer
2. Reply with information

#### Request
1. Fulfill the request
2. Reply with results

#### Feature Request
1. Assess feasibility
2. Create task if approved
3. Reply with plan

### Step 7: Compose Response

Build response with JSON frontmatter:

```markdown
{
  "poll_hint": "<continue|done>",
  "type": "<ack|response|resolution|question>",
  "session": "<SESSION_ID from incoming message>"
}

<your response content>

-- <AGENT_NAME>
```

**Choose poll_hint:**
- `continue` - You need more info or expect follow-up
- `done` - You've fully resolved the request

### Step 8: Send Response

```sql
SELECT send_message(
  'penfold',
  '<AGENT_NAME>',
  ARRAY['<SENDER>'],
  'Re: <SUBJECT>',
  $body$
{
  "poll_hint": "<continue|done>",
  "type": "<type>",
  "session": "<SESSION_ID>"
}

<response content>

-- <AGENT_NAME>
$body$,
  NULL,
  '<type>',
  '<MESSAGE_ID>'
);

SELECT add_labels('<NEW_ID>', ARRAY['sync:true', 'sync:session-<SESSION_ID>']);
```

### Step 9: Continue or End

**If you sent `poll_hint: continue`:**
- Return to Step 2, continue polling
- Filter by same session: `sync:session-<SESSION_ID>`

**If you sent `poll_hint: done`:**
- Conversation complete
- Mark all messages as read
- **EXIT the command entirely** (return to user)

**If sender sent `poll_hint: done`:**
- Mark all messages as read
- Conversation complete
- **EXIT the command entirely** (return to user)

**IMPORTANT:** When ANY party sends `poll_hint: done`, the conversation is over. Do NOT continue polling for new conversations. This command handles ONE synchronous conversation, then exits.

### Step 10: Summary on Exit

When stopping (timeout or user interrupt):

```
═══════════════════════════════════════════════════════════════
 LISTEN SESSION COMPLETE
═══════════════════════════════════════════════════════════════

 Duration: <elapsed time>
 Conversations handled: <count>

 Sessions:
   sync:session-abc123 with agent-penf - Resolved (3 messages)
   sync:session-def456 with agent-test - Timeout (1 message)

═══════════════════════════════════════════════════════════════
```

---

## Response Templates

### Acknowledgment (need more info)
```json
{
  "poll_hint": "continue",
  "type": "ack",
  "session": "<SESSION_ID>"
}
```
```markdown
## Acknowledged

Received your bug report. Investigating.

**Questions:**
1. What version are you running?
2. Can you provide the full error message?

-- <AGENT_NAME>
```

### Resolution (complete)
```json
{
  "poll_hint": "done",
  "type": "resolution",
  "session": "<SESSION_ID>"
}
```
```markdown
## Resolved

### What Was Done
<description of fix>

### Task Created
| Task | Title | Status |
|------|-------|--------|
| pf-xxx | fix: <title> | Closed |

### Verification
Run: `<command to verify>`

-- <AGENT_NAME>
```

### Pause (need time)
```json
{
  "poll_hint": "pause",
  "resume_in": 300,
  "type": "status",
  "session": "<SESSION_ID>"
}
```
```markdown
## Working on it

Deploying fix now. Will take about 5 minutes.

Please wait, I'll message when ready.

-- <AGENT_NAME>
```

---

## Handling Multiple Sessions

If multiple sync messages arrive:

1. Process them in order (oldest first)
2. Track each session separately
3. Use session labels to filter responses
4. Don't mix conversations

```sql
-- Filter by specific session
SELECT s.* FROM unread_for('penfold', '<AGENT_NAME>') s
JOIN labels l ON l.shard_id = s.id
WHERE l.label = 'sync:session-<SESSION_ID>';
```

---

## Notes

- Always preserve the `session` ID from incoming messages in your responses
- Add `sync:true` and `sync:session-<ID>` labels to all responses
- Use `send_message()` helper for atomic message creation
- Parse JSON frontmatter carefully - it's at the start of content
- If unsure whether to continue or end, use `continue` and let sender decide
- Prefer dollar-quoting (`$body$...$body$`) for message content
