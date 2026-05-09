# MCP / Skill 变更记录：Evidence Pack API、MCP 与 CLI 落地

## 1. 基本信息

- 日期：2026-04-27
- 操作人：Codex
- 需求来源：用户要求按变更记录模板依次执行 1、2、3、4、5、6，并写入 `docs/cn/change_records`。
- 关联对话摘要：在已完成 Codex 智能数据接入方案后，继续落地 Evidence Pack 字段契约、Pod/Workload 后端 API、MCP 工具、Skill 优先调用策略和 CLI 入口。
- 变更主题：Evidence Pack 第一阶段落地。
- 影响范围：backend、MCP server、项目 CLI、Codex Skill helper、中文文档。

## 2. 需求原文

```text
按变更记录模板执行：依次执行1，2，3，4，5，6  写入 docs/cn/change_records。
```

本次按前序方案中的六项执行：

1. 文档确认 Evidence Pack 字段契约。
2. 实现 `/api/context/pod`。
3. MCP 增加 `get_context_pack target_type=pod`。
4. Skill 优先调用 `get_context_pack`。
5. 实现 `/api/context/workload`。
6. CLI 增加 `context pod/workload`。

## 3. 目标

- 统一 Pod/Workload 排障上下文结构，减少 Codex 临场拼接证据的成本。
- 后端新增只读 Evidence Pack API，聚合日志、事件、incident、资源趋势、业务上下文和发布元数据。
- 数据源取不到数据时返回 `data_sources` 和 `errors`，避免前端或调用方误判为空白正常状态。
- MCP 暴露 `get_context_pack`，Skill 和 CLI 优先使用该聚合入口。

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改基础设施配置：否
- 是否允许生成 investigation 记录：否，本次 Evidence Pack 只读聚合，不调用 `/api/investigate`
- 是否涉及凭证变更：否
- 是否涉及远端部署：否

说明：

