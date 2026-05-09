# Prometheus Deployment

This directory contains the Prometheus deployment assets for the `monitoring`
namespace.

Files:

- `namespace.yaml`: namespace bootstrap
- `values.yaml`: Prometheus server uses a direct hostPath volume at `/data/observability/prometheus`; no PV/PVC is created.
- `values.yaml`: Helm values for `prometheus-community/prometheus`
- `install.ps1`: local install helper

Notes:

- `kube-state-metrics` is enabled and pinned to the reachable mirror
  `docker-hub.tpo.xzoa.com/auto-inspection/kube-state-metrics:v2.18.0`
- `alertmanager` and `pushgateway` stay disabled for a lightweight footprint

External access after deployment:

- Prometheus NodePort: `32092`
- Suggested URL: `http://192.168.48.200:32092`


