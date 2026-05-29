# 2026-05-29 GitLab repository phase 2 migration

## Goal

Move low-risk repositories into the new `tpo` governance layout without
touching the active OpsPilot release chain.

Not moved in this phase:

- `platform/opspilot`
- `platform/gitops-manifests`
- `platform/opspilot-skills`

## GitLab Changes

Moved:

| Old path | New path |
| --- | --- |
| `tpo/devex/opspilot/cluster-etcd-backups` | `tpo/ops/backups/node200-etcd-snapshots` |
| `root/test-cluster-backup` | `tpo/ops/backups/test-cluster-backup` |
| `root/yaml` | `tpo/ops/yaml` |

Updated descriptions:

| Project | Description intent |
| --- | --- |
| `tpo/ops/backups/node200-etcd-snapshots` | `[BACKUP]` machine-written node200 etcd snapshots, latest three retained. |
| `tpo/ops/backups/test-cluster-backup` | `[BACKUP]` legacy test cluster backup assets for audit and retirement review. |
| `tpo/ops/yaml` | `[OPS]` empty manual YAML holding area, not GitOps desired state. |
| `tpo/devex/opspilot/opspilot-core` | `[SHARED]` GitLab CI include source, not live OpsPilot core. |

## node200 Backup Update

Updated node200:

```text
/usr/local/sbin/etcd-snapshot-gitlab-backup.sh
/var/backups/etcd-gitlab/repo
```

New remote:

```text
ssh://git@192.168.48.206:2224/tpo/ops/backups/node200-etcd-snapshots.git
```

During validation, the backup job exposed an existing issue: `main` on the
backup repository was protected with force-push disabled. The job is designed to
rewrite `main` so the repository keeps only the latest three snapshots. Force
push was enabled for `main` on the backup repository.

## Validation

- `git ls-remote` from node200 to the new backup repository succeeded with the
  existing `/root/.ssh/etcd_backup_gitlab` deploy key.
- Manual `etcd-snapshot-gitlab-backup.service` run completed successfully.
- The new GitLab project contains only the latest three snapshot dates:
  `2026-05-27`, `2026-05-28`, and `2026-05-29`.
- `etcd-snapshot-gitlab-backup.timer` remains active. The next scheduled run is
  `2026-05-30 02:21:50 CST`.

## Deferred Demo Migration

Sandbox/demo project transfer is still pending. GitLab blocked transfer because
these projects have Container Registry tags:

| Project | Registry repositories | Tags |
| --- | ---: | ---: |
| `tpo/devex/demo/demo-api` | 1 | 2 |
| `platform/devex/demo/ai-loop-demo` | 1 | 5 |
| `platform/devex/frontend-vite-demo` | 1 | 2 |
| `platform/devex/java-spring-demo` | 1 | 2 |
| `platform/devex/python-fastapi-demo` | 1 | 2 |
| `platform/devex/demo/resource-guardrail-demo` | 1 | 2 |

No registry tags were deleted. Next step is to choose one of two paths:

1. Retire demo images, delete old registry tags, transfer the projects, then
   rebuild demos through the standard pipeline.
2. Leave current demo projects in place and create fresh sandbox demos only
   under `tpo/sandbox/devex`.

## Rollback

For GitLab project path rollback, transfer each moved project back to its old
namespace and restore the old project path where applicable.

For node200 backup rollback:

```bash
REMOTE_URL=ssh://git@192.168.48.206:2224/tpo/devex/opspilot/cluster-etcd-backups.git \
  /usr/local/sbin/etcd-snapshot-gitlab-backup.sh
```

The script also has timestamped local backups under:

```text
/usr/local/sbin/etcd-snapshot-gitlab-backup.sh.pre-phase2-*
```
