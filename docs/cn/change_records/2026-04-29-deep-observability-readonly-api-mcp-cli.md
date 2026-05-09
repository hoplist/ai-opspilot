# 2026-04-29 深层观测证据只读 API / MCP / CLI

## 背景

前一阶段已经通过 GitOps 接入 P0/P1/P2 组件：

- P0：Beyla / OTel eBPF 无侵入业务调用证据
- P1：Falco runtime 事件证据
- P2：Pyroscope / Alloy eBPF profiling 性能剖析

本次补齐 RCA Backend、MCP、skill CLI 的只读查询入口，让后续 RCA 可以从这些组件读取证据，而不是只完成采集组件部署。

## 本次新增

### Backend API

新增 3 个只读接口：

- `GET /api/observability/service-red-metrics`
  - 数据源：Prometheus
  - 采集链路：Beyla / OpenTelemetry 指标
  - 用途：查询服务 RED 证据，包括服务、namespace、route 维度的请求量与错误量查询结果。
- `GET /api/observability/runtime-events`
  - 数据源：OpenSearch `logs-k8s-*`
  - 采集链路：Falco 日志经 Fluent Bit 入库
  - 用途：查询 runtime 安全/进程/容器行为事件上下文。
- `GET /api/observability/profile-hotspots`
  - 数据源：Pyroscope API
  - 采集链路：Alloy eBPF profiling
  - 用途：查询 profile types、labels、service names 和热点 stack 节点。

### MCP 工具

新增只读工具白名单：

- `service_red_metrics`
- `runtime_events_context`
- `profile_hotspots`

这些工具只通过 RCA Backend 查询已有观测数据，不执行节点命令，不修改 Kubernetes 资源，不触发 Argo CD sync。

### Skill CLI

`auto-inspection-rca` skill helper 新增命令：

```bash
python scripts/auto_inspection_backend.py service-red-metrics --namespace observability --service pyroscope --limit 10
python scripts/auto_inspection_backend.py runtime-events --namespace observability --range-hours 6 --size 20
python scripts/auto_inspection_backend.py profile-hotspots --service-name pyroscope --range-hours 6 --limit 10
```

默认优先 MCP，MCP 不可用时回退 Backend HTTP；也可以使用 `--prefer-backend` 强制直连 Backend。

## 使用原则

- `service_red_metrics` 用于服务调用证据：请求量、错误量、route/service 维度关联。
- `runtime_events_context` 用于运行时证据：异常进程、敏感文件、容器逃逸类信号、Falco rule 命中。
- `profile_hotspots` 用于性能证据：高 CPU、热点函数、profile 类型、Pyroscope 查询入口。

当前 `service_red_metrics` 依赖 Beyla / OTel 指标已经进入 Prometheus。如果查询结果为 0，优先检查 OTel Collector 是否已经配置 Prometheus exporter、remote_write 或兼容抓取路径。

## 变更文件

- `auto_inspection/deep_observability.py`
- `auto_inspection/config.py`
- `auto_inspection/dashboard_server.py`
- `auto_inspection/backend_client.py`
- `auto_inspection/auto_inspection_mcp.py`
- `C:/Users/Administrator/.codex/skills/auto-inspection-rca/SKILL.md`
- `C:/Users/Administrator/.codex/skills/auto-inspection-rca/scripts/auto_inspection_backend.py`

## 验证记录

- 代码语法检查：通过
  - `python -m py_compile auto_inspection/deep_observability.py auto_inspection/config.py auto_inspection/dashboard_server.py auto_inspection/backend_client.py auto_inspection/auto_inspection_mcp.py .../auto_inspection_backend.py`
- RCA Backend 部署：已同步到运行 Pod 并滚动重启
  - `deployment/auto-inspection-rca` rollout 成功
