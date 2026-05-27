# 2026-05-27 Multilanguage Golden Demos

## Goal

Validate that OpsPilot onboarding, release, and AI-readable evidence work for
common non-Go services through the same standard release path.

## Scope

- Python backend demo, using a small FastAPI service.
- Frontend demo, using a small Vite static site served by nginx.
- Java backend demo, using a small Spring Boot service.
- Node.js is intentionally out of scope for this pass.

## Platform Changes

- Added `frontend` detection for common Vite/React/Vue/Angular package markers
  while leaving generic Node.js projects mapped to `node`.
- Added `java` detection for Maven/Gradle project files.
- Added generated Dockerfile support for frontend static builds and Java Maven
  jar builds.
- Added shared CI templates:
  - `ci/templates/buildkit-gitops.frontend.yml`
  - `ci/templates/buildkit-gitops.java.yml`

## Validation Plan

Each demo must use the standard path:

```text
node206 GitLab
-> node206 GitLab Runner
-> BuildKit rootless image build
-> GitLab Registry
-> GitOps repository update
-> node200 Argo CD automatic deployment
-> OpsPilot check/release/fix evidence validation
```

Expected checks:

- `repo preflight` or `onboard repo` detects language, namespace, resources,
  probes, and CI template.
- GitLab pipeline reaches `success`.
- GitOps desired image matches the running Kubernetes image.
- Argo CD reports `Synced / Healthy`.
- OpsPilot `check service`, `check release`, and `fix service --dry-run
  --output evidence` return AI-readable results and explicit missing evidence.
