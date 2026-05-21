# 2026-05-19 node206 Prometheus

## 背景

node206 作为外部主机，需要独立部署 Prometheus，用于采集主机资源和 Docker 容器资源，并作为 OpsPilot 的第二个 Prometheus 数据源接入。

## 部署目录

- Prometheus：`/opt/prometheus`
- node-exporter：`/opt/node-exporter`
- cAdvisor：`/opt/cadvisor`

部署文件在仓库中同步保存：

- `deploy/external/node206/prometheus`
- `deploy/external/node206/node-exporter`
- `deploy/external/node206/cadvisor`

## 镜像

- `quay.io/prometheus/prometheus:v3.11.0`
- `quay.io/prometheus/node-exporter:v1.11.1`
- `m.daocloud.io/gcr.io/cadvisor/cadvisor:v0.52.1`

## 端口

- Prometheus：`192.168.48.206:9090`
- node-exporter：`192.168.48.206:9100`
- cAdvisor：`192.168.48.206:8080`

## Prometheus 抓取目标

- `127.0.0.1:9090`
- `127.0.0.1:9100`
- `127.0.0.1:8080`

## OpsPilot 数据源

新增数据源：

```text
node206-host=http://192.168.48.206:9090
```

保留默认数据源：

```text
node200-k8s=http://opspilot-prometheus-server.monitoring.svc.cluster.local
```

## OpsPilot 查询入口

新增容器指标查询：

```bash
opspilot metrics containers --source node206-host --sort cpu --limit 10
opspilot metrics containers --source node206-host --sort memory --limit 10
```

用于查询 node206 上 Docker/cAdvisor 暴露的容器 CPU 和内存指标。

## 验证记录

- node206 Prometheus、node-exporter、cAdvisor 均已启动。
- Prometheus `up` 查询已返回 3 个目标：
  - `prometheus`
  - `node206-node-exporter`
  - `node206-cadvisor`
- OpsPilot 已识别 `node206-host` 数据源。
- OpsPilot 已能查询 node206 主机指标与 Docker 容器指标。
