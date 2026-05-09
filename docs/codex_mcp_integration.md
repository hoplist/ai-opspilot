# Codex MCP 集成

## 当前状态

当前仓库已经具备两种可供 Codex 调用的接入方式：

1. Skill
2. MCP server

Skill 适合现在就用，MCP 适合正式工具化接入。

当前推荐模式：

`Skill 触发 + MCP 优先 + backend HTTP 回退`

## 已落地文件

- MCP server:
  - [auto_inspection_mcp.py](D:/code/auto_inspection/auto_inspection_mcp.py)
  - [auto_inspection_mcp.py](D:/code/auto_inspection/auto_inspection/auto_inspection_mcp.py)
- Backend client:
  - [backend_client.py](D:/code/auto_inspection/auto_inspection/backend_client.py)
- 启动脚本:
  - [run_auto_inspection_stack.ps1](D:/code/auto_inspection/run_auto_inspection_stack.ps1)

## MCP 地址

- `http://127.0.0.1:18081/mcp`
- 当前按 MCP Streamable HTTP 单端点方式暴露
- 当前协议版本已对齐为 `2025-06-18`

## MCP 工具

- `health`
- `search_logs`
- `search_events`
- `investigate`
- `list_investigations`
- `list_targets`
- `get_investigation`
- `diagnose_pod`
- `search_business_logs`
- `search_traces`
- `correlate_business_context`
- `release_for_workload`
- `release_recent_changes`
- `correlate_change_with_incident`
- `argocd_app_status`
- `argocd_app_history`
- `argocd_diff_summary`

### 复杂问题工具

`diagnose_pod` 用于面向复杂 Pod / Workload 问题生成只读证据包。它会组合：

- backend health details
- RCA investigation summary
- matching logs
- matching Kubernetes events
- matching incidents
- Prometheus resource summary

典型入参：

- `namespace`
- `pod`
- `workload_name`
- `symptom`: `oom`、`crashloop`、`probe`、`pending`、`imagepull`、`latency`、`error`、`unknown`
- `q`
- `range_hours`
- `size`
- `use_ai`

它的定位是“读证据、汇总结论”，不是执行修复。

### OpenTelemetry Trace 与业务关联工具

`search_business_logs` 用于按业务字段检索日志，支持：

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
- `request_id`
- `event_id`
- `tenant_id`
- `user_id`
- `order_id`
- `error_code`

`search_traces` 用于按 OpenTelemetry span 字段检索 trace：

- `trace_id`
- `span_id`
- `service`
- `domain`
- `route`
- `request_id`
- `event_id`
- `business_key`
- `error`

`correlate_business_context` 用于把业务服务、前端、域名、日志和 trace 串起来。

默认命名规则：

- `workflow-server` -> business key: `workflow`
- `workflow-server` -> frontend service: `workflow-web`
- `workflow-server` -> candidate domain: `workflow.tpo.xzoa.com`

可通过配置调整：

- `BUSINESS_DOMAIN_SUFFIXES`
- `BUSINESS_BACKEND_SUFFIX`
- `BUSINESS_FRONTEND_SUFFIX`
- `BUSINESS_SERVICE_MAP`

### 发布变更只读工具

`release_for_workload` 用于查询 Pod / Workload 当前运行的发布元数据，包含：

- workload kind / name / namespace
- container image 与 imagePullPolicy
- replicas / readyReplicas / generation / resourceVersion
- Deployment revision、restart annotation、change-cause
- Helm 常见 label / annotation
- 可选 ConfigMap metadata 与 data keys

`release_recent_changes` 用于查询命名空间内近期 Deployment、StatefulSet、DaemonSet、ReplicaSet、ConfigMap 变化摘要。

`correlate_change_with_incident` 用于把 incident 时间窗口与发布变更候选串起来，辅助判断“发布后异常”。

这些工具保持 MCP 只读边界：不读取 Secret 正文，不执行 Helm/Kubectl 修改命令，不 SSH 到服务器。

### Argo CD 发布证据工具

