# Penfold Health Check

Run comprehensive health checks across all Penfold services and infrastructure.

## Arguments: $ARGUMENTS

Optional: Specific check to run (e.g., `gateway`, `worker`, `ai`, `db`, `temporal`, `all`)
If not provided, runs all health checks.

## Instructions

### Step 1: Infrastructure Checks

Check core infrastructure services first.

**PostgreSQL (dev02):**
```bash
source ~/github/otherjamesbrown/secrets/.env.penfold
psql "host=dev02.brown.chat dbname=penfold user=penfold sslmode=verify-full" -c "SELECT version();" 2>&1
```

If that fails, try without SSL:
```bash
PGPASSWORD="$PENFOLD_DB_PASSWORD" psql -h dev02.brown.chat -U penfold -d penfold -c "SELECT version();" 2>&1
```

**Check migration status:**
```bash
PGPASSWORD="$PENFOLD_DB_PASSWORD" psql -h dev02.brown.chat -U penfold -d penfold -c "SELECT version, applied_at FROM schema_migrations ORDER BY version DESC LIMIT 5;" 2>&1
```

**Temporal (dev02):**
```bash
curl -s http://dev02.brown.chat:8088 > /dev/null 2>&1 && echo "Temporal UI: OK" || echo "Temporal UI: UNREACHABLE"
```

**Redis (dev02):**
```bash
ssh dev02 "redis-cli ping" 2>&1
```

### Step 2: Service Health Endpoints

Hit each service's health endpoint directly.

**Gateway (dev02):**
```bash
echo "--- Gateway Health ---"
curl -s -w "\nHTTP Status: %{http_code}\nResponse Time: %{time_total}s\n" http://dev02.brown.chat:8080/health | jq . 2>/dev/null || echo "UNREACHABLE"

echo "--- Gateway Ready ---"
curl -s -w "\nHTTP Status: %{http_code}\n" http://dev02.brown.chat:8080/ready 2>/dev/null || echo "UNREACHABLE"

echo "--- Gateway Live ---"
curl -s -w "\nHTTP Status: %{http_code}\n" http://dev02.brown.chat:8080/live 2>/dev/null || echo "UNREACHABLE"
```

**AI Coordinator (dev02):**
```bash
echo "--- AI Coordinator Health ---"
curl -s -w "\nHTTP Status: %{http_code}\nResponse Time: %{time_total}s\n" http://dev02.brown.chat:8090/health | jq . 2>/dev/null || echo "UNREACHABLE"
```

**Worker (dev01):**
```bash
echo "--- Worker Health ---"
ssh dev01 "curl -s http://localhost:8085/health" 2>&1 | jq . 2>/dev/null || echo "UNREACHABLE or SSH failed"
```

### Step 3: CLI Connectivity

```bash
echo "--- CLI Status ---"
penf status 2>&1

echo "--- CLI Health ---"
penf health gateway --json 2>&1
```

### Step 4: Service Process Checks

Verify services are actually running.

**dev02 (systemd services):**
```bash
echo "--- dev02 Services ---"
ssh dev02 "sudo systemctl is-active penfold-gateway" 2>&1
ssh dev02 "sudo systemctl is-active penfold-ai-coordinator" 2>&1
```

**dev01 (launchd services):**
```bash
echo "--- dev01 Services ---"
ssh dev01 "sudo launchctl list | grep penfold" 2>&1
```

### Step 5: Recent Errors Check

Scan logs for recent errors (last 5 minutes).

**Gateway logs:**
```bash
echo "--- Gateway Errors (last 5min) ---"
ssh dev02 "journalctl -u penfold-gateway --since '5 minutes ago' -p err --no-pager" 2>&1
```

**AI Coordinator logs:**
```bash
echo "--- AI Coordinator Errors (last 5min) ---"
ssh dev02 "journalctl -u penfold-ai-coordinator --since '5 minutes ago' -p err --no-pager" 2>&1
```

**Worker logs:**
```bash
echo "--- Worker Errors (last 5min) ---"
ssh dev01 "tail -50 /var/log/penfold/worker.error.log 2>/dev/null" 2>&1
```

