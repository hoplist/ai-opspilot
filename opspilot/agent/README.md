# opspilot-agent

`opspilot-agent` is a small read-only node-side helper for Docker container
troubleshooting. It does not persist logs and does not mutate containers.

## Configuration

```bash
OPSPILOT_AGENT_PORT=19080
OPSPILOT_AGENT_DOCKER_SOCKET=/var/run/docker.sock
OPSPILOT_AGENT_ALLOWED_CONTAINERS=gitlab,prometheus,cadvisor,node-exporter
OPSPILOT_AGENT_TOKEN=
OPSPILOT_AGENT_HOST_ROOT=/host
OPSPILOT_AGENT_DISK_ALLOWED_PATHS=/var/lib/docker,/var/log,/opt,/data,/data*
OPSPILOT_AGENT_DISK_MAX_DEPTH=2
OPSPILOT_AGENT_DISK_TOP_LIMIT=20
```

If `OPSPILOT_AGENT_ALLOWED_CONTAINERS` is empty, every container request is
denied except `/health`.

`OPSPILOT_AGENT_TOKEN` is required when the agent listens on a non-local host
such as `0.0.0.0` or a LAN IP. Local-only development listeners like
`127.0.0.1` can still run without a token.

When `opspilot-core` calls token-protected agents, configure the matching
backend secret environment variable:

```bash
OPSPILOT_NODE_AGENT_TOKENS=node206=<token>
```

Do not store the token in ConfigMap or GitOps manifests.
For node206 Docker Compose, keep `OPSPILOT_AGENT_TOKEN` in the local `.env`
file and inject the matching `OPSPILOT_NODE_AGENT_TOKENS` into
`opspilot-core` through a Kubernetes Secret such as
`opspilot-node-agent-secrets`.

## API

- `GET /health`
- `GET /api/containers`
- `GET /api/containers/{container}/inspect`
- `GET /api/containers/{container}/logs?tail=300&since_seconds=1800`
- `GET /api/containers/{container}/stats`
- `GET /api/host/disk?limit=20&depth=2`
- `GET /api/host/network?limit=20&duration=5`

Limits are enforced in the agent:

- `tail <= 1000`
- `since_seconds <= 86400`
- `limit_bytes <= 5MiB`
- `disk depth <= 4`
- `disk top limit <= 100`
- `network duration <= 30`
- `network top limit <= 100`

`/api/host/disk` is read-only. It scans only
`OPSPILOT_AGENT_DISK_ALLOWED_PATHS`, does not follow symlinks, reports Docker
`system df` evidence, reports allowed container log file sizes, and returns a
plan-only cleanup recommendation. It never truncates logs, deletes files,
runs `docker prune`, edits Docker daemon config, or restarts containers.
Trailing `*` path patterns are expanded at runtime, so `/data*` includes
additional mounted directories such as `/data00` and `/data01` when they exist.

Disk attribution walks directory entries up to the configured depth. It is
bounded by depth, top limit, request timeout, and the allowed path list, but it
can still add I/O pressure on very large hostPath trees. Prefer Prometheus
filesystem metrics for frequent checks, and use `/api/host/disk` on demand when
you need directory-level attribution.

`/api/host/network` is read-only and does not use eBPF. It takes two short
samples of `/proc/net/dev` plus allowed Docker container stats, then reports
interface/container RX/TX rates and TCP state counts. It does not capture packet
payloads and does not persist traffic data.

When the agent runs in Docker and needs host directory attribution, mount the
host paths read-only under `OPSPILOT_AGENT_HOST_ROOT`, for example:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
  - /proc:/host/proc:ro
  - /var/lib/docker:/host/var/lib/docker:ro
  - /var/log:/host/var/log:ro
  - /opt:/host/opt:ro
  - /data:/host/data:ro
```

Kubernetes cluster node monitoring should normally use Prometheus plus
node-exporter through OpsPilot `metrics nodes` and `metrics filesystems`.
Deploying this agent on Kubernetes workers is only needed when OpsPilot must
attribute host-side directories such as `/var/lib/containerd`, `/var/log`, or
business hostPath directories.
