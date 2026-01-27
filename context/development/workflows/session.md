# Session Protocol

## Session Close

**Work is NOT complete until pushed to remote.**

```bash
git status              # Check changes
git add <files>         # Stage changes
bd sync                 # Sync beads
git commit -m "..."     # Commit with bead reference
git push                # MUST PUSH TO REMOTE
```

**Rules:**
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
- Create beads for any remaining work before ending

## Git Workflow

- All commits must reference bead: `[pe-xxx]`
- Push to remote before ending session
- Follow constitutional principles in `project-constitution.md`

## Commit Message Format

```bash
# Use HEREDOC for proper formatting
git commit -m "$(cat <<'EOF'
feat(component): description [pe-xxx]

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

## Before Ending Session Checklist

- [ ] All changes committed with bead reference
- [ ] `bd sync` run to sync beads
- [ ] `git push` successful
- [ ] `git status` shows "up to date with origin"
- [ ] Beads created for any remaining work
- [ ] **Context docs still accurate?** Did implementation change anything documented in `infrastructure.md`, `ARCHITECTURE.md`, or other context files? Update them or create a bead to fix.
