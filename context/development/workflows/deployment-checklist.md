# Deployment Checklist

> **Purpose:** Ensure every deployment is complete and verified before marking done.
> **Created:** 2026-01-26 after production issues from incomplete deployments.

---

## Pre-Deployment

### 1. Code Review Complete
- [ ] PR approved by at least one reviewer
- [ ] All CI checks passing (unit, integration, e2e)
- [ ] No unresolved comments

### 2. Database Migrations
- [ ] Run `penf migrate status` to check pending migrations
- [ ] Review migration SQL for correctness
- [ ] Verify migration has rollback (down migration)
- [ ] Test migration on staging/test database first

```bash
# Check migration status
source ~/github/otherjamesbrown/secrets/.env.penfold
penf migrate status

# If migrations pending:
penf migrate up
```

### 3. Service Dependencies
- [ ] List all services affected by this change
- [ ] Verify dependent services are compatible
- [ ] Check for breaking API changes

---

## Deployment Steps

### 4. Apply Database Migrations
```bash
# On dev02 or target environment
penf migrate up

# Verify
penf migrate status
# Should show: "All migrations applied"
```

### 5. Build Services
```bash
# Build affected services
cd services/gateway && go build -o gateway-linux -ldflags="-s -w" .
cd services/worker && go build -o worker -ldflags="-s -w" .
# etc.
```

### 6. Deploy Services
```bash
# Copy binary to target host
scp services/gateway/gateway-linux james@dev02.brown.chat:/tmp/penfold-gateway

# On dev02: Stop old, start new
ssh james@dev02.brown.chat << 'EOF'
pkill -f penfold-gateway || true
sleep 2
cd /tmp && PENFOLD_SERVICE_NAME=gateway \
  PENFOLD_DB_HOST=localhost \
  PENFOLD_DB_PORT=5432 \
  PENFOLD_DB_USER=penfold \
  PENFOLD_DB_PASSWORD=penfold2024 \
  PENFOLD_DB_NAME=penfold \
  nohup ./penfold-gateway > gateway.log 2>&1 &
EOF
```

---

## Post-Deployment Verification

### 7. Automated Verification (RECOMMENDED)

Use the automated verification script for comprehensive checks:

```bash
# Run full verification suite
./scripts/verify-deployment.sh

# Quick health check only
./scripts/verify-deployment.sh --quick

# Gateway-specific tests
./scripts/verify-deployment.sh --gateway

# Output as JSON for CI/CD
./scripts/verify-deployment.sh --output json
```

The script checks:
- Gateway /health, /ready, /live endpoints
- Database connectivity
- All gRPC services via CLI
- Worker health and ML services
- Log analysis for errors

Exit codes:
- 0: All checks passed
- 1: Critical failure (rollback recommended)
- 2: Non-critical warnings (may proceed)

### 8. Manual Smoke Tests (Alternative)

If automated verification is not available, run these manually:

```bash
# Test 1: Gateway reachable
penf status
# Expected: Shows "Gateway: Connected"

# Test 2: Service health
curl -s http://dev02.brown.chat:8080/health | jq .
# Expected: status: "healthy", database: "healthy"

# Test 3: Database operations work
penf project list
# Expected: Returns list (may be empty)

# Test 4: Search works (if search service deployed)
penf search "test" --limit 1 --timeout 10s
# Expected: Returns results or empty (no timeout/error)

# Test 5: Ingest works (dry run)
penf ingest email test_data/sample.eml --dry-run
# Expected: Shows what would be ingested (no error)

# Test 6: Glossary works
penf glossary list
# Expected: Returns glossary terms
```

### 9. Log Check
```bash
# Check for errors in logs
ssh dev02.brown.chat "tail -100 /tmp/gateway.log | grep -i error"

# Should show no critical errors
```

### 10. Notify Stakeholders
- [ ] Update deployment log/ticket
- [ ] Notify team in Slack/chat
- [ ] Update infrastructure.md if topology changed

---

## Rollback Procedure

If ANY smoke test fails:

### 1. Stop New Service
```bash
ssh dev02.brown.chat "pkill -f penfold-gateway"
```

### 2. Restore Previous Binary
```bash
ssh dev02.brown.chat << 'EOF'
mv /tmp/penfold-gateway.backup /tmp/penfold-gateway
# Restart with same command as deployment
EOF
```

### 3. Rollback Migration (if applied)
```bash
penf migrate down
# Or specific version:
penf migrate down-to 20
```

### 4. Verify Rollback
```bash
penf status
penf health gateway
```

### 5. Document Issue
- Create bead for the failure
- Include logs and error messages
- Tag as P0 if service is down

---

## Deployment Script

For convenience, use the deployment script:

```bash
# Full deployment with verification
./scripts/deploy-gateway.sh

# The script will:
# 1. Build the binary
# 2. Copy to dev02
# 3. Run migrations
# 4. Restart service
# 5. Run smoke tests
# 6. Rollback on failure

# Other options:
./scripts/deploy-gateway.sh --build    # Build only
./scripts/deploy-gateway.sh --deploy   # Deploy only
./scripts/deploy-gateway.sh --status   # Check status
./scripts/deploy-gateway.sh --verify   # Run verification only
```

## CI/CD Verification

A GitHub Actions workflow is available for post-deployment verification:

```bash
# Trigger manually via GitHub UI or CLI
gh workflow run deploy-verify.yml \
  -f environment=dev \
  -f quick_check=false

# Check results
gh run list --workflow=deploy-verify.yml
```

The workflow can also be called from other workflows for automated deployment pipelines.

---

## Checklist Summary

Copy this for each deployment:

```
## Deployment: [SERVICE] [VERSION] - [DATE]

### Pre-Deployment
- [ ] PR approved and merged
- [ ] CI passing
- [ ] Migration reviewed (if any)

### Deployment
- [ ] Migrations applied
- [ ] Service built
- [ ] Service deployed
- [ ] Old service stopped, new started

### Verification
- [ ] `penf status` - Connected
- [ ] `penf health gateway` - All healthy
- [ ] `penf project list` - Works
- [ ] `penf search "test"` - Works (or expected error)
- [ ] `penf glossary list` - Works
- [ ] Logs checked - No critical errors

### Sign-off
- [ ] Deployment verified by: _______________
- [ ] Time completed: _______________
```

---

## Service-Specific Notes

### Gateway
- Requires: PostgreSQL
- Port: 50051 (gRPC), 8080 (HTTP)
- Binary: `penfold-gateway`

### Worker
- Requires: PostgreSQL, Temporal, MLX services
- Port: 8085 (health only)
- Binary: `penfold-worker`
- Note: Must run on dev01 (Apple Silicon for MLX)

### Search Service
- Requires: PostgreSQL with pgvector
- Port: 50053 (gRPC), 8082 (HTTP)
- Note: NOT CURRENTLY DEPLOYED - implement or remove from gateway routing
