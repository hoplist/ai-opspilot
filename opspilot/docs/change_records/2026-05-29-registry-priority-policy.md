# 2026-05-29 Registry Priority Policy

## Context

The test environment should prefer node206 GitLab Registry for application
images deployed to node200. A previous CI recovery used
`docker-hub.tpo.xzoa.com/opspilot/ci-*` for runner job base images because the
node206 Docker executor rejected the GitLab HTTP registry endpoint unless it is
configured as trusted.

## Decision

- node200 runtime application images default to node206 GitLab Registry:
  `192.168.48.206:5050/...`.
- Standard BuildKit templates continue to push application images through
  `$CI_REGISTRY_IMAGE` and write that image into GitOps.
- `docker-hub.tpo.xzoa.com` is not the default runtime registry.
- Future pushes to `docker-hub.tpo.xzoa.com` require explicit confirmation and
  a recorded exception.
- CI base images are treated separately from runtime images. If they cannot be
  pulled from GitLab Registry yet, the runner trust/configuration gap must be
  made visible instead of silently pushing new private-registry mirrors.

## Follow-up

Configured the node206 Docker daemon to trust `192.168.48.206:5050` as an HTTP
registry, then moved OpsPilot and shared CI template base images from the
temporary private mirror to node206 GitLab Registry:

- `192.168.48.206:5050/platform/opspilot/ci-alpine:3.20`
- `192.168.48.206:5050/platform/opspilot/ci-golang:1.23-alpine`
- `192.168.48.206:5050/platform/opspilot/ci-buildkit:rootless`
- `192.168.48.206:5050/platform/opspilot/ci-python:3.12-alpine`
- `192.168.48.206:5050/platform/opspilot/ci-node:20-alpine`
- `192.168.48.206:5050/platform/opspilot/ci-maven:3.9.9-jdk21-alpine`

The temporary `docker-hub.tpo.xzoa.com/opspilot/ci-*` mirror is no longer used
by the checked-in OpsPilot CI templates.
