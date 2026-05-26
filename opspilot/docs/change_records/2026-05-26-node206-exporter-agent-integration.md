# 2026-05-26 node206 exporter and agent integration

## Scope

- Integrated node206 node_exporter into the node200 cluster Prometheus datasource.
- Confirmed node206 opspilot-agent remains connected through the OpsPilot Docker agent API.

## Runtime Change

Updated ConfigMap:

```text
monitoring/auto-prometheus-server
```

Added Prometheus static scrape job:

```yaml
- job_name: node206-node-exporter
  static_configs:
  - targets:
    - 192.168.48.206:9100
    labels:
      node: node206
      host: node206
      nodename: nfs-206
      role: external-docker-host
```

The Prometheus server has a configmap reloader sidecar watching `/etc/config`,
and it reloaded `/etc/config/prometheus.yml` successfully after the ConfigMap
change.

## Agent Handling

`opspilot-agent` on node206 exposes health and Docker troubleshooting APIs:

```text
http://192.168.48.206:19080/health
http://192.168.48.206:19080/api/containers
```

It does not currently expose a Prometheus `/metrics` endpoint; `/metrics`
returns 404. Therefore it was not added as a Prometheus scrape target. Its
OpsPilot integration remains API based:

```text
node206=http://192.168.48.206:19080
```

## Validation

- `up{job="node206-node-exporter"}` from datasource `node200-k8s` returns `1`.
- Prometheus active target `node206-node-exporter` is `up` with scrape URL
  `http://192.168.48.206:9100/metrics`.
- `node_uname_info{job="node206-node-exporter"}` returns node206 host evidence.
- `opspilot metrics nodes --source node200-k8s` now includes node206.
- `opspilot docker agents` reports node206 agent ready.
- `opspilot docker containers --host node206` returns node206 Docker container
  inventory.

## Follow-up

The edited Prometheus ConfigMap is Helm-owned. If the Prometheus chart is later
managed by GitOps/Helm values, this static scrape job should be moved into the
chart values to avoid losing it during a Helm upgrade.
