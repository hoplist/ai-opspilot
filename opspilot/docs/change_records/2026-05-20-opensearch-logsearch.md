# 2026-05-20 OpenSearch Log Search

> Superseded by `2026-05-20-test-env-elk.md`. The test environment now uses
> Elasticsearch, Logstash, and Kibana.

## Change

- Added a lightweight OpenSearch based logging stack in `deploy/opspilot/logging`.
- Fluent Bit collects only `opspilot` and `ai-dev` namespace logs.
- OpenSearch stores logs in `opspilot-k8s-*` indices.
- OpenSearch Dashboards is exposed on NodePort `32056`.
- OpsPilot Core can query collected logs through:
  - `GET /api/logs/search`
- OpsPilot CLI can query logs through:
  - `opspilot logs search -n ai-dev --pod deer-flow-provisioner-8b47f95bf-t8rbt`

## Endpoints

- OpenSearch: `http://192.168.48.200:32090`
- OpenSearch Dashboards: `http://192.168.48.200:32056`

## Scope

This is intentionally not full-cluster log collection. Fluent Bit keeps only:

- OpsPilot logs from `opspilot`
- Deer Flow provisioner logs from `ai-dev`
