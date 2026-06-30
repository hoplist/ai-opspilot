# GitLab Repository Governance

## Goal

Make the node206 GitLab project list understandable at first glance. A user
should be able to tell whether a repository is application code, GitOps deploy
state, platform code, shared templates, runtime skills, backups, or sandbox
work without opening the repository.

## Target Group Model

Use `tpo` as the single root group. Repositories should be grouped by purpose
first, then by owner or product:

```text
tpo/
  apps/
    devex/<project>/<service>
    office/<project>/<service>
    collab/<project>/<service>

  platform/
    opspilot/opspilot-core
    opspilot/opspilot-agent
    opspilot/opspilot-skills

  deploy/
    gitops-manifests

  shared/
    ci-templates
    dockerfile-templates
    service-templates

  ops/
    backups/node200-etcd-snapshots
    yaml

  sandbox/
    devex/demo-api
    devex/python-fastapi-demo
```

## Repository Type Prefixes

Every project description should start with one of these prefixes:

| Prefix | Meaning | Normal owner |
| --- | --- | --- |
| `[APP]` | Business application source code | Development team |
| `[DEPLOY]` | GitOps desired state consumed by Argo CD | Platform/Ops |
| `[PLATFORM]` | OpsPilot or internal platform service source code | Platform |
| `[SKILL]` | OpsPilot server-side runtime skills | Platform/Ops |
| `[SHARED]` | Shared CI, Dockerfile, or service templates | Platform |
| `[BACKUP]` | Automated backup snapshots or restore assets | Ops |
| `[OPS]` | Operational helper assets that are not GitOps source | Ops |
| `[SANDBOX]` | Demo, validation, or disposable test code | Platform/Ops |
| `[LEGACY]` | Old path kept for compatibility until migration | Owner of target |
| `[ARCHIVE]` | Deletion scheduled or historical project | Admin/Ops |

Descriptions are intentionally part of the governance model because GitLab's
project list shows them before a user opens the repository.

## Source Of Truth Rules

- Application repositories under `tpo/apps/...` own source code, tests, a thin
  `.gitlab-ci.yml`, and generated `deploy/k8s` starter manifests.
- `tpo/deploy/gitops-manifests` owns the cluster desired state consumed by
  Argo CD. It is not application source code.
- `tpo/platform/opspilot/opspilot-skills` owns runtime skills consumed by
  OpsPilot through git-sync.
- `tpo/shared/...` repositories are included by other repositories. They should
  not deploy workloads by themselves.
- `tpo/ops/backups/...` repositories are machine-written backup stores. They
  should not be used as application or deploy inputs.
- `tpo/sandbox/...` repositories are disposable validation assets. They should
  not be used as production examples without being promoted.

## Preflight Governance Checks

`opspilot repo preflight` now emits repository governance checks before a repo
enters the standard release path:

| Check | Purpose | Blocking behavior |
| --- | --- | --- |
| `repo_class` | Classifies the GitLab path as app, platform, deploy, shared, ops, sandbox, legacy, or unknown. | Legacy/unknown paths warn only during migration. |
| `business_repo_boundary` | Detects whether an application repository contains starter deploy manifests. | Warn only; current onboarding still generates starter manifests. |
| `immutable_image_tag` | Detects mutable image tags in deployment manifests. | `:latest` blocks app/sandbox repos; platform/deploy/shared/ops repos warn first. |

This gives OpsPilot a Google-style source-boundary check without requiring an
immediate monorepo or GitLab group migration.

## Identity-less Test Upload Default

Before user identity, team ownership, and permissions are fully wired, OpsPilot
can provide a read-only upload plan for test-only repositories:

```powershell
opspilot repo upload-plan --repo . --name my-demo-api
opspilot repo upload --repo . --name my-demo-api --confirm
```

Default target:

```text
GitLab project: tpo/sandbox/devex/<repo>
Namespace: sandbox
GitOps path: clusters/test/apps/sandbox/<repo>
Release scope: test-only
```

`repo upload-plan` is a planning contract only. `repo upload --confirm` can
create or reuse the sandbox GitLab project and push the current committed
`HEAD`, but it still does not update GitOps, change Kubernetes, configure
gateway routes, or promote the repository to an application-owned path. The
target is intentionally disposable and should be promoted to `tpo/apps/...` or
the agreed application layout when identity and ownership are added.

## Current Inventory And Target Paths

Snapshot date: 2026-05-29.

