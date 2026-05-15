# 2026-05-15 OpsPilot MVP Core / CLI

## 背景

OpsPilot 已确定按新方案完全重构。本次开始实现 MVP 版本，目标是先打通确定性 CLI 到只读 Backend 的最小排障链路。

## 本次实现

新增 `opspilot-core` Python 标准库 MVP：

- `GET /api/health`
- `GET /api/inventory/overview`
- `GET /api/k8s/pods`
- `GET /api/k8s/logs/pod`
- `GET /api/context/pod`
- `GET /api/diagnose/pod`

新增 `opspilot` CLI MVP：

- `python -m opspilot.cli schema`
- `python -m opspilot.cli inventory overview`
- `python -m opspilot.cli k8s pods`
- `python -m opspilot.cli k8s logs pod`
- `python -m opspilot.cli context pod`
- `python -m opspilot.cli diagnose pod`

新增 `opspilot-skill` 草案：

- `opspilot/skill/SKILL.md`

新增 MVP 镜像和部署骨架：

- `opspilot/Dockerfile`
- `deploy/opspilot/core/deployment.yaml`
- `deploy/opspilot/core/service.yaml`
- `deploy/opspilot/core/kustomization.yaml`
- `deploy/opspilot/rbac/namespace.yaml`
- `deploy/opspilot/rbac/serviceaccount.yaml`
- `deploy/opspilot/rbac/clusterrole.yaml`
- `deploy/opspilot/rbac/clusterrolebinding.yaml`
- `deploy/opspilot/rbac/kustomization.yaml`

## Kubernetes 访问模式

`opspilot-core` 支持两种模式：

- 集群内运行时，使用 ServiceAccount token 调 Kubernetes API。
- 本机运行时，fallback 到 `kubectl`。

## 只读边界

MVP 仅实现只读能力：

- 查询资源。
- 查询事件。
- 按需读取短窗口 Pod 日志。
- 生成 Pod Evidence Pack。
- 生成基础诊断摘要。

不实现：

- `exec`
- `attach`
- `portforward`
- `delete`
- `patch`
- `rollout restart`
- `scale`

## 验证

执行：

```bash
python -m compileall -q opspilot tests/test_opspilot_mvp.py
python -m unittest tests.test_opspilot_mvp
python -m opspilot.cli schema
```

并完成一次本地 HTTP 健康检查：

```bash
python -m opspilot.core --host 127.0.0.1 --port 18082
curl http://127.0.0.1:18082/api/health
```

## 后续

- 补完整 `opspilot-mcp` 适配层。
- 增加 Prometheus 查询代理。
- 增加 ELK 查询代理。
- 后续工具链满足后，再把 `opspilot-core` 在线接口迁移到 Go。
