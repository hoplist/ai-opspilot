# OpsPilot 文档中心

`OpsPilot` 是 `auto_inspection` 的下一版产品化方向：统一运维入口、确定性 CLI、AI Skill 编排、只读 RCA Backend、资源资产查询和按需证据聚合。

本目录是 OpsPilot 的独立文档入口，后续新设计文档都放在这里，不再混入 `docs/cn/` 的历史文档。

## 命名约定

| 对象 | 名称 |
| --- | --- |
| 项目名 | OpsPilot |
| CLI | `opspilot` |
| 在线核心 Backend | `opspilot-core` |
| MCP 服务 | `opspilot-mcp` |
| AI Skill | `opspilot-skill` |
| Web UI | `opspilot-console` |
| 异步任务 | `opspilot-worker` |

## 文档清单

建议阅读顺序：

1. `product.md`
   - 产品定位、能力边界、默认交付与可选模块。
2. `architecture.md`
   - 总体架构、数据来源、默认部署边界和证据链路。
3. `cli-skill-contract.md`
   - 确定性 CLI 与 AI Skill 编排边界。
4. `pod-logs-on-demand.md`
   - Kubernetes Pod 日志按需读取设计。
5. `backend-go-plan.md`
   - Go `opspilot-core` 演进方案。
6. `migration-plan.md`
   - 从当前 `auto_inspection` 迁移到 OpsPilot 的阶段路线。
7. `change_records/`
   - OpsPilot 相关变更记录。

## 核心原则

- Prometheus 负责指标。
- ELK 负责网关、业务和关键系统日志。
- Kubernetes API 负责资源、事件和按需 Pod 日志。
- RCA Backend 统一查询、聚合、限流、脱敏和权限边界。
- CLI 负责确定性命令。
- Skill 负责编排、解释和输出结论。
- eBPF、OpenSearch、MySQL、MinIO 默认不启用，按需作为 optional 模块。
