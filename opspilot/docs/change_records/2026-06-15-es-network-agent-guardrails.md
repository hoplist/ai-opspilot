# 2026-06-15 ES Query Guardrails And Agent Network Snapshot

## Background

OpsPilot is preparing to query an online Elasticsearch backend that may contain
multiple APISIX access-log indexes. The integration must stay read-only and
avoid expensive long-window searches. The node-side agent also needs a light
network traffic view without introducing OTel, a Kubernetes Operator, or a
long-running eBPF component.

Some hosts mount business data under paths such as `/data00`, so disk
attribution needs to include `data*` style mount directories while remaining
bounded.

## Changes

- Added Elasticsearch/OpenSearch query guardrails for `logs search`:
  - default lookback window: 1800 seconds;
  - maximum lookback window: 7200 seconds;
  - maximum result limit: 100;
  - request timeout: 5 seconds;
  - `track_total_hits=false`;
  - `_source` field filtering.
- Applied the same timeout, source filtering, and time-window clamping to
  APISIX/service-log evidence queries.
- Stopped treating `kind: kibana` as a queryable log datasource. Kibana remains
  display/navigation metadata; actual queries go to Elasticsearch/OpenSearch.
- Added `host network` / `host traffic` CLI support through OpsPilot Core and
  `opspilot-agent`.
- Added `GET /api/host/network` to the agent:
  - reads `/proc/net/dev` twice over a bounded duration;
  - reads allowed Docker container network counters;
  - reports RX/TX rates and TCP state counts;
  - does not use eBPF and does not capture payloads.
- Extended default disk allowed paths to:
  - `/var/lib/docker`
  - `/var/log`
  - `/opt`
  - `/data`
  - `/data*`
- Added runtime expansion for trailing `*` host path patterns, so `/data*`
  includes mounted paths such as `/data00` and `/data01` when present.
- Updated the node206 external compose template to pass `/data*` in
  `OPSPILOT_AGENT_DISK_ALLOWED_PATHS`. Extra host data directories still need
  matching read-only mounts such as `/data00:/host/data00:ro`.

## Boundaries

- No Kubernetes Operator or controller was introduced.
- No OTel stack was introduced.
- No eBPF code was introduced in this stage.
- Network sampling is read-only and short-lived. Default duration is 5 seconds;
  maximum duration is 30 seconds.
- Disk attribution remains read-only and bounded by allowed paths, depth, top
  limit, and request timeout. It can still add I/O pressure on very large
  directories, so frequent checks should prefer Prometheus filesystem metrics.

## Validation

Targeted validation completed:

```powershell
go test ./agent ./internal/nodeagent ./internal/logsearch ./internal/configloader ./cli ./core
```

Result: passed.
