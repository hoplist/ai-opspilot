# 2026-06-24 Probe Policy Config

## 目标

HTTP probe 的基础能力保留在代码中，但“查哪些证据源、缺失是否阻塞、索引和字段如何匹配”应由 GitLab 配置维护，避免把定制化排查逻辑写进 `core`。

## 本阶段改动

- 新增配置类型：`kind: ProbePolicy`。
- 新增配置目录：`config/opspilot-config/probes/`。
- 新增默认策略：`default-http-probe`。
- `probe http` 执行流程改为读取 `ProbePolicy.evidence`：
  - `gateway_logs`：APISIX/nginx access 日志，缺失默认 `warn`。
  - `service_logs`：应用 ES 日志，缺失默认 `warn`。
  - `kubernetes_pod`：Pod 状态，参数缺失或不可用默认 `skip`。
  - `prometheus_pod`：Pod 指标，缺失默认 `warn`。
- Evidence Pack 中记录实际使用的 `policy`。

## 代码边界收敛

- 新增 `internal/probeevidence`，集中处理：
  - `ProbePolicy.evidence` 的证据项解析。
  - gateway/service log 的启停和缺失策略。
  - HTTP probe 对应的 Evidence Pack 组装。
- `core/routes_probe.go` 收敛为：
  - 解析 HTTP/API 入参。
  - 调用 `internal/httpprobe` 发起受控请求。
  - 按 `ProbePolicy` 编排日志、Kubernetes、Prometheus 证据查询。
  - 持久化 Evidence Pack。
- 后续定制化排查不应继续写入 `core`：
  - 域名、索引、字段、时间窗口、证据源开关优先放 `opspilot-config`。
  - 新证据类型优先做 datasource/evidence adapter。
  - `core` 只保留通用 API 编排，不承载某个业务链路的特殊逻辑。
- 后续新增额外链路时，默认流程为：
  - 在 GitLab 管理的 `opspilot-config` 仓库新增或修改 `Service`、`Datasource`、`Flow`、`ProbePolicy`、`Inspection` 等配置。
  - 通过配置热同步让 `opspilot-core` 读取新链路。
  - 只有出现新的通用证据类型、通用数据源协议或通用查询能力时，才修改代码。
  - 不允许为了某个业务接口、某个固定域名、某个单独日志格式把特殊排查逻辑写入 `core`。

## 配置示例

```yaml
apiVersion: opspilot.io/v1
kind: ProbePolicy
metadata:
  name: default-http-probe
spec:
  default: true
  target: http
  window:
    since_seconds: 900
    window_seconds: 300
    limit: 20
  evidence:
    - name: gateway_logs
      type: gateway_logs
      enabled: true
      required: false
      on_missing: warn
      index: apisix-*
      match_fields: [host, uri, status, probe_id, user_agent]
    - name: service_logs
      type: service_logs
      enabled: true
      required: false
      on_missing: warn
      index: opspilot-k8s-*
      uri_field: msg
      match_fields: [uri, trace_id, probe_id, keyword]
    - name: pod
      type: kubernetes_pod
      enabled: true
      required: false
      on_missing: skip
    - name: metrics
      type: prometheus_pod
      enabled: true
      required: false
      on_missing: warn
      source: node200-k8s
```

## 边界

- HTTP 请求、超时、脱敏、响应截断仍由 `internal/httpprobe` 固定实现。
- 证据源启停和缺失策略由配置控制。
- 未配置或证据源不可用时不阻塞 HTTP probe 和 Evidence Pack 生成。
- 新增区域、多 ES、多 nginx/APISIX 索引、不同服务日志字段时，优先改 GitLab 配置，不改 `core`。
