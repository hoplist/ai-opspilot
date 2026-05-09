# 2026-05-07 内网部署镜像切换到私有仓库

## 背景

node206 已经把 RCA 平台相关镜像重打 tag 到统一私有仓库前缀：

```text
docker-hub.tpo.xzoa.com/auto-inspection/
```

为了避免内网部署时继续访问外部镜像源，本次同步修改 GitOps manifests 和内网部署包镜像清单。

## 变更内容

- 将 RCA Backend/MCP 镜像切换为：

```text
docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260429-rca-image
```

- 将 MySQL、MinIO、OpenSearch、Fluent Bit、OTel Collector、Beyla、Falco、Pyroscope、Alloy、Prometheus、Argo CD、GitLab 等镜像统一切换到：

```text
docker-hub.tpo.xzoa.com/auto-inspection/<image>:<tag>
```

- 修正 Prometheus Helm values 中分字段配置的镜像：
  - `server.image`
  - `configmapReload.prometheus.image`
  - `kube-state-metrics.image`
  - `prometheus-node-exporter.image`

- 更新内网部署包镜像清单：

```text
deploy/intranet-bundle/images/auto-inspection-images.txt
```

## 验证

已执行以下渲染检查：

```text
kubectl kustomize clusters/test/observability
kubectl kustomize source/deploy/db/mysql-31326
helm template auto-prometheus prometheus-community/prometheus --version 28.15.0 -f source/deploy/monitoring/prometheus/values.yaml
```

关键工作负载渲染出的 `image:` 均已指向 `docker-hub.tpo.xzoa.com/auto-inspection/`。

## 注意

当前只是修改部署引用，不代表镜像已经全部 push 到私有仓库。node206 本地已准备 23 个目标 tag，确认后还需要执行 `docker push`。
