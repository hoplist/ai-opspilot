# 2026-06-30 Google-Style Repository Governance Alignment

## Goal

Align OpsPilot repository governance with large-company source-of-truth
practices without introducing a heavyweight monorepo or restructuring GitLab
immediately.

The immediate objective is to make `repo preflight` detect repository class,
business repository boundaries, and mutable deployment image tags before a
repository enters the automated release path.

## Decisions

- Keep GitLab as the human-maintained desired-state source for repositories,
  OpsPilot config, skills, CI templates, and GitOps manifests.
- Do not require a Google-style monorepo. OpsPilot keeps multiple repositories
  but gives each repository a clear class and boundary.
- Recommended repository classes:
  - `tpo/apps/...` for business application source.
  - `tpo/platform/...` for OpsPilot/platform code.
  - `tpo/deploy/...` for GitOps desired state.
  - `tpo/shared/...` for CI, Dockerfile, and service templates.
  - `tpo/ops/...` for operational assets and backups.
  - `tpo/sandbox/...` for disposable test/demo repositories.
- Legacy paths such as `tpo/devex/...` remain tolerated during migration and
  only produce warnings.
- Business repositories may temporarily contain generated `deploy/k8s` starter
  manifests, but long-term live deployment state belongs in GitOps/config
  repositories.
- Application deployment manifests must not use mutable `:latest` image tags.
  Standard release should write commit tags or digests.

## Implemented

- Added repository governance checks to `repo preflight`:
  - `repo_class`
  - `business_repo_boundary`
  - `immutable_image_tag`
- Added compatibility for the target platform path
  `tpo/platform/opspilot/opspilot-core` when direct BuildKit CI is used.
- Added tests for:
  - recommended `tpo/apps/...` app path.
  - legacy app path warning.
  - app deployment manifest using `:latest`.
- Updated CLI schema description for `repo preflight`.
- Added ADR for GitLab repository classes and source boundaries.

## Not Changed

- No GitLab group or project was moved in this code release.
- No GitLab permission, deploy token, CI variable, Argo CD source URL, or
  registry path was changed.
- No live cluster resource was modified.
- `deploy/k8s` in business repositories is not removed yet because current
  onboarding still uses it as a beginner-friendly generated starter.

## GitLab Landing Scope

The user approved direct GitLab changes after the design review. The safe
landing order is:

1. publish the preflight guardrails through the standard node206 GitLab Runner
   -> BuildKit -> Registry -> GitOps -> Argo CD flow;
2. refresh GitLab metadata and descriptions where needed;
3. run `repo preflight` against candidate repositories;
4. only then migrate high-impact paths such as `platform/opspilot`,
   `platform/gitops-manifests`, and `platform/opspilot-skills`.

Core path migration remains a separate operation because current runtime config
still references `platform/opspilot` and `platform/gitops-manifests`.

## GitLab Metadata Landing

Applied after explicit approval on 2026-06-30.

Updated low-risk project descriptions only:

| Project | Description prefix |
| --- | --- |
| `platform/opspilot-config` | `[PLATFORM]` |
| `tpo/sandbox/devex/fullstack-vue-web` | `[SANDBOX]` |
| `tpo/sandbox/devex/fullstack-go-api` | `[SANDBOX]` |

No GitLab project was transferred, archived, deleted, or permission-modified.
No deploy token, CI/CD variable, registry setting, GitOps URL, or Argo CD source
URL was changed.

Current GitLab governance scan:

| Project class | Projects | Result |
| --- | ---: | --- |
| `ok` | 11 | Description prefix present and path is already governed enough for current phase. |
| `high-impact-path-migration-deferred` | 4 | `platform/opspilot`, `platform/opspilot-config`, `platform/opspilot-skills`, `platform/gitops-manifests`. These remain deferred because runtime config, GitOps, Argo CD, CI, and registry paths still reference them. |
| `needs-classification` | 1 | `tpo/devex/opspilot/opspilot-core`, currently documented as `[SHARED]` CI template include source. |

Sandbox repository preflight scan:

| Repository | Result | Main gaps |
| --- | --- | --- |
| `tpo/sandbox/devex/fullstack-go-api` | ready | none |
| `tpo/sandbox/devex/fullstack-vue-web` | ready | none |
| `tpo/sandbox/devex/ai-loop-demo` | not ready | deployment namespace does not match inferred namespace `cicd-sandbox-devex` |
| `tpo/sandbox/devex/frontend-vite-demo` | not ready | `serviceaccount`, `deployment` |
| `tpo/sandbox/devex/java-spring-demo` | not ready | `serviceaccount`, `deployment` |
| `tpo/sandbox/devex/python-fastapi-demo` | not ready | `serviceaccount`, `deployment` |
| `tpo/sandbox/devex/resource-guardrail-demo` | not ready | `serviceaccount`, `deployment` |
| `tpo/sandbox/devex/demo-api` | not ready | `limitrange`, `resourcequota`, `serviceaccount`, `deployment` |

