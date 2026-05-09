# MCP / Skill 变更记录：发布接入 Argo CD 状态复核

## 1. 基本信息

- 日期：2026-04-28
- 操作人：Codex
- 需求来源：用户要求确认“目前发布是否接入 Argo”，如果未接入则先接入，并同步到变更文档
- 关联对话摘要：复核 GitOps 清单、Argo CD Application、RCA backend 只读查询能力和当前同步状态
- 变更主题：发布链路已接入 Argo CD 的确认与文档同步
- 影响范围：GitLab GitOps 仓库、Argo CD `observability` Application、RCA backend/MCP、变更记录文档

## 2. 需求原文

```text
目前我的发布接入了argo么，如果没有，先接入argo，另外同步到变更文档里面去
```

## 3. 目标

- 确认当前发布链路是否已经由 Argo CD 管理。
- 确认 RCA backend/MCP 是否可以只读查询 Argo CD 状态。
- 如未接入则补齐 Argo CD 接入；如已接入则记录当前证据。
- 将确认结果同步到变更记录。

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改基础设施配置：否
- 是否允许生成 investigation 记录：否
- 是否涉及凭证变更：否
- 是否涉及远端部署：否

说明：

```text
本次为只读复核和文档记录。没有执行 Argo CD sync、rollback、delete、kubectl apply、Secret 修改或 GitLab 仓库推送。
当前发布已经接入 Argo CD，因此没有重复创建 Application / AppProject。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 新增 | `docs/cn/change_records/2026-04-28-argocd-release-integration-verified.md` | 记录发布接入 Argo CD 的复核结果 |

## 6. 新增或修改的 MCP 工具

本次没有新增或修改 MCP 工具，复用已有只读工具：

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `argocd_app_status` | 复用 | `app_name` | Argo CD API | 是 | 查询 Application health、sync、Git revision、镜像和资源状态 |
| `argocd_diff_summary` | 复用 | `app_name` | Argo CD API | 是 | 查询资源同步状态和 OutOfSync 摘要 |

## 7. 新增或修改的 Skill 能力

本次没有新增或修改 Skill 能力，复用已有 `auto-inspection-rca` 能力：

- 触发场景：用户询问 GitOps、Argo CD 同步状态、应用健康状态、发布变更是否关联故障。
- 推荐工作流：优先使用 `argocd-status`，再用 `argocd-history` / `argocd-diff` 补充证据。
- 输出格式：说明应用名、sync、health、revision、repo/path、OutOfSync 资源。
- 回退方式：如果 MCP 不可用，使用 RCA backend HTTP / helper script 只读查询。

## 8. 后端 API 变化

本次没有后端 API 变化，复用已有 API：

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/argocd/app-status` | 复用 | 是 | 查询 Argo CD Application 状态 |
| `GET` | `/api/argocd/diff-summary` | 复用 | 是 | 查询资源同步摘要 |

## 9. 部署动作

- 是否同步到 NFS：否
- NFS 路径：不适用
- 是否重启 Deployment：否
- Deployment：不适用
- Namespace：不适用
- 备份文件：不适用

命令或动作摘要：

```powershell
kubectl get application observability -n argocd -o wide
kubectl get appproject observability -n argocd -o name
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py argocd-status --app-name observability
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py argocd-diff --app-name observability
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| GitOps 工作目录状态 | `git status --short` in `worktrees/gitops-manifests` | 无未提交改动 | 通过 |
| GitOps 提交记录 | `git log --oneline -5` | 存在 Argo/RCA 接入提交 | 通过，看到 `e8d8bb6 configure rca backend argocd readonly access` 和当前 `2f1d217` |
| Argo CD Application | `kubectl get application observability -n argocd -o wide` | Application 存在且同步健康 | 通过，`Synced / Healthy` |
| Argo CD Project | `kubectl get appproject observability -n argocd -o name` | AppProject 存在 | 通过，`appproject.argoproj.io/observability` |
| RCA 只读状态查询 | `argocd-status --app-name observability` | `configured=true`，返回真实应用状态 | 通过 |
| RCA diff 查询 | `argocd-diff --app-name observability` | 资源同步状态可读 | 通过，`resource_status_counts={"Synced": 39}`，`out_of_sync_resources=[]` |

关键状态：

```text
Application: observability
Namespace: argocd
Project: observability
Repo: http://192.168.48.206:8929/platform/gitops-manifests.git
Path: clusters/test/observability
Target revision: main
Sync status: Synced
Health status: Healthy
Revision: 2f1d217bd43b7588c7b76c1ba0d3e7278c9f41bc
Last operation: Succeeded, 2026-04-28T03:44:01Z -> 2026-04-28T03:44:03Z
OutOfSync resources: 0
```

## 11. 回滚方式

本次只新增文档，没有发布或基础设施变更。

- 本地文件回滚：删除本变更记录文件或通过 Git revert 回滚文档提交。
- 远端 NFS 备份文件：不适用。
- Deployment 回滚方式：不适用。

```powershell
Remove-Item docs\cn\change_records\2026-04-28-argocd-release-integration-verified.md
```

## 12. 已知限制

- 当前只确认 `observability` 发布链路已经接入 Argo CD。
- `db/mysql-31326` 和 `monitoring/prometheus` 是否拆成独立 Argo CD Application 仍需后续规划。
- 当前 diff summary 基于 Argo CD Application resource status，不拉取完整 manifest diff。

## 13. 后续建议

- 将 `db/mysql-31326`、`monitoring/prometheus` 拆成独立 Application，便于分应用发布和回滚。
- 把 GitLab commit、MR、pipeline、镜像 digest 接入 RCA Evidence Pack，增强发布与故障的因果关联。
- 将仓库内明文 Secret 迁移到 SealedSecret、ExternalSecret 或 SOPS。
