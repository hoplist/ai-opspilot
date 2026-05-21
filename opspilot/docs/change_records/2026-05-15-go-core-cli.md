# 2026-05-15 OpsPilot Go Core / CLI

## 背景

按照新的 OpsPilot 架构，在线 Backend 和确定性 CLI 需要从 Python MVP 切到 Go，形成更适合内网分发、高并发只读查询和后续 MCP 接入的基础。

## 本次变更

- 本机安装 Go `1.23.2`，安装包来自桌面 `tmp` 目录。
- 新增 Go module：`github.com/dualistpeng-netizen/ai-observability`。
- `opspilot-core` 切换为 Go HTTP API：
  - `GET /api/health`
  - `GET /api/inventory/overview`
  - `GET /api/k8s/pods`
  - `GET /api/k8s/logs/pod`
  - `GET /api/context/pod`
  - `GET /api/diagnose/pod`
- `opspilot` CLI 切换为 Go：
  - `opspilot schema`
  - `opspilot inventory overview`
  - `opspilot k8s pods`
  - `opspilot k8s logs pod`
  - `opspilot context pod`
  - `opspilot diagnose pod`
- Kubernetes 查询逻辑沉到 `opspilot/internal/k8s`，支持：
  - 集群内 ServiceAccount 调 Kubernetes API。
  - 本机开发 fallback 到 `kubectl`。
  - Pod 日志按需读取，默认短窗口并限制大小。
- Dockerfile 改为 Go 多阶段构建，同时产出 `opspilot-core` 和 `opspilot` CLI 二进制。
- 移除 Python core/cli MVP 入口，避免双实现造成后续混乱。

## 验证

```bash
go test ./opspilot/...
go run ./opspilot/cli schema
go run ./opspilot/core --host 127.0.0.1 --port 18082
curl http://127.0.0.1:18082/api/health
kubectl kustomize deploy/opspilot/core
```

## 后续

- MCP 适配层优先复用 Go CLI/Core 的契约。
- 继续补 Prometheus、ELK/APISIX、服务器/Docker inventory。
- Python 只在异步 worker 场景保留，例如巡检报告、基线、备份校验和 AI 摘要。
