# Release Workflows

## CLI Release

**After making changes to CLI code (`cmd/penf/`) or documentation, ASK the user if a new release should be created.**

### When to Suggest a Release

- Bug fixes in CLI commands
- New CLI features or commands
- Help text or documentation updates
- Process definition changes (`~/.penf/processes.md`)
- Any user-facing changes

### How to Create a Release

#### 1. Bump the version

```bash
# Check current version
cat cmd/penf/VERSION

# Update to new version (e.g., v0.1.6)
echo "v0.1.6" > cmd/penf/VERSION
```

#### 2. Commit and push

```bash
git add cmd/penf/VERSION
git commit -m "chore(release): bump version to v0.1.6 [pe-xxx]"
git push
```

#### 3. GitHub Actions handles the rest

- `auto-release.yml` detects VERSION change on main
- Creates git tag automatically
- `release.yml` builds binaries for all platforms
- Creates GitHub release with assets

#### 4. Users update with

```bash
penf update
```

### Version Numbering

| Type | When | Example |
|------|------|---------|
| **Patch** (v0.1.x) | Bug fixes, minor improvements | v0.1.5 → v0.1.6 |
| **Minor** (v0.x.0) | New features, significant changes | v0.1.6 → v0.2.0 |
| **Major** (vx.0.0) | Breaking changes | v0.2.0 → v1.0.0 |

---

## Gateway Deployment

**When making changes to gateway code (`services/gateway/`, `pkg/`, `proto/`), you must deploy to dev02.**

### MANDATORY: Deployment Verification

**A deployment is NOT complete until verification passes.** Never mark a deployment task as done without running verification.

```bash
# Full deploy with automatic verification
./scripts/deploy-gateway.sh

# Or run verification separately
./scripts/verify-deployment.sh
```

**Exit codes from verify-deployment.sh:**
- `0` = All checks passed → Deployment complete
- `1` = Critical failure → MUST rollback
- `2` = Warnings only → May proceed with caution

See `context/development/workflows/deployment-checklist.md` for the full checklist.

### Quick Deploy (Recommended)

```bash
./scripts/deploy-gateway.sh
```

This script handles everything:
1. Cross-compiles for Linux amd64
2. Stops the running gateway
3. Copies binary to dev02
4. Starts with correct environment
5. **Runs verification (smoke tests)**
6. **Auto-rollback on failure**

### Script Options

```bash
./scripts/deploy-gateway.sh           # Full deploy (build + deploy + restart)
./scripts/deploy-gateway.sh --build   # Build only
./scripts/deploy-gateway.sh --deploy  # Deploy only (use existing binary)
./scripts/deploy-gateway.sh --status  # Check gateway status and logs
```

### Manual Steps (if needed)

#### 1. Cross-compile for Linux (from Mac)

```bash
cd services/gateway
GOOS=linux GOARCH=amd64 go build -o gateway-linux .
```

**Important:** dev02 is Intel Linux (amd64), not ARM. A Mac-compiled binary won't work.

#### 2. Deploy to dev02

```bash
scp gateway-linux dev02:/home/james/penfold-gateway
```

#### 3. Restart gateway with correct environment

```bash
ssh dev02 "pkill penfold-gateway; sleep 2; \
  PENFOLD_SERVICE_NAME=gateway \
  PENFOLD_DB_HOST=localhost \
  PENFOLD_DB_PASSWORD=<see secrets/.env.penfold> \
  nohup /home/james/penfold-gateway > /tmp/gateway.log 2>&1 &"
```

#### 4. Verify gateway started

```bash
# Check process is running
ssh dev02 "ps aux | grep gateway | grep -v grep"

# Check health endpoint
curl -s http://dev02.brown.chat:8080/health | jq .status

# Check logs if issues
ssh dev02 "tail -50 /tmp/gateway.log"
```

### Common Gateway Deployment Issues

| Problem | Cause | Fix |
|---------|-------|-----|
| "Syntax error" on startup | Binary compiled for wrong platform | Cross-compile with `GOOS=linux GOARCH=amd64` |
| "password authentication failed" | Wrong DB password | Check `secrets/.env.penfold` for correct password |
| Connection refused on :5432 | DB host wrong | Use `PENFOLD_DB_HOST=localhost` (co-located) |
| Gateway dies with SSH session | nohup not used properly | Use full nohup syntax |
| Service not found (gRPC) | Gateway not redeployed after proto changes | Run `./scripts/deploy-gateway.sh` |

---

## Proto Changes

When modifying `.proto` files:

```bash
cd api/proto
buf generate --path tenant/v1/tenant.proto  # Single proto
buf generate                                 # All protos
```

Then rebuild affected services (gateway, CLI, etc.)
