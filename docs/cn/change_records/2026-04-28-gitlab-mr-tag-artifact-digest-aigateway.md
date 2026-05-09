# 2026-04-28 GitLab MR/Tag/Artifact/Image Digest 与 AI Gateway 只读接入

## 背景

- RCA 服务已接入 Argo CD 与 GitLab 只读发布证据。
- 本次继续补齐 GitLab MR、tag、artifact、image digest，并把镜像 digest 与 Kubernetes workload image 关联。
- 目标是在 AI Gateway 中只开放 `auto-inspection-mcp` 的只读工具白名单。

## 已完成

- 确认 `observability/auto-inspection-rca-gitlab` Secret 已存在，用于最小权限 GitLab token。
- 后端新增 GitLab 只读 API：
  - `/api/gitlab/merge-requests`
  - `/api/gitlab/tags`
  - `/api/gitlab/artifacts`
  - `/api/gitlab/image-digest-context`
- MCP 新增只读工具：
  - `gitlab_merge_requests`
  - `gitlab_tags`
  - `gitlab_artifacts`
  - `gitlab_image_digest_context`
- `gitlab_release_context` 已扩展 commit、pipeline、MR、tag、artifact、image digest 证据。
- `image_digest_context` 会聚合：
  - Argo CD Application 镜像摘要。
  - Kubernetes workload 当前镜像。
  - GitLab Container Registry repository/tag digest。
  - 输入镜像、Argo CD 镜像、Kubernetes workload 镜像之间的匹配关系。
- Codex `auto-inspection-rca` skill helper 新增命令：
  - `gitlab-mrs`
  - `gitlab-tags`
  - `gitlab-artifacts`
  - `image-digest-context`
- AI Gateway 注册模板已写入：
  - `docs/cn/aigateway_auto_inspection_mcp_registration.json`
  - `platform/gitops-manifests/apps/aigateway-auto-inspection-mcp-registration.json`
- GitOps Deployment 增加发布证据版本注解，用于触发 Argo CD 自动同步。

## AI Gateway 状态

- 当前集群未发现 AI Gateway namespace、Deployment 或 Service。
- 因此本次无法直接调用 AI Gateway API 完成在线注册。
- 已准备可提交给 AI Gateway 的注册 payload，并限定只读工具白名单；后续网关服务可用后直接使用该 payload 注册。

## 安全边界

- GitLab token 不写入 GitOps 仓库。
- GitLab token 仅通过 Kubernetes Secret 注入 RCA backend/MCP 容器。
- AI Gateway 注册 payload 不包含 GitLab、Argo CD、Kubernetes 或 OpenSearch 凭据。
- MCP 白名单不包含 apply、delete、sync、rollback、push、merge、pipeline trigger 等变更类工具。

## 验证记录

- 本地 Python 编译检查通过。
- AI Gateway 注册 JSON 格式检查通过。
- GitOps 已提交并推送到 `platform/gitops-manifests`：
  - commit：`a30b5b5 add readonly mcp gateway registration`
- Argo CD `observability` Application 已自动同步到新 revision：
  - sync：`Synced`
  - health：`Healthy`
  - revision：`a30b5b526ee64efe74d5bc4f90d7723caf7147a7`
- `auto-inspection-rca` Deployment rollout 成功。
- Pod 内 Python 编译检查通过。
- 远端 backend 验证通过：
  - `/api/gitlab/merge-requests?limit=1`：`status=ok`，`configured=true`
  - `/api/gitlab/tags?limit=1`：`status=ok`，`configured=true`
  - `/api/gitlab/artifacts?limit=1`：`status=ok`，`configured=true`
  - `/api/gitlab/image-digest-context?...`：`status=ok`，`configured=true`
- 远端 MCP 验证通过：
  - `tools/list` 共返回 30 个工具。
  - `gitlab_merge_requests`、`gitlab_tags`、`gitlab_artifacts`、`gitlab_image_digest_context` 均已出现在工具列表。
