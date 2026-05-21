# 2026-05-19 Prometheus 多数据源

## 背景

OpsPilot 需要同时支持 Kubernetes 集群 Prometheus、外部 VM/Docker Prometheus、数据库或中间件专项 Prometheus，避免把所有采集目标强行塞进一个 Prometheus。

## 变更内容

- Prometheus 接入从单实例改为多数据源注册表。
- 保留 `OPSPILOT_PROMETHEUS_URL` 单实例兼容。
- 新增配置：
  - `OPSPILOT_PROMETHEUS_DEFAULT_SOURCE`
  - `OPSPILOT_PROMETHEUS_DATASOURCES`
- 当前部署配置：
  - 默认源：`node200-k8s`
  - 地址：`http://opspilot-prometheus-server.monitoring.svc.cluster.local`
- 新增接口：
  - `GET /api/metrics/datasources`
- 指标接口新增 `source` 参数：
  - `source=node200-k8s`
  - `source=all`
- `context pod` 和 `diagnose pod` 支持通过 `source` 指定 Prometheus 指标来源。

## 配置格式

```bash
OPSPILOT_PROMETHEUS_DEFAULT_SOURCE=node200-k8s
OPSPILOT_PROMETHEUS_DATASOURCES=node200-k8s=http://opspilot-prometheus-server.monitoring.svc.cluster.local,external-vm=http://prometheus-vm:9090
```

## 使用方式

```bash
opspilot metrics datasources
opspilot metrics nodes --source all --limit 10
opspilot metrics nodes --source node200-k8s --limit 10
opspilot metrics query --source node200-k8s --query "up"
opspilot metrics pods --source node200-k8s -n opspilot --sort cpu --limit 10
```

## 设计约束

- OpsPilot 只读查询 Prometheus，不写入 Prometheus。
- 多数据源由 OpsPilot 统一聚合展示，Prometheus 之间不强制互联。
- 外部服务器较少时可以接入当前集群 Prometheus；规模变大后建议独立 Prometheus，再通过 OpsPilot 多数据源统一入口查询。
