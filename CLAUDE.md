# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with this repository.

## 🚨 AUTONOMOUS DEVELOPMENT RULES

**This is 100% AI-coding assistant driven development.**

### Before Starting Major Work
**ALWAYS ASK:** "Do you want me to start on [specification/feature] and continue until complete?"
**Wait for user confirmation before beginning new major work streams.**

### Autonomy Guidelines
**CONTINUE AUTONOMOUSLY for:**
- Writing code and tests within approved work
- Making technical implementation decisions
- Running commands, tests, commits, and pushes
- Following established patterns
- Bug fixes and improvements within current work

**ONLY ASK USER when:**
- Starting a new specification or major feature
- Business requirements are ambiguous
- Multiple valid approaches exist and user preference is needed
- Adding new architectural components (observability, auth, etc.)
- Technical blockers require user intervention

### Architecture Coordination
**STOP and CHECK before adding:**
- Observability/monitoring systems
- Authentication/authorization
- Message queues or event systems
- Configuration management
- Caching layers, backup/recovery, CI/CD

**Search codebase first** - if similar infrastructure exists, use it or ask how to integrate.

## 📋 BEADS WORKFLOW - MANDATORY

**NEVER start work without a bead.**

### Essential Commands
```bash
bd ready                              # Find available work
bd create --title="..." --type=task  # Create new work
bd update <id> --status=in_progress   # Claim work
bd close <id> --reason="summary"      # Complete work
bd sync                               # Sync with git
```

### Critical Rules
- Find or create bead BEFORE writing code
- Update status when starting: `bd update <id> --status=in_progress`
- Reference bead in commits: `feat(component): description [pe-xxx]`
- Close with commit hash: `bd close <id> --reason="commit <hash>: summary"`
- Run `bd sync` before ending session

### After Closing a Bead
When closing a bead that belongs to an epic, check if all sibling beads are also closed:
```bash
bd dep tree <epic-id>                 # Check if all children are closed
```
If all child beads are closed, suggest closing the parent epic to the user.

## 🎯 EPIC-BASED BEAD MANAGEMENT - VITAL

**ALL beads must be associated with an epic to prevent proliferation.**

### Epic Structure
```bash
bd list --type=epic                   # Show all epics
bd dep add <bead-id> <epic-id>        # Associate bead with epic
bd dep tree <epic-id>                 # Show epic's bead tree
```

### Epic Creation Rules
**BEFORE creating any new bead:**
1. **Check existing epics** - Does it fit in current epic?
2. **Epic first** - Create epic before related beads
3. **Batch related work** - Group similar beads under same epic
4. **Link immediately** - `bd dep add <new-bead> <epic-id>`

### When to Create New Epics
- **Major feature areas** (5+ related beads expected)
- **Cross-cutting initiatives** (affects multiple systems)
- **Implementation phases** (setup → implement → polish)
- **Maintenance categories** (quarterly tasks, cleanup)

### Rare Non-Epic Exceptions
**ONLY create standalone beads for:**
- Immediate blockers preventing all work
- Emergency security issues
- Quick research tasks (<30 min, inform epic planning)
- Epic creation itself

### Epic Naming Convention
- `[EPIC] SpecKit: Complete All Feature Specifications`
- `[EPIC] Operationalization: Dev Agents and Documentation`
- `[EPIC] Integration: Cross-Cutting System Concerns`
- `[EPIC] Maintenance: Cleanup and Audits`

## 🔄 SESSION CLOSE PROTOCOL

**Work is NOT complete until pushed to remote.**

```bash
git status              # Check changes
git add <files>         # Stage changes
bd sync                 # Sync beads
git commit -m "..."     # Commit with bead reference
git push                # MUST PUSH TO REMOTE
```

**NEVER leave work stranded locally.**

## 🎯 FINDING CURRENT PRIORITIES

### Dynamic Priority Discovery
```bash
bd ready                # Current available work
bd stats                # Project health overview
bd list --status=open   # All open work
```

### Priority Guidelines
1. **Blocked work** - Unblock others first
2. **P0/P1 priorities** - Critical path items
3. **Complete epic chains** - Finish what's started
4. **Follow dependencies** - Use bead dependency chains

**When in doubt:** Run `bd ready` and ask user which direction they prefer.

## ⚙️ DEVELOPMENT STANDARDS

### Code Quality
- **TDD Required**: Tests first, ensure they fail, then implement
- **Go conventions**: Follow standard Go formatting (`gofmt`)
- **Zero warnings** from `go vet` and `staticcheck`
- **80% test coverage** minimum for core packages

### Git Workflow
- All commits must reference bead: `[pe-xxx]`
- Push to remote before ending session
- Follow constitutional principles in `project-constitution.md`

### Architecture Principles
- **gRPC + Protocol Buffers** for service communication
- **Temporal workflows** for durable execution
- **Test-driven development** workflow
- **Complete workflows** - specification to working, tested, committed code

### CLI Architecture (MANDATORY)
**The CLI must use the Gateway service via gRPC. Never call the database directly.**

