---
name: opspilot-ops
description: "Use when Codex needs to inspect or troubleshoot the OpsPilot-managed operations platform through the read-only OpsPilot CLI: Kubernetes inventory and Pods, ELK log search, Prometheus metrics, node206 Docker containers, APISIX/service-log evidence chains, service-only preliminary investigation, or OpsPilot health. CLI-first, no MCP."
---

# OpsPilot Ops

Use the OpsPilot CLI first. Do not use MCP for this platform unless the user
explicitly asks for it.

## Entry

From `D:\code\auto_inspection`, prefer:

```powershell
.\opspilot\scripts\opspilot.ps1 <command>
```

Default backend:

```text
http://192.168.48.200:32180
```

Fallback when the wrapper is unavailable:

```powershell
go run ./opspilot/cli --backend-url http://192.168.48.200:32180 <command>
```

## Start Every Investigation

Check platform reachability before deeper work:

```powershell
.\opspilot\scripts\opspilot.ps1 doctor --output human
```

If the user asks what the platform can do:

```powershell
.\opspilot\scripts\opspilot.ps1 schema
.\opspilot\scripts\opspilot.ps1 skills registry --output human
```

## Kubernetes Workflow

For broad cluster state:

```powershell
.\opspilot\scripts\opspilot.ps1 check cluster --source all --output human
```

For one Pod:

```powershell
.\opspilot\scripts\opspilot.ps1 check pod -n <namespace> --pod <pod> --source all --output human
```

Use short-window logs only when needed:

```powershell
.\opspilot\scripts\opspilot.ps1 k8s logs pod -n <namespace> --pod <pod> --tail 300
```

## Logs

Search collected Kubernetes logs:

```powershell
.\opspilot\scripts\opspilot.ps1 logs search -n opspilot --limit 20
.\opspilot\scripts\opspilot.ps1 logs search -n ai-dev --limit 20
```

Do not treat OpenObserve as the main log backend. It currently stores only
OpsPilot self logs in stream `opspilot_ops`.

## Interface And Service Evidence

If APISIX logs are not connected or the user only gives a service URI, use
service-only mode first:

```powershell
.\opspilot\scripts\opspilot.ps1 evidence request `
  --uri /api/hr/queryUserScheduleList `
  --service-index workflow-server* `
  --service-uri-field msg `
  --service-only
```

If APISIX host and logs are available, use full evidence mode:

```powershell
.\opspilot\scripts\opspilot.ps1 evidence request `
  --host workflow.tpo.xzoa.com `
  --uri /api/hr/queryUserScheduleList `
  --service-index workflow-server* `
  --service-uri-field msg `
  --since 900
```

Read these fields first:

- `investigation_mode`: `service_only`, `gateway_and_service`, `gateway_only`, or `no_evidence`.
- `evidence_strength`: `strong`, `medium`, `weak`, or `missing`.
- `gaps`: missing evidence such as `apisix_log_skipped` or missing service logs.
- `service_logs.items`: matched business or service logs.
- `apisix.latency`: gateway latency summary when APISIX evidence exists.

## Docker Host Workflow

For node206 Docker containers:

```powershell
.\opspilot\scripts\opspilot.ps1 docker containers --host node206
.\opspilot\scripts\opspilot.ps1 docker logs --host node206 --container <container> --tail 300
.\opspilot\scripts\opspilot.ps1 diagnose docker --host node206 --container <container>
```

## Output

For AI follow-up, prefer evidence output:

```powershell
.\opspilot\scripts\opspilot.ps1 check service <service> --output evidence
.\opspilot\scripts\opspilot.ps1 fix service <service> --dry-run --output evidence
```

Use `fix ... --dry-run` only as a plan generator. It does not mutate code,
repositories, or clusters.
Read `skill_recommendations` to decide whether the evidence should be followed
up with Kubernetes, monitoring, release, RCA, or debugging rules.

Summarize:

1. What is happening.
2. Evidence found.
3. Missing evidence.
4. Most likely cause or current confidence.
5. Next read-only check.

Do not dump raw JSON unless the user asks.

## Boundaries

Allowed: read-only OpsPilot CLI commands.

For packaging, image builds, and deployments, use the standard release flow:

```text
node206 GitLab
-> node206 GitLab Runner
-> BuildKit rootless image build
-> Push image registry
-> Update GitOps repository
-> node200 Argo CD automatic deployment
```

Local builds are only for CLI/unit-test validation. Release artifacts and
cluster deployments should go through CI and GitOps.

For release troubleshooting, treat the pipeline as a read-only evidence chain:
GitLab pipeline, BuildKit job logs, Registry image tag, GitOps commit, Argo CD
sync/health, Kubernetes rollout, Pod metrics, and logs. Always report missing
evidence explicitly.

For release status:

```powershell
.\opspilot\scripts\opspilot.ps1 --output human release status --service opspilot-core
```

Do not perform direct mutation:

- `kubectl apply`, `delete`, `patch`, `scale`, `rollout restart`
- shelling into production nodes
- reading Kubernetes Secret values
- changing ELK, Prometheus, OpenObserve, or Docker configuration

If a fix needs mutation, report the evidence and ask for confirmation.
