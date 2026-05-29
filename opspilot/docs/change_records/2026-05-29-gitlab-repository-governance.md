# 2026-05-29 GitLab repository governance

## Goal

Make node206 GitLab repositories distinguishable by purpose before users open
them. The current project list mixes application code, GitOps desired state,
OpsPilot platform code, runtime skills, backups, and sandbox demos.

## Changes

- Added `docs/gitlab-repository-governance.md`.
- Defined target groups under the `tpo` root:
  - `tpo/apps`
  - `tpo/platform`
  - `tpo/deploy`
  - `tpo/shared`
  - `tpo/ops`
  - `tpo/sandbox`
- Defined visible project description prefixes:
  - `[APP]`
  - `[DEPLOY]`
  - `[PLATFORM]`
  - `[SKILL]`
  - `[SHARED]`
  - `[BACKUP]`
  - `[OPS]`
  - `[SANDBOX]`
  - `[LEGACY]`
  - `[ARCHIVE]`
- Documented the current GitLab inventory and target paths.
- Documented a phased migration plan that avoids breaking CI, registry image
  paths, Argo CD source URLs, and OpsPilot release mappings.

## Adjustment Scope

The first adjustment is metadata only: update GitLab project descriptions so
the project list is readable immediately. Repository moves are deferred to
separate, explicit migration steps because they affect release automation.

Any actual GitLab mutation requires explicit confirmation first. This includes
metadata updates, group creation, project moves, archive/delete operations,
permission changes, deploy-token changes, CI/CD variables, and URL changes used
by Argo CD, GitOps, backup jobs, or git-sync.

## GitLab Adjustments Applied

Applied after explicit confirmation.

Created target groups:

| Group | Action |
| --- | --- |
| `tpo/apps` | created |
| `tpo/apps/devex` | created |
| `tpo/apps/office` | created |
| `tpo/apps/collab` | created |
| `tpo/platform` | created |
| `tpo/platform/opspilot` | created |
| `tpo/deploy` | created |
| `tpo/shared` | created |
| `tpo/ops` | created |
| `tpo/ops/backups` | created |
| `tpo/sandbox` | created |
| `tpo/sandbox/devex` | created |

Updated project descriptions:

| Project | Description prefix |
| --- | --- |
| `platform/opspilot` | `[PLATFORM]` |
| `platform/opspilot-skills` | `[SKILL]` |
| `platform/gitops-manifests` | `[DEPLOY]` |
| `tpo/devex/opspilot/cluster-etcd-backups` | `[BACKUP]` |
| `tpo/devex/opspilot/opspilot-core` | `[LEGACY]` |
| `tpo/devex/demo/demo-api` | `[SANDBOX]` |
| `platform/devex/demo/ai-loop-demo` | `[SANDBOX]` |
| `platform/devex/frontend-vite-demo` | `[SANDBOX]` |
| `platform/devex/java-spring-demo` | `[SANDBOX]` |
| `platform/devex/python-fastapi-demo` | `[SANDBOX]` |
| `platform/devex/demo/resource-guardrail-demo` | `[SANDBOX]` |
| `root/test-cluster-backup` | `[BACKUP]` |
| `root/yaml` | `[OPS]` |
| `platform/demo-api-deletion_scheduled-6` | `[ARCHIVE]` |
| `platform/password-deletion_scheduled-4` | `[ARCHIVE]` |

Not changed in this phase:

- repository paths;
- project permissions;
- CI/CD variables;
- deploy tokens;
- container registry image paths;
- Argo CD `repoURL`;
- GitOps source paths;
- backup job remotes;
- OpsPilot release mappings.

## Validation

- GitLab project inventory was read from the node206 GitLab API.
- Target groups exist under `tpo`.
- Project descriptions now show the repository type prefix in the GitLab project
  list.
- No GitLab repository path was moved in this change.
