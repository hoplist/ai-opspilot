# 2026-04-29 内网 GitOps 发布流程与自动化脚本

## 背景

RCA Backend/MCP 已完成镜像化部署。为了让后续发布不再依赖手工 `kubectl cp`、临时 Pod 修改或运行时安装依赖，需要把内网发布链路固定下来。

## 本次目标

- 整理完整内网 GitOps 发布闭环。
- 明确 GitLab、Registry、Argo CD、Kubernetes、RCA Backend/MCP 的职责边界。
- 提供可执行的 PowerShell 自动化脚本。
- 先聚焦部署规范，不新增观测功能。

## 新增文档

```text
docs/cn/deployment/intranet-gitops-release-flow.md
```

内容包括：

- 内网前置条件。
- 手工发布流程。
- 自动化脚本使用方式。
- GitLab CI 后续接入方向。
- 回滚流程。
- 常见排障入口。

## 新增脚本

```text
scripts/release-rca-image-gitops.ps1
```

脚本能力：

- 生成 RCA 镜像 tag。
- 构建镜像。
- 推送到本机内网 Registry。
- 修改 GitOps Deployment 镜像地址。
- 执行 `kubectl kustomize` 和 `kubectl apply --dry-run=server`。
- 可选提交并推送 GitOps 仓库。
- 可选等待 Argo CD 同步。
- 可选验证 Deployment rollout、镜像内容和 Backend 健康接口。

## 推荐执行方式

先查看计划：

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -PlanOnly
```

完整发布：

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -Commit -Push -WaitArgo -Verify
```

指定 tag：

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -Tag 20260429-prod01 -Commit -Push -WaitArgo -Verify
```

## 约束

- Argo CD 仍只负责同步 GitOps 仓库，不负责构建镜像。
- RCA Backend/MCP 镜像内置代码和文档。
- PVC 只保存 `data`、`outputs` 等运行态数据。
- 生产环境建议后续把当前本机 Registry 替换为固定内网 Registry 或 Harbor。

## 后续

- 可以把脚本拆进 GitLab CI。
- 可以增加镜像 digest 固化、SBOM、镜像漏洞扫描。
- 可以把 GitOps 镜像 tag 修改从正则替换升级为 yq/kustomize image patch。
