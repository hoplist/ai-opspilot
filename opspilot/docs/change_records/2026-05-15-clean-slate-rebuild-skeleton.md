# 2026-05-15 OpsPilot 完全重构骨架

## 背景

旧 `auto_inspection` 当前暂时没有生产使用约束，因此 OpsPilot 不再以兼容旧 Backend、旧 CLI、旧部署为目标，而是按新方案完全重构。

## 本次变更

更新：

- `docs/opspilot/migration-plan.md`

新增代码骨架：

- `opspilot/`
- `opspilot/core/`
- `opspilot/cli/`
- `opspilot/mcp/`
- `opspilot/worker/`
- `opspilot/console/`
- `opspilot/contracts/`

新增契约草案：

- `opspilot/contracts/openapi.yaml`
- `opspilot/contracts/cli-schema.json`
- `opspilot/contracts/mcp-tools.json`
- `opspilot/contracts/evidence-pack.schema.json`

新增部署骨架：

- `deploy/opspilot/`
- `deploy/opspilot/core/`
- `deploy/opspilot/mcp/`
- `deploy/opspilot/console/`
- `deploy/opspilot/worker/`
- `deploy/opspilot/rbac/`
- `deploy/opspilot/optional/`

## 重构边界

- 新代码进入 `opspilot/`。
- 新部署进入 `deploy/opspilot/`。
- 新文档进入 `docs/opspilot/`。
- 旧目录暂时保留，但不再继续扩展。

## 本次不涉及

- 未实现 Go Backend。
- 未实现 CLI。
- 未实现 MCP。
- 未修改运行中集群。
- 未修改 GitOps 清单。
- 未删除旧代码。
