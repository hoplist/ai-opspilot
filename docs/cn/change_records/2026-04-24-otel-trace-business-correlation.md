# MCP / Skill 变更记录：OpenTelemetry Trace 与业务关联字段

## 1. 基本信息

- 日期：2026-04-24
- 操作人：Codex
- 需求来源：用户要求按变更记录模板接入 OpenTelemetry Trace + 业务关联字段
- 关联对话摘要：用户说明后端 Pod 可能叫 `workflow-server`，前端叫 `workflow-web`，域名类似 `workflow.tpo.xzoa.com`；当前测试集群没有真实域名和对应服务，先接入字段和只读查询能力
- 变更主题：新增业务日志、Trace、业务上下文关联的只读 API / MCP / Skill 能力
- 影响范围：backend API、OpenSearch template、MCP server、Codex Skill、部署配置、文档

## 2. 需求原文

```text
按变更记录模板执行：接入 OpenTelemetry Trace + 业务关联字段，MCP 只读，写入 docs/cn/change_records。
另外业务比如后端pod会叫如workflow-server 前端就是workflow-web，另外域名也是workflow.tpo.xzoa.com这种形式，需要怎么串联起来，目前我测试集群没有域名和对应服务，先接入 OpenTelemetry Trace + 业务关联字这些
```

## 3. 目标

- 将业务关联字段扩展到日志查询能力中
- 新增 OpenTelemetry Trace 查询能力
- 新增业务上下文推断与关联能力
- 支持 `workflow-server -> workflow-web -> workflow.tpo.xzoa.com` 这种默认命名推断
- 保持 MCP 只读，不允许直接操作服务器或修改 Kubernetes 资源
- 部署到当前共享 RCA 服务并验证

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改基础设施配置：否
- 是否允许生成 investigation 记录：否
- 是否涉及凭证变更：否
- 是否涉及远端部署：是

说明：

```text
本次新增能力只查询 OpenSearch logs/traces，并做业务命名推断。
MCP 不 SSH、不执行 shell、不修改 Kubernetes、不重启业务服务、不修复资源。
部署动作由 Codex 作为本次实现流程执行，不属于 MCP 工具能力。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 修改 | `auto_inspection/config.py` | 新增 trace 索引和业务命名规则配置 |
| 修改 | `auto_inspection/opensearch_bootstrap.py` | 扩展日志字段模板，新增 trace index template |
| 修改 | `auto_inspection/log_search.py` | 支持业务字段过滤与返回 |
| 新增 | `auto_inspection/business_correlation.py` | 新增业务命名推断、trace 查询、业务上下文关联逻辑 |
| 修改 | `auto_inspection/dashboard_server.py` | 新增 backend 只读 API |
| 修改 | `auto_inspection/backend_client.py` | 新增 backend client 方法 |
| 修改 | `auto_inspection/auto_inspection_mcp.py` | 新增 MCP tools |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/SKILL.md` | 增加业务关联触发场景、命名规则、示例 |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/scripts/auto_inspection_backend.py` | 新增 Skill CLI 命令 |
| 修改 | `deploy/rca-service/configmap.yaml` | 增加 trace 索引和业务命名配置 |
| 修改 | `docs/codex_mcp_integration.md` | 增加新工具说明 |
| 新增 | `docs/cn/otel_business_correlation.md` | 增加 Trace + 业务关联字段说明 |
| 修改 | `docs/cn/README.md` | 增加新文档入口 |
| 新增 | `docs/cn/change_records/2026-04-24-otel-trace-business-correlation.md` | 本次变更记录 |

## 6. 新增或修改的 MCP 工具

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `search_business_logs` | 新增 | `service`, `domain`, `route`, `trace_id`, `request_id`, `tenant_id`, `user_id`, `order_id`, `error_code`, `version` 等 | `logs-k8s-*` | 是 | 按业务字段搜索日志 |
| `search_traces` | 新增 | `trace_id`, `span_id`, `service`, `domain`, `route`, `request_id`, `event_id`, `business_key`, `error` | `otel-traces-*` | 是 | 搜索 OpenTelemetry span |
| `correlate_business_context` | 新增 | `service`, `backend_service`, `frontend_service`, `domain`, `business_key`, 业务关联字段 | `logs-k8s-*`, `otel-traces-*` | 是 | 推断业务关系并组合日志与 trace |

## 7. 新增或修改的 Skill 能力

- 触发场景：用户提到业务服务、前端服务、域名、请求 ID、trace ID、route、租户、用户、订单、错误码、版本
- 推荐工作流：优先用 `correlate-business`，再按需用 `business-logs` 或 `traces`
- 输出格式：说明推断出的 backend/frontend/domain、日志命中、trace 命中、缺失字段和下一步建议
- 回退方式：MCP 不可用时调用 backend HTTP API

新增 Skill 命令：

- `business-logs`
- `traces`
- `correlate-business`

## 8. 后端 API 变化

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/search/business-logs` | 新增 | 是 | 按业务字段查询日志 |
| `GET` | `/api/traces/search` | 新增 | 是 | 查询 OpenTelemetry trace span |
| `GET` | `/api/business/correlate` | 新增 | 是 | 推断业务上下文并关联日志和 trace |

