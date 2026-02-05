# PE-GWCF: Gateway Configuration Fix

**Status:** Complete
**Created:** 2026-01-27
**Updated:** 2026-02-05

## Problem

The Gateway on dev02 had incorrect service addresses:
- Database: connecting to `localhost:5432` instead of using SSL certs
- AI Service: incorrectly pointed to dev01 when AI Coordinator runs on dev02
- Worker: connecting to `localhost:8085` instead of `dev01.brown.chat:8085`

## Solution

1. Copied SSL client certificates to dev02 (`~/.postgresql/`)
2. Updated `pkg/config` to support SSL certificate paths
3. Gateway connects to `dev02.brown.chat:5432` (not localhost) for proper SSL cert auth
4. AI Coordinator address: `localhost:50055` (runs on same host as Gateway)
5. MLX services (LLM, Embeddings) and Worker on dev01

## Service Architecture

| Service | Host | gRPC Port | HTTP Port |
|---------|------|-----------|-----------|
| Gateway | dev02 | 50051 | 8080 |
| AI Coordinator | dev02 | 50055 | 8090 |
| Worker | dev01 | - | 8085 |
| MLX LLM | dev01 | - | 8080 |
| MLX Embeddings | dev01 | - | 8081 |

## Changes Made

### Code Changes
- `pkg/config/config.go`: Added `SSLMode`, `SSLCert`, `SSLKey`, `SSLRootCert` fields to DatabaseConfig
- Environment variables: `PENFOLD_DB_SSL_MODE`, `PENFOLD_DB_SSL_CERT`, `PENFOLD_DB_SSL_KEY`, `PENFOLD_DB_SSL_ROOT_CERT`

### Gateway Environment (`/etc/penfold/gateway.env` on dev02)
```bash
export PENFOLD_DB_HOST=dev02.brown.chat
export PENFOLD_DB_SSL_MODE=verify-full
export PENFOLD_DB_SSL_CERT=/home/james/.postgresql/postgresql.crt
export PENFOLD_DB_SSL_KEY=/home/james/.postgresql/postgresql.key
export PENFOLD_DB_SSL_ROOT_CERT=/home/james/.postgresql/root.crt
export GATEWAY_AI_SERVICE_ADDR=localhost:50055  # AI Coordinator on same host
export GATEWAY_WORKER_HEALTH_URL=http://dev01.brown.chat:8085
export GATEWAY_EMBEDDINGS_URL=http://dev01.brown.chat:8081
export GATEWAY_LLM_URL=http://dev01.brown.chat:8080
```

## Verification

```bash
curl http://dev02.brown.chat:8080/health
# database: healthy, ai_service: healthy, embeddings: healthy, llm: healthy
```

## Key Insight

Gateway must connect to `dev02.brown.chat:5432` (not `localhost:5432`) for SSL cert auth to work. Connections via localhost go through Docker port mapping and appear as Docker network IPs, bypassing the SSL cert rules.
