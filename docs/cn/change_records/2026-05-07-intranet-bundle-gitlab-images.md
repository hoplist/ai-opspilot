# 2026-05-07 内网部署包补充 GitLab 与镜像预拉取

## 背景

内网部署时，GitLab 是 GitOps 仓库源头，也需要作为平台前置依赖纳入交付清单。为了方便迁移和部署，将相关 manifests、服务模板、镜像清单和脚本集中到同一个目录。

## 本次新增

新增内网部署包：

```text
deploy/intranet-bundle
```

包含：

```text
README.md
docs/
gitops-manifests/
images/auto-inspection-images.txt
scripts/pull-images-node206.sh
scripts/deploy-order.ps1
services/gitlab/docker-compose.yml
services/registry/docker-compose.yml
```

## GitLab 纳管方式

GitLab 作为前置依赖纳入部署包，但不由 Argo CD 管理。

当前 node206 GitLab 运行形态：

```text
image: docker.m.daocloud.io/gitlab/gitlab-ce:latest
http:  192.168.48.206:8929
ssh:   192.168.48.206:2224
```

原因：

- GitLab 是 GitOps 源头，不建议由依赖它的 Argo CD 反向管理。
- GitLab 数据需要独立备份和恢复。
- GitLab 故障会影响发布入口，应作为基础设施前置服务处理。

## 镜像预拉取

新增镜像清单：

```text
deploy/intranet-bundle/images/auto-inspection-images.txt
```

新增 node206 拉取脚本：

```text
deploy/intranet-bundle/scripts/pull-images-node206.sh
```

在 node206 上执行：

```bash
cd /opt/auto-inspection-intranet-bundle
bash scripts/pull-images-node206.sh
```

本次同步到 node206 后，已验证大部分镜像可拉取。`docker.1ms.run/grafana/pyroscope:1.15.1` 返回 manifest 不存在，已替换为：

```text
docker.m.daocloud.io/grafana/pyroscope:1.15.1
```

Alloy 镜像同步调整为已验证可拉取的：

```text
grafana/alloy:latest
```

## 文档更新

已更新：

```text
docs/cn/deployment/intranet-service-deployment-checklist.md
```

变更内容：

- 将 GitLab 从“外部已有服务”调整为“node206 Docker 前置服务”，并补充对应部署模板路径。
- 补充 `deploy/intranet-bundle` 作为完整内网部署包入口。
- 更新 Pyroscope 与 Alloy 镜像源，保持 GitOps manifests、部署包镜像清单和 node206 已拉取镜像一致。
