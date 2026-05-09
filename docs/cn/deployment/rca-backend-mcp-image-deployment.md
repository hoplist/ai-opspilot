# RCA Backend/MCP 镜像化部署说明

## 目标

将 `auto-inspection-rca` 从运行时安装依赖和 NFS 源码挂载，调整为镜像化发布：

- 代码和 Python 依赖内置到镜像。
- Backend 和 MCP 使用同一个镜像，通过 `RCA_SERVICE` 区分启动模式。
- PVC 只保留运行态数据目录，不再覆盖 `/opt/rca` 源码目录。

## 镜像

当前镜像：

```text
192.168.48.1:5002/auto-inspection-rca:20260429-rca-image
```

本机 registry 容器：

```text
registry-private
localhost:5002 -> registry:5000
```

构建与推送：

```powershell
docker build -t localhost:5002/auto-inspection-rca:20260429-rca-image .
docker push localhost:5002/auto-inspection-rca:20260429-rca-image
```

集群拉取地址使用：

```text
192.168.48.1:5002/auto-inspection-rca:20260429-rca-image
```

## 节点 containerd 配置

由于本机 registry 使用 HTTP，需要在 Kubernetes 节点配置 containerd insecure registry。

节点：

```text
192.168.48.200
192.168.48.201
192.168.48.202
```

配置文件：

```text
/etc/containerd/certs.d/192.168.48.1:5002/hosts.toml
```

内容：

```toml
server = "http://192.168.48.1:5002"

[host."http://192.168.48.1:5002"]
  capabilities = ["pull", "resolve", "push"]
  skip_verify = true
```

配置后重启：

```bash
systemctl restart containerd
systemctl is-active containerd
```

## GitOps 部署

Deployment 变更：

- `backend` 容器：
  - `image: 192.168.48.1:5002/auto-inspection-rca:20260429-rca-image`
  - `RCA_SERVICE=backend`
- `mcp` 容器：
  - `image: 192.168.48.1:5002/auto-inspection-rca:20260429-rca-image`
  - `RCA_SERVICE=mcp`

PVC 挂载：

```text
/opt/rca/data    -> app-state PVC subPath data
/opt/rca/outputs -> app-state PVC subPath outputs
```

不再挂载：

```text
/opt/rca
```

避免 NFS 源码覆盖镜像内代码。

## 验证

镜像拉取测试：

```powershell
kubectl run rca-image-pull-test -n observability `
  --image=192.168.48.1:5002/auto-inspection-rca:20260429-rca-image `
  --restart=Never `
  --image-pull-policy=Always `
  --command -- /bin/sh -lc 'python -V && test -f /opt/rca/backend_server.py && echo ok'
```

期望输出：

```text
Python 3.12.x
ok
```

GitOps 验证：

```powershell
kubectl kustomize clusters/test/observability
kubectl apply --dry-run=server -k clusters/test/observability
```

上线后验证：

```powershell
kubectl rollout status deployment/auto-inspection-rca -n observability
curl http://192.168.48.200:32180/api/health
```

## 发布规范

每次 RCA 代码变更建议：

1. 本地完成代码验证。
2. 构建唯一 tag 镜像。
3. 推送到 registry。
4. 修改 GitOps Deployment 镜像 tag。
5. `kubectl apply --dry-run=server -k clusters/test/observability`。
6. 提交并推送 `platform/gitops-manifests`。
7. 等 Argo CD 同步并验证 Backend/MCP。

不建议继续在运行 Pod 中手工 `kubectl cp` 覆盖代码；只允许作为临时调试手段。