`argocd_app_status` 用于查询 Argo CD Application health、sync status、Git revision、repo、path、destination、images 与资源摘要。

`argocd_app_history` 用于查询 Application sync history 和 latest revision。

`argocd_diff_summary` 用于查询资源级 sync/diff 摘要和 OutOfSync 资源。

配置环境变量：

- `ARGOCD_SERVER`
- `ARGOCD_TOKEN`
- `ARGOCD_VERIFY_TLS`
- `ARGOCD_TIMEOUT`

这些工具只调用 Argo CD GET API，不执行 sync、rollback、delete、override 或 Kubernetes 变更操作。

## 安全边界

当前 MCP 应保持只读排障边界：

允许：

- 查询 RCA backend
- 查询日志、事件、incident、investigation、Prometheus 资源摘要
- 通过 backend 生成 investigation 记录
- 返回 Dashboards / Discover 链接

禁止：

- SSH 到服务器
- 执行节点 shell 命令
- 修改 Kubernetes 对象
- 删除、重启、扩缩容、cordon、drain、patch、apply 资源
- 修改 OpenSearch、Prometheus、MinIO、MySQL 或集群配置

如果需要修复动作，MCP 只应给出建议动作和证据，由人确认后再通过其他运维流程执行。

## 推荐扩展的数据源

为了让 MCP 能处理内存溢出、CrashLoop、业务调用失败、上下游依赖异常这类复杂问题，建议后续接入以下只读数据源：

- OpenTelemetry Trace：Jaeger、Tempo 或 OpenSearch Trace Analytics，用于 `trace_id`、`span_id`、上下游调用图、慢 span 和失败 span。
- 业务日志关联字段：`service`、`biz_line`、`request_id`、`trace_id`、`span_id`、`event_id`、`tenant_id`、`user_id`、`order_id`、`error_code`、`route`、`version`。
- Kubernetes 元数据：kube-state-metrics、owner、revision、image tag、QoS、requests/limits、last state、restart counters。
- 发布与变更数据：Argo CD、Helm history、Git commit、image digest、ConfigMap/Secret revision、发布时间。
- 运行时指标：cAdvisor、kubelet、node-exporter、JVM/Micrometer、Go runtime、Python runtime、连接池、HTTP client、MQ 指标。
- Profiling / 内存证据：Pyroscope、Parca、JVM heap summary、Go pprof summary、eBPF OOM/process 事件。
- 依赖健康：数据库慢查询摘要、Redis/MQ 延迟和错误、Ingress/Gateway upstream 状态、第三方 API 错误摘要。

推荐做法是：先把这些能力收敛成后端只读 API，再暴露成 MCP 工具。MCP 不直接访问服务器，也不直接操作集群。

## Codex 本机配置

当前已写入：

`C:\Users\Administrator\.codex\config.toml`

```toml
[mcp_servers.autoInspectionRca]
url = "http://127.0.0.1:18081/mcp"
```

## 启动方式

先启动 backend：

```powershell
python backend_server.py --host 127.0.0.1 --port 18080
```

再启动 MCP：

```powershell
python auto_inspection_mcp.py --host 127.0.0.1 --port 18081
```

或者直接：

```powershell
.\run_auto_inspection_stack.ps1
```

## Transport 约定

- `GET /mcp` 可打开 `text/event-stream` SSE 流
- `POST /mcp` 承载 JSON-RPC 请求
- `initialize` 响应头返回 `Mcp-Session-Id`
- 后续 `GET /mcp` 与非 `initialize` 的 `POST /mcp` 需带 `Mcp-Session-Id`
- `notifications/initialized` 返回 `202` 空响应
- 服务端可通过 SSE 主动发送 `notifications/message`
- `DELETE /mcp` 可主动关闭当前 session
- 后续请求可带 `MCP-Protocol-Version: 2025-06-18`

## 结论

现在这台机器上，Codex 已经具备：

- Skill 直接调用本地后端
- MCP server 正式工具化调用本地后端

后续如果要团队分发，推荐再把 `Skill + MCP` 打包成 Plugin。
