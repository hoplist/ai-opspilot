#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARCHIVE="${ROOT_DIR}/argocd/artifacts/argocd-images.tar.gz"

if [ ! -s "${ARCHIVE}" ]; then
  echo "missing ${ARCHIVE}" >&2
  exit 1
fi

if command -v k3s >/dev/null 2>&1; then
  gzip -dc "${ARCHIVE}" | k3s ctr -n k8s.io images import -
elif command -v ctr >/dev/null 2>&1; then
  gzip -dc "${ARCHIVE}" | ctr -n k8s.io images import -
elif command -v docker >/dev/null 2>&1; then
  gzip -dc "${ARCHIVE}" | docker load
else
  echo "no k3s, ctr, or docker command found" >&2
  exit 1
fi

echo "Argo CD images loaded from ${ARCHIVE}"
