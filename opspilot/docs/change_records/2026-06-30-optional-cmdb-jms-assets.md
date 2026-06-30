# 2026-06-30 Optional CMDB and JMS Asset Context

## Goal

Add the first optional CMDB/JMS asset-context layer without making it a hard
dependency. OpsPilot should keep working when CMDB or JumpServer/JMS is absent,
unreachable, incomplete, or disabled.

## Decisions

- Reuse the existing `assets` module as the lightweight CMDB layer.
- Add `cmdb` as a CLI alias for asset commands instead of introducing a
  separate database or service.
- Treat JumpServer/JMS as an optional readonly asset source.
- Keep deletion safe: missing assets from JMS are planned as `mark_stale`, not
  physically deleted.
- Do not update Prometheus targets, JumpServer assets, or GitLab config
  automatically in the first rollout.

## Implemented

- `AssetSource` now supports:
  - `kind: jms`
  - `kind: cmdb`
  - `required`
  - `timeout`
  - `on_error`
  - `sync.enabled`
  - `sync.mode: readonly`
  - `sync.delete_policy: mark_stale`
  - `sync.interval`
- `Asset` now supports ownership context:
  - `business_line`
  - `business`
- Added API:
  - `GET /api/assets/sync-plan?source=<name>`
- Added CLI:
  - `opspilot cmdb catalog`
  - `opspilot cmdb diff`
  - `opspilot cmdb inspect --ip <ip>`
  - `opspilot cmdb sync-plan --source <name>`
  - Existing `opspilot assets ...` commands remain compatible.
- Added disabled sample config:
  - `config/opspilot-config/assets/cmdb-jms-example.yaml`
- Updated asset source schema to allow `jms`, `cmdb`, and sync policy fields.

## Runtime Contract

CMDB/JMS is advisory-only:

```text
CMDB/JMS available:
  enrich asset, owner, business line, network zone, and bastion context

CMDB/JMS missing:
  continue troubleshooting
  return missing_evidence such as cmdb_source_missing or cmdb_source_inactive
```

Delete behavior:

```text
JMS says asset is gone
  -> mark stale in plan
  -> show affected Prometheus/agent/service metadata
  -> do not delete automatically
```

## Minimum Validation

```powershell
go test ./internal/assets ./internal/configloader ./cli
go run ./cli --output human config validate --dir ./config/opspilot-config
go run ./cli --output human cmdb catalog
go run ./cli --output human cmdb sync-plan --source jms-chengdu-inner
```

## Risk Boundary

- No credentials are printed.
- No remote JMS API call is executed yet.
- No asset, Prometheus target, GitLab file, or JumpServer object is deleted.
- `required: true` is allowed for documentation but warns against making CMDB a
  blocking dependency during the first rollout.
