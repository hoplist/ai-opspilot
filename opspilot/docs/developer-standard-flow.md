# Developer standard flow

## What Developers Do

Developers should only work in the application repository:

```text
tpo/apps/<group>/<project>/<service>
```

For current demos, the temporary location is:

```text
tpo/sandbox/devex/<service>
```

When a user has no identity mapping yet, OpsPilot treats uploads as disposable
test-only sandbox work. It should first show the plan instead of creating a
GitLab project:

```powershell
opspilot repo upload-plan --repo . --name my-demo-api --output human
```

Default plan:

```text
GitLab project: tpo/sandbox/devex/<repo>
Namespace: sandbox
GitOps path: clusters/test/apps/sandbox/<repo>
Release scope: test-only
```

This command is read-only. It does not create a GitLab project, push code, or
mutate Kubernetes.

Normal workflow:

```bash
git clone <app-repo>
cd <app-repo>
# edit code
git add .
git commit -m "change service behavior"
git push origin main
```

## What Happens After Push

```text
developer push
-> GitLab CI preflight/code-precheck
-> BuildKit builds image
-> image is pushed to GitLab Registry
-> CI updates GitOps desired state
-> Argo CD reconciles the cluster
-> OpsPilot can show release evidence
```

Developers do not need to know Pod names, GitOps paths, Argo CD Application
names, Dockerfile details, or namespace naming rules.

## What Developers Should Not Do

Developers should not directly modify:

- `tpo/deploy/gitops-manifests` or the current `platform/gitops-manifests`.
- Kubernetes Secrets.
- Argo CD Applications.
- Registry tags.
- Runtime credentials.
- Cluster-level RBAC, CNI, StorageClass, or ingress controller settings.

## Optional Test Gateway

A future front gateway can route `*.test.tpo.xzoa.com` to the single test entry
machine, but this is external gateway configuration and must not block the
standard release path. In the current phase, OpsPilot only documents the
expected route:

```text
*.test.tpo.xzoa.com -> test ingress/APISIX/Nginx entry -> sandbox or app namespace Service
```

Generated applications should still be inspectable by Pod, Service, release,
and metrics evidence even when this gateway is not connected.

## When The Repository Is Not Standard

Ask OpsPilot to onboard or repair the repo:

```text
帮我把这个仓库接入标准发布
帮我检查这个仓库为什么不能发布
帮我生成 Dockerfile 和部署 YAML
帮我把 preflight 报错修成可发布状态
```

OpsPilot should produce or update:

```text
Dockerfile
.gitlab-ci.yml
opspilot.service.yaml
deploy/k8s/*
```

## How Developers Use Middleware

Developers declare intent, not passwords:

```yaml
middleware:
  mysql:
    mode: shared
```

OpsPilot/platform creates service-scoped credentials and injects them into the
runtime Deployment. Local debugging should use temporary debug credentials, not
the long-lived service account password.

## How Developers Check Release Status

Use OpsPilot or ask AI through OpsPilot:

```text
查看 skillshub-api 发布状态
查看 skillshub-api 最近一次发布为什么失败
查看 skillshub-api 当前 Pod 是否正常
查看 skillshub-api 是否可以回退
```

OpsPilot should report:

- GitLab pipeline status.
- BuildKit job evidence.
- Registry image tag.
- GitOps desired image.
- Argo CD sync and health.
- Kubernetes rollout and Pod status.
- Logs and metrics when configured.
- Missing evidence when a datasource is not connected.
