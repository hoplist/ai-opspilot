# 2026-05-19 Prometheus 接入 OpsPilot

## 背景

Prometheus 已在 node200 集群通过 Helm 部署完成，当前 `opspilot-prometheus` release 位于 `monitoring` 命名空间，Server 通过 NodePort `32092` 暴露。

本次变更将 Prometheus 正式接入 OpsPilot Go Core，作为默认指标查询后端。

## 变更内容

- OpsPilot Core 增加 Prometheus 查询配置。后续已升级为多数据源配置，详见 `2026-05-19-prometheus-multi-datasource.md`。
- 部署清单 `deploy/opspilot/core/deployment.yaml` 已指向集群内 Prometheus：
  - `http://opspilot-prometheus-server.monitoring.svc.cluster.local`
- 新增只读指标接口：
  - `GET /api/metrics/health`
  - `GET /api/metrics/query?query=<promql>`
  - `GET /api/metrics/nodes`
  - `GET /api/metrics/pods`
  - `GET /api/metrics/pod?namespace=<ns>&pod=<pod>`
- `context pod` 和 `diagnose pod` 的 evidence 中开始补充 Prometheus Pod 指标。
- CLI 增加指标命令：
  - `opspilot metrics health`
  - `opspilot metrics query --query "up"`
  - `opspilot metrics nodes --limit 10`
  - `opspilot metrics pods -n opspilot --sort cpu --limit 10`
  - `opspilot metrics pod -n opspilot --pod <pod>`

## 指标来源

- 节点 CPU、内存、磁盘来自 `node-exporter`。
- Pod CPU、内存来自 kubelet/cAdvisor。
- Pod 重启次数来自 `kube-state-metrics`。

## 部署验证

- 已构建并导入镜像 `opspilot-core:0.1.1-prometheus-go`。
- 已滚动更新 node200 集群 `opspilot/opspilot-core`。
- `/api/health` 已返回 `prometheus.configured=true`、`prometheus.ready=true`。
- `/api/metrics/nodes` 已能返回 3 个节点的 CPU、内存、根分区使用率。
- `/api/context/pod` 的 evidence 已包含 Prometheus Pod 指标。

## 后续

- 如需长期保存 Prometheus 数据，再为 Prometheus Server 接入 PVC。
- 后续可在 Console 中增加节点和 Pod Top 视图。
- 后续可在 MCP 中开放只读 metrics 白名单工具。
