# 2026-05-20 CLI And Skill Entry

## Change

OpsPilot now has a short local PowerShell CLI wrapper and an updated CLI-first
Codex skill.

Wrapper:

```text
opspilot/scripts/opspilot.ps1
```

Default backend:

```text
http://192.168.48.200:32180
```

Skill:

```text
opspilot/skill/SKILL.md
```

## Usage

From `D:\code\auto_inspection`:

```powershell
.\opspilot\scripts\opspilot.ps1 schema
.\opspilot\scripts\opspilot.ps1 metrics health
.\opspilot\scripts\opspilot.ps1 k8s pods --status abnormal
.\opspilot\scripts\opspilot.ps1 logs search -n opspilot --limit 20
```

Service-only evidence:

```powershell
.\opspilot\scripts\opspilot.ps1 evidence request `
  --uri /api/hr/queryUserScheduleList `
  --service-index workflow-server* `
  --service-uri-field msg `
  --service-only
```

## Direction

The CLI is the deterministic execution layer. The skill is the AI workflow layer
that chooses safe read-only CLI commands and summarizes evidence, confidence,
and missing sources.

## Deployment Config

Core runtime environment variables were moved out of the Deployment into:

```text
deploy/opspilot/core/configmap.yaml
```

The Deployment now reads them through `envFrom`, so deploying to another cluster
only requires editing the ConfigMap values for Prometheus, log search, APISIX,
and optional node agents.
