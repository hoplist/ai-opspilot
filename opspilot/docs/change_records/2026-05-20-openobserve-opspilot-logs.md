# 2026-05-20 OpenObserve OpsPilot Logs

## Change

OpenObserve was connected as a narrow log sink for OpsPilot service logs only.
This does not replace ELK and does not collect `ai-dev` / deer-flow logs into
OpenObserve.

## Collection Path

```text
Kubernetes /var/log/containers/*_opspilot_*.log
  -> existing Logstash DaemonSet
  -> Elasticsearch index opspilot-k8s-*
  -> OpenObserve stream opspilot_ops
```

OpenObserve ingest endpoint:

```text
http://openobserve.openobserve.svc.cluster.local:5080/api/default/opspilot_ops/_json
```

External UI/API endpoint:

```text
http://192.168.48.200:32580
```

## Scope

Only records whose parsed Kubernetes namespace is `opspilot` are forwarded to
OpenObserve. The existing ELK output is unchanged.

## Auth

Logstash uses a Kubernetes Secret in `logging` namespace:

```text
openobserve-auth
```

The current OpenObserve root credentials come from:

```text
openobserve/openobserve-root
```

## Verification

Query stream:

```sql
SELECT * FROM "opspilot_ops" ORDER BY _timestamp DESC LIMIT 5
```
