#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

push_pair() {
  local src="$1"
  local dst="$2"
  echo "==> ${src} -> ${dst}"
  docker pull "${src}"
  docker tag "${src}" "${dst}"
  docker push "${dst}"
}

for list in "${ROOT_DIR}/image-lists/required.txt" "${ROOT_DIR}/image-lists/ci.txt"; do
  while IFS= read -r line; do
    line="${line%%#*}"
    line="$(echo "${line}" | xargs || true)"
    [ -z "${line}" ] && continue
    if [[ "${line}" != *" -> "* ]]; then
      echo "invalid image mapping in ${list}: ${line}" >&2
      exit 1
    fi
    src="${line%% -> *}"
    dst="${line##* -> }"
    push_pair "${src}" "${dst}"
  done <"${list}"
done

echo "OpsPilot and CI images pushed to docker-hub.tpo.xzoa.com/opspilot/"
