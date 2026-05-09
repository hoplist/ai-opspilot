# 2026-04-29 RCA Backend/MCP 镜像化与部署文档规范

## 背景

当前 RCA Backend/MCP 已具备日志、事件、资源、发布证据、GitLab、Argo CD、Beyla、Falco、Pyroscope、MCP/CLI 等能力。继续扩展新组件前，先规范部署形态和使用文档。

## 本次目标

- RCA Backend/MCP 镜像化。
- GitOps Deployment 改为镜像启动。
- NFS PVC 不再覆盖 `/opt/rca` 源码目录。
- 补充部署文档、使用手册和常见排障 runbook。

## 镜像

镜像已构建并推送到本机 registry：

```text
localhost:5002/auto-inspection-rca:20260429-rca-image
```

集群拉取地址：

```text
192.168.48.1:5002/auto-inspection-rca:20260429-rca-image
```

镜像 digest：

```text
sha256:0ee1223e87ebe322a629b41e637d85be6e61c132febe3cdeb3a2491da74ff77f
```

## 节点运行时配置

已在 3 个 Kubernetes 节点配置 containerd HTTP registry：

```text
/etc/containerd/certs.d/192.168.48.1:5002/hosts.toml
```

节点：

```text
192.168.48.200
192.168.48.201
192.168.48.202
```

配置后已重启 containerd，并确认节点 Ready。

## GitOps 变更

更新以下 Deployment：

- `clusters/test/observability/auto-inspection-rca/deployment.yaml`
- `source/deploy/rca-service/deployment.yaml`
- `source/yaml/rca-service/deployment.yaml`

关键变化：

- backend/mcp 容器统一使用 `192.168.48.1:5002/auto-inspection-rca:20260429-rca-image`
- backend 使用 `RCA_SERVICE=backend`
- mcp 使用 `RCA_SERVICE=mcp`
- 删除启动时 `pip install`
- 删除 `/opt/rca` 源码覆盖挂载
- PVC 仅挂载：
  - `/opt/rca/data`
  - `/opt/rca/outputs`

GitOps 提交：

```text
351af0a Use imageized RCA backend and MCP
```

Argo CD 已同步到 revision：

```text
351af0ae86476d653cbe224aa5da90f6ed2b77f8
```

同步状态：

```text
Synced Healthy
```

## 文档

新增：

- `docs/cn/deployment/rca-backend-mcp-image-deployment.md`
- `docs/cn/user_guides/auto-inspection-rca-usage-and-runbook.md`

## 验证记录

- `docker build` 成功。
- `docker push localhost:5002/auto-inspection-rca:20260429-rca-image` 成功。
- 初次集群拉取失败，原因是节点 containerd 默认使用 HTTPS 拉取 HTTP registry。
- 已配置 containerd `hosts.toml` 后重试。
- 测试 Pod 已能拉取镜像，并确认镜像内包含 Backend 与部署文档：

```text
Python 3.12.12
ok
```

- Deployment rollout 成功：

```text
deployment "auto-inspection-rca" successfully rolled out
```

- 当前容器镜像：

```text
backend=192.168.48.1:5002/auto-inspection-rca:20260429-rca-image; RCA_SERVICE=backend
mcp=192.168.48.1:5002/auto-inspection-rca:20260429-rca-image; RCA_SERVICE=mcp
```

- 新 Pod 已确认 `/opt/rca` 来自镜像，NFS 只挂载状态目录：

```text
192.168.48.206:/srv/nfs/observability/auto-inspection-rca/data on /opt/rca/data
192.168.48.206:/srv/nfs/observability/auto-inspection-rca/outputs on /opt/rca/outputs
docs-ok
```

- Backend 健康检查通过：

```text
GET /api/health -> 200
GET /api/health/details -> 200
POST /api/alerts/notify dry_run=true -> 200
```

- MCP 协议检查通过：

```text
initialize -> 200
tools/list -> 200
```

## 后续

- 镜像化后不建议继续通过 `kubectl cp` 覆盖运行 Pod 代码。
- 后续代码发布应走镜像 tag + GitOps Deployment 修改 + Argo CD 同步。
