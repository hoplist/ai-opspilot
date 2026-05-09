# MCP / Skill 变更记录：Codex 智能数据接入设计

## 1. 基本信息

- 日期：2026-04-27
- 操作人：Codex
- 需求来源：用户希望在现有 MCP + Skill 方式之外，探讨如何更智能地将采集数据接入 Codex、AI Gateway 和 CLI，并要求先落地为设计文档和变更记录，不修改代码。
- 关联对话摘要：在阅读 `docs` 目录后，确认当前模式为 `Skill 触发 + MCP 优先 + backend HTTP 回退`，进一步提出 Evidence Pack、Snapshot Index、MCP Resources、AI Gateway 治理层和 CLI 证据包生成器方案。
- 变更主题：新增 Codex 智能数据接入设计文档。
- 影响范围：文档，不涉及运行时代码。

## 2. 需求原文

```text
可以  先落地成对应文档，需要记录到对应变更文档和设计文档，再行修改
```

## 3. 目标

- 记录 Codex 智能数据接入的下一阶段设计。
- 明确 Evidence Pack、Snapshot Index、MCP Resources、AI Gateway、CLI 的职责边界。
- 保持 MCP / AI Gateway / CLI 只读安全边界。
- 在现有文档目录、架构文档和 MCP 路线图中加入引用。
- 不修改业务代码、MCP server、Skill 脚本或 backend API。

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改基础设施配置：否
- 是否允许生成 investigation 记录：本文档设计允许后续只读流程中生成，但本次未实现
- 是否涉及凭证变更：否
- 是否涉及远端部署：否

说明：

```text
本次只做文档设计。后续实现 Evidence Pack、Snapshot Index、MCP Resources、AI Gateway 或 CLI 命令时，仍必须保持只读证据边界，并单独记录变更。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 新增 | `docs/cn/codex_intelligent_data_access.md` | 新增 Codex 智能数据接入设计文档 |
| 修改 | `docs/cn/README.md` | 增加新设计文档目录入口 |
| 修改 | `docs/cn/architecture.md` | 增加 Codex 智能接入扩展说明 |
| 修改 | `docs/cn/mcp_readonly_observability_roadmap.md` | 增加智能数据接入补充章节 |
| 新增 | `docs/cn/change_records/2026-04-27-codex-intelligent-data-access-design.md` | 本次文档变更记录 |

## 6. 新增或修改的 MCP 工具

本次未新增或修改 MCP 工具。

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| 无 | 无 | 无 | 无 | 是 | 本次仅记录后续设计 |

## 7. 新增或修改的 Skill 能力

本次未新增或修改 Skill 能力。

后续设计建议：

- Skill 继续作为触发说明和流程编排层。
- Skill 优先调用 MCP Resources / Tools。
- MCP 不可用时可回退到 CLI Evidence Commands。

## 8. 后端 API 变化

本次未修改后端 API。

后续建议新增只读 API：

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/context/pod` | 规划 | 是 | 返回 Pod Evidence Pack |
| `GET` | `/api/context/workload` | 规划 | 是 | 返回 Workload Evidence Pack |
| `GET` | `/api/context/service` | 规划 | 是 | 返回 Service Evidence Pack |
| `GET` | `/api/context/incident` | 规划 | 是 | 返回 Incident Evidence Pack |
| `GET` | `/api/context/namespace` | 规划 | 是 | 返回 Namespace Evidence Pack |

## 9. 部署动作

- 是否同步到 NFS：否
- 是否重启 Deployment：否
- 是否修改本机服务：否
- 是否修改 Codex 本机配置：否

命令或动作摘要：

```powershell
# 本次只修改本地 docs 文档，不执行部署动作。
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| 文档文件存在 | `Get-ChildItem docs\cn\codex_intelligent_data_access.md, docs\cn\change_records\2026-04-27-codex-intelligent-data-access-design.md` | 新文档和变更记录可见 | 通过 |
| 文档引用存在 | `Select-String` 查询新文档名和关键字 | README、架构、路线图包含引用 | 通过 |

## 11. 回滚方式

- 删除 `docs/cn/codex_intelligent_data_access.md`
- 删除 `docs/cn/change_records/2026-04-27-codex-intelligent-data-access-design.md`
- 从 `docs/cn/README.md` 移除新文档目录项
- 从 `docs/cn/architecture.md` 移除 Codex 智能接入扩展章节
- 从 `docs/cn/mcp_readonly_observability_roadmap.md` 移除 `9.1 Codex 智能数据接入补充`

## 12. 已知限制

- 本次没有实现 Evidence Pack API。
- 本次没有实现 Snapshot Index。
- 本次没有新增 MCP Resources。
- 本次没有实现 AI Gateway 策略层。
- 本次没有新增 CLI evidence 命令。

## 13. 后续建议

- 先实现 `Evidence Pack API`，从现有 `diagnose_pod` 泛化到 Pod / Workload / Service / Incident。
- 再实现 `Snapshot Index`，沉淀异常、趋势、发布变更和业务错误摘要。
- 然后增加 MCP Resources，让 Codex 能用对象 URI 读取上下文。
- 最后将 AI Gateway 和 CLI 纳入统一证据包链路。
