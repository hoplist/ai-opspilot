# OpenSearch Operations

## Current State

The local auto_inspection OpenSearch deployment now uses:

- explicit index templates for logs / events / incidents / investigations
- normalized log field mappings for `message`, `severity`, `logger`, `service`,
  exception fields, and stack language
- ISM retention policies for all four index families
- a local filesystem snapshot repository baseline
- disk watermarks enabled in the OpenSearch node config

## Retention Strategy

The current write strategy is date-based:

- `logs-k8s-*`
- `events-k8s-*`
- `inspection-incidents-*`
- `inspection-investigations-*`

This means the index family already rolls over by day. ISM is used here mainly
for retention / cleanup rather than alias-based rollover.

Default retention windows:

- logs: 14 days
- events: 30 days
- incidents: 60 days
- investigations: 90 days

These values are configurable through:

- `OPENSEARCH_RETENTION_LOGS_DAYS`
- `OPENSEARCH_RETENTION_EVENTS_DAYS`
- `OPENSEARCH_RETENTION_INCIDENTS_DAYS`
- `OPENSEARCH_RETENTION_INVESTIGATIONS_DAYS`

## Bootstrap

Run:

```powershell
python bootstrap_opensearch.py
```

This will install or refresh:

- index templates
- incident template
- ISM policies
- snapshot repository registration

## Snapshot Repository

The default snapshot repository is:

- repository: `auto-inspection-local-fs`
- location: `/usr/share/opensearch/data/snapshots`

This is suitable as a runnable baseline for the current test environment. For a
production-grade setup, move snapshots onto an independent storage location.

## Disk Safety

The deployment manifest enables:

- `cluster.routing.allocation.disk.threshold_enabled: true`
- low watermark: `85%`
- high watermark: `90%`
- flood stage: `95%`

This gives a safer baseline for a single-node test deployment while keeping
current disk utilization below the protection thresholds.

## Next Operations Work

The following items are still recommended as the next hardening step on the
cluster, but were not automated in this pass:

- snapshot repository and scheduled backup
- security plugin enablement with real auth
- alerting on disk, heap, and write errors
