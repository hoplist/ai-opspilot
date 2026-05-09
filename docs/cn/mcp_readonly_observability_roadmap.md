# 只读 MCP 可观测数据源接入路线

## 2026-04-28 深层排查路线修订

正式集群当前使用 Calico，且集群内部网络错误较少，网络排查证据暂不作为近期建设重点。本阶段舍弃 P3 网络证据接入，不规划 Cilium Hubble、Tetragon 或 Calico/CoreDNS 网络证据的近期落地。

新的深层排查优先级如下：

1. P0：OTel eBPF / Beyla 无侵入业务调用证据
   - 目标：补齐 HTTP/gRPC/DB 调用、RED 指标、服务依赖、慢请求、错误率与 trace span。
   - 接入方向：优先复用现有 OpenTelemetry Collector，把 eBPF 自动采集数据进入现有观测链路。
   - RCA 价值：定位发布后接口变慢、错误率升高、下游依赖异常、业务调用链断点。
   - 预期 MCP 工具：`service_dependency_context`、`service_red_metrics`、`slow_request_context`、`release_trace_correlation`。

2. P1：Falco runtime 事件证据
   - 目标：补齐容器内进程、shell、敏感文件访问、异常网络连接、权限提升等 runtime 行为证据。
   - 接入方向：先以 observe / alert 事件采集为主，不做自动阻断或 enforcement。
   - RCA 价值：判断故障窗口内 Pod 内部是否出现异常进程、异常文件访问或人为进入容器操作。
   - 预期 MCP 工具：`runtime_events_context`、`pod_process_events`、`pod_file_access_events`、`pod_exec_anomalies`。

3. P2：Pyroscope / Parca profiling 性能剖析
   - 目标：补齐 CPU 热点、函数调用栈、发布前后 profile diff、性能退化证据。
   - 接入方向：先做只读 profile summary，再逐步支持按 workload、service、release revision 查询。
   - RCA 价值：定位 CPU 高、延迟高但日志无明显错误、发布后性能退化等问题。
   - 预期 MCP 工具：`profile_hotspots`、`profile_diff_by_release`、`cpu_hot_path_context`。

明确后置或暂不建设：

- Cilium Hubble：正式集群使用 Calico，不作为近期方案。
- Cilium Tetragon：属于 Cilium 生态，不作为正式集群近期方案。
- Calico/CoreDNS 网络证据：网络故障低频，暂不投入。
- Pixie/Coroot：功能重叠较多，暂不替代现有 RCA Backend + MCP 架构。

本文记录 `auto_inspection` 后续面向开发排障的 MCP 能力扩展路线。核心前提是：

- MCP 只能读取证据
- MCP 不能直接操作服务器
- MCP 不能修改 Kubernetes 或基础设施资源
- 所有修复动作只能输出建议和证据，由人确认后通过运维流程执行

## 1. 当前 MCP 边界

当前 MCP 已具备以下只读或证据生成能力：

- `health`
- `health_details`
- `search_logs`
- `search_events`
- `investigate`
- `list_investigations`
- `get_investigation`
- `list_targets`
- `list_incidents`
- `search_incidents`
- `node_resources`
- `diagnose_pod`

其中 `diagnose_pod` 是面向复杂 Pod / Workload 问题的只读证据包工具，会组合：

- backend health details
- RCA investigation summary
- matching logs
- matching Kubernetes events
- matching incidents
- Prometheus resource summary

支持的症状类型：

- `oom`
- `crashloop`
- `probe`
- `pending`
- `imagepull`
- `latency`
- `error`
- `unknown`

## 2. 严格禁止范围

MCP 禁止具备以下能力：

- SSH 登录服务器
- 执行节点 shell 命令
- 删除、重启、扩缩容 Pod / Deployment / StatefulSet
- `kubectl apply`
- `kubectl patch`
- `kubectl delete`
- `kubectl rollout restart`
- `kubectl cordon`
- `kubectl drain`
- 修改 OpenSearch / Prometheus / MinIO / MySQL 配置
- 修改 Kubernetes Secret / ConfigMap / RBAC

如果排障结果需要这些动作，MCP 只能返回：

- 建议动作
- 风险说明
- 支持证据
- 验证方式

## 3. 推荐接入顺序

推荐按以下顺序增强 MCP 可读取的数据源：

1. OpenTelemetry Trace + 业务关联字段
2. 发布变更数据
3. Runtime / 依赖健康指标
4. Profiling / eBPF 证据

原因：

- Trace 和业务字段最直接提升开发排障体验
- 发布变更最容易解释“为什么刚刚开始坏”
- Runtime 和依赖健康帮助判断是否是服务自身或下游依赖问题
- Profiling 和 eBPF 适合处理更深层的内存、CPU、网络、进程问题

## 4. OpenTelemetry Trace

候选组件：

- Jaeger
- Grafana Tempo
- OpenSearch Trace Analytics

需要采集或暴露的字段：

- `trace_id`
- `span_id`
- `parent_span_id`
- `service.name`
- `span.name`
- `span.kind`
- `http.route`
- `http.method`
- `http.status_code`
- `rpc.method`
- `db.system`
- `db.operation`
- `error`
- `duration_ms`

MCP 可新增只读工具：

- `trace_get`
  按 `trace_id` 查询完整 trace
