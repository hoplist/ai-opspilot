# 2026-06-24 HTTP Probe Evidence Pack

## 目标

为 OpsPilot 增加受控 HTTP 探测链路，用于排查 nginx/APISIX 黑盒转发、Header 改写、资源加载失败、上游服务异常等问题。流程固定为：

1. OpsPilot 生成 `probe_id`。
2. 发起 HTTP/HTTPS 请求。
3. 保存请求/响应摘要。
4. 按 `host + uri + 时间窗口 + status + user_agent/probe_id` 查询 APISIX/nginx 日志。
5. 按 `时间窗口 + path + traceId + probe_id/关键字` 查询应用日志。
6. 可选读取 Pod 状态和资源。
7. 输出 Evidence Pack，供 AI 继续 RCA。

## 本阶段落地

- 新增 `internal/httpprobe`：只允许 HTTP/HTTPS，支持 GET/POST/HEAD/OPTIONS，默认 10 秒超时，最大 30 秒。
- 自动注入：
  - `X-OpsPilot-Probe-Id: <probe_id>`
  - `User-Agent: OpsPilot-Probe/<probe_id>`
- 请求/响应 Header 会脱敏 `Authorization/Cookie/Set-Cookie/token/secret/api-key`。
- 响应正文默认不返回；显式 `--include-response` 时只返回预览，默认 16KiB，最大 64KiB。
- 新增 API：`POST /api/probe/http`。
- 新增 CLI：`probe http`。
- 复用现有 `logsearch.CorrelateRequest`，扩展 `probe_id/user_agent/keyword` 作为弱关联条件。
- 可选传 `--namespace/--pod` 时补 Kubernetes Pod context 和 Prometheus 单 Pod 指标。
- 输出 `evidence_pack`，可用 `--persist` 写入服务端 Evidence Pack 存储。
- 审计策略中 `/api/probe/http` 归类为 `read_only`：它虽然使用 POST 承载表单参数，但不修改集群、代码、配置或远端系统。

## 使用示例

```powershell
.\scripts\opspilot.ps1 --output human probe http `
  --url http://demo.test.tpo.xzoa.com/api/health `
  --service-index demo-api-* `
  --service-uri-field message `
  --persist
```

带 Pod 证据：

```powershell
.\scripts\opspilot.ps1 --output human probe http `
  --url http://demo.test.tpo.xzoa.com/api/health `
  -n demo-test `
  --pod demo-api-xxxx `
  --service-index demo-api-* `
  --persist
```

二次复查日志关联：

```powershell
.\scripts\opspilot.ps1 evidence request `
  --host demo.test.tpo.xzoa.com `
  --uri /api/health `
  --probe-id probe-xxxx `
  --trace-id trace-abc `
  --keyword trace-abc `
  --service-index demo-api-*
```

## 风险边界

- 不提供任意 shell/curl 执行。
- 不支持非 HTTP 协议。
- 不保存完整响应体。
- 不泄露敏感 Header。
- 日志源缺失时不阻塞 probe，只在 Evidence Pack 中标记缺失证据。
- 该能力用于只读排查；高风险修复仍必须走 plan/dry-run 和审计边界。

## 验证

本地验证项：

- `go test ./internal/httpprobe ./internal/logsearch ./core ./cli`
- `go vet ./...`
- `git diff --check`
- `go run ./cli --output human --backend-url <live-backend> probe http --url <safe-url>`

发布验证项：

- 分支 Pipeline 通过。
- 主干 Pipeline 通过。
- Argo CD 同步 `opspilot-core`。
- 线上 CLI probe 返回 `probe_id`、HTTP 状态、日志关联强度和 Evidence Pack。
