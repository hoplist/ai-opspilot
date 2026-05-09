# 内网 GitOps 发布闭环流程

## 目标

把 RCA 平台发布规范成一条固定链路：

```text
代码变更 -> 构建镜像 -> 推送内网 Registry -> 修改 GitOps 镜像 tag -> GitLab 提交 -> Argo CD 自动同步 -> 验证 Backend/MCP
```

这条链路适合当前内网环境：GitLab、Registry、Argo CD、Kubernetes、RCA Backend/MCP、OpenSearch、Prometheus、Pyroscope、Falco、Beyla 都在内网。

## 角色边界

| 组件 | 职责 |
|---|---|
| GitLab | 保存业务代码、RCA 代码、GitOps YAML、发布历史 |
| Registry | 保存 RCA Backend/MCP 镜像和第三方组件镜像 |
| Argo CD | 只负责把 GitOps 仓库声明的 YAML 同步到 Kubernetes |
| Kubernetes | 运行 RCA Backend/MCP 和观测组件 |
| RCA Backend/MCP | 提供只读排障 API、MCP 工具、Evidence Pack |

Argo CD 不构建镜像，也不应该直接保存源码。源码进入镜像，运行态 PVC 只保存 `data`、`outputs` 等状态目录。

## 内网前置条件

1. Kubernetes 节点可以访问内网 Registry。
2. containerd 已配置 HTTP/insecure registry：

```text
/etc/containerd/certs.d/192.168.48.1:5002/hosts.toml
```

3. GitOps 仓库已接入 Argo CD。
4. Argo CD 可以访问内网 GitLab。
5. RCA Backend/MCP Deployment 使用镜像启动，不再通过 PVC 覆盖 `/opt/rca`。

## 标准手工流程

### 1. 生成镜像 tag

建议使用日期时间或 Git commit：

```powershell
$tag = Get-Date -Format "yyyyMMdd-HHmmss"
```

### 2. 构建并推送镜像

```powershell
cd D:\code\auto_inspection
docker build -t localhost:5002/auto-inspection-rca:$tag .
docker push localhost:5002/auto-inspection-rca:$tag
```

集群内使用的镜像地址：

```text
192.168.48.1:5002/auto-inspection-rca:<tag>
```

### 3. 修改 GitOps Deployment

需要同步修改：

```text
D:\code\auto_inspection\worktrees\gitops-manifests\clusters\test\observability\auto-inspection-rca\deployment.yaml
D:\code\auto_inspection\worktrees\gitops-manifests\source\deploy\rca-service\deployment.yaml
D:\code\auto_inspection\worktrees\gitops-manifests\source\yaml\rca-service\deployment.yaml
```

把 backend 和 mcp 容器镜像改成：

```text
192.168.48.1:5002/auto-inspection-rca:<tag>
```

### 4. GitOps 渲染和 server dry-run

```powershell
cd D:\code\auto_inspection\worktrees\gitops-manifests
kubectl kustomize clusters/test/observability
kubectl apply --dry-run=server -k clusters/test/observability
```

### 5. 提交并推送 GitOps

```powershell
git add clusters/test/observability/auto-inspection-rca/deployment.yaml source/deploy/rca-service/deployment.yaml source/yaml/rca-service/deployment.yaml
git commit -m "Release RCA image <tag>"
git push origin main
```

### 6. 等待 Argo CD 同步

```powershell
kubectl get application observability -n argocd -o jsonpath='{.status.sync.revision} {.status.sync.status} {.status.health.status}'
```

期望：

```text
<revision> Synced Healthy
```

### 7. 发布后验证

```powershell
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=240s
kubectl get pods -n observability -l app.kubernetes.io/name=auto-inspection-rca -o wide
```

检查镜像和挂载：

```powershell
kubectl get deploy auto-inspection-rca -n observability -o jsonpath='{.spec.template.spec.containers[*].image}'
```

```powershell
$pod = kubectl get pods -n observability -l app.kubernetes.io/name=auto-inspection-rca -o jsonpath='{.items[0].metadata.name}'
kubectl exec -n observability $pod -c backend -- sh -lc 'mount | grep /opt/rca || true; test -f /opt/rca/backend_server.py && echo backend-ok'
```

健康接口：

```powershell
Invoke-WebRequest -UseBasicParsing http://192.168.48.200:32180/api/health
Invoke-WebRequest -UseBasicParsing http://192.168.48.200:32180/api/health/details
```

MCP：

```text
POST http://192.168.48.200:32181/mcp initialize
POST http://192.168.48.200:32181/mcp tools/list
```

## 自动化脚本

已提供脚本：

```text
D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1
```

先看执行计划，不做任何修改：

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -PlanOnly
```

只构建镜像、推送镜像、修改 GitOps、本地 dry-run，不提交：

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1
```

完整发布：构建、推送、修改 GitOps、提交、推送、等待 Argo、验证：

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -Commit -Push -WaitArgo -Verify
```

指定 tag：

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -Tag 20260429-prod01 -Commit -Push -WaitArgo -Verify
```

只重新 patch GitOps，不重新构建镜像：

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -Tag 20260429-prod01 -SkipDocker -Commit -Push -WaitArgo -Verify
```

## 后续可接 CI

后续如果要更自动，可以在 GitLab CI 中拆成两个阶段：

```text
build_image:
  - docker build
  - docker push

update_gitops:
  - 修改 GitOps 镜像 tag
  - git commit
  - git push
```

CI token 权限建议最小化：

- 构建仓库：允许读取代码、推送镜像。
- GitOps 仓库：只允许写入指定 GitOps 项目。
- Argo CD：不需要 CI 直接操作，继续由 Argo 自动同步。

## 回滚流程

推荐回滚 GitOps commit，而不是手动改线上 Deployment：

```powershell
cd D:\code\auto_inspection\worktrees\gitops-manifests
git log --oneline -- clusters/test/observability/auto-inspection-rca/deployment.yaml
```

找到上一个稳定镜像 tag 后，修改 Deployment 镜像 tag，重新提交推送。Argo CD 会自动把集群同步回该版本。

## 排障入口

镜像拉取失败：

```powershell
kubectl describe pod -n observability <pod>
kubectl get events -n observability --sort-by=.lastTimestamp
```

Argo 没同步：

```powershell
kubectl get application observability -n argocd -o yaml
```

Deployment 没更新：

```powershell
kubectl rollout history deployment/auto-inspection-rca -n observability
kubectl describe deployment auto-inspection-rca -n observability
```

Backend/MCP 不健康：

```powershell
kubectl logs -n observability deployment/auto-inspection-rca -c backend --tail=100
kubectl logs -n observability deployment/auto-inspection-rca -c mcp --tail=100
```
