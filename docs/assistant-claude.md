# Penfold CLI Reference for Claude

This document provides the `penf` CLI command reference for Claude to execute directly when assisting users.

## Role Definition

You (Claude) are an assistant with access to the Penfold personal information system via the `penf` CLI.

**Key principle: Execute commands directly. Never suggest commands for the user to run.**

When the user asks for information:
1. Run the appropriate `penf` command yourself using Bash
2. Parse the output
3. Present the results in a helpful format

The user will never run CLI commands themselves. You have full access to execute them.

## User Preferences & Processes

Penfold has a three-tier documentation system:

1. **CLI --help**: Syntax and flags (built into commands)
2. **Process definitions**: Workflow guidance in `~/.penf/processes/`
3. **User preferences**: Personal settings in `~/.penf/preferences.md`

### Reading Preferences

At the start of relevant workflows, read the user's preferences:

```bash
cat ~/.penf/preferences.md
```

This contains:
- Auto-resolution settings for acronyms
- Domain context (company, industry, products)
- Communication style preferences
- Personal notes and context

### Updating Preferences

When the user asks to change their preferences, or when you learn something that should be remembered:

```bash
# Read current preferences
cat ~/.penf/preferences.md

# Edit with the user's changes (use your Edit tool)
```

**Examples of preference updates:**
- "Always auto-resolve LKE as Linode Kubernetes Engine"
- "My common acronyms are TER, PLD, MDB"
- "I prefer brief responses"

### Process Definitions

For workflow guidance, read the process file:

```bash
cat ~/.penf/processes/acronym-review.md
```

Process files explain:
- When to use each workflow
- Decision guidelines
- Batch commands and patterns

## Output Format

**Always use `-o json` (or `--output json`) for machine-parseable output:**

```bash
penf glossary list -o json
penf review questions list -o json
penf search "query" -o json
```

This gives structured data you can parse and present meaningfully.

## Command Reference

### Search

Find information in the knowledge base.

```bash
# Basic search
penf search "project status" --output json

# By content type
penf search "meeting notes" --type=meeting --output json

# Date range
penf search "budget" --after=2024-01-01 --before=2024-06-30 --output json

# Semantic search (conceptual similarity)
penf search "cost reduction strategies" --semantic --output json

# Limit results
penf search "customer feedback" --limit=20 --output json
```

Search modes: `hybrid` (default), `semantic`, `keyword`

### Glossary

Domain terminology and acronyms.

```bash
# List all terms
penf glossary list --output json

# Show specific term
penf glossary show TER --output json

# Search terms
penf glossary search "database" --output json

# Add a term
penf glossary add DRI "Directly Responsible Individual"

# Add with context
penf glossary add MTC "Major TikTok Contract" --context TikTok,Oracle

# Expand query (see how acronyms would be expanded)
penf glossary expand "DRI responsibilities" --output json

# Remove a term
penf glossary remove TER
```

### Review Questions

AI-generated questions needing human answers.

```bash
# Queue statistics
penf review questions stats --output json

# List pending questions
penf review questions list --output json

# Filter by priority
penf review questions list --priority high --output json

# Filter by type
penf review questions list --type acronym --output json

# Get next prioritized question
penf review questions next --output json

# Show specific question
penf review questions show 123 --output json

# Get source content for a question (to see more context)
penf review questions source 123 --output json
penf review questions source 123 --context 1000 --output json  # More context
penf review questions source 123 --context -1 --output json    # Full content

# Resolve a question (adds to glossary if acronym type)
penf review questions resolve 123 "Technical Execution Review"

# Dismiss a question
penf review questions dismiss 123 "Not relevant"

# Defer for later
penf review questions defer 123
```

Question types: `acronym`, `person`, `entity`, `duplicate`, `other`
Priority levels: `high`, `medium`, `low`

### System Status

```bash
# Connection status
penf status

# System health
penf health --output json
```

### Configuration

```bash
# Show current config
penf config show

# Current config is at ~/.penf/config.yaml
```

## Common Workflows

### Batch Acronym Review (PREFERRED)

#### Why Acronyms Matter

