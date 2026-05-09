# MCP / Skill 变更记录：Argo CD 插件与 GitLab 部署

## 1. 基本信息

- 日期：2026-04-24
- 操作人：Codex
- 需求来源：用户要求按变更记录模板执行
- 关联对话摘要：用户要求在 node206 上用 Docker 启动 GitLab，并接入 Argo CD 插件，支持应用状态、同步历史、Git revision、资源 diff summary 查询；MCP 保持只读；写入 `docs/cn/change_records`
- 变更主题：Argo CD 发布证据只读查询 + GitLab Docker 部署文件
- 影响范围：RCA backend、MCP server、Codex Skill、本地文档、node206 `/opt/gitlab`

## 2. 需求原文

```text
可以  不需要只读，直接起全量得，另外在node206上面使用docker起一个gitlab，部署文件放在206/opt/gitlab/下面，然后按变更记录模板执行：接入 Argo CD 插件，支持应用状态、同步历史、Git revision、资源 diff summary 查询，MCP 只读，写入 docs/cn/change_records。
```

## 3. 目标

- 在 `192.168.48.206:/opt/gitlab` 准备 GitLab Docker Compose 部署。
- 新增 Argo CD 应用状态、同步历史、Git revision、资源 diff summary 查询能力。
- MCP 工具保持只读，只读取 RCA backend 和 Argo CD GET API。
- 将本次新增能力写入文档和变更记录。

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改 Argo CD Application：否
- 是否允许 MCP 修改基础设施配置：否
- 是否涉及凭证变更：否
- 是否涉及远端部署：是，GitLab Docker Compose 文件写入 node206；RCA backend/MCP 代码同步到 NFS 后滚动重启

说明：

```text
用户允许在 node206 上直接部署完整 GitLab，这是本次交付部署动作，不属于 MCP 能力。
Argo CD MCP 工具仍保持只读，只查询 Argo CD 应用状态、同步历史、Git revision 和资源状态摘要。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 新增 | `auto_inspection/argocd_integration.py` | Argo CD 只读 API 客户端与输出归一化 |
| 修改 | `auto_inspection/dashboard_server.py` | 新增 `/api/argocd/*` 后端 API |
| 修改 | `auto_inspection/backend_client.py` | 新增 Argo CD backend client 方法 |
| 修改 | `auto_inspection/auto_inspection_mcp.py` | 新增 3 个 Argo CD MCP 工具 |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/SKILL.md` | 增加 Argo CD 排查触发场景、工作流和示例 |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/scripts/auto_inspection_backend.py` | 增加 `argocd-status`、`argocd-history`、`argocd-diff` 命令 |
| 新增 | `docs/cn/argocd_readonly_integration.md` | Argo CD 只读接入说明 |
| 修改 | `docs/codex_mcp_integration.md` | 更新 MCP 工具清单与 Argo CD 工具说明 |
| 修改 | `docs/cn/README.md` | 增加 Argo CD 文档入口 |
| 新增 | `docs/cn/change_records/2026-04-24-argocd-plugin-gitlab.md` | 本次变更记录 |
| 新增 | `192.168.48.206:/opt/gitlab/docker-compose.yml` | GitLab Docker Compose 部署文件 |
| 新增 | `192.168.48.206:/opt/gitlab/README.md` | GitLab 访问与运维说明 |
| 新增 | `192.168.48.206:/opt/gitlab/start.sh` | GitLab 启动脚本 |

## 6. 新增或修改的 MCP 工具

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `argocd_app_status` | 新增 | `app_name`, `refresh` | Argo CD API | 是 | 查询应用 health、sync、Git revision、images、resources |
| `argocd_app_history` | 新增 | `app_name`, `limit` | Argo CD API | 是 | 查询应用同步历史和 latest revision |
| `argocd_diff_summary` | 新增 | `app_name`, `refresh` | Argo CD API | 是 | 查询资源状态计数和 OutOfSync 资源摘要 |

## 7. 新增或修改的 Skill 能力

- 触发场景：用户询问 Argo CD、GitOps、应用同步状态、应用健康、Git revision、sync history、diff summary。
- 推荐工作流：先用 `argocd-status` 看应用状态，再用 `argocd-history` 查发布历史，最后用 `argocd-diff` 看资源差异摘要。
- 输出格式：结构化 JSON，包含 `mode`、`configured`、`safety`、`application`、`history`、`resource_status_counts`、`out_of_sync_resources`。
- 回退方式：优先 MCP；MCP 不可用时回退 backend HTTP。

## 8. 后端 API 变化

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/argocd/app-status` | 新增 | 是 | 查询 Argo CD Application 状态 |
| `GET` | `/api/argocd/app-history` | 新增 | 是 | 查询 Application sync history |
| `GET` | `/api/argocd/diff-summary` | 新增 | 是 | 查询资源 diff summary |

## 9. 部署动作

- 是否同步到 NFS：是
- NFS 路径：`192.168.48.206:/srv/nfs/observability/auto-inspection-rca`
- 是否重启 Deployment：是
- Deployment：`auto-inspection-rca`
- Namespace：`observability`
- GitLab 部署目录：`192.168.48.206:/opt/gitlab`

GitLab 部署摘要：

```bash
cd /opt/gitlab
docker compose up -d
```

由于 node206 访问 Docker Hub 超时，已将镜像源从 `gitlab/gitlab-ce:latest` 调整为 `docker.m.daocloud.io/gitlab/gitlab-ce:latest`，并保留备份 `docker-compose.yml.bak.20260424_1708`。

GitLab 端口：

```text
Web: http://192.168.48.206:8929
SSH: ssh://git@192.168.48.206:2224/<group>/<repo>.git
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| node206 Docker | `docker --version && docker compose version` | Docker / Compose 可用 | 通过 |
| GitLab 部署文件 | `/opt/gitlab/docker-compose.yml`, `README.md`, `start.sh` | 文件存在 | 通过 |
| GitLab 启动 | `docker compose up -d` | 容器启动 | 通过，容器 `healthy`，`/users/sign_in` 返回 HTTP 200 |
| 语法检查 | `python -m py_compile ...` | 通过 | 通过 |
| Backend API | `GET /api/argocd/app-status` | 未配置时返回 `configured=false` | 通过 |
| MCP tools/list | JSON-RPC `tools/list` | 出现 3 个 Argo CD 工具 | 通过，`tool_count=21` |
| Skill CLI | `argocd-status --help` 等 | 命令可见 | 通过 |

## 11. 回滚方式

- 本地文件回滚：使用版本控制或备份恢复本次修改文件。
- 远端 NFS 回滚：使用同步前备份覆盖对应文件。
- GitLab 回滚：

```bash
cd /opt/gitlab
docker compose down
```

如需删除数据，由人工确认后再删除 `/opt/gitlab/config`、`/opt/gitlab/logs`、`/opt/gitlab/data`。

## 12. 已知限制

- Argo CD 接入需要配置 `ARGOCD_SERVER` 和 `ARGOCD_TOKEN`，当前未内置凭证。
- `argocd_diff_summary` 基于 Application resource status 做摘要，不拉取完整 manifest diff。
- GitLab 镜像首次从 Docker Hub 拉取发生网络超时，已临时改用 DaoCloud 镜像代理。

## 13. 后续建议

- 配置 Argo CD 只读 token，并将 `ARGOCD_SERVER` / `ARGOCD_TOKEN` 注入 RCA backend Deployment。
- GitLab 启动后创建 GitOps 仓库，用于保存应用 manifests / Helm values / 发布记录。
- 后续接入 GitLab API，只读查询 commit、merge request、pipeline、image digest 与发布记录。
