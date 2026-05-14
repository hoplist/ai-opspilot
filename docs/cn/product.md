# RCA 产品说明

## 产品定位

本项目定位为一套轻量化的智能运维入口，用于把现有运维数据源组织成可查询、可关联、可给 AI 调用的 RCA 能力。

平台不替代 Prometheus，不替代 ELK，不默认自建 OpenSearch，也不默认全量采集 Kubernetes 容器日志。

推荐定位：

- Prometheus 负责指标、趋势、告警和资源水位。
- ELK 负责网关日志、业务日志、关键系统日志和长期全文检索。
- Kubernetes API 负责资源状态、事件和按需 Pod 日志。
- Docker 主机通过 `node_exporter + cAdvisor` 接入 Prometheus。
- RCA Backend 负责统一只读查询、证据聚合、资源资产视图和 AI/MCP 调用入口。

## 核心能力

### 1. 统一资源资产

平台需要让 AI 和运维人员知道当前环境中有哪些资源：

- Kubernetes 集群、Namespace、Node、Pod、Service、Workload。
- Docker 主机和容器。
- 普通服务器、CPU、内存、磁盘、网络。
- 应用、服务、网关路由、业务标签和负责人。

这些信息由 RCA Backend 归一为 Inventory 模型，提供给 UI、CLI 和 MCP。

### 2. 只读证据聚合

一次 RCA 调查优先聚合以下证据：

- Prometheus 指标。
- Kubernetes 资源状态。
- Kubernetes Event。
- Kubernetes Pod 短窗口日志。
- ELK 中的网关日志和业务日志。
- GitLab / Argo CD 发布变更。
- 巡检、基线和健康快照结果。

Pod 日志由 Backend 通过 `pods/log` 按需读取，不长期保存到 RCA 平台。

### 3. AI 调查与 MCP

AI 不直接访问 Kubernetes、Prometheus 或 ELK，而是通过 MCP 调用 RCA Backend 的只读工具。

典型能力：

- 查询当前有多少服务器、集群、Pod、容器。
- 查询异常 Pod、资源 TopN、最近告警。
- 查询某个 Pod 的当前日志和 previous 日志。
- 关联 504、慢请求、网关日志、业务日志和发布变更。
- 生成 Evidence Pack 和调查摘要。

### 4. 自动巡检与基线

自动巡检和基线应作为异步任务运行：

- `baseline-job`
- `health-snapshot-job`
- `incident-correlation-job`
- `report-job`
- `backup-verify-ingest-job`

在线接口不做全量重计算，只查询预计算结果和短窗口实时证据。

### 5. 可视化工作台

RCA UI 作为统一入口，重点展示：

- 全局资源概览。
- 异常 Pod / Workload / Server。
- Prometheus 指标趋势。
- 按需 Pod 日志。
- ELK 网关/业务日志跳转。
- 发布变更和调查摘要。

## 当前交付边界

默认交付：

- RCA Backend。
- MCP Server。
- RCA UI。
- Kubernetes 只读 RBAC，包含 `pods/log`。
- Prometheus 查询配置。
- ELK 外部查询配置。
- 自动巡检和基线任务。

可选交付：

- OpenSearch / OpenSearch Dashboards。
- MinIO / 对象存储归档。
- MySQL / PostgreSQL 状态存储。
- eBPF / runtime security / profiling。

可选组件只在明确需要长期日志检索、调查归档、深度性能或安全排查时启用。

## 后续方向

近期优先：

- 补齐统一 Inventory API。
- 补齐按需 Pod 日志 API。
- 补齐 MCP 资源查询和日志查询工具。
- 将旧的 OpenSearch 内置部署降级为 optional。
- 自动巡检和基线任务异步化。

中期方向：

- 将在线高并发查询核心演进为 Go / Java `rca-core`。
- Python 保留在 AI、MCP、报告和异步 Worker 层。
- 所有入口统一调用 RCA Core API。
