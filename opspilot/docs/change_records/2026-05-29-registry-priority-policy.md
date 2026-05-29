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

To make CI base images also come from node206 GitLab Registry, configure the
node206 GitLab Runner Docker daemon to trust `192.168.48.206:5050` or enable a
TLS registry endpoint. After that, the CI image variables can move from the
temporary private mirror to GitLab Registry.
