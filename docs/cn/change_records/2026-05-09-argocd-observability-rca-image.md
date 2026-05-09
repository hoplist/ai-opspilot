# 2026-05-09 Argo CD 接管 observability RCA 镜像更新

## 背景

node206 目标集群中 Argo CD 已经部署在 `argocd` 命名空间，`observability` Application 跟踪 GitLab 仓库：

- repoURL：`http://192.168.48.206:8929/platform/gitops-manifests.git`
- branch：`main`
- path：`clusters/test/observability`

因此后续更新不应只修改 node206 本地普通目录，而应把变更提交到 GitLab，由 Argo CD 自动同步。

## 本次变更

- 将 `auto-inspection-rca` Deployment 镜像临时更新为公网可拉取地址：
  - `ttl.sh/auto-inspection-rca-20260509-k8s-inventory:24h`
- 镜像 digest：
  - `sha256:0043b240c3b50eabdfd1c52a508a8e0d7fe2fa3183461a98210f80392336f9ac`
- 说明：`ttl.sh` 镜像仅保留 24 小时，用于临时验证公网拉取链路；长期应替换为正式 Docker Hub/GHCR/Quay 公共仓库地址。
- RCA ClusterRole 增加只读 Node 权限：
  - `nodes get/list/watch`
- 保持 `ARGOCD_SERVER` 指向实际 Argo CD 服务：
  - `https://argocd-server.argocd.svc.cluster.local`
- 当前 node200 GitOps 目标集群没有 `xzyc115-19` 节点，hostPath workload 的 `nodeSelector` 调整为当前已有且承载旧 RCA Pod 的节点：
  - `kubernetes.io/hostname: k8s-worker-2`

## 后续使用方式

标准更新流程：

```bash
git clone http://192.168.48.206:8929/platform/gitops-manifests.git
cd gitops-manifests
vim clusters/test/observability/auto-inspection-rca/deployment.yaml
git add clusters/test/observability/auto-inspection-rca source/deploy/rca-service source/yaml/rca-service
git commit -m "Update auto-inspection RCA image"
git push origin main
```

Argo CD 会自动同步 `observability` Application。需要立即触发刷新时：

```bash
kubectl annotate application observability -n argocd argocd.argoproj.io/refresh=hard --overwrite
```

验证：

```bash
kubectl get application observability -n argocd
kubectl rollout status deployment/auto-inspection-rca -n observability
curl "http://<rca-backend>:18080/api/k8s/cluster/overview?limit=10"
```
