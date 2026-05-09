# 2026-04-30 内网部署服务清单

## 背景

RCA 平台已经包含 GitOps、Argo CD、RCA Backend/MCP、OpenSearch、Prometheus、Fluent Bit、MinIO、Beyla、Falco、Pyroscope/Alloy 等组件。为了后续内网部署和迁移，需要一份可以逐项核对的服务清单。

## 本次新增

新增文档：

```text
docs/cn/deployment/intranet-service-deployment-checklist.md
```

## 覆盖内容

- 需要部署的前置依赖。
- Argo CD 组件清单。
- Observability/RCA 服务清单。
- 每个服务使用的镜像。
- 服务启动顺序。
- NodePort/ClusterIP 暴露端口。
- 对应 GitOps/部署文件。
- 镜像同步清单。
- 最小可用部署集合。
- 验证命令。

## 关键约定

- 当前 Argo CD 同步入口仍为 `apps/observability-application.yaml`。
- 当前实际同步路径仍为 `clusters/test/observability`。
- `source/yaml` 和 `source/deploy` 作为维护副本/来源副本记录。
- Prometheus、MySQL、GitLab、Registry、Argo CD 作为平台前置依赖或独立部署项单独列出。

## 后续建议

- 将所有公网/代理镜像统一改写为正式内网 Registry 地址。
- Secret 从明文 YAML 迁移到 SOPS、SealedSecret 或 ExternalSecret。
- 将此清单作为生产迁移 checklist 使用。
