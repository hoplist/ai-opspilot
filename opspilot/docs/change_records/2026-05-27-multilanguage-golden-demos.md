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

## 2026-06-03 Demo-Test Retest

Retested four demo services in the shared `demo-test` namespace through the
standard GitLab Runner -> BuildKit -> GitLab Registry -> GitOps -> Argo CD path:

- `python-fastapi-demo`, pipeline `81`, image tag `267ab00f`.
- `java-spring-demo`, pipeline `80`, image tag `2d07da41`.
- `resource-guardrail-demo`, pipeline `79`, image tag `abb9d20e`.
- `frontend-vite-demo`, pipeline `82`, image tag `fac74e0d`.

Findings:

- Argo initially blocked all four apps because AppProject `cicd-apps` allowed
  `cicd-*` and `opspilot`, but not `demo-test`. GitOps commit `349d0c4` added
  `demo-test` as an allowed destination.
- The services then reached deployment, but all Pods hit `ErrImagePull` /
  `ImagePullBackOff` with GitLab Registry `403 Forbidden`. Root cause: the
  shared `demo-test` namespace had no `gitlab-registry-pull` secret bound to
  the default ServiceAccount.
- For the retest only, the existing read-only Registry pull secret was copied
  into `demo-test` and the default ServiceAccount was configured with
  `imagePullSecrets: gitlab-registry-pull`. After Pod recreation, all four Pods
  became `Running` and `Ready` with zero restarts.
- No Redis, MySQL, MinIO/S3, Kafka, or other middleware Pods were created in
  `demo-test`. This is expected for the current phase: middleware detection
  records intent/signals only; it does not automatically provision middleware.
- Interface smoke tests through temporary port-forward succeeded:
  - Python `/health` and `/users`: HTTP 200.
  - Java `/health` and `/orders`: HTTP 200.
  - Go guardrail `/health` and `/records`: HTTP 200.
  - Frontend `/`: HTTP 200.
- OpsPilot `inspect pod` correctly reported the `demo-test` Pods as Ready with
  Kubernetes logs available and ELK unavailable. `inspect service` still used
  older release mappings for some demos, and `resource-guardrail-demo` had no
  service mapping. The release mapping generator/config sync needs to be
  updated so shared namespace demo releases are visible by service name.

Follow-up platform fixes:

- Namespace bootstrap should handle GitLab Registry pull access for shared
  namespaces, or the workload generator should attach `imagePullSecrets` when
  using the GitLab Registry.
- Release service mappings must be regenerated from the current GitLab/GitOps
  path after repository governance migration.
- Middleware provisioning remains out of scope for this test; future support
  should create shared middleware instances or databases only when the platform
  policy explicitly allows it.

## 2026-06-03 Cleanup

After user confirmation, removed the temporary `demo-test` validation resources:

- Removed four `demo-test-*` Argo Application manifests and
  `clusters/test/apps/demo-test/*` from the GitOps repository.
- Pushed GitOps cleanup commit `39c804d`.
- Deleted the remaining Argo Application CRs and the `demo-test` namespace
  because bootstrap uses `prune: false` and the temporary Applications had no
  finalizers for resource cascade deletion.

Follow-up defects were addressed in OpsPilot source:

- onboarding now generates ServiceAccount and `gitlab-registry-pull`
  references for GitLab Registry image pulls;
- onboarding now auto-generates lightweight Redis/MySQL/PostgreSQL/MinIO
  middleware for detected common dependencies;
- `inspect service` / release status can fall back to Kubernetes Deployment
  lookup when `OPSPILOT_RELEASE_SERVICES` has not yet caught up, and reports
  `release_mapping_missing` instead of failing with only
  `unknown release service`.
