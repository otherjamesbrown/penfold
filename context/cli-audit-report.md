# penf CLI Audit Report: AI-Native Design Gap Analysis

**Date**: 2026-01-20
**Bead**: pe-nqp9
**Goal**: Audit penf CLI against Git/kubectl patterns for AI-native design

## Executive Summary

The penf CLI has a solid foundation but has inconsistencies that prevent Claude from intuitively understanding it. Key issues:

1. **Inconsistent subcommand naming** (show vs info vs status)
2. **Flag naming conflicts** (--format vs --output)
3. **Missing CRUD operations** (no update/edit commands)
4. **Missing Git-like patterns** (alias, rename, set)

## Detailed Findings

### 1. Command Structure ✅ Good

Top-level follows noun structure (like `kubectl`):
```
penf glossary ...
penf review ...
penf product ...
```

This is correct - noun first, then verb.

### 2. Subcommand Patterns ⚠️ Inconsistent

#### Current State by Command Group

| Group | add | list | show | remove | update | search | Other |
|-------|-----|------|------|--------|--------|--------|-------|
| glossary | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | expand |
| product | ✅ | ✅ | ❌ `info` | ❌ | ❌ | ❌ | alias, event, hierarchy |
| tenant | ❌ | ✅ | ❌ `info` | ❌ | ❌ | ❌ | switch, current |
| workflow | ❌ | ✅ | ❌ `status` | ❌ | ❌ | ❌ | cancel |
| relationship | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ | conflict, entity, network |
| config | ❌ | ❌ | ✅ | ❌ | ❌ `set` | ❌ | init |
| auth | ❌ | ❌ | ❌ `status` | ❌ | ❌ | ❌ | login, logout, refresh |
| review | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | many session commands |

#### Issues

1. **`show` vs `info` vs `status`**: Should standardize on `show` (Git uses `show`)
   - `product info` → `product show`
   - `tenant info` → `tenant show`
   - `workflow status` → `workflow show`
   - `auth status` → `auth show` (or keep as `status` since it's about connection)

2. **Missing `update`/`edit` commands**: Can't modify existing items
   - `glossary` has no way to add aliases to existing terms
   - `product` has no way to update details
   - Need: `penf glossary update TER --add-alias ter`

3. **Missing `remove`/`delete`**: Only `glossary remove` exists
   - Other groups can't delete items

### 3. Flag Naming ✅ Standardized

| Flag | Global | glossary | product | relationship | search |
|------|--------|----------|---------|--------------|--------|
| Output format | `--output` | `-o, --output` | `-o, --output` | `-o, --output` | `-o, --output` |
| Limit | ❌ | `-l, --limit` | ❌ | `-l, --limit` | `-l, --limit` |
| Tenant | ❌ | ❌ | `-t, --tenant` | `-t, --tenant` | ❌ |

**COMPLETED (pe-14kv)**: All commands now use `-o, --output` for output format (kubectl pattern).

### 4. Missing Git-like Patterns (Partial)

| Pattern | Git Example | penf Status | Recommendation |
|---------|-------------|-------------|----------------|
| `alias` | `git config alias.co checkout` | ✅ `product alias`, `glossary alias` | DONE (pe-vtcs) |
| `rename` | `git branch -m old new` | ❌ None | Add where applicable |
| `set` | `git config --set` | Only `config set` | Add `glossary set`, `product set` |
| `get` | `git config --get` | ❌ None | Add for consistency |
| `--dry-run` | `git commit --dry-run` | ❌ None | Add to batch operations |
| `-v, --verbose` | `git status -v` | ❌ None | Add globally |

### 5. Help Text Quality ✅ Generally Good

**Strengths:**
- Long descriptions explain purpose
- Examples included in some commands
- Aliases documented (glossary → terms, dict)

**Weaknesses:**
- Not all commands have examples
- Missing conceptual explanations in some
- No `--help` examples for flags

### 6. Specific Gaps

#### glossary
- ❌ No `update` command (can't add alias to existing term)
- ❌ No `rename` command
- ✅ Has `add`, `remove`, `list`, `show`, `search`

#### product
- ❌ Uses `info` instead of `show`
- ❌ No `remove`/`delete` command
- ❌ No `update` command
- ✅ Has `alias` subcommand (good pattern!)

#### review questions
- ✅ Has `resolve`, `dismiss`, `defer` (domain-specific, fine)
- ❌ No batch operations exposed in CLI (only via `process`)

#### config
- ⚠️ Uses `set` but no `get` (asymmetric)
- ❌ No `list` to show all possible keys
- ❌ `show` exists but `get <key>` would be more Git-like

## Prioritized Recommendations

### P0 - Breaking/Confusing

1. **Standardize output flag**: `--output` / `-o` everywhere (breaking change for `--format`)
2. **Rename `info` → `show`** everywhere for consistency

### P1 - Missing Essential Patterns

3. **Add `glossary update`**: `penf glossary update TER --add-alias OBJE`
4. **Add `glossary alias`**: `penf glossary alias TER OBJE` (shorthand)
5. **Add `--dry-run`** to batch operations
6. **Add `-v, --verbose`** globally

### P2 - Nice to Have

7. **Add `config get <key>`** for symmetry with `set`
8. **Add `product update`** for editing
9. **Add `rename` commands** where applicable
10. **Add `--help` examples** to all flags

## Breaking Changes Summary

| Change | Impact | Migration |
|--------|--------|-----------|
| `--format` → `--output` | Scripts using `-f` | Warn for 2 versions, then remove |
| `info` → `show` | Scripts using `info` | Add `show` as alias first |

## Git/kubectl Pattern Reference

```bash
# Git patterns penf should follow:
git config --get key           # penf config get key
git config --set key value     # penf config set key value  ✅ exists
git config --list              # penf config list
git remote rename old new      # penf glossary rename old new
git branch -m old new          # penf product rename old new

# kubectl patterns penf should follow:
kubectl get pods -o json       # penf glossary list -o json
kubectl describe pod X         # penf glossary show X  ✅ exists
kubectl delete pod X           # penf glossary remove X  ✅ exists
kubectl apply --dry-run        # penf process acronyms --dry-run
```

## Next Steps

1. Create beads for P0 and P1 items
2. Implement `glossary update` and `glossary alias` (pe-vtcs)
3. Standardize flag naming
4. Add `--dry-run` to process commands
