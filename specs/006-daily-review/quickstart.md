# Quickstart: Daily Review Workflow

**Feature**: 006-daily-review
**Date**: 2026-01-15

## Prerequisites

- PostgreSQL 16+ with existing Penfold schema
- Python 3.12+ with penf_lib installed
- AI processing results from 004-gmail-integration or 005-meeting-pipeline
- Active tenant configured: `penf tenant switch work`

## Installation

The daily review module is included in penf_lib. No additional installation required.

```bash
# Verify installation
penf --version

# Check review commands available
penf review --help
```

## Quick Start (5 minutes)

### 1. Start a Review Session

```bash
# Start standard review (recommended for first use)
penf review

# Or with specific options
penf review --mode=quick --priority=confidence --limit=50
```

### 2. Review Items

The review interface displays:
- Item summary (type, subject, confidence)
- Content preview
- AI suggestion (category, participants, tags)
- Available actions

**Keyboard shortcuts**:
| Key | Action |
|-----|--------|
| `a` | Accept suggestion |
| `r` | Reject suggestion |
| `m` | Modify suggestion |
| `s` | Skip item |
| `d` | Show full details |
| `u` | Undo last decision |
| `b` | Batch operations |
| `q` | Pause and exit |
| `?` | Show help |

### 3. Complete Session

```bash
# When finished reviewing
# Press 'q' to pause, or review until queue is empty

# View session summary
penf review status

# Complete and generate learning suggestions
penf review complete
```

## Review Modes

### Quick Mode
- Single-key decisions only
- No modification option
- Fastest throughput
- Best for high-confidence items

```bash
penf review --mode=quick
```

### Standard Mode (Default)
- All decision types
- Inline modification
- Balanced speed and control
- Best for daily use

```bash
penf review --mode=standard
```

### Detailed Mode
- Full content view
- Detailed modification options
- Note-taking enabled
- Best for complex items

```bash
penf review --mode=detailed
```

## Priority Modes

### Confidence Priority
- Low confidence items first
- Maximizes learning signal
- May be slower

```bash
penf review --priority=confidence
```

### Importance Priority
- High business value first
- Based on sender/content type
- Critical items addressed first

```bash
penf review --priority=importance
```

### Recency Priority
- Most recent items first
- Time-sensitive content prioritized

```bash
penf review --priority=recency
```

### Mixed Priority (Default)
- Warm-up with quick wins (5 items)
- Then optimized mix
- Best overall experience

```bash
penf review --priority=mixed
```

## Batch Operations

### Review by Thread
```bash
# During review, press 'b' then 't'
# Or use CLI:
penf review batch --group=thread --action=accept
```

### Review by Sender
```bash
penf review batch --group=sender --action=accept
```

### Auto-Accept High Confidence
```bash
# Accept all items with confidence > 90%
penf review batch --filter="confidence>0.9" --action=accept
```

## Session Management

### Check Status
```bash
penf review status
```

### Resume Previous Session
```bash
# Automatic resume (default)
penf review

# Force new session
penf review --new
```

### View Queue
```bash
penf review queue
penf review queue --all --limit=50
```

## Learning Rules

### View Suggested Rules
```bash
penf review rules --pending
```

### Accept Rule Suggestion
```bash
penf review rules accept RULE_ID
```

### List Active Rules
```bash
penf review rules list
```

### Disable Rule
```bash
penf review rules disable RULE_ID
```

## Common Workflows

### Daily Morning Review
```bash
# Quick review of overnight processing
penf review --mode=standard --priority=mixed

# Accept/reject/modify as needed
# Complete when done
penf review complete
```

### Bulk Email Processing
```bash
# Focus on email with batch operations
penf review --filter="type=email" --priority=confidence

# Use batch operations for threads
# Press 'b' -> 't' for thread batching
```

### Training the System
```bash
# Detailed mode for training
penf review --mode=detailed --priority=confidence

# Take time with low-confidence items
# Add notes explaining corrections
# System learns from detailed feedback
```

## Troubleshooting

### No Items in Queue
```bash
# Check processing status
penf coordination status

# Verify content ingestion
penf gmail status
penf meetings status
```

### Session Expired
```bash
# Sessions expire after 24 hours
# Start new session
penf review --new
```

### Undo Not Available
```bash
# Undo only available within 5 minutes
# Check undo eligibility
penf review status

# Recent decisions shown with undo status
```

## Performance Tips

1. **Use keyboard shortcuts** - faster than CLI commands
2. **Enable batch mode** for similar items - save 40% time
3. **Start with mixed priority** - builds momentum
4. **Review daily** - smaller queues = faster reviews
5. **Accept learning rules** - reduces future review load

## Next Steps

- Review analytics: `penf review analytics --period=week`
- Customize rules: `penf review rules list`
- Check AI accuracy: `penf coordination performance`
