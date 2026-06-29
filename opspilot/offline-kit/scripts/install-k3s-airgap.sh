#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARTIFACT_DIR="${ROOT_DIR}/k3s/artifacts"

if [ "$(id -u)" -ne 0 ]; then
  echo "install-k3s-airgap.sh must run as root" >&2
  exit 1
fi

for file in k3s install.sh k3s-airgap-images-amd64.tar.zst; do
  if [ ! -f "${ARTIFACT_DIR}/${file}" ]; then
    echo "missing ${ARTIFACT_DIR}/${file}" >&2
    exit 1
  fi
done

install -m 0755 "${ARTIFACT_DIR}/k3s" /usr/local/bin/k3s
mkdir -p /var/lib/rancher/k3s/agent/images /etc/rancher/k3s
cp "${ARTIFACT_DIR}/k3s-airgap-images-amd64.tar.zst" /var/lib/rancher/k3s/agent/images/
cp "${ROOT_DIR}/k3s/config.yaml" /etc/rancher/k3s/config.yaml
cp "${ROOT_DIR}/k3s/registries.yaml" /etc/rancher/k3s/registries.yaml

INSTALL_K3S_SKIP_DOWNLOAD=true \
INSTALL_K3S_EXEC="server --data-dir /data/k3s" \
  sh "${ARTIFACT_DIR}/install.sh"

systemctl enable k3s
systemctl restart k3s
echo "Waiting for K3s node..."
kubectl wait --for=condition=Ready node --all --timeout=180s
kubectl get nodes -o wide
