#!/usr/bin/env bash
set -euo pipefail

BASE_DIR="/data/ai-dev/DeerFlow"

mkdir -p "${BASE_DIR}/skills" "${BASE_DIR}/threads"
chmod 0777 "${BASE_DIR}" "${BASE_DIR}/skills" "${BASE_DIR}/threads"
find "${BASE_DIR}/threads" -type d -exec chmod 0777 {} +

if [[ -f ./app.py ]]; then
  cp ./app.py "${BASE_DIR}/app.py"
  chmod 0644 "${BASE_DIR}/app.py"
fi

echo "Prepared ${BASE_DIR}"
find "${BASE_DIR}" -maxdepth 2 -type d -printf '%m %u:%g %p\n' | sort
