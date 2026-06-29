#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NODE206_DIR="${ROOT_DIR}/manifests/node206"

if [ "$(id -u)" -ne 0 ]; then
  echo "install-node206-services.sh must run as root" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required on the node206 GitLab/Runner host" >&2
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose plugin is required on the node206 GitLab/Runner host" >&2
  exit 1
fi

install_compose() {
  local name="$1"
  local source="$2"
  local target="/opt/${name}"
  mkdir -p "${target}"
  cp "${source}" "${target}/docker-compose.yml"
}

install_compose gitlab "${NODE206_DIR}/compose/gitlab-docker-compose.yml"
install_compose gitlab-runner "${NODE206_DIR}/compose/gitlab-runner-docker-compose.yml"
install_compose node-exporter "${NODE206_DIR}/compose/node-exporter-docker-compose.yml"
install_compose cadvisor "${NODE206_DIR}/compose/cadvisor-docker-compose.yml"
install_compose prometheus "${NODE206_DIR}/compose/prometheus-docker-compose.yml"
install_compose opspilot-agent "${NODE206_DIR}/compose/opspilot-agent-docker-compose.yml"

mkdir -p /opt/prometheus
cp "${NODE206_DIR}/host-config/prometheus/prometheus.yml" /opt/prometheus/prometheus.yml

if [ ! -f /opt/opspilot-agent/.env ]; then
  cp "${NODE206_DIR}/compose/.env.example" /opt/opspilot-agent/.env
  echo "created /opt/opspilot-agent/.env; set OPSPILOT_AGENT_TOKEN before starting opspilot-agent" >&2
fi

start_service() {
  local name="$1"
  echo "==> start ${name}"
  (cd "/opt/${name}" && docker compose up -d)
}

start_service gitlab
start_service gitlab-runner
start_service node-exporter
start_service cadvisor
start_service prometheus

if grep -q '^OPSPILOT_AGENT_TOKEN=change-me' /opt/opspilot-agent/.env; then
  echo "skip opspilot-agent: set OPSPILOT_AGENT_TOKEN in /opt/opspilot-agent/.env first" >&2
else
  start_service opspilot-agent
fi

echo "GitLab Runner compose installed at /opt/gitlab-runner."
echo "Register the runner with a GitLab runner token before expecting CI jobs to run."
