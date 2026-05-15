# OpsPilot

OpsPilot is the next-generation, clean-slate implementation of the RCA platform.

This directory is intentionally separated from the legacy `auto_inspection`
implementation. New code should be added here instead of extending the legacy
modules.

## Components

- `core/`
  - Online read-only API. Intended to become the high-concurrency service
    boundary, preferably implemented in Go.
- `cli/`
  - Deterministic command-line interface for humans and AI agents.
- `mcp/`
  - MCP adapter exposing read-only tools.
- `worker/`
  - Async jobs: baseline, health snapshots, reports, backup verification, AI
    summaries.
- `console/`
  - Web UI.
- `contracts/`
  - OpenAPI, JSON Schema, CLI schema, and MCP tool contracts.

## Principles

- Contract first.
- Read-only first.
- Kubernetes Pod logs are queried on demand through `pods/log`.
- Prometheus remains the metrics backend.
- ELK remains the gateway/business/critical-log backend.
- OpenSearch, MinIO, MySQL, and eBPF are optional modules, not defaults.
