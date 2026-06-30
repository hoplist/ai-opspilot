# OpsPilot 当前准确信息入口

更新时间：2026-06-30

本文只记录当前测试环境真实入口。历史迁移细节继续保留在
`docs/change_records/`。

## 仓库入口

| 用途 | GitLab 仓库 | 说明 |
| --- | --- | --- |
| OpsPilot core 源码 | `tpo/platform/opspilot/opspilot-core` | 当前核心源码和 CI 所在仓库。旧 `platform/opspilot` 保留 registry 历史和兼容入口。 |
| Runtime config | `tpo/platform/opspilot/opspilot-config` | `opspilot-core` 通过 git-sync 读取。人工配置集群、数据源、凭证台账、服务目录。 |
| Runtime skills | `tpo/platform/opspilot/opspilot-skills` | `opspilot-core` 通过 git-sync 读取。客户端不需要自带完整 skills registry。 |
| GitOps desired state | `tpo/deploy/gitops-manifests` | Argo CD 读取的部署期望状态。不是应用源码仓库。 |
| Sandbox demo | `tpo/sandbox/devex/*` | 临时验证和 demo 服务。 |

## 标准发布链路

```text
node206 GitLab
-> node206 GitLab Runner
-> BuildKit rootless 打包镜像
-> GitLab Registry
-> tpo/deploy/gitops-manifests
-> node200 Argo CD
-> Kubernetes rollout
-> OpsPilot release status 验证
```

当前 OpsPilot core 已验证：

```text
pipeline: 196 success
image: 192.168.48.206:5050/platform/opspilot/opspilot-core:5c14eb0e
gitops: matches_cluster
argocd: Synced / Healthy
kubernetes: ready=1 desired=1 updated=1 available=1
```

## 回滚方式

优先使用 OpsPilot：

```powershell
opspilot release history --service opspilot-core --output human
opspilot release rollback --service opspilot-core --to <tag-or-revision> --confirm --output human
opspilot release status --service opspilot-core --output human
```

回滚本质是修改 GitOps 期望镜像，再由 Argo CD 收敛。不要直接手改
Deployment 镜像作为常规回滚方式。

## 凭证管理

| 凭证 | 存放位置 | 用途 |
| --- | --- | --- |
| `GITOPS_TOKEN` | `tpo/platform/opspilot/opspilot-core` GitLab CI/CD variable | CI 写入 `tpo/deploy/gitops-manifests`。 |
| `opspilot-config-secrets` | node200 `opspilot` namespace Secret | git-sync 读取 runtime config 仓库。 |
| `opspilot-skills-secrets` | node200 `opspilot` namespace Secret | git-sync 读取 runtime skills 仓库。 |
| `opspilot-release-secrets` | node200 `opspilot` namespace Secret | OpsPilot 查询 GitLab、Registry、GitOps 等发布证据。 |

敏感值不写入普通文档。凭证台账只记录用途、归属、权限和存放位置。

## 当前已知非阻塞缺口

| 缺口 | 含义 | 当前处理 |
| --- | --- | --- |
| `pod_metrics_missing` | 发布状态没有拿到 Pod CPU/Memory 指标样本。 | 不阻塞发布；影响资源趋势 RCA。后续检查 Prometheus 标签、scrape 和数据源映射。 |
| `elk_logs_missing` | 外部服务日志源未配置或查询失败。 | 不阻塞发布；Kubernetes Pod 日志作为 fallback。后续按服务接 ELK/OpenSearch/OpenObserve adapter。 |
| `quality_job_not_found` | 可选质量检查 Job 不存在。 | optional=true，不阻塞发布。 |

## 当前边界

- Argo CD 只管理 node200 测试集群。
- 正式集群暂不接入 Argo CD。
- 多 Kibana/ES、多集群、JumpServer/CMDB、前置网关目前以配置模型和文档为主，按需接入。
- 旧 `platform/opspilot` 暂不删除，用于保留历史 registry tag 和兼容线索。
