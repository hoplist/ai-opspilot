# OpsPilot 文档中心

OpsPilot 是 `auto_inspection` 的产品化方向：统一运维入口、确定性 CLI、
服务端 skills、只读优先的 RCA Backend、资源资产查询、发布证据链和按需
排障证据聚合。

如果只想看当前应该怎么用，先读：

1. [current-state.md](current-state.md)
2. [developer-standard-flow.md](developer-standard-flow.md)
3. [gitlab-repository-governance.md](gitlab-repository-governance.md)
4. [release-evidence-chain.md](release-evidence-chain.md)
5. [credential-ledger.md](credential-ledger.md)

## 当前核心仓库

| 类型 | 当前仓库 |
| --- | --- |
| OpsPilot core source | `tpo/platform/opspilot/opspilot-core` |
| Runtime config | `tpo/platform/opspilot/opspilot-config` |
| Runtime skills | `tpo/platform/opspilot/opspilot-skills` |
| GitOps desired state | `tpo/deploy/gitops-manifests` |

## 命名约定

| 对象 | 名称 |
| --- | --- |
| 项目 | OpsPilot |
| CLI | `opspilot` |
| 在线核心 Backend | `opspilot-core` |
| AI Skill | `opspilot-skill` |
| Web UI | `opspilot-console` |
| 异步任务 | `opspilot-worker` |

## 核心原则

- GitLab 是测试环境平台代码、配置、skills 和 GitOps 的来源。
- GitOps 仓库只保存期望部署状态，不保存业务源代码。
- Prometheus 负责指标；ELK/OpenSearch/OpenObserve 负责可选日志证据。
- Kubernetes API 负责资源、事件和按需 Pod 日志。
- 缺失的观测源不阻塞发布和基础排障，但必须在 evidence/gap 中说明。
- 高风险动作保持 plan-first，并给出最小验证方式和回滚边界。
