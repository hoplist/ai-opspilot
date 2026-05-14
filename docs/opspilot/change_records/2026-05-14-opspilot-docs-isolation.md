# 2026-05-14 OpsPilot 文档隔离

## 背景

`auto_inspection` 原文档中混合了当前实现、历史部署、OpenSearch 全量日志方案、eBPF 方案、AI Gateway 方案和下一版轻量架构方案。为了避免后续实现混乱，需要为新项目名称 `OpsPilot` 建立独立文档目录。

## 本次变更

新增：

- `docs/README.md`
- `docs/opspilot/README.md`
- `docs/opspilot/product.md`
- `docs/opspilot/architecture.md`
- `docs/opspilot/cli-skill-contract.md`
- `docs/opspilot/pod-logs-on-demand.md`
- `docs/opspilot/backend-go-plan.md`
- `docs/opspilot/migration-plan.md`
- `docs/opspilot/change_records/2026-05-14-opspilot-docs-isolation.md`

## 目录边界

- `docs/opspilot/`
  - OpsPilot 新项目文档，后续新设计默认写入这里。
- `docs/cn/`
  - 保留为当前 `auto_inspection` 实现和历史资料。

## 本次不涉及

- 未修改 Backend 代码。
- 未修改 GitOps 清单。
- 未修改运行中集群。
- 未开始 Go 迁移。
