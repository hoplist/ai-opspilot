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

## Current Inventory And Target Paths

Snapshot date: 2026-05-29.

| Current path | Type | Target path | Action |
| --- | --- | --- | --- |
| `platform/opspilot` | `[PLATFORM]` | `tpo/platform/opspilot/opspilot-core` | Keep as current source until CI, image paths, release mapping, and GitOps are updated. |
| `platform/opspilot-skills` | `[SKILL]` | `tpo/platform/opspilot/opspilot-skills` | Move after OpsPilot `OPSPILOT_SKILLS_GIT_URL` is updated and verified. |
| `platform/gitops-manifests` | `[DEPLOY]` | `tpo/deploy/gitops-manifests` | Move only after Argo CD Application source URLs and OpsPilot release mappings are updated. |
| `tpo/devex/opspilot/cluster-etcd-backups` | `[BACKUP]` | `tpo/ops/backups/node200-etcd-snapshots` | Move after backup job remote URL is updated and a fresh snapshot push is verified. |
| `tpo/devex/opspilot/opspilot-core` | `[LEGACY]` | `tpo/platform/opspilot/opspilot-core` | Compare with `platform/opspilot`; archive when confirmed duplicate. |
| `tpo/devex/demo/demo-api` | `[SANDBOX]` | `tpo/sandbox/devex/demo-api` | Move when demo CI/GitOps references are no longer needed. |
| `platform/devex/demo/ai-loop-demo` | `[SANDBOX]` | `tpo/sandbox/devex/ai-loop-demo` | Move after demo release mapping is updated or remove if no longer needed. |
| `platform/devex/frontend-vite-demo` | `[SANDBOX]` | `tpo/sandbox/devex/frontend-vite-demo` | Move or archive after validation window. |
| `platform/devex/java-spring-demo` | `[SANDBOX]` | `tpo/sandbox/devex/java-spring-demo` | Move or archive after validation window. |
| `platform/devex/python-fastapi-demo` | `[SANDBOX]` | `tpo/sandbox/devex/python-fastapi-demo` | Move or archive after validation window. |
| `platform/devex/demo/resource-guardrail-demo` | `[SANDBOX]` | `tpo/sandbox/devex/resource-guardrail-demo` | Move or archive after validation window. |
| `root/test-cluster-backup` | `[BACKUP]` | `tpo/ops/backups/test-cluster-backup` | Move if still used; archive if superseded by node200 etcd backups. |
| `root/yaml` | `[OPS]` | `tpo/ops/yaml` or `tpo/deploy/manual-yaml` | Inspect contents before moving; do not mix with GitOps desired state. |
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
- No repository paths have been moved yet.
- CI, Registry, GitOps, Argo CD, backup jobs, and OpsPilot release mappings
  still use the existing project paths.

1. **Metadata first**
   Add descriptions and topics so GitLab's project list is readable without
   moving any repository.

2. **Create target groups**
   Create `tpo/apps`, `tpo/platform`, `tpo/deploy`, `tpo/shared`, `tpo/ops`, and
   `tpo/sandbox` as needed. This is non-breaking.

3. **Move sandbox and backups**
   Move demo and backup repositories first. These have lower release-chain
   blast radius than GitOps and OpsPilot core.

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
