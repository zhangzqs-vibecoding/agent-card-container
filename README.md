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

## 本地云端运行

Go 服务可以在本地以 API 和 worker 一体模式做端到端验证。环境变量名见
services/cloud/.env.example；真实密钥只通过进程环境提供，不得提交填充后的文件。

~~~bash
cd services/cloud
go run ./cmd/agentcard
~~~

默认 all 模式使用内存 repository。生产拆分 api/worker 部署仍需要 M3 计划中的
PostgreSQL 与 S3 adapter。

付费 DeepSeek 烟测默认关闭，只从环境读取凭据：

~~~bash
cd services/cloud
AGENTCARD_DEEPSEEK_LIVE=1 go test ./internal/modelprovider \
  -run TestDeepSeekLiveGeneratesValidNativeCard -v
~~~
