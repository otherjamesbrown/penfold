# Penfold Production Deployment Guide

**Last Updated**: 2026-01-15
**Target Environment**: Mac Mini M4 (32GB RAM)
**Deployment Model**: Single-node Docker Compose

This guide provides step-by-step instructions for deploying Penfold to a production environment. It covers container orchestration, security hardening, monitoring, and operational procedures.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Architecture Overview](#architecture-overview)
- [Environment Configuration](#environment-configuration)
- [Docker Compose Production Setup](#docker-compose-production-setup)
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

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | Apple M1 | Apple M4 |
| RAM | 16GB | 32GB |
| Storage | 256GB SSD | 512GB+ SSD |
| Network | 100Mbps | 1Gbps |

### Software Requirements

```bash
# macOS Sonoma or later
sw_vers

# Docker Desktop 4.25+ with Apple Silicon support
docker --version

# Docker Compose v2
docker compose version

# Homebrew (for additional tools)
brew --version
```

### Required Accounts and Credentials

- Google Cloud project with enabled APIs (Gmail, Calendar, Speech-to-Text)
- OAuth2 credentials for Gmail integration
- Optional: OpenAI API key for embeddings

---

## Architecture Overview

```
                                    +-----------------+
                                    |   Internet      |
                                    +--------+--------+
                                             |
                                    +--------v--------+
                                    |   Firewall      |
                                    |   (pf rules)    |
                                    +--------+--------+
                                             |
                          +------------------+------------------+
                          |                                     |
                 +--------v--------+                   +--------v--------+
                 |   Caddy/nginx   |                   |   Caddy/nginx   |
                 |   (SSL Proxy)   |                   |   (SSL Proxy)   |
                 |   :443 -> :8000 |                   |   :443 -> :8001 |
                 +--------+--------+                   +--------+--------+
                          |                                     |
           +--------------+-------------+                       |
           |                            |                       |
  +--------v--------+        +----------v---------+   +---------v--------+
  |   API Service   |        |   Worker Service   |   |  Observability   |
  |   (FastAPI)     |        |   (Procrastinate)  |   |  Dashboard       |
  |   Port 8000     |        |   Background Jobs  |   |  Port 8001       |
  +--------+--------+        +----------+---------+   +------------------+
           |                            |
           +------------+---------------+
                        |
         +--------------+--------------+
         |                             |
+--------v--------+          +--------v--------+
|   PostgreSQL    |          |     Redis       |
|   + pgvector    |          |   (Cache/Pub)   |
|   Port 5432     |          |   Port 6379     |
+-----------------+          +-----------------+
```

### Service Components

| Service | Purpose | Container Image |
|---------|---------|-----------------|
| postgres | Primary database with pgvector | pgvector/pgvector:pg16 |
| redis | Event bus, caching | redis:7-alpine |
| api | REST API (FastAPI) | penfold:latest |
| worker | Background job processing | penfold:latest |
| caddy | SSL termination, reverse proxy | caddy:2-alpine |

---

## Environment Configuration

### Directory Structure

```bash
# Create production directories
sudo mkdir -p /opt/penfold/{config,data,logs,backups,certs}
sudo mkdir -p /opt/penfold/data/{postgres,redis,uploads,processed}
sudo chown -R $(whoami):staff /opt/penfold
```

### Production Environment File

Create `/opt/penfold/config/.env.production`:

```bash
# ===========================================
# PENFOLD PRODUCTION CONFIGURATION
# ===========================================

# Application
ENVIRONMENT=production
DEBUG=false
LOG_LEVEL=INFO
HOST=0.0.0.0
PORT=8000

# Domain configuration (update for your domain)
DOMAIN=penfold.local
ALLOWED_ORIGINS=["https://penfold.local"]

# Database (use strong, unique password)
DATABASE_URL=postgresql://penfold:CHANGE_ME_STRONG_PASSWORD@postgres:5432/penfold_prod
DATABASE_POOL_SIZE=20
DATABASE_MAX_OVERFLOW=30
DATABASE_POOL_TIMEOUT=30

# Redis
REDIS_URL=redis://redis:6379/0

# File storage
UPLOAD_DIR=/app/uploads
PROCESSED_DIR=/app/processed
MAX_UPLOAD_SIZE=2147483648

# Security (generate unique secrets)
# Generate with: openssl rand -hex 32
JWT_SECRET_KEY=CHANGE_ME_GENERATE_NEW_SECRET
JWT_ALGORITHM=HS256
JWT_EXPIRATION_HOURS=24
ENCRYPTION_KEY_PATH=/app/config/encryption-keys

# Speech-to-Text
WHISPER_MODEL_SIZE=large-v3
STT_CONFIDENCE_THRESHOLD=0.8
USE_LOCAL_STT_ONLY=true

# Processing limits
MAX_CONCURRENT_JOBS=20
MAX_CPU_INTENSIVE_JOBS=4
JOB_TIMEOUT_SECONDS=7200
FIFO_QUEUE_ENABLED=true

# Observability
OBSERVABILITY_ENABLED=true
OBSERVABILITY_DATABASE_URL=postgresql://penfold:CHANGE_ME_STRONG_PASSWORD@postgres:5432/penfold_prod
METRICS_RETENTION_DAYS=90
```

### Generate Security Secrets

```bash
# Generate JWT secret
JWT_SECRET=$(openssl rand -hex 32)
echo "JWT_SECRET_KEY=${JWT_SECRET}"

# Generate encryption key
mkdir -p /opt/penfold/config/encryption-keys
openssl rand -base64 32 > /opt/penfold/config/encryption-keys/master.key
chmod 600 /opt/penfold/config/encryption-keys/master.key

# Generate database password
DB_PASSWORD=$(openssl rand -base64 24)
echo "Database password: ${DB_PASSWORD}"
```

---

## Docker Compose Production Setup

### Production Docker Compose File

Create `/opt/penfold/config/docker-compose.production.yml`:

```yaml
# Penfold Production Docker Compose Configuration
# Deploy with: docker compose -f docker-compose.production.yml up -d

version: '3.8'

services:
  # PostgreSQL with pgvector extension
  postgres:
    image: pgvector/pgvector:pg16
    container_name: penfold-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: penfold_prod
      POSTGRES_USER: penfold
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      PGDATA: /var/lib/postgresql/data/pgdata
    ports:
      - "127.0.0.1:5432:5432"  # Only bind to localhost
    volumes:
      - /opt/penfold/data/postgres:/var/lib/postgresql/data
      - ./init-db:/docker-entrypoint-initdb.d:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U penfold -d penfold_prod"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s
    deploy:
      resources:
        limits:
          memory: 4G
        reservations:
          memory: 1G
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "5"

  # Redis for caching and event bus
  redis:
    image: redis:7-alpine
    container_name: penfold-redis
    restart: unless-stopped
    command: redis-server --appendonly yes --maxmemory 512mb --maxmemory-policy allkeys-lru
    ports:
      - "127.0.0.1:6379:6379"  # Only bind to localhost
    volumes:
      - /opt/penfold/data/redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    deploy:
      resources:
        limits:
          memory: 1G
        reservations:
          memory: 256M
    logging:
      driver: "json-file"
      options:
        max-size: "50m"
        max-file: "3"

  # Penfold API Service
  api:
    image: penfold:latest
    container_name: penfold-api
    build:
      context: .
      dockerfile: Dockerfile.production
    restart: unless-stopped
    env_file:
      - .env.production
    ports:
      - "127.0.0.1:8000:8000"  # Only bind to localhost
    volumes:
      - /opt/penfold/data/uploads:/app/uploads
      - /opt/penfold/data/processed:/app/processed
      - /opt/penfold/config/encryption-keys:/app/config/encryption-keys:ro
      - /opt/penfold/logs:/app/logs
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s
    deploy:
      resources:
        limits:
          memory: 8G
        reservations:
          memory: 2G
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "10"
    command: >
      uvicorn app.main:app
      --host 0.0.0.0
      --port 8000
      --workers 4
      --loop uvloop
      --http httptools
      --access-log
      --log-level info

  # Penfold Background Worker
  worker:
    image: penfold:latest
    container_name: penfold-worker
    restart: unless-stopped
    env_file:
      - .env.production
    volumes:
      - /opt/penfold/data/uploads:/app/uploads
      - /opt/penfold/data/processed:/app/processed
      - /opt/penfold/config/encryption-keys:/app/config/encryption-keys:ro
      - /opt/penfold/logs:/app/logs
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    deploy:
      resources:
        limits:
          memory: 12G  # Higher for AI model processing
        reservations:
          memory: 4G
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "10"
    command: python -m procrastinate worker --concurrency=4

  # Observability Dashboard
  observability:
    image: penfold:latest
    container_name: penfold-observability
    restart: unless-stopped
    env_file:
      - .env.production
    ports:
      - "127.0.0.1:8001:8001"
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8001/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 30s
    deploy:
      resources:
        limits:
          memory: 1G
        reservations:
          memory: 256M
    logging:
      driver: "json-file"
      options:
        max-size: "50m"
        max-file: "5"
    command: >
      uvicorn observability_lib.cli.dashboard:app
      --host 0.0.0.0
      --port 8001
      --workers 1
      --log-level info

  # Caddy reverse proxy with automatic SSL
  caddy:
    image: caddy:2-alpine
    container_name: penfold-caddy
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - /opt/penfold/certs:/data
      - /opt/penfold/certs:/config
    depends_on:
      - api
      - observability
    logging:
      driver: "json-file"
      options:
        max-size: "50m"
        max-file: "5"

networks:
  default:
    name: penfold-network
    driver: bridge

volumes:
  postgres_data:
  redis_data:
```

### Production Dockerfile

Create `/opt/penfold/config/Dockerfile.production`:

```dockerfile
# Penfold Production Docker Image
# Multi-stage build for optimized image size

# Stage 1: Build dependencies
FROM python:3.12-slim as builder

WORKDIR /build

# Install build dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    && rm -rf /var/lib/apt/lists/*

# Copy and install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

# Stage 2: Production image
FROM python:3.12-slim

# Create non-root user for security
RUN groupadd -r penfold && useradd -r -g penfold penfold

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    postgresql-client \
    ffmpeg \
    curl \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Copy installed packages from builder
COPY --from=builder /install /usr/local

# Set working directory
WORKDIR /app

# Copy application code
COPY --chown=penfold:penfold . .

# Create required directories
RUN mkdir -p uploads processed logs config/encryption-keys \
    && chown -R penfold:penfold /app

# Switch to non-root user
USER penfold

# Environment variables
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONPATH=/app

# Expose API port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# Default command
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

---

## SSL/TLS Configuration

### Caddyfile for Automatic SSL

Create `/opt/penfold/config/Caddyfile`:

```caddyfile
# Penfold Caddy Configuration
# Automatic HTTPS with Let's Encrypt

{
    # Global options
    email admin@yourdomain.com
    acme_ca https://acme-v02.api.letsencrypt.org/directory

    # For local development/testing, use internal CA
    # acme_ca https://acme-staging-v02.api.letsencrypt.org/directory
}

# Main API endpoint
penfold.yourdomain.com {
    # Enable compression
    encode gzip zstd

    # Security headers
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "DENY"
        X-XSS-Protection "1; mode=block"
        Referrer-Policy "strict-origin-when-cross-origin"
        -Server
    }

    # Rate limiting
    rate_limit {
        zone api {
            key {remote_host}
            events 100
            window 1m
        }
    }

    # API routes
    handle /api/* {
        reverse_proxy api:8000 {
            health_uri /health
            health_interval 30s
            health_timeout 10s
        }
    }

    # Health check endpoint
    handle /health {
        reverse_proxy api:8000
    }

    # Default handler
    handle {
        respond "Penfold API" 200
    }

    # Access logging
    log {
        output file /var/log/caddy/access.log {
            roll_size 100mb
            roll_keep 5
        }
        format json
    }
}

# Observability dashboard (internal only)
observability.penfold.yourdomain.com {
    # Require basic auth for dashboard access
    basicauth * {
        admin $2a$14$HASHED_PASSWORD_HERE
    }

    reverse_proxy observability:8001

    log {
        output file /var/log/caddy/observability.log {
            roll_size 50mb
            roll_keep 3
        }
    }
}
```

### Self-Signed Certificates for Local/Internal Use

For local network or development environments:

```bash
# Generate self-signed certificate
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout /opt/penfold/certs/penfold.key \
    -out /opt/penfold/certs/penfold.crt \
    -subj "/CN=penfold.local/O=Penfold/C=US"

# Create combined PEM file
cat /opt/penfold/certs/penfold.crt /opt/penfold/certs/penfold.key > \
    /opt/penfold/certs/penfold.pem
chmod 600 /opt/penfold/certs/*.key /opt/penfold/certs/*.pem
```

---

## Network Security

### macOS Firewall Rules (pf)

Create `/etc/pf.anchors/penfold.rules`:

```
# Penfold Firewall Rules
# Load with: sudo pfctl -f /etc/pf.conf

# Define interfaces and networks
ext_if = "en0"                    # Primary network interface
penfold_net = "172.18.0.0/16"     # Docker network range

# Default deny policy
block in all
pass out all keep state

# Allow established connections
pass in quick on lo0 all
pass in quick on $ext_if proto tcp from any to any port {80, 443} keep state

# Allow SSH from local network only
pass in quick on $ext_if proto tcp from 192.168.0.0/16 to any port 22 keep state

# Block direct access to internal service ports from external
block in quick on $ext_if proto tcp from any to any port {5432, 6379, 8000, 8001}

# Rate limiting for HTTP(S)
pass in on $ext_if proto tcp from any to any port {80, 443} \
    keep state (max-src-conn 100, max-src-conn-rate 20/10)
```

Enable firewall rules:

```bash
# Add anchor to main pf.conf
sudo cat >> /etc/pf.conf << 'EOF'
anchor "penfold"
load anchor "penfold" from "/etc/pf.anchors/penfold.rules"
EOF

# Enable pf
sudo pfctl -e
sudo pfctl -f /etc/pf.conf
```

### Docker Network Isolation

```bash
# Create isolated Docker network
docker network create \
    --driver bridge \
    --subnet 172.18.0.0/16 \
    --ip-range 172.18.1.0/24 \
    --opt "com.docker.network.bridge.enable_ip_masquerade=true" \
    --opt "com.docker.network.bridge.enable_icc=false" \
    penfold-network
```

---

## Health Checks and Monitoring

### Health Check Endpoints

The API service exposes these health check endpoints:

| Endpoint | Purpose | Expected Response |
|----------|---------|-------------------|
| `/health` | Basic liveness check | `{"status": "healthy"}` |
| `/health/ready` | Readiness probe (with dependencies) | `{"status": "ready", "components": {...}}` |
| `/health/live` | Kubernetes-style liveness | `{"status": "ok"}` |

### Health Check Script

Create `/opt/penfold/scripts/health-check.sh`:

```bash
#!/bin/zsh
# Penfold Health Check Script

set -e

API_URL="http://localhost:8000"
OBSERVABILITY_URL="http://localhost:8001"

echo "=== Penfold Health Check ==="
echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo ""

# Check API health
echo "Checking API service..."
API_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${API_URL}/health" || echo "000")
if [[ "$API_STATUS" == "200" ]]; then
    echo "  API: HEALTHY"
    API_RESPONSE=$(curl -s "${API_URL}/health")
    echo "  Response: ${API_RESPONSE}"
else
    echo "  API: UNHEALTHY (HTTP ${API_STATUS})"
fi
echo ""

# Check database connectivity (via API readiness)
echo "Checking database connectivity..."
READY_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${API_URL}/health/ready" || echo "000")
if [[ "$READY_STATUS" == "200" ]]; then
    READY_RESPONSE=$(curl -s "${API_URL}/health/ready")
    DB_STATUS=$(echo "$READY_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('components',{}).get('database','unknown'))")
    echo "  Database: ${DB_STATUS}"
else
    echo "  Database: UNREACHABLE"
fi
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

# Check worker status
echo "Checking worker processes..."
WORKER_COUNT=$(docker exec penfold-worker pgrep -c python 2>/dev/null || echo "0")
echo "  Active workers: ${WORKER_COUNT}"
echo ""

# Check disk space
echo "Checking disk space..."
DISK_USAGE=$(df -h /opt/penfold | tail -1 | awk '{print $5}')
echo "  Disk usage: ${DISK_USAGE}"
echo ""

# Check observability dashboard
echo "Checking observability dashboard..."
OBS_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${OBSERVABILITY_URL}/health" || echo "000")
if [[ "$OBS_STATUS" == "200" ]]; then
    echo "  Observability: HEALTHY"
else
    echo "  Observability: UNHEALTHY (HTTP ${OBS_STATUS})"
fi
echo ""

echo "=== Health Check Complete ==="
```

### Monitoring with cron

Add to crontab (`crontab -e`):

```cron
# Penfold health checks every 5 minutes
*/5 * * * * /opt/penfold/scripts/health-check.sh >> /opt/penfold/logs/health-check.log 2>&1

# Log rotation daily
0 0 * * * /opt/penfold/scripts/rotate-logs.sh >> /opt/penfold/logs/maintenance.log 2>&1

# Database backup daily at 2 AM
0 2 * * * /opt/penfold/scripts/backup-database.sh >> /opt/penfold/logs/backup.log 2>&1
```

---

## Deployment Runbook

### Pre-Deployment Checklist

```markdown
- [ ] Backup current database
- [ ] Document current container versions
- [ ] Review changelog for breaking changes
- [ ] Verify sufficient disk space (>20% free)
- [ ] Notify stakeholders of maintenance window
- [ ] Prepare rollback commands
- [ ] Verify test environment deployment succeeded
```

### Initial Deployment

```bash
#!/bin/zsh
# Initial Penfold Production Deployment

set -e

DEPLOY_DIR="/opt/penfold"
CONFIG_DIR="${DEPLOY_DIR}/config"
BACKUP_DIR="${DEPLOY_DIR}/backups"

echo "=== Penfold Initial Deployment ==="
echo "Started: $(date)"

# 1. Clone/update repository
echo "Step 1: Setting up application code..."
cd ${DEPLOY_DIR}
git clone https://github.com/otherjamesbrown/penfold.git app || \
    (cd app && git pull origin main)

# 2. Copy production configuration
echo "Step 2: Configuring environment..."
cp ${CONFIG_DIR}/.env.production ${DEPLOY_DIR}/app/.env
cp ${CONFIG_DIR}/docker-compose.production.yml ${DEPLOY_DIR}/app/docker-compose.yml
cp ${CONFIG_DIR}/Caddyfile ${DEPLOY_DIR}/app/

# 3. Build Docker images
echo "Step 3: Building Docker images..."
cd ${DEPLOY_DIR}/app
docker compose build --no-cache

# 4. Initialize database
echo "Step 4: Starting database services..."
docker compose up -d postgres redis
sleep 30  # Wait for PostgreSQL to be ready

# 5. Run database migrations
echo "Step 5: Running database migrations..."
docker compose run --rm api alembic upgrade head

# 6. Initialize pgvector extension
echo "Step 6: Initializing pgvector..."
docker compose exec -T postgres psql -U penfold -d penfold_prod -c \
    "CREATE EXTENSION IF NOT EXISTS vector;"

# 7. Start all services
echo "Step 7: Starting all services..."
docker compose up -d

# 8. Verify deployment
echo "Step 8: Verifying deployment..."
sleep 30
/opt/penfold/scripts/health-check.sh

echo "=== Deployment Complete ==="
echo "Finished: $(date)"
```

### Standard Update Procedure

```bash
#!/bin/zsh
# Penfold Update Deployment

set -e

DEPLOY_DIR="/opt/penfold"
BACKUP_DIR="${DEPLOY_DIR}/backups"
APP_DIR="${DEPLOY_DIR}/app"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "=== Penfold Update Deployment ==="
echo "Started: $(date)"

# 1. Pre-deployment backup
echo "Step 1: Creating pre-deployment backup..."
${DEPLOY_DIR}/scripts/backup-database.sh

# Record current image versions
docker compose -f ${APP_DIR}/docker-compose.yml images > \
    ${BACKUP_DIR}/images_${TIMESTAMP}.txt

# 2. Pull latest code
echo "Step 2: Pulling latest code..."
cd ${APP_DIR}
git fetch origin
git checkout main
git pull origin main

# 3. Build new images
echo "Step 3: Building new Docker images..."
docker compose build

# 4. Run migrations (if any)
echo "Step 4: Running database migrations..."
docker compose run --rm api alembic upgrade head

# 5. Rolling restart of services
echo "Step 5: Performing rolling restart..."

# Restart workers first (drain jobs)
docker compose stop worker
docker compose up -d worker
sleep 10

# Restart API (zero-downtime with multiple workers)
docker compose up -d --no-deps api
sleep 10

# Restart observability
docker compose up -d --no-deps observability

# 6. Verify deployment
echo "Step 6: Verifying deployment..."
sleep 30
/opt/penfold/scripts/health-check.sh

# 7. Cleanup old images
echo "Step 7: Cleaning up old images..."
docker image prune -f

echo "=== Update Complete ==="
echo "Finished: $(date)"
```

### Zero-Downtime Deployment

For critical updates requiring zero downtime:

```bash
#!/bin/zsh
# Zero-Downtime Penfold Deployment

set -e

APP_DIR="/opt/penfold/app"

# 1. Scale up API instances
docker compose -f ${APP_DIR}/docker-compose.yml up -d --scale api=2

# 2. Wait for new instance to be healthy
echo "Waiting for new API instance..."
sleep 60

# 3. Remove old API instance
docker compose -f ${APP_DIR}/docker-compose.yml up -d --scale api=1 --no-recreate

echo "Zero-downtime deployment complete"
```

---

## Rollback Procedures

### Quick Rollback (< 5 minutes)

```bash
#!/bin/zsh
# Emergency Rollback Script

set -e

DEPLOY_DIR="/opt/penfold"
APP_DIR="${DEPLOY_DIR}/app"
BACKUP_DIR="${DEPLOY_DIR}/backups"

echo "=== EMERGENCY ROLLBACK ==="
echo "Started: $(date)"

# Stop current services
echo "Stopping services..."
docker compose -f ${APP_DIR}/docker-compose.yml down

# Rollback to previous git commit
echo "Rolling back code..."
cd ${APP_DIR}
git checkout HEAD~1

# Restart with previous version
echo "Restarting services..."
docker compose -f ${APP_DIR}/docker-compose.yml up -d

# Verify
sleep 30
/opt/penfold/scripts/health-check.sh

echo "=== Rollback Complete ==="
```

### Database Rollback

```bash
#!/bin/zsh
# Database Rollback Script

set -e

BACKUP_FILE=$1
DEPLOY_DIR="/opt/penfold"

if [[ -z "$BACKUP_FILE" ]]; then
    echo "Usage: rollback-database.sh <backup_file>"
    echo "Available backups:"
    ls -la ${DEPLOY_DIR}/backups/*.sql.gz
    exit 1
fi

echo "=== Database Rollback ==="
echo "Restoring from: ${BACKUP_FILE}"

# Stop application services (keep database running)
docker compose -f ${DEPLOY_DIR}/app/docker-compose.yml stop api worker observability

# Restore database
gunzip -c ${BACKUP_FILE} | docker exec -i penfold-postgres \
    psql -U penfold -d penfold_prod

# Run any necessary migrations
docker compose -f ${DEPLOY_DIR}/app/docker-compose.yml run --rm api \
    alembic upgrade head

# Restart services
docker compose -f ${DEPLOY_DIR}/app/docker-compose.yml up -d

echo "=== Database Rollback Complete ==="
```

### Complete System Rollback

For catastrophic failures requiring full system restoration:

```bash
#!/bin/zsh
# Full System Rollback

set -e

BACKUP_DATE=$1
DEPLOY_DIR="/opt/penfold"

echo "=== FULL SYSTEM ROLLBACK ==="
echo "Restoring to: ${BACKUP_DATE}"

# 1. Stop all services
docker compose -f ${DEPLOY_DIR}/app/docker-compose.yml down

# 2. Restore database
${DEPLOY_DIR}/scripts/rollback-database.sh \
    ${DEPLOY_DIR}/backups/penfold_${BACKUP_DATE}.sql.gz

# 3. Restore uploaded files
rsync -av ${DEPLOY_DIR}/backups/uploads_${BACKUP_DATE}/ \
    ${DEPLOY_DIR}/data/uploads/

# 4. Checkout specific git tag/commit
cd ${DEPLOY_DIR}/app
git checkout tags/v${BACKUP_DATE} || git checkout ${BACKUP_DATE}

# 5. Rebuild and restart
docker compose build
docker compose up -d

# 6. Verify
sleep 60
/opt/penfold/scripts/health-check.sh

echo "=== Full System Rollback Complete ==="
```

---

## Environment Differences

### Configuration Comparison

| Setting | Development | Staging | Production |
|---------|-------------|---------|------------|
| DEBUG | true | true | false |
| LOG_LEVEL | DEBUG | INFO | INFO |
| DATABASE_POOL_SIZE | 5 | 10 | 20 |
| MAX_CONCURRENT_JOBS | 4 | 10 | 20 |
| WHISPER_MODEL_SIZE | base | medium | large-v3 |
| USE_LOCAL_STT_ONLY | true | false | true |
| SSL/TLS | none | self-signed | Let's Encrypt |
| Resource Limits | none | moderate | strict |
| Backup Frequency | none | daily | daily |
| Monitoring | basic | full | full |

### Environment-Specific Compose Files

```bash
# Development
docker compose -f docker-compose.yml up -d

# Staging
docker compose -f docker-compose.yml -f docker-compose.staging.yml up -d

# Production
docker compose -f docker-compose.production.yml up -d
```

### Staging-Specific Overrides

Create `docker-compose.staging.yml`:

```yaml
version: '3.8'

services:
  api:
    environment:
      - DEBUG=true
      - LOG_LEVEL=DEBUG
    deploy:
      resources:
        limits:
          memory: 4G

  worker:
    environment:
      - MAX_CONCURRENT_JOBS=10
    deploy:
      resources:
        limits:
          memory: 8G
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

# Create backup directory if not exists
mkdir -p ${BACKUP_DIR}

# Backup PostgreSQL
echo "Backing up PostgreSQL..."
docker exec penfold-postgres pg_dump -U penfold penfold_prod | \
    gzip > ${BACKUP_DIR}/penfold_${TIMESTAMP}.sql.gz

# Backup Redis (RDB snapshot)
echo "Backing up Redis..."
docker exec penfold-redis redis-cli BGSAVE
sleep 5
docker cp penfold-redis:/data/dump.rdb ${BACKUP_DIR}/redis_${TIMESTAMP}.rdb

# Backup uploaded files
echo "Backing up uploads..."
tar -czf ${BACKUP_DIR}/uploads_${TIMESTAMP}.tar.gz \
    -C /opt/penfold/data uploads/

# Record backup metadata
cat > ${BACKUP_DIR}/backup_${TIMESTAMP}.json << EOF
{
    "timestamp": "${TIMESTAMP}",
    "database_file": "penfold_${TIMESTAMP}.sql.gz",
    "redis_file": "redis_${TIMESTAMP}.rdb",
    "uploads_file": "uploads_${TIMESTAMP}.tar.gz",
    "git_commit": "$(cd /opt/penfold/app && git rev-parse HEAD)",
    "docker_images": $(docker compose -f /opt/penfold/app/docker-compose.yml images --format json)
}
EOF

# Cleanup old backups
echo "Cleaning up backups older than ${RETENTION_DAYS} days..."
find ${BACKUP_DIR} -type f -mtime +${RETENTION_DAYS} -delete

echo "Backup complete: ${BACKUP_DIR}/penfold_${TIMESTAMP}.sql.gz"
echo "Finished: $(date)"
```

### Recovery Verification

After any restore, run verification:

```bash
#!/bin/zsh
# Post-Recovery Verification

set -e

echo "=== Post-Recovery Verification ==="

# Check database connectivity
echo "Checking database..."
docker exec penfold-postgres psql -U penfold -d penfold_prod -c \
    "SELECT COUNT(*) FROM sources;" || echo "Database check failed"

# Check vector extension
echo "Checking pgvector..."
docker exec penfold-postgres psql -U penfold -d penfold_prod -c \
    "SELECT * FROM pg_extension WHERE extname = 'vector';" || echo "pgvector check failed"

# Check Redis data
echo "Checking Redis..."
docker exec penfold-redis redis-cli DBSIZE || echo "Redis check failed"

# Run health checks
/opt/penfold/scripts/health-check.sh

echo "=== Verification Complete ==="
```

---

## Troubleshooting

### Common Issues

#### Container Won't Start

```bash
# Check container logs
docker compose logs api --tail 100

# Check container status
docker compose ps

# Inspect container
docker inspect penfold-api

# Check resource usage
docker stats --no-stream
```

#### Database Connection Issues

```bash
# Test database connectivity
docker exec penfold-postgres pg_isready -U penfold -d penfold_prod

# Check PostgreSQL logs
docker compose logs postgres --tail 100

# Test from API container
docker exec penfold-api python -c "
import asyncpg
import asyncio
async def test():
    conn = await asyncpg.connect('postgresql://penfold:password@postgres:5432/penfold_prod')
    print(await conn.fetchval('SELECT 1'))
    await conn.close()
asyncio.run(test())
"
```

#### Memory Issues

```bash
# Check memory usage
docker stats --format "table {{.Name}}\t{{.MemUsage}}\t{{.MemPerc}}"

# If worker runs out of memory, reduce concurrency
docker exec penfold-worker kill -HUP 1  # Graceful reload

# Or restart with reduced resources
docker compose up -d --scale worker=1 worker
```

#### SSL Certificate Issues

```bash
# Check certificate expiry
openssl s_client -connect penfold.yourdomain.com:443 -servername penfold.yourdomain.com \
    2>/dev/null | openssl x509 -noout -dates

# Force certificate renewal (Caddy)
docker exec penfold-caddy caddy reload --config /etc/caddy/Caddyfile

# Check Caddy logs
docker compose logs caddy --tail 100
```

### Log Analysis

```bash
# Search for errors across all services
docker compose logs --since 1h 2>&1 | grep -i "error\|exception\|failed"

# API request tracing
docker compose logs api --since 1h | grep -E "^\d{4}-\d{2}-\d{2}.*POST|GET|PUT|DELETE"

# Worker job failures
docker compose logs worker --since 1h | grep -i "failed\|error\|timeout"
```

### Performance Debugging

```bash
# Database slow queries
docker exec penfold-postgres psql -U penfold -d penfold_prod -c "
SELECT query, calls, total_time, mean_time
FROM pg_stat_statements
ORDER BY mean_time DESC
LIMIT 10;
"

# API response times
curl -w "@curl-format.txt" -s -o /dev/null https://penfold.yourdomain.com/health

# Container resource limits
docker inspect penfold-api --format '{{.HostConfig.Memory}} {{.HostConfig.MemoryReservation}}'
```

---

## Quick Reference

### Essential Commands

```bash
# Start all services
docker compose -f /opt/penfold/app/docker-compose.yml up -d

# Stop all services
docker compose -f /opt/penfold/app/docker-compose.yml down

# View logs
docker compose -f /opt/penfold/app/docker-compose.yml logs -f

# Restart specific service
docker compose -f /opt/penfold/app/docker-compose.yml restart api

# Check status
docker compose -f /opt/penfold/app/docker-compose.yml ps

# Run database migration
docker compose -f /opt/penfold/app/docker-compose.yml run --rm api alembic upgrade head

# Access database shell
docker exec -it penfold-postgres psql -U penfold -d penfold_prod

# Access Redis CLI
docker exec -it penfold-redis redis-cli

# Run health check
/opt/penfold/scripts/health-check.sh

# Create backup
/opt/penfold/scripts/backup-database.sh
```

### Important File Locations

| Purpose | Location |
|---------|----------|
| Application code | `/opt/penfold/app/` |
| Configuration | `/opt/penfold/config/` |
| Database data | `/opt/penfold/data/postgres/` |
| Redis data | `/opt/penfold/data/redis/` |
| Uploaded files | `/opt/penfold/data/uploads/` |
| Processed files | `/opt/penfold/data/processed/` |
| Application logs | `/opt/penfold/logs/` |
| Backups | `/opt/penfold/backups/` |
| SSL certificates | `/opt/penfold/certs/` |
| Encryption keys | `/opt/penfold/config/encryption-keys/` |

---

## Related Documentation

- [Architecture Overview](../../ARCHITECTURE.md)
- [Observability Framework](../observability-framework/README.md)
- [Meeting Pipeline Quickstart](../../specs/005-meeting-pipeline/quickstart.md)
- [Gmail Integration Setup](../gmail-integration/setup-guide.md)
