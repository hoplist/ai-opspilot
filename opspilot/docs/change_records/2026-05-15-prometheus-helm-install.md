# 2026-05-15 Prometheus Helm 安装

## 背景

OpsPilot Go Core 当前已接入 Kubernetes API，但尚未接入 Prometheus 指标查询。为后续补充 `metrics` API 与 CLI，需要先在 node200 集群恢复基础 Prometheus。

## 安装方式

使用 Helm chart：

```powershell
helm upgrade --install opspilot-prometheus prometheus-community/prometheus `
  --version 29.3.0 `
  -n monitoring `
  --create-namespace `
  -f deploy/opspilot/optional/prometheus-values.yaml
```

## Values

配置文件：`deploy/opspilot/optional/prometheus-values.yaml`

- Prometheus Server 使用 NodePort `32092`。
- Prometheus Server 镜像固定为节点已有的 `quay.io/prometheus/prometheus:v3.11.0`，避免集群拉取 `v3.11.3` 超时。
- kube-state-metrics 镜像使用 `k8s.m.daocloud.io/kube-state-metrics/kube-state-metrics:v2.18.0`，避免 `registry.k8s.io` 被网络拒绝。
- 暂时关闭 Alertmanager。
- 暂时关闭 Pushgateway。
- 开启 kube-state-metrics。
- 开启 node-exporter。
- Server 数据使用 `emptyDir`，先跑通链路；需要长期保存时再接 PVC。

## 访问

- 集群内：`http://opspilot-prometheus-server.monitoring.svc.cluster.local:80`
- 集群外：`http://192.168.48.200:32092`

## 后续

- 在 `opspilot-core` 增加 Prometheus 查询配置 `OPSPILOT_PROMETHEUS_URL`。
- 增加 `/api/metrics/*`。
- 增加 CLI：`opspilot metrics top-nodes/top-pods/pod`。
