# Fluent Bit Deployment

This folder deploys:

- `fluent-bit-logs`: DaemonSet for container logs
- `fluent-bit-events`: singleton Deployment for Kubernetes Events

Both ship records to the in-cluster OpenSearch service.

The logs pipeline now normalizes and enriches these common fields before write:

- `message`
- `message_normalized`
- `severity`
- `logger`
- `namespace`
- `pod`
- `container`
- `node`
- `service`
- `exception_type`
- `exception_message`
- `stack_language`

It also strips ANSI color sequences and extracts common exception signatures for
Java, Python, and Go style errors.
