# 2026-05-27 Resource guardrails

## Decision

Do not force load testing, Service Mesh, or strict NetworkPolicy in the first
developer-friendly onboarding phase.

The required baseline is:

- every generated Deployment must include CPU and memory requests/limits;
- every generated Deployment must include readiness and liveness probes;
- every generated namespace must include LimitRange and ResourceQuota guardrails;
- repository preflight must block release onboarding when these guardrails are
  missing.

## Defaults

The default service resource profile is `small`:

```text
requests.cpu: 50m
requests.memory: 64Mi
limits.cpu: 500m
limits.memory: 256Mi
```

The default namespace guardrails are intentionally conservative for the test
environment:

```text
LimitRange default request: 50m / 64Mi
LimitRange default limit:   500m / 256Mi
ResourceQuota requests:     4 CPU / 8Gi memory
ResourceQuota limits:       8 CPU / 16Gi memory
ResourceQuota pods:         50
```

## Scope

This change only updates generated service manifests and repository readiness
checks. It does not apply live Kubernetes mutations directly. Deployment still
goes through the standard GitLab Runner -> BuildKit -> Registry -> GitOps ->
Argo CD flow.

OpsPilot Core's own Deployment manifest also gets the same small resource
profile so the platform follows the rule it enforces for generated services.
The shared GitLab BuildKit templates for Go, Node, and Python check the generated LimitRange,
ResourceQuota, Deployment resources, and probes during `preflight:onboarding`,
so broken repositories fail before image build and GitOps update.
Generated service `.gitlab-ci.yml` now includes templates from the actual
node206 platform repository `platform/opspilot`, which is the project currently
available to the GitLab token used for automated demo/release validation.
Ownership inference now also understands four-segment GitLab paths outside the
default `tpo` root, for example `platform/devex/demo/service` maps to
`group=devex`, `project=demo`, and `service=service`.
The demo pipeline caught a GitLab CI parsing edge case: shell script entries
that grep for YAML keys containing `:` must quote the whole command. The shared
templates now quote those guardrail grep commands so included pipelines lint
and create jobs correctly.

## Follow-up

Future optional phases can add:

- baseline NetworkPolicy generation with explicit opt-in;
- smoke tests after release;
- load tests only when requested by service owners.

## Validation

Ran:

```powershell
go test ./opspilot/...
.\opspilot\scripts\build-cli.ps1
```

Smoke checked generated output with a temporary Go service:

```powershell
opspilot repo autofix --project tpo/devex/demo/demo-api --write --output human
opspilot repo preflight --project tpo/devex/demo/demo-api --output human
```

The generated repository includes:

- `deploy/k8s/deployment.yaml` with CPU/memory requests and limits;
- `deploy/k8s/deployment.yaml` with readiness and liveness probes;
- `deploy/k8s/limitrange.yaml`;
- `deploy/k8s/resourcequota.yaml`.
