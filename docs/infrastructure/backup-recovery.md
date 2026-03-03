# Backup and Recovery Procedures

> **Environment**: Mac Mini M4 (32GB RAM) Development Server
> **Last Updated**: 2026-01-15
> **Version**: 1.0

## Overview

This document provides comprehensive backup and recovery procedures for the Penfold personal AI information system. Given the local-first architecture and the irreplaceable nature of personal information (emails, meetings, documents), robust backup procedures are essential.

---

## Data Inventory

### Critical Data Categories

| Category | Location | Size Estimate | Backup Priority |
|----------|----------|---------------|-----------------|
| PostgreSQL Database | Docker volume `postgres_data` | ~90MB/year structured + vectors | **P0 - Critical** |
| Uploaded Files | `/app/uploads` or configured `UPLOAD_DIR` | ~165GB/year (meetings, docs) | **P0 - Critical** |
| Configuration | `.env`, secrets directory | <1MB | **P0 - Critical** |
| Redis Cache | Docker volume `redis_data` | ~100MB | P2 - Reconstructable |
| AI Model Weights | Ollama models directory | ~50GB | P2 - Redownloadable |
| Application Code | Git repository | N/A | P3 - Version controlled |

### PostgreSQL Database Contents

The database contains:
- **Tenant data**: Multi-tenant isolation configuration
- **Source content**: Raw emails, meeting transcripts, documents
- **Assertions**: AI-extracted decisions, risks, commitments, milestones
- **People**: Canonical person records with cross-tenant linking
- **Projects**: Hierarchical project structure with timelines
- **Embeddings**: 1024-dimensional vectors (mxbai-embed-large compatible)
- **Processing state**: Events, jobs, subscriptions, results
- **Gmail connections**: OAuth credentials (encrypted), sync state

### File Storage Contents

Configured via environment variables:
- `UPLOAD_DIR`: Raw uploaded files (audio, video, documents)
- `PROCESSED_DIR`: Processed/transcribed content
- `ENCRYPTION_KEY_PATH`: Encryption keys for credentials

---

## Recovery Objectives

### Definitions

| Metric | Target | Description |
|--------|--------|-------------|
| **RTO** (Recovery Time Objective) | **4 hours** | Maximum time to restore full functionality |
| **RPO** (Recovery Point Objective) | **24 hours** | Maximum acceptable data loss window |

### Rationale

- **RTO of 4 hours**: Personal development system; 4-hour recovery allows same-day restoration without business-critical urgency.
- **RPO of 24 hours**: Daily sync with external sources (Gmail, Slack) means most data can be re-fetched; local-only content (meeting notes, manual entries) captured in nightly backups.

---

## PostgreSQL Backup Strategy

### Option 1: pg_dump (Recommended for Daily Backups)

Creates logical backups suitable for point-in-time snapshots.

#### Backup Script

```bash
#!/bin/zsh
# backup-postgres.sh - PostgreSQL daily backup

set -euo pipefail

# Configuration
BACKUP_DIR="${BACKUP_DIR:-/Users/james/backups/penfold/postgres}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/penfold_${TIMESTAMP}.sql.gz"
RETENTION_DAYS=30

# Ensure backup directory exists
mkdir -p "${BACKUP_DIR}"

# Perform backup (assumes Docker container named 'postgres')
docker exec penfold-postgres-1 pg_dump \
    -U postgres \
    -d penfold_dev \
    --verbose \
    --format=plain \
    --no-owner \
    --no-privileges \
    2>> "${BACKUP_DIR}/backup.log" \
    | gzip > "${BACKUP_FILE}"

# Verify backup
if [[ -s "${BACKUP_FILE}" ]]; then
    BACKUP_SIZE=$(du -h "${BACKUP_FILE}" | cut -f1)
    echo "[$(date)] SUCCESS: Backup created ${BACKUP_FILE} (${BACKUP_SIZE})" >> "${BACKUP_DIR}/backup.log"
else
    echo "[$(date)] ERROR: Backup file is empty or missing" >> "${BACKUP_DIR}/backup.log"
    exit 1
fi

# Cleanup old backups
find "${BACKUP_DIR}" -name "penfold_*.sql.gz" -mtime +${RETENTION_DAYS} -delete
echo "[$(date)] Cleaned up backups older than ${RETENTION_DAYS} days" >> "${BACKUP_DIR}/backup.log"
```

