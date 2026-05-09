# Incidents Flow

## Current Status

The incidents pipeline now writes Pod-oriented incidents into OpenSearch using the
current in-cluster Prometheus source and current OpenSearch deployment.

Current index pattern:

- `inspection-incidents-*`

## Sources

Incidents are now built from:

- the current Prometheus URL configured in `config.json`
- the current OpenSearch deployment configured in `config.json`

The local source fingerprint is attached to generated artifacts and incident
documents so source changes can be isolated.

## Backend APIs

- `GET /api/incidents/list`
- `GET /api/incidents/search`

Current behavior:

- `inspection-incidents-*` is the primary source for current incidents.
- Local JSON artifacts are now treated as cache/fallback only.
- Incident list/search responses are filtered by the current Prometheus source
  fingerprint to avoid mixing old-source data.
- Incident responses can include direct Dashboards links for logs, events, and
  overview dashboard jumps.

## RCA Integration

- The RCA page `dashboard-rca` now reads current incidents from the incidents
  API rather than directly from local event artifacts.
- Each incident card exposes direct actions:
  - `Run RCA`
  - `Open Logs`
  - `Open Events`
  - `Open Dashboard`

## Mapping Chain

`instance -> namespace/pod/workload` resolution now prefers this chain:

1. Prometheus active target metadata
2. `kube_pod_info` pod identity metadata
3. Kubernetes Service selector fallback

This gives the investigation target list a more stable route from old-style
Prometheus instance values into Kubernetes investigation objects.

## MCP Tools

- `list_incidents`
- `search_incidents`

## Skill Commands

- `list-incidents`
- `search-incidents`

## Notes

- The incident pipeline currently focuses on Pod incidents generated from
  restart, waiting, readiness, termination, and OOM-related signals.
- This gives Codex real incident records to query even when the legacy
  node anomaly path does not produce results.