| Current path | Type | Target path | Action |
| --- | --- | --- | --- |
| `platform/opspilot` | `[PLATFORM]` | `tpo/platform/opspilot/opspilot-core` | Keep as current source until CI, image paths, release mapping, and GitOps are updated. |
| `platform/opspilot-skills` | `[SKILL]` | `tpo/platform/opspilot/opspilot-skills` | Move after OpsPilot `OPSPILOT_SKILLS_GIT_URL` is updated and verified. |
| `platform/gitops-manifests` | `[DEPLOY]` | `tpo/deploy/gitops-manifests` | Move only after Argo CD Application source URLs and OpsPilot release mappings are updated. |
| `tpo/ops/backups/node200-etcd-snapshots` | `[BACKUP]` | current | Moved from `tpo/devex/opspilot/cluster-etcd-backups` in phase 2; node200 backup remote updated and verified. |
| `tpo/devex/opspilot/opspilot-core` | `[SHARED]` | `tpo/shared/ci-templates` | This is a GitLab CI include source, not the live OpsPilot core. Move after generated service `.gitlab-ci.yml` includes are updated. |
| `tpo/sandbox/devex/demo-api` | `[SANDBOX]` | current | Moved after registry cleanup; source retained, unused GitOps deployment residue removed. |
| `tpo/sandbox/devex/ai-loop-demo` | `[SANDBOX]` | current | Moved after registry cleanup; rebuilt through standard demo pipeline. |
| `tpo/sandbox/devex/frontend-vite-demo` | `[SANDBOX]` | current | Moved after registry cleanup; rebuilt through standard demo pipeline. |
| `tpo/sandbox/devex/java-spring-demo` | `[SANDBOX]` | current | Moved after registry cleanup; rebuilt through standard demo pipeline. |
| `tpo/sandbox/devex/python-fastapi-demo` | `[SANDBOX]` | current | Moved after registry cleanup; rebuilt through standard demo pipeline. |
| `tpo/sandbox/devex/resource-guardrail-demo` | `[SANDBOX]` | current | Moved after registry cleanup; rebuilt through standard demo pipeline. |
| `tpo/ops/backups/test-cluster-backup` | `[BACKUP]` | current | Moved from `root/test-cluster-backup` in phase 2; keep for audit and later retirement review. |
| `tpo/ops/yaml` | `[OPS]` | current | Moved from `root/yaml` in phase 2; currently empty manual YAML holding area, not GitOps desired state. |
| `platform/demo-api-deletion_scheduled-6` | `[ARCHIVE]` | none | Leave scheduled for deletion; do not use. |
| `platform/password-deletion_scheduled-4` | `[ARCHIVE]` | none | Leave scheduled for deletion; do not use. |

## Migration Phases

All GitLab mutations require explicit confirmation before execution. This
includes group creation, project move/transfer, project description/topic
updates, archive/delete actions, permission changes, deploy-token changes,
CI/CD variable changes, and repository URL changes used by Argo CD, GitOps,
backup jobs, or git-sync.

Current status on 2026-05-29:

- Target groups under `tpo` have been created.
- Existing project descriptions have been updated with visible type prefixes.
- Backup and ops holding repositories have been moved under `tpo/ops`.
- Sandbox/demo repositories have been moved under `tpo/sandbox/devex` after
  backing up Git repositories and runnable registry images.
- CI, Registry, GitOps, Argo CD, backup jobs, and OpsPilot release mappings
  still use the existing paths for sandbox/demo, platform, skills, and GitOps
  repositories.

1. **Metadata first**
   Add descriptions and topics so GitLab's project list is readable without
   moving any repository.

2. **Create target groups**
   Create `tpo/apps`, `tpo/platform`, `tpo/deploy`, `tpo/shared`, `tpo/ops`, and
   `tpo/sandbox` as needed. This is non-breaking.

3. **Move sandbox and backups**
   Move demo and backup repositories first. Backup and ops holding repositories
   were moved in phase 2. Demo repositories were not moved because GitLab blocks
   project transfer when Container Registry tags still exist.

3a. **Demo registry migration**
   Completed on 2026-05-29. Runnable old images were archived on node206 before
   tag deletion. BuildKit `buildcache` tags could not be archived with Docker
   because they use BuildKit cache media types; they were regenerated by the
   follow-up demo builds.

4. **Move runtime skills**
   Move `platform/opspilot-skills`, update `OPSPILOT_SKILLS_GIT_URL`, let
   git-sync pull from the new URL, and verify `skills registry` reports
   `source=dynamic+embedded`.

5. **Move GitOps**
   Move `platform/gitops-manifests`, update Argo CD Application source URLs,
   update OpsPilot release mappings, and verify a no-op sync.

6. **Move OpsPilot core**
   Move `platform/opspilot`, update remote URL, GitLab CI variables, image
   registry paths, GitOps image references, and release mappings. Run the
   standard release flow after migration.

7. **Archive legacy duplicates**
   Archive duplicate or deletion-scheduled repositories after the new paths are
   verified.

## Move Checklist

Before executing any checklist item against GitLab, present the exact target
project, API action, old value, new value, and rollback path for confirmation.

Before moving a repository, check:

- GitLab CI project path and variables.
- Container Registry image path.
- GitOps repository file references.
- Argo CD `repoURL` and Application path.
- OpsPilot `OPSPILOT_RELEASE_SERVICES`.
- Any CronJob, backup script, deploy token, or git-sync URL.
- Local working copies and documented clone URLs.

## Immediate Metadata Standard

Descriptions should be short and operational:

```text
[PLATFORM] OpsPilot core source. Released by node206 GitLab Runner -> BuildKit -> GitOps -> node200 Argo CD.
[DEPLOY] Argo CD GitOps desired state for node200 test cluster. Not application source code.
[SKILL] OpsPilot server-side runtime skills consumed by opspilot-core through git-sync.
```

This gives users a first-screen signal before the full migration is complete.
