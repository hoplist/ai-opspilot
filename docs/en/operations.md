# RCA Operations Guide

## 1. Daily Checks

At minimum, check these components daily:

- OpenSearch Pod health
- Fluent Bit DaemonSet readiness on all nodes
- OTel Collector health
- Prometheus target health
- backend and MCP reachability

## 2. Recommended Validation Entry Points

### 2.1 OpenSearch Dashboards

Focus on:

- `dashboard-auto-inspection-overview`
- `search-incidents-current`
- `search-k8s-logs-recent-errors`
- `search-k8s-events-warnings`

### 2.2 RCA APIs

Focus on:

- `/api/incidents/list`
- `/api/investigation-targets`
- `/api/investigations/latest`

## 3. Retention Policy

The stack currently installs ISM retention policies for:

- logs: 14 days
- events: 30 days
- incidents: 60 days
- investigations: 90 days

These are controlled by:

- `OPENSEARCH_RETENTION_LOGS_DAYS`
- `OPENSEARCH_RETENTION_EVENTS_DAYS`
- `OPENSEARCH_RETENTION_INCIDENTS_DAYS`
- `OPENSEARCH_RETENTION_INVESTIGATIONS_DAYS`

## 4. Snapshot Baseline

A local filesystem snapshot repository is now registered:

- repository: `auto-inspection-local-fs`
- location: `/usr/share/opensearch/data/snapshots`

This is a runnable baseline. Recommended next hardening steps:

- scheduled snapshots
- independent backup storage
- snapshot retention policy

## 4.1 Investigation Storage Layers

The shared deployment now uses:

- MySQL for hot investigation metadata
- MinIO for cold archived investigation payloads
- OpenSearch for indexed investigation search

Use hot storage for:

- recent investigation lists
- summary cards
- frequent metadata lookups

Use cold storage for:

- full JSON payloads
- long-term retention
- export and replay scenarios

## 5. Disk Safety

The OpenSearch node currently enables:

- low watermark: 85%
- high watermark: 90%
- flood stage: 95%

Watch for:

- PVC / NFS free space
- read-only index states
- write failures

## 6. Log Normalization Quality Checks

Regularly confirm that these fields are populated:

- `severity`
- `logger`
- `message`
- `message_normalized`
- `service`
- `exception_type`
- `exception_message`
- `stack_language`

If they are missing, inspect:

- Fluent Bit `configmap-logs.yaml`
- application log format
- whether OpenSearch templates were refreshed

## 7. Incident and Recommendation Quality Checks

Verify:

- incidents appear in `search-incidents-current`
- the investigation target list merges `investigation + event` for the same object
- high-risk targets expose `restart_total / waiting_reason / last_terminated_reason`

## 8. Common Issues

### 8.1 Dashboards opens but new charts are missing

Actions:

1. Run `python bootstrap_dashboards.py`
2. refresh browser cache
3. verify saved objects were created

### 8.2 OpenSearch bootstrap fails

Check:

- whether OpenSearch is ready
- whether `_plugins/_ism` is available
- whether `path.repo` is configured for snapshot registration

### 8.3 No incident data is visible

Check:

1. whether Fluent Bit logs / events are reaching OpenSearch
2. whether Prometheus and kube-state-metrics have data
3. whether the pipeline has executed through the `runbook` step

### 8.4 RCA summary cards are empty

Check:

- `/api/investigations/latest`
- whether Prometheus context includes restart and request/limit data
- whether Kubernetes Pod fallback is still working

## 9. Recommended Next Work

- OpenSearch security authentication
- scheduled snapshots
- periodic backend health probes
- more advanced log parsers and exception extraction
