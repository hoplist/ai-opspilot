# MCP / Skill 变更记录：Context 扩展、Snapshot Index、前端信号与 MCP Resources

## 1. 基本信息

- 日期：2026-04-28
- 操作人：Codex
- 需求来源：用户要求按变更记录模板执行，并写入 `docs/cn/change_records`。
- 关联对话摘要：在 Pod/Workload Evidence Pack 已落地后，继续扩展 Service、Incident、Namespace 上下文，前端异常 Pod 清单突出 CPU/Mem 短期上涨，新增 Snapshot Index，并通过 MCP Resources 暴露对象 URI。
- 变更主题：Evidence Pack 第二阶段能力扩展。
- 影响范围：backend、MCP server、前端资源看板、中文文档。

## 2. 需求原文

```text
按变更记录模板执行：增加 /api/context/service、/api/context/incident、/api/context/namespace。
将 summary.top_signals 接入前端异常 Pod 清单，用颜色和排序突出 CPU/Mem 短期上涨。
增加 Snapshot Index，预计算异常 Pod、资源趋势、发布变更和业务错误 TopN。
增加 MCP Resources，让 Codex 可用 pod://、workload://、incident:// 读取对象上下文。写入 docs/cn/change_records。
```

## 3. 目标

- 扩展 Evidence Pack API 覆盖 `service`、`incident`、`namespace`。
- 让 `/api/resources` 携带 `summary.top_signals`，前端异常 Pod 清单直接合并该信号并按短期上涨优先排序。
- 新增 `/api/snapshot-index`，提供异常 Pod、资源趋势、发布变更和业务错误 TopN。
- 新增 MCP Resources / Resource Templates，支持 `pod://`、`workload://`、`incident://` 读取对象上下文。

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改基础设施配置：否
- 是否允许生成 investigation 记录：否
- 是否涉及凭证变更：否
- 是否涉及远端部署：本地代码待验证后再按部署流程同步

说明：

```text
本次仍然只读取 backend 已配置的数据源。Snapshot Index 是只读预计算摘要，不写 OpenSearch、不写 Kubernetes、不修改配置。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 修改 | `auto_inspection/context_pack.py` | 支持 `service`、`incident`、`namespace` 目标，并导出资源信号函数 |
| 新增 | `auto_inspection/snapshot_index.py` | Snapshot Index TopN 预计算模块 |
| 修改 | `auto_inspection/dashboard_server.py` | 新增 `/api/context/service`、`/api/context/incident`、`/api/context/namespace`、`/api/snapshot-index`，并让 `/api/resources` 附带 `summary.top_signals` |
| 修改 | `auto_inspection/backend_client.py` | 新增 `snapshot_index()` 和 Resource 读取用的 `context_pack_resource()` |
| 修改 | `auto_inspection/auto_inspection_mcp.py` | 新增 MCP Resources / Resource Templates 支持 |
| 修改 | `dashboard/core.js` | 异常 Pod 清单合并 `summary.top_signals`，新增 Signals 列并按短期上涨排序 |
| 修改 | `dashboard/styles.css` | 新增 CPU/Mem 信号标签颜色样式 |
| 修改 | `docs/cn/evidence_pack_contract.md` | 更新 service/incident/namespace、Snapshot Index 和 MCP Resource 契约 |
| 新增 | `docs/cn/change_records/2026-04-28-context-snapshot-mcp-resources.md` | 本变更记录 |

## 6. 新增或修改的 MCP 工具

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `get_context_pack` | 修改 | `target_type=pod/workload/service/incident/namespace` 等 | logs/events/incidents/prometheus/business/release | 是 | 扩展目标类型 |

MCP Resources：

| URI | 类型 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `pod://<cluster>/<namespace>/<pod>` | 新增 | `/api/context/pod` | 是 | 读取 Pod Evidence Pack |
| `workload://<cluster>/<namespace>/<kind>/<name>` | 新增 | `/api/context/workload` | 是 | 读取 Workload Evidence Pack |
| `incident://<incident_id>` | 新增 | `/api/context/incident` | 是 | 读取 Incident Evidence Pack |

