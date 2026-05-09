# 2026-05-08 Prometheus Helm Chart 离线部署包

## 背景

新内网集群机器没有配置 `prometheus-community` Helm repo，直接执行：

```bash
helm upgrade --install auto-prometheus prometheus-community/prometheus ...
```

会报错：

```text
repo prometheus-community not found
```

## 变更

内网部署包补充离线 chart：

```text
charts/prometheus-28.15.0.tgz
```

## 使用方式

在新集群管理机上执行：

```bash
cd /opt/auto-inspection-intranet-bundle/gitops-manifests/source/yaml/monitoring/prometheus

helm upgrade --install auto-prometheus \
  /opt/auto-inspection-intranet-bundle/charts/prometheus-28.15.0.tgz \
  -n observability \
  --create-namespace \
  -f values.yaml
```

不再依赖内网机器配置 Helm repo。