The preflight findings were not auto-fixed in this step because these demo
repositories already participate in release/GitOps history. They should be
fixed one repository at a time through the standard release flow.

## Sandbox Demo Preflight Landing

Applied after explicit approval on 2026-06-30.

### `tpo/sandbox/devex/ai-loop-demo`

Goal: make the first legacy sandbox demo pass the new repository governance
preflight without changing its existing runtime namespace.

GitLab commits:

| Commit | Purpose |
| --- | --- |
| `7a27e55` | Added `opspilot.namespaces.yaml` so `tpo/sandbox/devex/ai-loop-demo` maps to existing namespace `cicd-devex-demo`. |
| `6e41dcf` | Updated Deployment to use the existing `ai-loop-demo` ServiceAccount. |
| `9a1a3ed` | Added `serviceaccount.yaml` to `deploy/k8s/kustomization.yaml` so Argo CD actually applies it. |

Validation:

- `repo preflight --project tpo/sandbox/devex/ai-loop-demo` is ready.
- `go test ./...` in the demo repository passed.
- GitLab pipelines `185`, `186`, and `187` reached success while iterating the
  fix.
- Argo CD Application `devex-demo-ai-loop-demo` is `Synced` and `Healthy`.
- Deployment `cicd-devex-demo/ai-loop-demo` rolled out successfully.
- Current Pod is `Running`.

Operational fix discovered during rollout:

- `cicd-devex-demo` lacked a usable `gitlab-registry-pull` secret for the new
  sandbox registry path.
- A project-level read-only deploy token
  `opspilot-ai-loop-demo-read-registry-20260630` was created for
  `tpo/sandbox/devex/ai-loop-demo` with `read_registry`, expiring on
  2027-06-30.
- `gitlab-registry-pull` was created/updated in `cicd-devex-demo` without
  printing the token value.

Follow-up:

- Generalize the registry pull secret step into a platform-side namespace
  bootstrap or documented per-demo onboarding step.
- Do not bulk reactivate inactive demos. `frontend-vite-demo`,
  `java-spring-demo`, `python-fastapi-demo`, and `resource-guardrail-demo`
  currently have no active Argo Application or running Deployment, so they
  should be fixed only when intentionally re-enabled.

## Preflight Hardening After Demo Rollout

Applied on 2026-06-30 after the `ai-loop-demo` rollout exposed a real blind
spot.

Root cause:

- The repository contained `deploy/k8s/serviceaccount.yaml`.
- `repo preflight` only checked that the file existed.
- `deploy/k8s/kustomization.yaml` did not reference `serviceaccount.yaml`, so
  Argo CD never applied it.
- The first rollout therefore still failed even though the repository looked
  structurally complete.

Change:

- Added `kustomization_references` to `repo preflight`.
- The rule verifies that critical existing manifests are listed under
  `resources:` in `deploy/k8s/kustomization.yaml`.
- Missing references are blockers because the standard GitOps path will not
  apply unreferenced files.
- The rule also accepts valid Kustomize directory resources such as `../rbac`
  when that directory's own `kustomization.yaml` owns the referenced manifests.

Current checked references:

- `deployment.yaml`
- `service.yaml`
- `namespace.yaml` when present
- `limitrange.yaml` when present
- `resourcequota.yaml` when present
- `serviceaccount.yaml` when present
- `configmap.yaml` when config sources exist or the file is present

Registry pull secret boundary:

- The `ai-loop-demo` fix used a project-level read-only `read_registry` deploy
  token and updated the namespace pull secret without printing the token value.
- This is recorded as an operational workaround, not a permanent platform
  contract.
- Longer term, registry pull secret handling should be documented as a
  namespace bootstrap step or moved behind a governed platform operation.

## GitLab Repository Adjustment Landing

Applied on 2026-06-30 after approving the seven-step GitLab repository
governance adjustment.

Completed actions:

1. **Inventory current GitLab repositories**
   - Pulled the node206 GitLab project list through the GitLab API.
   - Confirmed sandbox/demo repositories are already under
     `tpo/sandbox/devex`.
   - Confirmed backup and ops holding repositories are already under
     `tpo/ops`.

2. **Mark migration status**
   - Kept `platform/opspilot` as the live OpsPilot core source because CI,
     registry image paths, release mapping, and GitOps still point there.
   - Kept `platform/gitops-manifests` as the live Argo CD source because all
     current Argo CD Applications still use this repo URL.
   - Marked runtime config and runtime skills as safe to move after checking
     that neither project has Container Registry repositories.

