# Penfold Production Deployment Guide

**Last Updated**: 2026-01-23
**Target Environment**: Mac Mini M4 (dev01) + Intel NUC (dev02)
**Deployment Model**: Distributed Go services with Temporal orchestration

This guide provides step-by-step instructions for deploying Penfold to a production environment. It covers the distributed architecture, service deployment, security hardening, monitoring, and operational procedures.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Architecture Overview](#architecture-overview)
- [Environment Configuration](#environment-configuration)
- [Service Deployment](#service-deployment)
- [SSL/TLS Configuration](#ssltls-configuration)
- [Network Security](#network-security)
- [Health Checks and Monitoring](#health-checks-and-monitoring)
- [Deployment Runbook](#deployment-runbook)
- [Rollback Procedures](#rollback-procedures)
- [Environment Differences](#environment-differences)
- [Backup and Recovery](#backup-and-recovery)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

### Hardware Requirements

| Component | dev01 (Mac Mini M4) | dev02 (Intel NUC) |
|-----------|---------------------|---------------------|
| CPU | Apple M4 | Intel Core i5+ |
| RAM | 32GB | 16GB+ |
| Storage | 512GB+ SSD | 256GB+ SSD |
| Network | 1Gbps | 1Gbps |
| Role | Worker, MLX inference | Gateway, Data services |

### Software Requirements

```bash
# dev01 (Mac Mini M4)
sw_vers                    # macOS Sonoma or later
go version                 # Go 1.22+
python3 --version          # Python 3.11+ (for MLX sidecar)

# dev02 (Intel NUC)
docker --version           # Docker 24+
docker compose version     # Compose v2
```

### Required Services (dev02)

```bash
# Docker containers
docker ps --filter 'name=penfold'
# Expected: penfold-postgres, penfold-redis, penfold-temporal, penfold-temporal-ui
```

### Required Accounts and Credentials

- Google Cloud project with enabled APIs (Gmail, Calendar)
- OAuth2 credentials for Gmail integration
- Credentials stored in `~/github/otherjamesbrown/secrets/.env.penfold`

---

## Architecture Overview

```
                                    +-----------------+
                                    |   penf CLI      |
                                    |   (any host)    |
                                    +--------+--------+
                                             |
                                    gRPC :50051
                                             |
+-----------------------------------------------------------------------------------------------------------------+
|                                   dev02.brown.chat (Intel NUC)                                                 |
|                                                                                                                  |
|  +-----------------------+     +---------------------+     +------------------+     +------------------+         |
|  |  Penfold Gateway      |     |  PostgreSQL         |     |  Redis           |     |  Temporal        |         |
|  |  Go binary            |     |  + pgvector         |     |  (cache)         |     |  Server          |         |
|  |  gRPC: :50051         |<--->|  :5432              |     |  :6379           |     |  :7233           |         |
|  |  HTTP: :8080          |     |  penfold-postgres   |     |  penfold-redis   |     |  UI: :8088       |         |
|  +-----------+-----------+     +---------------------+     +------------------+     +--------+---------+         |
|              |                                                                               |                   |
+--------------+-------------------------------------------------------------------------------+-------------------+
               |                                                                               |
               |                                      Network (1 Gbps)                         |
               |                                                                               |
+--------------+-------------------------------------------------------------------------------+-------------------+
|                                   dev01.brown.chat (Mac Mini M4)                                                 |
|                                                                                                                  |
|  +-----------------------+     +----------------------------------------------+                                 |
|  |  Ollama               |     |  Penfold Worker                              |                                 |
|  |  mxbai-embed-large    |     |  Go binary                                   |                                 |
|  |  :11434               |<----|  Temporal activities & workflows             |                                 |
|  |                       |     |  Health: :8085                               |                                 |
|  +-----------------------+     +----------------------------------------------+                                 |
|                                                                                                                  |
+-----------------------------------------------------------------------------------------------------------------+
```

### Service Components

| Service | Host | Type | Port | Purpose |
|---------|------|------|------|---------|
| Gateway | dev02 | Go binary | 50051 (gRPC), 8080 (HTTP) | API gateway, gRPC services |
| PostgreSQL | dev02 | Docker | 5432 | Primary database with pgvector |
| Redis | dev02 | Docker | 6379 | Caching |
| Temporal | dev02 | Docker | 7233, 8088 (UI) | Workflow orchestration |
| Worker | dev01 | Go binary | 8085 (health) | Temporal workflow execution |
| Ollama | dev01 | launchd service | 11434 | Embeddings (mxbai-embed-large) |

### Go Service Modules

| Service | Build Location | Binary |
|---------|----------------|--------|
| penf CLI | `cmd/penf/` | `penf` |
| Gateway | `services/gateway/` | `penfold-gateway` |
| Worker | `services/worker/` | `penfold-worker` |
| Gmail | `services/gmail/` | `penfold-gmail` |
| Search | `services/search/` | `penfold-search` |

---

## Environment Configuration

### Directory Structure

```bash
# dev01 - Development machine
~/github/otherjamesbrown/penfold/     # Source code
~/github/otherjamesbrown/secrets/     # Credentials
~/.penf/                              # CLI configuration

# dev02 - Data services
/opt/penfold/                         # Deployment directory
/opt/penfold/data/postgres/           # PostgreSQL data
/opt/penfold/data/redis/              # Redis data
/opt/penfold/logs/                    # Service logs
/opt/penfold/backups/                 # Database backups
```

### Environment Variables

#### dev01 - Worker Environment

Source credentials from the secrets file:

```bash
# Load environment
source ~/github/otherjamesbrown/secrets/.env.penfold

# Worker configuration (set in ~/.zshrc or launchd plist)
export PENFOLD_SERVICE_NAME=worker
export PENFOLD_DB_HOST=dev02.brown.chat
export PENFOLD_DB_PORT=5432
export PENFOLD_DB_USER=penfold
export PENFOLD_DB_PASSWORD=<from secrets>
export PENFOLD_DB_NAME=penfold
export PENFOLD_TEMPORAL_HOST=dev02.brown.chat:7233
export OLLAMA_URL=http://localhost:11434      # Local Ollama embeddings
export EMBEDDING_MODEL=mxbai-embed-large
```

#### dev02 - Gateway Environment

```bash
export PENFOLD_SERVICE_NAME=gateway
export PENFOLD_DB_HOST=localhost              # Co-located with PostgreSQL
export PENFOLD_DB_PORT=5432
export PENFOLD_DB_USER=penfold
export PENFOLD_DB_PASSWORD=<from secrets>
export PENFOLD_DB_NAME=penfold
export PENFOLD_GRPC_PORT=50051
export PENFOLD_HTTP_PORT=8080

# ML service URLs for health aggregation
export GATEWAY_OLLAMA_URL=http://dev01.brown.chat:11434
export GATEWAY_WORKER_HEALTH_URL=http://dev01.brown.chat:8085
```

### CLI Configuration

Create `~/.penf/config.yaml`:

```yaml
server_address: dev02.brown.chat:50051
timeout: 30s
output_format: text
insecure: true    # Set to false with TLS
```

---

## Service Deployment

### Docker Compose for Data Services (dev02)

Create `/opt/penfold/docker-compose.yml`:

```yaml
version: '3.8'

services:
  postgres:
    image: pgvector/pgvector:pg16
    container_name: penfold-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: penfold
      POSTGRES_USER: penfold
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      PGDATA: /var/lib/postgresql/data/pgdata
    ports:
      - "5432:5432"
    volumes:
      - /opt/penfold/data/postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U penfold -d penfold"]
      interval: 10s
      timeout: 5s
      retries: 5
    deploy:
      resources:
        limits:
          memory: 4G

  redis:
    image: redis:7-alpine
    container_name: penfold-redis
    restart: unless-stopped
    command: redis-server --appendonly yes --maxmemory 512mb --maxmemory-policy allkeys-lru
    ports:
      - "6379:6379"
    volumes:
      - /opt/penfold/data/redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  temporal:
    image: temporalio/auto-setup:latest
    container_name: penfold-temporal
    restart: unless-stopped
    environment:
      - DB=postgresql
      - DB_PORT=5432
      - POSTGRES_USER=penfold
      - POSTGRES_PWD=${DB_PASSWORD}
      - POSTGRES_SEEDS=postgres
    ports:
      - "7233:7233"
    depends_on:
      postgres:
        condition: service_healthy

  temporal-ui:
    image: temporalio/ui:latest
    container_name: penfold-temporal-ui
    restart: unless-stopped
    environment:
      - TEMPORAL_ADDRESS=temporal:7233
    ports:
      - "8088:8080"
    depends_on:
      - temporal

networks:
  default:
    name: penfold-network
```

### Building Go Services

```bash
# On dev01
cd ~/github/otherjamesbrown/penfold

# Build all services
make build

# Or build individually
cd services/gateway && go build -o ../../bin/penfold-gateway .
cd services/worker && go build -o ../../bin/penfold-worker .
cd cmd/penf && go build -o ../../bin/penf .
```

### Deploying Gateway (dev02)

```bash
# Copy binary to dev02
scp bin/penfold-gateway dev02.brown.chat:/tmp/

# SSH and start gateway
ssh dev02.brown.chat

# Set environment and run
source /opt/penfold/.env
nohup /tmp/penfold-gateway > /opt/penfold/logs/gateway.log 2>&1 &
```

### Deploying Worker (dev01)

The worker runs on dev01 to leverage Apple Silicon for MLX inference.

#### Launchd Configuration

Create `~/Library/LaunchAgents/com.penfold.worker.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.penfold.worker</string>
    <key>ProgramArguments</key>
    <array>
        <string>/Users/james/github/otherjamesbrown/penfold/bin/penfold-worker</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PENFOLD_DB_HOST</key>
        <string>dev02.brown.chat</string>
        <key>PENFOLD_TEMPORAL_HOST</key>
        <string>dev02.brown.chat:7233</string>
        <key>AI_SERVICE_URL</key>
        <string>http://localhost:8081</string>
        <key>LLM_URL</key>
        <string>http://localhost:8080</string>
    </dict>
    <key>WorkingDirectory</key>
    <string>/Users/james/github/otherjamesbrown/penfold</string>
    <key>StandardOutPath</key>
    <string>/tmp/penfold-worker.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/penfold-worker.log</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

Load the service:

```bash
launchctl load ~/Library/LaunchAgents/com.penfold.worker.plist
```

### Ollama Service (dev01)

Ollama runs as a launchd service and provides embedding generation:

```bash
# Manual start (if needed)
ollama serve

# Test embeddings endpoint
curl http://localhost:11434/api/embeddings -d '{
  "model": "mxbai-embed-large",
  "prompt": "test embedding"
}'

# Check status
curl http://localhost:11434/
```

Managed by launchd: `~/Library/LaunchAgents/com.ollama.serve.plist`

---

## SSL/TLS Configuration

### gRPC with TLS (Optional)

For secure gRPC connections, generate certificates:

```bash
# Generate CA and server certificates
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout /opt/penfold/certs/server.key \
    -out /opt/penfold/certs/server.crt \
    -subj "/CN=dev02.brown.chat"
```

Update CLI config:

```yaml
# ~/.penf/config.yaml
server_address: dev02.brown.chat:50051
insecure: false
tls_cert_path: /path/to/server.crt
```

---

## Network Security

### Firewall Rules

Internal network only. For external exposure, use a reverse proxy:

```bash
# macOS pf rules (if needed)
# Block external access to service ports
block in quick on en0 proto tcp from any to any port {5432, 6379, 7233, 8080, 8081, 50051}

# Allow from local network
pass in quick on en0 proto tcp from 10.0.10.0/24 to any port {5432, 6379, 7233, 8080, 8081, 50051}
```

### Service Binding

Services bind to specific interfaces:

| Service | Host | Binding |
|---------|------|---------|
| PostgreSQL | dev02 | 0.0.0.0:5432 (internal network) |
| Redis | dev02 | 0.0.0.0:6379 (internal network) |
| Gateway | dev02 | 0.0.0.0:50051, 0.0.0.0:8080 |
| Worker | dev01 | 0.0.0.0:8085 (health only) |
| MLX Embeddings | dev01 | 0.0.0.0:8081 |
| MLX LLM | dev01 | 0.0.0.0:8080 |

---

## Health Checks and Monitoring

### Health Check Endpoints

| Service | Endpoint | Purpose |
|---------|----------|---------|
| Gateway | `http://dev02.brown.chat:8080/health` | Full health with backend services |
| Gateway | `http://dev02.brown.chat:8080/ready` | Readiness probe |
| Gateway | `http://dev02.brown.chat:8080/live` | Liveness probe |
| Gateway | `http://dev02.brown.chat:8080/metrics` | Prometheus metrics |
| Worker | `http://dev01.brown.chat:8085/health` | Worker health status |
| Worker | `http://dev01.brown.chat:8085/ready` | Readiness probe |
| Ollama | `http://dev01.brown.chat:11434/` | Ollama service (returns "Ollama is running") |

### CLI Health Commands

```bash
# Check all services via gateway
penf health gateway

# Check local ML services (from dev01)
penf health local

# Quick status
penf status
```

### Health Check Script

Create `/opt/penfold/scripts/health-check.sh`:

```bash
#!/bin/zsh
# Penfold Health Check Script

set -e

echo "=== Penfold Health Check ==="
echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo ""

# Check Gateway (dev02)
echo "Checking Gateway..."
GATEWAY_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://dev02.brown.chat:8080/health" || echo "000")
if [[ "$GATEWAY_STATUS" == "200" ]]; then
    echo "  Gateway: HEALTHY"
    curl -s "http://dev02.brown.chat:8080/health" | jq -r '.services | to_entries[] | "    \(.key): \(.value.status)"'
else
    echo "  Gateway: UNHEALTHY (HTTP ${GATEWAY_STATUS})"
fi
echo ""

# Check Worker (dev01)
echo "Checking Worker..."
WORKER_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://dev01.brown.chat:8085/health" || echo "000")
if [[ "$WORKER_STATUS" == "200" ]]; then
    echo "  Worker: HEALTHY"
else
    echo "  Worker: UNHEALTHY (HTTP ${WORKER_STATUS})"
fi
echo ""

# Check PostgreSQL
echo "Checking PostgreSQL..."
PG_STATUS=$(docker exec penfold-postgres pg_isready -U penfold -d penfold 2>/dev/null && echo "READY" || echo "NOT READY")
echo "  PostgreSQL: ${PG_STATUS}"
echo ""

# Check Redis
echo "Checking Redis..."
REDIS_PING=$(docker exec penfold-redis redis-cli ping 2>/dev/null || echo "FAILED")
if [[ "$REDIS_PING" == "PONG" ]]; then
    echo "  Redis: HEALTHY"
else
    echo "  Redis: UNHEALTHY (${REDIS_PING})"
fi
echo ""

# Check Temporal
echo "Checking Temporal..."
TEMPORAL_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://dev02.brown.chat:7233" || echo "000")
if [[ "$TEMPORAL_STATUS" != "000" ]]; then
    echo "  Temporal: REACHABLE"
else
    echo "  Temporal: UNREACHABLE"
fi
echo ""

echo "=== Health Check Complete ==="
```

### Monitoring with cron

```cron
# Health checks every 5 minutes
*/5 * * * * /opt/penfold/scripts/health-check.sh >> /opt/penfold/logs/health-check.log 2>&1

# Database backup daily at 2 AM
0 2 * * * /opt/penfold/scripts/backup-database.sh >> /opt/penfold/logs/backup.log 2>&1
```

---

## Deployment Runbook

### Pre-Deployment Checklist

```markdown
- [ ] Backup current database
- [ ] Document current binary versions
- [ ] Verify sufficient disk space (>20% free)
- [ ] Review changelog for breaking changes
- [ ] Prepare rollback commands
- [ ] Test new binaries locally
```

### Full Stack Startup

```bash
# 1. Verify Docker services on dev02
ssh dev02.brown.chat "docker ps --filter 'name=penfold'"

# 2. Start Gateway on dev02 (if not running)
ssh dev02.brown.chat "source /opt/penfold/.env && \
  nohup /tmp/penfold-gateway > /opt/penfold/logs/gateway.log 2>&1 &"

# 3. Start Ollama on dev01 (usually already running)
launchctl start com.ollama.serve

# 4. Start Worker on dev01
launchctl start com.penfold.worker

# 5. Verify CLI connection
penf status
penf health gateway
```

### Standard Update Procedure

```bash
#!/bin/zsh
# Penfold Update Deployment

set -e

PENFOLD_DIR=~/github/otherjamesbrown/penfold
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "=== Penfold Update Deployment ==="
echo "Started: $(date)"

# 1. Pull latest code
echo "Step 1: Pulling latest code..."
cd ${PENFOLD_DIR}
git pull origin main

# 2. Run database migrations (if any)
echo "Step 2: Running migrations..."
penf db migrate up

# 3. Build new binaries
echo "Step 3: Building binaries..."
make build

# 4. Restart Worker (dev01)
echo "Step 4: Restarting Worker..."
launchctl stop com.penfold.worker
sleep 5
launchctl start com.penfold.worker

# 5. Deploy and restart Gateway (dev02)
echo "Step 5: Deploying Gateway..."
scp bin/penfold-gateway dev02.brown.chat:/tmp/
ssh dev02.brown.chat "pkill penfold-gateway || true; sleep 2; \
  source /opt/penfold/.env && \
  nohup /tmp/penfold-gateway > /opt/penfold/logs/gateway.log 2>&1 &"

# 6. Verify deployment
echo "Step 6: Verifying deployment..."
sleep 10
penf status
penf health gateway

echo "=== Update Complete ==="
echo "Finished: $(date)"
```

---

## Rollback Procedures

### Quick Rollback

```bash
#!/bin/zsh
# Emergency Rollback Script

set -e

PENFOLD_DIR=~/github/otherjamesbrown/penfold

echo "=== EMERGENCY ROLLBACK ==="

# 1. Stop services
echo "Stopping services..."
launchctl stop com.penfold.worker || true
ssh dev02.brown.chat "pkill penfold-gateway || true"

# 2. Rollback to previous commit
echo "Rolling back code..."
cd ${PENFOLD_DIR}
git checkout HEAD~1

# 3. Rebuild binaries
echo "Rebuilding binaries..."
make build

# 4. Redeploy
echo "Redeploying..."
scp bin/penfold-gateway dev02.brown.chat:/tmp/
ssh dev02.brown.chat "source /opt/penfold/.env && \
  nohup /tmp/penfold-gateway > /opt/penfold/logs/gateway.log 2>&1 &"
launchctl start com.penfold.worker

# 5. Verify
echo "Verifying..."
sleep 10
penf status

echo "=== Rollback Complete ==="
```

### Database Rollback

```bash
#!/bin/zsh
# Database Rollback Script

set -e

BACKUP_FILE=$1
if [[ -z "$BACKUP_FILE" ]]; then
    echo "Usage: rollback-database.sh <backup_file>"
    echo "Available backups:"
    ssh dev02.brown.chat "ls -la /opt/penfold/backups/*.sql.gz"
    exit 1
fi

echo "=== Database Rollback ==="
echo "Restoring from: ${BACKUP_FILE}"

# Stop application services
launchctl stop com.penfold.worker || true
ssh dev02.brown.chat "pkill penfold-gateway || true"

# Restore database
ssh dev02.brown.chat "gunzip -c ${BACKUP_FILE} | \
  docker exec -i penfold-postgres psql -U penfold -d penfold"

# Restart services
ssh dev02.brown.chat "source /opt/penfold/.env && \
  nohup /tmp/penfold-gateway > /opt/penfold/logs/gateway.log 2>&1 &"
launchctl start com.penfold.worker

echo "=== Database Rollback Complete ==="
```

---

## Environment Differences

### Configuration Comparison

| Setting | Development | Production |
|---------|-------------|------------|
| LOG_LEVEL | debug | info |
| Database Host | localhost | dev02.brown.chat |
| Temporal Host | localhost:7233 | dev02.brown.chat:7233 |
| TLS | disabled | optional |
| gRPC Reflection | enabled | disabled |
| Max Concurrent Activities | 4 | 10 |
| Max Concurrent Workflows | 4 | 10 |
| Graceful Shutdown Timeout | 10s | 30s |

### Development Setup

For local development on a single machine:

```bash
# Start all services locally
docker compose up -d  # PostgreSQL, Redis, Temporal

# Run Gateway
PENFOLD_DB_HOST=localhost ./bin/penfold-gateway &

# Run Worker
PENFOLD_DB_HOST=localhost PENFOLD_TEMPORAL_HOST=localhost:7233 ./bin/penfold-worker &
```

---

## Backup and Recovery

### Automated Backup Script

Create `/opt/penfold/scripts/backup-database.sh`:

```bash
#!/bin/zsh
# Penfold Database Backup Script

set -e

BACKUP_DIR="/opt/penfold/backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RETENTION_DAYS=30

echo "=== Database Backup ==="
echo "Started: $(date)"

mkdir -p ${BACKUP_DIR}

# Backup PostgreSQL
echo "Backing up PostgreSQL..."
docker exec penfold-postgres pg_dump -U penfold penfold | \
    gzip > ${BACKUP_DIR}/penfold_${TIMESTAMP}.sql.gz

# Backup Redis (RDB snapshot)
echo "Backing up Redis..."
docker exec penfold-redis redis-cli BGSAVE
sleep 5
docker cp penfold-redis:/data/dump.rdb ${BACKUP_DIR}/redis_${TIMESTAMP}.rdb

# Record backup metadata
cat > ${BACKUP_DIR}/backup_${TIMESTAMP}.json << EOF
{
    "timestamp": "${TIMESTAMP}",
    "database_file": "penfold_${TIMESTAMP}.sql.gz",
    "redis_file": "redis_${TIMESTAMP}.rdb",
    "gateway_version": "$(ssh dev02.brown.chat '/tmp/penfold-gateway --version 2>/dev/null || echo unknown')",
    "git_commit": "$(cd ~/github/otherjamesbrown/penfold && git rev-parse HEAD)"
}
EOF

# Cleanup old backups
echo "Cleaning up backups older than ${RETENTION_DAYS} days..."
find ${BACKUP_DIR} -type f -mtime +${RETENTION_DAYS} -delete

echo "Backup complete: ${BACKUP_DIR}/penfold_${TIMESTAMP}.sql.gz"
echo "Finished: $(date)"
```

---

## Troubleshooting

### Common Issues

#### Gateway Won't Start

```bash
# Check logs
ssh dev02.brown.chat "tail -100 /opt/penfold/logs/gateway.log"

# Check database connectivity
ssh dev02.brown.chat "docker exec penfold-postgres pg_isready -U penfold -d penfold"

# Check port availability
ssh dev02.brown.chat "lsof -i :50051"
```

#### Worker Won't Connect to Temporal

```bash
# Check Temporal is running
curl -s http://dev02.brown.chat:7233

# Check worker logs
tail -100 /tmp/penfold-worker.log

# Verify network connectivity
nc -zv dev02.brown.chat 7233
```

#### Ollama Service Not Responding

```bash
# Check launchd status
launchctl list | grep ollama

# Check logs
tail -100 /tmp/ollama.out.log
tail -100 /tmp/ollama.err.log

# Restart service
launchctl stop com.ollama.serve
launchctl start com.ollama.serve

# Test directly
curl http://localhost:11434/
```

#### Database Connection Issues

```bash
# Test from dev01
source ~/github/otherjamesbrown/secrets/.env.penfold
psql "host=$PENFOLD_DB_HOST user=$PENFOLD_DB_USER password=$PENFOLD_DB_PASSWORD dbname=$PENFOLD_DB_NAME" -c "SELECT 1"

# Check PostgreSQL logs
ssh dev02.brown.chat "docker logs penfold-postgres --tail 100"
```

### Log Analysis

```bash
# Gateway errors
ssh dev02.brown.chat "grep -i 'error\|failed' /opt/penfold/logs/gateway.log | tail -50"

# Worker errors
grep -i 'error\|failed' /tmp/penfold-worker.log | tail -50

# Temporal workflow failures
open http://dev02.brown.chat:8088  # Temporal UI
```

### Performance Debugging

```bash
# Database slow queries
ssh dev02.brown.chat "docker exec penfold-postgres psql -U penfold -d penfold -c \"
SELECT query, calls, total_exec_time/calls as avg_time
FROM pg_stat_statements
ORDER BY avg_time DESC
LIMIT 10;
\""

# Container resource usage
ssh dev02.brown.chat "docker stats --no-stream"

# Worker metrics
curl -s http://dev01.brown.chat:8085/metrics
```

---

## Quick Reference

### Essential Commands

```bash
# Status check
penf status
penf health gateway

# Service management (dev01)
launchctl list | grep penfold
launchctl stop com.penfold.worker
launchctl start com.penfold.worker

# Service management (dev02)
ssh dev02.brown.chat "docker ps --filter 'name=penfold'"
ssh dev02.brown.chat "pkill penfold-gateway"

# Database access
ssh dev02.brown.chat "docker exec -it penfold-postgres psql -U penfold -d penfold"

# Redis access
ssh dev02.brown.chat "docker exec -it penfold-redis redis-cli"

# Temporal UI
open http://dev02.brown.chat:8088

# View logs
tail -f /tmp/penfold-worker.log
ssh dev02.brown.chat "tail -f /opt/penfold/logs/gateway.log"
```

### Important File Locations

| Purpose | Location |
|---------|----------|
| Source code | `~/github/otherjamesbrown/penfold/` |
| Credentials | `~/github/otherjamesbrown/secrets/.env.penfold` |
| CLI config | `~/.penf/config.yaml` |
| Worker logs | `/tmp/penfold-worker.log` |
| Ollama logs | `/tmp/ollama.out.log`, `/tmp/ollama.err.log` |
| Gateway logs | `/opt/penfold/logs/gateway.log` (dev02) |
| Database data | `/opt/penfold/data/postgres/` (dev02) |
| Backups | `/opt/penfold/backups/` (dev02) |

---

## Related Documentation

- [Architecture Overview](../../ARCHITECTURE.md)
- [Infrastructure Details](../../context/infrastructure.md)
- [Gmail Integration Setup](../gmail-integration/setup-guide.md)
