# 2026-05-20 Docker Node Agent

## Background

node206 already exposes Docker container CPU and memory metrics through
Prometheus/cAdvisor, but metrics do not include stdout/stderr logs or Docker
runtime state. OpsPilot needs a lightweight, temporary troubleshooting path
without enabling full log collection for every container.

## Change

- Added `opspilot-agent`, a read-only Docker node helper.
- Added local allowlist based access control through
  `OPSPILOT_AGENT_ALLOWED_CONTAINERS`.
- Added bounded short-window Docker log reads.
- Added read-only Docker inspect and one-shot stats endpoints.
- Added OpsPilot Core node-agent registry through:
  - `OPSPILOT_NODE_AGENT_DEFAULT_HOST`
  - `OPSPILOT_NODE_AGENTS`
- Added Core APIs:
  - `GET /api/node-agents`
  - `GET /api/docker/containers`
  - `GET /api/docker/inspect`
  - `GET /api/docker/logs`
  - `GET /api/docker/stats`
  - `GET /api/diagnose/docker`
- Added CLI commands:
  - `opspilot docker agents`
  - `opspilot docker containers --host node206`
  - `opspilot docker inspect --host node206 --container gitlab`
  - `opspilot docker logs --host node206 --container gitlab`
  - `opspilot docker stats --host node206 --container gitlab`
  - `opspilot diagnose docker --host node206 --container gitlab`

## Safety

- The agent only exposes `GET` routes.
- The Docker socket is mounted read-only in the compose example.
- The agent does not support exec, restart, stop, remove, pull, or update.
- If the allowlist is empty, container-level requests are denied.
- Logs are bounded by tail, since, and byte limits.

## node206 Deployment

Deployment files are stored in:

- `deploy/external/node206/opspilot-agent`

Current example allowlist:

```text
gitlab,prometheus,cadvisor,node-exporter,opspilot-agent
```
