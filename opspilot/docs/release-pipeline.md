# OpsPilot Release Pipeline

All future OpsPilot packaging, image builds, and deployments should use the
standard CI/GitOps release flow:

```text
node206 GitLab
-> node206 GitLab Runner
-> BuildKit rootless image build
-> Push image registry
-> Update GitOps repository
-> node200 Argo CD automatic deployment
```

For the test environment, use the node206 GitLab Container Registry:

```text
http://192.168.48.206:5050
```

## Rules

- Do not build release images from a local workstation.
- Do not deploy by manually editing live Kubernetes resources.
- Do not run `kubectl apply`, `rollout restart`, `scale`, or direct image
  patching for normal releases.
- Build jobs should run on the node206 GitLab Runner.
- Image packaging should use rootless BuildKit.
- Kubernetes desired state should be changed through the GitOps repository.
- node200 Argo CD should reconcile changes into the cluster.

## Local Work

Local builds are only for fast CLI or unit-test validation. For example:

```powershell
.\opspilot\scripts\build-cli.ps1
go test ./opspilot/cli
```

These local artifacts are not release artifacts. Anything intended for cluster
deployment must go through the CI/GitOps release flow above.

OpsPilot integrates with this flow as a read-only evidence chain. See
[release-evidence-chain.md](release-evidence-chain.md).

## History And Rollback

OpsPilot can now read release history from the GitOps repository and submit a
rollback as a GitOps commit:

```powershell
.\opspilot\scripts\opspilot.ps1 --output human release history --service opspilot-core --limit 10
.\opspilot\scripts\opspilot.ps1 --output human release rollback --service opspilot-core --to <tag-or-gitops-revision> --confirm
```

The rollback command changes only the configured GitOps manifest image and
commits it to the configured GitOps branch. It does not run `kubectl`, does not
call `argocd app sync`, and does not mutate live Kubernetes resources directly.
node200 Argo CD remains responsible for reconciling the committed desired state.

The GitLab token configured for OpsPilot release operations must be able to
read the service/GitOps history. Rollback additionally requires write access to
the GitOps repository.
