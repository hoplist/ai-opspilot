# Evidence Pack 字段契约

本文记录 `auto_inspection` 面向 Codex、MCP、Skill 和 CLI 的只读 Evidence Pack 返回结构。该契约用于统一 `/api/context/pod`、`/api/context/workload`、MCP `get_context_pack` 和 CLI `context` 命令。

## 1. 安全边界

- Evidence Pack 只读聚合已有数据源，不执行 SSH、kubectl、节点命令或 Kubernetes 写操作。
- Evidence Pack 可以查询日志、事件、Prometheus 资源指标、incident、业务上下文和发布元数据。
- 数据源取不到数据时必须返回 `data_sources` 状态和 `errors`，不能静默显示为空。
- 修复动作只允许作为建议输出，不允许由 MCP、Skill 或 CLI 自动执行。

## 2. 入口

| 类型 | 入口 | 说明 |
| --- | --- | --- |
| Backend API | `GET /api/context/pod` | Pod 证据包 |
| Backend API | `GET /api/context/workload` | Workload 证据包 |
| Backend API | `GET /api/context/service` | Service 证据包 |
| Backend API | `GET /api/context/incident` | Incident 证据包 |
| Backend API | `GET /api/context/namespace` | Namespace 证据包 |
| Backend API | `GET /api/snapshot-index` | Snapshot Index TopN 预计算摘要 |
| MCP Tool | `get_context_pack` | 通过 MCP 获取 Pod/Workload 证据包 |
| MCP Resource | `pod://<cluster>/<namespace>/<pod>` | 读取 Pod Evidence Pack |
| MCP Resource | `workload://<cluster>/<namespace>/<kind>/<name>` | 读取 Workload Evidence Pack |
| MCP Resource | `incident://<incident_id>` | 读取 Incident Evidence Pack |
| Project CLI | `python -m auto_inspection.context_cli pod|workload` | 本地或自动化任务获取证据包 |
| Skill CLI | `scripts/auto_inspection_backend.py context-pod|context-workload` | Skill 使用的 MCP 优先、backend 回退入口 |

## 3. 请求字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `target_type` | string | MCP 必填 | `pod` 或 `workload` |
| `namespace` | string | 是 | Kubernetes namespace |
| `pod` | string | Pod 场景二选一 | Pod 名称 |
| `workload_name` | string | Pod/Workload 场景二选一 | Deployment/StatefulSet/DaemonSet 等 workload 名称 |
| `workload_kind` | string | 否 | workload 类型 |
| `service` | string | 否 | 业务服务名 |
| `incident_id` | string | Incident 场景可填 | incident 标识或查询关键字 |
| `symptom` | string | 否 | `oom`、`crashloop`、`probe`、`pending`、`imagepull`、`latency`、`error`、`unknown` |
| `q` | string | 否 | 显式查询条件，优先级高于 `symptom` 默认查询 |
| `range_hours` | integer | 否 | 查询时间窗，默认 6 小时 |
| `size` | integer | 否 | 每类证据最大返回条数，默认 30 |

## 4. 返回字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `mode` | string | 固定为 `read_only_evidence_pack` |
| `target_type` | string | `pod` 或 `workload` |
| `target` | object | 目标 namespace、pod、workload、service |
| `request` | object | 归一化后的 symptom、query、range_hours、size |
| `window` | object | 查询窗口起止时间 |
| `summary.status` | string | `ok`、`partial`、`empty`、`error` |
| `summary.top_signals` | array | CPU/Mem 短期上涨、OOM、重启、Throttle、FS 等高价值信号 |
| `summary.matched_pods` | array | 匹配到的 Pod 名称 |
| `summary.missing_or_failed_sources` | array | 空数据或失败的数据源 |
| `data_sources` | object | 每个数据源的状态、耗时、数量和错误消息 |
| `evidence.resources` | object | Pod 资源趋势、基线规则和匹配 Pod 资源行 |
| `evidence.logs` | array | 日志摘要 |
| `evidence.events` | array | Kubernetes event 摘要 |
| `evidence.incidents` | array | 当前 incident 摘要 |
| `evidence.business_context` | object | 业务日志和 Trace 关联摘要 |
| `evidence.release` | object | 当前 workload 发布元数据 |
| `evidence.recent_changes` | array | 最近发布/配置变更 |
| `evidence.timeline` | array | 聚合时间线 |
| `errors` | array | 数据源失败或目标无匹配数据的明细 |
| `safety` | object | 只读安全声明 |

## 5. 状态语义

- `ok`：至少一个核心数据源成功返回有效证据，且没有数据源硬失败。
- `partial`：部分数据源失败，但仍返回了可用证据。
- `empty`：数据源可查询但没有匹配证据，API 使用 404 提醒调用方不是正常有数。
- `error`：核心数据源均不可用或聚合失败，API 使用 502 或 500。

## 6. 短期上涨信号

`summary.top_signals` 中的 `cpu_short_trend` 和 `mem_short_trend` 使用当前 Prometheus 资源面板已有的短窗口基线字段：

- `*_short_recent_avg`：短期最近窗口平均值。
- `*_short_prev_avg`：上一短期窗口平均值，作为 baseline。
- `ratio`：`recent_avg / max(baseline_avg, baseline_floor)`。
- `delta`：`recent_avg - baseline_avg`。
- `severity=watch`：达到观察阈值。
- `severity=alert`：达到告警阈值。

阈值来自 `pod_trend_rules`，包括 `watch_ratio`、`alert_ratio`、`watch_delta`、`alert_delta`、`min_current` 和 `baseline_floor`。

## 7. Snapshot Index

`GET /api/snapshot-index` 返回当前窗口内预计算 TopN：

- `items.abnormal_pods`：异常 Pod TopN，聚合 OOM、重启、CPU/Mem 短期上涨、Throttle、FS 等信号。
- `items.resource_trends`：资源趋势 TopN，主要来自 `summary.top_signals`。
- `items.recent_changes`：发布/配置变更 TopN。
- `items.business_errors`：业务错误 TopN。

`GET /api/resources` 也会携带 `summary.top_signals`，前端异常 Pod 清单可直接使用同一套信号做排序和颜色突出。
