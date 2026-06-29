#!/usr/bin/env bash
set -euo pipefail

# Export local Git repositories into offline-kit/repos/*.bundle.
# Usage:
#   OPSPILOT_REPO_ROOT=/path/to/opspilot \
#   GITOPS_REPO_ROOT=/path/to/gitops-manifests \
#   OPSPILOT_CONFIG_REPO_ROOT=/path/to/opspilot-config \
#   OPSPILOT_SKILLS_REPO_ROOT=/path/to/opspilot-skills \
#   CI_TEMPLATES_REPO_ROOT=/path/to/ci-templates \
#     bash scripts/export-source-bundles.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/repos"
mkdir -p "${OUT_DIR}"

bundle_repo() {
  local env_name="$1"
  local bundle_name="$2"
  local repo="${!env_name:-}"
  if [ -z "${repo}" ]; then
    echo "skip ${bundle_name}: ${env_name} is not set"
    return
  fi
  if [ ! -d "${repo}/.git" ]; then
    echo "skip ${bundle_name}: ${repo} is not a Git repository" >&2
    return
  fi
  echo "==> ${repo} -> ${OUT_DIR}/${bundle_name}"
  git -C "${repo}" bundle create "${OUT_DIR}/${bundle_name}" --all
}

bundle_repo OPSPILOT_REPO_ROOT platform-opspilot.bundle
bundle_repo GITOPS_REPO_ROOT platform-gitops-manifests.bundle
bundle_repo OPSPILOT_CONFIG_REPO_ROOT platform-opspilot-config.bundle
bundle_repo OPSPILOT_SKILLS_REPO_ROOT platform-opspilot-skills.bundle
bundle_repo CI_TEMPLATES_REPO_ROOT platform-ci-templates.bundle

echo "source bundle export completed"
