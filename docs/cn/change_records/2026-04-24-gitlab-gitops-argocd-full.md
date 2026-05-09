# MCP / Skill 变更记录：GitLab GitOps + Argo CD 全量接入

## 1. 基本信息

- 日期：2026-04-24
- 操作人：Codex
- 需求来源：用户要求按变更记录模板执行
- 关联对话摘要：修复 GitLab root 登录，创建 GitOps 仓库结构，同步本地 `deploy/`、`yaml/` 和集群当前运行资源，安装 Argo CD，创建 `observability` 应用，MCP 只读查询 Argo CD
- 变更主题：GitLab GitOps 仓库 + Argo CD 应用交付闭环
- 影响范围：node206 GitLab、测试 Kubernetes 集群、Argo CD、RCA backend/MCP、文档

## 2. 需求原文

```text
按变更记录模板执行：全量接入 GitLab GitOps + Argo CD，先修复 GitLab root 登录，创建 GitOps 仓库结构，同步 deploy/yaml 和集群里正在运行得到 GitLab，安装 Argo CD，创建 observability 应用，MCP 只读查询 Argo CD，写入 docs/cn/change_records。
```

## 3. 目标

- 修复 GitLab root 登录凭证。
- 创建 GitLab `platform/gitops-manifests` 仓库。
- 同步本地 `deploy/`、`yaml/` 到 GitLab。
- 导出测试集群 `observability` namespace 当前运行资源，生成清洗后的运行快照。
- 安装 Argo CD 并创建 `observability` Application。
- 将 RCA backend 的 Argo CD 查询环境变量纳入 GitOps 管理。
- MCP 保持只读查询 Argo CD，不执行 sync / rollback / delete。

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改 Argo CD Application：否
- 是否允许 MCP 修改 GitLab 仓库：否
- 是否涉及凭证变更：是，GitLab root 密码重置；创建 GitLab bootstrap token；创建 Argo CD `rca-reader` 只读 token
- 是否涉及远端部署：是，GitLab / Argo CD / GitOps / RCA backend 环境变量

说明：

```text
本次“全量接入”包含由 Codex 执行的部署与 GitOps 初始化动作。
MCP 工具本身仍保持只读，只通过 RCA backend 查询 Argo CD API，不执行 sync、rollback、delete、kubectl apply 或服务器命令。
Argo CD token 存储在 Kubernetes Secret `observability/auto-inspection-rca-argocd`，没有写入 Git 仓库或文档。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 修改 | `deploy/rca-service/deployment.yaml` | 增加 `ARGOCD_SERVER`、`ARGOCD_VERIFY_TLS`、`ARGOCD_TOKEN` 环境变量引用 |
| 新增 | `worktrees/gitops-manifests/` | 本地 GitOps 仓库工作目录 |
| 新增 | `worktrees/gitops-manifests/source/deploy` | 本地 `deploy/` 目录副本 |
| 新增 | `worktrees/gitops-manifests/source/yaml` | 本地 `yaml/` 目录副本 |
| 新增 | `worktrees/gitops-manifests/clusters/test/observability` | Argo CD 同步路径 |
| 新增 | `worktrees/gitops-manifests/clusters/test/observability-live-export` | 集群当前运行资源清洗快照 |
| 新增 | `worktrees/gitops-manifests/apps/observability-project.yaml` | Argo CD AppProject |
| 新增 | `worktrees/gitops-manifests/apps/observability-application.yaml` | Argo CD Application |
| 新增 | `docs/cn/change_records/2026-04-24-gitlab-gitops-argocd-full.md` | 本次变更记录 |

## 6. 新增或修改的 MCP 工具

本次没有新增 MCP 工具，复用上一阶段已接入的 Argo CD 只读工具：

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `argocd_app_status` | 复用 | `app_name`, `refresh` | Argo CD API | 是 | 查询应用状态、health、sync、Git revision |
| `argocd_app_history` | 复用 | `app_name`, `limit` | Argo CD API | 是 | 查询同步历史 |
| `argocd_diff_summary` | 复用 | `app_name`, `refresh` | Argo CD API | 是 | 查询资源 diff summary |

## 7. 新增或修改的 Skill 能力

本次没有新增 Skill 命令，复用：

- `argocd-status`
- `argocd-history`
- `argocd-diff`

已验证这些命令可通过 MCP 读取真实 Argo CD 数据。

## 8. 后端 API 变化

本次没有新增 backend API，复用：

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/argocd/app-status` | 复用 | 是 | 查询 Application 状态 |
| `GET` | `/api/argocd/app-history` | 复用 | 是 | 查询 Application history |
| `GET` | `/api/argocd/diff-summary` | 复用 | 是 | 查询资源 diff summary |

