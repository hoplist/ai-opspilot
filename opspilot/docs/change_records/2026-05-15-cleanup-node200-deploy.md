# 2026-05-15 旧平台清理与 node200 部署

## 背景

OpsPilot 已切换为 Go Core / Go CLI MVP，旧 `auto_inspection` Python 后端、Dashboard、OpenSearch/MinIO/MySQL/eBPF 等重组件不再作为当前默认平台继续保留。

## 仓库清理

- 移除旧 Python RCA 后端与 MCP 入口。
- 移除旧 Dashboard、旧测试、旧 runbook、旧脚本。
- 移除旧 `deploy/observability`、`deploy/monitoring`、`deploy/rca-service`、`yaml/`、`docs/cn`、`docs/en`、`deploy/intranet-bundle` 等历史部署与文档。
- 根 README 改为 OpsPilot Go MVP 入口。
- 保留新目录：
  - `opspilot/`
  - `deploy/opspilot/`
  - `docs/opspilot/`

## node200 集群清理

- 删除旧命名空间：
  - `observability`
  - `monitoring`
  - `db`
- 删除旧 PV：
  - `auto-inspection-rca-*`
  - `minio-*`
  - `opensearch-*`
  - `pyroscope-*`
  - `prometheus-server-nfs-pv-206`
  - `pvc-db-nfs-pv-206`
  - `pvc-backup-nfs-pv-206`
  - `pvc-del-nfs-pv-206`
- 清空旧存储目录：
  - `192.168.48.206:/srv/nfs/observability/*`
  - `192.168.48.206:/srv/nfs/monitoring/prometheus`
  - `192.168.48.206:/srv/nfs/mysql-lab`
  - `192.168.48.200:/data/auto-inspection`
  - `192.168.48.201:/data/auto-inspection`
  - `192.168.48.202:/data/auto-inspection`

## 新服务部署

- 本机交叉编译 Linux amd64 二进制：
  - `build/linux-amd64/opspilot-core`
  - `build/linux-amd64/opspilot`
- 打包镜像：`opspilot-core:0.1.0-mvp-go`。
- 镜像 tar 大小约 8.5MB。
- 导入到 node200 三个节点的 containerd。
- 部署 `deploy/opspilot/core`。
- Service 使用 NodePort `32180`。

## 验证

- `opspilot-core` Pod 已 Running。
- `GET http://192.168.48.200:32180/api/health` 返回 `ok=true`。
- `GET http://192.168.48.200:32180/api/inventory/overview?limit=5` 返回集群资源统计。

## 保留项

- 未触碰 `argocd`、`ai-dev`、`ai-tools`、`keep` 等非本平台命名空间。
- 未跟踪目录 `deploy/ai-dev/` 属于本地测试内容，暂未纳入本次 git 清理提交。
