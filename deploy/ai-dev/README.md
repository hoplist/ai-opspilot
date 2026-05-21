# ai-dev DeerFlow hostPath deployment

This package deploys a test DeerFlow sandbox provisioner into namespace `ai-dev`.

## Images

- Provisioner runtime: `docker-hub.tpo.xzoa.com/python:3.12-slim-tools`
- Sandbox runtime: `docker-hub.tpo.xzoa.com/xagent/all-in-one-sandbox:latest`

## Node and hostPath

- Node selector: `kubernetes.io/hostname=xzyc115-19`
- `NODE_HOST`: the address returned to DeerFlow for sandbox NodePort access. Use the node IP or a DNS name reachable from the DeerFlow backend. If `xzyc115-19` cannot be resolved from the backend, replace it in the YAML.
- Base directory on worker node: `/data/ai-dev/DeerFlow`
- Provisioner script: `/data/ai-dev/DeerFlow/app.py`
- Skills: `/data/ai-dev/DeerFlow/skills`
- Thread data: `/data/ai-dev/DeerFlow/threads`

## Prepare worker node

Run on `xzyc115-19`:

```bash
cd /path/to/ai-dev
bash prepare-deerflow-node.sh
```

## Deploy

Run from a machine with kubeconfig access:

```bash
kubectl apply -f deer-flow-provisioner-hostpath.yaml
kubectl -n ai-dev rollout status deploy/deer-flow-provisioner
kubectl -n ai-dev get pod,svc -o wide
```

If DeerFlow reports `Sandbox ... failed to become ready within timeout`, check:

```bash
kubectl -n ai-dev get pod,svc -l app=deer-flow-sandbox -o wide
kubectl -n ai-dev describe pod -l app=deer-flow-sandbox
curl http://<NODE_HOST>:<SANDBOX_NODEPORT>/v1/sandbox
```

If sandbox is Ready but cannot write `/mnt/user-data`, rerun `prepare-deerflow-node.sh` on `xzyc115-19`, copy the updated `app.py` to `/data/ai-dev/DeerFlow/app.py`, restart the provisioner, and recreate the affected sandbox. The provisioner adds an initContainer to every new sandbox. It creates `/mnt/user-data/workspace`, `/mnt/user-data/uploads`, and `/mnt/user-data/outputs`, chowns the mounted thread directory to `gem:gem` when the `gem` user exists, and makes the three writable work directories sticky-writable for compatibility with non-root sandbox processes.

Health check:

```bash
curl http://xzyc115-19:31082/health
```