## 9. 部署动作

- GitLab 地址：`http://192.168.48.206:8929`
- GitLab 仓库：`http://192.168.48.206:8929/platform/gitops-manifests`
- Argo CD namespace：`argocd`
- Argo CD UI：`https://192.168.48.200:32443`
- Argo CD Application：`argocd/observability`
- Argo CD 同步路径：`clusters/test/observability`
- RCA Argo CD token Secret：`observability/auto-inspection-rca-argocd`

关键动作摘要：

```powershell
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
kubectl apply -f worktrees/gitops-manifests/apps/observability-project.yaml
kubectl apply -f worktrees/gitops-manifests/apps/observability-application.yaml
kubectl -n observability create secret generic auto-inspection-rca-argocd --from-literal=ARGOCD_TOKEN=<redacted>
git push origin main
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| GitLab 登录修复 | Rails 重置 root 密码 | root 可登录 | 通过 |
| GitLab 仓库 | `platform/gitops-manifests` | 仓库存在 | 通过 |
| GitOps 推送 | `git push origin main` | 推送成功 | 通过，提交 `4204d5d`、`e8d8bb6` |
| Kustomize 构建 | `kubectl kustomize clusters/test/observability` | 构建成功 | 通过 |
| 运行快照构建 | `kubectl kustomize clusters/test/observability-live-export` | 构建成功 | 通过 |
| Argo CD 安装 | `kubectl get pods -n argocd` | 核心组件 Running | 通过 |
| Argo CD 应用 | `kubectl get application observability -n argocd` | `Synced / Healthy` | 通过，revision `e8d8bb6e4246d8a856506b257284c4bd0de2c884` |
| Argo CD UI | `curl -k -I https://192.168.48.200:32443/` | HTTP 200 | 通过 |
| RCA backend 查询 | `GET /api/argocd/app-status?app_name=observability` | configured=true | 通过 |
| MCP / Skill 查询 | `argocd-history`、`argocd-diff` | 返回真实 Argo CD 数据 | 通过 |

## 11. 回滚方式

- Argo CD Application 回滚：

```powershell
kubectl delete application observability -n argocd
kubectl delete appproject observability -n argocd
```

- Argo CD 安装回滚：

```powershell
kubectl delete namespace argocd
```

- GitOps 仓库回滚：

```powershell
cd worktrees/gitops-manifests
git revert <commit>
git push
```

- RCA Argo CD token 回滚：

```powershell
kubectl delete secret auto-inspection-rca-argocd -n observability
```

## 12. 已知限制

- `applicationsets.argoproj.io` CRD 在安装时出现 annotation 超长报错，但本次使用的 `Application` / `AppProject` CRD 已安装成功，不影响当前 `observability` 应用。
- GitOps 仓库保留了本地已有 Secret YAML，生产环境建议迁移到 SealedSecret、ExternalSecret 或 SOPS。
- `observability-live-export` 是运行快照，已移除 `status`、`uid`、`resourceVersion`、`managedFields` 等运行时字段，但不作为当前 Argo CD 自动同步路径。
- Argo CD UI 使用自签/集群证书，浏览器访问 `https://192.168.48.200:32443` 时可能需要接受证书提示。

## 13. 后续建议

- 把 `db/mysql-31326` 和 `monitoring/prometheus` 也拆成独立 Argo CD Application。
- 将 GitLab token、Argo CD token 纳入正式凭证管理，而不是临时 bootstrap token。
- 将仓库内明文 Secret 逐步替换为 SealedSecret / ExternalSecret / SOPS。
- 后续接入 GitLab 只读 MCP 工具，查询 commit、MR、pipeline、发布人和镜像 digest。
