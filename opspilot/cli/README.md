# opspilot CLI

Deterministic command entrypoint for humans and AI agents.

The CLI should:

- Expose stable commands.
- Expose `schema`.
- Return JSON by default.
- Avoid direct cluster mutation.
- Route all data access through `opspilot-core`.

Example future commands:

```bash
opspilot schema
opspilot inventory overview
opspilot k8s pods --status abnormal
opspilot k8s logs pod --namespace prod --pod xxx --tail 300
opspilot context pod --namespace prod --pod xxx
opspilot diagnose pod --namespace prod --pod xxx
```

MVP invocation from source:

```bash
go run ./opspilot/cli schema
go run ./opspilot/cli capabilities --output human
go run ./opspilot/cli inventory overview
go run ./opspilot/cli metrics health
go run ./opspilot/cli metrics datasources
go run ./opspilot/cli metrics nodes --source all --limit 10
go run ./opspilot/cli metrics pods --source node200-k8s -n opspilot --sort memory --limit 10
go run ./opspilot/cli metrics containers --source node206-host --sort cpu --limit 10
go run ./opspilot/cli metrics filesystems --source all --output table
go run ./opspilot/cli k8s pods --status abnormal
go run ./opspilot/cli docker agents
go run ./opspilot/cli docker containers --host node206
go run ./opspilot/cli docker logs --host node206 --container gitlab --tail 300
go run ./opspilot/cli diagnose docker --host node206 --container gitlab
go run ./opspilot/cli host disk --host node206 --limit 20 --output human
go run ./opspilot/cli host network --host node206 --duration 5 --limit 20 --output human
go run ./opspilot/cli host cleanup plan --host node206 --output human
go run ./opspilot/cli logs search -n ai-dev --pod deer-flow-provisioner-8b47f95bf-t8rbt --query error --since 1800 --limit 10
go run ./opspilot/cli evidence request --host workflow.tpo.xzoa.com --uri /api/hr/queryUserScheduleList --service-index workflow-server* --service-uri-field msg --since 900
go run ./opspilot/cli evidence request --uri /api/hr/queryUserScheduleList --service-index workflow-server* --service-uri-field msg --service-only
go run ./opspilot/cli inspect pod -n ai-dev --pod sandbox-errno36-test --source node200-k8s --output human
go run ./opspilot/cli inspect service opspilot-core --output human
go run ./opspilot/cli inspect cluster --source all --output human
go run ./opspilot/cli release service opspilot-core --output human
go run ./opspilot/cli release service opspilot-core --trigger --ref main --output human
go run ./opspilot/cli release status --service opspilot-core --output human
go run ./opspilot/cli release jobs --service opspilot-core --output human
go run ./opspilot/cli release logs --service opspilot-core --job build-image --tail 200 --output human
go run ./opspilot/cli quality run service opspilot-core --output human
go run ./opspilot/cli quality status service opspilot-core --output evidence
go run ./opspilot/cli onboard repo tpo/devex/demo/demo-api --write --output human
go run ./opspilot/cli onboard service --config opspilot.service.yaml --write
go run ./opspilot/cli repo preflight --repo . --project tpo/devex/demo/demo-api --output human
go run ./opspilot/cli repo upload-plan --repo . --name demo-api --output human
go run ./opspilot/cli repo upload --repo . --name demo-api --confirm --output human
go run ./opspilot/cli ask "检查 opspilot-core 是否正常" --output human
go run ./opspilot/cli ask "发布 opspilot-core" --dry-run --output human
```

Generated onboarding manifests include CPU/memory requests and limits,
readiness/liveness probes, plus namespace `LimitRange` and `ResourceQuota`
guardrails. `repo preflight` blocks release readiness when those are missing.
Onboarding also writes optional `.opspilot/quality.yaml` API smoke checks.
Those checks are release evidence only; missing quality config or runner setup
does not block Kubernetes-first troubleshooting or normal release status.

`capabilities` reports which evidence sources are currently usable and which
ones are missing. `inspect pod`, `inspect service`, `inspect cluster`, and
natural-language inspect output include the same available/missing evidence
summary so missing integrations do not block Kubernetes-first troubleshooting.

Cluster nodes are monitored through Prometheus plus node-exporter:

```powershell
.\opspilot\scripts\opspilot.ps1 metrics nodes --source node200-k8s --output human
.\opspilot\scripts\opspilot.ps1 metrics filesystems --source node200-k8s --output table
```

Use `host disk` only when mountpoint-level metrics are not enough and you need
host directory attribution, Docker reclaimable bytes, or container json log
sizes from a configured read-only node agent. The agent supports trailing
`*` allowed path patterns such as `/data*` for extra mounted directories like
`/data00`, but directory attribution can add I/O pressure on very large trees;
keep depth and limit small for routine checks.

Use `host network` for a short, read-only network snapshot from a configured
node agent. It samples `/proc/net/dev` and allowed Docker container stats for a
bounded duration, reports RX/TX rates and TCP state counts, and does not require
OTel or eBPF.

`logs search` defaults to a short Elasticsearch/OpenSearch time window and
caps large requests before they reach the backend. Use `--since` when you need a
wider window; OpsPilot still clamps oversized windows and result limits.

Build a local binary:

```powershell
.\opspilot\scripts\build-cli.ps1
.\build\opspilot.exe --backend-url http://192.168.48.200:32180 metrics health
```

The wrapper prefers the built binary when `build\opspilot.exe` exists, and
falls back to `go run` when it does not:

```powershell
.\opspilot\scripts\opspilot.ps1 metrics health
```

Cross-build examples:

```powershell
.\opspilot\scripts\build-cli.ps1 -TargetOS linux -TargetArch amd64
.\opspilot\scripts\build-cli.ps1 -TargetOS windows -TargetArch amd64
```
