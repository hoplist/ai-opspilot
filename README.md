# auto_inspection

Prometheus-based inspection pipeline that builds baselines, detects anomalies,
generates events, and outputs a weekly report with optional AI summary.

## Requirements
- Python 3
- Prometheus HTTP API доступ (configured by env vars)
- Optional: Ollama for AI summary

## Quick start
```powershell
python pipeline.py
```

Core source code now lives under `auto_inspection/`. The root keeps only the
common entry scripts.

## Unified backend
Run the consolidated backend:
```powershell
python backend_server.py
```

Run the local MCP server:
```powershell
python auto_inspection_mcp.py --host 127.0.0.1 --port 18081
```

Key endpoints:
- `GET /api/health`
- `GET /api/backend/overview`
- `GET /api/search/status`
- `GET /api/search/logs`
- `GET /api/search/events`
- `GET /api/incidents/list`
- `GET /api/incidents/search`
- `POST /api/investigate`
- `GET /api/investigations/{id}`
- `GET /api/resources`
- `GET /api/alerts`
- `GET /api/pipeline/steps`
- `POST /api/pipeline/run`
- `GET /api/artifacts`
- `GET /api/incidents`
- `GET /api/report`
- `POST /api/report/generate`

## Pipeline steps
```text
targets    -> discover targets from Prometheus
baseline   -> build historical baseline (p50/p95)
anomaly    -> detect deviations from baseline
health     -> build health profiles
merge      -> merge anomalies into events
lifecycle  -> mark new/ongoing/resolved
escalation -> apply escalation policy
runbook    -> attach runbook suggestions
report     -> generate outputs/reports/weekly_report.md
```

Run specific steps:
```powershell
python pipeline.py --list
python pipeline.py --steps targets,baseline,anomaly
python pipeline.py --from baseline --to runbook
python pipeline.py --steps report
```

Run modules individually:
```powershell
python -m auto_inspection.discover_targets
python -m auto_inspection.baseline_builder
python -m auto_inspection.baseline_anomaly
python -m auto_inspection.health_profile
python -m auto_inspection.anomaly_merge
python -m auto_inspection.event_lifecycle
python -m auto_inspection.event_escalation
python -m auto_inspection.runbook_attach
python weekly_inspection.py
```

`weekly_inspection.py` also sends pod restart notifications when the feature is enabled.

## Configuration
All defaults live in `auto_inspection/config.py`. You can override with a JSON file and/or
environment variables (env vars take precedence).

Search-related configuration:
- `OPENSEARCH_URL`
- `OPENSEARCH_USERNAME`
- `OPENSEARCH_PASSWORD`
- `OPENSEARCH_VERIFY_SSL`
- `OPENSEARCH_INDEX_LOGS`
- `OPENSEARCH_INDEX_EVENTS`
- `AI_INVESTIGATION_ENABLED`
- `K8S_DIRECT_ENABLED`
- `KUBECTL_BIN`
- `KUBECONFIG_PATH`

Investigation results are stored locally under `data/investigations/` and can
also be indexed into OpenSearch when `OPENSEARCH_INDEX_INVESTIGATIONS` is configured.

Use `config.json` or a profile file like `config.prod.json`:
```powershell
$env:CONFIG_PROFILE="prod"
python pipeline.py
```
Or point to a custom path:
```powershell
$env:CONFIG_FILE="C:\\configs\\auto_inspection.json"
python pipeline.py
```

Example `config.json`:
```json
{
  "PROMETHEUS_URLS": ["http://10.234.4.233:9090"],
  "RANGE_DAYS": 7,
  "BASELINE_HISTORY_DAYS": 28,
  "ANOMALY_DEVIATION_RATIO": 0.2,
  "OLLAMA_URL": "http://127.0.0.1:11434/api/generate",
  "OLLAMA_MODEL": "gemma3:12b",
  "AI_SUMMARY_MODE": "strict",
  "POD_RESTART_NOTIFY_ENABLED": true,
  "POD_RESTART_NOTIFY_WEBHOOK_URL": "https://example.com/webhook",
  "POD_RESTART_NOTIFY_WEBHOOK_TYPE": "generic",
  "POD_RESTART_NOTIFY_TARGETS": ["cluster-a/default/api-0", "default/worker-0"],
  "RESOURCE_POD_TREND_DAYS": 1,
  "RESOURCE_POD_SHORT_WINDOW_MINUTES": 30,
  "RESOURCE_POD_TREND_ANOMALY_ENABLED": true,
  "RESOURCE_POD_TREND_WATCH_RATIO": 1.5,
  "RESOURCE_POD_TREND_ALERT_RATIO": 2.0,
  "RESOURCE_POD_TREND_WATCH_DELTA": 0.1,
  "RESOURCE_POD_TREND_ALERT_DELTA": 0.2,
  "RESOURCE_POD_TREND_MIN_CURRENT": 0.3,
  "RESOURCE_POD_TREND_BASELINE_FLOOR": 0.05
}
```
You can copy `config.example.json` to `config.json` and tweak values.

