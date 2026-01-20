# Workflow: [NAME]

## Purpose
[What this workflow accomplishes and why it exists]

## When to Use
[Conditions that trigger this workflow]

## Batch Data Command
```bash
penf process [workflow] context
```

## Available Actions

| Action | Command | Effect |
|--------|---------|--------|
| [action] | `penf [command]` | [what happens] |

## Decision Guidelines

### Auto-resolve (Claude can handle)
- [criteria for automatic resolution]

### Needs Human Input
- [criteria requiring user decision]

### Common Patterns
- [pattern]: [how to handle]

## Data Structures

### Input Format (JSON)
```json
{
  "items": [...],
  "related_data": {...}
}
```

### Batch Action Format
```bash
penf process [workflow] batch-[action] '[json]'
```

## Examples

### Simple Case
```bash
# Get context
penf process acronyms context

# Resolve single item
penf review questions resolve 123 "Expansion Here"
```

### Batch Processing
```bash
# Resolve multiple at once
penf process acronyms batch-resolve '{
  "123": "Expansion One",
  "456": "Expansion Two"
}'
```
