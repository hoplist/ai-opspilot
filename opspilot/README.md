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
./opspilot/scripts/opspilot.ps1 schema
./opspilot/scripts/opspilot.ps1 metrics health
./opspilot/scripts/opspilot.ps1 k8s pods --status abnormal
go run ./opspilot/cli schema
go run ./opspilot/cli inventory overview
go run ./opspilot/cli metrics health
go run ./opspilot/cli metrics datasources
go run ./opspilot/cli metrics nodes --source all --limit 10
go run ./opspilot/cli metrics pods --source node200-k8s -n opspilot --sort cpu --limit 10
go run ./opspilot/cli metrics containers --source node206-host --sort memory --limit 10
go run ./opspilot/cli k8s pods --status abnormal
go run ./opspilot/cli k8s logs pod -n default --pod example --tail 100
go run ./opspilot/cli docker agents
go run ./opspilot/cli docker containers --host node206
go run ./opspilot/cli docker logs --host node206 --container gitlab --tail 300
go run ./opspilot/cli diagnose docker --host node206 --container gitlab
go run ./opspilot/cli logs search -n ai-dev --pod deer-flow-provisioner-8b47f95bf-t8rbt --limit 10
go run ./opspilot/cli evidence request --host workflow.tpo.xzoa.com --uri /api/hr/queryUserScheduleList --service-index workflow-server* --service-uri-field msg --since 900
go run ./opspilot/cli evidence request --uri /api/hr/queryUserScheduleList --service-index workflow-server* --service-uri-field msg --service-only
go run ./opspilot/cli context pod -n default --pod example --source node200-k8s
go run ./opspilot/cli diagnose pod -n default --pod example --source node200-k8s
```

On this workspace the PowerShell wrapper defaults to:

```text
http://192.168.48.200:32180
```

Override it with:

```powershell
.\opspilot\scripts\opspilot.ps1 -BackendUrl http://<opspilot-core>:32180 schema
```

The MVP core uses in-cluster Kubernetes API when service account environment is
available. Outside Kubernetes it falls back to `kubectl`.

Prometheus datasources are enabled by setting:

```bash
export OPSPILOT_PROMETHEUS_DEFAULT_SOURCE=node200-k8s
export OPSPILOT_PROMETHEUS_DATASOURCES=node200-k8s=http://opspilot-prometheus-server.monitoring.svc.cluster.local,external-vm=http://prometheus-vm:9090
```

Docker node agents are enabled by setting:

```bash
export OPSPILOT_NODE_AGENT_DEFAULT_HOST=node206
export OPSPILOT_NODE_AGENTS=node206=http://192.168.48.206:19080
```

OpenSearch/Elasticsearch log search is enabled by setting:

```bash
export OPSPILOT_LOGSEARCH_URL=http://opensearch.logging.svc.cluster.local:9200
export OPSPILOT_LOGSEARCH_INDEX=opspilot-k8s-*
```

APISIX to service-log evidence correlation is enabled by the same Elasticsearch
endpoint. APISIX index defaults to `apisix-*`; service logs can be provided per
query, or configured as route rules:

```bash
export OPSPILOT_APISIX_INDEX=apisix-*
export OPSPILOT_LOG_CORRELATION_ROUTES='workflow-hr|workflow.tpo.xzoa.com|/api/hr/|workflow-server*|msg|||;devops-steps|devops.tpo.xzoa.com|/cis/api/internal/jobserver/steps/|devops-server-*|msg|evtName|cis_steps_${id}|evtName:cis_jobserver_steps'
```

If APISIX logs are not connected yet, pass `--service-only` or set
`OPSPILOT_APISIX_DISABLED=true`; OpsPilot will still query service logs and mark
the missing gateway evidence as a gap.

Build the MVP image:

```bash
$env:CGO_ENABLED="0"
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot-core ./opspilot/core
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot ./opspilot/cli
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot-agent ./opspilot/agent
docker build -f opspilot/Dockerfile -t opspilot-core:0.1.0-mvp-go .
docker build -f opspilot/Dockerfile.agent -t opspilot-agent:0.1.5-docker-agent .
```

## Principles

- Contract first.
- Read-only first.
- Kubernetes Pod logs are queried on demand through `pods/log`.
- Prometheus remains the metrics backend.
- ELK remains the gateway/business/critical-log backend.
- OpenSearch, MinIO, MySQL, and eBPF are optional modules, not defaults.
