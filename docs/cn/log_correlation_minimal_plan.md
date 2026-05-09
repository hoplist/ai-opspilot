# 业务日志关联最小改动方案

本文给出一套适用于当前 `auto_inspection` 项目的最小改动方案，目标是让多服务日志能够围绕固定上下文字段进行检索、串联和 RCA 分析，同时尽量不推翻现有采集链路。

## 1. 当前现状

当前仓库已经具备这些基础：

- Kubernetes 控制台日志通过 `Fluent Bit -> OpenSearch` 进入 `logs-k8s-*`
- Kubernetes Events 通过 `Fluent Bit kubernetes_events -> OpenSearch` 进入 `events-k8s-*`
- 后端 RCA 服务已经支持按 `cluster / namespace / workload_name / pod / service / severity` 检索日志
- 后端 RCA 服务已经支持按 `cluster / namespace / pod / reason / type` 检索事件

目前缺口主要有两个：

- 业务日志中的跨服务关联字段尚未标准化
- OpenSearch 模板和查询逻辑尚未把 `biz_line / trace_id / span_id / request_id / event_id` 做成一等字段

## 2. 字段职责约定

推荐先统一以下字段语义，不要混用：

- `service`
  服务名。稳定不变。示例：`apisix-gateway`、`devops-server`、`order-api`
- `biz_line`
  业务线或产品线。示例：`observability`、`trade`
- `trace_id`
  分布式调用链 ID。跨服务共享，通常来自 OpenTelemetry
- `span_id`
  当前服务内当前 span 的 ID。通常每个服务节点不同
- `request_id`
  一次入口请求 ID。建议由网关生成并向下透传
- `event_id`
  一次业务事件 ID。适合任务、消息、告警、异步事件链路

## 3. 最小改动原则

推荐顺序：

1. 先统一 `service` 和 `biz_line`
2. 再统一 `request_id`
3. 再补 `trace_id / span_id`
4. 最后在异步链路补 `event_id`

原因：

- `service` / `biz_line` 可以先由采集器静态补齐，成本最低
- `request_id` 最容易落地，收益最大
- `trace_id / span_id` 适合在 OpenTelemetry 接入后统一收敛
- `event_id` 更适合 MQ / 任务 / 领域事件，不必第一天全面铺开

## 4. 采集链路建议

### 4.1 控制台日志

如果服务能输出到 `stdout/stderr`，优先保留现有方案：

`应用 -> 容器控制台 -> Fluent Bit -> OpenSearch`

这是当前仓库最稳定的路径，推荐继续使用。

### 4.2 文件日志

如果有业务服务不打印控制台，只写文件，推荐继续使用 `Filebeat`：

- VM / 物理机：直接使用 Filebeat agent
- 容器内文件：优先用 Filebeat sidecar
- 不建议让同一份日志同时被 Filebeat 和 Fluent Bit 重复采集

关键点不是换采集器，而是让日志里稳定带出标准字段。

## 5. 应用日志格式建议

推荐统一输出 JSON 日志。最小可用字段如下：

```json
{
  "@timestamp": "2026-04-22T10:00:00Z",
  "service": "devops-server",
  "biz_line": "observability",
  "request_id": "req-20260422-abc123",
  "trace_id": "8f3c1c5d2f7e4b4c9f0d7c1a9b8e6d12",
  "span_id": "a1b2c3d4e5f67890",
  "event_id": "evt-20260422-xyz789",
  "severity": "error",
  "logger": "job.branch_sync",
  "message": "GetBranchesByIds timeout",
  "evtName": "bs_external_GetBranchesByIds",
  "timeCost": 10
}
```

说明：

- `service` / `biz_line` 可以由应用输出，也可以先让 Filebeat 静态补齐
- `request_id / trace_id / span_id / event_id` 必须来自应用上下文，采集器无法凭空生成正确值

## 6. 对当前日志样例的最小方案

### 6.1 APISIX access log

建议：

- 保持 Filebeat 采集
- 给该输入静态补：
  - `service: apisix-gateway`
  - `biz_line: observability`
- 尽量让 APISIX access log 中出现：
  - `request_id`
  - `trace_id`
  - `route`
  - `upstream_service`
  - `status`
  - `latency`

### 6.2 常用后端日志

针对你给的样例，当前 `message` 内已经是 JSON 字符串，建议：

- 保持 Filebeat 采集
- 在 Filebeat 里直接对 `message` 做 JSON 解包
- 按日志路径静态补：
  - `service: devops-server`
  - `biz_line: observability`
- 后端代码最小增加统一中间件，先输出：
  - `request_id`
  - `trace_id`
  - `span_id`

## 7. Go / Python 侧最小改动

### 7.1 Go

建议增加统一 HTTP / gRPC 中间件：

- 从请求头读取 `X-Request-Id`
- 没有则生成一个
- 从 OpenTelemetry 当前上下文读取 `trace_id / span_id`
- 读取或透传 `X-Event-Id`
- 所有日志统一从上下文自动带出这些字段

### 7.2 Python

建议增加统一 web 中间件：

- 从请求头读取 `X-Request-Id`
- 没有则生成一个
- 从 OpenTelemetry 当前上下文读取 `trace_id / span_id`
- 读取或透传 `X-Event-Id`
- 使用 `contextvars + logging.Filter` 自动注入日志字段

## 8. 对 auto_inspection 后续要做什么

方案确认后，建议再推进以下代码改造：

1. 扩展 OpenSearch 日志模板
   - 增加 `biz_line / trace_id / span_id / request_id / event_id`
2. 扩展日志查询
   - 支持按这些字段过滤
3. 新增关联分析接口
   - 例如 `correlate_logs` 或 `investigate_business_line`
4. 再挂到 MCP
   - 让 Codex 能直接按业务线、请求、事件做串联分析

## 9. 当前建议结论

最小改动路线固定为：

- 采集器层：
  - 控制台日志继续用 Fluent Bit
  - 文件日志继续用 Filebeat
- 应用层：
  - 先统一 `request_id`
  - 再补 `trace_id / span_id`
  - 异步链路再补 `event_id`
- 检索层：
  - 把这些字段接入 OpenSearch 模板和查询接口
- 智能分析层：
  - 最后集中到 MCP / Skill 中做跨服务串联分析

配套样例文件：

- [filebeat-business-logs.example.yml](/D:/code/auto_inspection/docs/examples/filebeat-business-logs.example.yml)
- [go_request_context_middleware.go](/D:/code/auto_inspection/docs/examples/go_request_context_middleware.go)
- [python_request_context_middleware.py](/D:/code/auto_inspection/docs/examples/python_request_context_middleware.py)