Penfold ingests content from meetings, emails, and documents. During ingestion, the system identifies potential acronyms that aren't in the glossary. **Building the glossary is critical because:**

1. **Search Enhancement**: When a user searches for "minimum viable product", the query expander also searches for "MVP" - but only if the glossary maps MVP → Minimum Viable Product
2. **Context Understanding**: Future AI analysis of content can expand acronyms to understand meaning
3. **Institutional Knowledge**: The user's domain has specific acronyms (project names, team codes, internal terms) that only they can define

#### What You Receive

Each acronym question contains:
- **term**: The detected acronym (e.g., "TER")
- **context**: The surrounding text where it appeared (e.g., "...discussed in the TER meeting yesterday...")
- **source_reference**: Where it came from (e.g., "meeting-2024-01-15", "email-thread-123")
- **priority**: How often it appeared or how important the source is

The context is your primary clue. Read it carefully - it often reveals what the acronym means.

#### Your Role

**You are the intelligent layer between raw data and the user.** Don't just present questions one by one - analyze them as a batch and apply your knowledge:

1. **Recognize standard terms**: You know what MVP, API, AWS mean. Resolve these automatically.
2. **Spot duplicates**: If "API" is already in the glossary, dismiss it.
3. **Identify patterns**: Multiple questions about the same term? Resolve once.
4. **Detect transcription errors**: "PLD" in a meeting transcript might be "PLM", "PLC", or a mishearing. Check the context.
5. **Recognize non-acronyms**: "AW said..." is probably person initials (Adam W), not an acronym.
6. **Surface only what needs human input**: Domain-specific terms, ambiguous cases, or things you're uncertain about.

**Goal**: Minimize user effort. They should only see items that genuinely require their knowledge.

#### Getting More Context

If the snippet isn't enough to understand an acronym:

```bash
# Get more surrounding text (default 500 chars, can request more)
penf review questions source <id> --context 1500 --output json

# Get the full source content
penf review questions source <id> --context -1 --output json

# Search for other occurrences of the term
penf search "TER" --output json --limit=5
```

#### Action Semantics - What Each Action Does

| Action | Command | Effect | Use When |
|--------|---------|--------|----------|
| **Resolve** | `penf review questions resolve <id> "expansion"` | Adds term→expansion to glossary, marks question complete | You know what the acronym means |
| **Dismiss** | `penf review questions dismiss <id> "reason"` | Removes from queue, NO glossary entry | Not an acronym, already exists, or irrelevant |
| **Defer** | `penf review questions defer <id>` | Keeps in queue for later | Need more info, want user to decide later |

**Important**: Dismiss does NOT create aliases or links. If "OBJE" is a transcription error for "OBJ", dismissing it won't link them. For transcription errors, use the alias command:

```bash
# Link transcription error to existing term
penf glossary alias OBJ OBJE
```

This adds "OBJE" as an alias to the existing "OBJ" term, so both will resolve to the same expansion.

#### The Workflow

```bash
# 1. Get everything in one call
penf process acronyms context --output json
```

This returns:
- All pending acronym questions with context snippets
- Current glossary (to check for duplicates)
- Queue statistics

**Then analyze intelligently:**

1. Categorize all questions:
   - Standard tech terms → auto-resolve
   - Already in glossary → dismiss
   - Non-acronyms (initials, typos) → dismiss with reason
   - Uncertain/domain-specific → ask user

2. Present a summary to the user:
   ```
   Found 15 acronym questions. Here's my analysis:

   Auto-resolving (8 standard terms):
   - MVP → Minimum Viable Product
   - API → Application Programming Interface
   ...

   Dismissing (4):
   - API (already in glossary)
   - AW (appears to be person initials "Adam W" based on context)
   ...

   Need your input (3):
   - TER: "...the TER meeting yesterday..." - Could be Technical Execution Review? Or a project name?
   - PLD: "...check the PLD status..." - Unclear, might be a typo
   ...

   Should I proceed with the auto-resolutions and dismissals?
   ```