- `trace_search`
  按服务、接口、错误、耗时搜索 trace
- `trace_dependencies`
  查询服务上下游依赖
- `trace_slow_spans`
  查询慢 span 和失败 span

典型价值：

- 判断失败发生在哪一跳
- 找出当前服务的上游调用方
- 找出当前服务依赖的下游服务
- 把日志和 trace 通过 `trace_id` 串起来

## 5. 业务日志关联字段

推荐最小字段：

- `service`
- `biz_line`
- `request_id`
- `trace_id`
- `span_id`
- `event_id`
- `tenant_id`
- `user_id`
- `order_id`
- `error_code`
- `route`
- `version`

MCP 可新增只读工具：

- `search_business_logs`
  按业务字段检索日志
- `correlate_request`
  按 `request_id` 或 `trace_id` 聚合一次请求的日志、事件、trace
- `correlate_event`
  按 `event_id` 聚合异步任务或消息链路
- `error_code_summary`
  按 `error_code` 统计影响面

典型价值：

- 从“Pod 出错”升级到“哪个业务请求出错”
- 从“某服务异常”升级到“哪个租户、用户、订单受影响”
- 支持开发快速定位业务代码路径

## 6. 发布与变更数据

候选来源：

- Argo CD
- Helm history
- Git commit
- CI/CD 发布记录
- 镜像 tag
- 镜像 digest
- ConfigMap / Secret revision metadata
- Deployment / StatefulSet revision

MCP 可新增只读工具：

- `release_recent_changes`
  查询服务最近发布和配置变更
- `release_diff_window`
  查询异常时间窗前后的版本变化
- `release_for_workload`
  查询 workload 当前镜像、revision、commit、发布时间

典型价值：

- 判断问题是否与发布高度相关
- 判断是否某个镜像或配置变更引入异常
- 支持 RCA 页面展示“异常开始时间 vs 最近发布时间”

## 7. Runtime 与依赖健康

推荐接入：

- JVM / Micrometer
- Go runtime metrics
- Python runtime metrics
- cAdvisor
- kubelet
- node-exporter
- MySQL slow query summary
- Redis metrics
- MQ metrics
- Ingress / Gateway upstream metrics
- Third-party API error summary

MCP 可新增只读工具：

- `runtime_summary`
  查询服务运行时内存、GC、线程、goroutine、FD 等摘要
- `dependency_health`
  查询数据库、缓存、MQ、第三方 API 健康摘要
- `upstream_errors`
  查询入口网关到服务的错误率、延迟和状态码
- `db_slow_queries`
  查询慢查询摘要和影响服务

典型价值：

- 判断 OOM 是否来自堆、缓存、连接池或对象增长
- 判断错误是否来自下游数据库、Redis、MQ 或第三方接口
- 判断延迟是入口、服务自身还是依赖导致

## 8. Profiling 与 eBPF

候选组件：

- Pyroscope
- Parca
- Go pprof summary exporter
- JVM heap summary exporter
- Cilium Hubble
- Pixie
- Kepler
- 只读 eBPF exporter

MCP 可新增只读工具：

- `profile_cpu_summary`
  查询 CPU 热点摘要
- `profile_memory_summary`
  查询内存增长和分配热点摘要
- `network_flow_summary`
  查询服务间连接、丢包、重传、连接失败
- `process_oom_events`
  查询 OOM 和进程异常事件

典型价值：

- 判断 CPU 热点函数
- 判断内存增长路径
- 判断网络连接异常
- 判断 OOM 发生前后的进程级证据

## 9. 后端 API 优先原则

所有新增数据源建议先收敛到 backend 只读 API，再暴露给 MCP。

推荐路径：

1. 数据源采集或查询适配
2. backend 增加只读 API
3. RCA 页面增加只读展示
4. MCP 增加只读 tool
5. Skill 增加使用说明和排障顺序

不建议让 MCP 直接访问服务器、执行命令或绕过 backend 直接操作基础设施。

## 9.1 Codex 智能数据接入补充

在现有只读 MCP 工具基础上，后续更智能的接入方式应优先建设统一上下文层，而不是简单增加更多原子工具。

推荐补充方向：

- `Evidence Pack API`：把日志、事件、指标、Trace、发布变更、业务字段、runbook 组织成面向 Pod / Workload / Service / Incident 的结构化证据包。
- `Snapshot Index`：预计算异常 Pod、资源快速上涨、发布变更、业务错误、依赖健康等 TopN 和趋势摘要。
- `MCP Resources`：提供 `pod://`、`workload://`、`incident://`、`release://` 等对象化上下文入口。
- `AI Gateway`：统一权限、脱敏、上下文裁剪、工具路由、模型选择和审计。
- `CLI Evidence Commands`：让本地、CI、自动化任务和 Codex 共享同一份 Evidence Pack 格式。

详细设计见：

- `docs/cn/codex_intelligent_data_access.md`

## 10. 每次变更必须记录

每次让 Codex 增强 MCP / Skill / backend / frontend 后，都应写入变更记录，至少包含：

- 需求来源
- 变更目标
- 安全边界
- 修改文件
- 新增工具或接口
- 部署动作
- 验证命令
- 验证结果
- 回滚方式

推荐模板见：

- `docs/cn/mcp_skill_change_record_template.md`
