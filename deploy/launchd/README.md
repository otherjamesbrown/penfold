# Penfold launchd Services (macOS)

launchd service files for Penfold services running on dev01 (macOS/Apple Silicon).

## Services

| Service | Binary | Ports |
|---------|--------|-------|
| com.penfold.worker | `/opt/penfold/bin/penfold-worker` | 8085 (HTTP health) |

The worker runs on dev01 because it requires Apple Silicon for MLX-based AI models.

## Installation

```bash
# Run as root on dev01
sudo ./install.sh
```

The installer:
1. Creates directories: `/opt/penfold/bin`, `/var/log/penfold`
2. Copies plist to `/Library/LaunchDaemons/`
3. Creates reference env file in `/etc/penfold/` (for documentation only)

## Directory Layout

After installation:
```
/opt/penfold/
└── bin/
    └── penfold-worker

/Library/LaunchDaemons/
└── com.penfold.worker.plist

/var/log/penfold/
├── worker.log
└── worker.error.log

/etc/penfold/
└── worker.env    # Reference only - env vars are in plist
```

## Service Management

```bash
# Load and start service (auto-starts on load)
sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist

# Stop and unload service
sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist

# Check if loaded
sudo launchctl list | grep penfold

# Check detailed status
sudo launchctl print system/com.penfold.worker
```

## Viewing Logs

Logs are written to files in `/var/log/penfold/`:

```bash
# Follow logs in real-time
tail -f /var/log/penfold/worker.log

# Follow error log
tail -f /var/log/penfold/worker.error.log

# Last 100 lines
tail -100 /var/log/penfold/worker.log

# Search for errors
grep -i error /var/log/penfold/worker.log

# View both logs
tail -f /var/log/penfold/worker.log /var/log/penfold/worker.error.log
```

## Configuration

Environment variables are defined **in the plist file itself**, not in a separate env file.

To modify configuration:

```bash
# Unload service
sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist

# Edit plist
sudo vim /Library/LaunchDaemons/com.penfold.worker.plist

# Reload service
sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist
```

Key environment variables in plist:
- `WORKER_SERVICE_NAME` - Service identifier
- `WORKER_ENVIRONMENT` - dev/staging/prod
- `TEMPORAL_HOST_PORT` - Temporal server address
- `DATABASE_URL` - PostgreSQL connection string
- `AI_SERVICE_ADDR` - AI service gRPC address
- `LANGFUSE_*` - Tracing configuration

## Updating Binaries

```bash
# Build on local machine (must be Apple Silicon for MLX)
cd services/worker
go build -o worker-darwin-arm64 -ldflags="-s -w" .

# Copy to dev01
scp worker-darwin-arm64 james@dev01.brown.chat:/tmp/

# On dev01: replace binary
sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist
sudo mv /tmp/worker-darwin-arm64 /opt/penfold/bin/penfold-worker
sudo chmod +x /opt/penfold/bin/penfold-worker
sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist
```

## Behavior

The plist configures the following behavior:

- **RunAtLoad**: Service starts automatically when loaded
- **KeepAlive**: Restarts on crash (but not clean exit)
- **ThrottleInterval**: 5 seconds between restart attempts
- **HardResourceLimits/SoftResourceLimits**: 65536 file descriptor limit

## Troubleshooting

**Service won't load:**
```bash
# Check plist syntax
plutil /Library/LaunchDaemons/com.penfold.worker.plist

# Check permissions
ls -la /Library/LaunchDaemons/com.penfold.worker.plist
# Should be: -rw-r--r-- root wheel

# Fix permissions if needed
sudo chown root:wheel /Library/LaunchDaemons/com.penfold.worker.plist
sudo chmod 644 /Library/LaunchDaemons/com.penfold.worker.plist
```

**Service keeps crashing:**
```bash
# Check error log
cat /var/log/penfold/worker.error.log

# Check system log for launchd messages
log show --predicate 'subsystem == "com.apple.launchd"' --last 5m | grep penfold

# Verify binary runs manually
/opt/penfold/bin/penfold-worker
```

**Can't connect to external services:**
```bash
# Check network from dev01
curl http://dev02.brown.chat:7233/health  # Temporal
curl http://dev02.brown.chat:8080/health  # Gateway

# Check database connectivity
psql "host=dev02.brown.chat dbname=penfold user=penfold sslmode=verify-full"
```

**Permission errors:**
```bash
# Fix ownership
sudo chown -R james:staff /opt/penfold
sudo chown -R james:staff /var/log/penfold
```
