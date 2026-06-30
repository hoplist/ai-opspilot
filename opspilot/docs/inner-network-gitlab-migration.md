# 内网 GitLab 同步与 OpsPilot 镜像切换操作文档

## 目标

在内网环境启动 OpsPilot 时，只同步当前必需仓库，避免把测试 demo、
历史临时仓库和无用 registry tag 带进去。

当前判断：内网侧主要需要改两类内容：

1. GitLab 仓库结构和仓库 URL。
2. OpsPilot 运行镜像地址。

其他能力尽量保持配置兼容，不在内网第一次启动时引入大范围重构。

## 外网当前基线

外网 node206 GitLab 当前保留 7 个项目：

| 仓库 | 是否需要同步到内网 | 用途 |
| --- | --- | --- |
| `tpo/platform/opspilot/opspilot-core` | 是 | OpsPilot core 源码、CI、镜像构建入口。 |
| `tpo/platform/opspilot/opspilot-config` | 是 | Runtime 配置：集群、数据源、凭证台账、服务目录、巡检、probe 策略。 |
| `tpo/platform/opspilot/opspilot-skills` | 是 | 服务端 runtime skills。 |
| `tpo/deploy/gitops-manifests` | 是 | Argo CD 读取的测试集群 GitOps desired state。 |
| `tpo/ops/backups/node200-etcd-snapshots` | 可选 | node200 etcd 备份仓库。内网如果是新集群，可新建同名空仓库。 |
| `tpo/devex/opspilot/opspilot-core` | 可选 | 旧共享 CI template include source。只有旧业务仓库仍 include 它时才同步。 |
| `platform/opspilot` | 可选 | 旧兼容和 registry-history holder。内网新环境建议不同步，除非需要旧回滚历史。 |

建议内网首版必同步：

```text
tpo/platform/opspilot/opspilot-core
tpo/platform/opspilot/opspilot-config
tpo/platform/opspilot/opspilot-skills
tpo/deploy/gitops-manifests
```

建议内网按需同步：

```text
tpo/ops/backups/node200-etcd-snapshots
tpo/devex/opspilot/opspilot-core
platform/opspilot
```

## 内网 GitLab 需要建立的结构

建议保持外网一致路径，减少代码和配置差异：

```text
tpo/
  platform/
    opspilot/
      opspilot-core
      opspilot-config
      opspilot-skills
  deploy/
    gitops-manifests
  ops/
    backups/
      node200-etcd-snapshots
  devex/
    opspilot/
      opspilot-core

platform/
  opspilot
```

如果内网不需要旧兼容路径，可以暂不建：

```text
platform/opspilot
tpo/devex/opspilot/opspilot-core
```

但要确认没有配置或 CI include 还指向它们。

## 仓库导入方式

外网导出 mirror：

```bash
git clone --mirror http://<outer-gitlab>/<path>.git <safe-name>.git
```

内网导入：

```bash
cd <safe-name>.git
git remote set-url origin http://<inner-gitlab>/<same-path>.git
git push --mirror origin
```

推荐保持同名路径，不要在内网改成扁平路径。原因：

- `opspilot-config` 内已有服务目录、GitOps、GitLab project metadata。
- `git-sync` 的 URL 可以少改。
- `release status`、回滚、发布证据链更容易保留。

## 必须修改的配置

以下内容需要从外网地址改成内网地址。

### 1. GitLab URL

位置：

```text
tpo/platform/opspilot/opspilot-config/settings/platform.yaml
```

需要检查字段：

```yaml
platform:
  gitlab_url: http://<inner-gitlab>
  gitops_project: tpo/deploy/gitops-manifests
```

如果仓库路径保持一致，`gitops_project` 不用改。

### 2. Cluster 配置

位置：

```text
tpo/platform/opspilot/opspilot-config/clusters/*.yaml
```

需要改：

- cluster 名称和 endpoint；
- kubeconfig 引用；
- Prometheus URL；
- GitOps project；
- Argo CD namespace；
- registry 地址。

### 3. Service Catalog

位置：

```text
tpo/platform/opspilot/opspilot-config/services/platform/opspilot-core.yaml
```

外网当前镜像：

```text
192.168.48.206:5050/tpo/platform/opspilot/opspilot-core/opspilot-core:8553f0ba
```

内网建议改成私仓镜像：

```text
docker-hub.tpo.xzoa.com/opspilot/opspilot-core:8553f0ba
```

需要同步改这些字段：

```yaml
release:
  image: docker-hub.tpo.xzoa.com/opspilot/opspilot-core
  gitlab_project: tpo/platform/opspilot/opspilot-core
  gitops_path: clusters/test/apps/opspilot-core/deployment.yaml
  argocd_app: opspilot-core
```

### 4. GitOps Deployment 镜像

位置：

```text
tpo/deploy/gitops-manifests/clusters/test/apps/opspilot-core/deployment.yaml
```

需要把 Deployment 容器镜像改为：

```text
docker-hub.tpo.xzoa.com/opspilot/opspilot-core:8553f0ba
```

同时检查 `config-sync`、`skills-sync`、Prometheus、Argo CD、git-sync 等镜像。
这些镜像如果内网不能访问外部 registry，也需要提前放入私仓或离线镜像包。

