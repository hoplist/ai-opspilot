# 2026-06-10 architecture hardening and thin CLI

## Goal

Continue the OpsPilot architecture review follow-up without connecting new
remote clusters or new log sources in this phase.

This phase focuses on:

- keeping the CLI as a thinner client;
- making server-side skills state clearer, including fallback status;
- improving credential and datasource planning commands for non-operators;
- preserving the current node200 and node206 runtime model.

## Scope

Included:

- Move catalog, credential, datasource, and cluster command handling out of
  `cli/main.go`.
- Keep credential operations plan-first and metadata-only.
- Add beginner-oriented aliases for common credential plans such as app
  database access, temporary debug access, datasource registration, and cluster
  registration.
- Expose skills runtime source and fallback evidence clearly in validation and
  registry output.
- Validate with Go tests, CLI demo commands, and the standard release flow.

Excluded for this phase:

- No new external cluster kubeconfig registration.
- No ELK/APISIX/service-log datasource onboarding.
- No high-risk credential creation, rotation, deletion, or namespace deletion.
- No client-side skills registry.

## Architecture Decision

OpsPilot remains the backend owner of skills, credential catalogs, cluster
catalogs, and policy. The CLI should only request plans, evidence, or approved
actions from the backend.

Credential-related commands stay plan-first because the platform still needs a
clear audit trail before creating or rotating real secrets. The command output
should explain scope, required keys, GitOps paths, validation commands, and
whether the action is read-only or controlled mutation.

## Validation Plan

- `go test ./...`
- `go vet ./...`
- `opspilot credentials plan app-db --service demo-api --kind mysql --output human`
- `opspilot credentials plan debug-access --service demo-api --kind mysql --ttl 2h --output human`
- `opspilot datasources plan prometheus --cluster node200-test --output human`
- `opspilot skills validate --output human`
- Standard release:
  GitLab Runner -> BuildKit -> Registry -> GitOps -> Argo CD -> rollout
  verification.
