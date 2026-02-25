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
├── certs/            # TLS certificates
└── nomad-archived/   # Archived Nomad config (replaced by launchd/systemd)
```

## Service Architecture

| Service | Host | Platform | Init System |
|---------|------|----------|-------------|
| Gateway | dev02.brown.chat | Linux | systemd |
| AI Coordinator | dev02.brown.chat | Linux | systemd |
| Worker | dev01.brown.chat | macOS | launchd |
| Observability | dev02.brown.chat | Docker | docker-compose |
| Langfuse | dev02.brown.chat | Docker | docker-compose |

## Deployment

**Always use deploy scripts. Never hand-roll deploys.**

```bash
./scripts/deploy.sh worker     # Build + deploy worker to dev01 (launchd)
./scripts/deploy.sh gateway    # Build + deploy gateway to dev02 (systemd)
./scripts/deploy.sh ai         # Build + deploy AI coordinator to dev02 (systemd)
./scripts/deploy.sh all        # Deploy all in dependency order
./scripts/deploy.sh status     # Check all services
./scripts/deploy.sh rollback worker  # Rollback to previous binary
```

Deploy scripts automatically:
1. Build the binary with version info (git commit via ldflags)
2. Back up the current binary (`.prev`)
3. Upload and swap the new binary
4. Restart the service via native process manager
5. Verify the deployed version matches expected commit
6. Rollback automatically if verification fails
7. Record the deployment via `penf deploy record`
8. Log to `/var/log/penfold/deploys.log`

## Quick Reference

### Service Management

**dev02 (Linux/systemd):**
```bash
sudo systemctl status penfold-gateway penfold-ai-coordinator
sudo systemctl restart penfold-gateway
journalctl -u penfold-gateway -f
```

**dev01 (macOS/launchd):**
```bash
sudo launchctl print system/com.penfold.worker
sudo launchctl kickstart -k system/com.penfold.worker
tail -f /var/log/penfold/worker.log
```

### Observability Stack

```bash
cd deploy/observability
docker compose up -d
docker compose down

# Grafana: http://dev02.brown.chat:3001
# Prometheus: http://dev02.brown.chat:9090
```

## Installation

### First-time Setup

1. **Install service files:**
   ```bash
   # On dev02 (Linux)
   cd deploy/systemd && sudo ./install.sh

   # On dev01 (macOS)
   cd deploy/launchd && sudo ./install.sh
   ```

2. **Configure environment:**
   - Review `/etc/penfold/*.env` files on each host

3. **Deploy and start:**
   ```bash
   ./scripts/deploy.sh all
   ```

## See Also

- [systemd/README.md](systemd/README.md) - Linux service configuration
- [launchd/README.md](launchd/README.md) - macOS service configuration
- [observability/README.md](observability/README.md) - Monitoring and logging
- [env/README.md](env/README.md) - Environment variables
