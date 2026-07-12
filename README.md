# Agent Card Container

Agent Card Container 是一个 Windows-first、Flutter 驱动的桌面卡片容器。它支持：

- NativeCard：由受控声明式协议驱动的 Flutter 原生卡片。
- CodeCard：由本地 Dart Runtime Server 提供资源和能力 RPC 的离线 Web 卡片。
- 主工作区、透明悬浮层和独立窗口。
- Go 云端 AI、编码 Agent、构建验证、签名及版本管理。

## 目录

~~~text
apps/desktop       Flutter 桌面端
services/cloud     Go 云端 API 与 worker
contracts          JSON Schema、OpenAPI 和跨语言 fixture
tooling            CodeCard 模板与沙箱
docs               设计与实施计划
~~~

顶层设计见 docs/superpowers/specs/2026-07-12-agent-card-container-design.md。

## 快速验证

项目已按 M0–M4 拆分实现。自动化与实机证据分开记录：

- `docs/verification/m3-production-integration.md`：PostgreSQL、S3 和 Docker
  sandbox 的真实 adapter 集成证据。
- `docs/verification/m4-acceptance.md`：当前总验收状态及未执行的
  Windows/macOS 设备门禁。
- `docs/verification/linux-m4-evidence.json`：Linux 普通窗口构建的
  机器可读证据。

~~~bash
cd apps/desktop && flutter analyze && flutter test
cd ../../services/cloud && go vet ./... && CGO_ENABLED=1 go test ./... -race
cd ../.. && sh tooling/security/run-security-gate.sh
~~~

## 本地云端运行

Go 服务可以在本地以 API 和 worker 一体模式做端到端验证。环境变量名见
services/cloud/.env.example；真实密钥只通过进程环境提供，不得提交填充后的文件。

~~~bash
cd services/cloud
go run ./cmd/agentcard
~~~

服务启动必须提供模型、认证和 Ed25519 签名配置。默认 `all`
模式在未配置持久化时使用内存 repository，仅用于单进程本地开发。
生产 `api`/`worker` 拆分部署必须同时配置 `AGENTCARD_DATABASE_URL`
和 `AGENTCARD_S3_ENDPOINT`；程序会执行 PostgreSQL migration，并使用
S3 兼容对象存储发布签名制品。

后端使用 Go `log/slog` 将一行一个 JSON 事件写入 stdout。systemd 部署可用：

~~~bash
journalctl -u agent-card-cloud.service -f -o cat
journalctl -u agent-card-cloud.service -o cat | grep '"event":"http_request"'
~~~

稳定事件包含 `http_request`、`service_started`、`generation_job_started`、
`model_request_started`、`sandbox_build_started`、`artifact_publish_started` 及其
completed/failed 结果。日志只记录关联 ID、状态、耗时、runtime 和稳定错误类别，
不记录 Authorization、Token、Cookie、query、请求/响应正文、prompt、模型输出、
构建输出或签名材料。

付费 DeepSeek 烟测默认关闭，只从环境读取凭据：

~~~bash
cd services/cloud
AGENTCARD_DEEPSEEK_LIVE=1 go test ./internal/modelprovider \
  -run TestDeepSeekLiveGeneratesValidNativeCard -v
~~~

该命令还需通过进程环境安全注入 `AGENTCARD_MODEL_API_KEY`、
`AGENTCARD_MODEL` 和 `AGENTCARD_MODEL_BASE_URL`。不要把真实凭据写入
`.env`、shell history 或 Git。

## 桌面端运行

详细的平台依赖、云端连接环境变量和离线行为见
`apps/desktop/README.md`。桌面包也可在“设置 → 云端服务”中测试并保存 Go
后端连接；Access Token 只进入操作系统安全凭据库，保存后重启应用生效。
客户端不保存模型 API key；它只连接 Go 云端。

## GitHub CI 与 Windows 便携包

`develop` 的 Windows workflow 会上传经过解压回验的 x64 便携 ZIP。
下载后直接解压，双击 `agent_card_desktop.exe` 启动。`v*` tag
会同时创建包含 ZIP 和 `SHA256SUMS.txt` 的 GitHub prerelease。

当前 CI 产物没有 Authenticode 签名，Windows SmartScreen 可能提示风险。
ZIP 不包含模型、云端或签名凭据。DeepSeek live test 只能手动运行，
并从受保护的 `deepseek-live` environment 读取 secret。
