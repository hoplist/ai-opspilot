# 2026-05-27 Capability detection

## Decision

Add a unified capability/evidence probe so OpsPilot can tell users and AI
assistants which troubleshooting sources are available and which ones are
missing.

## Scope

- Added `GET /api/capabilities`.
- Added `opspilot capabilities`.
- Added available/missing evidence summaries to:
  - `inspect pod`;
  - `inspect service`;
  - `inspect cluster`;
  - natural-language service inspection output.
- Kept Kubernetes API and Kubernetes Pod logs as the baseline troubleshooting
  capability.
- Treated Prometheus, ELK/OpenSearch, service-log correlation, APISIX logs,
  Docker node-agent, GitLab/GitOps/Registry evidence, and Argo CD as optional
  evidence sources.

## Behavior

If an optional integration is not configured or not reachable, OpsPilot reports
the missing evidence directly while continuing with the available evidence.

Example:

```text
Available evidence: Pod status; Pod events; Kubernetes current logs
Missing evidence: Prometheus is unavailable, so CPU/memory trends are missing;
ELK/OpenSearch is unavailable, so historical or rotated logs are missing
```

## Validation

Run:

```powershell
go test ./opspilot/...
.\opspilot\scripts\build-cli.ps1
.\opspilot\scripts\opspilot.ps1 capabilities --output human
```
