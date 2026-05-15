# OpsPilot

OpsPilot is the next-generation, clean-slate implementation of the RCA platform.

This directory is intentionally separated from the legacy `auto_inspection`
implementation. New code should be added here instead of extending the legacy
modules.

## Components

- `core/`
  - Online read-only API. Intended to become the high-concurrency service
    boundary, implemented in Go.
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
go run ./opspilot/core --host 127.0.0.1 --port 18080
```

Use the CLI:

```bash
go run ./opspilot/cli schema
go run ./opspilot/cli inventory overview
go run ./opspilot/cli k8s pods --status abnormal
go run ./opspilot/cli k8s logs pod -n default --pod example --tail 100
go run ./opspilot/cli context pod -n default --pod example
go run ./opspilot/cli diagnose pod -n default --pod example
```

The MVP core uses in-cluster Kubernetes API when service account environment is
available. Outside Kubernetes it falls back to `kubectl`.

Build the MVP image:

```bash
$env:CGO_ENABLED="0"
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot-core ./opspilot/core
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot ./opspilot/cli
docker build -f opspilot/Dockerfile -t opspilot-core:0.1.0-mvp-go .
```

## Principles

- Contract first.
- Read-only first.
- Kubernetes Pod logs are queried on demand through `pods/log`.
- Prometheus remains the metrics backend.
- ELK remains the gateway/business/critical-log backend.
- OpenSearch, MinIO, MySQL, and eBPF are optional modules, not defaults.
