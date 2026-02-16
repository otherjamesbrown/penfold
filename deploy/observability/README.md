# Penfold Observability Stack

Prometheus, Loki, Grafana, and Alertmanager stack for monitoring Penfold services.

## Components

| Component | Port | Purpose |
|-----------|------|---------|
| Prometheus | 9090 | Metrics collection and alerting |
| Loki | 3100 | Log aggregation |
| Promtail | 9080 | Log shipping agent |
| Grafana | 3001 | Dashboards and visualization |
| Alertmanager | 9094 | Alert routing and notifications |
| Alert Webhook | 9095 | Converts alerts to CXP agent messages (Nomad job, not Docker) |

## Quick Start

```bash
# On dev02
cd deploy/observability
docker compose up -d

# Check status
docker compose ps

# View logs
docker compose logs -f
```

## Access

| Service | URL | Credentials |
|---------|-----|-------------|
| Grafana | http://dev02.brown.chat:3001 | admin / penfold2024 |
| Prometheus | http://dev02.brown.chat:9090 | - |
| Alertmanager | http://dev02.brown.chat:9094 | - |

## Directory Structure

```
observability/
├── docker-compose.yml        # Main compose file
├── prometheus/
│   ├── prometheus.yml        # Scrape configuration
│   └── rules/
│       └── penfold.yml       # Alert rules
├── loki/
│   └── config.yml            # Loki configuration
├── promtail/
│   └── config.yml            # Log collection config
├── grafana/
│   └── provisioning/         # Datasources and dashboards
└── alertmanager/
    └── alertmanager.yml      # Alert routing
```

## Monitored Services

Prometheus scrapes metrics from (13 targets):

**Penfold Services:**
- Gateway (dev02:8080/metrics)
- AI Coordinator (dev02:8090/metrics)
- Worker (dev01:8085/metrics)

**Infrastructure:**
- Temporal Server (dev02:8233/metrics)
- PostgreSQL (via postgres_exporter on dev02:9187)
- Nomad (dev02:4646/v1/metrics — allocation, node, and server metrics)

**Ollama (dev01):**
- Ollama service health via Gateway (dev01:11434/)

**Node Exporters:**
- dev02 (localhost:9100)
- dev01 (dev01:9100)

## Alert Rules

Alerts are defined in `prometheus/rules/penfold.yml`:

### Critical Alerts
- `PenfoldGatewayDown` - Gateway unreachable for 1 minute
- `PenfoldAICoordinatorDown` - AI Coordinator unreachable for 1 minute
- `PenfoldWorkerDown` - Worker unreachable for 2 minutes
- `DiskSpaceLow` - Disk space below 10%

### Warning Alerts
- `HighErrorRate` - Error rate > 0.1/sec for 5 minutes
- `HighGoMemoryUsage` - Go heap > 500MB for 10 minutes
- `TooManyGoroutines` - Goroutine count > 1000 for 5 minutes
- `DiskSpaceWarning` - Disk space below 20%
- `HighMemoryUsage` - System memory > 90%
- `HighCPUUsage` - CPU > 80% for 10 minutes

### Nomad Alerts
- `NomadJobNotRunning` - Job has no running or queued allocations
- `NomadJobFailed` - Job has failed allocations
- `NomadAllocRestarts` - Allocation restarted > 2 times in an hour
- `NomadPendingAllocations` - Allocations stuck in queue > 10 minutes

### Observability Alerts
- `PrometheusTargetDown` - Any scrape target down
- `LokiNotReceivingLogs` - No logs received for 10 minutes

## Using Grafana

### Dashboards

| Dashboard | UID | Description |
|-----------|-----|-------------|
| Penfold Overview | `penfold-overview` | Service health, request rates, latency, connections |
| Nomad Cluster | `nomad-cluster` | Job status, allocation health, RPC, resources |
| Temporal Queues | `temporal-queues` | Temporal workflow queues |
| vLLM-MLX | `vllm-mlx` | MLX model server metrics |

### Pre-configured Datasources

1. **Prometheus** - Metrics queries
2. **Loki** - Log queries

### Querying Metrics (Prometheus)

```promql
# Service up/down
up{job="penfold-gateway"}

# Request rate
rate(http_requests_total{job="penfold-gateway"}[5m])

# Error rate
rate(penfold_errors_total[5m])

# Memory usage
go_memstats_alloc_bytes{job=~"penfold-.*"}

# Goroutines
go_goroutines{job=~"penfold-.*"}
```

### Querying Logs (Loki)

```logql
# Gateway logs
{job="penfold-gateway"}

# Error logs only
{job="penfold-gateway"} |= "error"

# JSON parsing
{job="penfold-gateway"} | json | level="error"

# Filter by time
{job="penfold-gateway"} |= "error" | json | duration > 1s
```

## Management Commands

```bash
# Start all services
docker compose up -d

# Stop all services
docker compose down

# Restart a single service
docker compose restart prometheus

# View logs
docker compose logs -f prometheus
docker compose logs -f grafana

# Reload Prometheus config (without restart)
curl -X POST http://localhost:9090/-/reload

# Check Prometheus targets
curl http://localhost:9090/api/v1/targets | jq .

# Check active alerts
curl http://localhost:9090/api/v1/alerts | jq .

# Update images
docker compose pull
docker compose up -d
```

## Data Persistence

Data is stored in Docker volumes:
- `prometheus_data` - Metrics (30-day retention)
- `loki_data` - Logs
- `grafana_data` - Dashboards and settings
- `alertmanager_data` - Alert state

**Backup:**
```bash
# Create backup directory
mkdir -p ~/backups/observability

# Backup Grafana (dashboards, datasources)
docker cp grafana:/var/lib/grafana ~/backups/observability/grafana-data

# Backup Prometheus (metrics)
docker cp prometheus:/prometheus ~/backups/observability/prometheus-data
```

## Alert Webhook Bridge

Alerts are delivered to the Penfold agent inbox via a lightweight webhook receiver:

- **Script:** `/opt/penfold/bin/alert-webhook.py` (Python, stdlib only)
- **Port:** 9095
- **Nomad job:** `penfold-alert-webhook`
- **Flow:** Alertmanager POST -> webhook -> `cxp message send agent-penfold`

Alerts appear as messages in the agent inbox with subjects like:
- `[FIRING] PenfoldGatewayDown - critical`
- `[RESOLVED] HighCPUUsage - warning`

## Adding New Scrape Targets

Edit `prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'new-service'
    static_configs:
      - targets: ['hostname:port']
    metrics_path: /metrics
```

Then reload:
```bash
curl -X POST http://localhost:9090/-/reload
```

## Adding Alert Rules

Edit `prometheus/rules/penfold.yml`:

```yaml
groups:
  - name: my-alerts
    rules:
      - alert: MyAlert
        expr: my_metric > threshold
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "My alert fired"
```

Then reload:
```bash
curl -X POST http://localhost:9090/-/reload
```

## Troubleshooting

**Prometheus can't scrape target:**
```bash
# Check target status in UI: http://dev02.brown.chat:9090/targets

# Test connectivity from container
docker exec prometheus wget -q -O- http://host.docker.internal:8080/metrics

# For dev01 targets
docker exec prometheus wget -q -O- http://dev01.brown.chat:8085/metrics
```

**Loki not receiving logs:**
```bash
# Check promtail status
docker compose logs promtail

# Verify log files exist
docker exec promtail ls -la /var/log/
```

**Grafana datasource error:**
```bash
# Check datasource connectivity
docker exec grafana wget -q -O- http://prometheus:9090/api/v1/query?query=up

# Restart grafana
docker compose restart grafana
```
