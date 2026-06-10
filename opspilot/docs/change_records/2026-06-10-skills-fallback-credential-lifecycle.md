# 2026-06-10 skills fallback and credential lifecycle commands

## Goal

Complete the next architecture hardening step:

- bundle a minimal OpsPilot runtime skills set inside the OpsPilot image;
- make GitLab skills sync failure degrade to the image-bundled skills;
- expand credential ledger commands from catalog and plan to access, revoke,
  and rotate plan flows.

## Scope

Included:

- Copy `opspilot/skills-repo/skills` into the OpsPilot image as the minimal
  fallback skills directory.
- Add `OPSPILOT_SKILLS_FALLBACK_DIR` so runtime fallback is configurable.
- Keep runtime preference order:
  1. GitLab-synced skills directory;
  2. image-bundled fallback skills directory;
  3. embedded metadata fallback.
- Add credential lifecycle planning commands:
  - `opspilot credentials access`
  - `opspilot credentials revoke`
  - `opspilot credentials rotate`
- Keep access, revoke, and rotate plan-first. They do not create, reveal,
  revoke, or rotate real secret values in this phase.

Excluded:

- No real database account creation.
- No real secret rotation.
- No real credential revocation.
- No external secret manager integration.
- No new cluster or log datasource onboarding.

## Safety Boundary

Credential lifecycle commands return:

- target scope;
- risk level;
- automation policy;
- required evidence;
- execution steps;
- validation commands;
- warnings.

They must not print passwords, tokens, kubeconfigs, or GitLab tokens. Real
mutation remains a later controlled OpsPilot action path.

## Validation Plan

- `go test ./...`
- `go vet ./...`
- local CLI demo:
  - `credentials access --service demo-api --kind mysql --mode readonly --ttl 2h`
  - `credentials revoke --name debug-demo-api-mysql`
  - `credentials rotate --name demo-api-mysql-credentials`
  - fallback skills validation with a missing GitLab skills directory
- standard release:
  GitLab Runner -> BuildKit -> Registry -> GitOps -> Argo CD -> rollout
  verification.
