# MCP / Skill 变更记录：GitLab 发布证据与 AI Gateway 接入准备

## 1. 基本信息

- 日期：2026-04-28
- 操作人：Codex
- 需求来源：用户要求继续完善 MCP 和 Skill，并推动到 AI Gateway
- 关联对话摘要：在已接入 Argo CD 自动同步的基础上，增加 GitLab GitOps 发布证据读取能力，并补充 AI Gateway 接入说明
- 变更主题：GitLab 只读发布证据、MCP 工具、Skill helper 命令和 AI Gateway 文档
- 影响范围：RCA backend、MCP server、Evidence Pack、Skill helper、GitOps Deployment 清单、中文文档

## 2. 需求原文

```text
可以开始下一步，主义写入变更文档
```

上文上下文：

```text
下一步怎么继续完善我的mcp和skill 如何推动到aigateway
```

## 3. 目标

- 增加 GitLab GitOps 仓库的只读发布证据查询能力。
- 暴露 GitLab commit、pipeline、release context 到 backend API 和 MCP tools。
- 让 Evidence Pack 能按 `app_name` 携带 GitLab + Argo CD 发布上下文。
- 更新 Skill helper，方便通过命令行查询 GitLab 发布证据。
- 编写 AI Gateway 接入说明。
- 将本次变更同步到变更文档。

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改基础设施配置：否
- 是否允许生成 investigation 记录：否
- 是否涉及凭证变更：否
- 是否涉及远端部署：否

说明：