## 9. 部署动作

- 是否同步到 NFS：是
- NFS 路径：`192.168.48.206:/srv/nfs/observability/auto-inspection-rca`
- 是否重启 Deployment：是
- Deployment：`auto-inspection-rca`
- Namespace：`observability`
- NFS 备份时间戳：`20260424_153019`

备份文件示例：

```text
/srv/nfs/observability/auto-inspection-rca/auto_inspection/config.py.bak.20260424_153019
/srv/nfs/observability/auto-inspection-rca/auto_inspection/log_search.py.bak.20260424_153019
/srv/nfs/observability/auto-inspection-rca/auto_inspection/business_correlation.py.bak.20260424_153019
/srv/nfs/observability/auto-inspection-rca/auto_inspection/dashboard_server.py.bak.20260424_153019
/srv/nfs/observability/auto-inspection-rca/auto_inspection/backend_client.py.bak.20260424_153019
/srv/nfs/observability/auto-inspection-rca/auto_inspection/auto_inspection_mcp.py.bak.20260424_153019
/srv/nfs/observability/auto-inspection-rca/auto_inspection/opensearch_bootstrap.py.bak.20260424_153019
```

命令或动作摘要：

```powershell
python -m py_compile auto_inspection\config.py auto_inspection\log_search.py auto_inspection\business_correlation.py auto_inspection\dashboard_server.py auto_inspection\backend_client.py auto_inspection\auto_inspection_mcp.py

# 通过 SSH/SFTP 同步到 192.168.48.206 NFS 源码目录

kubectl apply -f deploy\rca-service\configmap.yaml
kubectl rollout restart deployment/auto-inspection-rca -n observability
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=180s
kubectl exec -n observability deployment/auto-inspection-rca -c backend -- python bootstrap_opensearch.py
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| Python 语法检查 | `python -m py_compile ...` | 通过 | 通过 |
| Skill CLI 帮助 | `business-logs --help`, `traces --help`, `correlate-business --help` | 出现参数 | 通过 |
| Deployment rollout | `kubectl rollout status ...` | 成功滚动 | 通过 |
| OpenSearch template | `python bootstrap_opensearch.py` | logs/events/traces acknowledged | 通过 |
| Backend business correlate | `/api/business/correlate?service=workflow-server` | 推断 workflow 关系 | 通过 |
| Backend business logs | `/api/search/business-logs?service=workflow-server` | 返回 ok | 通过，当前 0 条 |
| Backend traces | `/api/traces/search?service=workflow-server` | 返回 ok | 通过，当前 0 条 |
| MCP tools/list | JSON-RPC `tools/list` | 出现 3 个新工具 | 通过，工具数 15 |
| MCP correlate | JSON-RPC `tools/call` | 返回业务上下文 | 通过 |
| Skill correlate | `correlate-business --service workflow-server` | 返回业务上下文 | 通过 |

业务推断验证结果：

```json
{
  "business_key": "workflow",
  "backend_service": "workflow-server",
  "frontend_service": "workflow-web",
  "domains": ["workflow.tpo.xzoa.com"]
}
```

当前日志和 trace 返回 0 条，是因为测试集群尚未接入真实 `workflow` 服务、域名和 trace 数据。

## 11. 回滚方式

本地文件回滚：

```powershell
# 从版本控制或备份恢复本次修改文件
```

远端 NFS 回滚：

```powershell
# 在 192.168.48.206 上恢复对应 .bak.20260424_153019 文件
cp /srv/nfs/observability/auto-inspection-rca/auto_inspection/auto_inspection_mcp.py.bak.20260424_153019 /srv/nfs/observability/auto-inspection-rca/auto_inspection/auto_inspection_mcp.py
```

Deployment 回滚：

```powershell
kubectl rollout restart deployment/auto-inspection-rca -n observability
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=180s
```

## 12. 已知限制

- 当前测试集群没有真实 `workflow-server`、`workflow-web`、`workflow.tpo.xzoa.com` 数据
- 当前仅建立 trace 查询索引和只读查询能力，尚未部署 OTel Collector trace pipeline
- 上下游依赖图需要真实 trace span 数据后才能展示
- 业务字段需要应用日志或采集器实际写入后才能命中

## 13. 后续建议

- 让后端服务日志输出 `trace_id`、`request_id`、`route`、`error_code`、`version`
- 让前端或网关生成并透传 `request_id`
- 接入 OTel SDK / OTel Collector，将 trace 写入 `otel-traces-*`
- 对非标准服务名用 `BUSINESS_SERVICE_MAP` 配置显式映射
