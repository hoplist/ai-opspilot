# 2026-06-04 Governance implementation loop

## Goal

Continue the platform governance roadmap after backend intent parsing. The goal
is to keep OpsPilot as the backend-owned operations entrypoint while making the
developer-facing flow easier to understand and safer to automate.

## Implemented

- Added backend skill recommendation routing:
  - `GET /api/skills/recommend`
  - recommendations are generated from the active backend skills registry;
  - CLI `inspect` and `fix --dry-run` now request recommendations from the
    backend instead of owning routing locally.
- Added plan-first registration APIs:
  - `GET /api/credentials/plan`
  - `GET /api/datasources/plan`
  - these return required keys, steps, validation commands, risk, and
    automation mode without creating Secrets or exposing secret values.
- Added CLI wrappers:
  - `opspilot credentials plan --kind mysql --service <service>`
  - `opspilot datasources plan --kind prometheus --name <source>`
- Added structured GitOps onboarding output:
  - generated GitOps path;
  - Deployment path;
  - Argo CD Application name;
  - target image;
  - standard release flow;
  - middleware credential plan summaries.
- Configured catalog metadata in the OpsPilot core ConfigMap:
  - `OPSPILOT_CREDENTIAL_CATALOG`
  - `OPSPILOT_CLUSTER_CATALOG`

## Safety Boundary

- Catalogs and plans are metadata only.
- Secret values, tokens, passwords, and kubeconfigs are not stored in Git.
- Credential and datasource registration remains `plan_first`.
- Missing observability datasources must not block Pod-first troubleshooting.

## Demo Scope

The validation demo should create a disposable service under a demo namespace,
run `repo autofix/onboard`, inspect the generated GitOps plan, push through the
standard GitLab/BuildKit/GitOps/Argo CD flow, then verify:

- release status;
- service runtime status;
- backend natural-language inspect;
- backend skill recommendations;
- credential and cluster catalogs;
- credential and datasource plans.