## 7. 新增或修改的 Skill 能力

- 触发场景：Codex 可通过 MCP Resource URI 直接读取对象上下文。
- 推荐工作流：打开 Pod/Workload/Incident 对象时优先读取 Resource；主动排障仍可调用 `get_context_pack`。
- 输出格式：MCP `resources/read` 返回 `application/json` Evidence Pack。
- 回退方式：Resource 读取失败时回退对应 `/api/context/*` HTTP API 或 MCP tool。

## 8. 后端 API 变化

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/context/service` | 新增 | 是 | 返回 Service Evidence Pack |
| `GET` | `/api/context/incident` | 新增 | 是 | 返回 Incident Evidence Pack |
| `GET` | `/api/context/namespace` | 新增 | 是 | 返回 Namespace Evidence Pack |
| `GET` | `/api/snapshot-index` | 新增 | 是 | 返回 Snapshot Index TopN |
| `GET` | `/api/resources` | 修改 | 是 | 附带 `summary.top_signals` |

## 9. 部署动作

- 是否同步到 NFS：是
- NFS 路径：`192.168.48.206:/srv/nfs/observability/auto-inspection-rca`，容器内挂载为 `/opt/rca`
- 是否重启 Deployment：是
- Deployment：`auto-inspection-rca`
- Namespace：`observability`
- 新 Pod：`auto-inspection-rca-769dffd549-pz657`
- 备份文件：`/opt/rca/.backup/context_snapshot_20260428_101526`

命令或动作摘要：

```powershell
python -m py_compile auto_inspection/context_pack.py auto_inspection/snapshot_index.py auto_inspection/dashboard_server.py auto_inspection/backend_client.py auto_inspection/auto_inspection_mcp.py
node --check dashboard/core.js
node --check dashboard/app.js
kubectl rollout restart deployment/auto-inspection-rca -n observability
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=300s
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| Python 语法检查 | `python -m py_compile ...` | 通过 | 通过 |
| 前端语法检查 | `node --check dashboard/core.js` | 通过 | 通过 |
| 前端入口语法检查 | `node --check dashboard/app.js` | 通过 | 通过 |
| Context API 本地验证 | `GET /api/context/service`、`/api/context/incident`、`/api/context/namespace` | 返回 Evidence Pack 或明确 empty/error | 通过 |
| Snapshot Index 本地验证 | `GET /api/snapshot-index?namespace=prod&range_hours=6&limit=3` | 返回 TopN 摘要 | 通过 |
| MCP Resource Templates | `resources/templates/list` | 返回 3 个模板 | 通过 |
| MCP Resource Read | `resources/read pod://kubernetes/prod/minio4-0` | 返回 Evidence Pack JSON | 通过 |
| Deployment 同步验证 | `kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=300s` | rollout 成功 | 通过 |
| 集群健康检查 | `GET http://192.168.48.200:32180/api/health` | 返回 `ok` | 通过 |
| 集群 Context API 验证 | `GET /api/context/service`、`/api/context/incident`、`/api/context/namespace` | 返回 Evidence Pack 或明确 empty/error | 通过 |
| 集群 Snapshot Index 验证 | `GET /api/snapshot-index?namespace=prod&range_hours=6&limit=3` | 返回 TopN 摘要 | 通过 |
| 集群 MCP Resource Templates | `resources/templates/list` | 返回 Resource Templates | 通过 |
| 集群 MCP Resource Read | `resources/read pod://kubernetes/prod/minio4-0` | 返回 Evidence Pack JSON | 通过 |

## 11. 回滚方式

- 删除 `auto_inspection/snapshot_index.py`。
- 从 `auto_inspection/context_pack.py` 回退 service/incident/namespace 扩展。
- 从 `auto_inspection/dashboard_server.py` 移除新增 context 路由和 `/api/snapshot-index`。
- 从 `auto_inspection/auto_inspection_mcp.py` 移除 Resources / Resource Templates 支持。
- 从 `dashboard/core.js` 移除 Signals 列、`context_signals` 合并和排序调整。
- 从 `dashboard/styles.css` 移除 `.signal-*` 样式。

