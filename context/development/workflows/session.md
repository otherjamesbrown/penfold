# Session Protocol

## Session Close

**Work is NOT complete until pushed to remote.**

```bash
git status              # Check changes
git add <files>         # Stage changes
git commit -m "..."     # Commit with shard reference [pf-xxx]
git push                # MUST PUSH TO REMOTE
```

**(No sync needed - shards are always live in Context-Palace)**

**Rules:**
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
- Create shards for any remaining work before ending

## Git Workflow

- All commits must reference shard: `[pf-xxx]`
- Push to remote before ending session
- Follow constitutional principles in `project-constitution.md`

## Commit Message Format

```bash
# Use HEREDOC for proper formatting
git commit -m "$(cat <<'EOF'
feat(component): description [pf-xxx]

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

## Before Ending Session Checklist

- [ ] All changes committed with shard reference
- [ ] `git push` successful
- [ ] `git status` shows "up to date with origin"
- [ ] Shards created for any remaining work
- [ ] **Context docs still accurate?** Did implementation change anything documented in `infrastructure.md`, `ARCHITECTURE.md`, or other context files? Update them or create a shard to fix.