```text
本次只新增只读查询能力和 GitOps 清单中的环境变量引用，没有创建 GitLab token、没有写入明文凭证、没有执行 kubectl apply、没有触发 Argo CD sync、没有推送 GitLab 仓库。
GitLab token 通过 Kubernetes Secret `observability/auto-inspection-rca-gitlab` 的 `GITLAB_TOKEN` 键注入，清单中设置 optional=true，未创建 Secret 时不会阻断当前 Pod 模板渲染。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 新增 | `auto_inspection/gitlab_integration.py` | GitLab 只读 API 集成，支持 commits、commit detail、pipelines、release context |
| 修改 | `auto_inspection/dashboard_server.py` | 新增 `/api/gitlab/*` backend API，并允许 Evidence Pack 接收发布上下文字段 |
| 修改 | `auto_inspection/backend_client.py` | 新增 GitLab backend client 方法 |
| 修改 | `auto_inspection/auto_inspection_mcp.py` | 新增 GitLab MCP tools，并扩展 `get_context_pack` 输入 |
| 修改 | `auto_inspection/context_pack.py` | Evidence Pack 可按 `app_name` 聚合 GitLab + Argo CD release context |
| 修改 | `config.example.json` | 增加 GitLab 只读配置示例 |
| 修改 | `deploy/rca-service/deployment.yaml` | RCA backend/MCP 增加 GitLab 只读环境变量引用 |
| 修改 | `worktrees/gitops-manifests/clusters/test/observability/auto-inspection-rca/deployment.yaml` | GitOps 同步路径增加 GitLab 只读环境变量引用 |
| 修改 | `worktrees/gitops-manifests/source/deploy/rca-service/deployment.yaml` | GitOps source 副本同步 GitLab 环境变量引用 |
| 修改 | `worktrees/gitops-manifests/source/yaml/rca-service/deployment.yaml` | GitOps source 副本同步 GitLab 环境变量引用 |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/SKILL.md` | Skill 增加 GitLab GitOps 发布证据工作流和命令示例 |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/scripts/auto_inspection_backend.py` | Skill helper 增加 GitLab / release context 命令 |
| 新增 | `docs/cn/aigateway_mcp_integration.md` | AI Gateway 接入 auto_inspection MCP 说明 |
| 新增 | `docs/cn/change_records/2026-04-28-gitlab-release-evidence-mcp-aigateway.md` | 本次变更记录 |

## 6. 新增或修改的 MCP 工具

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `gitlab_recent_commits` | 新增 | `project_id`, `ref`, `limit`, `since`, `until` | GitLab API | 是 | 查询 GitOps 仓库最近提交 |
| `gitlab_commit_detail` | 新增 | `project_id`, `sha` | GitLab API | 是 | 查询指定 commit 元数据和 stats |
| `gitlab_pipeline_status` | 新增 | `project_id`, `ref`, `sha`, `status`, `limit` | GitLab API | 是 | 查询 pipeline 状态 |
| `gitlab_release_context` | 新增 | `project_id`, `app_name`, `ref`, `sha`, `history_limit`, `limit` | GitLab API + Argo CD API | 是 | 聚合 Argo CD revision/health/diff 与 GitLab commit/pipeline |
| `get_context_pack` | 修改 | 新增 `app_name`, `ref`, `sha`, `project_id` 等可选字段 | RCA backend | 是 | Evidence Pack 可携带发布上下文 |

## 7. 新增或修改的 Skill 能力

- 触发场景：用户询问发布、GitLab、commit、pipeline、Argo CD revision、发布是否导致故障、AI Gateway 接入。
- 推荐工作流：
  1. `release-context --app-name observability --ref main`
  2. `argocd-diff --app-name observability`
  3. `context-workload --namespace <ns> --workload-name <name> --app-name observability`
  4. 必要时补 `gitlab-commit --sha <revision>` 或 `gitlab-pipelines --sha <revision>`
- 输出格式：发布事实、Git revision、pipeline 状态、Argo CD sync/health、OutOfSync、相关日志/事件/指标、建议动作。
- 回退方式：MCP 不可用时 helper 回退 backend HTTP API；GitLab token 未配置时返回 `configured=false`，仍可使用 Argo CD 和其他 RCA 证据。

## 8. 后端 API 变化

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/gitlab/recent-commits` | 新增 | 是 | 查询 GitLab 最近提交 |
| `GET` | `/api/gitlab/commit-detail` | 新增 | 是 | 查询 GitLab commit 详情 |
| `GET` | `/api/gitlab/pipeline-status` | 新增 | 是 | 查询 GitLab pipeline 状态 |
| `GET` | `/api/gitlab/release-context` | 新增 | 是 | 聚合 GitLab 与 Argo CD 发布上下文 |
| `GET` | `/api/context/pod` / `/api/context/workload` / `/api/context/service` | 修改 | 是 | 支持 `app_name` / `sha` / `project_id` 等发布上下文字段 |

## 9. 部署动作

- 是否同步到 NFS：是
- NFS 路径：`192.168.48.206:/srv/nfs/observability/auto-inspection-rca`
- 是否重启 Deployment：是
- Deployment：`auto-inspection-rca`
- Namespace：`observability`
- 备份文件：远端 NFS `.backup/20260428_142731`

命令或动作摘要：

```powershell
# 同步源码与文档到远端 NFS，并备份旧文件到 .backup/20260428_142731

# 创建 GitLab 只读 token Secret，token 未写入 Git 或文档
kubectl -n observability create secret generic auto-inspection-rca-gitlab --from-literal=GITLAB_TOKEN=<readonly-token>

# 重启 RCA 服务加载 NFS 上的新代码
kubectl rollout restart deployment/auto-inspection-rca -n observability
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=300s

# 通过 GitOps 推送 Deployment 环境变量引用，由 Argo CD 自动同步
cd worktrees/gitops-manifests
git add clusters/test/observability/auto-inspection-rca/deployment.yaml source/deploy/rca-service/deployment.yaml source/yaml/rca-service/deployment.yaml
git commit -m "add gitlab readonly release evidence env"
git push origin main

# 触发 Argo CD refresh，并等待自动同步完成
kubectl annotate application observability -n argocd argocd.argoproj.io/refresh=hard --overwrite
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=300s
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| Python 语法检查 | `python -m py_compile ...` | 通过 | 通过 |
| JSON 配置示例 | `python -m json.tool config.example.json` | 通过 | 通过 |
| Kustomize 渲染 | `kubectl kustomize worktrees\gitops-manifests\clusters\test\observability` | 通过 | 通过 |
| MCP 工具注册 | import `TOOLS` 检查工具名 | 出现 GitLab 工具 | 通过，4 个新工具均存在 |
| Skill helper 命令 | `auto_inspection_backend.py --help` | 出现 GitLab 命令 | 通过 |
| Backend GitLab API | 本地临时 backend `/api/gitlab/recent-commits` | token 未配置时返回 `configured=false` | 通过 |
| Release context API | 本地临时 backend `/api/gitlab/release-context` | 未配置凭证时返回只读错误，不写入任何资源 | 通过 |
| 脱敏检查 | `Select-String` 检查明文密码 / token | 无真实凭证 | 通过，仅保留 `<readonly-token>` 占位符 |
| NFS 同步 | `kubectl cp` 到 `/opt/rca` | 远端源码更新 | 通过，备份戳 `20260428_142731` |
| GitLab Secret | `kubectl apply` Secret | Secret 存在且不写入 Git | 通过，`auto-inspection-rca-gitlab` 已创建 |
| GitOps push | `git push origin main` | 推送成功 | 通过，commit `6da6da8` |
| Argo CD 同步 | `kubectl get application observability -n argocd` | `Synced / Healthy` 且 revision 为新提交 | 通过，`Synced / Healthy / 6da6da83d0a30132142f9c038631cf835361629c` |
| 集群 rollout | `kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=300s` | rollout 完成 | 通过 |
| 远端 GitLab API | `gitlab-commits --limit 1 --prefer-backend` | 返回最新提交 | 通过，返回 `6da6da8 add gitlab readonly release evidence env` |
| 远端 release context | `release-context --app-name observability --prefer-backend` | 返回 Argo + GitLab 发布证据 | 通过，`meta.status=ok`，revision `6da6da8` |
| 远端 MCP tools/list | JSON-RPC `tools/list` | 出现 GitLab 工具 | 通过，`tool_count=26`，包含 `gitlab_recent_commits` / `gitlab_release_context` |
| Pod 内语法检查 | `python -m py_compile ...` | 通过 | 通过 |

补充说明：

```text
远端 backend 已通过 NFS 源码同步和 Deployment rollout 部署新代码。
GitOps 清单已推送到 GitLab，Argo CD 已自动同步到 revision 6da6da83d0a30132142f9c038631cf835361629c。
GitLab pipeline 查询返回空列表，表示当前 GitOps 仓库没有匹配该 commit 的 pipeline 记录。
```

## 11. 回滚方式

- 本地文件回滚：通过 Git revert 或删除本次新增文件并还原修改文件。
- 远端 NFS 备份文件：`.backup/20260428_142731`。
- Deployment 回滚方式：回滚 Git commit 后由 Argo CD 同步，或恢复 NFS 备份后滚动重启。

```powershell
git revert <commit>
git push origin main
kubectl rollout restart deployment/auto-inspection-rca -n observability
```

## 12. 已知限制

- 当前 GitLab release context 覆盖 commit 和 pipeline，尚未接入 MR、tag、artifact、image digest。
- AI Gateway 文档是接入规范，还没有在网关侧完成注册和验收。

## 13. 后续建议

- 创建 `auto-inspection-rca-gitlab` Secret，使用最小权限 GitLab token：`read_api`、`read_repository`。
- 将 GitOps 工作目录变更提交到 `platform/gitops-manifests`，让 Argo CD 自动同步。
- 在 AI Gateway 中注册 `auto-inspection-mcp`，只开放只读工具白名单。
- 继续扩展 GitLab MR、tag、artifact、image digest，并把镜像 digest 与 Kubernetes workload image 关联。