#### Scheduling with launchd

Create `/Library/LaunchDaemons/com.penfold.backup.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.penfold.backup</string>
    <key>ProgramArguments</key>
    <array>
        <string>/Users/james/scripts/backup-postgres.sh</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key>
        <integer>2</integer>
        <key>Minute</key>
        <integer>0</integer>
    </dict>
    <key>StandardOutPath</key>
    <string>/Users/james/backups/penfold/launchd-stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/james/backups/penfold/launchd-stderr.log</string>
</dict>
</plist>
```

Load with:
```bash
sudo launchctl load /Library/LaunchDaemons/com.penfold.backup.plist
```

### Option 2: WAL Archiving (Continuous Backup)

For near-zero RPO, enable PostgreSQL Write-Ahead Log archiving.

#### PostgreSQL Configuration

Add to `postgresql.conf` (or via Docker environment):

```ini
# WAL archiving for point-in-time recovery
wal_level = replica
archive_mode = on
archive_command = 'cp %p /Users/james/backups/penfold/wal/%f'
archive_timeout = 300  # Archive every 5 minutes if no activity
```

#### Base Backup Script

```bash
#!/bin/zsh
# base-backup.sh - Create base backup for WAL recovery

BACKUP_DIR="/Users/james/backups/penfold/base"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "${BACKUP_DIR}"

docker exec penfold-postgres-1 pg_basebackup \
    -U postgres \
    -D /tmp/backup \
    -Ft \
    -z \
    -P

docker cp penfold-postgres-1:/tmp/backup "${BACKUP_DIR}/base_${TIMESTAMP}"
docker exec penfold-postgres-1 rm -rf /tmp/backup

echo "[$(date)] Base backup created: base_${TIMESTAMP}"
```

Run weekly to create recovery checkpoints.

### Retention Policy

| Backup Type | Frequency | Retention |
|-------------|-----------|-----------|
| Daily pg_dump | 02:00 daily | 30 days |
| Weekly base backup | Sunday 03:00 | 4 weeks |
| Monthly archive | 1st of month | 12 months |
| WAL segments | Continuous | 7 days |

---

## File Storage Backup

### Uploaded Files Backup

```bash
#!/bin/zsh
# backup-files.sh - Backup uploaded files

set -euo pipefail

SOURCE_DIR="${UPLOAD_DIR:-/Users/james/penfold-uploads}"
BACKUP_DIR="/Users/james/backups/penfold/files"
TIMESTAMP=$(date +%Y%m%d)

# Use rsync for incremental backup
rsync -avz --delete \
    --exclude='*.tmp' \
    --exclude='.DS_Store' \
    "${SOURCE_DIR}/" \
    "${BACKUP_DIR}/current/"

# Create daily snapshot (hardlinks for space efficiency)
SNAPSHOT_DIR="${BACKUP_DIR}/snapshots/${TIMESTAMP}"
if [[ ! -d "${SNAPSHOT_DIR}" ]]; then
    cp -al "${BACKUP_DIR}/current" "${SNAPSHOT_DIR}"
    echo "[$(date)] Snapshot created: ${SNAPSHOT_DIR}"
fi

# Cleanup snapshots older than 30 days
find "${BACKUP_DIR}/snapshots" -maxdepth 1 -type d -mtime +30 -exec rm -rf {} \;
```

### Network Storage Backup

For the network storage server mentioned in infrastructure sizing:

```bash
#!/bin/zsh
# backup-to-nas.sh - Sync backups to network storage

NAS_MOUNT="/mnt/penfold-archive"
LOCAL_BACKUP="/Users/james/backups/penfold"

# Ensure NAS is mounted
if ! mount | grep -q "${NAS_MOUNT}"; then
    echo "ERROR: NAS not mounted at ${NAS_MOUNT}"
    exit 1
fi

# Sync all backups to NAS
rsync -avz --delete \
    "${LOCAL_BACKUP}/" \
    "${NAS_MOUNT}/backups/"

echo "[$(date)] Synced backups to NAS"
```

