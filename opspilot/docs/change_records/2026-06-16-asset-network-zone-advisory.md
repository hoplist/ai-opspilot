# Asset Network Zone Advisory

## Background

OpsPilot needs to correlate server resources across fixed network segments,
multiple JumpServer deployments, Prometheus, and node agents. Current JumpServer
coverage is incomplete, and the first rollout must not delete JumpServer assets
or Prometheus targets.

Known first-stage network hints:

- 成都外网: `10.234.4.0/24`
- 成都内网: `10.65.0.0/16`
- 广州内网: `10.236.21.0/24`
- 广州/云上入口: `10.236.12.19`

## Goal

Land a read-only asset registry foundation:

- classify IPs into configured network zones;
- expose planned JumpServer/Prometheus/agent asset sources;
- report missing evidence and removal candidates as advice only;
- keep all destructive actions out of scope.

## Design

`opspilot-config` becomes the human-maintained source for asset metadata:

- `NetworkZone`: region, zone, CIDR ranges, entrypoint IPs, coverage state, and
  advisory-only action policy;
- `AssetSource`: planned data source such as JumpServer, Prometheus, agent, or
  manual inventory;
- `Asset`: optional static/manual asset record with IPs, observed sources, and
  expected sources.

OpsPilot exposes read-only APIs and CLI commands:

- `assets zones`
- `assets catalog`
- `assets inspect --ip <ip>`
- `assets diff`

## Safety Boundary

This stage is advisory only:

- no JumpServer mutation;
- no Prometheus target deletion;
- no `file_sd` generation;
- no background reconciler;
- no automatic asset removal.

`remove_candidate` findings only mean "needs human confirmation".

## Minimal Verification

- `opspilot config validate --dir ./opspilot/config/opspilot-config`
- `opspilot assets zones --output human`
- `opspilot assets inspect --ip 10.236.12.19 --output human`
- `opspilot assets diff --output human`
- Go tests for config loading, zone matching, advisory diff, and CLI output.

## Deferred

- Real JumpServer API pull.
- Prometheus `file_sd_configs` target generation.
- Automatic stale/removal policy execution.
- VM platform adapters such as vCenter, ESXi, OpenStack, or Proxmox.
