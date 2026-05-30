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

### Step 7: Worker Activity Check

Check if the worker is idle while there's pending work. This catches cases where health endpoints pass but no actual processing is happening.

```bash
echo "--- Worker Activity Check ---"
source ~/github/otherjamesbrown/secrets/.env.penfold

# Get seconds since last activity and pending count
RESULT=$(PGPASSWORD="$PENFOLD_DB_PASSWORD" psql -h dev02.brown.chat -U penfold -d penfold -t -c "
SELECT
  COALESCE(EXTRACT(EPOCH FROM (NOW() - MAX(updated_at)))::int, 9999) as idle_seconds,
  COUNT(*) FILTER (WHERE processing_status = 'pending') as pending_count
FROM sources;" 2>&1)

IDLE_SECONDS=$(echo "$RESULT" | awk '{print $1}')
PENDING_COUNT=$(echo "$RESULT" | awk '{print $3}')

# Threshold: 5 minutes (300 seconds) of inactivity with pending items is concerning
if [ "$IDLE_SECONDS" -gt 300 ] && [ "$PENDING_COUNT" -gt 0 ]; then
  IDLE_MINS=$((IDLE_SECONDS / 60))
  echo "⚠️ WORKER IDLE: No activity for ${IDLE_MINS}m while ${PENDING_COUNT} items pending"
  echo "   Recommendation: Run 'penf pipeline kick' to start processing"
else
  echo "✅ Worker activity OK (idle ${IDLE_SECONDS}s, ${PENDING_COUNT} pending)"
fi
```

Also check the worker log timestamp directly:
```bash
# Get last log entry timestamp from worker
LAST_LOG=$(ssh dev01 "tail -1 /var/log/penfold/worker.log 2>/dev/null" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z' | head -1)
if [ -n "$LAST_LOG" ]; then
  echo "   Last worker log: $LAST_LOG"
fi
```

### Step 8: Observability Stack (optional)

Note: Step numbers 9-13 follow below (Process Audit, Stuck Docker, Functional Tests, Summary, Recommendations).

```bash
echo "--- Grafana ---"
curl -s -o /dev/null -w "%{http_code}" http://dev02.brown.chat:3001 2>/dev/null || echo "UNREACHABLE"

echo "--- Prometheus ---"
curl -s -o /dev/null -w "%{http_code}" http://dev02.brown.chat:9090 2>/dev/null || echo "UNREACHABLE"
```

### Step 8: Ollama Service (dev01)

```bash
echo "--- Ollama Service ---"
ssh dev01 "curl -s http://localhost:11434/" 2>&1 | grep -q "Ollama is running" && echo "✅ Ollama running" || echo "❌ Ollama not running"

echo "--- Ollama Models ---"
ssh dev01 "curl -s http://localhost:11434/api/tags" 2>&1 | jq -r '.models[] | select(.name | contains("mxbai-embed-large")) | .name' 2>/dev/null || echo "mxbai-embed-large not found"
```

### Step 9: Process Age Audit

Check for stale or rogue processes that may interfere with services. Flag any penfold-related process running longer than 24 hours.

**dev01 processes:**
```bash
echo "--- dev01 Process Audit ---"
ssh dev01 "ps -eo pid,etime,command | grep -E 'penfold|ollama' | grep -v grep" 2>&1
```

**dev02 processes:**
```bash
echo "--- dev02 Process Audit ---"
ssh dev02 "ps -eo pid,etime,command | grep -E 'penfold-(gateway|ai)' | grep -v grep" 2>&1
```

Parse the `etime` column:
- Format `MM:SS` = minutes (OK)
- Format `HH:MM:SS` = hours (check if > 24h)
- Format `D-HH:MM:SS` = days (⚠️ STALE - flag for review)

Any process with uptime > 1 day should be flagged as potentially stale.

### Step 10: Stuck Docker Processes

Check for orphaned docker processes that may be consuming resources.

```bash
echo "--- Stuck Docker Processes (dev02) ---"
ssh dev02 "ps -eo pid,etime,command | grep 'docker run' | grep -v grep | awk '\$2 ~ /-/ {print \"⚠️ STUCK (\" \$2 \"):\", \$0}'" 2>&1
```

