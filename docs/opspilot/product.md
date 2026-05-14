# OpsPilot 产品说明

## 产品定位

OpsPilot 是一套面向运维和 SRE 的智能运维入口。它不重新建设完整观测平台，而是把已有的 Prometheus、ELK、Kubernetes、Docker、GitLab、Argo CD 等数据源统一成稳定的只读查询能力，再通过 CLI、UI、MCP 和 AI Skill 提供排障入口。

OpsPilot 的核心不是“自动执行一切”，而是：

- 快速知道当前有哪些资源。
- 快速发现异常资源。
- 快速聚合指标、日志、事件和发布变更。
- 让 AI 通过确定性命令拿到结构化证据。
- 输出人能判断的 RCA 结论。

## 默认能力

- 资源资产查询
  - Kubernetes 集群、Node、Namespace、Pod、Service、Workload。
  - Docker 主机和容器。
  - 普通服务器 CPU、内存、磁盘、网络。
- 指标查询
  - Prometheus 资源水位、趋势、TopN、告警上下文。
- 日志查询
  - Kubernetes Pod 日志通过 `pods/log` 按需读取。
  - 网关日志、业务日志、关键系统日志继续走 ELK。
- 发布变更
  - GitLab commit、MR、tag、artifact。
  - Argo CD app 状态、revision、sync history。
- 自动巡检
  - 基线、健康快照、巡检报告、备份校验状态。
- AI 调查
  - Evidence Pack。
  - 异常摘要。
  - 根因排序。
  - 下一步建议。

## 默认不做

- 不默认自建 OpenSearch。
- 不默认全量采集 Kubernetes 容器日志。
- 不默认接入 eBPF。
- 不默认把高危操作暴露给 AI。
- 不让 AI 直接拼接 `kubectl`、SQL 或 shell 去操作生产环境。

## 可选模块

| 模块 | 何时启用 |
| --- | --- |
| OpenSearch | 没有现成 ELK，且需要长期全文检索日志 |
| MinIO / 对象存储 | 需要长期保存调查归档和报告 |
| MySQL / PostgreSQL | 需要保存大量任务状态、用户配置和资源快照 |
| eBPF / profiling | 指标和日志解释不了的深层网络、系统调用、性能问题 |
| AI Gateway | 多团队、多模型、多权限域统一治理时 |

## 产品边界

OpsPilot 先做只读排障入口。后续如果加入动作命令，必须默认二次确认，并且需要明确权限边界和审计记录。
