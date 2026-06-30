# 2026-06-30 Remove Compatibility GitLab Repositories

## Goal

Remove compatibility/history repositories from node206 GitLab so the inner
network baseline contains only active OpsPilot repositories and the active etcd
backup repository.

## Deleted Repositories

Deleted after mirror backup:

- `platform/opspilot`
- `tpo/devex/opspilot/opspilot-core`

Kept:

- `tpo/ops/backups/node200-etcd-snapshots`

## Backup

Mirror backups were created before deletion:

```text
D:\code\auto_inspection\backups\gitlab-compat-delete-20260630-183105
```

## Registry Cleanup

The deleted projects still had Container Registry repositories and tags. The
cleanup removed:

- old `platform/opspilot` runtime tags;
- old CI base image tags under the compatibility project;
- old git-sync and BuildKit cache tags;
- `tpo/devex/opspilot/opspilot-core` shared-template image tags.

After deleting tags and registry repositories, the GitLab projects were removed.
GitLab created `*-deletion_scheduled-*` entries first; those scheduled entries
were removed through GitLab Rails `Projects::DestroyService` on node206.

## Verified Remaining GitLab Projects

```text
tpo/deploy/gitops-manifests
tpo/ops/backups/node200-etcd-snapshots
tpo/platform/opspilot/opspilot-config
tpo/platform/opspilot/opspilot-core
tpo/platform/opspilot/opspilot-skills
```

## Empty Group Cleanup

After the project cleanup, GitLab still showed empty groups in the UI. Removed
the unused groups:

- `platform`
- `tpo/apps`
- `tpo/devex`
- `tpo/sandbox`
- `tpo/shared`

GitLab first renamed those groups to `*-deletion_scheduled-*`. The scheduled
group entries were then removed with GitLab Rails `Groups::DestroyService` on
node206.

Verified remaining groups:

```text
tpo
tpo/deploy
tpo/ops
tpo/ops/backups
tpo/platform
tpo/platform/opspilot
```

## Validation

- GitLab project API returns only the five repositories above.
- GitLab group API returns only the required groups above.
- Argo CD Applications remain `Synced` and `Healthy`.
- `node200-etcd-snapshots` backup repository was not changed.

## Rollback

If the compatibility source repositories are needed again, restore them from
the mirror backup directory. Registry images were intentionally deleted because
the current baseline uses:

```text
192.168.48.206:5050/tpo/platform/opspilot/opspilot-core/opspilot-core:<tag>
```

or the prepared inner-network private tag:

```text
docker-hub.tpo.xzoa.com/opspilot/opspilot-core:8553f0ba
```
