# Infrastructure Access

Quick reference for accessing Penfold infrastructure components.

## PostgreSQL

**Location:** home-01 (10.0.10.253) in Docker container `penfold-postgres`

### Direct Access (via SSH + Docker)
```bash
# Interactive psql session
ssh home-01 "docker exec -it penfold-postgres psql -U penfold -d penfold"

# Run a single command
ssh home-01 "docker exec -i penfold-postgres psql -U penfold -d penfold -c 'SELECT COUNT(*) FROM sources'"

# Run a SQL file
ssh home-01 "docker exec -i penfold-postgres psql -U penfold -d penfold" < migrations/001_example.sql
```

### Remote Connection (from dev01)
```bash
# Connection string (password was reset to 'penfold' for simplicity)
DATABASE_URL="postgresql://penfold:penfold@home-01:5432/penfold?sslmode=disable"

# Test connection
psql "postgresql://penfold:penfold@home-01:5432/penfold?sslmode=disable" -c 'SELECT 1'
```

### Common Operations
```bash
# List tables
ssh home-01 "docker exec -i penfold-postgres psql -U penfold -d penfold -c '\dt'"

# Check row counts
ssh home-01 "docker exec -i penfold-postgres psql -U penfold -d penfold -c 'SELECT schemaname, relname, n_live_tup FROM pg_stat_user_tables ORDER BY n_live_tup DESC'"

# Truncate tables (careful!)
ssh home-01 "docker exec -i penfold-postgres psql -U penfold -d penfold -c 'TRUNCATE sources, source_attachments, ingest_jobs, ingest_errors CASCADE'"
```

## Redis

**Location:** home-01 (10.0.10.253) in Docker container `penfold-redis`

### Direct Access
```bash
# Interactive redis-cli
ssh home-01 "docker exec -it penfold-redis redis-cli"

# Run a single command
ssh home-01 "docker exec -i penfold-redis redis-cli PING"
ssh home-01 "docker exec -i penfold-redis redis-cli KEYS '*'"
```

### Remote Connection (from dev01)
```bash
REDIS_HOST="home-01"
REDIS_PORT="6379"
# No password required
```

## Running penf CLI with Infrastructure

```bash
# Full environment for penf commands
DATABASE_URL="postgresql://penfold:penfold@home-01:5432/penfold?sslmode=disable" \
REDIS_HOST="home-01" \
./bin/penf <command>

# Example: ingest emails
DATABASE_URL="postgresql://penfold:penfold@home-01:5432/penfold?sslmode=disable" \
REDIS_HOST="home-01" \
./bin/penf ingest email ~/mnt/nas/akamai-test-data/emails --source=test-run
```

## Temporal (if running)

**Location:** home-01 in Docker container `penfold-temporal`

```bash
# Check Temporal status
ssh home-01 "docker exec -i penfold-temporal tctl cluster health"
```

## Container Status

```bash
# Check all Penfold containers
ssh home-01 "docker ps --filter 'name=penfold'"
```

## NAS Mount

**Location:** dev01 at `~/mnt/nas`

```bash
# Check if mounted
ls ~/mnt/nas

# Remount if needed (credentials in secrets/infrastructure.md)
mount_smbfs '//dev:nbk9UJU8qyw1zdh%21grk@10.0.10.235/dev_nas' ~/mnt/nas
```