## 12. 已知限制

- Snapshot Index 当前是请求时轻量预计算，不是落盘索引。
- 发布变更 TopN 需要传入 `namespace` 才能读取 Kubernetes metadata。
- 业务错误 TopN 依赖 OpenSearch 业务日志字段质量。
- `incident://latest` 当前按 incident 查询关键字读取，后续可改成真正 latest incident Resource。

## 13. 后续建议

- 将 Snapshot Index 定时写入 OpenSearch 或本地热存储，避免每次实时计算。
- 前端增加 Snapshot Index 独立面板，展示异常 Pod、资源趋势、发布变更、业务错误四个 TopN。
- MCP Resources 后续增加 `service://`、`namespace://`。
- 将 `summary.top_signals` 与告警通知策略打通，CPU/Mem 快速上涨达到 alert 阈值时触发通知。

## 14. 集群日志采集排查记录

- 排查对象：`observability/auto-inspection-rca-769dffd549-pz657`
- 原生日志：`kubectl logs -n observability auto-inspection-rca-769dffd549-pz657 -c backend` 可看到后端健康检查和 API 请求日志。
- RCA 查询结果：`/api/search/logs?namespace=observability&pod=auto-inspection-rca-769dffd549-pz657&range_hours=2` 返回 0 条。
- 采集入口：`fluent-bit-logs-rsxzg` 已通过 tail input 发现 `/var/log/containers/auto-inspection-rca-769dffd549-pz657_observability_backend-*.log` 和 `mcp-*.log`。
- 索引侧结果：`logs-k8s-2026.04.28` 中 `pod:auto-inspection-rca-769dffd549-pz657`、`namespace:observability`、`kubernetes.namespace_name:observability` 计数均为 0。
- Fluent Bit 状态：同节点 `fluent-bit-logs-rsxzg` 持续出现大量 `re-schedule retry`，retry id 已超过 2000，说明 OpenSearch output 存在长期重试/积压。
- OpenSearch 状态：集群 `yellow`，单节点副本未分配；写线程池未见 rejected，pending tasks 为 0。
- 初步结论：新 Pod 日志文件没有被漏扫，问题发生在 Fluent Bit 输出到 OpenSearch 的阶段；更像是 Fluent Bit filesystem buffer / 历史 chunk 长期重试积压，导致新日志未及时写入索引。当前未对日志组件做重启、清理 buffer 或配置变更。

## 15. Fluent Bit 卡死修复记录

- 修复时间：2026-04-28 10:24-10:44 CST
- 根因：历史 backlog chunk 携带 `kubernetes.labels.*` 和 `log_processed.*` 原始对象，写入旧索引时触发 OpenSearch dynamic mapping 冲突；同时 Fluent Bit `Retry_Limit False` 导致 400 错误无限重试，形成输出队列积压。
- 配置修复：将 `Retry_Limit False` 调整为 `Retry_Limit 3`，避免不可恢复的 400 bulk 错误永久重试。
- 配置修复：Lua normalize 末尾清理 `record["log_processed"] = nil`，并继续移除 `kubernetes.labels`，避免动态字段类型冲突继续产生。
- 状态修复：暂停 `fluent-bit-logs` 后，隔离三台节点 `/var/lib/fluent-bit/storage` 到 `/var/lib/fluent-bit/storage.quarantine-20260428_103250`，保留 `flb_kube.db` 读取位置。
- 涉及节点：`k8s-master-1`、`k8s-worker-1`、`k8s-worker-2`。
- 集群恢复：`fluent-bit-logs-5p5pp`、`fluent-bit-logs-rtt6m`、`fluent-bit-logs-w4m2z` 均为 `1/1 Running`。
- 验证结果：重启后 60 秒内三只 Fluent Bit Pod 未再出现 `mapper_parsing_exception`、`failed to parse field`、`failed to flush`、`retry` 匹配错误。
- 验证结果：`/api/search/logs?namespace=observability&pod=auto-inspection-rca-769dffd549-pz657&range_hours=2` 返回 `total=135`，新 Pod 日志已可被 RCA 查询。
