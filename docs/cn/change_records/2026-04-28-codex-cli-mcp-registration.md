# 2026-04-28 Codex CLI MCP 注册方式调整

## 背景

- `auto-inspection-mcp` 已提供 Streamable HTTP MCP endpoint。
- 当前 Codex CLI 支持通过 `codex mcp add --url` 注册 Streamable HTTP MCP server。
- 本次将本机 Codex MCP 配置切换为 CLI 管理方式。

## 已执行

```powershell
codex mcp add --url http://192.168.48.200:32181/mcp autoInspectionRca
codex mcp list
codex mcp get autoInspectionRca
```

## 当前状态

```text
Name               Url                              Bearer Token Env Var  Status   Auth
autoInspectionRca  http://192.168.48.200:32181/mcp  -                     enabled  Unsupported
```

```text
autoInspectionRca
  enabled: true
  transport: streamable_http
  url: http://192.168.48.200:32181/mcp
  bearer_token_env_var: -
```

## 说明

- 不需要把 MCP 服务改成 stdio；当前 RCA MCP 已经是 Codex CLI 支持的 Streamable HTTP 接入方式。
- `Auth: Unsupported` 表示当前这个内网 MCP 没有 OAuth 登录流程，不影响无鉴权内网调用。
- 如需接入 AI Gateway 后再统一鉴权，可把 URL 切换到 AI Gateway 暴露的 MCP 入口，并配置 token 环境变量。
