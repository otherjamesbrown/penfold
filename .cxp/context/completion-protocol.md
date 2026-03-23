# Completion Protocol

## Communication Model

Do NOT send messages. Instead:
- **Claim shards** to show you're working on them (status → in_progress)
- **Update shard content** with findings, progress, review details
- **Set status to `needs-review` when done**
- **Label shards** `blocked` when stuck (`cxp shard label add <id> blocked`)

## When you finish work on a shard

**CRITICAL: You MUST NOT run `cxp shard close`. You do not have authority to close shards. Only penfold closes shards after independent verification.**

1. **Write evidence to the shard body** (`cxp shard update <id> --body-file ...`):
   - Commit hash
   - Test output (actual stdout, not just "tests pass")
   - Files modified
   - Deploy verification (running version from `penf health`)
   - For pipeline changes: before/after output or grpcurl acceptance test results

2. **Set status to `needs-review`**:
   ```bash
   cxp shard status <id> needs-review
   ```

3. **Stop and move to the next shard.** Do not close. Do not set any other status.

**Why:** Shards without evidence get sent back. Shards closed without review bypass the quality gate.
