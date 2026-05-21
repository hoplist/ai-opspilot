#!/usr/bin/env bash
set -euo pipefail

PYTHON_SOURCE_IMAGE="docker-hub.tpo.xzoa.com/python:3.12-slim"
PYTHON_TARGET_IMAGE="docker-hub.tpo.xzoa.com/xagent/python:3.12-slim"
SANDBOX_SOURCE_IMAGE="enterprise-public-cn-beijing.cr.volces.com/vefaas-public/all-in-one-sandbox:latest"
SANDBOX_TARGET_IMAGE="docker-hub.tpo.xzoa.com/xagent/all-in-one-sandbox:latest"

echo "==> ensure python base image exists: ${PYTHON_SOURCE_IMAGE}"
docker image inspect "${PYTHON_SOURCE_IMAGE}" >/dev/null

echo "==> tag ${PYTHON_TARGET_IMAGE}"
docker tag "${PYTHON_SOURCE_IMAGE}" "${PYTHON_TARGET_IMAGE}"

echo "==> push ${PYTHON_TARGET_IMAGE}"
docker push "${PYTHON_TARGET_IMAGE}"

echo "==> pull ${SANDBOX_SOURCE_IMAGE}"
docker pull "${SANDBOX_SOURCE_IMAGE}"

echo "==> tag ${SANDBOX_TARGET_IMAGE}"
docker tag "${SANDBOX_SOURCE_IMAGE}" "${SANDBOX_TARGET_IMAGE}"

echo "==> push ${SANDBOX_TARGET_IMAGE}"
docker push "${SANDBOX_TARGET_IMAGE}"

echo "==> mirrored images"
docker images --format '{{.Repository}}:{{.Tag}} {{.ID}} {{.Size}}' | grep -E 'docker-hub.tpo.xzoa.com/xagent/(python|all-in-one-sandbox)'
