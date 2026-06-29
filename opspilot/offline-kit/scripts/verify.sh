#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "== K3s =="
if command -v kubectl >/dev/null 2>&1; then
  kubectl get nodes -o wide

  echo "== system pods =="
  kubectl get pods -A
else
  echo "kubectl not found; skip live cluster checks"
fi

echo "== registry access test =="
if command -v k3s >/dev/null 2>&1; then
  k3s ctr -n k8s.io images ls | grep -E 'prometheus|node-exporter|kube-state-metrics|opspilot|git-sync' || true
else
  echo "k3s not found; skip live image checks"
fi

echo "== offline artifacts =="
test -s "${ROOT_DIR}/k3s/artifacts/k3s"
test -s "${ROOT_DIR}/k3s/artifacts/install.sh"
test -s "${ROOT_DIR}/k3s/artifacts/k3s-airgap-images-amd64.tar.zst"
test -s "${ROOT_DIR}/monitoring/artifacts/monitoring-images.tar.gz"
test -s "${ROOT_DIR}/argocd/artifacts/argocd-images.tar.gz"

echo "== disk =="
df -h / /data || true

echo "verify completed"
