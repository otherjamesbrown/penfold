# Penfold launchd Services (macOS)

launchd service files for Penfold services running on dev01 (macOS/Apple Silicon).

## Services

| Service | Binary | Ports |
|---------|--------|-------|
| com.penfold.worker | `/opt/penfold/bin/penfold-worker` | 8085 (HTTP health) |

The worker runs on dev01 because it requires Apple Silicon for ML inference via Ollama.

## Architecture

The plist runs a **wrapper script** (`penfold-worker-start.sh`) that:
1. Sources environment from `/etc/penfold/worker.env`
2. `exec`s the worker binary

This means launchd sends SIGTERM directly to the worker process (no intermediate shell).
The plist sets `ExitTimeOut: 30` — launchd waits 30s for graceful shutdown before SIGKILL.

## Installation

```bash
# Run as root on dev01
sudo ./install.sh
```

The installer:
1. Creates directories: `/opt/penfold/bin`, `/var/log/penfold`
2. Copies plist to `/Library/LaunchDaemons/`
3. Copies wrapper script to `/opt/penfold/bin/`
4. Creates reference env file in `/etc/penfold/`

## Directory Layout

After installation:
```
/opt/penfold/
└── bin/
    ├── penfold-worker
    └── penfold-worker-start.sh

/Library/LaunchDaemons/
└── com.penfold.worker.plist

/var/log/penfold/
├── worker.log
├── worker.error.log
└── deploys.log

/etc/penfold/
└── worker.env
```

## Deployment

Use the deploy scripts — never deploy manually:

```bash
./scripts/deploy.sh worker    # Build, upload, restart, verify
./scripts/deploy.sh status    # Check all services
./scripts/deploy.sh rollback worker  # Restore previous binary
```

## Service Management

```bash
# Restart (preferred — sends SIGTERM then starts fresh)
sudo launchctl kickstart -k system/com.penfold.worker

# Check status
sudo launchctl print system/com.penfold.worker

# Stop
sudo launchctl kill SIGTERM system/com.penfold.worker

# Load/unload
sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist
sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist
```

## Viewing Logs

```bash
tail -f /var/log/penfold/worker.log
tail -f /var/log/penfold/worker.error.log
```

## Configuration

Environment variables are in `/etc/penfold/worker.env`:

```bash
# Edit env vars
sudo vim /etc/penfold/worker.env

# Restart to pick up changes
sudo launchctl kickstart -k system/com.penfold.worker
```

Key variables: `TEMPORAL_HOST_PORT`, `DATABASE_URL`, `AI_SERVICE_ADDR`, `LANGFUSE_*`

## Behavior

- **RunAtLoad**: Service starts automatically when loaded
- **KeepAlive**: Restarts on crash (but not clean exit)
- **ExitTimeOut**: 30s graceful shutdown before SIGKILL
- **ThrottleInterval**: 5s between restart attempts
- **Resource Limits**: 65536 file descriptors

## Troubleshooting

```bash
# Check plist syntax
plutil /Library/LaunchDaemons/com.penfold.worker.plist

# Check permissions (should be root:wheel 644)
ls -la /Library/LaunchDaemons/com.penfold.worker.plist

# Check system log for launchd messages
log show --predicate 'subsystem == "com.apple.launchd"' --last 5m | grep penfold

# Verify binary runs manually
/opt/penfold/bin/penfold-worker-start.sh
```
