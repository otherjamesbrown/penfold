# Penfold Environment Configuration

Template environment files for Penfold services. These files are copied to `/etc/penfold/` during installation.

## Files

| File | Service | Target Host |
|------|---------|-------------|
| `common.env` | Shared settings | Reference only |
| `gateway.env` | penfold-gateway | dev02 |
| `ai-coordinator.env` | penfold-ai-coordinator | dev02 |
| `worker.env` | penfold-worker | dev01 (reference only) |

**Note:** The worker on macOS uses environment variables embedded in the launchd plist, not a separate env file. The `worker.env` file is for reference only.

## Setup Scripts

| Script | Purpose |
|--------|---------|
| `setup-dev01.sh` | Prepare dev01 environment |
| `setup-dev02.sh` | Prepare dev02 environment |

## Common Variables

Defined in `common.env` (shared across services):

```bash
# Environment
PENFOLD_ENVIRONMENT=dev

# Langfuse Tracing
LANGFUSE_HOST=http://dev02.brown.chat:3000
LANGFUSE_PUBLIC_KEY=pk-lf-penfold
LANGFUSE_SECRET_KEY=sk-lf-penfold-secret

# Temporal
TEMPORAL_HOST_PORT=dev02.brown.chat:7233
TEMPORAL_NAMESPACE=default

# Database
PENFOLD_DB_HOST=dev02.brown.chat
PENFOLD_DB_PORT=5432
PENFOLD_DB_NAME=penfold
PENFOLD_DB_USER=penfold
PENFOLD_DB_SSL_MODE=verify-full
```

## Gateway Variables

Key variables in `gateway.env`:

```bash
# Service identity
PENFOLD_SERVICE_NAME=gateway

# Server ports
PENFOLD_GRPC_PORT=50051
PENFOLD_HTTP_PORT=8080

# Database
PENFOLD_DB_HOST=localhost          # Local on dev02
PENFOLD_DB_PASSWORD=<secret>

# TLS (mTLS for gRPC)
PENFOLD_TLS_CERT=/home/james/.penfold/certs/server.crt
PENFOLD_TLS_KEY=/home/james/.penfold/certs/server.key
PENFOLD_TLS_CA=/home/james/.penfold/certs/ca.crt

# Connected services
WORKER_SERVICE_ADDR=dev01.brown.chat:8085
AI_COORDINATOR_ADDR=localhost:50055
```

## AI Coordinator Variables

Key variables in `ai-coordinator.env`:

```bash
# Service identity
PENFOLD_SERVICE_NAME=ai-coordinator

# Server ports
PENFOLD_GRPC_PORT=50055
PENFOLD_HTTP_PORT=8090

# AI backend (MLX on dev01)
MLX_LLM_URL=http://dev01.brown.chat:8080
MLX_EMBEDDINGS_URL=http://dev01.brown.chat:8081

# Anthropic fallback
ANTHROPIC_API_KEY=<secret>
```

## Worker Variables

Key variables (defined in launchd plist on dev01):

```bash
# Service identity
WORKER_SERVICE_NAME=penfold-worker
WORKER_ENVIRONMENT=dev
WORKER_HTTP_PORT=8085

# Temporal connection
TEMPORAL_HOST_PORT=dev02.brown.chat:7233
TEMPORAL_NAMESPACE=default

# Database (full URL with SSL)
DATABASE_URL=postgres://penfold@dev02.brown.chat:5432/penfold?sslmode=verify-full&...

# AI services
AI_SERVICE_ADDR=dev02.brown.chat:50055
AI_SERVICE_URL=http://dev01.brown.chat:8081

# Tracing
LANGFUSE_HOST=http://dev02.brown.chat:3000
LANGFUSE_PUBLIC_KEY=pk-lf-penfold
LANGFUSE_SECRET_KEY=sk-lf-penfold-secret
```

## Security Notes

1. **File permissions:** Env files on target hosts should be mode `600`:
   ```bash
   sudo chmod 600 /etc/penfold/*.env
   ```

2. **Sensitive values:** The template files contain placeholder values. Update passwords and API keys on the target hosts.

3. **Do not commit secrets:** The actual env files on target hosts should never be committed to git.

## Updating Configuration

**systemd services (dev02):**
```bash
# Edit env file
sudo vim /etc/penfold/gateway.env

# Restart service
sudo systemctl restart penfold-gateway
```

**launchd service (dev01):**
```bash
# Edit plist directly (env vars are embedded)
sudo vim /Library/LaunchDaemons/com.penfold.worker.plist

# Reload service
sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist
sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist
```
