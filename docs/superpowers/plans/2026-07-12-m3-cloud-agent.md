# M3 Go 云端与生成 Agent 实施计划

设计依据：docs/superpowers/specs/2026-07-12-agent-card-container-design.md。

目标：实现可恢复的生成会话、Native-first 编码 Agent、OCI CodeCard 构建验证、Ed25519 签名发布及桌面端生成、预览、安装链路。

架构：Go 保持模块化单体，api 与 worker 共用领域和 repositories。PostgreSQL 是会话、事件、任务和版本的事实来源；对象存储保存源码、日志、预览和制品。模型、沙箱、签名器、对象存储均以窄接口注入，测试不依赖真实云服务。

### Task 1：共享云合同与生成状态机

- [x] 建立 OpenAPI、SSE event schema、错误 envelope 和 Go 一致性测试。
- [x] 先写失败测试，覆盖固定状态转换、非法转换、取消和 ready 终态。
- [x] 实现 GenerationSession、Message、Event 核心领域模型；Job 与 CardVersion 在任务和发布步骤补齐。

### Task 2：Repository、任务租约与 API/SSE

- [ ] 已提供确定性的事务内存实现、PostgreSQL schema 和 SKIP LOCKED 查询；database/sql repository 与 pgx driver 待网络恢复后接入。
- [x] 实现 SKIP LOCKED 领取语义、租约超时重排、幂等发布和取消，并通过并发 race 测试。
- [x] 实现设计中的全部 REST 端点、统一错误、OIDC RS256/JWKS Bearer 鉴权边界。
- [ ] SSE 支持 Last-Event-ID 重连，断线不取消任务。

### Task 3：Native-first Agent 与验证器

- [x] 定义 modelprovider 接口，密钥只从环境注入，日志不记录 prompt 或密钥。
- [x] 实现结构化需求摘要、确认、auto、native、web 选择和禁止能力拒绝。
- [x] NativeCard 生成并通过共享 schema、catalog、限制验证。
- [x] 失败最多自动修复三轮，随后产生稳定失败 code。

### Task 4：CodeCard OCI 沙箱与签名发布

- [x] 定义 sandbox 接口与 Docker OCI adapter：无网络、非 root、只读根、2 CPU、2 GiB、5 分钟、白名单输出。
- [ ] 固定 TypeScript、Preact、Vite 模板并接通类型检查、测试、构建和静态 bundle policy；隔离浏览器加载、依赖许可与漏洞扫描待补。
- [x] 生成规范化 manifest、文件 hash、Ed25519 签名和 agentcard ZIP。
- [ ] 已实现对象存储接口、内存 adapter、短期下载元数据及不可变 CardVersion 幂等发布；真实 S3 adapter 待补。

### Task 5：桌面端生成与端到端验收

- [ ] 桌面端已接入生成创建、确认、SSE 重连和取消；预览、自动安装和版本历史界面待补。
- [ ] NativeCard 与 CodeCard 均经过签名安装链路进入 workspace。
- [ ] 运行 Go test、race、vet、Flutter 全量测试、沙箱与篡改回归。
- [ ] 记录真实模型、PostgreSQL、S3、Docker 和 Windows E2E 环境门禁。
