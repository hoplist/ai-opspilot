# 从 auto_inspection 迁移到 OpsPilot

## 迁移原则

- 不推倒现有能力。
- 不影响当前 RCA Backend/MCP 运行。
- 先隔离文档和命名，再逐步调整代码、CLI、部署和镜像。
- 新能力优先进入 `OpsPilot` 命名空间和文档目录。

## 阶段路线

### P0：文档隔离

- 新建 `docs/opspilot/`。
- OpsPilot 新文档只写入该目录。
- `docs/cn/` 保留为历史和当前实现资料。

### P1：命名统一

规划命名：

- `opspilot`
- `opspilot-core`
- `opspilot-mcp`
- `opspilot-skill`
- `opspilot-console`
- `opspilot-worker`

### P2：CLI 收敛

新增或改造 CLI：

```bash
opspilot schema
opspilot inventory overview
opspilot k8s pods --status abnormal
opspilot k8s logs pod --namespace prod --pod xxx
opspilot context pod --namespace prod --pod xxx
opspilot diagnose pod --namespace prod --pod xxx
```

旧 CLI 可以保留一段时间作为兼容入口。

### P3：Backend API 契约

先在当前 Python Backend 中整理稳定 API：

- Inventory。
- Pod logs on demand。
- Prometheus metrics。
- ELK query。
- Release context。
- Evidence Pack。

### P4：Go opspilot-core

按 `backend-go-plan.md` 逐步迁移在线高频接口。

### P5：部署清单隔离

后续新增：

```text
deploy/opspilot/
  core/
  mcp/
  console/
  worker/
  optional/
```

默认不带 OpenSearch、MinIO、MySQL、eBPF。

## 本阶段完成定义

- OpsPilot 文档独立。
- CLI/Skill/Backend 的职责边界清晰。
- 后续代码改造有稳定目标。
