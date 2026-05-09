# OpenSearch Snapshot CronJob

This directory installs a daily snapshot CronJob for the OpenSearch cluster.

Apply:

```powershell
kubectl apply -k deploy/observability/opensearch-snapshot
```

The job:

- creates a daily snapshot in `auto-inspection-local-fs`
- keeps recent snapshots and prunes older ones

It depends on the snapshot repository having already been created by:

```powershell
python bootstrap_opensearch.py
```