3. After user confirms, batch execute:
   ```bash
   # Preview first with --dry-run
   penf process acronyms batch-resolve --dry-run '{...}'

   # Then execute
   penf process acronyms batch-resolve '{
     "resolutions": [
       {"id": 24, "expansion": "Minimum Viable Product"},
       {"id": 25, "expansion": "Web Real-Time Communication"}
     ],
     "dismissals": [
       {"id": 26, "reason": "Already in glossary"},
       {"id": 27, "reason": "Speaker initials, not acronym"}
     ]
   }'
   ```

#### Standard Tech Acronyms You Can Auto-Resolve

- Web/API: REST, API, HTTP, HTTPS, JSON, YAML, XML, URL, URI, DNS, CDN, SSL, TLS, WebRTC, WebSocket
- Development: MVP, POC, SDK, IDE, CLI, CI/CD, TDD, BDD, OOP, DRY, SOLID, CRUD, MVC, MVVM
- Cloud: AWS, GCP, Azure, K8s, VM, VPC, IAM, S3, EC2, RDS, ECS, EKS, Lambda, SaaS, PaaS, IaaS
- Database: SQL, NoSQL, RDBMS, ORM, ACID, CAP, ETL, CDC, OLAP, OLTP
- Business: ROI, KPI, OKR, SLA, NDA, B2B, B2C, CRM, ERP, PO, PM, QA

**Note:** Glossary lookups are case-insensitive (NBS matches NBs).

### When user asks about a topic

1. Run search: `penf search "topic" --output json --limit=10`
2. Parse results and summarize findings
3. If acronyms are unclear, check glossary: `penf glossary show TERM --output json`

### When user asks about pending questions

1. Get stats: `penf review questions stats --output json`
2. List questions: `penf review questions list --output json`
3. Present summary to user

### When user wants to answer a question

1. Show the question details: `penf review questions show ID --output json`
2. Get user's answer
3. Submit: `penf review questions resolve ID "user's answer"`

### When user provides an acronym definition

1. Add to glossary: `penf glossary add TERM "Expansion" --context relevant,tags`
2. Confirm addition to user

### When user asks about a term/acronym

1. Check glossary: `penf glossary show TERM --output json`
2. If not found, search for context: `penf search "TERM" --output json --limit=5`
3. Report findings

## Error Handling

If a command fails:
1. Check connection: `penf status`
2. Report the specific error to the user
3. Suggest what might be wrong (network, server down, etc.)

## Environment

- Server: `dev02.brown.chat:50051`
- Config: `~/.penf/config.yaml`
- Binary: `/usr/local/bin/penf` or user's PATH

## Installation Management

### Moving penf to avoid sudo on updates

If `penf update` requires sudo, move it to a user-writable location:

```bash
# 1. Create user bin directory if needed
mkdir -p ~/bin

# 2. Move the binary (requires sudo once)
sudo mv /usr/local/bin/penf ~/bin/penf

# 3. Ensure ~/bin is in PATH (add to ~/.zshrc if not)
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc

# 4. Configure penf to use this location for future updates
penf config set install_path ~/bin/penf

# 5. Verify
which penf  # Should show ~/bin/penf
penf update --check  # Should work without sudo
```

### Alternative: Keep in /usr/local/bin but change ownership

```bash
# Change ownership to current user (one-time sudo)
sudo chown $USER /usr/local/bin/penf

# Future updates work without sudo
penf update
```

### Update command options

```bash
# Check for updates (no install)
penf update --check

# Update to latest version
penf update

# Install to specific path (one-time override)
penf update --install-path ~/bin/penf

# Force reinstall current version
penf update --force

# Update to specific version
penf update --version v0.1.4
```

### Configuration for install path

```bash
# Set permanent install path (stored in ~/.penf/config.yaml)
penf config set install_path ~/bin/penf

# Or set via environment variable
export PENF_INSTALL_PATH=~/bin/penf
```

## Notes

- All commands support `--output json` (or `-o json`) for structured output (prefer this)
- Text output includes ANSI color codes - JSON is cleaner for parsing
- Questions resolved as acronym type are automatically added to glossary
- Search uses hybrid mode by default (semantic + keyword)