---

## Redis Persistence Configuration

Redis data is generally reconstructable (cache/events), but persisting it avoids reprocessing overhead.

### docker-compose.yml Redis Configuration

```yaml
redis:
  image: redis:7-alpine
  ports:
    - "6379:6379"
  volumes:
    - redis_data:/data
  command: >
    redis-server
    --appendonly yes
    --appendfsync everysec
    --save 900 1
    --save 300 10
    --save 60 10000
```

### Configuration Explanation

- `appendonly yes`: Enable AOF persistence
- `appendfsync everysec`: Sync to disk every second (balance of safety/performance)
- `save` directives: Create RDB snapshots at intervals

---

## Configuration and Secrets Backup

### Encrypted Secrets Backup

```bash
#!/bin/zsh
# backup-config.sh - Backup configuration and encrypted secrets

SECRETS_DIR="${HOME}/github/otherjamesbrown/secrets"
CONFIG_FILES=(
    "${HOME}/github/otherjamesbrown/penfold/.env"
    "${HOME}/.penfold/encryption-keys"
)
BACKUP_DIR="/Users/james/backups/penfold/config"
TIMESTAMP=$(date +%Y%m%d)

mkdir -p "${BACKUP_DIR}"

# Create encrypted archive of secrets
tar -czf - "${SECRETS_DIR}" 2>/dev/null | \
    gpg --symmetric --cipher-algo AES256 \
    > "${BACKUP_DIR}/secrets_${TIMESTAMP}.tar.gz.gpg"

# Backup configuration files (without secrets values)
for config in "${CONFIG_FILES[@]}"; do
    if [[ -f "${config}" ]]; then
        cp "${config}" "${BACKUP_DIR}/$(basename ${config}).${TIMESTAMP}"
    fi
done

echo "[$(date)] Configuration backup completed"
```

### Important Notes

- **Never commit** `.env` or secrets to git
- Store GPG passphrase separately (password manager, physical safe)
- Test decryption periodically:
  ```bash
  gpg --decrypt secrets_YYYYMMDD.tar.gz.gpg | tar -tzf -
  ```

---

## Recovery Procedures

### Scenario 1: Complete System Recovery

**Situation**: Mac Mini failure, need to restore to new hardware or fresh install.

#### Step 1: Prepare Environment

```bash
# Install Docker Desktop for Mac
# Install Homebrew and dependencies

brew install postgresql
brew install python@3.12
pip install -r requirements.txt
```

#### Step 2: Restore Docker Volumes

```bash
# Start containers to create volumes
docker compose up -d postgres redis
docker compose stop postgres redis

# Restore PostgreSQL data
docker run --rm \
    -v penfold_postgres_data:/target \
    -v /Users/james/backups/penfold/postgres:/backup \
    alpine sh -c "cd /target && gunzip -c /backup/penfold_LATEST.sql.gz > /restore.sql"

# Import backup
docker compose up -d postgres
docker exec -i penfold-postgres-1 psql -U postgres -d penfold_dev < /restore.sql
```

#### Step 3: Restore Files

```bash
# Restore uploaded files
rsync -avz /Users/james/backups/penfold/files/current/ \
    ${UPLOAD_DIR:-/Users/james/penfold-uploads}/

# Restore configuration
cp /Users/james/backups/penfold/config/.env.LATEST \
    ${HOME}/github/otherjamesbrown/penfold/.env
```

#### Step 4: Restore Secrets

```bash
# Decrypt secrets archive (will prompt for passphrase)
gpg --decrypt /Users/james/backups/penfold/config/secrets_LATEST.tar.gz.gpg | \
    tar -xzf - -C ${HOME}/github/otherjamesbrown/
```

#### Step 5: Verify Recovery

```bash
# Run migrations
alembic upgrade head

# Test database connectivity
pytest tests/unit/storage/ -v --maxfail=3

# Verify data integrity
docker exec penfold-postgres-1 psql -U postgres -d penfold_dev \
    -c "SELECT COUNT(*) FROM sources;"
```

