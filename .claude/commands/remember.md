# Remember

Store something to remember across sessions, optionally with a trigger condition.

## Arguments: $ARGUMENTS

Required: The thing to remember. Optionally include "when <condition>" to set a trigger.

## Instructions

### Step 1: Parse Arguments

Determine if there's a trigger condition:

- Simple memory: "Delete test data after content delete ships"
- Triggered memory: "Clean up temp files when release v0.4.0 done"

Look for patterns like:
- "when ..."
- "after ..."
- "once ..."
- "if ..."

### Step 2: Create Memory

**Without trigger:**
```bash
penf memory add "$ARGUMENTS"
```

**With trigger (extract the condition):**
```bash
penf memory add "<memory content>" --trigger "<condition>"
```

### Step 3: Confirm

Output:

```
Remembered: <memory content>
[Trigger: <condition>]  # if trigger provided
ID: <memory-id>

To see all memories:
  penf memory list

To see triggered memories:
  penf memory list --triggered

To resolve when done:
  penf memory resolve <id>
```

## Examples

**Simple:**
```
/remember Check if TLS cert expires soon
```

**With trigger:**
```
/remember Delete test fixtures when content delete implemented
```

**Deferred:**
After creating, can defer:
```bash
penf memory defer <id> --until 2026-02-15
```

## Notes

- Memories persist across sessions in Context-Palace
- Triggered memories appear when you run `penf memory list --triggered`
- Use `penf memory resolve <id>` to mark as done
- Use `penf memory defer <id> --until <date>` to snooze
