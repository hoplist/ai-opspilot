# MCP / Skill 变更记录：发布变更数据只读接入

## 1. 基本信息

- 日期：2026-04-24
- 操作人：Codex
- 需求来源：用户要求按变更记录模板执行
- 关联对话摘要：在 MCP 只读边界下接入发布变更数据，支持 Helm / Deployment / 镜像版本 / ConfigMap 只读查询，并写入 `docs/cn/change_records`
- 变更主题：发布变更数据只读查询与 incident 关联
- 影响范围：RCA backend、MCP server、Codex Skill、本地文档、Kubernetes 只读 RBAC

## 2. 需求原文

```text
按变更记录模板执行：接入发布变更数据，支持 Helm/Deployment/镜像版本/ConfigMap 只读查询，MCP 只读，写入 docs/cn/change_records。
```

## 3. 目标

- 新增发布变更数据只读查询能力。
- 支持按 Pod / Workload 查询 Deployment / StatefulSet / DaemonSet / ReplicaSet 元数据、镜像版本、revision、Helm 常见标注。
- 支持查询近期发布变更和 ConfigMap 元数据 / data keys。
- 支持将 incident 时间窗口与发布变更做候选关联。
- 严格保持 MCP 只读，不允许 MCP 直接操作服务器或修改 Kubernetes 资源。

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改基础设施配置：否
- 是否允许生成 investigation 记录：本次新增工具不生成 investigation
- 是否涉及凭证变更：否
- 是否涉及远端部署：是

说明：

