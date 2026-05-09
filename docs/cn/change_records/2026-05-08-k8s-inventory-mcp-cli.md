# 2026-05-08 Kubernetes 资产发现只读 API / MCP / CLI

## 背景

当前 RCA 平台已经能查日志、事件、Prometheus 资源、Evidence Pack、发布变更和深层观测证据，但 AI 在回答“当前有多少 Pod”“哪些 Pod 异常”“模糊找一下 mysql 相关资源”这类集群管理问题时，缺少一个直接的 Kubernetes 资产发现入口。

本次补齐只读 Kubernetes inventory 能力，让 Backend、MCP、CLI 都可以直接读取集群资源清单，便于 AI 快速做故障定位前的资源发现和范围收敛。

## Backend API

新增只读接口：

- `GET /api/k8s/namespaces`
- `GET /api/k8s/pods`
- `GET /api/k8s/pods/abnormal`
- `GET /api/k8s/workloads`
- `GET /api/k8s/services`
- `GET /api/k8s/resources/search`
- `GET /api/k8s/resources/count`
- `GET /api/k8s/cluster/overview`

支持参数：

- `namespace`：命名空间，空值、`all`、`*` 表示全命名空间。
- `q`：模糊搜索，匹配名称、命名空间、状态、节点、owner、labels、images、异常原因等字段。
- `status`：Pod 状态过滤，支持 `running`、`pending`、`failed`、`abnormal`、`not_ready`、`crashloop`、`imagepull` 等。
- `kind`：workload 类型，支持 `deployment`、`statefulset`、`daemonset`、`replicaset`、`all`。
- `kinds`：资源搜索范围，如 `pods,workloads,services,namespaces`。
- `limit`：返回数量上限，最大 500。

示例：

```bash
curl "http://<rca-backend>:18080/api/k8s/cluster/overview?limit=20"
curl "http://<rca-backend>:18080/api/k8s/resources/count"
curl "http://<rca-backend>:18080/api/k8s/pods/abnormal?namespace=observability"
curl "http://<rca-backend>:18080/api/k8s/resources/search?q=mysql&kinds=pods,services,workloads"
```

## MCP 工具

新增只读 MCP tools：

- `list_namespaces`
- `list_pods`
- `list_abnormal_pods`
- `list_workloads`
- `list_services`
- `search_k8s_resources`
- `count_k8s_resources`
- `cluster_overview`

这些工具只通过 RCA Backend 读取 Kubernetes API 或本地 `kubectl get ... -o json`，不执行 `apply/delete/patch/scale/restart` 等任何变更动作。

AI 可直接处理的问题示例：

- “当前集群有多少 Pod、多少 Service、多少 workload？”
- “列出 observability 命名空间异常 Pod。”
- “模糊搜索 mysql 相关的 Pod、Service、Deployment。”
- “哪些 workload ready 副本数低于期望副本数？”
- “给我一个集群概览，包括节点、命名空间和异常 Pod。”

## CLI

新增 CLI：

```bash
python k8s_inventory_cli.py --backend-url http://<rca-backend>:18080 overview
python k8s_inventory_cli.py --backend-url http://<rca-backend>:18080 count
python k8s_inventory_cli.py --backend-url http://<rca-backend>:18080 abnormal-pods -n observability
python k8s_inventory_cli.py --backend-url http://<rca-backend>:18080 search -q mysql --kinds pods,services,workloads
python k8s_inventory_cli.py --backend-url http://<rca-backend>:18080 pods --status abnormal --limit 50
```

也可以通过环境变量配置 Backend 地址：

```bash
export AUTO_INSPECTION_BACKEND_URL=http://<rca-backend>:18080
python k8s_inventory_cli.py overview
```

## RBAC

RCA ServiceAccount 仍保持只读边界。

内网部署包中 `auto-inspection-rca-read` ClusterRole 增加：

- core API `nodes` 的 `get/list/watch`

原因：`cluster_overview` 需要读取 Node Ready 状态、kubelet 版本、内核版本和节点地址。其他资源仍沿用原有只读权限：

- core：`pods`、`pods/log`、`services`、`events`、`namespaces`、`configmaps`
- apps：`deployments`、`statefulsets`、`daemonsets`、`replicasets`

## 变更文件

- `auto_inspection/k8s_inventory.py`
- `auto_inspection/dashboard_server.py`
- `auto_inspection/backend_client.py`
- `auto_inspection/auto_inspection_mcp.py`
- `auto_inspection/k8s_inventory_cli.py`
- `k8s_inventory_cli.py`
- `deploy/intranet-bundle/gitops-manifests/source/deploy/rca-service/clusterrole.yaml`
- `deploy/intranet-bundle/gitops-manifests/source/yaml/rca-service/clusterrole.yaml`
- `deploy/intranet-bundle/gitops-manifests/clusters/test/observability/auto-inspection-rca/clusterrole.yaml`
- `C:/Users/Administrator/.codex/skills/auto-inspection-rca/SKILL.md`

## 验证记录

- 语法检查通过：

```bash
python -m py_compile auto_inspection/k8s_inventory.py auto_inspection/dashboard_server.py auto_inspection/backend_client.py auto_inspection/auto_inspection_mcp.py auto_inspection/k8s_inventory_cli.py k8s_inventory_cli.py
```

- CLI help 验证通过：

```bash
python k8s_inventory_cli.py --help
python k8s_inventory_cli.py pods --help
```

## 部署说明

本次已经完成代码和内网部署包 YAML 调整，但运行中的 RCA Backend/MCP 需要重新构建并替换 RCA 镜像后才会包含这些新 API 和 MCP 工具。

2026-05-09 已重新构建并推送 RCA 镜像：

- 镜像：`docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260509-k8s-inventory`
- digest：`sha256:bc90ed791f0e5882be0e8ea576d2049bb9fd5b0d9f1fc5714bd77b0eece429e2`
- 本地 Docker Desktop 访问该仓库 HTTPS 失败，已通过 node206 的 insecure registry 配置完成推送。
- 内网部署包中的 `auto-inspection-rca` Deployment image tag 已同步更新为 `20260509-k8s-inventory`。

下一步部署：

```bash
kubectl apply -k /opt/observability/gitops-manifests/clusters/test/observability/auto-inspection-rca
kubectl rollout status deployment/auto-inspection-rca -n observability
```

验证：

```bash
curl "http://<rca-backend>:18080/api/k8s/cluster/overview?limit=10"
curl "http://<rca-backend>:18080/api/k8s/pods/abnormal?namespace=observability"
```
