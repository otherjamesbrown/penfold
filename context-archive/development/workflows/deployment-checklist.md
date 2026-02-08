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

### 5. Build & Deploy Services

Use the deploy scripts — they handle cross-compilation, ldflags (embedding git version/commit/buildTime), upload, and Nomad job submission:

### 6. Deploy Services

All services are managed by Nomad. Use the deployment scripts which handle build, upload, and `nomad job run`:

**Gateway (dev02):**
```bash
./scripts/deploy-gateway.sh          # Build + upload + nomad job run
./scripts/deploy-gateway.sh --build  # Build only
./scripts/deploy-gateway.sh --status # Check Nomad job status
```

**Worker (dev01 - Apple Silicon):**
```bash
./scripts/deploy-worker.sh           # Build + upload + nomad job run
./scripts/deploy-worker.sh --build   # Build only
./scripts/deploy-worker.sh --status  # Check Nomad job status
```

**AI Coordinator (dev02):**
```bash
./scripts/deploy-ai-coordinator.sh           # Build + upload + nomad job run
./scripts/deploy-ai-coordinator.sh --build   # Build only
./scripts/deploy-ai-coordinator.sh --status  # Check Nomad job status
```

**MLX Services (dev01):**
```bash
# MLX services are managed directly via Nomad (no build step)
export NOMAD_ADDR=http://dev02.brown.chat:4646
nomad job run deploy/nomad/mlx-services.nomad.hcl
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
export NOMAD_ADDR=http://dev02.brown.chat:4646

# Gateway logs (via Nomad)
nomad alloc logs -job penfold-gateway -stderr | tail -50

# Worker logs (via Nomad)
nomad alloc logs -job penfold-worker -stderr | tail -50

# AI Coordinator logs
nomad alloc logs -job penfold-ai-coordinator -stderr | tail -50

# Should show no critical errors
```

### 10. Notify Stakeholders
- [ ] Update deployment log/ticket
- [ ] Notify team in Slack/chat
- [ ] Update infrastructure.md if topology changed

---

## Rollback Procedure

If ANY smoke test fails:

### 1. Automatic Rollback (Nomad)

Nomad jobs are configured with `auto_revert = true`. If the canary allocation fails its health check, Nomad automatically reverts to the previous version. Check if this already happened:

```bash
export NOMAD_ADDR=http://dev02.brown.chat:4646
nomad job status penfold-gateway   # Check "Latest Deployment" section
```

### 2. Manual Rollback

If automatic revert didn't trigger or you need to revert further:

```bash
export NOMAD_ADDR=http://dev02.brown.chat:4646

# View deployment history
nomad job history penfold-gateway

# Revert to a specific version
nomad job revert penfold-gateway <version>

# Or stop the job entirely
nomad job stop penfold-gateway
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
- Create shard for the failure: `SELECT create_shard('penfold', 'Deploy failure: ...', 'Details', 'task', 'agent-mycroft');`
- Include logs and error messages
- Tag as P0 if service is down

---

## Deployment Scripts

Each service has a dedicated deploy script that handles build, SCP, and Nomad job submission:

```bash
# Gateway (builds Linux amd64, deploys to dev02)
./scripts/deploy-gateway.sh           # Full: build + upload + nomad job run
./scripts/deploy-gateway.sh --build   # Build only
./scripts/deploy-gateway.sh --status  # Check Nomad job status + health

# Worker (builds Darwin arm64, deploys to dev01)
./scripts/deploy-worker.sh            # Full: build + upload + nomad job run
./scripts/deploy-worker.sh --build    # Build only
./scripts/deploy-worker.sh --status   # Check Nomad job status + health

# AI Coordinator (builds Linux amd64, deploys to dev02)
./scripts/deploy-ai-coordinator.sh           # Full: build + upload + nomad job run
./scripts/deploy-ai-coordinator.sh --build   # Build only
./scripts/deploy-ai-coordinator.sh --status  # Check Nomad job status + health
```

All scripts use canary deployments via Nomad. If the new version fails its health check, Nomad auto-reverts.

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

### AI Coordinator
- Requires: MLX services (dev01)
- Host: dev02.brown.chat (Nomad: `penfold-ai-coordinator`)
- Port: 50055 (gRPC), 8090 (HTTP)
- Binary: `/opt/penfold/bin/penfold-ai-coordinator`

---

## Service Management Reference (Nomad)

All Penfold services are managed by Nomad. The Nomad server runs on dev02.

```bash
export NOMAD_ADDR=http://dev02.brown.chat:4646

# --- Status ---
nomad job status                            # All jobs
nomad job status penfold-gateway            # Specific job
nomad node status                           # All nodes
nomad server members                        # Cluster membership

# --- Start/Stop ---
nomad job run deploy/nomad/gateway.nomad.hcl    # Start/update
nomad job stop penfold-gateway                   # Stop
nomad job restart penfold-gateway                # Restart (stop + start)

# --- Logs ---
nomad alloc logs -job penfold-gateway            # stdout
nomad alloc logs -job penfold-gateway -stderr    # stderr
nomad alloc logs -job penfold-gateway -f         # Follow

# --- Rollback ---
nomad job history penfold-gateway                # View versions
nomad job revert penfold-gateway <version>       # Revert

# --- Debugging ---
nomad alloc status <alloc-id>                    # Allocation details + events
```

### Observability Stack (dev02)

```bash
# Manage
cd ~/penfold/deploy/observability
docker compose up -d
docker compose down
docker compose restart prometheus

# Logs
docker compose logs -f grafana

# Access
# Grafana: http://dev02.brown.chat:3001 (admin/penfold2024)
# Prometheus: http://dev02.brown.chat:9090
# Alertmanager: http://dev02.brown.chat:9094
```

---

## See Also

- [deploy/nomad/README.md](../../deploy/nomad/README.md) - Nomad job specs, operations, troubleshooting
- [deploy/observability/README.md](../../deploy/observability/README.md) - Monitoring stack
