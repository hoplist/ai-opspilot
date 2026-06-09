# opspilot-agent

`opspilot-agent` is a small read-only node-side helper for Docker container
troubleshooting. It does not persist logs and does not mutate containers.

## Configuration

```bash
OPSPILOT_AGENT_PORT=19080
OPSPILOT_AGENT_DOCKER_SOCKET=/var/run/docker.sock
OPSPILOT_AGENT_ALLOWED_CONTAINERS=gitlab,prometheus,cadvisor,node-exporter
OPSPILOT_AGENT_TOKEN=
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

## API

- `GET /health`
- `GET /api/containers`
- `GET /api/containers/{container}/inspect`
- `GET /api/containers/{container}/logs?tail=300&since_seconds=1800`
- `GET /api/containers/{container}/stats`

Limits are enforced in the agent:

- `tail <= 1000`
- `since_seconds <= 86400`
- `limit_bytes <= 5MiB`
