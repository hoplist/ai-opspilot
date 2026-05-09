# Argo CD 发布变更只读接入说明

本文记录 RCA / MCP 对 Argo CD 的只读接入方式，用于把应用状态、同步历史、Git revision 与资源 diff summary 纳入排障证据。

## 1. 目标

- 查询 Argo CD Application 当前 health / sync 状态。
- 查询当前 Git revision、目标 revision、仓库地址、部署路径和镜像摘要。
- 查询 Application sync history，辅助判断问题是否出现在某次 GitOps 发布之后。
- 查询资源级 sync 状态和 out-of-sync 资源摘要。
- MCP 保持只读：不执行 sync、rollback、delete、override，也不直接操作 Kubernetes。

## 2. 新增 MCP 工具

| 工具 | 作用 | 常用入参 | 只读边界 |
| --- | --- | --- | --- |
| `argocd_app_status` | 查询应用 health、sync、Git revision、镜像和资源状态 | `app_name`, `refresh` | 只调用 Argo CD GET API |
| `argocd_app_history` | 查询应用同步历史和 latest revision | `app_name`, `limit` | 只读取 Application status history |
| `argocd_diff_summary` | 查询资源 sync/diff 摘要与 out-of-sync resources | `app_name`, `refresh` | 只读取 Application resource status |

## 3. 后端 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/argocd/app-status` | 查询单个或全部 Argo CD Application 状态 |
| `GET` | `/api/argocd/app-history` | 查询指定 Application 的 sync history |
| `GET` | `/api/argocd/diff-summary` | 查询指定 Application 的资源 diff summary |

## 4. 配置

RCA backend 通过环境变量连接 Argo CD：

```bash
ARGOCD_SERVER=http://argocd.example.com
ARGOCD_TOKEN=<readonly-token>
ARGOCD_VERIFY_TLS=false
ARGOCD_TIMEOUT=20
```

建议 token 绑定 Argo CD 只读角色，只允许 `applications get` / `applications list` 之类读取权限。

## 5. 使用示例

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py argocd-status --app-name workflow
```

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py argocd-history --app-name workflow --limit 10
```

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py argocd-diff --app-name workflow
```

## 6. 输出字段

- `mode`: 只读模式，例如 `read_only_argocd_app_status`
- `configured`: 是否已配置 `ARGOCD_SERVER` 和 `ARGOCD_TOKEN`
- `safety`: `server_commands=not_allowed`、`kubernetes_mutations=not_allowed`、`argocd_mutations=not_allowed`
- `applications` / `application`: 应用基础状态、health、sync、Git revision、repo、path、destination
- `history`: sync history
- `resource_status_counts`: 资源状态计数
- `out_of_sync_resources`: OutOfSync 资源摘要

## 7. 已知限制

- 当前 diff summary 基于 Argo CD Application status 中的资源同步状态，不拉取完整 manifest diff。
- 如果 Argo CD 没有开启或没有配置只读 token，API 会返回 `configured=false` 和缺失配置说明。
- 该接入不创建、同步、回滚或修改 Argo CD Application。

## 8. GitLab 部署补充

本次同时在 `192.168.48.206:/opt/gitlab` 准备了 GitLab Docker Compose 部署文件：

- Web: `http://192.168.48.206:8929`
- SSH: `ssh://git@192.168.48.206:2224/<group>/<repo>.git`
- 部署目录：`/opt/gitlab`
- 镜像：`docker.m.daocloud.io/gitlab/gitlab-ce:latest`

GitLab 用于后续承载 GitOps 仓库、应用 manifest、Helm values 或发布记录。
