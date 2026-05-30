# Agent Instructions

## Work tracking: Linear (not CoBuild, not beads)

Work is tracked in **Linear** (`linear.app/james-brown`, team `PEN`, issue identifiers `PEN-<n>`). Agents create, update, query, and get assigned issues via the **Linear GraphQL API** (`https://api.linear.app/graphql`); the API key lives in `~/github/otherjamesbrown/secrets/linear.app`. CoBuild is **retired** (2026-05) — there is no decompose/dispatch/gate pipeline, and `bd`/beads is no longer used.

**To change code:**

1. Pick (or be assigned) a Linear issue in the `PEN` team.
2. Do the work in a Claude Code session in this repo — run its build/test commands (`make build`, `make test`, `make vet`) per the rules in `CLAUDE.md`.
3. Open a PR on GitHub. Reference the `PEN-<n>` issue identifier in the commit/PR so Linear links the PR and advances the issue.
4. Keep scope to the issue. File a **new** Linear issue for adjacent work rather than expanding the current one.

For research, synthesis, or strategy work, use the M-Intel session rather than this repo.

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create Linear issues (team `PEN`) for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items in Linear
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
