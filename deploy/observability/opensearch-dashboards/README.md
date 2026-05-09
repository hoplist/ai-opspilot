# OpenSearch Dashboards Deployment

This folder deploys OpenSearch Dashboards in the `observability` namespace.

Notes:

- It uses the `opensearchproject/opensearch-dashboards:2` image line.
- Because the OpenSearch cluster disables the security plugin, the init container
  removes the `securityDashboards` plugin before startup.
- External access is exposed through NodePort `32091`.
