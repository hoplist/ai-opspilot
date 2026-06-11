# 2026-06-11 Node Disk Read-only Agent

## Background

OpsPilot 已经能通过 Prometheus/node-exporter 查看集群 node 的 CPU、内存和挂载点使用率，也能通过 `opspilot-agent` 只读查看 node206 Docker 容器状态、日志和 stats。

这次补的是“磁盘占用归因”：当 `metrics filesystems` 只能告诉我们某个挂载点使用率高时，OpsPilot 还需要只读查看宿主机上哪些目录、Docker 日志或 Docker 可回收对象可能在占用空间。

## Decision

- 集群 node 常规监控仍走 Prometheus + node-exporter。
- `opspilot-agent` 只补宿主机目录级证据，不替代 node-exporter。
- agent 只提供 GET 接口，不执行清理、不删除文件、不 truncate 日志、不运行 `docker prune`、不修改 Docker 配置。
- 高风险或受控变更只输出 plan 和最小验证方式。

## Implementation

新增只读接口：

```text
GET /api/host/disk?host=node206&limit=20&depth=2
```

新增 CLI：

```powershell
opspilot host disk --host node206 --output human
opspilot host cleanup plan --host node206 --output human
```

返回证据：

- 文件系统容量：基于 agent 白名单路径的 `statfs`。
- Top paths：仅扫描 `OPSPILOT_AGENT_DISK_ALLOWED_PATHS`，默认 `/var/lib/docker,/var/log,/opt,/data`。
- Docker disk：读取 Docker API `/system/df`。
- 容器日志大小：读取允许容器的 Docker inspect `LogPath`，通过只读 hostRoot 映射 stat 文件大小。
- cleanup plan：只读建议，包含风险等级、证据、建议、最小验证、执行边界。
- capabilities 和 server-side `monitoring-expert` skill registry 已补充
  `host disk`，便于自然语言/AI 路由到只读磁盘归因。
- GitLab CI 增加 `opspilot-agent` Linux binary 和 agent image 构建，推送
  `opspilot-agent:<commit>` 与 `opspilot-agent:main`。
- `Dockerfile.agent` 基础镜像对齐 core 使用的
  `m.daocloud.io/docker.io/library/alpine:3.20`，避免内网 BuildKit 直连
  Docker Hub。
- node206 外部 compose 改为使用 GitLab Registry 的
  `192.168.48.206:5050/platform/opspilot/opspilot-agent:main`。
- node206 agent token 不写入 Git。compose 从 node206 本机 `.env` 读取
  `OPSPILOT_AGENT_TOKEN`；core Deployment 增加可选
  `opspilot-node-agent-secrets`，用于注入
  `OPSPILOT_NODE_AGENT_TOKENS=node206=<token>`。
- 凭证台账新增 `opspilot-node-agent-secrets`，只记录用途、权限和轮换边界。

agent 新增配置：

```text
OPSPILOT_AGENT_HOST_ROOT=/host
OPSPILOT_AGENT_DISK_ALLOWED_PATHS=/var/lib/docker,/var/log,/opt,/data
OPSPILOT_AGENT_DISK_MAX_DEPTH=2
OPSPILOT_AGENT_DISK_TOP_LIMIT=20
```

node206 compose 需要只读挂载：

```yaml
- /proc:/host/proc:ro
- /var/lib/docker:/host/var/lib/docker:ro
- /var/log:/host/var/log:ro
- /opt:/host/opt:ro
- /data:/host/data:ro
```

node206 agent 是外部 Docker Compose 服务，不归 Argo CD 管理。发布代码后，
需要在确认窗口内人工或受控执行 compose pull/up 才会让新 agent 接口生效。
如果启用 token，必须同时保证 node206 `.env` 和 node200
`opspilot-node-agent-secrets` 中的 token 一致。

## Cluster Node Monitoring

集群里的 node 不建议优先部署 agent。默认方式：

```powershell
opspilot metrics nodes --source node200-k8s --output human
opspilot metrics filesystems --source node200-k8s --output table
opspilot inspect cluster --source node200-k8s --output human
```

这些命令依赖 Prometheus + node-exporter，适合看：

- node CPU
- node 内存
- rootfs / mountpoint 剩余量
- Pod Top CPU/Memory
- 异常 Pod 对节点资源的影响

只有需要回答“具体哪个宿主机目录打爆磁盘”时，才在对应 node 上部署只读 agent，并把路径加入白名单。

## Risk Boundary

允许：

- 读取白名单目录大小。
- 读取 Docker `/system/df`。
- 读取允许容器的 Docker inspect 日志路径。
- 生成 cleanup plan。

禁止：

- 删除文件或目录。
- truncate 容器日志。
- 运行 `docker prune`。
- 修改 Docker daemon 配置。
- 重启容器或节点服务。
- 扫描白名单外路径。

## Minimum Validation

- `go test ./opspilot/agent ./opspilot/internal/nodeagent ./opspilot/core ./opspilot/cli`
- `go vet ./opspilot/...`
- `opspilot host disk --host node206 --output human`
- `opspilot host cleanup plan --host node206 --output human`

如果 cleanup plan 建议配置 Docker log rotation，人工执行后最小验证：

```powershell
opspilot host disk --host node206 --output human
```

确认：

- 容器 `log_options.max-size` 已存在。
- 对应日志文件不再无界增长。
- 文件系统 available bytes 有改善或保持稳定。
