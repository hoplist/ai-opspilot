# 2026-04-28 深层排查能力路线调整

## 背景

- 测试集群当前存在 Cilium，但正式集群使用 Calico。
- 当前正式集群内部网络错误较少，Calico/CoreDNS 网络证据的近期收益不高。
- AI Gateway 暂不作为当前阻塞项，后续平台化治理时再补。

## 决策

舍弃近期 P3 网络排查建设，不再把以下内容作为当前优先级：

- Cilium Hubble
- Cilium Tetragon
- Calico 网络证据
- CoreDNS 网络证据

新的深层排查路线按以下顺序推进：

1. P0：OTel eBPF / Beyla 无侵入业务调用证据
2. P1：Falco runtime 事件证据
3. P2：Pyroscope / Parca profiling 性能剖析

## P0：OTel eBPF / Beyla

目标：

- 无代码改造采集 HTTP/gRPC/DB 调用证据。
- 补齐服务依赖、RED 指标、慢请求、错误率、trace span。
- 复用现有 OpenTelemetry Collector。

计划新增 backend 只读 API：

- `/api/otel/service-dependencies`
- `/api/otel/red-metrics`
- `/api/otel/slow-requests`
- `/api/otel/release-trace-correlation`

计划新增 MCP 只读工具：

- `service_dependency_context`
- `service_red_metrics`
- `slow_request_context`
- `release_trace_correlation`

## P1：Falco runtime 事件

目标：

- 采集容器内进程启动、shell、敏感文件访问、异常网络连接、权限提升等 runtime 事件。
- 仅作为 RCA 证据源使用，先不启用自动阻断或 enforcement。

计划新增 backend 只读 API：

- `/api/runtime/events`
- `/api/runtime/pod-process-events`
- `/api/runtime/pod-file-access-events`
- `/api/runtime/pod-exec-anomalies`

计划新增 MCP 只读工具：

- `runtime_events_context`
- `pod_process_events`
- `pod_file_access_events`
- `pod_exec_anomalies`

## P2：Pyroscope / Parca profiling

目标：

- 补齐 CPU 热点、函数调用栈、发布前后 profile diff、性能退化证据。
- 优先提供 summary，不直接让 MCP 访问节点或采集原始 profile。

计划新增 backend 只读 API：

- `/api/profiles/hotspots`
- `/api/profiles/release-diff`
- `/api/profiles/cpu-hot-path`

计划新增 MCP 只读工具：

- `profile_hotspots`
- `profile_diff_by_release`
- `cpu_hot_path_context`

## 安全边界

- 所有能力先进入 backend 只读 API，再暴露给 MCP。
- MCP 不直接访问 Kubernetes 节点，不执行主机命令。
- Falco 仅采集事件，不做自动阻断。
- profiling 只返回摘要、TopN 热点和查询链接，不返回超大原始 profile。
- 不在当前阶段建设网络 P3 能力。

## 文档变更

- 已更新 `docs/cn/mcp_readonly_observability_roadmap.md`。
- 本记录用于后续 P0/P1/P2 实施时追踪范围和优先级。
