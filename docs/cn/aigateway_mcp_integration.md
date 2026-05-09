# AI Gateway 接入 auto_inspection MCP 说明

本文记录如何把 `auto_inspection` RCA 能力推动到 AI Gateway，目标是让 AI Gateway 统一暴露只读排障、发布证据和 GitOps 状态查询能力。

## 1. 接入目标

- AI Gateway 作为统一 AI 入口，注册 `auto-inspection-mcp`。
- RCA backend/MCP 继续负责读取 OpenSearch、Prometheus、Kubernetes metadata、Argo CD 和 GitLab。
- AI Gateway 只调用 MCP 工具，不直接操作 Kubernetes、Argo CD 或 GitLab。
- GitLab / Argo CD token 由 Kubernetes Secret 或网关侧凭证管理注入，不写入 Git 仓库。

## 2. 推荐链路

```text
User / Agent
  -> AI Gateway
  -> auto-inspection MCP
  -> RCA backend
  -> OpenSearch / Prometheus / Argo CD / GitLab
```

发布证据链：

```text
GitLab MR / commit / pipeline
  -> Argo CD sync revision
  -> Kubernetes resources
  -> RCA Evidence Pack
  -> AI Gateway answer
```

## 3. MCP 服务地址

集群内推荐地址：

```text
http://auto-inspection-rca.observability.svc.cluster.local:18081/mcp
```

NodePort / 外部测试地址按环境配置：

```text
http://192.168.48.200:32181/mcp
```

## 4. 建议暴露的只读工具

基础健康：

- `health`
- `health_details`

RCA 证据：

- `get_context_pack`
- `diagnose_pod`
- `investigate`
- `search_logs`
- `search_events`
- `search_business_logs`
- `search_traces`
- `correlate_business_context`

发布 / 变更：

- `release_for_workload`
- `release_recent_changes`
- `correlate_change_with_incident`
- `argocd_app_status`
- `argocd_app_history`
- `argocd_diff_summary`
- `gitlab_recent_commits`
- `gitlab_commit_detail`
- `gitlab_pipeline_status`
- `gitlab_release_context`
- `gitlab_merge_requests`
- `gitlab_tags`
- `gitlab_artifacts`
- `gitlab_image_digest_context`

## 5. 凭证注入

RCA Deployment 需要只读凭证：

```text
ARGOCD_SERVER=https://argocd-server.argocd.svc.cluster.local
ARGOCD_TOKEN=<from secret>
GITLAB_URL=http://192.168.48.206:8929
GITLAB_PROJECT_ID=platform/gitops-manifests
GITLAB_TOKEN=<from secret>
```

GitLab token 建议最小权限：

- `read_api`
- `read_repository`

Secret 示例：

```powershell
kubectl -n observability create secret generic auto-inspection-rca-gitlab --from-literal=GITLAB_TOKEN=<readonly-token>
```

## 6. AI Gateway 策略建议

- 工具白名单：只允许上述只读工具。
- 审计字段：记录用户、工具名、参数摘要、时间、响应状态。
- 超时：普通查询 30-60 秒，Evidence Pack 90-120 秒。
- 限流：按用户和工具限流，避免日志/trace 查询压垮 OpenSearch。
- 脱敏：不要在网关日志里记录 token、Secret、完整认证头。
- 禁止动作：不通过 AI Gateway 暴露 sync、rollback、delete、apply、scale、restart、merge、push 等写操作。

## 7. 发布排障推荐工作流

当用户问“这次发布有没有问题”“是不是发布引起故障”时：

1. 调用 `gitlab_release_context`，读取 Argo CD revision、GitLab commit、MR、tag、artifact、pipeline 和 image digest context。
2. 调用 `argocd_diff_summary`，检查 OutOfSync 资源。
3. 调用 `get_context_pack`，按 namespace / workload / service 聚合日志、事件、指标和发布证据。
4. 输出发布事实、异常证据、可能原因和下一步建议。

## 8. 当前限制

- GitLab 接入是只读 API，不会创建 MR、触发 pipeline 或修改仓库。
- `gitlab_release_context` 依赖 GitLab token；未配置时会返回 `configured=false`。
- 当前 GitLab release context 覆盖 commit / MR / tag / artifact / pipeline / image digest / Argo CD 状态。