Pod restart notification target formats:
- `pod-name`
- `namespace/pod-name`
- `cluster/namespace/pod-name`

Supported webhook payload types:
- `generic`: POST JSON with `event`, `text`, and `items`
- `feishu`: text bot payload
- `wecom` or `dingtalk`: text bot payload

Override via environment variables:
```powershell
$env:PROMETHEUS_URLS="http://10.234.4.233:9090"
$env:RANGE_DAYS="7"
$env:BASELINE_HISTORY_DAYS="28"
$env:ANOMALY_DEVIATION_RATIO="0.2"
$env:REQUEST_RETRIES="3"
$env:REQUEST_BACKOFF_SECONDS="0.5"
$env:HISTORY_RETENTION_DAYS="90"
$env:OLLAMA_URL="http://127.0.0.1:11434/api/generate"
$env:OLLAMA_MODEL="gemma3:12b"
$env:AI_SUMMARY_MODE="strict"  # strict|llm|off
$env:POD_RESTART_NOTIFY_ENABLED="true"
$env:POD_RESTART_NOTIFY_WEBHOOK_URL="https://example.com/webhook"
$env:POD_RESTART_NOTIFY_WEBHOOK_TYPE="generic"
$env:POD_RESTART_NOTIFY_TARGETS="cluster-a/default/api-0,default/worker-0"
$env:RESOURCE_POD_TREND_DAYS="1"
$env:RESOURCE_POD_SHORT_WINDOW_MINUTES="30"
$env:RESOURCE_POD_TREND_ANOMALY_ENABLED="true"
$env:RESOURCE_POD_TREND_WATCH_RATIO="1.5"
$env:RESOURCE_POD_TREND_ALERT_RATIO="2.0"
$env:RESOURCE_POD_TREND_WATCH_DELTA="0.1"
$env:RESOURCE_POD_TREND_ALERT_DELTA="0.2"
$env:RESOURCE_POD_TREND_MIN_CURRENT="0.3"
$env:RESOURCE_POD_TREND_BASELINE_FLOOR="0.05"
python pipeline.py
```

## Outputs
- `outputs/reports/weekly_report.md`: weekly inspection report
- `data/targets.json`: discovered instances/jobs
- `data/baseline/*.json`: baselines per metric
- `data/anomalies.json`: raw anomalies
- `data/health_profiles.json`: health scores
- `data/events.json`: merged events
- `data/events_lifecycle.json`: lifecycle status
- `data/events_escalated.json`: escalated events
- `data/events_with_runbook.json`: events with runbook
- `data/pod_restart_notify_state.json`: last notified restart totals for monitored pods

## Runbooks
Edit `runbooks/runbooks.json` to customize recommendations.

## Layout
- `auto_inspection/`: Python package with pipeline, collectors, API server logic, and helpers
- `deploy/observability/`: Kubernetes manifests for OpenSearch, Dashboards, Fluent Bit, and OTel Collector
- `dashboard/`, `dashboard-live/`, `dashboard-alerts/`, `dashboard-rca/`: static UI assets
- `data/`: generated JSON artifacts
- `outputs/`: generated markdown and dashboard outputs
- `docs/`: design and review notes

## Cluster Access
- Prometheus: `http://192.168.48.200:32092`
- OpenSearch: `http://192.168.48.200:32090`
- OpenSearch Dashboards: `http://192.168.48.200:32091`
- RCA Workbench: `http://127.0.0.1:18080/dashboard-rca/` when the local backend is running

## OpenSearch Bootstrap
Initialize OpenSearch index templates:

```powershell
python bootstrap_opensearch.py
```

## Dashboards Bootstrap
Initialize OpenSearch Dashboards data views, visualizations, saved searches, and dashboards:

```powershell
python bootstrap_dashboards.py
```

Created dashboards:
- Overview: `http://192.168.48.200:32091/app/dashboards#/view/dashboard-auto-inspection-overview`
- Langfuse RCA: `http://192.168.48.200:32091/app/dashboards#/view/dashboard-langfuse-clickhouse-rca`

## RCA Page
- Local RCA page: `http://127.0.0.1:18080/dashboard-rca/`
- Supports:
  - Direct jump to object-specific Dashboards log/event views
  - Recent investigation list
  - Investigation target list

## Prometheus Notes
- The project now defaults to the in-cluster Prometheus exposed at `32092`
- Helm deployment assets are under `deploy/monitoring/prometheus/`
- Current deployment keeps Prometheus server, node-exporter, and kube-state-metrics enabled
- kube-state-metrics is pinned to `k8s.m.daocloud.io/kube-state-metrics/kube-state-metrics:v2.18.0`
- Resource and restart metrics now come from the new in-cluster Prometheus instead of the legacy external addresses

## Tests
```powershell
python -m unittest discover -s tests
```
