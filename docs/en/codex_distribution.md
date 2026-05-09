# Codex Skill and MCP Distribution Guide

## Goal

When sharing the current RCA capability with other Codex users, the cleanest
delivery model is to split it into:

1. a shared RCA service
2. an installable Skill package

## Recommended Model

### Model A: Shared Service

Recommended team setup:

- deploy backend centrally
- deploy MCP centrally

This lets regular users only:

- install the Skill
- configure the MCP endpoint in Codex

They do not need their own:

- kubeconfig
- cluster credentials
- OpenSearch / Prometheus connection details

## In-cluster Service

This repository now includes:

- `deploy/rca-service`

It starts:

- backend
- mcp

inside the cluster and exposes them through NodePort:

- backend: `32180`
- mcp: `32181`

## Skill Package

Distribution path:

- `rca/integration/codex/skill/auto-inspection-rca`

This package removes the previous machine-specific absolute path dependency and
uses:

- `AUTO_INSPECTION_BACKEND_URL`
- `AUTO_INSPECTION_MCP_URL`

or the configured Codex MCP server.

## Plugin and Installer

The distributable bundle now also includes:

- plugin skeleton:
  `rca/integration/codex/plugins/auto-inspection-rca`
- marketplace entry:
  `rca/integration/codex/.agents/plugins/marketplace.json`
- installer script:
  `rca/integration/codex/scripts/install_auto_inspection_codex.ps1`

## Codex Configuration Example

```toml
[mcp_servers.autoInspectionRca]
url = "http://<RCA_MCP_HOST>:32181/mcp"
```

Example file:

- `rca/integration/codex/config.toml.example`

## Recommended Delivery Contents

Share these items:

1. `rca/`
2. `rca/integration/codex/skill/auto-inspection-rca/`
3. `rca/integration/codex/config.toml.example`

## User Installation Steps

1. Copy the Skill directory into:
   `%USERPROFILE%\.codex\skills\auto-inspection-rca`
2. Add the MCP config into:
   `%USERPROFILE%\.codex\config.toml`
3. Use it in Codex:

```text
Use auto-inspection-rca and list recent incidents
```

Or use the bundled installer script first, then merge the MCP config template.

## Recommended Next Improvements

- expose the service through Ingress / DNS
- add authentication
- provide a Skill install helper
- add remote MCP health checks
