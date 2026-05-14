# 2026-05-14 旧设计文档清理与 Go 后端演进策略

## 背景

平台目标架构已经从“自带完整观测栈”收敛为“统一运维入口 + 只读 RCA 分析层”。旧文档中仍有不少 OpenSearch 全量日志、AI Gateway、eBPF 默认接入、MySQL/MinIO 默认存储等设计，容易误导后续实现。

## 本次清理

删除以下旧主线设计/部署文档：

- `docs/cn/aigateway_mcp_integration.md`
- `docs/cn/codex_intelligent_data_access.md`
- `docs/cn/deployment.md`
- `docs/cn/log_correlation_minimal_plan.md`
- `docs/cn/mcp_readonly_observability_roadmap.md`
- `docs/cn/operations.md`
- `docs/cn/otel_business_correlation.md`
- `docs/cn/storage_architecture.md`

保留：

- `docs/cn/change_records/*`
- `docs/cn/recovery_runbook_2026-04-21.md`

历史变更记录和恢复记录不删除，用于追溯曾经的部署和演进过程。

## 本次更新

更新以下文档为新架构口径：

- `docs/cn/README.md`
- `docs/cn/product.md`
- `docs/cn/frontend_ui_design.md`
- `docs/cn/codex_distribution.md`
- `docs/cn/slim_ops_architecture.md`

## Go 后端结论

不建议马上全量把 Python Backend 改成 Go。

推荐方式：

```text
rca-core        Go / Java，在线主 API，高并发只读查询
rca-ai-worker   Python，AI 摘要、报告、复杂分析
rca-mcp         Python 或 Go，MCP 工具适配层
rca-cronjobs    Python，巡检、基线、备份校验、离线任务
rca-ui          前端，只调用 rca-core
```

短期继续保留 Python Backend，先补查询限制、缓存、超时和异步任务拆分。

中期再新增 Go `rca-core`，优先承载：

- Inventory 资源资产查询。
- Kubernetes API 聚合查询。
- Pod 日志按需读取。
- Prometheus 查询代理。
- ELK 查询代理。
- Evidence Pack 基础组装。
- 权限、审计、超时、连接池、限流。

Python 后续保留在 AI、MCP、巡检、报告和离线分析层。

## 本次不涉及

- 未修改运行中 Kubernetes 集群。
- 未修改 GitOps 清单。
- 未变更 Backend 代码。
- 未开始 Go 代码迁移。