### Scenario 2: PostgreSQL Point-in-Time Recovery

**Situation**: Need to recover to specific point in time (e.g., before accidental deletion).

#### Prerequisites

- Base backup exists
- WAL archives available for target time

#### Recovery Steps

```bash
# Stop PostgreSQL
docker compose stop postgres

# Clear current data
docker run --rm -v penfold_postgres_data:/data alpine rm -rf /data/*

# Restore base backup
docker run --rm \
    -v penfold_postgres_data:/data \
    -v /Users/james/backups/penfold/base/base_YYYYMMDD:/backup \
    alpine tar -xzf /backup/base.tar.gz -C /data

# Create recovery.conf
cat > /tmp/recovery.conf << 'EOF'
restore_command = 'cp /wal_archive/%f %p'
recovery_target_time = '2026-01-15 14:30:00'
recovery_target_action = 'promote'
EOF

# Copy recovery.conf and WAL archives
docker run --rm \
    -v penfold_postgres_data:/data \
    -v /Users/james/backups/penfold/wal:/wal_archive \
    -v /tmp/recovery.conf:/recovery.conf \
    alpine cp /recovery.conf /data/

# Start PostgreSQL (will replay WAL to target time)
docker compose up -d postgres
```

### Scenario 3: Single Table Recovery

**Situation**: Accidentally deleted or corrupted specific table data.

```bash
# Extract specific table from backup
docker exec penfold-postgres-1 pg_dump \
    -U postgres -d penfold_dev \
    -t sources \
    --data-only \
    > sources_current_backup.sql

# Restore from backup (example: restore sources table)
gunzip -c /Users/james/backups/penfold/postgres/penfold_YYYYMMDD.sql.gz | \
    grep -A 10000 "COPY public.sources" | \
    grep -B 10000 "^\\\\\." | \
    docker exec -i penfold-postgres-1 psql -U postgres -d penfold_dev
```

### Scenario 4: Gmail Connection Recovery

**Situation**: OAuth tokens expired or corrupted; need to re-establish Gmail sync.

The system includes automated recovery procedures in `penf_lib/connectors/gmail/recovery.py`:

```python
from penf_lib.connectors.gmail.recovery import system_recovery

# Run automated recovery
results = await system_recovery.run_automated_recovery(
    fix_tokens=True,
    fix_sync=True,
    fix_webhooks=True
)

# Check health status
health = await system_recovery.perform_health_check()
print(health)
```

Manual CLI recovery:

```bash
# Check connection status
penf gmail status

# Re-authenticate if needed
penf gmail auth --account work@example.com

# Trigger resync if data gaps detected
penf gmail sync --full --account work@example.com
```

---

## Disaster Recovery Testing

### Monthly DR Test Procedure

#### Test 1: Backup Integrity Verification

```bash
#!/bin/zsh
# test-backup-integrity.sh

BACKUP_FILE=$(ls -t /Users/james/backups/penfold/postgres/*.sql.gz | head -1)

# Test decompression
gunzip -t "${BACKUP_FILE}"
if [[ $? -eq 0 ]]; then
    echo "PASS: Backup file integrity verified"
else
    echo "FAIL: Backup file corrupted"
    exit 1
fi

# Test restore to temporary database
docker exec penfold-postgres-1 createdb -U postgres penfold_test
gunzip -c "${BACKUP_FILE}" | \
    docker exec -i penfold-postgres-1 psql -U postgres -d penfold_test

# Verify record counts
SOURCES=$(docker exec penfold-postgres-1 psql -U postgres -d penfold_test \
    -t -c "SELECT COUNT(*) FROM sources;")
echo "Sources restored: ${SOURCES}"

# Cleanup
docker exec penfold-postgres-1 dropdb -U postgres penfold_test
```

#### Test 2: Full Recovery Simulation

Quarterly, perform full recovery test:

1. Spin up fresh Docker environment
2. Restore from backups
3. Verify application functionality
4. Run integration tests against restored data
5. Document recovery time and any issues

#### Test 3: Secrets Recovery

