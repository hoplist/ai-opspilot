#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PARENT_DIR="$(dirname "${ROOT_DIR}")"
VERSION="$(cat "${ROOT_DIR}/VERSION")"
OUT_DIR="${ROOT_DIR}/packages"
OUT_FILE="${OUT_DIR}/opspilot-offline-kit-${VERSION}.tar.gz"
TMP_DIR="$(mktemp -d)"
TMP_FILE="${TMP_DIR}/opspilot-offline-kit-${VERSION}.tar.gz"

cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

mkdir -p "${OUT_DIR}"
rm -f "${OUT_FILE}" "${OUT_FILE}.sha256"

tar \
  --exclude='offline-kit/packages/*.tar.gz' \
  --exclude='offline-kit/packages/*.sha256' \
  -C "${PARENT_DIR}" \
  -czf "${TMP_FILE}" \
  "$(basename "${ROOT_DIR}")"

mv "${TMP_FILE}" "${OUT_FILE}"

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${OUT_FILE}" >"${OUT_FILE}.sha256"
fi

echo "created ${OUT_FILE}"
