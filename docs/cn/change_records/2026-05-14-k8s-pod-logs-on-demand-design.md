# 2026-05-14 Kubernetes Pod 日志按需读取设计调整

## 背景

当前平台原设计中，Kubernetes 容器日志可以通过 Fluent Bit 写入 OpenSearch/ELK，再由 RCA Backend 查询。这个方案适合长期日志检索，但在默认部署中会带来较高的资源成本：

- 容器日志量增长快，索引和磁盘压力大。
- OpenSearch/ELK 组件本身较重。
- RCA 平台如果自带日志搜索栈，会和现有运维日志平台职责重叠。
- 日常 RCA 更多需要某个 Pod 或 Workload 的短窗口日志，不一定需要全量长期检索。

## 本次调整

更新设计文档：

- `docs/cn/architecture.md`
- `docs/cn/slim_ops_architecture.md`

核心调整：

- RCA 平台不默认自建 OpenSearch。
- RCA 平台不默认全量采集 Kubernetes 容器日志。
- RCA Backend 通过 Kubernetes RBAC 的 `pods/log` 权限按需读取 Pod 日志。
- ELK 继续保留网关日志、业务日志和关键系统日志。
- Evidence Pack 在调查时聚合 Prometheus 指标、Kubernetes 资源状态、Kubernetes Event、按需 Pod 日志、ELK 网关/业务日志和发布变更。

## 推荐日志分层

| 日志类型 | 默认方式 | 说明 |
| --- | --- | --- |
| Pod 当前日志 | Kubernetes API `pods/log` 按需读取 | 用于最近异常排查 |
| Pod previous 日志 | Kubernetes API `pods/log?previous=true` 按需读取 | 用于 CrashLoopBackOff / 重启排查 |
| Kubernetes Event | Kubernetes API 读取 | 用于调度、探针、镜像、驱逐等证据 |
| 网关日志 | ELK | 用于 504、5xx、慢请求、入口流量分析 |
| 业务日志 | ELK | 用于 request_id、trace_id、user_id、order_id 关联 |
| 普通服务器关键日志 | ELK 或现有日志平台 | 按需接入 |

## Backend 设计约束

Pod 日志接口必须限制：

- `tail_lines`
- `since_seconds`
- `limit_bytes`
- `namespace`
- `pod`
- `container`
- `previous`

建议默认值：

- `tail_lines <= 300`
- `since_seconds <= 1800`
- `limit_bytes <= 1MiB`

不做：

- 全集群全量日志扫描。
- 最近 7 天全局全文搜索。
- 大量用户持续 tail 全集群日志。
- 在 RCA 平台长期保存容器日志明细。

长期检索、审计和跨业务全文搜索继续交给 ELK。

## RBAC 边界

RCA Backend ServiceAccount 只需要只读权限：

- `pods`
- `pods/log`
- `events`
- `namespaces`
- `services`
- `configmaps`
- `deployments`
- `statefulsets`
- `daemonsets`
- `replicasets`

不授予：

- `create`
- `update`
- `patch`
- `delete`
- `exec`
- `attach`
- `portforward`
- `secrets` 正文读取

## 后续落地

1. Backend 增加按需 Pod 日志 API。
2. MCP 增加 `get_pod_logs`、`get_workload_logs` 等只读工具。
3. GitOps/RBAC 确认 `pods/log` 权限已包含。
4. 默认部署从日志存储依赖中移除 RCA 自带 OpenSearch。
5. ELK 仅作为外部日志数据源接入，不作为 RCA 默认内置组件。

## 本次不涉及

- 未修改运行中 Kubernetes 集群。
- 未修改 GitOps 清单。
- 未删除部署文件。
- 未变更 Backend 代码。

