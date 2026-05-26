# 2026-05-26 AI readable error evidence

## Background

Automatic middleware and release automation must expose failures in a form that
OpsPilot and AI assistants can read directly. A raw CI trace or Pod log is not
enough because the AI needs stable fields: stage, service, severity, message,
evidence, probable cause, and next checks.

## Decision

Add an OpsPilot error evidence surface. It should be usable by CLI, API, and AI
tools without requiring manual log browsing.

Initial scope:

- expose recent structured error events through OpsPilot Core;
- provide CLI access for AI calls;
- include Kubernetes abnormal Pods, Argo CD degraded/out-of-sync apps, and
  release evidence gaps;
- define a file/JSON contract that future middleware provisioning steps can
  write when database/user/vhost/bucket/topic provisioning fails.

## Error Event Contract

```json
{
  "id": "stable-id",
  "time": "2026-05-26T18:00:00+08:00",
  "source": "kubernetes|argocd|release|middleware",
  "stage": "preflight|build|gitops|sync|rollout|provision|runtime",
  "service": "orders-api",
  "namespace": "cicd-devex-orders",
  "resource": "pod/orders-api-xxx",
  "severity": "info|warning|critical",
  "status": "open|resolved|unknown",
  "message": "human readable summary",
  "evidence": ["bounded evidence lines"],
  "probable_cause": "short reason when inferable",
  "next_checks": ["safe read-only checks"],
  "raw": {}
}
```

## Follow-up

Middleware runtime provisioning should write one event per failed allocation,
for example:

```text
source=middleware
stage=provision
service=orders-api
resource=mysql/devex_orders_orders_api_mysql
message=failed to create database user
```

OpsPilot release status and `inspect service` should later include these events
beside Pod, Argo, GitLab, GitOps, metric, and log evidence.

## Implemented In This Change

- Added OpsPilot Core endpoint:

```text
GET /api/errors/recent
```

- Added CLI command:

```text
opspilot errors recent --source middleware --service orders-api --limit 20
```

- Added aggregated event sources:
  - Kubernetes abnormal Pods;
  - Argo CD Applications that are not `Synced/Healthy`;
  - release status gaps and unhealthy release evidence;
  - JSON/JSONL event files from `OPSPILOT_ERROR_EVENT_DIR`.

- Added default file event directory:

```text
/var/lib/opspilot/error-events
```

- Added structured event fields for AI use:
  - `source`
  - `stage`
  - `service`
  - `namespace`
  - `resource`
  - `severity`
  - `message`
  - `evidence`
  - `probable_cause`
  - `next_checks`

- Bumped CLI/Core version to `0.1.16-ai-readable-error-evidence`.

## Validation

Ran:

```text
go test ./...
```