```text
MCP 工具只读取 backend 暴露的只读 API。backend 只通过 Kubernetes API 执行 GET/LIST/WATCH 类读取动作。
不读取 Secret 正文，不执行 Helm/Kubectl 修改命令，不通过 MCP SSH 到服务器。
部署过程中由 Codex 按用户上下文同步代码、应用 RBAC、滚动重启服务；这些是交付部署动作，不是 MCP 能力。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 新增 | `auto_inspection/release_changes.py` | 新增发布变更只读查询服务，读取 workload、镜像、revision、Helm 标注、ConfigMap metadata/data keys |
| 修改 | `auto_inspection/dashboard_server.py` | 新增 `/api/releases/*` 三个只读 API |
| 修改 | `auto_inspection/backend_client.py` | 新增 release 相关 backend client 方法 |
| 修改 | `auto_inspection/auto_inspection_mcp.py` | 新增 3 个 MCP 工具定义与调用处理 |
| 修改 | `deploy/rca-service/clusterrole.yaml` | 扩展只读 RBAC：apps workloads 与 configmaps，verbs 保持 `get/list/watch` |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/SKILL.md` | 增加发布变更排查触发场景与推荐工作流 |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/scripts/auto_inspection_backend.py` | 增加 `release-workload`、`release-changes`、`release-correlate` 命令 |
| 新增 | `docs/cn/release_change_correlation.md` | 新增发布变更数据只读接入说明 |
| 修改 | `docs/codex_mcp_integration.md` | 更新 MCP 工具清单与发布变更工具说明 |
| 修改 | `docs/cn/README.md` | 增加发布变更文档入口 |
| 新增 | `docs/cn/change_records/2026-04-24-release-change-readonly.md` | 本次变更记录 |

## 6. 新增或修改的 MCP 工具

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `release_for_workload` | 新增 | `namespace`, `pod`, `workload_name`, `workload_kind`, `include_configmaps` | Kubernetes workload metadata、ConfigMap metadata | 是 | 查询 Pod / Workload 当前运行版本、镜像、revision、Helm 标注 |
| `release_recent_changes` | 新增 | `namespace`, `range_hours`, `service`, `workload_name`, `limit`, `include_configmaps` | Kubernetes workload metadata、ConfigMap metadata | 是 | 查询近期发布 / 配置变化摘要 |
| `correlate_change_with_incident` | 新增 | `namespace`, `pod`, `workload_name`, `workload_kind`, `range_hours`, `limit` | Kubernetes workload metadata、ConfigMap metadata | 是 | 将异常窗口与发布变更候选做关联 |

## 7. 新增或修改的 Skill 能力

- 触发场景：用户询问“是否是发布导致”“镜像版本是什么”“Deployment revision 有没有变化”“ConfigMap 是否近期变化”“Helm 信息是什么”。
- 推荐工作流：先使用 `release-workload` 查询当前运行版本，再用 `release-changes` 看近期变化，最后用 `release-correlate` 把异常窗口与变更候选串起来。
- 输出格式：保留结构化 JSON，包含 `mode`、`safety`、`workload`、`items`、`correlation`、`errors`、`meta`。
- 回退方式：优先调用 MCP；MCP 不可用时回退到 backend HTTP API。

## 8. 后端 API 变化

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/releases/workload` | 新增 | 是 | 查询 Pod / Workload 发布元数据 |
| `GET` | `/api/releases/recent-changes` | 新增 | 是 | 查询近期 workload / ConfigMap 变化 |
| `GET` | `/api/releases/correlate` | 新增 | 是 | 关联 incident 时间窗口与发布变更 |

## 9. 部署动作

- 是否同步到 NFS：是
- NFS 路径：`192.168.48.206:/srv/nfs/observability/auto-inspection-rca`
- 是否重启 Deployment：是
- Deployment：`auto-inspection-rca`
- Namespace：`observability`
- 备份文件：远端文件已生成 `.bak.20260424_163039`；`release_changes.py` 追加修正前备份为 `.bak.20260424_1642_correlate_filter`

命令或动作摘要：

```powershell
# 同步源码到 NFS，并为远端旧文件生成 .bak.20260424_163039
# 远端语法检查
python -m py_compile auto_inspection/release_changes.py auto_inspection/dashboard_server.py auto_inspection/backend_client.py auto_inspection/auto_inspection_mcp.py

# 应用只读 RBAC 并滚动重启
kubectl apply -f deploy/rca-service/clusterrole.yaml
kubectl rollout restart deployment/auto-inspection-rca -n observability
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=180s

# 修正 correlate 按 Pod owner 收窄后，再次同步 release_changes.py 并滚动重启
scp auto_inspection/release_changes.py root@192.168.48.206:/srv/nfs/observability/auto-inspection-rca/auto_inspection/release_changes.py
kubectl rollout restart deployment/auto-inspection-rca -n observability
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=180s
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| 本地语法检查 | `python -m py_compile auto_inspection/release_changes.py auto_inspection/dashboard_server.py auto_inspection/backend_client.py auto_inspection/auto_inspection_mcp.py C:/Users/Administrator/.codex/skills/auto-inspection-rca/scripts/auto_inspection_backend.py` | 通过 | 通过 |
| Skill CLI help | `release-workload --help`、`release-changes --help`、`release-correlate --help` | 三个命令可见 | 通过 |
| 远端语法检查 | NFS 上执行 `python -m py_compile ...` | 通过 | 通过 |
| Deployment rollout | `kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=180s` | rollout 完成 | 通过 |
| Backend workload API | `GET /api/releases/workload?namespace=observability&pod=opensearch-0` | 返回 workload 与镜像 | 返回 `StatefulSet/opensearch`，镜像 `opensearchproject/opensearch:2.19.5` |
| Backend recent changes API | `GET /api/releases/recent-changes?namespace=observability&range_hours=168&limit=5` | 返回近期变化列表 | 返回 `auto-inspection-rca` ReplicaSet revision 5/6/7 |
| MCP tools/list | JSON-RPC `tools/list` | 出现 3 个新工具 | 通过，`tool_count=18` |
| MCP tools/call | JSON-RPC `tools/call release_for_workload` | 返回结构化只读结果 | 通过，返回 `read_only_release_for_workload` 与安全边界 |
| Skill 调用 | `python ... auto_inspection_backend.py release-workload --namespace observability --pod opensearch-0` | 优先 MCP 调用成功 | 通过 |
| Correlate 收窄 | `GET /api/releases/correlate?namespace=observability&pod=opensearch-0&range_hours=168&limit=5` | 只读关联先解析 Pod owner，再按解析出的 workload 过滤近期变化 | 通过，`opensearch-0` 解析为 `StatefulSet/opensearch`，候选变化收窄到 `opensearch` 相关 workload / ConfigMap |

## 11. 回滚方式

- 本地文件回滚：使用版本控制恢复本次修改文件。
- 远端 NFS 回滚：使用 `.bak.20260424_163039` 备份覆盖对应文件。
- Deployment 回滚方式：

```powershell
kubectl rollout undo deployment/auto-inspection-rca -n observability
```

- RBAC 回滚：移除 `deploy/rca-service/clusterrole.yaml` 中新增的 apps workloads 与 configmaps 只读资源后重新 `kubectl apply`。

## 12. 已知限制

- 当前通过 Kubernetes metadata 推断发布变化，不等价于完整 Helm history。
- 不读取 Secret 正文，因此不会从 Helm release Secret 解析完整历史。
- ConfigMap 当前只返回 metadata 与 data keys，不返回完整 data 值。
- 如果镜像 tag 没有关联 digest / git commit 标注，无法仅靠 Kubernetes metadata 精确定位构建提交。

## 13. 后续建议

- 下一步接入 Argo CD 只读 API，补齐 sync history、Git revision、health、diff 状态。
- 接入镜像仓库只读 API，建立 image tag / digest / commit 的映射。
- 建立发布记录只读索引，把 Helm chart version、ConfigMap checksum、发布人、发布时间、Git commit 统一纳入 RCA 证据。
