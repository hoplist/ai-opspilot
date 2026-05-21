# 2026-05-20 APISIX Service Log Correlation

## Change

OpsPilot now has a lightweight request evidence chain that keeps ELK as the
source of truth and avoids adding SigNoz, Jaeger, Tempo, or OpenObserve to the
main path.

New Core API:

```text
GET /api/evidence/request
```

New CLI command:

```bash
opspilot evidence request \
  --host workflow.tpo.xzoa.com \
  --uri /api/hr/queryUserScheduleList \
  --service-index workflow-server* \
  --service-uri-field msg \
  --since 900
```

When APISIX logs are not connected yet, the same command can run as a
service-log-only preliminary investigation:

```bash
opspilot evidence request \
  --uri /api/hr/queryUserScheduleList \
  --service-index workflow-server* \
  --service-uri-field msg \
  --service-only
```

## Behavior

The query gathers:

- APISIX access logs by `host_02`, `uri`, and time range.
- APISIX latency aggregates for `request_time` and `upstream_response_time`.
- Service logs from an explicit `--service-index` or a configured route rule.
- Evidence strength: `strong`, `medium`, `weak`, or `missing`.
- Investigation mode: `gateway_and_service`, `gateway_only`, `service_only`,
  or `no_evidence`.
- Gaps such as missing APISIX `trace_id` / `request_id` or missing service log
  route mapping.

CORS preflight requests are excluded by default through the APISIX `request`
field. Use `--include-options` only when OPTIONS traffic is the target.

## Route Rules

Long-lived host-to-service mappings can be configured with:

```text
OPSPILOT_LOG_CORRELATION_ROUTES
```

Format:

```text
name|host|path_prefix|service_index|service_uri_field|service_event_field|service_event_template|service_fallback_query
```

Examples:

```text
workflow-hr|workflow.tpo.xzoa.com|/api/hr/|workflow-server*|msg|||
devops-steps|devops.tpo.xzoa.com|/cis/api/internal/jobserver/steps/|devops-server-*|msg|evtName|cis_steps_${id}|evtName:cis_jobserver_steps
```

The `${id}` placeholder is rendered from the last URI path segment. This allows
rules such as:

```text
/cis/api/internal/jobserver/steps/19635751 -> evtName: cis_steps_19635751
```

## Evidence Strength

- `strong`: APISIX and service logs share a trace/request identifier.
- `medium`: APISIX and service logs match by route, URI, business id, and time
  window.
- `weak`: only APISIX or only service logs are available, enough for preliminary
  investigation but not enough for a full request correlation.
- `missing`: no APISIX or service-log evidence is available for the requested
  shape and time range.

APISIX is optional. If APISIX logs are missing, disabled, or intentionally
skipped, OpsPilot still queries service logs and reports `apisix_log_skipped`,
`apisix_log_empty`, or `apisix_log_query_error` as a gap instead of failing the
whole investigation.

## Notes

This is intentionally a weak-correlation design for production systems where
the log format cannot be changed. If APISIX and service logs later gain a shared
`trace_id` or `request_id`, the same API can report stronger evidence without a
new backend.