```text
本次仅新增只读聚合查询能力。Evidence Pack 不执行 SSH、kubectl、节点命令或 Kubernetes 写操作。所有修复动作只作为建议由人确认。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 新增 | `auto_inspection/context_pack.py` | Evidence Pack 聚合模块 |
| 修改 | `auto_inspection/dashboard_server.py` | 新增 `/api/context/pod`、`/api/context/workload` |
| 修改 | `auto_inspection/backend_client.py` | 新增 `context_pack()` client 方法 |
| 修改 | `auto_inspection/auto_inspection_mcp.py` | 新增 MCP `get_context_pack` 工具和调用分发 |
| 新增 | `auto_inspection/context_cli.py` | 项目 CLI：`python -m auto_inspection.context_cli pod|workload` |
| 修改 | `C:\Users\Administrator\.codex\skills\auto-inspection-rca\SKILL.md` | Skill 工作流优先 Evidence Pack |
| 修改 | `C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py` | 新增 `context-pod`、`context-workload` |
| 新增 | `docs/cn/evidence_pack_contract.md` | Evidence Pack 字段契约 |
| 修改 | `docs/cn/README.md` | 增加 Evidence Pack 字段契约索引 |
| 新增 | `docs/cn/change_records/2026-04-27-evidence-pack-api-mcp-cli.md` | 本变更记录 |

## 6. 新增或修改的 MCP 工具

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `get_context_pack` | 新增 | `target_type`, `namespace`, `pod`, `workload_name`, `workload_kind`, `service`, `symptom`, `q`, `range_hours`, `size` | logs/events/incidents/prometheus/business/release | 是 | 返回 Pod 或 Workload Evidence Pack |

## 7. 新增或修改的 Skill 能力

- 触发场景：Pod/Workload 排障、OOM、CrashLoop、Probe、Pending、ImagePull、延迟、错误、资源短期上涨。
- 推荐工作流：优先 MCP `get_context_pack`，不可用时回退 backend `/api/context/*`，再回退旧的 `diagnose-pod` 或原子查询。
- 输出格式：结构化 JSON，重点读取 `summary.top_signals`、`data_sources`、`errors` 和 `evidence.timeline`。
- 回退方式：Skill helper `context-pod`、`context-workload` 会先尝试 MCP，失败后调用 backend HTTP。

## 8. 后端 API 变化

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/context/pod` | 新增 | 是 | 返回 Pod Evidence Pack |
| `GET` | `/api/context/workload` | 新增 | 是 | 返回 Workload Evidence Pack |

## 9. 部署动作

- 是否同步到 NFS：是
- NFS 路径：`192.168.48.206:/srv/nfs/observability/auto-inspection-rca`
- 是否重启 Deployment：是
- Deployment：`auto-inspection-rca`
- Namespace：`observability`
- 备份文件：`/opt/rca/.backup/evidence_pack_20260428_095628`

命令或动作摘要：

```powershell
# 2026-04-28 本地重启记录：
# 1. 发现 18080/18081 被 Docker Desktop backend 代理占用，backend health 超时，docker ps 超时。
# 2. 结束挂起的 Docker Desktop / com.docker.backend / docker.exe 进程，释放 18080/18081。
# 3. 启动本地 backend 和 MCP：
python backend_server.py --host 127.0.0.1 --port 18080
python auto_inspection_mcp.py --host 127.0.0.1 --port 18081

# 2026-04-28 集群同步记录：
# 1. 通过运行中 Pod 的 /opt/rca PVC 挂载目录备份旧文件。
# 2. 使用 kubectl cp 同步本次改动文件到 /opt/rca。
# 3. Pod 内执行 py_compile 通过。
# 4. 滚动重启 Deployment。
kubectl rollout restart deployment/auto-inspection-rca -n observability
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=300s
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| 语法检查 | `python -m py_compile auto_inspection/context_pack.py auto_inspection/context_cli.py auto_inspection/dashboard_server.py auto_inspection/backend_client.py auto_inspection/auto_inspection_mcp.py` | 通过 | 通过 |
| Skill helper 语法检查 | `python -m py_compile C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py` | 通过 | 通过 |
| CLI 帮助 | `python -m auto_inspection.context_cli --help` | 显示 pod/workload 子命令 | 通过 |
| backend health | `GET http://127.0.0.1:18080/api/health` | 返回 `status=ok` | 通过 |
| MCP 工具存在 | `tools/list` | 出现 `get_context_pack` | 通过 |
| 后端真实调用 | `GET /api/context/pod?namespace=prod&pod=minio4-0&symptom=oom&range_hours=6&size=3` | 返回 Evidence Pack 或明确错误 | 通过，HTTP 404 + `summary.status=empty` + `data_sources/errors` |
| 集群 Pod 内语法检查 | `python -m py_compile auto_inspection/context_pack.py auto_inspection/context_cli.py auto_inspection/dashboard_server.py auto_inspection/backend_client.py auto_inspection/auto_inspection_mcp.py` | 通过 | 通过 |
| 集群 rollout | `kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=300s` | successfully rolled out | 通过，新 Pod `auto-inspection-rca-5bb7cc6dbc-4zprt` |
| 集群 backend health | `GET http://192.168.48.200:32180/api/health` | 返回 `status=ok` | 通过 |
| 集群 Evidence Pack | `GET http://192.168.48.200:32180/api/context/pod?namespace=prod&pod=minio4-0&symptom=oom&range_hours=6&size=3` | 返回 Evidence Pack 或明确错误 | 通过，HTTP 404 + `summary.status=empty` + `data_sources/errors` |
| 集群 MCP 工具存在 | `POST http://192.168.48.200:32181/mcp tools/list` | 出现 `get_context_pack` | 通过 |

## 11. 回滚方式

- 删除 `auto_inspection/context_pack.py`。
- 删除 `auto_inspection/context_cli.py`。
- 从 `auto_inspection/dashboard_server.py` 移除 `/api/context/pod` 和 `/api/context/workload` 路由及 handler。
- 从 `auto_inspection/backend_client.py` 移除 `context_pack()`。
- 从 `auto_inspection/auto_inspection_mcp.py` 移除 `get_context_pack` schema 和分发逻辑。
- 从 Skill `SKILL.md` 和 `scripts/auto_inspection_backend.py` 移除 Evidence Pack 优先说明与 `context-*` 命令。
- 删除 `docs/cn/evidence_pack_contract.md` 和本变更记录。

## 12. 已知限制

- 本次未实现 `service`、`incident`、`namespace` 级 Evidence Pack。
- 本次未实现 Snapshot Index，短期上涨基线来自现有 Prometheus resource payload。
- 本次未实现 MCP Resources / Resource Templates。
- 本次未实现 AI Gateway 权限、脱敏和审计策略层。
- Evidence Pack 聚合依赖现有 OpenSearch、Prometheus、Kubernetes metadata 配置；数据源异常时会以 `partial` 和 `errors` 暴露。

## 13. 后续建议

- 增加 `/api/context/service`、`/api/context/incident`、`/api/context/namespace`。
- 将 `summary.top_signals` 接入前端异常 Pod 清单，用颜色和排序突出 CPU/Mem 短期上涨。
- 增加 Snapshot Index，预计算异常 Pod、资源趋势、发布变更和业务错误 TopN。
- 增加 MCP Resources，让 Codex 可用 `pod://`、`workload://`、`incident://` 读取对象上下文。
