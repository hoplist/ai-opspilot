# 2026-06-11 Apollo Config Source Onboarding

## Background

Some services pass Apollo configuration through command flags or environment
variables, for example:

```text
/go/bin/task --env=prod --cfg=http://apolloconfig-server-inner.tpo.xzoa.com
```

Before this change, OpsPilot onboarding could generate resources, probes,
storage, middleware, and CI/GitOps files, but Apollo was not represented as a
first-class service configuration source.

## Goal

Make Apollo configurable through the same onboarding flow without forcing every
service to hand-write Kubernetes YAML.

## Change

- Added `configSources` to `opspilot.service.yaml`.
- Added Apollo detection from repository signals such as `--cfg=`, `--env=`,
  `APOLLO_META`, `apollo.meta`, and `apolloconfig`.
- Added generated `deploy/k8s/configmap.yaml` for non-secret Apollo metadata.
- Added Deployment injection modes:
  - `env`: explicit environment variables from the generated ConfigMap.
  - `args`: command arguments such as `--env=$(APOLLO_ENV)` and
    `--cfg=$(APOLLO_META)`.
  - `file`: read-only `apollo.yaml` ConfigMap mount.
- Added optional Apollo token Secret reference. OpsPilot references the Secret
  but does not generate or commit token values.
- Added onboarding and repo preflight checks for configured config sources.

## Boundaries

- This does not deploy or manage Apollo itself.
- This does not create Apollo apps, namespaces, or tokens.
- YAML generation can wire the configuration only when the application already
  reads Apollo values through env, args, or a mounted file.
- For frontend projects, build-time variables such as `VITE_*` still need CI
  build args or a runtime config file pattern; Apollo is not forced into the
  frontend image by default.

## Example

```yaml
configSources:
  apollo:
    type: apollo
    required: true
    appId: task-server
    env: prod
    cluster: default
    namespaces: application,gms
    meta: http://apolloconfig-server-inner.tpo.xzoa.com
    tokenSecret: task-server-apollo-token
    inject: args
    envFlag: --env
    metaFlag: --cfg
```

## Validation

- Unit tests cover explicit Apollo generation and repository Apollo detection.
- Demo validation should generate a small service repository, confirm
  `configmap.yaml`, Deployment env/args, Kustomize reference, and onboarding
  checks.
