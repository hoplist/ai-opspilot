# opspilot-mcp

MCP adapter for OpsPilot.

The MCP layer should expose only read-only tools by default and should call
`opspilot-core` or `opspilot` CLI instead of accessing Kubernetes, Prometheus, or
ELK directly.

Initial tools:

- `inventory_overview`
- `list_servers`
- `list_clusters`
- `list_abnormal_pods`
- `get_pod_logs`
- `get_pod_context`
- `diagnose_pod`
- `top_nodes`
- `top_containers`
