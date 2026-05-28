# 2026-05-28 CI Base Image Source

## Context

The OpsPilot release pipeline failed before tests because the GitLab Runner
could not pull `m.daocloud.io/docker.io/library/alpine:3.20`; the external
mirror returned a TLS handshake timeout. This is a CI infrastructure dependency
failure, not an application test failure. An intermediate attempt to use
`pull_policy: if-not-present` was rejected because the node206 GitLab Runner
only allows the default `always` pull policy.

## Change

- Mirrored the CI base images into the node206-trusted internal registry:
  - `docker-hub.tpo.xzoa.com/opspilot/ci-alpine:3.20`
  - `docker-hub.tpo.xzoa.com/opspilot/ci-golang:1.23-alpine`
  - `docker-hub.tpo.xzoa.com/opspilot/ci-buildkit:rootless`
  - `docker-hub.tpo.xzoa.com/opspilot/ci-python:3.12-alpine`
  - `docker-hub.tpo.xzoa.com/opspilot/ci-node:20-alpine`
  - `docker-hub.tpo.xzoa.com/opspilot/ci-maven:3.9.9-jdk21-alpine`
- Pointed the OpsPilot pipeline and shared BuildKit/GitOps templates at these
  internal CI images.
- Removed the unsupported `pull_policy: if-not-present` setting.

## Intent

The standard release path still runs through node206 GitLab Runner and BuildKit,
but runner job-image pulls no longer depend on an external mirror handshake.
Application images continue to be built and pushed by the normal GitLab
Registry/GitOps/Argo CD flow.
