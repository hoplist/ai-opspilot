# OpenSearch Deployment

This folder contains the Kubernetes manifests for the test-cluster
OpenSearch deployment in the `observability` namespace.

Apply:

```powershell
kubectl apply -k deploy/observability/opensearch
```

External access:

- NodePort HTTP: `32090`
- Suggested URL from local machine: `http://192.168.48.200:32090`

Bootstrap templates and retention policies:

```powershell
python bootstrap_opensearch.py
```

This now installs:

- index templates for logs / events / incidents / investigations
- normalized log field mappings
- ISM retention policies for logs, events, incidents, and investigations
- local filesystem snapshot repository registration
- disk watermark settings in `opensearch.yml`
