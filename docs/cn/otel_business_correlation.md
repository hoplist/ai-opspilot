# OpenTelemetry Trace 与业务关联字段接入说明

本文记录当前 `auto_inspection` 已接入的 OpenTelemetry Trace 与业务关联字段能力。所有能力均为只读查询，不允许 MCP 直接操作服务器或修改 Kubernetes 资源。

## 1. 当前已落地能力

新增 backend 只读 API：

- `GET /api/search/business-logs`
- `GET /api/traces/search`
- `GET /api/business/correlate`

新增 MCP 工具：

- `search_business_logs`
- `search_traces`
- `correlate_business_context`

新增 Skill 命令：

- `business-logs`
- `traces`
- `correlate-business`

## 2. 业务命名规则

当前默认按以下规则推断业务关系：

- 后端服务：`<业务名>-server`
- 前端服务：`<业务名>-web`
- 域名：`<业务名>.tpo.xzoa.com`

示例：

| 输入 | 推断结果 |
| --- | --- |
| `workflow-server` | business key: `workflow` |
| `workflow-server` | backend service: `workflow-server` |
| `workflow-server` | frontend service: `workflow-web` |
| `workflow-server` | domain: `workflow.tpo.xzoa.com` |

当前测试集群没有真实 `workflow` 域名和对应服务，因此验证重点是确认推断、查询和只读边界正常。

## 3. 可配置项

配置位于 RCA 服务配置中：

- `OPENSEARCH_INDEX_TRACES`
  默认：`otel-traces-*`
- `BUSINESS_DOMAIN_SUFFIXES`
  默认：`["tpo.xzoa.com"]`
- `BUSINESS_BACKEND_SUFFIX`
  默认：`-server`
- `BUSINESS_FRONTEND_SUFFIX`
  默认：`-web`
- `BUSINESS_SERVICE_MAP`
  默认：`{}`

如果某些业务不是标准命名，可以用 `BUSINESS_SERVICE_MAP` 显式配置。

示例：

```json
{
  "BUSINESS_SERVICE_MAP": {
    "workflow": {
      "backend_service": "workflow-server",
      "frontend_service": "workflow-web",
      "domains": ["workflow.tpo.xzoa.com"]
    }
  }
}
```

## 4. 推荐日志字段

业务日志建议输出以下字段：

- `service`
- `biz_line`
- `business_key`
- `frontend_service`
- `backend_service`
- `domain`
- `route`
- `version`
- `trace_id`
- `span_id`
- `parent_span_id`
- `request_id`
- `event_id`
- `tenant_id`
- `user_id`
- `order_id`
- `error_code`
- `severity`
- `message`

最低可用组合：

- `service`
- `trace_id`
- `request_id`
- `route`
- `error_code`
- `version`

## 5. 推荐 Trace 字段

OpenTelemetry span 建议保留：

- `trace_id`
- `span_id`
- `parent_span_id`
- `service.name`
- `span.name`
- `span.kind`
- `duration_ms`
- `status.code`
- `error`
- `http.route`
- `http.method`
- `http.status_code`
- `rpc.method`
- `db.system`
- `db.operation`
- `request_id`
- `event_id`
- `business_key`

当前默认 trace 索引：

- `otel-traces-*`

## 6. 使用示例

按后端服务串联：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py correlate-business --service workflow-server --range-hours 6
```

按域名串联：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py correlate-business --domain workflow.tpo.xzoa.com --range-hours 6
```

按请求 ID 查业务日志：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py business-logs --request-id req-xxx --range-hours 6
```

按 trace ID 查 trace：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py traces --trace-id trace-xxx --range-hours 6
```

## 7. 当前验证结果

验证输入：

```text
service=workflow-server
```

推断结果：

```json
{
  "business_key": "workflow",
  "backend_service": "workflow-server",
  "frontend_service": "workflow-web",
  "domains": ["workflow.tpo.xzoa.com"]
}
```

因为测试集群当前没有 `workflow` 真实服务和域名数据，日志和 trace 查询返回 `0` 条是预期结果。

## 8. 后续接入真实数据时要做什么

1. 应用日志输出标准业务字段。
2. Filebeat / Fluent Bit 保留这些字段写入 `logs-k8s-*`。
3. OTel Collector 将 trace 写入 `otel-traces-*` 或配置的 trace 索引。
4. 服务名、域名、route、request_id、trace_id 在日志和 trace 中保持一致。
5. 如果业务命名不满足默认规则，在 `BUSINESS_SERVICE_MAP` 中显式配置。

## 9. 安全边界

这些能力只允许：

- 查询日志
- 查询 trace
- 推断业务服务关系
- 返回候选上下游线索

不允许：

- SSH 到服务器
- 执行 shell 命令
- 修改 Kubernetes 资源
- 修改基础设施配置
- 自动重启或扩缩容服务