```bash
# Test secrets decryption (without extracting to disk)
gpg --decrypt /Users/james/backups/penfold/config/secrets_LATEST.tar.gz.gpg | \
    tar -tzf -

# Verify expected files present
# Expected: credentials.json, api_keys/, etc.
```

### DR Test Checklist

- [ ] Backup file exists and is recent (< 24 hours)
- [ ] Backup decompresses without error
- [ ] Database restore completes successfully
- [ ] Application starts after restore
- [ ] Sample queries return expected data
- [ ] Secrets can be decrypted
- [ ] File storage backup is current
- [ ] Network storage sync is working
- [ ] Recovery time documented

---

## Monitoring and Alerting

### Backup Health Checks

```bash
#!/bin/zsh
# check-backup-health.sh - Run as daily cron job

BACKUP_DIR="/Users/james/backups/penfold"
ALERT_EMAIL="james@example.com"
MAX_AGE_HOURS=26  # Alert if backup older than 26 hours

# Check PostgreSQL backup age
LATEST_BACKUP=$(ls -t "${BACKUP_DIR}/postgres/"*.sql.gz 2>/dev/null | head -1)
if [[ -z "${LATEST_BACKUP}" ]]; then
    echo "ALERT: No PostgreSQL backup found" | mail -s "Penfold Backup Alert" "${ALERT_EMAIL}"
    exit 1
fi

BACKUP_AGE=$(( ($(date +%s) - $(stat -f %m "${LATEST_BACKUP}")) / 3600 ))
if [[ ${BACKUP_AGE} -gt ${MAX_AGE_HOURS} ]]; then
    echo "ALERT: PostgreSQL backup is ${BACKUP_AGE} hours old" | mail -s "Penfold Backup Alert" "${ALERT_EMAIL}"
fi

# Check backup size (detect potential corruption)
BACKUP_SIZE=$(stat -f %z "${LATEST_BACKUP}")
if [[ ${BACKUP_SIZE} -lt 1000 ]]; then
    echo "ALERT: PostgreSQL backup suspiciously small (${BACKUP_SIZE} bytes)" | mail -s "Penfold Backup Alert" "${ALERT_EMAIL}"
fi

echo "[$(date)] Backup health check completed - all OK"
```

### Key Metrics to Monitor

| Metric | Threshold | Action |
|--------|-----------|--------|
| Backup age | > 26 hours | Alert and investigate |
| Backup size change | > 50% variance | Review for issues |
| WAL archive lag | > 1 hour | Check archive_command |
| Disk space | < 20% free | Expand or cleanup |
| Recovery test age | > 30 days | Schedule test |

---

## Quick Reference

### Daily Operations

```bash
# Check backup status
ls -la /Users/james/backups/penfold/postgres/ | tail -5

# Manual backup
/Users/james/scripts/backup-postgres.sh

# Check backup log
tail -20 /Users/james/backups/penfold/postgres/backup.log
```

### Emergency Recovery

```bash
# Quick database restore (most recent backup)
LATEST=$(ls -t /Users/james/backups/penfold/postgres/*.sql.gz | head -1)
docker compose stop postgres
docker compose up -d postgres
gunzip -c "${LATEST}" | docker exec -i penfold-postgres-1 psql -U postgres -d penfold_dev
```

### Contact Information

- **System Owner**: James
- **Backup Location (Local)**: `/Users/james/backups/penfold/`
- **Backup Location (NAS)**: `/mnt/penfold-archive/backups/`
- **Secrets Recovery**: GPG passphrase in password manager

---

## Appendix: Backup Script Collection

All backup scripts should be stored in `/Users/james/scripts/` with the following structure:

```
/Users/james/scripts/
  backup-postgres.sh      # Daily PostgreSQL backup
  backup-files.sh         # File storage backup
  backup-config.sh        # Configuration/secrets backup
  backup-to-nas.sh        # NAS synchronization
  base-backup.sh          # Weekly base backup for PITR
  check-backup-health.sh  # Health monitoring
  test-backup-integrity.sh # Monthly integrity test
```

Make scripts executable:
```bash
chmod +x /Users/james/scripts/backup-*.sh
chmod +x /Users/james/scripts/check-*.sh
chmod +x /Users/james/scripts/test-*.sh
```
