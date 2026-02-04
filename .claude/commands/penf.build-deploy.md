# Build and Deploy All Penfold Services

Build all services from source and deploy to dev01/dev02.

## Arguments: $ARGUMENTS

Optional: Specific service to build/deploy (e.g., `gateway`, `worker`, `ai`, `all`)
If not provided, builds and deploys all services.

## Instructions

### Step 1: Load Credentials

```bash
source ~/github/otherjamesbrown/secrets/.env.penfold
```

Verify credentials are loaded:
```bash
if [ -z "$PENFOLD_DB_PASSWORD" ]; then
    echo "PENFOLD_DB_PASSWORD not set — credentials not loaded"
    exit 1
fi
echo "Credentials loaded"
```

### Step 2: Determine What to Build

Parse `$ARGUMENTS` to determine scope:
- `gateway` — Gateway only
- `worker` — Worker only
- `ai` — AI Coordinator only
- `cli` — CLI only
- `all` or empty — All services

### Step 3: Build Services

Run builds from the repo root (`~/github/otherjamesbrown/penfold`). Build all requested services in parallel where possible.

**Gateway (Linux/amd64 for dev02):**
```bash
cd ~/github/otherjamesbrown/penfold
GOOS=linux GOARCH=amd64 go build -o services/gateway/gateway-linux ./services/gateway/
echo "Gateway built: services/gateway/gateway-linux"
```

**Worker (Darwin/arm64 for dev01):**
```bash
cd ~/github/otherjamesbrown/penfold
GOOS=darwin GOARCH=arm64 go build -o services/worker/worker-darwin-arm64 ./services/worker/
echo "Worker built: services/worker/worker-darwin-arm64"
```

**AI Coordinator (Linux/amd64 for dev02):**
```bash
cd ~/github/otherjamesbrown/penfold
GOOS=linux GOARCH=amd64 go build -o services/ai/ai-coordinator-linux ./services/ai/
echo "AI Coordinator built: services/ai/ai-coordinator-linux"
```

**CLI (local arch):**
```bash
cd ~/github/otherjamesbrown/penfold
go build -o cmd/penf/penf ./cmd/penf/
echo "CLI built: cmd/penf/penf"
```

**Note:** This builds the CLI locally but does NOT release it. For a GitHub release (so users can `penf update`), you must:
1. Bump version in `cmd/penf/VERSION`
2. Commit and push to main
3. GitHub Actions `auto-release.yml` automatically creates the release

If any build fails, stop and report the error. Do not proceed to deployment.

### Step 4: Present Build Summary

```
## Build Results

| Service          | Target          | Binary                              | Status |
|------------------|-----------------|-------------------------------------|--------|
| Gateway          | linux/amd64     | services/gateway/gateway-linux      | ...    |
| Worker           | darwin/arm64    | services/worker/worker-darwin-arm64 | ...    |
| AI Coordinator   | linux/amd64     | services/ai/ai-coordinator-linux    | ...    |
| CLI              | local           | cmd/penf/penf                       | ...    |
```

### Step 5: Deploy Services

Deploy using manual commands (the deploy scripts have aggressive smoke tests that may roll back on unrelated pre-existing issues).

**Gateway (dev02):**
```bash
cd ~/github/otherjamesbrown/penfold
scp services/gateway/gateway-linux dev02:/tmp/penfold-gateway
ssh dev02 "sudo systemctl stop penfold-gateway && sudo mv /opt/penfold/bin/penfold-gateway /opt/penfold/bin/penfold-gateway.backup && sudo mv /tmp/penfold-gateway /opt/penfold/bin/penfold-gateway && sudo chmod +x /opt/penfold/bin/penfold-gateway && sudo systemctl start penfold-gateway"
```

**AI Coordinator (dev02):**
```bash
cd ~/github/otherjamesbrown/penfold
scp services/ai/ai-coordinator-linux dev02:/tmp/penfold-ai-coordinator
ssh dev02 "sudo systemctl stop penfold-ai-coordinator && sudo mv /opt/penfold/bin/penfold-ai-coordinator /opt/penfold/bin/penfold-ai-coordinator.backup && sudo mv /tmp/penfold-ai-coordinator /opt/penfold/bin/penfold-ai-coordinator && sudo chmod +x /opt/penfold/bin/penfold-ai-coordinator && sudo systemctl start penfold-ai-coordinator"
```

**Worker (dev01):**
```bash
cd ~/github/otherjamesbrown/penfold
scp services/worker/worker-darwin-arm64 dev01:/tmp/penfold-worker
ssh dev01 "sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist 2>/dev/null; sleep 2; sudo mv /opt/penfold/bin/penfold-worker /opt/penfold/bin/penfold-worker.backup; sudo mv /tmp/penfold-worker /opt/penfold/bin/penfold-worker; sudo chmod +x /opt/penfold/bin/penfold-worker; sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist"
```

**Note:** The deploy scripts (`./scripts/deploy-*.sh`) exist but have aggressive smoke tests that may trigger rollback on pre-existing bugs unrelated to the deployment. Use the manual commands above for reliable deployment.

### Step 6: Post-Deploy Verification

After all deploys complete, run a quick health check:

```bash
# Gateway
curl -s http://dev02.brown.chat:8080/health | jq .

# AI Coordinator
curl -s http://dev02.brown.chat:8090/health | jq .

# Worker
ssh dev01 "curl -s http://localhost:8085/health"

# CLI connectivity
penf status
```

### Step 7: Present Deploy Summary

```
## Deploy Results

| Service          | Host   | Method   | Status |
|------------------|--------|----------|--------|
| Gateway          | dev02  | systemd  | ...    |
| AI Coordinator   | dev02  | systemd  | ...    |
| Worker           | dev01  | launchd  | ...    |

### Health Check

| Service          | Endpoint                              | Status |
|------------------|---------------------------------------|--------|
| Gateway          | http://dev02.brown.chat:8080/health   | ...    |
| AI Coordinator   | http://dev02.brown.chat:8090/health   | ...    |
| Worker           | http://dev01.brown.chat:8085/health   | ...    |
| CLI (penf)       | penf status                           | ...    |
```

### Step 8: Recommendations

- If all healthy: "All services deployed and healthy. Run `/penf.health` for detailed diagnostics or `/test.unit` to start testing."
- If any unhealthy: Show the failing service logs:
  - Gateway: `ssh dev02 "journalctl -u penfold-gateway -n 30 --no-pager"`
  - AI Coordinator: `ssh dev02 "journalctl -u penfold-ai-coordinator -n 30 --no-pager"`
  - Worker: `ssh dev01 "tail -30 /var/log/penfold/worker.log"`
- If deploy failed: "Rollback available — previous binaries saved as .backup on each host."

## Service Locations Reference

| Service          | Host   | Ports                    | Binary Path                              |
|------------------|--------|--------------------------|------------------------------------------|
| Gateway          | dev02  | 50051 (gRPC), 8080 (HTTP)| /opt/penfold/bin/penfold-gateway         |
| AI Coordinator   | dev02  | 50055 (gRPC), 8090 (HTTP)| /opt/penfold/bin/penfold-ai-coordinator  |
| Worker           | dev01  | 8085 (health)            | /opt/penfold/bin/penfold-worker          |
| CLI              | local  | —                        | /usr/local/bin/penf                      |
