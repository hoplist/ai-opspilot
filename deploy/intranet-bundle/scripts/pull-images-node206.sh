#!/usr/bin/env bash
set -euo pipefail

bundle_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image_file="${1:-$bundle_dir/images/auto-inspection-images.txt}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker command not found" >&2
  exit 1
fi

if [ ! -f "$image_file" ]; then
  echo "image list not found: $image_file" >&2
  exit 1
fi

echo "image list: $image_file"
echo

while IFS= read -r image || [ -n "$image" ]; do
  image="$(echo "$image" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
  if [ -z "$image" ] || [ "${image#\#}" != "$image" ]; then
    continue
  fi
  echo "==> docker pull $image"
  docker pull "$image"
done < "$image_file"

echo
echo "done"
