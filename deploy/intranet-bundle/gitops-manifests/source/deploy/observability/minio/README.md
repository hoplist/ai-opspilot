# MinIO Deployment

This directory deploys a single-node MinIO instance for RCA cold archive data.

It is intended for:

- investigation archive payloads
- long-term cold snapshots or exported artifacts

Apply:

```powershell
kubectl apply -k deploy/observability/minio
```

Default endpoints:

- API NodePort: `32093`
- Console NodePort: `32094`
