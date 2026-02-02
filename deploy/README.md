# Penfold Deployment Configuration

This directory contains all deployment configuration for Penfold services.

## Directory Structure

```
deploy/
├── systemd/          # Linux service files (dev02)
├── launchd/          # macOS service files (dev01)
├── observability/    # Prometheus, Loki, Grafana stack
├── env/              # Environment file templates
├── langfuse/         # AI tracing platform
└── certs/            # TLS certificates
```

## Service Architecture

| Service | Host | Platform | Init System |
|---------|------|----------|-------------|
| Gateway | dev02.brown.chat | Linux | systemd |
| AI Coordinator | dev02.brown.chat | Linux | systemd |
| Worker | dev01.brown.chat | macOS | launchd |
| Observability | dev02.brown.chat | Docker | docker-compose |
| Langfuse | dev02.brown.chat | Docker | docker-compose |

## Quick Reference

### Service Management

**dev02 (Linux/systemd):**
```bash
# Status
sudo systemctl status penfold-gateway penfold-ai-coordinator

# Start/stop
sudo systemctl start penfold-gateway
sudo systemctl stop penfold-gateway
sudo systemctl restart penfold-gateway

# Logs
journalctl -u penfold-gateway -f
journalctl -u penfold-gateway --since "1 hour ago"
```

**dev01 (macOS/launchd):**
```bash
# Status
sudo launchctl list | grep penfold

# Start/stop
sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist
sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist

# Logs
tail -f /var/log/penfold/worker.log
tail -f /var/log/penfold/worker.error.log
```

### Observability Stack

```bash
# Start/stop
cd deploy/observability
docker compose up -d
docker compose down

# Access
# Grafana: http://dev02.brown.chat:3001 (admin/penfold2024)
# Prometheus: http://dev02.brown.chat:9090
# Alertmanager: http://dev02.brown.chat:9094
```

## Installation

### First-time Setup

1. **Install service files:**
   ```bash
   # On dev02 (Linux)
   cd deploy/systemd
   sudo ./install.sh

   # On dev01 (macOS)
   cd deploy/launchd
   sudo ./install.sh
   ```

2. **Copy binaries:**
   ```bash
   # Gateway to dev02
   scp services/gateway/gateway-linux james@dev02:/opt/penfold/bin/penfold-gateway

   # Worker to dev01
   scp services/worker/worker-darwin-arm64 james@dev01:/opt/penfold/bin/penfold-worker
   ```

3. **Configure environment:**
   - Review `/etc/penfold/*.env` files on each host
   - Update credentials and connection strings as needed

4. **Enable and start:**
   ```bash
   # dev02
   sudo systemctl enable penfold-gateway penfold-ai-coordinator
   sudo systemctl start penfold-gateway penfold-ai-coordinator

   # dev01
   sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist
   ```

5. **Start observability:**
   ```bash
   ssh james@dev02.brown.chat
   cd ~/penfold/deploy/observability
   docker compose up -d
   ```

## See Also

- [systemd/README.md](systemd/README.md) - Linux service configuration
- [launchd/README.md](launchd/README.md) - macOS service configuration
- [observability/README.md](observability/README.md) - Monitoring and logging
- [env/README.md](env/README.md) - Environment variables
- [langfuse/README.md](langfuse/README.md) - AI tracing platform
- [context/development/workflows/deployment-checklist.md](../context/development/workflows/deployment-checklist.md) - Deployment procedures
