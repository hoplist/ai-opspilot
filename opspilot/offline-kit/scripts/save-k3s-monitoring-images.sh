#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT_DIR}/monitoring/artifacts/monitoring-images.tar.gz"
TMP="${ROOT_DIR}/monitoring/artifacts/monitoring-images.tar"

rm -f "${TMP}" "${OUT}"

images=()
while IFS= read -r line; do
  line="${line%%#*}"
  line="$(echo "${line}" | xargs || true)"
  [ -z "${line}" ] && continue
  images+=("${line}")
done <"${ROOT_DIR}/image-lists/monitoring.txt"

if [ "${#images[@]}" -eq 0 ]; then
  echo "no monitoring images configured" >&2
  exit 1
fi

for image in "${images[@]}"; do
  docker pull "${image}"
done

docker save -o "${TMP}" "${images[@]}"
gzip -f "${TMP}"
echo "saved ${OUT}"
