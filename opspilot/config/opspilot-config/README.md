# OpsPilot Config

This repository is the human-maintained runtime configuration source for the
test OpsPilot deployment.

OpsPilot loads YAML files recursively from the repository root. Keep these
directories small and explicit:

- `settings/`: platform defaults such as GitLab URL, GitOps project, quality
  runner metadata, and default cluster.
- `credentials/`: credential catalog metadata. Passwords are allowed only for
  internal test datasource access and are redacted by OpsPilot APIs.
- `clusters/`: cluster and GitOps placement metadata.
- `kubeconfigs/`: optional plaintext kubeconfig files for internal test
  clusters. Access is controlled by the private GitLab repository membership.
  OpsPilot APIs must not print these file contents.
- `datasources/`: Prometheus, Elasticsearch/OpenSearch, APISIX, and app log
  datasource definitions.
- `agents/`: read-only node agent endpoints.
- `assets/`: advisory-only network zones, planned JumpServer/Prometheus asset
  sources, and optional static assets.
- `services/`: service catalog, runtime mapping, release mapping, and log
  correlation hints.
- `probes/`: HTTP probe evidence policies. These decide which optional evidence
  sources are queried after a probe and whether missing sources warn, skip, or
  become required.
- `topology/`: region and network path hints.

Validate before pushing:

```powershell
go run ./opspilot/cli --output human config validate --dir ./opspilot/config/opspilot-config
```

Runtime should report:

```text
Config: source=file valid=true
```

## Plaintext Cluster Kubeconfigs

For the current internal test stage, new remote clusters can be managed with a
plain kubeconfig file in this private repository:

```text
kubeconfigs/<cluster>.kubeconfig
```

The cluster catalog should reference the synced runtime path:

```yaml
clusters:
  - name: gz-inner
    environment: test
    region: guangzhou
    network_zone: inner
    business_line: collaboration
    business: Guangzhou collaboration test cluster
    owner: ops
    kubernetes_mode: remote
    kubeconfig_path: /etc/opspilot/config/current/kubeconfigs/gz-inner.kubeconfig
    kube_context: gz-inner
```

Required ownership fields for new clusters:

- `environment`
- `region`
- `network_zone`
- `business_line`
- `business`
- `owner`

Only GitLab administrators should have repository access. CLI and API responses
may show the kubeconfig path and ownership metadata, but must not return the
kubeconfig content.
