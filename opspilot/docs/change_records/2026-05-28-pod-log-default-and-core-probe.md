# 2026-05-28 Pod Log Default And Core Probe

## Goal

Make Pod console/log inspection default to a longer troubleshooting window and
avoid restarting `opspilot-core` because its heavy health endpoint depends on
external integrations.

## Changes

- Changed the Kubernetes Pod log default window from 30 minutes to 10 hours
  (`36000` seconds).
- Updated CLI/OpenAPI contract defaults for `k8s logs pod` to match the
  10-hour default.
- Added `/api/live` as a lightweight core liveness/readiness endpoint.
- Updated `opspilot-core` readiness and liveness probes to use `/api/live`
  instead of `/api/health`.

`/api/health` remains available for CLI doctor/capability checks and still
reports Prometheus, node-agent, logsearch, and release mapping health.
