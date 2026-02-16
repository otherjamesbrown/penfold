# Nomad Deployment

Penfold services are orchestrated by [Nomad](https://www.nomadproject.io/) using the `raw_exec` driver (no containers).

> **Important:** Nomad is the **sole orchestrator** for Penfold services. The legacy systemd units
> (`penfold-gateway.service`, `penfold-ai-coordinator.service`) are stopped and masked on dev02.
> Do not re-enable them — running both Nomad and systemd causes port conflicts and restart loops.

## Cluster Topology

| Node | Role | Meta Tags | Services |
|------|------|-----------|----------|
| dev02.brown.chat | Server + Client | `os=linux` | Gateway, AI Coordinator |
| dev01.brown.chat | Client | `apple-silicon=true` | Worker, MLX Embeddings, MLX LLM, MLX Exporter |

**Nomad server:** `http://dev02.brown.chat:4646`

Node meta tags control placement constraints — each job spec declares which node type it requires.

## Job Specs

| Job | File | Constraint | Binary | Env File |
|-----|------|------------|--------|----------|
| `penfold-gateway` | `gateway.nomad.hcl` | `meta.os = linux` | `/opt/penfold/bin/penfold-gateway` | `/etc/penfold/gateway.env` |
| `penfold-worker` | `worker.nomad.hcl` | `meta.apple-silicon = true` | `/opt/penfold/bin/penfold-worker` | `/etc/penfold/worker.env` |
| `penfold-ai-coordinator` | `ai-coordinator.nomad.hcl` | `meta.os = linux` | `/opt/penfold/bin/penfold-ai-coordinator` | `/etc/penfold/ai-coordinator.env` |
| `penfold-mlx` | `mlx-services.nomad.hcl` | `meta.apple-silicon = true` | Python (uvicorn, mlx_lm) | Inline env |
| `penfold-alert-webhook` | (on dev02 only) | `meta.os = linux` | `/opt/penfold/bin/alert-webhook.py` | - |

All Go services use the same pattern: source env file, then exec binary via `raw_exec`.
All services on dev02 run as `user = "james"` (not root).

## Update Strategy

All jobs use canary deployments with automatic rollback:

```hcl
update {
  canary           = 1       # Deploy 1 new instance
  max_parallel     = 1       # One at a time
  min_healthy_time = "10s"   # Must stay healthy for 10s
  healthy_deadline = "60s"   # Must become healthy within 60s
  auto_revert      = true    # Revert on failure
  auto_promote     = true    # Promote canary automatically
}
```

If the new allocation fails its health check within 60s, Nomad automatically reverts to the previous version.

## Common Operations

### Deploy a Service

Use the deployment scripts (recommended):

```bash
./scripts/deploy-gateway.sh          # Build + upload + nomad job run
./scripts/deploy-worker.sh           # Build + upload + nomad job run
./scripts/deploy-ai-coordinator.sh   # Build + upload + nomad job run
```

Each script: cross-compiles the binary, SCPs it to the target host, then runs `nomad job run`.

### Manual Nomad Commands

```bash
export NOMAD_ADDR=http://dev02.brown.chat:4646

# Submit/update a job
nomad job run deploy/nomad/gateway.nomad.hcl

# Check job status
nomad job status penfold-gateway

# View allocations (running instances)
nomad job status penfold-gateway | grep Allocations -A 10

# View logs for latest allocation
nomad alloc logs -job penfold-gateway
nomad alloc logs -job penfold-gateway -stderr

# Restart a job (stop + start)
nomad job restart penfold-gateway

# Stop a job
nomad job stop penfold-gateway

# Revert to previous version
nomad job revert penfold-gateway <version>

# View deployment history
nomad job history penfold-gateway
```

### Check Cluster Health

```bash
export NOMAD_ADDR=http://dev02.brown.chat:4646

# Server members
nomad server members

# Node status (all clients)
nomad node status

# All running jobs
nomad job status
```

### View Logs

```bash
# Nomad allocation logs (stdout)
nomad alloc logs -job penfold-gateway

# Nomad allocation logs (stderr)
nomad alloc logs -job penfold-gateway -stderr

# Follow logs
nomad alloc logs -job penfold-gateway -f

# Specific allocation (get ID from `nomad job status`)
nomad alloc logs <alloc-id>
```

## Environment Files

Each Go service reads its environment from `/etc/penfold/<service>.env` on the target host:

| File | Host | Service |
|------|------|---------|
| `/etc/penfold/gateway.env` | dev02 | Gateway |
| `/etc/penfold/worker.env` | dev01 | Worker |
| `/etc/penfold/ai-coordinator.env` | dev02 | AI Coordinator |

These files contain database credentials, service URLs, and configuration. See `~/github/otherjamesbrown/secrets/.env.penfold` for actual values.

## Health Checks

All Go services expose HTTP `/health` endpoints. Nomad polls these every 10s:

| Job | Health URL | Timeout |
|-----|-----------|---------|
| `penfold-gateway` | `:8080/health` | 3s |
| `penfold-worker` | `:8085/health` | 3s |
| `penfold-ai-coordinator` | `:8090/health` | 3s |
| `penfold-mlx` (embeddings) | `:8081/health` | 5s |
| `penfold-mlx` (llm) | `:8080/health` | 5s |
| `penfold-mlx` (exporter) | `:9101/metrics` | 5s |

If a health check fails, Nomad restarts the allocation (up to 3 attempts within 5 minutes).

All services also have `check_restart` configured — if a health check fails 3 consecutive times,
Nomad automatically restarts the task. Both HTTP and gRPC (TCP) ports are checked.

## Troubleshooting

### Job Won't Start

```bash
# Check allocation status for error details
nomad job status penfold-gateway
nomad alloc status <alloc-id>

# Common causes:
# - Binary not found at /opt/penfold/bin/...  (deploy script didn't upload)
# - Env file missing at /etc/penfold/...      (host not configured)
# - No node matches constraints               (meta tags not set)
```

### Health Check Failing

```bash
# Check service logs
nomad alloc logs -job penfold-gateway -stderr

# Test health endpoint directly
curl -s http://dev02.brown.chat:8080/health | jq .
ssh dev01 "curl -s http://localhost:8085/health"
```

### Allocation Keeps Restarting

```bash
# Check restart count and events
nomad alloc status <alloc-id>

# If restart limit hit (3 in 5min), job enters "failed" state
# Fix the issue, then re-submit:
nomad job run deploy/nomad/gateway.nomad.hcl
```

### Rollback

```bash
# Automatic: Nomad auto-reverts on failed canary (built-in)

# Manual: revert to a specific version
nomad job history penfold-gateway          # Find version number
nomad job revert penfold-gateway <version> # Revert
```

## Nomad Server Configuration

The Nomad server on dev02 requires these settings in `/etc/nomad.d/nomad.hcl`:

```hcl
# Telemetry — exposes Prometheus metrics at :4646/v1/metrics?format=prometheus
telemetry {
  publish_allocation_metrics = true
  publish_node_metrics       = true
  prometheus_metrics         = true
}
```

The systemd unit needs cgroup delegation for `user` field support in `raw_exec`:

```bash
# /etc/systemd/system/nomad.service.d/override.conf
[Service]
Delegate=yes
```

The data directory must be traversable (711) for non-root task execution:

```bash
chmod 711 /opt/nomad/data/
```

## Node Meta Setup

Nomad agents must have meta tags configured for placement constraints to work:

**dev02** (`/etc/nomad.d/client.hcl`):
```hcl
client {
  meta {
    "os" = "linux"
  }
}
```

**dev01** (`/etc/nomad.d/client.hcl` or equivalent):
```hcl
client {
  meta {
    "apple-silicon" = "true"
  }
}
```
