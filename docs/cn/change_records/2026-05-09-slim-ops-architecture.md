# 2026-05-09 RCA 平台瘦身与统一运维入口架构方案

## 背景

当前 RCA 平台已经接入 Prometheus、OpenSearch、Kubernetes API、GitLab、Argo CD、MCP、CLI、巡检基线和深度观测组件。功能已经覆盖较广，但默认部署包含 OpenSearch、OpenSearch Dashboards、MinIO、MySQL、eBPF、profiling 等组件后，整体偏重。

同时，Python Backend 同时承担在线 API、MCP、AI 调查、巡检任务和报告生成，后续多人使用时需要更清晰的职责拆分。

## 本次变更

新增目标架构文档：

- `docs/cn/slim_ops_architecture.md`

更新中文文档索引：

- `docs/cn/README.md`

## 架构结论

下一版平台建议收敛为：

```text
Prometheus 指标 + ELK 日志 + Kubernetes API + Docker/node_exporter/cAdvisor
    -> RCA Backend 统一只读分析入口
    -> UI / CLI / MCP / AI
    -> 异步巡检和基线任务
```

默认部署只保留：

- RCA Backend / Core
- MCP Server
- RCA UI
- 巡检和基线 CronJob
- Kubernetes 只读 RBAC
- Prometheus 查询配置
- ELK 查询配置

默认不再部署：

- RCA 自带 OpenSearch
- RCA 自带 OpenSearch Dashboards
- RCA 自带 MinIO
- RCA 自带 MySQL
- Beyla / Falco / Pyroscope / Alloy eBPF

这些组件后续作为 optional 模块按需开启。

## 后续落地方向

1. 补统一 Inventory API 和 MCP 工具，让 AI 能查询所有服务器、集群、Docker 主机、容器和资源使用情况。
2. Docker 主机通过 `node_exporter + cAdvisor` 接入 Prometheus。
3. 自动巡检、基线、健康快照、报告生成拆为异步任务。
4. Python 保留在 AI、MCP、Worker、报告生成层。
5. 在线高并发 Backend 后续可演进为 Go / Java `rca-core`。

## 本次不涉及

- 未修改运行中 Kubernetes 集群。
- 未修改 GitOps 部署清单。
- 未删除现有组件 YAML。
- 未变更 RCA Backend 代码。

