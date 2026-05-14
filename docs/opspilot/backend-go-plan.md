# Backend Go 化演进方案

## 结论

OpsPilot 不建议一开始全量重写 Python Backend，但应该从现在开始按未来 Go `opspilot-core` 的边界设计 API。

推荐拆分：

```text
opspilot-core     Go / Java，在线主 API，高并发只读查询
opspilot-worker   Python，AI 摘要、报告、巡检、基线、备份校验
opspilot-mcp      Python 或 Go，MCP 工具适配层
opspilot-cli      Go 或 Python，确定性命令入口
opspilot-console  前端，只调用 opspilot-core
```

## 为什么不马上全量重写

- 当前 Python 已经沉淀了大量数据源适配逻辑。
- AI 摘要、报告、巡检、脚本分析更适合 Python 快速迭代。
- 全量重写容易中断现有能力。
- 先冻结 API 契约更重要。

## 为什么在线核心适合 Go

Go 更适合承载：

- Kubernetes API 聚合查询。
- Pod 日志按需读取。
- Prometheus 查询代理。
- ELK 查询代理。
- Inventory 资源资产查询。
- Evidence Pack 基础组装。
- 权限、审计、限流、缓存、超时和连接池。

这些都是高并发 I/O 场景。

## 阶段路线

### P0：保留 Python Backend

- 继续服务现有功能。
- 增加超时、缓存和查询限制。
- 重任务移到异步 Job。

### P1：冻结 API 契约

- `/api/inventory/*`
- `/api/k8s/logs/*`
- `/api/context/*`
- `/api/metrics/*`
- `/api/release/*`
- `/api/backup/*`

明确请求参数、返回 JSON、错误码和权限边界。

### P2：新增 Go opspilot-core

先实现高频接口：

- Inventory。
- Kubernetes Pod / Event / Workload 查询。
- Pod logs on demand。
- Prometheus resource query。
- Evidence Pack 基础组装。

### P3：入口切换

- UI 切到 `opspilot-core`。
- CLI 切到 `opspilot-core`。
- MCP 优先调用 `opspilot-core`。
- Python worker 只负责异步分析。

### P4：高可用和治理

- 多副本部署。
- HPA。
- 鉴权和审计。
- 多集群、多租户。
- 查询缓存。

## 触发条件

满足以下条件时开始 P1/P2：

- 多人同时使用 UI/MCP 延迟明显升高。
- Pod 日志和 Prometheus 查询需要强限流。
- Backend CPU 或内存随用户数明显增长。
- 需要统一鉴权、审计和租户隔离。
- 需要作为团队级总运维 API。
