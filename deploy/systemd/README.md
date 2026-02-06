# Penfold systemd Services (Linux)

systemd service files for Penfold services running on dev02 (Linux).

## Services

| Service | Binary | Ports |
|---------|--------|-------|
| penfold-gateway | `/opt/penfold/bin/penfold-gateway` | 50051 (gRPC), 8080 (HTTP) |
| penfold-ai-coordinator | `/opt/penfold/bin/penfold-ai-coordinator` | 50055 (gRPC), 8090 (HTTP) |

## Installation

```bash
# Run as root on dev02
sudo ./install.sh              # Install all services
sudo ./install.sh gateway      # Install gateway only
sudo ./install.sh ai           # Install AI coordinator only
```

The installer:
1. Creates directories: `/opt/penfold/bin`, `/var/lib/penfold`, `/var/log/penfold`, `/etc/penfold`
2. Copies service files to `/etc/systemd/system/`
3. Creates template env files in `/etc/penfold/` (if they don't exist)

## Directory Layout

After installation:
```
/opt/penfold/
└── bin/
    ├── penfold-gateway
    └── penfold-ai-coordinator

/etc/penfold/
├── gateway.env           # Gateway configuration
└── ai-coordinator.env    # AI coordinator configuration

/var/lib/penfold/         # Service data
/var/log/penfold/         # Additional logs

/etc/systemd/system/
├── penfold-gateway.service
└── penfold-ai-coordinator.service
```

## Service Management

```bash
# Enable services to start on boot
sudo systemctl enable penfold-gateway penfold-ai-coordinator

# Start services
sudo systemctl start penfold-gateway
sudo systemctl start penfold-ai-coordinator

# Stop services
sudo systemctl stop penfold-gateway

# Restart (after config changes)
sudo systemctl restart penfold-gateway

# Check status
sudo systemctl status penfold-gateway
sudo systemctl status penfold-ai-coordinator

# Disable auto-start
sudo systemctl disable penfold-gateway
```

## Viewing Logs

Logs are sent to the systemd journal:

```bash
# Follow logs in real-time
journalctl -u penfold-gateway -f
journalctl -u penfold-ai-coordinator -f

# Last 100 lines
journalctl -u penfold-gateway -n 100

# Since last boot
journalctl -u penfold-gateway -b

# Time-based
journalctl -u penfold-gateway --since "1 hour ago"
journalctl -u penfold-gateway --since "2026-02-01 10:00:00" --until "2026-02-01 12:00:00"

# Filter by priority
journalctl -u penfold-gateway -p err    # Errors only
journalctl -u penfold-gateway -p warning

# Export to file
journalctl -u penfold-gateway --no-pager > gateway.log
```

## Configuration

Environment files in `/etc/penfold/`:

**gateway.env:**
```bash
PENFOLD_SERVICE_NAME=gateway
PENFOLD_ENVIRONMENT=dev
PENFOLD_DB_HOST=localhost
PENFOLD_DB_PORT=5432
# ... see deploy/env/gateway.env for full template
```

After editing configuration:
```bash
sudo systemctl restart penfold-gateway
```

## Updating Binaries

> **Note:** systemd is superseded by Nomad. Use `./scripts/deploy-gateway.sh` instead,
> which handles build (with git version via ldflags), upload, and Nomad job submission.

```bash
# Preferred: use the deploy script
./scripts/deploy-gateway.sh

# Manual alternative (legacy):
cd services/gateway
GOOS=linux GOARCH=amd64 go build -o gateway-linux -ldflags="-s -w" .
scp gateway-linux james@dev02.brown.chat:/tmp/
# On dev02:
sudo systemctl stop penfold-gateway
sudo mv /tmp/gateway-linux /opt/penfold/bin/penfold-gateway
sudo chmod +x /opt/penfold/bin/penfold-gateway
sudo systemctl start penfold-gateway
```

## Security Hardening

The service files include security hardening options:

- `NoNewPrivileges=true` - Prevents privilege escalation
- `ProtectSystem=strict` - Filesystem is read-only except allowed paths
- `ProtectHome=read-only` - Home directories read-only
- `PrivateTmp=true` - Private /tmp namespace
- `ReadWritePaths=/var/lib/penfold /var/log/penfold` - Only allowed write paths
- `LimitNOFILE=65536` - File descriptor limit

## Troubleshooting

**Service won't start:**
```bash
# Check detailed status
sudo systemctl status penfold-gateway -l

# Check journal for errors
journalctl -u penfold-gateway -n 50 --no-pager

# Verify binary exists and is executable
ls -la /opt/penfold/bin/penfold-gateway

# Verify env file
cat /etc/penfold/gateway.env
```

**Permission errors:**
```bash
# Fix ownership
sudo chown -R james:james /opt/penfold
sudo chown -R james:james /var/lib/penfold
sudo chown -R james:james /var/log/penfold

# Check env file permissions (should be 600)
sudo chmod 600 /etc/penfold/*.env
```

**After editing service file:**
```bash
sudo systemctl daemon-reload
sudo systemctl restart penfold-gateway
```
