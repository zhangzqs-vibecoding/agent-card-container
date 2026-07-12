# Go 云端 HTTP Access Log 设计

## 目标

为 Go 云端所有 HTTP 请求增加安全、可检索的结构化 access log，使桌面端“测试连接”、Agent 生成和卡片库请求能在部署环境中直接追踪。

日志写入进程 stdout。systemd 部署由 journald 负责持久化、轮转、磁盘限额和查询；应用不自行创建或轮转日志文件。

## 日志事件

每个完成的 HTTP 请求写一行 JSON：

```json
{
  "timestamp": "2026-07-12T14:30:00.000Z",
  "event": "http_request",
  "requestId": "req_01",
  "method": "GET",
  "path": "/v1/cards",
  "status": 200,
  "durationMs": 3,
  "userId": "test-user",
  "remoteIp": "192.168.242.21"
}
```

字段规则：

- `timestamp` 使用 UTC RFC3339Nano。
- `requestId` 优先复用请求上下文中的现有 ID；入口没有 ID 时由中间件生成，并写入 `X-Request-ID` 响应头。
- `path` 仅记录规范化 URL path，不记录 query 和 fragment。
- `status` 捕获 handler 最终写出的 HTTP 状态；未显式写 header 时为 200。
- `durationMs` 为非负整数毫秒。
- `userId` 只在认证中间件已将稳定用户 ID 放入上下文时记录，否则省略。
- `remoteIp` 只解析直连 `RemoteAddr`，MVP 不信任客户端提供的 `X-Forwarded-For`。

`/healthz` 同样记录。panic 不在本功能中恢复；已有或未来的 recovery middleware 负责恢复时，access logger 仍应记录其最终 500 状态。

## 安全边界

日志禁止记录：

- `Authorization`、Cookie、Token、API Key 和任何请求头全集。
- 请求体、响应体、表单、上传内容。
- URL query string。
- 模型提示词、卡片状态、签名私钥或环境变量。

日志编码失败不得中断 HTTP 响应。日志 writer 写入失败只丢弃该条日志，不向客户端暴露内部错误。

## 组件与接入

- `internal/httpapi/accesslog.go` 提供 `AccessLogMiddleware`。
- 中间件依赖最小 `io.Writer`、时钟和 request ID 生成函数，便于确定性测试。
- `cmd/agentcard`/bootstrap composition root 将 stdout 注入中间件。
- 中间件包裹完整 cloud handler，确保健康、认证失败、404 和业务响应都被记录。
- 状态捕获 writer 保留 `http.Flusher` 等 handler 所需能力，不能破坏 SSE。

## 测试与验收

- TDD 验证默认 200、显式错误状态、请求 ID、路径去 query、远端 IP 和耗时。
- 验证日志中不存在 Authorization、Token、Cookie、query、请求体和响应体。
- 验证中间件不破坏 `http.Flusher` 和 SSE。
- 运行 `gofmt`、`go vet ./...`、`go test ./... -race` 与共享安全门禁。
- 重新构建静态 Linux 二进制并部署测试服务器。
- 实际请求 `/healthz`、有效 Token `/v1/cards` 和无效 Token `/v1/cards`，确认 journald 分别出现 200、200、401 的 JSON access log，且无凭据泄露。
