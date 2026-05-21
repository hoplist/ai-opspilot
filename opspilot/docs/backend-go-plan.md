# Backend Go 化方案

## 结论

OpsPilot 的在线入口统一切到 Go：

```text
opspilot-core     Go，在线只读 API，高并发查询边界
opspilot-cli      Go，确定性命令入口，给人和 AI 使用
opspilot-mcp      后续优先复用 CLI/Core 契约，只暴露只读工具
opspilot-worker   后续按需保留 Python，承载异步巡检、基线、报告、备份校验和 AI 摘要
opspilot-console  后续 Web UI，只调用 opspilot-core
```

## 当前状态

MVP 已完成 Go 版本：

- `opspilot/core`：HTTP API。
- `opspilot/cli`：确定性 CLI。
- `opspilot/internal/k8s`：Kubernetes 只读查询和短窗口日志读取。
- `opspilot/internal/response`：统一 JSON envelope。
- `opspilot/contracts`：CLI schema embed。

Python 不再作为在线 core/cli 的实现语言。后续只有异步分析、AI 摘要、报表生成这类任务确实更适合 Python 时，才放入 `opspilot-worker`。

## 为什么在线核心适合 Go

- Kubernetes API 聚合查询、Pod 日志按需读取、Prometheus/ELK 查询代理都属于高并发 I/O。
- Go 更容易做连接池、超时、限流、并发控制和单二进制交付。
- CLI 和 Core 可以共享契约、版本号、基础结构，减少运行时依赖。
- 部署镜像更小，内网分发更简单。

## 保留 Python 的位置

Python 只放在异步 worker 侧，适合：

- AI 摘要和报告生成。
- 备份校验文件解析。
- 基线快照和离线分析。
- 第三方 SDK 较多、开发速度优先的任务。

## 分阶段路线

### P0：Go Core / CLI MVP

已完成：

- `/api/health`
- `/api/inventory/overview`
- `/api/k8s/pods`
- `/api/k8s/logs/pod`
- `/api/context/pod`
- `/api/diagnose/pod`
- `opspilot schema`
- `opspilot inventory overview`
- `opspilot k8s pods`
- `opspilot k8s logs pod`
- `opspilot context pod`
- `opspilot diagnose pod`

### P1：补齐运维入口能力

- Prometheus 查询代理和资源 TopN。
- ELK / OpenSearch / APISIX 日志查询代理，按需接入。
- 服务器、Docker、Kubernetes 统一 inventory。
- 只读审计、超时、限流、脱敏。

### P2：MCP 与 Skill

- MCP 只暴露白名单只读工具。
- Skill 先查 `opspilot schema`，再选择命令。
- AI 不直接 `kubectl exec/delete/patch/scale`。

### P3：Console 与 Worker

- Console 只调用 `opspilot-core`。
- Worker 负责定时巡检、基线、报告、备份校验和 AI 摘要。

## 交付原则

- 默认只读。
- 默认短窗口日志，避免全量日志采集。
- OpenSearch、MinIO、MySQL、eBPF 都是可选模块，不再作为核心强依赖。
- Core/CLI 先稳定，MCP/Skill/UI 再围绕契约扩展。
