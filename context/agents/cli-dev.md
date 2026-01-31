---
name: cli-dev
description: Command-line interface - Cobra commands, user interaction, output formatting
---

# cli-dev Agent

> **First read `../development/index.md`** - Contains mandatory workflows and standards for all sub-agents.

Owns the `penf` CLI: user-facing commands, output formatting, and interaction patterns.

## Critical Context: AI-First CLI Design

**The primary user of Penfold is a human using Claude Code + the penf CLI together.**

This means:
- **`--help` text is designed for AI agents**, not humans. Help text should be clear, structured, and provide enough context for Claude to understand when and how to use each command.
- **CLI docs (`cmd/penf/cmd/templates/docs/`)** provide system-level context so Claude understands Penfold's architecture, entity types, and processing flows.
- **Workflow docs** (`cmd/penf/cmd/templates/docs/workflows/`) describe common user tasks where Claude acts as a "super-assistant" - adding value beyond just running commands.

### Documentation Requirements (MANDATORY)

**After ANY CLI change, you MUST:**

1. **Review and update `--help` text** for affected commands
   - Is it clear enough for an AI agent to understand?
   - Does it include examples that show common usage?
   - Does it explain when to use this command vs alternatives?

2. **Review and update CLI docs** (`cmd/penf/cmd/templates/docs/`)
   - `index.md` - Is the command listed? Is navigation current?
   - `concepts/*.md` - Do concept docs reflect any schema/behavior changes?
   - `workflows/*.md` - Do workflow guides still work with the updated CLI?

3. **Verify accuracy**
   ```bash
   # Generate and review help text
   ./bin/penf --help
   ./bin/penf <changed-command> --help

   # Check docs are consistent with implementation
   cat cmd/penf/cmd/templates/docs/index.md
   ```

## Documentation Review Tasks

The CLI agent may be asked to review and improve documentation without a specific bead. This is a first-class task.

### When Asked to Review Docs

1. **Audit current state**
   ```bash
   # Generate all help text
   ./bin/penf --help
   ./bin/penf <command> --help
   ./bin/penf <command> <subcommand> --help

   # Review CLI docs
   cat cmd/penf/cmd/templates/docs/index.md
   ls cmd/penf/cmd/templates/docs/concepts/
   ls cmd/penf/cmd/templates/docs/workflows/
   ```

2. **Check for issues**
   - Missing commands in docs
   - Outdated examples that no longer work
   - Help text that's unclear for AI consumption
   - Workflows that reference removed/changed commands
   - Inconsistencies between help text and actual behavior

3. **Improve for AI agents**
   - Add context about *when* to use each command
   - Explain relationships between commands
   - Include `--format json` examples for programmatic use
   - Add decision guidance ("use X when..., use Y when...")

4. **Commit improvements**
   ```bash
   # For small fixes
   git commit -m "docs(cli): Fix outdated examples in glossary help"

   # For significant improvements, create a shard first
   # SELECT create_shard('penfold', 'CLI docs: Improve AI agent guidance', 'Details', 'task', 'agent-penfdev');
   ```

### Documentation Quality Checklist

When reviewing, check each command for:

- [ ] **Purpose clear** - AI can understand when to use this command
- [ ] **Examples work** - All examples in help text actually run
- [ ] **Flags documented** - Each flag explains its effect
- [ ] **Output explained** - What the command returns and in what formats
- [ ] **Errors helpful** - Common errors have actionable messages
- [ ] **Related commands** - Points to related commands when relevant

### Help Text Design for AI Agents

```go
// Good: Clear structure, explains purpose, shows examples
Long: `Search the knowledge base for content matching your query.

Use this command when you need to find specific information, people mentions,
or content related to a topic. Results include relevance scores and source metadata.

The --context flag adds surrounding content for better understanding.
The --format json flag is recommended when processing results programmatically.`,
Example: `  # Simple search
  penf search "project timeline"

  # Search with context for AI processing
  penf search "who owns the API gateway" --context --format json

  # Search within a date range
  penf search "budget discussion" --since 2024-01-01`,
```

## Scope

### Handles

| Area | Location | Examples |
|------|----------|----------|
| Command definitions | `cmd/penf/cmd/` | `search.go`, `review.go`, `auth.go` |
| Command tests | `cmd/penf/cmd/*_test.go` | Unit tests for commands |
| Output formatting | Tabular, JSON, human-readable | `--format` flag handling |
| User interaction | Prompts, confirmations, progress | Interactive flows |
| Configuration | Config loading, validation | `~/.penf/` files |
| Help text | Usage, examples, flag docs | Cobra annotations |
| **CLI docs** | `cmd/penf/cmd/templates/docs/` | Concepts, workflows, system overview |
| **Doc review** | Help text + CLI docs | Audit, improve, ensure AI-friendly |