- Backend API：
  - `/api/observability/service-red-metrics` 返回 `200`，Prometheus 已配置，当前 `item_count=0`
  - `/api/observability/runtime-events` 返回 `200`，OpenSearch 已配置，当前 `item_count=0`
  - `/api/observability/profile-hotspots` 返回 `200`，Pyroscope 已配置，当前 `hotspot_count=0`
- MCP tools/list：通过，已包含 `service_red_metrics`、`runtime_events_context`、`profile_hotspots`
- Skill CLI：通过，`profile-hotspots --prefer-backend` 可返回 Pyroscope/Alloy 查询结果

## 后续建议

- 把三个深层证据接口纳入 `get_context_pack` 的可选数据源，在 pod/workload RCA 时自动拼进 Evidence Pack。
- 如果 Pyroscope profile 需要长期保存，继续沿用已接入的 PVC；后续再按保留周期补对象存储或归档策略。

## 2026-04-29 追加：Beyla / OTel 指标进入 Prometheus

### 背景

`service_red_metrics` 已经能查询 Prometheus，但返回 `item_count=0`。原因是 OTel Collector 的 metrics pipeline 仍只使用 `debug` exporter，Beyla 通过 OTLP 上报的指标没有暴露给 Prometheus 抓取。

### GitOps 变更

在 `platform/gitops-manifests` 中补齐 OTel Collector Prometheus exporter：

- `clusters/test/observability/otel-collector/configmap.yaml`
- `clusters/test/observability/otel-collector/deployment.yaml`
- `clusters/test/observability/otel-collector/service.yaml`
- `source/deploy/observability/otel-collector/*`
- `source/yaml/observability/otel-collector/*`

关键配置：

- OTel Collector 新增 `prometheus` exporter：
  - `endpoint: 0.0.0.0:8889`
  - `namespace: beyla`
  - `resource_to_telemetry_conversion.enabled: true`
- metrics pipeline 从 `exporters: [debug]` 调整为 `exporters: [prometheus, debug]`
- Deployment 暴露 `metrics` 容器端口 `8889`
- Service 暴露 `metrics` 服务端口 `8889`
- Service 增加 Prometheus 抓取注解：
  - `prometheus.io/scrape: "true"`
  - `prometheus.io/port: "8889"`
  - `prometheus.io/path: /metrics`

### RCA 查询兼容

`service_red_metrics` 同步兼容：

- 原始指标名：`http_server_request_duration_seconds_count`
- Collector namespace 后指标名：`beyla_http_server_request_duration_seconds_count`
- namespace 标签：同时兼容 `namespace` 与 `k8s_namespace_name`

### 验证记录

- `python -m py_compile auto_inspection/deep_observability.py` 通过
- `kubectl kustomize clusters/test/observability` 通过
- `kubectl apply --dry-run=server -k clusters/test/observability` 通过
- GitOps 已提交并推送到 `platform/gitops-manifests`
  - commit: `feb9401 Expose OTel Beyla metrics to Prometheus`
- Argo CD `observability` 已同步到 `feb94018c3d9bc10947f1fc30e06390beb03bec6`
  - sync: `Synced`
  - health: `Healthy`
- `deployment/otel-collector` rollout 成功
- `otel-collector:8889/metrics` 已返回 `beyla_*` 指标
- Prometheus target 已抓取成功：
  - `up{service="otel-collector"} = 1`
- Prometheus 已能查询 Beyla 指标：
  - `beyla_http_server_request_duration_seconds_count{k8s_namespace_name="keep"}` 返回 2 条
  - `beyla_http_client_request_duration_seconds_count{k8s_namespace_name="keep"}` 返回 1 条
- RCA Backend `service_red_metrics` 已返回数据：
  - `GET /api/observability/service-red-metrics?namespace=keep&service=keep&limit=5`
  - `item_count=4`
  - `server_rate=2`
  - `server_error_rate=1`
  - `client_rate=1`
  - `rpc_rate=0`

## 2026-04-29 追加：Pod 内存提前预警

### 背景

