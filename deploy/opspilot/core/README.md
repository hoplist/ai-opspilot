# opspilot-core deployment

MVP deployment for the Go `opspilot-core`.

Build the image from repository root:

```bash
$env:CGO_ENABLED="0"
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot-core ./opspilot/core
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot ./opspilot/cli
docker build -f opspilot/Dockerfile -t opspilot-core:0.1.0-mvp-go .
```

Deploy:

```bash
kubectl apply -k deploy/opspilot/core
```

The service exposes `18080` inside the cluster and NodePort `32180` for direct
access from the node200 network.

## Runtime skills

The deployment mounts server-side runtime skills at:

```text
/opt/opspilot/skills/current
```

The default mount uses `emptyDir` plus the `skills-sync` sidecar. GitLab remains
the source of truth; the mounted directory is only a cache. Use a PVC later only
if the skills repo becomes large or slow to pull. Avoid hostPath for normal
multi-node deployments because it ties the Pod to one node-local directory.

If the skills repository is private, create an out-of-band Secret named
`opspilot-skills-secrets` in namespace `opspilot` with:

```text
OPSPILOT_SKILLS_GIT_USERNAME=<deploy token username or oauth2>
OPSPILOT_SKILLS_GIT_PASSWORD=<read-only GitLab token>
```
