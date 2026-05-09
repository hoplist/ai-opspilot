# Codex Skill 与 MCP 分发说明

## 目标

把当前 RCA 能力分发给其他 Codex 用户时，建议拆成两部分：

1. 共享 RCA 服务
2. 可安装的 Skill 包

## 推荐模式

### 模式 A：共享服务

推荐由团队统一部署：

- backend
- MCP

这样普通用户只需要：

- 安装 Skill
- 在 Codex 配置 MCP 地址

不需要每个人单独持有：

- kubeconfig
- 集群凭证
- OpenSearch / Prometheus 接入配置

## 集群内服务

当前仓库已补充：

- `deploy/rca-service`

它会在集群内启动：

- backend
- mcp

并通过 NodePort 暴露：

- backend: `32180`
- mcp: `32181`

## Skill 分发包

分发包位置：

- `rca/integration/codex/skill/auto-inspection-rca`

这个 Skill 包已经去掉了本机绝对路径依赖，改成通过：

- `AUTO_INSPECTION_BACKEND_URL`
- `AUTO_INSPECTION_MCP_URL`

或通过 Codex MCP 配置来工作。

## Codex 配置示例

```toml
[mcp_servers.autoInspectionRca]
url = "http://<RCA_MCP_HOST>:32181/mcp"
```

示例文件位置：

- `rca/integration/codex/config.toml.example`

## 交付建议

建议交付给他人的内容：

1. `rca/`
2. `rca/integration/codex/skill/auto-inspection-rca/`
3. `rca/integration/codex/config.toml.example`

## 用户安装步骤

1. 把 Skill 目录复制到：
   `%USERPROFILE%\.codex\skills\auto-inspection-rca`
2. 把 MCP 配置加入：
   `%USERPROFILE%\.codex\config.toml`
3. 在 Codex 中直接使用：

```text
使用 auto-inspection-rca，列出最近 incidents
```

## 后续建议

- 增加 Ingress / 域名暴露
- 增加鉴权
- 增加 Skill 安装脚本
- 增加 remote MCP 健康检查