```
CLI (penf) → Gateway (gRPC) → Database
```

- All CLI commands that need data must go through `services/gateway`
- Use gRPC clients to call Gateway services (see `cmd/penf/cmd/product.go` for pattern)
- File parsing and user interaction stay in CLI (hybrid approach)
- Database operations, duplicate detection, and business logic live in Gateway
- Proto definitions in `api/proto/<service>/v1/`
- Gateway services in `services/gateway/<service>service/`

## 🤖 CLAUDE-NATIVE WORKFLOWS

**Use batch processing instead of one-at-a-time CLI commands.**

Penfold has Claude-native workflows that provide full context for intelligent batch processing.
Instead of executing commands one at a time, use the `penf process` commands.

**ALWAYS READ THESE FILES AT SESSION START:**
- `~/.penf/processes.md` - Available workflows and how to run them
- `~/.penf/preferences.md` - User's personal preferences (NEVER modify)

### Available Workflows

| Workflow | Context Command | Batch Command |
|----------|-----------------|---------------|
| Acronym Review | `penf process acronyms context` | `penf process acronyms batch-resolve` |

### Intelligent Processing Pattern

1. **Get Full Context** (single command):
   ```bash
   penf process acronyms context --output json
   ```
   Returns: all pending questions, existing glossary, stats, workflow guidance

2. **Analyze Intelligently**:
   - Categorize: known tech terms, duplicates, needs user input
   - Group: similar terms, potential typos
   - Auto-resolve: standard acronyms (MVP, API, AWS, etc.)

3. **Present Summary to User**:
   ```
   Found 15 acronym questions:
   - 8 standard tech terms (auto-resolving)
   - 3 already in glossary (dismissing)
   - 4 need your input: [list ambiguous items]
   ```

4. **Execute Batch** (after user confirms):
   ```bash
   penf process acronyms batch-resolve '{"resolutions":[...],"dismissals":[...]}'
   ```

### Workflow Documentation
See `context/workflows/` for detailed workflow guides:
- `acronym-review.md` - Acronym processing decision criteria and patterns

### When to Use Batch vs Interactive
- **Batch**: Processing queues, bulk operations, repetitive tasks
- **Interactive**: Single items needing detailed context, debugging

## 🏗️ PROJECT CONTEXT

**Penfold** is an AI-powered personal information system that aggregates and correlates information from communication channels (email, Slack, documents, meetings) into a queryable institutional memory.

### Current Architecture (Go)
- **CLI Tool** (`cmd/penf`): Cobra-based CLI with all commands
- **Gateway** (`services/gateway`): gRPC + HTTP gateway with auth
- **Gmail Connector** (`services/gmail`): OAuth2 PKCE, sync, push notifications
- **Worker** (`services/worker`): Temporal activities and workflows
- **Database** (`pkg/db`): PostgreSQL + pgvector utilities
- **Protos** (`api/proto`): gRPC service definitions

### Technology Stack
- **Go 1.22+** with Cobra CLI, gRPC
- **PostgreSQL 16+** with pgvector extension
- **Temporal** for workflow orchestration
- **Protocol Buffers** for API definitions
- **MLX** embeddings sidecar (Python, Apple Silicon)

---

## 📍 QUICK REFERENCE

- **Find work**: `bd ready`
- **Project status**: `bd stats`
- **User preferences**: `~/.penf/preferences.md` (read at session start, NEVER modify)
- **Process definitions**: `~/.penf/processes.md` (read at session start for available workflows)
- **Development standards**: `project-constitution.md`
- **Infrastructure**: `context/infrastructure.md` (architecture, services, connections)
- **Workflow guides**: `context/workflows/` (batch processing patterns)
- **Credentials**: `source ~/github/otherjamesbrown/secrets/.env.penfold` (NEVER hardcode passwords)
- **Batch processing**: `penf process <workflow> context` then `batch-resolve`
- **When stuck**: Ask user for direction on priorities
- **Before ending**: `git push` + `bd sync`

## Active Technologies
- Go 1.22+ with Cobra, gRPC, Protocol Buffers
- PostgreSQL 16+ with pgvector extension
- Temporal for workflow orchestration
- Redis for caching (optional)
- MLX embeddings sidecar (Python) for Apple Silicon
- Go 1.22+ + Cobra CLI, gRPC, Protocol Buffers, pgx (PostgreSQL driver), MLX embeddings sidecar (013-content-enrichment)
- PostgreSQL 16+ with existing schema (extending `pkg/mentions/`, `pkg/glossary/`) (013-content-enrichment)

## Recent Changes
- CLI gRPC Refactoring: Complete (pipeline, product, ingest commands use Gateway)
- Go Migration Phase 0-5: Complete (all services migrated to Go)
- Python Decommissioning: Complete (penf_lib, app, observability_lib removed)
- Gmail OAuth2 PKCE: Complete with AES-256-GCM token encryption
- Search Service: Complete with hybrid full-text + vector search
- Review Service: Complete with daily review workflows
