# RCA Service Deployment

This directory deploys the shared RCA backend and MCP service into the cluster.

The deployment exposes:

- backend HTTP on port `18080`
- MCP HTTP on port `18081`

It expects the application source to be mounted from an NFS-backed PV and uses
in-cluster Kubernetes API access via ServiceAccount credentials.

Apply:

```powershell
kubectl apply -k deploy/rca-service
```

Validation:

- backend: `http://192.168.48.200:32180/api/health`
- mcp: `http://192.168.48.200:32181/mcp`
