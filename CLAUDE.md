# Penfold Backend

## How work gets done

CoBuild is **retired** (2026-05) — there is no decompose/dispatch/gate pipeline. Work is tracked in **Linear** (`linear.app/james-brown`, team `PEN`) and done in Claude Code sessions against the repo, then PR'd to GitHub. Keep scope to the issue; file a new Linear issue for adjacent work.

**Read `AGENTS.md` for the full workflow and session-completion protocol.**

## Go toolchain — DO NOT FREESTYLE

Penfold is a multi-module Go workspace. The Go version is **pinned** across all modules, `go.work`, and CI. Agents must not change it ad-hoc.

- **Required Go directive:** `go 1.24.0` in every `go.mod` and in `go.work`
- **CI runtime:** `GO_VERSION: "1.25"` in `.github/workflows/ci.yml` (GitHub runner toolchain; newer than the directive is fine, older is not)
- **Linter:** `golangci-lint v2.8.0` via `golangci/golangci-lint-action@v7` — v1.x will fail typecheck on embedded generics

**Rules:**

1. Do not bump a single `go.mod` to a newer version in isolation — version drift between modules causes golangci-lint typecheck to fail on embedded fields (classic symptom: spurious `Next undefined` errors on Temporal interceptors). See pf-f4ffe2 post-mortem.
2. If a new Go feature is genuinely required, bump **every** `go.mod`, `go.work`, **and** `GO_VERSION` in `ci.yml` in the same commit.
3. Do not downgrade `golangci-lint-action` below `@v7` — earlier versions ship golangci-lint v1.x which can't typecheck the current SDK code.
4. When generating a new module (`go mod init`), explicitly set `go 1.24.0` — don't let it pick up the local toolchain version.

**Quick sanity check:**

```bash
# All go.mod files should report the same version
for f in $(find . \( -name go.mod -o -name go.work \) -not -path "*/node_modules/*"); do
  printf "%s\t%s\n" "$(grep '^go ' "$f" | awk '{print $2}')" "$f"
done | sort -u
```

## Building

```bash
make build              # Build all modules
make test               # Test all modules
make vet                # go vet all modules
```

## Deploying

```bash
penf deploy gateway     # Build + deploy gateway to dev02 (systemd)
penf deploy worker      # Build + deploy worker to dev01 (launchd)
penf deploy ai          # Build + deploy AI coordinator to dev02 (systemd)
penf deploy all         # Deploy all in dependency order
penf deploy --status    # Check all services
```

## Quick Reference

| System | Server | Config |
|--------|--------|--------|
| Penfold | dev02.brown.chat:50051 | ~/.penf/config.yaml |
| Context Palace (KB only) | dev02.brown.chat:5432 | ~/.cobuild/config.yaml |

```bash
penf status / penf health / penf update
./scripts/deploy.sh status
cxp recall / cxp kb        # Context Palace is knowledge-only now (not work tracking)
```
