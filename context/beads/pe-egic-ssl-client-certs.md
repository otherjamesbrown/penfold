# PE-EGIC: PostgreSQL SSL Client Certificate Authentication

**Status:** Complete
**Created:** 2026-01-27
**Completed:** 2026-01-27

## Summary

Set up SSL client certificate authentication for PostgreSQL on dev02, enabling secure connections from dev01 and Go applications without password management.

## What Was Done

### 1. Certificate Generation

Generated CA, server, and client certificates using OpenSSL:

- **CA Certificate** (`root.crt`): Self-signed CA for signing all certs
- **Server Certificate** (`server.crt`): CN=dev02.brown.chat for PostgreSQL
- **Client Certificate** (`postgresql.crt`): CN=penfold for client authentication

### 2. Certificate Deployment

**dev01 (~/.postgresql/):**
- `root.crt` - CA certificate
- `postgresql.crt` - Client certificate
- `postgresql.key` - Client private key (chmod 600)

**dev02 (PostgreSQL container):**
- Server certs in `/home/postgres/pgdata/data/`
- postgresql.conf updated with SSL settings

### 3. pg_hba.conf Configuration

All rules now use SSL certificate authentication:

```
hostssl all all 10.0.10.144/32 cert    # dev01
hostssl all all 10.0.10.251/32 cert    # dev02
hostssl all all 172.16.0.0/12  cert    # Docker networks
hostssl all all 0.0.0.0/0      cert    # External
```

### 4. Temporal SSL Migration

Initially Temporal used password auth due to limitations in `auto-setup` image. This was later resolved in [PE-TMSL](pe-tmsl-temporal-ssl.md) by switching to `temporalio/server` with SSL environment variables.

## Verification

```bash
# From dev01 - SSL cert auth
psql "host=dev02.brown.chat dbname=penfold user=penfold sslmode=verify-full"

# From Temporal container - SSL cert auth
# Verified via pg_stat_ssl showing all connections using TLSv1.3
```

## Files Modified

- `~/infrastructure.md` - Updated with SSL setup status
- `pkg/config/config.go` - Added SSL cert path support
- PostgreSQL pg_hba.conf - SSL-only authentication
- PostgreSQL postgresql.conf - Enabled SSL

## Related

- [PE-TMSL](pe-tmsl-temporal-ssl.md) - Temporal SSL migration (complete)
- [PE-GWCF](pe-gwcf-gateway-config.md) - Gateway SSL configuration (complete)
- `context/infrastructure.md` - Updated documentation
