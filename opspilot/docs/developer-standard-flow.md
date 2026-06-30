# Developer standard flow

## 开发者只需要做什么

开发者只在应用源码仓库里工作：

```text
tpo/apps/<group>/<project>/<service>
```

当前 demo 和无身份测试上传默认走临时位置：

```text
tpo/sandbox/devex/<service>
```

当用户身份、团队归属和权限还没有完全接入时，OpsPilot 会把上传视为
test-only sandbox 工作，并先给出计划：

```powershell
opspilot repo upload-plan --repo . --name my-demo-api --output human
opspilot repo upload --repo . --name my-demo-api --confirm --output human
```

默认计划：

```text
GitLab project: tpo/sandbox/devex/<repo>
Namespace: sandbox
GitOps path: clusters/test/apps/sandbox/<repo>
Release scope: test-only
```

`repo upload-plan` 是只读计划。`repo upload --confirm` 会做代码预检查、
创建或复用 sandbox GitLab 项目，并推送当前已提交的 `HEAD`。它不会自动
提交本地改动、修改 GitOps、变更 Kubernetes 或配置网关路由。

## 推送后发生什么

```text
developer push
-> GitLab CI preflight/code-precheck
-> BuildKit builds image
-> image is pushed to GitLab Registry
-> CI updates GitOps desired state
-> Argo CD reconciles the cluster
-> OpsPilot shows release evidence
```

开发者不需要知道 Pod 名、GitOps 路径、Argo CD Application 名、Dockerfile
细节或 namespace 命名规则。

## 开发者不应该直接改什么

- `tpo/deploy/gitops-manifests`
- Kubernetes Secrets
- Argo CD Applications
- Registry tags
- Runtime credentials
- Cluster-level RBAC、CNI、StorageClass、ingress controller

## 仓库不规范时怎么做

让 OpsPilot 检查或修复：

```text
帮我把这个仓库接入标准发布
帮我检查这个仓库为什么不能发布
帮我生成 Dockerfile 和部署 YAML
帮我把 preflight 报错修成可发布状态
```

OpsPilot 应产出或更新：

```text
Dockerfile
.gitlab-ci.yml
opspilot.service.yaml
deploy/k8s/*
```

## 中间件怎么处理

开发者声明意图，不直接维护长期密码：

```yaml
middleware:
  mysql:
    mode: shared
```

平台创建服务级凭证并注入运行时 Deployment。本地调试使用临时 debug
凭证，不复用长期服务账号密码。

## 怎么看发布状态

用 OpsPilot CLI，或通过 AI 调用 OpsPilot：

```text
查看 skillshub-api 发布状态
查看 skillshub-api 最近一次发布为什么失败
查看 skillshub-api 当前 Pod 是否正常
查看 skillshub-api 是否可以回滚
```

OpsPilot 应报告：

- GitLab pipeline 状态。
- BuildKit job 证据。
- Registry image tag。
- GitOps desired image。
- Argo CD sync 和 health。
- Kubernetes rollout 和 Pod 状态。
- 日志和指标，前提是数据源已配置。
- 缺失证据及其影响，数据源未接入时不伪装成功。

## 可选测试网关

未来可以让前置网关把 `*.test.tpo.xzoa.com` 指向测试入口机，但这是外部
网关配置，不应阻塞标准发布链路。

当前只记录期望路由：

```text
*.test.tpo.xzoa.com -> test ingress/APISIX/Nginx entry -> sandbox or app namespace Service
```

即使网关未接入，生成应用仍应能通过 Pod、Service、release、metrics 证据
被 OpsPilot 排查。
