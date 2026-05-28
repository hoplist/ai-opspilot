# 2026-05-28 CI Image Pull Policy

## Context

The OpsPilot release pipeline failed before tests because the GitLab Runner
could not pull `m.daocloud.io/docker.io/library/alpine:3.20`; the external
mirror returned a TLS handshake timeout. This is a CI infrastructure dependency
failure, not an application test failure.

## Change

- Changed OpsPilot pipeline job images to use `pull_policy: if-not-present`.
- Applied the same policy to shared BuildKit/GitOps templates for Go, Python,
  Node, frontend, and Java services.

## Intent

When node206 GitLab Runner already has the base image cached, the standard
release path can continue without depending on an external mirror handshake for
every job. If the image is absent, Runner can still pull it normally.
