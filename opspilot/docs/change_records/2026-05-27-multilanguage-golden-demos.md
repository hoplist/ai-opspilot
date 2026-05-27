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

## Validation Notes

- Python, frontend, and Java demo repositories were created under
  `platform/devex/*` and released through GitLab Runner, BuildKit, GitLab
  Registry, GitOps, and Argo CD.
- The frontend demo initially used `vite --host 0.0.0.0` as its build command,
  which correctly exposed a hanging CI test. It was fixed to `vite build`.
- New demo namespaces initially hit `ImagePullBackOff` because node200 cannot
  anonymously pull private GitLab Registry images. Per-project `read_registry`
  deploy tokens and namespace `gitlab-registry-pull` secrets were added for the
  demo namespaces to complete validation.
- Release mappings for `ai-loop-demo`, `python-fastapi-demo`,
  `frontend-vite-demo`, and `java-spring-demo` are now stored in the source
  OpsPilot ConfigMap so future platform releases do not erase them.

## Final Results

- `python-fastapi-demo`
  - Pipeline `38`: success.
  - Argo: `Synced / Healthy`.
  - Runtime: Pod `Ready`, `/health` returned `{"status":"ok"}`.
  - OpsPilot: `check service`, `release status`, and `fix service --dry-run
    --output evidence` returned `healthy`.
- `frontend-vite-demo`
  - Pipeline `41`: success after fixing the Vite build command.
  - Argo: `Synced / Healthy`.
  - Runtime: Pod `Ready`, nginx served the generated Vite `index.html`.
  - OpsPilot: `check service`, `release status`, and `fix service --dry-run
    --output evidence` returned `healthy`.
- `java-spring-demo`
  - Pipeline `40`: success.
  - Argo: `Synced / Healthy`.
  - Runtime: Pod `Ready`, `/health` returned `{"status":"ok"}`.
  - OpsPilot: `check service`, `release status`, and `fix service --dry-run
    --output evidence` returned `healthy`.

Remaining expected gaps are evidence integrations, not release blockers:
`elk_logs_missing`, service log index missing, and APISIX gateway evidence
missing.
