# Flow Evidence Catalog

## Background

Some business pipelines can fail even when all Kubernetes workloads are Running
and service health endpoints are green. The current example is a long log
pipeline where SDK upload, collector, Kafka topics, pre-aggregation, engine
consumers, ClickHouse tables, and Web APIs must all move for the business result
to be usable.

This is business-specific and should not be guessed fully by OpsPilot. Important
chains should be configured by humans once, then inspected automatically.

## Decision

Add a first version of configuration-driven Flow Evidence:

- `kind: Flow` in the GitLab-managed OpsPilot config repository;
- `/api/flows/catalog` and `/api/flows/inspect`;
- `opspilot flows catalog`;
- `opspilot flow inspect`;
- stage-level evidence gap output;
- multi-container stage metadata with `default_container`;
- no hard dependency on MinIO/OSS, KubeShark, pwru, or in-cluster OpsPilot.

## Scope

This phase is configuration and evidence-shape only. It does not yet query ES,
Kafka exporter, ClickHouse, Web APIs, or remote Kubernetes logs. Missing adapters
must be reported as gaps instead of blocking other OpsPilot features.

## Configuration Example

```yaml
apiVersion: opspilot.io/v1
kind: Flow
metadata:
  name: exception-tracking-crash
spec:
  type: crash-pipeline
  cluster: gz-inner-prod
  region: guangzhou-inner
  service: exception-tracking
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
      evidence:
        kubernetes_logs:
          enabled: true
        es_logs:
          datasource: gz-app-logs
    - name: crash-topic
      type: kafka_topic
      datasource: gz-kafka-exporter
      topic: crash
      consumer_group: exception-tracking-preagg
      evidence:
        prometheus:
          lag_query: sum by (consumergroup, topic) (kafka_consumergroup_lag{topic="crash"})
```

## Minimum Validation

High-risk operations are not introduced in this phase. Minimum validation is:

1. config validation can load `kind: Flow`;
2. `flows catalog` lists configured flows;
3. `flow inspect` returns stage metadata and explicit missing evidence;
4. existing OpsPilot release and inspect features continue to work.

The user requested no business demo test for this change.