### Step 6: Database Health

Check key tables and pipeline state.

```bash
source ~/github/otherjamesbrown/secrets/.env.penfold
PGPASSWORD="$PENFOLD_DB_PASSWORD" psql -h dev02.brown.chat -U penfold -d penfold -t -c "
SELECT 'sources' AS tbl, COUNT(*) AS cnt FROM sources
UNION ALL SELECT 'assertions', COUNT(*) FROM assertions
UNION ALL SELECT 'people', COUNT(*) FROM people
UNION ALL SELECT 'embeddings', COUNT(*) FROM embeddings
UNION ALL SELECT 'pipeline_runs', COUNT(*) FROM pipeline_runs
UNION ALL SELECT 'prompt_templates', COUNT(*) FROM prompt_templates
ORDER BY tbl;
" 2>&1
```

**Recent pipeline activity:**
```bash
PGPASSWORD="$PENFOLD_DB_PASSWORD" psql -h dev02.brown.chat -U penfold -d penfold -t -c "
SELECT stage_name, status, COUNT(*) AS cnt,
       MAX(completed_at) AS last_completed
FROM pipeline_runs
GROUP BY stage_name, status
ORDER BY stage_name, status;
" 2>&1
```

### Step 7: Observability Stack (optional)

```bash
echo "--- Grafana ---"
curl -s -o /dev/null -w "%{http_code}" http://dev02.brown.chat:3001 2>/dev/null || echo "UNREACHABLE"

echo "--- Prometheus ---"
curl -s -o /dev/null -w "%{http_code}" http://dev02.brown.chat:9090 2>/dev/null || echo "UNREACHABLE"
```

### Step 8: LLM / MLX Services (dev01)

```bash
echo "--- LLM Server ---"
ssh dev01 "curl -s http://localhost:8080/v1/models" 2>&1 | jq .model 2>/dev/null || echo "LLM server not running"

echo "--- Embedding Server ---"
ssh dev01 "curl -s http://localhost:8081/v1/models" 2>&1 | jq .model 2>/dev/null || echo "Embedding server not running"
```

### Step 9: Present Health Summary

Compile all checks into a single summary table.

```
## Penfold Health Report

### Infrastructure

| Component      | Host   | Status | Details                    |
|----------------|--------|--------|----------------------------|
| PostgreSQL     | dev02  | ...    | Version, migration status  |
| Temporal       | dev02  | ...    | UI reachable               |
| Redis          | dev02  | ...    | PONG response              |

### Services

| Service          | Host   | Health | Process | Response Time |
|------------------|--------|--------|---------|---------------|
| Gateway          | dev02  | ...    | ...     | ...           |
| AI Coordinator   | dev02  | ...    | ...     | ...           |
| Worker           | dev01  | ...    | ...     | ...           |
| CLI (penf)       | local  | ...    | —       | ...           |

### ML Services

| Service          | Host   | Status | Model        |
|------------------|--------|--------|--------------|
| LLM Server       | dev01  | ...    | ...          |
| Embedding Server | dev01  | ...    | ...          |

### Database Counts

| Table            | Count |
|------------------|-------|
| sources          | ...   |
| assertions       | ...   |
| people           | ...   |
| embeddings       | ...   |
| pipeline_runs    | ...   |
| prompt_templates | ...   |

### Recent Errors

(list any errors from the last 5 minutes, or "No recent errors")

### Observability

| Component  | URL                            | Status |
|------------|--------------------------------|--------|
| Grafana    | http://dev02.brown.chat:3001   | ...    |
| Prometheus | http://dev02.brown.chat:9090   | ...    |
```

### Step 10: Recommendations

Based on results:

- If all green: "All systems healthy. Ready for `/test.unit` -> `/test.integration` -> `/test.e2e`."
- If service down: "Service X is down. Check logs with: `ssh devXX 'journalctl -u penfold-X -n 50'`"
- If pending migrations: "Pending migrations found — run `penf migrate up` to apply."
- If LLM not running: "LLM server not running on dev01. Start with: `ssh dev01 'launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist'`"
- If errors found: List the errors and suggest investigation steps.