当前平台已有 `/api/alerts` 汇总能力，但原先主要覆盖磁盘、负载、Jenkins 离线等节点级或外部告警。为提前发现 Pod 内存持续上涨、逼近 limit、潜在 OOM 风险，本次在 RCA Backend 告警汇总中补充 Pod 内存类预警。

### 新增规则

- `Pod内存快速增长预警`
  - 最近 15 分钟内 `container_memory_working_set_bytes` 增加超过 100Mi
  - 同时 15 分钟趋势斜率为正
- `Pod内存预计触顶预警`
  - 使用 `predict_linear(...[30m], 3600)` 预测 1 小时后内存
  - 预测值超过 container memory limit 的 90%
- `Pod内存Limit使用率告警`
  - 当前 working set / memory limit 超过 85%

### 查询入口

```text
GET /api/alerts?range_hours=1
```

返回对象格式示例：

```json
{
  "category": "Pod内存预计触顶预警",
  "object": "observability/opensearch-0/opensearch",
  "hours": 0.73
}
```

### 验证记录

- `python -m py_compile auto_inspection/prom_alert_summary.py` 通过
- RCA Backend 已同步并 rollout 成功
- `/api/alerts?range_hours=1` 返回 `200`
- 验证时命中：
  - `observability/opensearch-0/opensearch`：`Pod内存Limit使用率告警`
  - `observability/opensearch-0/opensearch`：`Pod内存预计触顶预警`
  - `observability/pyroscope-.../pyroscope`：`Pod内存快速增长预警`

### 后续建议

- 当前是平台/API/前端可见的告警汇总；如需主动通知，需要接 Alertmanager 或 RCA webhook 轮询任务。
- 建议下一步新增一个只读告警通知 CronJob：定时调用 `/api/alerts`，只对新增或持续时间超过阈值的告警发送飞书/企业微信，避免重复刷屏。

## 2026-04-29 追加：深层证据接入 Evidence Pack

### 背景

P0/P1/P2 已经分别具备独立查询工具，但排障时还需要分别调用 `service_red_metrics`、`runtime_events_context`、`profile_hotspots`。本次把这三类证据接入 `context-pod`、`context-workload`、`context-service`、`context-namespace` 的 Evidence Pack 聚合链路。

### 变更

- `auto_inspection/context_pack.py`
  - 新增 `deep_observability` 数据源聚合。
  - 自动读取：
    - `service_red_metrics`
    - `runtime_events_context`
    - `profile_hotspots`
  - 在 `data_sources` 中记录每个来源的状态、耗时、数量。
  - 在 `summary.top_signals` 中加入深层证据信号。
  - 在 `evidence.deep_observability` 中返回原始只读查询结果。
- `auto-inspection-rca` skill 文档已同步说明 Evidence Pack 会包含深层证据。

### 输出位置

```json
{
  "data_sources": {
    "service_red_metrics": {},
    "runtime_events_context": {},
    "profile_hotspots": {}
  },
  "evidence": {
    "deep_observability": {
      "service_red_metrics": {},
      "runtime_events_context": {},
      "profile_hotspots": {}
    }
  }
}
```

### 验证记录

- `python -m py_compile auto_inspection/context_pack.py` 通过
- 本地调用 `context_pack.build_context_pack("namespace", {"namespace": "keep", "range_hours": 1, "size": 3})` 成功
- 验证时 `service_red_metrics` 返回有效数据，并进入 `summary.top_signals`

## 2026-04-29 追加：轻量告警通知器

### 背景

当前先不拆独立告警平台，也不替换夜鹰。保留 `/api/alerts` 作为平台风险汇总入口，新增一个轻量通知器用于后续 CronJob 或手动触发，把新增/持续的告警发送到飞书、企微、钉钉或 generic webhook。

### 变更

- 新增 `auto_inspection/alert_notify.py`
  - 查询 `prom_alert_summary.collect_alert_rows`
  - 按 `category + object` 去重
  - 支持 cooldown，避免重复刷屏
  - 支持 `dry_run`
  - 支持 `generic`、`feishu`、`wecom`、`dingtalk` payload
