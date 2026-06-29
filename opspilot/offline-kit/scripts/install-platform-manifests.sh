#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GITOPS_DIR="${ROOT_DIR}/manifests/node206/gitops"

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required before installing platform manifests" >&2
  exit 1
fi

apply_kustomize() {
  local path="$1"
  local label="$2"
  if [ ! -f "${path}/kustomization.yaml" ]; then
    echo "missing ${path}/kustomization.yaml" >&2
    exit 1
  fi
  echo "==> apply ${label}"
  kubectl apply -k "${path}"
}

wait_deployment() {
  local namespace="$1"
  local name="$2"
  local timeout="${3:-180s}"
  echo "==> wait deploy/${name} in ${namespace}"
  kubectl -n "${namespace}" rollout status "deployment/${name}" --timeout="${timeout}"
}

wait_statefulset() {
  local namespace="$1"
  local name="$2"
  local timeout="${3:-180s}"
  echo "==> wait statefulset/${name} in ${namespace}"
  kubectl -n "${namespace}" rollout status "statefulset/${name}" --timeout="${timeout}"
}

apply_kustomize "${GITOPS_DIR}/clusters/test/argocd-bootstrap" "Argo CD CRDs"
apply_kustomize "${GITOPS_DIR}/clusters/test/argocd-core" "Argo CD core"

wait_deployment argocd argocd-applicationset-controller 240s
wait_deployment argocd argocd-dex-server 240s
wait_deployment argocd argocd-notifications-controller 240s
wait_deployment argocd argocd-redis 240s
wait_deployment argocd argocd-repo-server 240s
wait_deployment argocd argocd-server 240s
wait_statefulset argocd argocd-application-controller 240s

apply_kustomize "${GITOPS_DIR}/apps" "Argo CD application entrypoints"
apply_kustomize "${GITOPS_DIR}/clusters/test/apps/opspilot-rbac" "OpsPilot namespace and RBAC"
apply_kustomize "${GITOPS_DIR}/clusters/test/apps/opspilot-prometheus" "OpsPilot Prometheus service alias"
apply_kustomize "${GITOPS_DIR}/clusters/test/apps/opspilot-core" "OpsPilot core"

echo "==> wait OpsPilot core"
kubectl -n opspilot rollout status deployment/opspilot-core --timeout=240s

echo "Platform manifests installed. If opspilot-core is not Ready, check git-sync config/skills access:"
echo "  kubectl -n opspilot describe pod -l app.kubernetes.io/name=opspilot-core"
echo "  kubectl -n opspilot logs deploy/opspilot-core -c opspilot-config-init --tail=100"
echo "  kubectl -n opspilot logs deploy/opspilot-core -c core --tail=100"
