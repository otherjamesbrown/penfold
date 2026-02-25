# Penfold systemd Services (Linux)

systemd service files for Penfold services running on dev02 (Linux).

## Services

| Service | Binary | Ports |
|---------|--------|-------|
| penfold-gateway | `/opt/penfold/bin/penfold-gateway` | 50051 (gRPC), 8080 (HTTP) |
| penfold-ai-coordinator | `/opt/penfold/bin/penfold-ai-coordinator` | 50055 (gRPC), 8090 (HTTP) |
| penfold-alert-webhook | `/opt/penfold/bin/alert-webhook.py` | (configured in script) |

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

## Deployment

Use the deploy scripts — never deploy manually:

```bash
./scripts/deploy.sh gateway   # Build, upload, restart, verify
./scripts/deploy.sh ai        # Build, upload, restart, verify
./scripts/deploy.sh status    # Check all services
./scripts/deploy.sh rollback gateway  # Restore previous binary
```

## Service Management

```bash
# Enable services to start on boot
sudo systemctl enable penfold-gateway penfold-ai-coordinator

# Start/stop/restart
sudo systemctl start penfold-gateway
sudo systemctl stop penfold-gateway
sudo systemctl restart penfold-gateway

# Check status
sudo systemctl status penfold-gateway penfold-ai-coordinator
```

## Viewing Logs

```bash
journalctl -u penfold-gateway -f
journalctl -u penfold-ai-coordinator -f
journalctl -u penfold-gateway --since "1 hour ago"
journalctl -u penfold-gateway -p err    # Errors only
```

## Configuration

Environment files in `/etc/penfold/`:

```bash
sudo vim /etc/penfold/gateway.env
sudo systemctl restart penfold-gateway
```

## Security Hardening

The service files include:
- `ExecStartPre=/usr/bin/test -x <binary>` — fail fast if binary missing
- `TimeoutStopSec=30` — 30s graceful shutdown
- `NoNewPrivileges=true` — no privilege escalation
- `ProtectSystem=strict` — read-only filesystem except allowed paths
- `ProtectHome=read-only` — home directories read-only
- `PrivateTmp=true` — private /tmp namespace
- `LimitNOFILE=65536` — file descriptor limit

## Troubleshooting

```bash
# Check detailed status
sudo systemctl status penfold-gateway -l

# Check journal for errors
journalctl -u penfold-gateway -n 50 --no-pager

# Verify binary exists and is executable
ls -la /opt/penfold/bin/penfold-gateway

# After editing service file
sudo systemctl daemon-reload
sudo systemctl restart penfold-gateway
```