Any docker process running for days (format `D-HH:MM:SS`) is likely stuck and should be killed.

### Step 11: Functional Inference Test

Health endpoints can pass while actual inference fails. Test real inference directly against MLX servers on dev01.

**Embedding test (direct to Ollama on dev01):**
```bash
echo "--- Functional Embedding Test ---"
RESULT=$(ssh dev01 "curl -s -X POST http://localhost:11434/api/embeddings \
  -H 'Content-Type: application/json' \
  -d '{\"model\":\"mxbai-embed-large\",\"prompt\":\"test\"}' \
  --max-time 10" 2>&1)

if echo "$RESULT" | jq -e '.embedding' > /dev/null 2>&1; then
  DIM=$(echo "$RESULT" | jq '.embedding | length')
  echo "✅ Embeddings OK: ${DIM} dimensions"
else
  echo "❌ Embeddings FAILED: $RESULT"
fi
```

Note: The AI Coordinator on dev02 uses gRPC, not HTTP REST. These tests verify the underlying Ollama service is working. If these pass but pipeline processing fails, check the AI Coordinator logs for gRPC errors.

### Step 12: Present Health Summary

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

| Service          | Host   | Status | Model              |
|------------------|--------|--------|--------------------|
| Ollama           | dev01  | ...    | mxbai-embed-large  |

### Functional Tests

| Test             | Status | Details                    |
|------------------|--------|----------------------------|
| LLM Inference    | ...    | Response or error message  |
| Embeddings       | ...    | Dimensions or error        |

### Process Audit

| Host   | Process              | Uptime   | Status |
|--------|----------------------|----------|--------|
| dev01  | penfold-worker       | ...      | ...    |
| dev01  | ollama               | ...      | ...    |
| dev02  | penfold-gateway      | ...      | ...    |
| dev02  | penfold-ai-coord     | ...      | ...    |

Flag any process with uptime > 1 day as ⚠️ STALE for review.
Flag any unexpected penfold process (e.g., /tmp/penfold-*) as ⚠️ ROGUE.

### Stuck Docker Processes

List any docker processes running > 1 day, or "None found".

### Worker Activity

| Metric              | Value  | Status |
|---------------------|--------|--------|
| Time since activity | ...    | ...    |
| Pending items       | ...    | ...    |

Flag ⚠️ IDLE if > 5 minutes since last activity AND pending items > 0.
This catches the case where health endpoints pass but no actual work is being processed.

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

### Step 13: Recommendations

Based on results:

- If all green: "All systems healthy. Ready for `/test.unit` -> `/test.integration` -> `/test.e2e`."
- If service down: "Service X is down. Check logs with: `ssh devXX 'journalctl -u penfold-X -n 50'`"
- If pending migrations: "Pending migrations found — run `penf migrate up` to apply."
- If Ollama not running: "Ollama server not running on dev01. Start with: `ssh dev01 'launchctl load ~/Library/LaunchAgents/com.ollama.serve.plist'`"
- If errors found: List the errors and suggest investigation steps.
- If stale process found: "Stale process detected: `kill <PID>` or investigate why it wasn't restarted."
- If rogue process found (e.g., `/tmp/penfold-*`): "Rogue process detected. Kill with: `ssh devXX 'kill <PID>'` and check why it was started outside normal deployment."
- If stuck docker processes: "Stuck docker processes found. Clean up with: `ssh dev02 'docker ps -a | grep -E \"days ago|weeks ago\" | awk \"{print \\$1}\" | xargs docker rm -f'`"
- If functional inference fails but health passes: "AI Coordinator health passes but inference fails. Check AI coordinator logs: `ssh dev01 'tail -100 /tmp/penfold-ai*.log | grep ERR'` or restart the service."
- If embeddings fail: "Embeddings not working. Check Ollama service: `ssh dev01 'curl -s http://localhost:11434/'`"
- If worker idle with pending items: "Worker idle but items pending. Run `penf pipeline kick` to start processing. If this happens frequently, check if workflows are completing or failing silently."
