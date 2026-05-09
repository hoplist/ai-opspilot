# Codex 集成方案

## 目标

把当前已经落地的 `OpenSearch + Dashboards + Prometheus + RCA Backend` 能力，
封装成 Codex 可以直接调用的能力，让用户在 Codex App / CLI / IDE 里直接说：

- “查一下这个 Pod 的日志”
- “看最近的 Warning 事件”
- “帮我排查 langfuse-clickhouse-shard0-0 为什么重启”

然后 Codex 自动调用后端、检索日志/事件/调查结果，而不是手工拼命令。

## 当前建议

当前最适合先落地的方式是：

`Skill + 本地脚本 + 现有 backend API`

原因：

- 成本最低，不需要先改协议栈。
- Codex 已原生支持 Skills。
- 你现在已经有统一后端 API，可以直接被脚本调用。
- 可先跑通“对话直接查日志 / 发起 RCA”，再演进到 MCP。

## 三种接入方式对比

### 方案 A：Skill

形态：

- Codex 发现一个本地 Skill。
- Skill 说明什么时候该用。
- Skill 自带脚本去调用 `backend_server.py` 暴露的接口。

优点：

- 实现最快。
- 对现有系统侵入最小。
- 适合“查日志 / 查事件 / 发起调查 / 看最近调查”这种固定工作流。

缺点：

- 本质仍是“说明 + 脚本”，不是原生工具调用。
- 参数结构和返回结果不如 MCP 工具化自然。

适用：

- 现在立刻可用。

### 方案 B：MCP Server

形态：

- 新建一个 MCP server，把后端 API 暴露成工具：
  - `search_logs`
  - `search_events`
  - `investigate`
  - `list_investigations`
  - `list_targets`

优点：

- 最适合“对话直接调用工具”。
- 参数和返回值结构化，模型稳定性更高。
- 后续可扩展到更多客户端，不只 Codex。

缺点：

- 需要额外维护 MCP server。
- 要处理服务发现、启动方式、权限和失败重试。

适用：

- 推荐作为第二阶段正式方案。

### 方案 C：Plugin

形态：

- 用 Codex Plugin 打包：
  - 一个或多个 Skill
  - 可选 MCP server 配置
  - 可选 app integration

优点：

- 适合团队分发。
- 安装和启用更统一。
- 适合后续沉淀成团队标准能力。

缺点：

- 前提是 Skill / MCP 能力本身已经稳定。
- 初期比直接 Skill 多一层包装。

适用：

- 推荐作为第三阶段包装方案。

## 推荐演进路径

### 第一阶段：现在就做

- 安装本地 Skill
- Skill 自带脚本调用现有 backend API
- 让 Codex 能直接：
  - 查日志
  - 查事件
  - 发起调查
  - 看最近调查
  - 看调查对象推荐

### 第二阶段：升级 MCP

- 新建 `auto_inspection_mcp`
- 把当前 backend API 包成 MCP tools
- 保留 Skill，但 Skill 变成“选择何时调用 MCP 工具”的说明层

### 第三阶段：打包 Plugin

- 用 Plugin 把 Skill + MCP server 配置打包
- 方便团队多机器安装和统一启用

## 结合当前落地的接入点

当前后端已有可用接口：

- `GET /api/search/logs`
- `GET /api/search/events`
- `POST /api/investigate`
- `GET /api/investigations/{id}`
- `GET /api/investigations`
- `GET /api/investigation-targets`

因此 Skill 或 MCP 不需要重复实现检索逻辑，只需要编排调用。

## 开源 / 标准方案参考

### MCP

- Model Context Protocol 已经是开源项目，并由 Linux Foundation 承接生态。
- 适合把现有后端能力暴露成标准工具接口。

参考：

- MCP 官方 GitHub: https://github.com/modelcontextprotocol
- FastMCP（Python）: https://github.com/jlowin/fastmcp
- Awesome MCP Servers: https://github.com/wong2/awesome-mcp-servers

### Codex 官方接入形态

Codex 官方当前支持的能力形态包括：

- Skills
- MCP servers
- Plugins

其中 Plugin 可以打包 Skills、app integrations 和 MCP server 配置。

参考：

- OpenAI Codex Help: https://help.openai.com/en/articles/11369540-codex-in-chatgpt
- OpenAI Docs MCP: https://platform.openai.com/docs/docs-mcp
- OpenAI Codex product page: https://openai.com/codex/

## 当前已落地的实际实现

本轮已新增：

- 本地 Skill：
  - `C:\Users\Administrator\.codex\skills\auto-inspection-rca`
- Skill 脚本：
  - `scripts/auto_inspection_backend.py`

作用：

- 统一调用本机后端
- 支持：
  - `health`
  - `search-logs`
  - `search-events`
  - `investigate`
  - `recent`
  - `targets`

## 建议给 Codex 的触发语句

- “查一下 langfuse-clickhouse-shard0-0 最近 6 小时日志”
- “帮我看 observability/opensearch-0 最近事件”
- “排查 langfuse-clickhouse-shard0-0 为什么反复重启”
- “列出最近调查记录”
- “给我推荐当前优先排查对象”

## 结论

如果目标是“现在就让 Codex App 直接能用”，最佳方案不是先做复杂插件，而是：

`先装本地 Skill，Skill 通过脚本调用现有 backend API`

如果目标是“后续团队化、结构化工具调用”，下一步最值得做的是：

`把这套 backend API 再包成 MCP server`
