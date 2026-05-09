# Prometheus Deployment

This directory contains the Prometheus deployment assets for the `monitoring`
namespace.

Files:

- `namespace.yaml`: namespace bootstrap
- `pv.yaml`: static NFS PV backed by `192.168.48.206:/srv/nfs/monitoring/prometheus`
- `values.yaml`: Helm values for `prometheus-community/prometheus`
- `install.ps1`: local install helper

Notes:

- `kube-state-metrics` is enabled and pinned to the reachable mirror
  `k8s.m.daocloud.io/kube-state-metrics/kube-state-metrics:v2.18.0`
- `alertmanager` and `pushgateway` stay disabled for a lightweight footprint

External access after deployment:

- Prometheus NodePort: `32092`
- Suggested URL: `http://192.168.48.200:32092`
