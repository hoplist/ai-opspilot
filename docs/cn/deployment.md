# RCA 部署指南

## 1. 部署目标

部署完成后，应具备以下组件：

- Prometheus
- OpenSearch
- OpenSearch Dashboards
- Fluent Bit
- OTel Collector
- auto_inspection backend
- auto_inspection MCP

## 2. 目录说明

当前工程中的关键目录：

- `deploy/monitoring/prometheus`
- `deploy/observability/opensearch`
- `deploy/observability/opensearch-dashboards`
- `deploy/observability/fluent-bit`
- `deploy/observability/otel-collector`
- `auto_inspection`
- `dashboard-rca`

## 3. 依赖前提

需要准备：

- 可访问的 Kubernetes 集群
- 可写的持久化存储
  建议为 Prometheus 与 OpenSearch 提供独立持久卷
- Python 3
- 本地可访问 Kubernetes 的 `kubectl`

## 4. 配置准备

建议从示例配置开始：

```powershell
Copy-Item config.example.json config.json
```

至少需要确认：

- `PROMETHEUS_URLS`
- `PROMETHEUS_CLUSTERS`
- `OPENSEARCH_URL`
- `OPENSEARCH_DASHBOARDS_URL`
- `KUBECONFIG_PATH`

## 5. 集群侧部署

### 5.1 Prometheus

Prometheus 目录：

- `deploy/monitoring/prometheus`

按目录内说明部署。

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

## 6. OpenSearch 初始化

部署 OpenSearch 后，执行：

```powershell
python bootstrap_opensearch.py
```

该步骤会安装：

- logs / events / incidents / investigations index template
- ISM retention policy
- snapshot repository

## 7. Dashboards 初始化

执行：

```powershell
python bootstrap_dashboards.py
```

该步骤会安装：

- data views
- saved searches
- visualizations
- dashboards

## 8. 本地服务启动

### 8.1 启动 backend + MCP

```powershell
.\run_auto_inspection_stack.ps1
```

或手动启动：

```powershell
python backend_server.py --host 127.0.0.1 --port 18080
python auto_inspection_mcp.py --host 127.0.0.1 --port 18081
```

## 9. 验证步骤

### 9.1 后端健康检查

打开：

- `http://127.0.0.1:18080/api/health`

### 9.2 Dashboards 验证

应能访问：

- `<OPENSEARCH_DASHBOARDS_URL>/app/dashboards#/view/dashboard-auto-inspection-overview`
- `<OPENSEARCH_DASHBOARDS_URL>/app/discover#/view/search-incidents-current`

### 9.3 RCA 页面验证

打开：

- `http://127.0.0.1:18080/dashboard-rca/`

确认可以看到：

- `Current Incidents`
- `Run RCA / Open Logs / Open Events / Open Dashboard`
- `关键指标摘要`

### 9.4 数据验证

打开：

- `http://127.0.0.1:18080/api/incidents/list?limit=5`
- `http://127.0.0.1:18080/api/investigation-targets?limit=5`
- `http://127.0.0.1:18080/api/investigations/latest`

## 10. 常见部署后动作

- 修改 retention 天数
- 调整 Prometheus 地址
- 调整 NFS / PV 配置
- 调整 Dashboards 暴露方式
- 调整 `runbooks/runbooks.json`
