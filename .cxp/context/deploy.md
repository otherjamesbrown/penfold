# Deploying

Services are managed by native process managers — launchd on dev01 (macOS), systemd on dev02 (Linux). **Always use deploy scripts. Never hand-roll deploys.**

```bash
./scripts/deploy.sh gateway     # Build + deploy gateway to dev02 (systemd)
./scripts/deploy.sh worker      # Build + deploy worker to dev01 (launchd)
./scripts/deploy.sh ai          # Build + deploy AI coordinator to dev02 (systemd)
./scripts/deploy.sh all         # Deploy all in dependency order
./scripts/deploy.sh status      # Check all services
```

Deploy backs up the current binary before swapping. Failed health checks trigger automatic rollback.

To check service status without deploying:
```bash
ssh dev02 'systemctl status penfold-gateway penfold-ai-coordinator penfold-alert-webhook'
sudo launchctl print system/com.penfold.worker    # on dev01
```

To view logs:
```bash
ssh dev02 'journalctl -u penfold-gateway -f'      # gateway/ai/webhook on dev02
tail -f /var/log/penfold/worker.log                # worker on dev01
```
