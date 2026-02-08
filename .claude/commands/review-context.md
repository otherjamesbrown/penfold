# Context Review

Review all context documentation for consistency, completeness, and quality. Context drift is a common problem - this command ensures everything lines up.

## Arguments: $ARGUMENTS

Optional: Specific area to focus on (e.g., "client", "agents", "cli")

## Instructions

### Overview

Context lives in multiple places that must stay synchronized:

| Location | Purpose | Consumed By |
|----------|---------|-------------|
| **Context Palace** `mycroft-playbook` | Dev orchestrator identity | Dev Claude |
| **Context Palace** `mycroft-agent-*` | Sub-agent definitions | Dev sub-agents |
| **Context Palace** `mycroft-shared-*` | Shared docs (vision, entities) | Both dev and client |
| `context/client/` | Client docs (source of truth) | Client Claude (via penf init) |
| `cmd/penf/main.go` | CLI --help text | Users, AI assistants |
| `cmd/penf/cmd/init.go` | Doc installation logic | penf init/update |
| `.claude/agents/` | Claude Code agent definitions | Claude Code |

### Phase 1: Inventory

Read and inventory all context files:

```bash
# List dev context in Context Palace
cp knowledge list --label mycroft-context

# List client context on disk
find context/client -name "*.md" -type f | sort

# List CLI help references
grep -r "docs/" cmd/penf/main.go cmd/penf/cmd/*.go | grep -v "_test.go"

# List .claude agents
ls -la .claude/agents/
```

### Phase 2: Reference Chain Validation

Verify the reference chain is complete and all links resolve:

**Client reference chain:**
1. `cmd/penf/main.go` --help text → references `docs/assistant-rules.md`
2. `cmd/penf/cmd/init.go` → downloads files from `context/client/`
3. `context/client/assistant-rules.md` → links to other docs
4. Cross-links between docs all resolve

**Dev reference chain:**
1. `CLAUDE.md` → bootstraps from `cp knowledge show mycroft-playbook`
2. `mycroft-playbook` → references sub-agent docs and context docs via `cp knowledge show`
3. `mycroft-agent-*` → consistent naming and cross-references
4. `.claude/agents/*.md` → point to correct CP knowledge docs

For each reference found:
- Verify the target file exists
- Verify the content at the target is relevant
- Note any broken links or stale references

### Phase 3: Consistency Checks

**Naming consistency:**
- Agent files use `*-dev.md` format (not `dev-*.md`)
- Agent names in Agent Mail match (RedWolf for client, RusticDesert for dev)
- Cross-references use consistent names

**Content consistency:**
- `context/client/index.md` navigation matches actual client files
- `context/client/processes.md` workflow links resolve
- `context/client/assistant-rules.md` file listings are accurate
- CLI --help DOCUMENTATION section matches what `penf init` installs

**No duplicates:**
- Client docs only in `context/client/` (not duplicated in `docs/` or `cmd/penf/cmd/templates/`)
- Dev context only in Context Palace (not duplicated on disk)
- No stale copies in unexpected locations

### Phase 4: Quality Assessment

For each major context file, assess:

1. **Clarity**: Is the purpose clear in the first few lines?
2. **Completeness**: Does it cover what it claims to cover?
3. **Currency**: Does it reflect current code/architecture?
4. **Links**: Do all internal links work?
5. **Actionability**: Can the reader act on this information?

Rate each file: Good / Needs Update / Stale

### Phase 5: Report

Create a report with:

```markdown
# Context Review Report

**Date:** [timestamp]
**Focus:** [all | $ARGUMENTS]

## Summary

- Total context files reviewed: X
- Files with issues: Y
- Broken links found: Z

## Reference Chain Status

### Client Chain
- [ ] CLI --help → docs/assistant-rules.md (status)
- [ ] init.go downloads correct files (status)
- [ ] assistant-rules.md links valid (status)
- [ ] Cross-links resolve (status)

### Dev Chain
- [ ] CLAUDE.md → cp knowledge show mycroft-playbook (status)
- [ ] mycroft-playbook → sub-agent docs via cp knowledge show (status)
- [ ] .claude/agents/ → cp knowledge show mycroft-agent-* (status)

## Issues Found

### Broken Links
| Source File | Broken Link | Should Point To |
|-------------|-------------|-----------------|
| ... | ... | ... |

### Stale Content
| File | Issue | Recommendation |
|------|-------|----------------|
| ... | ... | ... |

### Naming Inconsistencies
| Location | Found | Expected |
|----------|-------|----------|
| ... | ... | ... |

## Quality Assessment

| File | Clarity | Complete | Current | Links | Action |
|------|---------|----------|---------|-------|--------|
| mycroft-playbook (CP) | Good | Good | Good | Good | None |
| ... | ... | ... | ... | ... | ... |

## Recommendations

1. [Priority 1 fix]
2. [Priority 2 fix]
3. ...

## Files Verified Good
- file1.md
- file2.md
- ...
```

### Phase 6: Quick Fixes

If issues are found that are simple to fix (broken links, typos, stale file references):

1. Ask user: "Found X quick fixes. Apply them? (yes/no)"
2. If yes, make the edits directly
3. List changes made

For larger issues, create shards:
```sql
SELECT create_shard('penfold', 'docs: [brief description]', 'Details', 'task', 'agent-mycroft');
```

### Output

Print the report to stdout and save to `review/context/[timestamp].md`.

If all checks pass:
```
Context Review: PASSED
All reference chains valid, no broken links, documentation current.
```

If issues found:
```
Context Review: X ISSUES FOUND
See report for details: review/context/[timestamp].md
Quick fixes available: Y (run with --fix to apply)
```
