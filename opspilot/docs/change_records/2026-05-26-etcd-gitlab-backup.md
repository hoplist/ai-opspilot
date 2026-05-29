# 2026-05-26 etcd GitLab backup

## Scope

Configured an independent node200 host-level etcd backup job. This change does
not modify OpsPilot deployments, OpsPilot configuration, Argo Applications, or
Kubernetes workloads.

## GitLab Repository

Created a private GitLab project for backups:

```text
tpo/ops/backups/node200-etcd-snapshots
ssh://git@192.168.48.206:2224/tpo/ops/backups/node200-etcd-snapshots.git
```

Original path on 2026-05-26 was
`tpo/devex/opspilot/cluster-etcd-backups`. It was moved during the GitLab
repository governance phase 2 migration on 2026-05-29.

Access uses a dedicated SSH deploy key:

```text
node200:/root/.ssh/etcd_backup_gitlab
```

Only the public key was registered in GitLab. No personal GitLab token is stored
in the backup script.

## node200 Files

```text
/usr/local/sbin/etcd-snapshot-gitlab-backup.sh
/etc/systemd/system/etcd-snapshot-gitlab-backup.service
/etc/systemd/system/etcd-snapshot-gitlab-backup.timer
/var/backups/etcd-gitlab/repo
```

## Schedule

The backup timer is enabled and active:

```text
etcd-snapshot-gitlab-backup.timer
OnCalendar=*-*-* 02:20:00
Persistent=true
RandomizedDelaySec=5m
```

First scheduled run after setup:

```text
2026-05-27 02:24:32 CST
```

## Retention

The job keeps only the latest 3 snapshot files in the GitLab branch. Because
etcd snapshots are large binary files, the job rewrites the `main` branch on
each run instead of accumulating normal Git history.

Current backup file pattern:

```text
snapshots/etcd-node200-YYYY-MM-DD.db
snapshots/etcd-node200-YYYY-MM-DD.db.meta.txt
snapshots/etcd-node200-YYYY-MM-DD.db.status.txt
```

## Validation

Manual test run completed successfully on 2026-05-26:

- `etcdctl snapshot save` produced a snapshot of about 19 MB.
- Snapshot was pushed to GitLab branch `main`.
- A fresh clone from GitLab contained:
  - `README.md`
  - `snapshots/etcd-node200-2026-05-26.db`
  - `snapshots/etcd-node200-2026-05-26.db.meta.txt`
  - `snapshots/etcd-node200-2026-05-26.db.status.txt`
- `etcdctl endpoint health` remained healthy after the backup.

After the repository move on 2026-05-29:

- node200 backup script remote was updated to the new `tpo/ops/backups` path.
- The local backup repository remote was updated.
- `main` force-push was enabled on the backup repository because the job
  intentionally rewrites the branch to retain only the latest three snapshots.
- A manual service run completed successfully and pushed snapshots for
  `2026-05-27`, `2026-05-28`, and `2026-05-29`.

## Security Note

etcd snapshots can contain Kubernetes Secrets and other sensitive cluster state.
The GitLab project must remain private and should only be accessible to trusted
cluster administrators.
