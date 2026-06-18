# Inspection Policy

OpsPilot inspections are GitLab-managed YAML policies that describe what should be checked for a cluster, namespace, service, or configured business flow.

They are intentionally lightweight. The policy tells OpsPilot what evidence is expected; OpsPilot reports unavailable evidence as a gap instead of blocking unrelated checks.

## Example

```yaml
apiVersion: opspilot.io/v1
kind: Inspection
metadata:
  name: node200-daily
spec:
  cluster: node200-test
  schedule: daily
  scope:
    namespaces:
      - opspilot
    services:
      - opspilot-core
    flows:
      - exception-tracking-crash
  checks:
    - name: abnormal-pods
      type: kubernetes_pods
      enabled: true
    - name: node-resources
      type: node_resources
      enabled: true
      thresholds:
        cpu_usage_percent: 85
        memory_usage_percent: 85
    - name: filesystems
      type: filesystems
      enabled: true
      thresholds:
        usage_percent: 85
        free_gib: 20
    - name: pod-restarts
      type: pod_restarts
      enabled: true
      thresholds:
        restart_count: 3
    - name: flow-health
      type: flow
      enabled: true
      flows:
        - exception-tracking-crash
    - name: kafka-lag
      type: kafka_lag
      enabled: false
      datasource: node200-kafka-exporter
```

## Commands

```powershell
opspilot inspections catalog --output human
opspilot inspection run --name node200-daily --output human
opspilot inspection generate --cluster node200-test --output human
```

`generate` only returns a draft. It does not write config files or commit to GitLab.

## Status Meaning

| Status | Meaning |
|---|---|
| `configured` | The check is configured and can be evaluated from config-only evidence. |
| `not_executed` | The check is enabled, but no runtime adapter is wired in this phase. |
| `disabled` | The check exists but is intentionally disabled. |
| `missing_evidence` | The policy references missing config, such as a missing flow or datasource. |

## Boundary

This phase does not add a scheduler or controller. It prepares the policy model and read-only entrypoints so later automatic巡检 can use the same config without changing command behavior.
