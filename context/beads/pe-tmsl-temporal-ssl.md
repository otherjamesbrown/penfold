# PE-TMSL: Temporal PostgreSQL SSL Certificate Authentication

**Status:** Complete
**Created:** 2026-01-27
**Completed:** 2026-01-27
**Blocked by:** None

## Problem (Resolved)

The `temporalio/auto-setup:1.24` image had a known issue where it doesn't support PostgreSQL SSL/TLS for the schema setup phase ([GitHub #2293](https://github.com/temporalio/temporal/issues/2293)). This caused Temporal to use password authentication while all other services used SSL certificate authentication.

## Solution

Replaced `auto-setup` with `temporalio/server` and configured SSL via environment variables:

1. ✅ Created schema setup script (`setup-schema.sh`) - Ready but not needed (schema already exists)
2. ✅ Updated docker-compose to use `temporalio/server:1.24.2`
3. ✅ Configured SSL via environment variables in docker-compose
4. ✅ Verified all Temporal connections use SSL with TLSv1.3

## Tasks

- [x] Create schema setup script using temporal-sql-tool with SSL
- [x] Update docker-compose to use temporalio/server with SSL environment variables
- [x] Test Temporal connects with SSL certificates
- [x] Verify Worker can connect to Temporal (port 7233 accessible)
- [x] Create verification script to check SSL status

## Files Created/Modified

All files are on dev02.brown.chat:

- `~/penfold-temporal/docker-compose.yml` - Updated to use temporalio/server:1.24.2 with SSL env vars
- `~/penfold-temporal/setup-schema.sh` - Schema setup script (idempotent, uses temporal-sql-tool with SSL)
- `~/penfold-temporal/verify-ssl.sh` - SSL verification script

## SSL Certificate Paths (in container)

```
/certs/root.crt        - CA certificate
/certs/postgresql.crt  - Client certificate (CN=penfold)
/certs/postgresql.key  - Client private key
```

## Environment Variables Used

The `temporalio/server` image uses these environment variables for SSL configuration:

```yaml
SQL_TLS_ENABLED: true
SQL_CA: /certs/root.crt
SQL_CERT: /certs/postgresql.crt
SQL_CERT_KEY: /certs/postgresql.key
SQL_HOST_VERIFICATION: false
```

These apply to both default and visibility datastores.

## Verification Results

```bash
# Run verification script
cd ~/penfold-temporal && ./verify-ssl.sh

# Results: ✓ All 38 Temporal connections using SSL with TLSv1.3
# - cipher: TLS_AES_256_GCM_SHA384
# - client_addr: 172.18.0.2 (Docker network)
# - databases: temporal, temporal_visibility
```

## Implementation Notes

1. **Environment variables vs config file**: The temporalio/server image uses config_template.yaml which substitutes environment variables. Using env vars is cleaner than mounting custom config files.

2. **Schema setup**: The existing schema from auto-setup works with temporalio/server. The setup-schema.sh script is available for future schema migrations but wasn't needed for the migration.

3. **No password required**: With SSL certificate authentication configured in pg_hba.conf, PostgreSQL validates the client certificate (CN=penfold) instead of requiring a password.

4. **Worker compatibility**: The Worker on dev01 connects to Temporal on port 7233, which remains accessible at dev02.brown.chat:7233.