- 新增配置：
  - `ALERT_NOTIFY_ENABLED`
  - `ALERT_NOTIFY_WEBHOOK_URL`
  - `ALERT_NOTIFY_WEBHOOK_TYPE`
  - `ALERT_NOTIFY_STATE_FILE`
  - `ALERT_NOTIFY_RANGE_HOURS`
  - `ALERT_NOTIFY_COOLDOWN_SECONDS`
  - `ALERT_NOTIFY_MIN_HOURS`
- 新增 Backend 触发入口：

```text
POST /api/alerts/notify
```

请求示例：

```json
{
  "enabled": true,
  "dry_run": true,
  "range_hours": 1,
  "cooldown_seconds": 1800,
  "min_hours": 0
}
```

### 使用建议

- 当前先用于手动触发或后续 CronJob 调用。
- 真正生产启用时建议设置 webhook secret/env，不把 webhook URL 写入 Git。
- 后续如需要主动通知，只需要加一个 GitOps CronJob 定时 POST `/api/alerts/notify`。

### 验证记录

- `python -m py_compile auto_inspection/alert_notify.py auto_inspection/config.py auto_inspection/dashboard_server.py` 通过
- 本地 `alert_notify.process_alerts(enabled=True, dry_run=True, range_hours=1)` 成功
- RCA Backend 已同步并 rollout 成功
- `POST /api/alerts/notify` dry-run 返回 `200`
- 验证时返回 `firing_count=3`、`selected_count=3`、`sent=0`、`skipped=1`

## 2026-04-29 追加：告警通知 CronJob dry-run 预置

### 背景

先补齐 GitOps CronJob 形态，但暂时不接入任何通知渠道，不配置 webhook Secret，不发送飞书/企微/钉钉消息。

### GitOps 变更

新增 dry-run CronJob：

- `clusters/test/observability/auto-inspection-rca/alert-notify-cronjob.yaml`
- `source/deploy/rca-service/alert-notify-cronjob.yaml`
- `source/yaml/rca-service/alert-notify-cronjob.yaml`

并加入对应 `kustomization.yaml`。

CronJob 名称：

```text
auto-inspection-alert-notify-dryrun
```

执行频率：

```text
*/5 * * * *
```

调用目标：

```text
POST http://auto-inspection-rca.observability.svc.cluster.local:18080/api/alerts/notify
```

请求体：

```json
{
  "enabled": false,
  "dry_run": true,
  "range_hours": 1,
  "cooldown_seconds": 1800,
  "min_hours": 0
}
```

### 安全边界

- 不接入任何 webhook URL。
- 不引用通知 Secret。
- `enabled=false`，即使接口返回告警，也不会发送通知。
- `dry_run=true`，仅验证告警查询与去重链路。

### 验证记录

- `kubectl kustomize clusters/test/observability` 通过
- `kubectl apply --dry-run=server -k clusters/test/observability` 通过
- server dry-run 显示：
  - `cronjob.batch/auto-inspection-alert-notify-dryrun created (server dry run)`
- GitOps 已提交并推送：
  - `1b0a929 Add dry-run alert notify cronjob`
- Argo CD `observability` 已同步到 `1b0a92939a49ba777d892d5d354d215f58ab4450`
  - sync: `Synced`
  - health: `Healthy`
- 集群中已创建 CronJob：
  - `auto-inspection-alert-notify-dryrun`
  - schedule: `*/5 * * * *`
  - suspend: `false`
- 手动触发临时 Job 验证通过：
  - 返回 `enabled=false`
  - 返回 `dry_run=true`
  - 返回 `sent=0`
  - 返回 `skipped=1`
  - 未接入 webhook，未发送任何通知

### 后续启用方式

后续需要正式通知时，再新增 Secret 保存 webhook URL，并将 CronJob 请求体改为：

```json
{
  "enabled": true,
  "dry_run": false
}
```
