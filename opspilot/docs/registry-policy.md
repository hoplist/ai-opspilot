# OpsPilot Registry Policy

OpsPilot test-environment releases should prefer the node206 GitLab Container
Registry for application images.

## Runtime Images

Application images deployed into node200 should use node206 GitLab Registry
references:

```text
192.168.48.206:5050/<group>/<project>/<image>:<commit-tag>
```

The standard GitLab CI templates already build and push application images with
`$CI_REGISTRY_IMAGE`, then write the same image reference into GitOps for Argo CD
to reconcile on node200.

## Private Registry Use

`docker-hub.tpo.xzoa.com` is not the default runtime image registry for OpsPilot
managed releases.

Pushes to that private registry require explicit confirmation before they are
performed. If it is used, record why it is needed and whether it is a runtime
image, CI base image, or temporary bootstrap artifact.

## CI Base Images

CI job images are separate from runtime application images.

The preferred CI base-image location is also node206 GitLab Registry:

```text
192.168.48.206:5050/platform/opspilot/ci-*:*
```

If the node206 GitLab Runner cannot pull CI base images from the GitLab Registry
because the Docker executor does not trust the HTTP registry endpoint, either:

- configure the runner host to trust `192.168.48.206:5050`, or
- use a confirmed temporary CI base-image mirror and document the exception.

Do not silently push new CI base images to a private registry as part of normal
release automation.

## Generated Manifests

Generated Kubernetes manifests should not hardcode private-registry credentials
or create per-namespace image pull secrets. Registry trust and credentials are
owned by the node/container runtime in the test environment.
