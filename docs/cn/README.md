# RCA 文档目录

本目录提供面向交付、运维和产品设计的中文文档，围绕当前 `auto_inspection` RCA 方案整理。

文档清单：

- `product.md`
  产品能力、定位与边界
- `architecture.md`
  整体架构图与链路说明
- `slim_ops_architecture.md`
  RCA 平台瘦身、统一资源资产、自动巡检基线和总运维入口目标架构
- `recovery_runbook_2026-04-21.md`
  一次实际集群恢复记录，包含 etcd 快照恢复、NFS/PVC 恢复与 RCA 验证
- `frontend_ui_design.md`
  RCA 工作台的页面布局、组件层级与交互设计
- `codex_distribution.md`
  Codex Skill、MCP、插件分发说明
- `release_change_correlation.md`
  发布变更数据只读接入说明，覆盖 Helm/Deployment/镜像版本/ConfigMap 查询与 incident 关联
- `argocd_readonly_integration.md`
  Argo CD 应用状态、同步历史、Git revision 与资源 diff summary 只读接入说明
- `change_records/2026-04-24-gitlab-gitops-argocd-full.md`
  GitLab GitOps 仓库、Argo CD 安装、observability 应用同步与 MCP 只读查询全量接入记录
- `mcp_skill_change_record_template.md`
  MCP / Skill / backend / 页面变更记录模板，用于严格追踪修改内容

建议阅读顺序：

1. `product.md`
2. `architecture.md`
3. `slim_ops_architecture.md`
4. `frontend_ui_design.md`
5. `evidence_pack_contract.md`
6. `codex_distribution.md`

补充索引：

- `evidence_pack_contract.md`
  Evidence Pack 字段契约，覆盖 `/api/context/pod`、`/api/context/workload`、MCP `get_context_pack` 和 CLI context 命令。