### 5. Git Sync URL

位置：

```text
tpo/deploy/gitops-manifests/clusters/test/apps/opspilot-core/deployment.yaml
```

需要检查环境变量或参数：

```text
OPSPILOT_CONFIG_GIT_URL
OPSPILOT_SKILLS_GIT_URL
```

改成内网 GitLab URL：

```text
http://<inner-gitlab>/tpo/platform/opspilot/opspilot-config.git
http://<inner-gitlab>/tpo/platform/opspilot/opspilot-skills.git
```

### 6. Argo CD GitOps URL

位置：

```text
tpo/deploy/gitops-manifests/apps/*.yaml
tpo/deploy/gitops-manifests/platform/argocd/**/*
```

把外网 GitOps URL：

```text
http://192.168.48.206:8929/tpo/deploy/gitops-manifests.git
```

改成内网 GitLab URL：

```text
http://<inner-gitlab>/tpo/deploy/gitops-manifests.git
```

## 凭证需要怎么处理

不要把外网 token 原样搬到内网。

内网需要重新生成：

| 凭证 | 存放位置 | 用途 |
| --- | --- | --- |
| `GITOPS_TOKEN` | `tpo/platform/opspilot/opspilot-core` CI/CD variable | CI 写入 GitOps 仓库。 |
| config repo read token | Kubernetes Secret `opspilot-config-secrets` | `config-sync` 读取 `opspilot-config`。 |
| skills repo read token | Kubernetes Secret `opspilot-skills-secrets` | `skills-sync` 读取 `opspilot-skills`。 |
| release evidence token | Kubernetes Secret `opspilot-release-secrets` | OpsPilot 查询 GitLab、Registry、GitOps 发布证据。 |

内网测试阶段如果允许明文管理，可以在 `opspilot-config` 的凭证台账记录：

- 业务线；
- 用途；
- 权限；
- 存放位置；
- 负责人；
- 过期/轮换策略。

但 API 和 CLI 输出仍不应该返回 token 原值。

## OpsPilot Core 镜像私仓基线

本次已在 node206 推送：

```text
docker-hub.tpo.xzoa.com/opspilot/opspilot-core:8553f0ba
```

来源镜像：

```text
192.168.48.206:5050/tpo/platform/opspilot/opspilot-core/opspilot-core:8553f0ba
```

推送后验证 digest：

```text
docker-hub.tpo.xzoa.com/opspilot/opspilot-core@sha256:3a6b75f18b820034c9b667cf0a7cfd4117537de2897c5a7b8947c6a58bf2d554
```

手工复现命令：

```bash
docker pull 192.168.48.206:5050/tpo/platform/opspilot/opspilot-core/opspilot-core:8553f0ba
docker tag 192.168.48.206:5050/tpo/platform/opspilot/opspilot-core/opspilot-core:8553f0ba docker-hub.tpo.xzoa.com/opspilot/opspilot-core:8553f0ba
docker push docker-hub.tpo.xzoa.com/opspilot/opspilot-core:8553f0ba
docker pull docker-hub.tpo.xzoa.com/opspilot/opspilot-core:8553f0ba
```

## 内网首次启动最小步骤

1. 启动内网 GitLab。
2. 创建 `tpo/platform/opspilot` 和 `tpo/deploy` 等 group。
3. 导入必需仓库 mirror。
4. 推送 OpsPilot 需要的镜像到内网私仓。
5. 修改 GitOps 中的：
   - GitLab URL；
   - GitOps repo URL；
   - `opspilot-core` 镜像；
   - config/skills git-sync URL；
   - Secret 名称保持不变。
6. 在内网 Kubernetes 创建 `opspilot` namespace 和必要 Secret。
7. 安装 Argo CD 或直接 `kubectl apply` 首次 bootstrap。
8. 等 Argo CD 同步 `opspilot-core`。
9. 用 CLI 验证：

```powershell
opspilot config status --output human
opspilot release status --service opspilot-core --output human
opspilot inventory overview --output human
```

## 最小验证

内网改完后，必须验证：

```bash
kubectl -n argocd get applications
kubectl -n opspilot get pods -o wide
kubectl -n opspilot logs deploy/opspilot-core --tail=100
```

OpsPilot 侧：

```powershell
opspilot config validate --output human
opspilot config status --output human
opspilot release status --service opspilot-core --output human
```

判断标准：

- `opspilot-core` Pod Running；
- config/skills sync 没有认证失败；
- `release status` 能看到 GitLab/GitOps/Argo/Kubernetes 证据；
- 缺失 Prometheus/ELK/APISIX 等外部观测源可以作为 gap，不阻塞启动。

## 回滚

如果内网启动失败：

1. 先回滚 GitOps 中 `opspilot-core` Deployment 镜像 tag。
2. 再回滚 `OPSPILOT_CONFIG_GIT_URL` / `OPSPILOT_SKILLS_GIT_URL`。
3. 如果是配置错误，回滚 `opspilot-config` 仓库最近一次 commit。
4. 如果是 GitLab 仓库导入错误，从 mirror 重新 `git push --mirror`。

不要直接手改运行中 Pod 内文件。OpsPilot 的期望状态应来自 GitLab/GitOps。
