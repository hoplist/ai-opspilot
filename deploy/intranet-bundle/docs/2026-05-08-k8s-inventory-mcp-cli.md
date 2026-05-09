# 2026-05-08 RCA Kubernetes 资产发现能力

本次内网包补齐 RCA Backend/MCP 的只读 Kubernetes inventory 能力，目标是让 AI 可以直接回答资源发现类问题，例如：

- 当前集群有多少 Pod、Service、workload。
- 哪些 Pod 异常。
- 模糊搜索某个业务名对应的 Pod、Service、workload。
- 查看节点、命名空间和异常 Pod 概览。

## 新增 Backend API

- `GET /api/k8s/namespaces`
- `GET /api/k8s/pods`
- `GET /api/k8s/pods/abnormal`
- `GET /api/k8s/workloads`
- `GET /api/k8s/services`
- `GET /api/k8s/resources/search`
- `GET /api/k8s/resources/count`
- `GET /api/k8s/cluster/overview`

## 新增 MCP tools

- `list_namespaces`
- `list_pods`
- `list_abnormal_pods`
- `list_workloads`
- `list_services`
- `search_k8s_resources`
- `count_k8s_resources`
- `cluster_overview`

全部工具只读，不执行 Kubernetes 变更。

## CLI 示例

```bash
export AUTO_INSPECTION_BACKEND_URL=http://<rca-backend-nodeport-or-svc>:18080
python k8s_inventory_cli.py overview
python k8s_inventory_cli.py count
python k8s_inventory_cli.py abnormal-pods -n observability
python k8s_inventory_cli.py search -q mysql --kinds pods,services,workloads
```

## RBAC

`auto-inspection-rca-read` ClusterRole 增加只读 `nodes get/list/watch`，用于 `cluster_overview` 展示节点 Ready 状态、内核版本、kubelet 版本等信息。

仍然不包含 `create/update/patch/delete` 等写权限。

## 生效方式

代码能力需要重新构建 RCA 镜像后生效。仅 apply YAML 只能更新 RBAC，不能让旧镜像出现新接口。

2026-05-09 已重新构建并推送：

- 镜像：`docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260509-k8s-inventory`
- digest：`sha256:bc90ed791f0e5882be0e8ea576d2049bb9fd5b0d9f1fc5714bd77b0eece429e2`
- 部署包中的 RCA Deployment image tag 已更新为 `20260509-k8s-inventory`。

部署顺序：

1. 执行 `kubectl apply -k <bundle>/gitops-manifests/clusters/test/observability/auto-inspection-rca`。
2. 等待 rollout 完成。
3. 验证新接口：

```bash
kubectl rollout status deployment/auto-inspection-rca -n observability
curl "http://<rca-backend>:18080/api/k8s/cluster/overview?limit=10"
curl "http://<rca-backend>:18080/api/k8s/pods/abnormal?namespace=observability"
```
