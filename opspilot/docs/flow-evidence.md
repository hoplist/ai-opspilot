# Flow Evidence

OpsPilot flow evidence is a configuration-driven model for business pipelines
that can be broken even when every Pod is Running and every local health check
returns success.

The first implementation is intentionally conservative:

- important business stages are configured by humans;
- OpsPilot reads flow metadata from the GitLab-managed config repository;
- OpsPilot does not need to run inside the managed business cluster;
- unavailable datasources are reported as evidence gaps;
- MinIO/OSS, packet capture, and eBPF tools are optional follow-up evidence,
  not required for the first pass.

## Model

Each flow is a small stage graph. A stage can represent a service, Kafka topic,
consumer group, ClickHouse table, HTTP API, or another read-only evidence point.

```yaml
apiVersion: opspilot.io/v1
kind: Flow
metadata:
  name: exception-tracking-crash
spec:
  type: crash-pipeline
  cluster: gz-inner-prod
  environment: prod
  region: guangzhou-inner
  service: exception-tracking
  window:
    default: 10m
    max: 2h
  match_keys:
    - trace_id
    - log_uuid
    - log_unique
  stages:
    - name: collector
      type: service
      service: exception-tracking-collect-server
      namespace: exception-tracking
      workload: exception-tracking-collect-server
      default_container: collect-server
      containers:
        - name: collect-server
          role: app
        - name: sidecar
          role: sidecar
      evidence:
        kubernetes_logs:
          enabled: true
          patterns:
            success:
              - send message success
            failure:
              - send message fail
        es_logs:
          datasource: gz-app-logs
          indexes:
            - app-exception-tracking-*

    - name: collector-topic
      type: kafka_topic
      datasource: gz-kafka-exporter
      topic: exception-tracking-collector
      consumer_group: exception-tracking-preagg
      evidence:
        prometheus:
          lag_query: sum by (consumergroup, topic) (kafka_consumergroup_lag{topic="exception-tracking-collector"})

    - name: clickhouse-final
      type: clickhouse
      datasource: gz-clickhouse
      database: default
      table: crash_report_data
      evidence:
        clickhouse:
          recent_rows_sql: select count() from crash_report_data where event_time >= now() - interval 10 minute
```

## Multi-Container Pods

When a stage has more than one container, set `default_container`. OpsPilot
will report `stage_default_container_missing:<stage>` when the flow is
ambiguous.

```yaml
containers:
  - name: app
    role: app
  - name: filebeat
    role: log-agent
default_container: app
```

## Commands

```powershell
opspilot flows catalog --output human
opspilot flow inspect --name exception-tracking-crash --window 10m --output human
opspilot flow inspect exception-tracking-crash --output evidence
```

The first phase returns configured stages and evidence gaps. Real ES, Kafka,
ClickHouse, HTTP, or Kubernetes log adapters can be added behind the same config
model later.

## Decision Boundary

Use manual Flow YAML for important long business chains. Automated discovery can
generate drafts from service names, topics, consumer groups, and indexes, but it
must not silently promote guessed business semantics into production config.

MinIO/OSS is deliberately optional. Add it only when the common failure mode is
attachment existence or read errors.
