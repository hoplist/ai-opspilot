# 2026-05-09 Argo CD 更新 observability RCA

当前目标集群已经部署 Argo CD，`observability` Application 跟踪：

- `repoURL: http://192.168.48.206:8929/platform/gitops-manifests.git`
- `targetRevision: main`
- `path: clusters/test/observability`

后续更新 RCA 镜像时，流程改为：

```bash
git clone http://192.168.48.206:8929/platform/gitops-manifests.git
cd gitops-manifests
vim clusters/test/observability/auto-inspection-rca/deployment.yaml
git add clusters/test/observability/auto-inspection-rca source/deploy/rca-service source/yaml/rca-service
git commit -m "Update auto-inspection RCA image"
git push origin main
```

本次已推送的公网临时镜像：

```text
ttl.sh/auto-inspection-rca-20260509-k8s-inventory:24h
```

digest：

```text
sha256:0043b240c3b50eabdfd1c52a508a8e0d7fe2fa3183461a98210f80392336f9ac
```

注意：`ttl.sh` 镜像只保留 24 小时，仅适合临时验证公网拉取。正式使用需要替换为 Docker Hub/GHCR/Quay 等长期公共仓库地址。

RCA Deployment 中 `ARGOCD_SERVER` 应保持为：

```text
https://argocd-server.argocd.svc.cluster.local
```

因为 Argo CD 服务实际位于 `argocd` 命名空间。

当前 node200 GitOps 目标集群没有 `xzyc115-19` 节点，hostPath workload 的固定节点使用：

```text
kubernetes.io/hostname: k8s-worker-2
```

查看同步状态：

```bash
kubectl get application observability -n argocd
kubectl describe application observability -n argocd
```

强制刷新：

```bash
kubectl annotate application observability -n argocd argocd.argoproj.io/refresh=hard --overwrite
```
