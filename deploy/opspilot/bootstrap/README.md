# OpsPilot Namespace Bootstrap

This package copies the platform GitLab Registry pull secret from
`opspilot/gitlab-registry-pull` into generated namespaces labelled
`opspilot.io/managed=true`.

The source Secret should use a GitLab credential with `read_registry` only and
enough project/group visibility to pull all managed service images.

Service repositories and GitOps application directories must not contain
registry credentials. Namespace bootstrap is a platform responsibility.
