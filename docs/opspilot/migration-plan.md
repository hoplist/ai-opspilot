# OpsPilot 完全重构路线

## 结论

旧的 `auto_inspection` 暂时没有生产使用约束，OpsPilot 可以按新架构完全重构。

这次不再以“兼容旧 Backend / 旧 CLI / 旧部署”为目标，而是建立一套新的项目边界：

```text
opspilot/
  core/       在线主 API
  cli/        确定性命令入口
  mcp/        MCP 工具适配
  worker/     巡检、基线、报告、备份校验
  console/    Web UI
  contracts/  OpenAPI / JSON Schema / 工具契约
```

旧代码只作为参考，不作为新实现的目录结构约束。

## 重构原则

- 新代码进入 `opspilot/`。
- 新部署进入 `deploy/opspilot/`。
- 新文档进入 `docs/opspilot/`。
- 旧 `auto_inspection/`、`dashboard-*`、`yaml/`、`deploy/observability/` 暂时保留，但不再继续扩展。
- 不再默认部署 OpenSearch、MinIO、MySQL、eBPF。
- 第一阶段只做只读能力。
- CLI 和 Backend API 先定义 schema，再实现。

## 目标分层

```text
opspilot-core
  Go 优先，承载在线高并发只读查询。

opspilot-cli
  Go，提供确定性命令入口。

opspilot-mcp
  MCP 适配层，只暴露只读工具。

opspilot-worker
  Python 优先，承载异步巡检、基线、报告、备份校验和 AI 摘要。

opspilot-console
  Web UI，只调用 opspilot-core。
```

## P0：项目骨架

目标：

- 建立 `opspilot/` 目录。
- 建立 `deploy/opspilot/` 目录。
- 建立 contracts 目录。
- 文档统一放入 `docs/opspilot/`。

完成标准：

- 新旧目录边界清楚。
- 后续代码不再往旧目录继续堆。

## P1：契约优先

先定义接口和命令，不急着实现全部逻辑。

Backend API：

- `/api/health`
- `/api/inventory/overview`
- `/api/inventory/servers`
- `/api/inventory/clusters`
- `/api/inventory/containers`
- `/api/k8s/pods`
- `/api/k8s/logs/pod`
- `/api/metrics/top-nodes`
- `/api/context/pod`
- `/api/diagnose/pod`

CLI：

```bash
opspilot schema
opspilot inventory overview
opspilot inventory servers
opspilot k8s pods --status abnormal
opspilot k8s logs pod --namespace prod --pod xxx --tail 300
opspilot context pod --namespace prod --pod xxx
opspilot diagnose pod --namespace prod --pod xxx
```

完成标准：

- 有 OpenAPI / JSON Schema / CLI schema。
- AI Skill 可以根据 schema 知道应该调用什么。
- 每个接口都标注只读边界、参数限制和返回结构。

## P2：最小可用闭环

优先实现一条排障主链路：

```text
用户/AI 问某个 Pod 为什么异常
  -> opspilot CLI / MCP
  -> opspilot-core
  -> Kubernetes API 查 Pod / Event / pods-log
  -> Prometheus 查资源指标
  -> 返回 Evidence Pack
```

完成标准：

- 能列出集群资源。
- 能列出异常 Pod。
- 能按需读取 Pod 当前日志和 previous 日志。
- 能生成 Pod Evidence Pack。
- CLI 和 MCP 都能调用。

当前 MVP 已切换为 Go 实现，`opspilot-core` 与 `opspilot CLI` 共享同一 Go 模块：

- `go run ./opspilot/core`
- `go run ./opspilot/cli schema`
- `go run ./opspilot/cli inventory overview`
- `go run ./opspilot/cli k8s pods --status abnormal`
- `go run ./opspilot/cli k8s logs pod -n <ns> --pod <pod>`
- `go run ./opspilot/cli context pod -n <ns> --pod <pod>`
- `go run ./opspilot/cli diagnose pod -n <ns> --pod <pod>`

MCP 适配层仍待实现。

## P3：服务器与 Docker 资源

目标：

- 接入 Prometheus 中的 node_exporter 指标。
- 接入 Docker/cAdvisor 指标。
- 统一 Inventory：
  - server
  - docker_host
  - container
  - kubernetes_node
  - workload

完成标准：

- AI 能回答“当前有哪些服务器”。
- AI 能回答“哪台服务器内存最高”。
- AI 能回答“某台 Docker 主机有多少容器”。

## P4：ELK 网关/业务日志

目标：

- 接入外部 ELK 查询。
- 只查网关、业务、关键系统日志。
- 不全量采集 Kubernetes 容器日志。

完成标准：

- 能按 request_id / trace_id / route / service 查询业务日志。
- 能关联 504、5xx、慢请求。
- Evidence Pack 能包含 ELK 日志摘要和跳转信息。

## P5：异步巡检和基线

目标：

- `baseline-job`
- `health-snapshot-job`
- `incident-correlation-job`
- `backup-verify-ingest-job`
- `report-job`

完成标准：

- 在线 API 不做重计算。
- Worker 生成快照和摘要。
- UI/CLI/MCP 只读取结果。

## P6：部署和发布

目标部署目录：

```text
deploy/opspilot/
  core/
  cli/
  mcp/
  console/
  worker/
  rbac/
  optional/
```

默认部署：

- `opspilot-core`
- `opspilot-mcp`
- `opspilot-console`
- `opspilot-worker`
- RBAC
- ConfigMap / Secret 引用

不默认部署：

- OpenSearch
- MinIO
- MySQL
- eBPF

## 旧代码处理策略

短期：

- 保留旧代码。
- 不继续扩展旧目录。
- 新实现从 `opspilot/` 开始。

中期：

- OpsPilot 最小闭环跑通后，逐步删除旧入口脚本和旧 dashboard。
- 只保留仍有参考价值的文档和测试样例。

长期：

- 仓库主入口改成 OpsPilot。
- 旧 `auto_inspection` 作为历史分支或 archive。