3. **Update descriptions and README files**
   - Updated GitLab descriptions for:
     - `platform/opspilot`
     - `platform/gitops-manifests`
     - `tpo/platform/opspilot/opspilot-config`
     - `tpo/platform/opspilot/opspilot-skills`
   - Updated README files in the config and skills repositories so users can
     identify their purpose without opening platform docs.

4. **Migrate low-risk repositories**
   - Moved `platform/opspilot-config` to
     `tpo/platform/opspilot/opspilot-config`.
   - Moved `platform/opspilot-skills` to
     `tpo/platform/opspilot/opspilot-skills`.
   - Existing project deploy tokens remain project-scoped and were not printed
     or rotated in this step.

5. **Modify OpsPilot config references**
   - Updated GitOps desired state:
     - `OPSPILOT_CONFIG_GIT_URL`
     - `OPSPILOT_SKILLS_GIT_URL`
   - Updated the offline kit copy of the same config so future installs use
     the governed path.
   - Updated current docs that point operators to the runtime config and skills
     repositories.

6. **Run preflight and standard validation**
   - `repo preflight` is expected to include the new
     `kustomization_references` guard.
   - Standard validation for this landing must include:
     - `go test ./...`
     - `go vet ./...`
     - `go run ./cli --output human config validate --dir ./config/opspilot-config`
     - `git diff --check`
     - standard node206 GitLab Runner -> BuildKit -> Registry -> GitOps ->
       Argo CD release status check.

7. **Core/GitOps migration boundary**
   - `platform/opspilot` was intentionally not moved in this stage.
   - Moving the core source repository still requires CI, registry, release
     mapping, and GitOps review.
   - GitOps repository migration was executed in the follow-up stage below.

Risk boundary:

- No GitLab permission model, branch protection, deploy token, CI variable, or
  project deletion setting was changed.
- No Kubernetes Secret value was printed.
- The only runtime-impacting change is the GitOps URL update for config and
  skills git-sync. Rollback is to restore the two URLs to
  `platform/opspilot-config.git` and `platform/opspilot-skills.git`, then hard
  refresh the `opspilot-core` Argo CD Application.

## GitOps Repository Migration Landing

Applied on 2026-06-30 after the runtime config and skills migration was
verified.

Completed actions:

- Moved `platform/gitops-manifests` to `tpo/deploy/gitops-manifests`.
- Updated OpsPilot source and shared CI templates:
  - `.gitlab-ci.yml`
  - `ci/templates/buildkit-gitops.*.yml`
  - embedded Application manifests generated by release jobs.
- Updated OpsPilot config:
  - `settings.gitops_project`
  - `clusters.node200.gitops_project`
  - credential ledger scope for `GITOPS_TOKEN`
- Updated offline-kit Argo CD app manifests to use the governed GitOps URL.
- Updated the live GitOps repository through the GitLab Commit API:
  - Application `repoURL` values.
  - AppProject `sourceRepos`.
  - CI/GitOps docs inside the GitOps repository.
- Updated the Argo CD repository credential:
  - created `gitlab-gitops-manifests-tpo-deploy` for the new URL.
  - retained the old URL credential only during convergence.
  - removed the old `gitlab-gitops-manifests` Secret after all Applications and
    AppProjects reported the new URL.
- Updated `obsidian-sync` live Application and AppProject because it did not
  fully converge from app-of-apps during the migration window, while the GitOps
  desired file already had the new URL.

Validation:

- All Argo CD Applications point to
  `http://192.168.48.206:8929/tpo/deploy/gitops-manifests.git`.
- All Argo CD AppProjects that restrict source repos allow the new URL.
- All Applications are `Synced` and `Healthy`.

Rollback:

- Restore Application/AppProject repo URLs to
  `http://192.168.48.206:8929/platform/gitops-manifests.git`.
- Recreate or restore the old Argo CD repository Secret URL.
- Transfer the GitLab project back only if redirect compatibility is not enough.
- Revert `.gitlab-ci.yml`, `ci/templates/*`, OpsPilot config, and offline-kit
  URL changes.

## Risk Boundary

- Path governance warnings do not block current test workflows.
- `:latest` is a blocker for application/sandbox repositories because it makes
  rollback and evidence correlation ambiguous.
- `:latest` is warning-only for platform/deploy/shared/ops repositories until
  their migration path is explicitly reviewed.

## Minimum Validation

- `go test ./cli ./internal/assets ./internal/configloader`
- `go test ./...`
- `go vet ./...`
- `go run ./cli --output human config validate --dir ./config/opspilot-config`
- `git diff --check`

## Rollback

- Revert the changes in `cli/repo_preflight.go`, `cli/repo_test.go`,
  `contracts/cli-schema.json`, and the newly added governance docs.
- No external state needs rollback because this stage does not mutate GitLab,
  registry, GitOps, Argo CD, or Kubernetes.
