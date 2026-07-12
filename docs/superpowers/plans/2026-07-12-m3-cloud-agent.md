# M3 Go 云端与生成 Agent 实施计划

设计依据：docs/superpowers/specs/2026-07-12-agent-card-container-design.md。

目标：实现可恢复的生成会话、Native-first 编码 Agent、OCI CodeCard 构建验证、Ed25519 签名发布及桌面端生成、预览、安装链路。

架构：Go 保持模块化单体，api 与 worker 共用领域和 repositories。PostgreSQL 是会话、事件、任务和版本的事实来源；对象存储保存源码、日志、预览和制品。模型、沙箱、签名器、对象存储均以窄接口注入，测试不依赖真实云服务。

### Task 1：共享云合同与生成状态机

- [x] 建立 OpenAPI、SSE event schema、错误 envelope 和 Go 一致性测试。
- [x] 先写失败测试，覆盖固定状态转换、非法转换、取消和 ready 终态。
- [x] 实现 GenerationSession、Message、Event 核心领域模型；Job 与 CardVersion 在任务和发布步骤补齐。

### Task 2：Repository、任务租约与 API/SSE

- [x] 实现事务内存与 PostgreSQL repository，使用 pgx driver 持久化 session/message/event、租约 job 和不可变 CardVersion，并由嵌入式 migration 管理 schema。
- [x] 实现 SKIP LOCKED 领取语义、租约超时重排、幂等发布和取消，并通过并发 race 测试。
- [x] 实现设计中的全部 REST 端点、统一错误、OIDC RS256/JWKS Bearer 鉴权边界。
- [x] SSE 支持历史补发、持续事件推送和 Last-Event-ID 重连，断线只释放订阅、不取消任务。

### Task 3：Native-first Agent 与验证器

- [x] 定义 modelprovider 接口，密钥只从环境注入，日志不记录 prompt 或密钥。
- [x] 实现结构化需求摘要、确认、auto、native、web 选择和禁止能力拒绝。
- [x] NativeCard 生成并通过共享 schema、catalog、限制验证。
- [x] 失败最多自动修复三轮，随后产生稳定失败 code。

### Task 4：CodeCard OCI 沙箱与签名发布

- [x] 定义 sandbox 接口与 Docker OCI adapter：无网络、非 root、只读根、2 CPU、2 GiB、5 分钟、白名单输出。
- [ ] 固定 TypeScript、Preact、Vite 模板并接通类型检查、测试、构建和静态 bundle policy；隔离浏览器加载、依赖许可与漏洞扫描待补。
- [x] 生成规范化 manifest、文件 hash、Ed25519 签名和 agentcard ZIP。
- [x] 实现内存与 S3 兼容对象存储 adapter、条件写入、短期签名下载及不可变 CardVersion 幂等发布。

### Task 5：桌面端生成与端到端验收

- [ ] 桌面端已接入生成创建、确认、持续 SSE、取消、卡片库、不可变版本历史、制品下载验签和 NativeCard 自动/指定版本安装；独立预览待补。
- [ ] NativeCard 与 CodeCard 均经过签名安装链路动态进入 workspace；CodeCard 已创建独立 RuntimeSession 并接入 WebView2 adapter，仍待 Windows 实机门禁。
- [ ] 运行 Go test、race、vet、Flutter 全量测试、沙箱与篡改回归。
- [ ] 已用 PostgreSQL 17 与 MinIO 实容器验证独立 API/worker 共享 session、job、catalog 和制品；真实模型、Docker CodeCard 与 Windows E2E 门禁待记录。
