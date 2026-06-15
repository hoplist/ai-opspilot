# 2026-06-15 Sandbox Upload Plan And Optional Test Gateway

## Goal

Support the early test environment where user identity is not available yet.
Unknown users should have a clear default place to put test code without
requiring platform ownership decisions up front.

Also document the future `*.test.tpo.xzoa.com` front-gateway option without
making it part of the current release blocker set.

## Changes

- Added `opspilot repo upload-plan`.
  - Default GitLab project: `tpo/sandbox/devex/<repo>`.
  - Default namespace: `sandbox`.
  - Default GitOps path: `clusters/test/apps/sandbox/<repo>`.
  - Release scope: `test-only`.
  - The command is plan-only and does not create GitLab projects, push code,
    edit GitOps, or mutate Kubernetes.
- Updated CLI schema so AI/skills can discover `repo upload-plan`.
- Updated developer and onboarding docs with the identity-less sandbox flow.
- Updated GitLab repository governance docs with the sandbox upload boundary.
- Documented the optional future test front gateway:
  `*.test.tpo.xzoa.com -> one test ingress/APISIX/Nginx entry`.

## Explicit Non-Changes

- No service metadata model change for the front gateway.
- No schema change for gateway fields.
- No APISIX/Nginx/Kubernetes gateway mutation.
- Missing test gateway configuration must not block:
  - `repo preflight`;
  - `repo autofix`;
  - `onboard generate`;
  - release.

## Validation Plan

- Run CLI unit tests for `repo upload-plan`.
- Run broader CLI tests.
- Run `go test ./...` and `go vet ./...`.
- For release, use the standard GitLab Runner -> BuildKit -> Registry ->
  GitOps -> Argo CD flow if this change is deployed.
