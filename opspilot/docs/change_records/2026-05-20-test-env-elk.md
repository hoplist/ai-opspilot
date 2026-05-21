# 2026-05-20 Test Environment ELK

## Change

The node200 test cluster logging backend was switched from the temporary
OpenSearch/Fluent Bit stack to an Elastic Stack deployment:

- Elasticsearch 9.3.1
- Logstash 9.3.1
- Kibana 9.3.1

Deployment manifests are stored in:

- `deploy/opspilot/elk`

## Collection Scope

Logstash runs as a DaemonSet and tails Kubernetes container logs from:

- `opspilot`
- `ai-dev`

It does not collect full-cluster logs by default.

## Endpoints

- Elasticsearch: `http://192.168.48.200:32090`
- Kibana: `http://192.168.48.200:32056`

## OpsPilot Integration

OpsPilot Core now points log search to:

```text
OPSPILOT_LOGSEARCH_URL=http://elasticsearch.logging.svc.cluster.local:9200
OPSPILOT_LOGSEARCH_INDEX=opspilot-k8s-*
```

Verified queries:

```bash
opspilot logs search -n ai-dev --pod deer-flow-provisioner-8b47f95bf-t8rbt --limit 3
opspilot logs search -n opspilot --limit 3
```
