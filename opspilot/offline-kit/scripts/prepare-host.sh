#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "prepare-host.sh must run as root" >&2
  exit 1
fi

mkdir -p \
  /data/k3s \
  /data/opspilot \
  /data/opspilot/hostpath \
  /data/opspilot/audit \
  /data/opspilot/evidence-packs \
  /data/opspilot/error-events \
  /data/logs \
  /data/backup

cat >/etc/sysctl.d/99-opspilot-k3s.conf <<'EOF'
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
fs.inotify.max_user_instances = 8192
fs.inotify.max_user_watches = 524288
EOF

sysctl --system >/dev/null

echo "Host prepared. Reboot is recommended before installing K3s if this is a fresh host."
