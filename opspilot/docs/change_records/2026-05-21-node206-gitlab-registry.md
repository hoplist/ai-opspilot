# 2026-05-21 node206 GitLab Registry

## Change

Enabled the built-in GitLab Container Registry for the node206 GitLab test
environment over HTTP.

```text
GitLab Web:      http://192.168.48.206:8929
GitLab Registry: http://192.168.48.206:5050
```

The GitLab Docker Compose service now exposes:

```text
8929:8929
5050:5050
2224:22
```

Registry configuration added to the GitLab Omnibus config:

```ruby
registry_external_url 'http://192.168.48.206:5050'
gitlab_rails['registry_enabled'] = true
gitlab_rails['registry_host'] = '192.168.48.206'
gitlab_rails['registry_port'] = '5050'
registry_nginx['listen_port'] = 5050
registry_nginx['listen_https'] = false
```

## Verification

`http://192.168.48.206:5050/v2/` returns `401 Unauthorized` with Docker
Registry response headers, which is the expected unauthenticated Registry API
response.

## Follow-up

Because this Registry uses HTTP, Docker/containerd clients that push or pull
images from it must trust it as an insecure registry.

