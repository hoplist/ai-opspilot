# 2026-05-29 demo registry cleanup and sandbox migration

## Goal

Clear old demo GitLab Container Registry tags that block project transfer, move
demo repositories into the governed sandbox namespace, then rebuild the demos
through the standard pipeline.

The migration must be recoverable. No registry tag should be deleted until the
tag list and source repositories are backed up.

## Scope

Candidate demo repositories:

| Current path | Target path |
| --- | --- |
| `tpo/devex/demo/demo-api` | `tpo/sandbox/devex/demo-api` |
| `platform/devex/demo/ai-loop-demo` | `tpo/sandbox/devex/ai-loop-demo` |
| `platform/devex/frontend-vite-demo` | `tpo/sandbox/devex/frontend-vite-demo` |
| `platform/devex/java-spring-demo` | `tpo/sandbox/devex/java-spring-demo` |
| `platform/devex/python-fastapi-demo` | `tpo/sandbox/devex/python-fastapi-demo` |
| `platform/devex/demo/resource-guardrail-demo` | `tpo/sandbox/devex/resource-guardrail-demo` |

## Safety Plan

Before cleanup:

- Export GitLab project metadata, registry repository metadata, and tag metadata
  to a timestamped backup directory.
- Create Git bundle backups for each source repository.
- Attempt image archive backups when a registry tag is still pullable from
  node206.
- Record every deleted tag in this change record.

Rollback paths:

- Git repositories can be restored from the `.bundle` files if a transfer fails.
- Deleted registry tags can be restored from image archives when available, or
  rebuilt from Git commit tags through the standard GitLab Runner -> BuildKit ->
  Registry -> GitOps -> Argo CD flow.
- Project transfer can be rolled back by transferring the project back to the old
  namespace while GitLab redirects are still present.

## Planned Cleanup

- Delete only sandbox/demo registry tags needed to unblock GitLab project
  transfer.
- Do not delete OpsPilot core, GitOps, skills, CI base images, production
  images, or node200 backup data.
- Remove or de-emphasize obsolete demo release references only after migrated
  projects can build from their new path.

## Execution Log

### Inventory

Registry tags blocking transfer:

| Project | Image | Tags |
| --- | --- | --- |
| `tpo/devex/demo/demo-api` | `192.168.48.206:5050/tpo/devex/demo/demo-api/demo-api` | `8d5df845`, `buildcache` |
| `platform/devex/demo/ai-loop-demo` | `192.168.48.206:5050/platform/devex/demo/ai-loop-demo/ai-loop-demo` | `73f80a62`, `ae4710ca`, `bb0bf8db`, `buildcache`, `e3d7fc78` |
| `platform/devex/demo/resource-guardrail-demo` | `192.168.48.206:5050/platform/devex/demo/resource-guardrail-demo/resource-guardrail-demo` | `5dd5ed52`, `buildcache` |
| `platform/devex/frontend-vite-demo` | `192.168.48.206:5050/platform/devex/frontend-vite-demo/frontend-vite-demo` | `84107620`, `buildcache` |
| `platform/devex/java-spring-demo` | `192.168.48.206:5050/platform/devex/java-spring-demo/java-spring-demo` | `1edd81e6`, `buildcache` |
| `platform/devex/python-fastapi-demo` | `192.168.48.206:5050/platform/devex/python-fastapi-demo/python-fastapi-demo` | `697adcb6`, `buildcache` |

GitOps currently contains demo Argo Applications and deployment images for
these demos. OpsPilot `OPSPILOT_RELEASE_SERVICES` currently contains release
mappings for `ai-loop-demo`, `python-fastapi-demo`, `frontend-vite-demo`, and
`java-spring-demo`.

Backup targets:

```text
node206:/var/opt/opspilot/backups/demo-registry-migration-20260529
D:\code\auto_inspection\backups\demo-registry-migration-20260529
```
