# Quickstart: Automation Rules Engine

**Feature**: 008-automation-engine
**Date**: 2026-01-15

## Overview

The Automation Rules Engine enables intelligent content processing based on user-defined rules and AI confidence scores. It progressively learns from your feedback to automate repetitive categorization tasks.

## Prerequisites

- Penfold CLI installed (`penf` command available)
- Active tenant context (`penf tenant switch work`)
- Database migrations applied
- AI coordination configured (for confidence scores)

## Quick Setup

### 1. Check Current Settings

```bash
# View your automation settings
penf automation settings show

# Output:
# Aggressiveness: moderate
# Auto Threshold Adjustment: enabled
# Target Automation Rate: 60%
# Accuracy Floor: 95%
```

### 2. Set Confidence Thresholds

```bash
# Set global default (85% recommended)
penf automation thresholds set --content-type "*" --value 0.85

# Set per-content-type thresholds
penf automation thresholds set --content-type email --value 0.80
penf automation thresholds set --content-type meeting --value 0.90

# List all thresholds
penf automation thresholds list
```

### 3. Create Your First Rule

```bash
# Create a rule to categorize emails from a specific sender
penf automation rules create \
  --name "Project Alpha Emails" \
  --conditions '{"operator": "AND", "conditions": [
    {"field": "content_type", "operator": "equals", "value": "email"},
    {"field": "sender", "operator": "contains", "value": "@projectalpha.com"}
  ]}' \
  --actions '{"action_type": "assign_project", "parameters": {"project_id": "uuid-here"}}'

# Or use a conditions file
penf automation rules create \
  --name "Newsletter Archive" \
  --conditions @conditions.json \
  --actions @actions.json
```

### 4. Test Your Rule

```bash
# Test rule against specific content (simulation only)
penf automation rules test RULE_ID --content-id EMAIL_123

# Output:
# Rule: Newsletter Archive
# Would Match: Yes
# Matched Conditions: content_type=email, subject contains "newsletter"
# Would Apply: archive
# Conflicts: None
```

### 5. View Automation Activity

```bash
# See recent automation decisions
penf automation decisions list --since "7 days ago"

# Get automation statistics
penf automation stats --period 30

# Output:
# Period: 30 days
# Total Processed: 1,247
# Auto-Processed: 748 (60%)
# Review Queue: 499 (40%)
# Accuracy Rate: 96.2%
# Active Rules: 12
# Pending Patterns: 3
```

## Common Workflows

### Managing Rules

```bash
# List all rules
penf automation rules list

# Show rule details with version history
penf automation rules show RULE_ID --include-history

# Update a rule (creates new version)
penf automation rules update RULE_ID \
  --conditions @new_conditions.json \
  --message "Expanded sender matching"

# Disable a rule temporarily
penf automation rules disable RULE_ID

# Re-enable a rule
penf automation rules enable RULE_ID

# Rollback to previous version
penf automation rules rollback RULE_ID --version 2

# Delete a rule (soft delete)
penf automation rules delete RULE_ID --reason "No longer needed"
```

### Working with Patterns

The system automatically detects patterns in your categorization decisions.

```bash
# View pending pattern suggestions
penf automation patterns list --status pending

# Output:
# ID: 42
# Pattern: Emails from *@vendor.com -> Project Procurement
# Occurrences: 8
# Confidence: 0.92
# Suggested Rule: assign_project(Procurement)

# Accept pattern and create rule
penf automation patterns accept 42 --name "Vendor Emails"

# Reject pattern suggestion
penf automation patterns reject 42
```

### Progressive Automation Settings

```bash
# Adjust aggressiveness level
penf automation settings set --aggressiveness conservative  # fewer auto-decisions
penf automation settings set --aggressiveness moderate     # balanced (default)
penf automation settings set --aggressiveness aggressive   # more auto-decisions

# Set target automation rate
penf automation settings set --target-rate 0.70  # aim for 70% automation

# Set accuracy floor (minimum acceptable accuracy)
penf automation settings set --accuracy-floor 0.95  # require 95% accuracy

# Disable automatic threshold adjustment
penf automation settings set --auto-threshold-adjustment false
```

## Rule Condition Reference

### Available Fields

| Field | Description | Example Values |
|-------|-------------|----------------|
| `content_type` | Type of content | email, meeting, document |
| `sender` | Email sender address | user@example.com |
| `subject` | Email/meeting subject | "Weekly Report" |
| `project` | Associated project ID | uuid |
| `keywords` | Content keywords | ["urgent", "review"] |
| `confidence` | AI confidence score | 0.85 |

### Available Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `equals` | Exact match | `{"field": "content_type", "operator": "equals", "value": "email"}` |
| `contains` | String contains | `{"field": "sender", "operator": "contains", "value": "@acme.com"}` |
| `greater_than` | Numeric greater | `{"field": "confidence", "operator": "greater_than", "value": 0.9}` |
| `less_than` | Numeric less | `{"field": "confidence", "operator": "less_than", "value": 0.7}` |
| `matches` | Regex match | `{"field": "subject", "operator": "matches", "value": "^\\[URGENT\\]"}` |
| `in` | Value in list | `{"field": "content_type", "operator": "in", "value": ["email", "meeting"]}` |

### Combining Conditions

```json
{
  "operator": "AND",
  "conditions": [
    {"field": "content_type", "operator": "equals", "value": "email"},
    {
      "operator": "OR",
      "conditions": [
        {"field": "sender", "operator": "contains", "value": "@important.com"},
        {"field": "subject", "operator": "contains", "value": "[Priority]"}
      ]
    }
  ]
}
```

## Action Types

| Action | Description | Parameters |
|--------|-------------|------------|
| `categorize` | Assign category | `{"category": "work"}` |
| `tag` | Add tags | `{"tags": ["urgent", "review"]}` |
| `assign_project` | Link to project | `{"project_id": "uuid"}` |
| `archive` | Archive content | `{}` |
| `flag` | Flag for attention | `{"priority": "high"}` |

## Troubleshooting

### Rule Not Matching

```bash
# Test rule to see what's happening
penf automation rules test RULE_ID --content-id CONTENT_ID

# Check if rule is enabled
penf automation rules show RULE_ID
```

### Low Automation Rate

```bash
# Check current thresholds
penf automation thresholds list

# Lower threshold to increase automation (carefully)
penf automation thresholds set --content-type email --value 0.75

# Or adjust aggressiveness
penf automation settings set --aggressiveness aggressive
```

### Accuracy Dropping

```bash
# View recent override patterns
penf automation decisions list --type queued_review

# Check rule effectiveness
penf automation rules show RULE_ID  # shows accuracy_rate

# Disable problematic rule
penf automation rules disable RULE_ID

# Rollback to known-good version
penf automation rules rollback RULE_ID --version 1
```

## Next Steps

1. Start with conservative thresholds (85%) and few rules
2. Let the system learn from your feedback for 2-4 weeks
3. Review pattern suggestions and accept good ones
4. Gradually increase automation as accuracy proves reliable
5. Monitor stats weekly: `penf automation stats`

## Related Documentation

- [Specification](./spec.md) - Detailed requirements
- [Data Model](./data-model.md) - Entity definitions
- [API Contract](./contracts/automation-api.yaml) - Full CLI reference
- [Events](./contracts/events.md) - Event integration