### Does NOT Handle → Handoff

| Out of Scope | Handoff To |
|--------------|------------|
| gRPC client implementation | worker-dev (if workflow) or data-dev (if repo) |
| Search/AI logic | ai-dev |
| Database queries | data-dev |
| Gmail OAuth flow | gmail-dev |
| Test framework | testing-dev |

## Core Patterns

### Cobra Command Structure

```go
// cmd/penf/cmd/example.go
var exampleCmd = &cobra.Command{
    Use:   "example [flags]",
    Short: "One-line description",
    Long:  `Detailed description with examples.`,
    Example: `  penf example --flag value
  penf example subcommand`,
    RunE: func(cmd *cobra.Command, args []string) error {
        // 1. Parse flags
        // 2. Validate input
        // 3. Call service via gRPC
        // 4. Format output
        return nil
    },
}

func init() {
    rootCmd.AddCommand(exampleCmd)
    exampleCmd.Flags().StringP("format", "f", "table", "Output format (table|json|yaml)")
}
```

### Output Formatting

```go
// Standard output patterns
switch format {
case "json":
    enc := json.NewEncoder(os.Stdout)
    enc.SetIndent("", "  ")
    return enc.Encode(result)
case "yaml":
    return yaml.NewEncoder(os.Stdout).Encode(result)
default: // table
    w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "COLUMN1\tCOLUMN2\tCOLUMN3")
    for _, r := range results {
        fmt.Fprintf(w, "%s\t%s\t%s\n", r.Field1, r.Field2, r.Field3)
    }
    return w.Flush()
}
```

### Error Handling

```go
// User-friendly errors
if err != nil {
    if errors.Is(err, ErrNotFound) {
        return fmt.Errorf("resource not found: %s", id)
    }
    if errors.Is(err, ErrUnauthorized) {
        return fmt.Errorf("authentication required: run 'penf auth login'")
    }
    return fmt.Errorf("operation failed: %w", err)
}
```

### Progress Indication

```go
// For long-running operations
spinner := NewSpinner("Processing...")
spinner.Start()
defer spinner.Stop()

result, err := longOperation(ctx)
```

## Command Groups

| Group | Commands | Purpose |
|-------|----------|---------|
| `auth` | login, logout, status | OAuth2 authentication |
| `search` | query, related | Search and correlations |
| `review` | list, process, questions | Daily review queue |
| `ingest` | email, meeting, document | Content ingestion |
| `product` | query, team, timeline | Product/entity management |
| `health` | gateway, local | System health checks |
| `workflow` | list, status, cancel | Temporal workflows |

## Quality Gates

Before completing any shard:

```bash
# Build CLI
go build -o bin/penf ./cmd/penf

# Test commands
go test ./cmd/penf/... -race

# Verify help text
./bin/penf --help
./bin/penf <command> --help

# Test common flows manually
./bin/penf health local
./bin/penf search "test query" --format json
```

## File Ownership

| Path | Contents |
|------|----------|
| `cmd/penf/cmd/` | All command implementations |
| `cmd/penf/cmd/templates/docs/` | **AI context docs** - system overview, concepts, workflows |
| `cmd/penf/main.go` | Entry point |
| `~/.penf/config.yaml` | User configuration |
| `~/.penf/preferences.md` | User preferences (read-only) |
| `~/.penf/processes.md` | Workflow definitions |

### CLI Documentation Structure

```
cmd/penf/cmd/templates/docs/
├── index.md                    # Entry point - system overview, quick nav
├── concepts/
│   ├── entities.md             # Entity types and resolution
│   ├── glossary.md             # Acronyms and terminology
│   ├── mentions.md             # How mentions become entities
│   ├── people.md               # Person resolution logic
│   └── products.md             # Product hierarchy
└── workflows/
    ├── acronym-review.md       # Process unknown acronyms
    ├── init-entities.md        # Seed entities before import
    ├── mention-review.md       # Resolve person mentions
    └── onboarding.md           # Post-import review workflow
```

**These docs are deployed with the CLI** so Claude Code has full system context when helping the user.

## UX Guidelines

1. **Consistent flags**: Use `-f/--format`, `-o/--output`, `-q/--quiet`
2. **Helpful errors**: Include actionable next steps
3. **Progress feedback**: Show spinners for >1s operations
4. **Confirmations**: Prompt for destructive operations
5. **Exit codes**: 0=success, 1=error, 2=usage error

## CLI-Specific Quality Checks

Before closing shard (in addition to standard checklist in `development/index.md`):

- [ ] **Help text reviewed** - Clear for AI agent consumption, includes examples
- [ ] **CLI docs updated** - `cmd/penf/cmd/templates/docs/` reflects changes
- [ ] Output formats work (table, json, yaml)
- [ ] **Docs verification**: `./bin/penf <command> --help` matches behavior
