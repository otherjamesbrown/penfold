# CLAUDE.md

Entry point for Claude Code in the Penfold repository.

## Session Start

**Read `context/agents.md`** - That's the main development context.

It tells you:
- What to read and when
- How to find work (`bd ready`)
- Which agent to spawn for which domain
- Where all the other docs are

## Project Summary

**Penfold** is an AI-powered institutional memory system.

| Component | Technology |
|-----------|------------|
| Language | Go 1.22+ |
| CLI | Cobra |
| API | gRPC + Protocol Buffers |
| Database | PostgreSQL 16+ with pgvector |
| Workflows | Temporal |
| Embeddings | MLX (Apple Silicon) |

## Quick Commands

| Task | Command |
|------|---------|
| Find work | `bd ready` |
| Claim work | `bd update <id> --status=in_progress` |
| Before ending | `git push` + `bd sync` |

## Architecture Rules

**CLI must NEVER access the database directly.**

```
CLI → Gateway (gRPC) → Database
```

- All CLI commands call the Gateway via gRPC
- If a gRPC endpoint doesn't exist, create it first (proto + gateway service)
- No `pgxpool` or SQL in `cmd/penf/` - that belongs in `services/gateway/`
- This ensures proper separation, auth, and observability

## CLI Releases

**After any CLI feature or bug fix, bump the version to trigger a release.**

```bash
# Check current version
cat cmd/penf/VERSION

# Bump version (edit file)
echo "v0.2.5" > cmd/penf/VERSION

# Commit and push
git add cmd/penf/VERSION
git commit -m "chore(release): bump version to v0.2.5 [pe-xxxx]"
git push
```

- Version file: `cmd/penf/VERSION`
- Pushing to `main` triggers `.github/workflows/auto-release.yml`
- This creates a git tag, which triggers the release build
- **Always bump version** when CLI changes ship - otherwise users won't get updates

**After releasing, notify the client via Agent Mail:**

```
mcp__agent-mail__send_message(
  project_key: "/Users/james/github/otherjamesbrown/penfold",
  sender_name: "RusticDesert",
  to: ["RedWolf"],
  subject: "CLI Update: v0.2.5 released",
  body_md: "## New Release: v0.2.5\n\n### Changes\n- [list changes]\n\n### Update\n```bash\npenf update\n```\n\nLet me know if you have questions!"
)
```

This ensures the client knows to pull the latest version and what's new.

## Context Structure

```
context/
├── agents.md           ← START HERE
├── development/        # Workflows and standards
├── agents/            # Agent definitions
├── shared/            # Vision, entities, use-cases
├── client/            # User-facing docs (shipped with CLI)
├── ARCHITECTURE.md    # System overview
└── infrastructure.md  # Deployment details
```
