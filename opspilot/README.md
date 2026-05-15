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

## MVP Usage

Run the core API:

```bash
python -m opspilot.core --host 127.0.0.1 --port 18080
```

Use the CLI:

```bash
python -m opspilot.cli schema
python -m opspilot.cli inventory overview
python -m opspilot.cli k8s pods --status abnormal
python -m opspilot.cli k8s logs pod -n default --pod example --tail 100
python -m opspilot.cli context pod -n default --pod example
python -m opspilot.cli diagnose pod -n default --pod example
```

The MVP core uses in-cluster Kubernetes API when service account environment is
available. Outside Kubernetes it falls back to `kubectl`.

Build the MVP image:

```bash
docker build -f opspilot/Dockerfile -t opspilot-core:0.1.0-mvp .
```

## Principles

- Contract first.
- Read-only first.
- Kubernetes Pod logs are queried on demand through `pods/log`.
- Prometheus remains the metrics backend.
- ELK remains the gateway/business/critical-log backend.
- OpenSearch, MinIO, MySQL, and eBPF are optional modules, not defaults.
