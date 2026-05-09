# RCA Deployment Guide

## 1. Deployment Target

After deployment, the stack should include:

- Prometheus
- OpenSearch
- OpenSearch Dashboards
- Fluent Bit
- OTel Collector
- auto_inspection backend
- auto_inspection MCP

## 2. Main Directories

Key directories in the current workspace:

- `deploy/monitoring/prometheus`
- `deploy/observability/opensearch`
- `deploy/observability/opensearch-dashboards`
- `deploy/observability/fluent-bit`
- `deploy/observability/otel-collector`
- `deploy/rca-service`
- `yaml/`
- `auto_inspection`
- `dashboard-rca`
- `Dockerfile`

## 3. Prerequisites

Prepare:

- a reachable Kubernetes cluster
- writable persistent storage
  use dedicated persistent volumes for Prometheus and OpenSearch
- Python 3
- a working local `kubectl`

## 4. Configuration

Start from the example file:

```powershell
Copy-Item config.example.json config.json
```

At minimum, confirm:

- `PROMETHEUS_URLS`
- `PROMETHEUS_CLUSTERS`
- `OPENSEARCH_URL`
- `OPENSEARCH_DASHBOARDS_URL`
- `KUBECONFIG_PATH`

## 5. Cluster-side Deployment

### 5.1 Prometheus

Prometheus assets:

- `deploy/monitoring/prometheus`

Deploy based on the directory-specific instructions.

### 5.2 OpenSearch

```powershell
kubectl apply -k deploy/observability/opensearch
```

### 5.3 OpenSearch Dashboards

```powershell
kubectl apply -k deploy/observability/opensearch-dashboards
```

### 5.4 OTel Collector

```powershell
kubectl apply -k deploy/observability/otel-collector
```

### 5.5 Fluent Bit

```powershell
kubectl apply -k deploy/observability/fluent-bit
```

### 5.6 Shared RCA Service

```powershell
kubectl apply -k deploy/rca-service
```

This deploys:

- shared backend
- shared MCP service

NodePort exposure:

- backend: `32180`
- mcp: `32181`

### 5.7 MinIO Cold Archive

```powershell
kubectl apply -k deploy/observability/minio
```

NodePort exposure:

- MinIO API: `32093`
- MinIO Console: `32094`

## 6. OpenSearch Bootstrap

After OpenSearch is ready, run:

```powershell
python bootstrap_opensearch.py
```

This installs:

- logs / events / incidents / investigations index templates
- ISM retention policies
- snapshot repository registration

## 7. Dashboards Bootstrap

Run:

```powershell
python bootstrap_dashboards.py
```

This installs:

- data views
- saved searches
- visualizations
- dashboards

## 8. Local Service Startup

### 8.1 Start backend + MCP

```powershell
.\run_auto_inspection_stack.ps1
```

Or start them manually:

```powershell
python backend_server.py --host 127.0.0.1 --port 18080
python auto_inspection_mcp.py --host 127.0.0.1 --port 18081
```

## 9. Validation

### 9.1 Backend Health

Open:

- `http://127.0.0.1:18080/api/health`

### 9.2 Dashboards Validation

You should be able to open:

- `<OPENSEARCH_DASHBOARDS_URL>/app/dashboards#/view/dashboard-auto-inspection-overview`
- `<OPENSEARCH_DASHBOARDS_URL>/app/discover#/view/search-incidents-current`

### 9.3 RCA Page Validation

Open:

- `http://127.0.0.1:18080/dashboard-rca/`

Confirm the page includes:

- `Current Incidents`
- `Run RCA / Open Logs / Open Events / Open Dashboard`
- the key summary card section

### 9.4 Data Validation

Open:

- `http://127.0.0.1:18080/api/incidents/list?limit=5`
- `http://127.0.0.1:18080/api/investigation-targets?limit=5`
- `http://127.0.0.1:18080/api/investigations/latest`

## 10. Common Post-deployment Tasks

- adjust retention windows
- update Prometheus endpoints
- adapt NFS / PV configuration
- change Dashboards exposure mode
- customize `runbooks/runbooks.json`
- build and publish the image from `Dockerfile` if you do not want runtime source mounts
- use `yaml/` as a direct manifest mirror when you do not want to work from `deploy/`
