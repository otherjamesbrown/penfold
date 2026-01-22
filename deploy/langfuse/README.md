# Langfuse Deployment for Penfold AI Provenance

Self-hosted Langfuse instance for tracing and visualizing Penfold's AI operations.

## Quick Start

```bash
# On home-01.brown.chat
cd /opt/langfuse
docker compose up -d
```

Access: http://home-01.brown.chat:3000

## Port Allocations

| Service | Port | Notes |
|---------|------|-------|
| langfuse-web | 3000 | Main UI and API |
| langfuse-postgres | 5433 | Separate from penfold-postgres (5432) |
| langfuse-redis | 6380 | Separate from penfold-redis (6379) |
| langfuse-clickhouse | 8123, 9000 | Trace storage |
| langfuse-minio | 9090, 9091 | Blob storage |

## Initial Credentials

- **URL**: http://home-01.brown.chat:3000
- **Email**: james@brown.chat
- **Password**: (see .env file, change on first login)

## Penfold Integration

API keys for OTEL exporter:
- **Public Key**: `pk-lf-penfold`
- **Secret Key**: `sk-lf-penfold-secret`

Configure in Penfold worker:
```bash
LANGFUSE_PUBLIC_KEY=pk-lf-penfold
LANGFUSE_SECRET_KEY=sk-lf-penfold-secret
LANGFUSE_HOST=http://home-01.brown.chat:3000
```

## Deployment

```bash
# Copy files to home-01
scp -r deploy/langfuse home-01.brown.chat:/opt/

# SSH to home-01 and start
ssh home-01.brown.chat
cd /opt/langfuse
docker compose up -d

# Check status
docker compose ps
docker compose logs -f langfuse-web
```

## Maintenance

```bash
# View logs
docker compose logs -f

# Restart
docker compose restart

# Update images
docker compose pull
docker compose up -d

# Backup (PostgreSQL data)
docker exec langfuse-postgres pg_dump -U langfuse langfuse > langfuse_backup.sql
```

## Troubleshooting

```bash
# Check service health
curl http://home-01.brown.chat:3000/api/public/health

# Check ClickHouse
curl http://localhost:8123/ping

# Redis connectivity
docker exec langfuse-redis redis-cli -a $REDIS_AUTH ping
```
