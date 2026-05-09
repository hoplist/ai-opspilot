# 2026-05-08 内网部署 hostPath 目录统一为 observability

## 背景

206 内网部署包已经统一使用 `observability` namespace。为了让节点本地数据目录和运行命名空间一致，本次同步调整 hostPath 数据目录前缀。

## 变更

仅修改 206 内网部署包，不同步 GitLab/GitOps 远端。

目录前缀从：

```text
/data/auto-inspection
```

调整为：

```text
/data/observability
```

部署前需要在固定节点 `xzyc115-19` 上准备：

```bash
mkdir -p /data/observability/mysql-31326
mkdir -p /data/observability/prometheus
mkdir -p /data/observability/minio
mkdir -p /data/observability/opensearch
mkdir -p /data/observability/pyroscope
mkdir -p /data/observability/auto-inspection-rca/data
mkdir -p /data/observability/auto-inspection-rca/outputs
```

## 覆盖范围

- MySQL hostPath
- Prometheus hostPath
- MinIO hostPath
- OpenSearch hostPath
- Pyroscope hostPath
- RCA Backend/MCP data 与 outputs hostPath
- 内网部署包文档中的目录说明

## 验证

已确认部署包中不再残留 `/data/auto-inspection`，关键渲染入口可正常渲染。
