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
