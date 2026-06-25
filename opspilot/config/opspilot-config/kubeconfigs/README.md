# Kubeconfigs

This directory is reserved for internal test-stage plaintext remote cluster
kubeconfigs.

Use file names like:

```text
<cluster>.kubeconfig
```

Do not use `.yaml` or `.yml` suffixes here. OpsPilot recursively loads YAML
files as runtime config documents, while kubeconfig files are consumed only by
the Kubernetes client through `clusters/*.yaml` `kubeconfig_path`.

Only GitLab administrators should have read access to this repository.
