# P1-C CodeCard 生产链路实施计划

> 执行要求：逐任务使用 TDD；每个行为先确认 RED，再做最小实现；每个任务独立验证和小步提交。Windows 专属项只准备执行包，不伪造设备证据。

**目标：** 在 Linux/headless 环境完成 CodeCard 固定评测、受控构建镜像、真实浏览器、离线/RPC 和签名发布链路，并把 Windows WebView2 剩余项明确隔离。

**架构：** 保留 Go 模块化单体、现有 CodingAgent、DockerBuilder 和 Flutter LocalRuntimeServer。评测 fixture 是质量事实来源；模板内增加窄 JS SDK；Linux Chromium 作为 Web 语义证据；OCI digest 是 builder 身份。不得引入通用 Agent 框架、消息中间件或第二套 RPC。

## Task 1：冻结固定评测合同

**文件：**

- Create: `services/cloud/internal/agent/testdata/codecard_eval_cases.v1.json`
- Create: `services/cloud/internal/agent/codecard_eval_test.go`
- Create: `services/cloud/internal/agent/codecard_eval.go`

- [x] RED：fixture 不是精确 20 项、五类不是各 4 项、未知字段、重复 ID、空语义断言或非法能力时失败。
- [x] 定义严格 decoder/validator，固定总量、类别、prompt 长度、语义和安全断言。
- [x] 定义报告器：16/20、每类 3/4、60 次调用、输入 300k、输出 160k、费用 5 美元和超时门槛。
- [x] 单元测试覆盖边界值和“总分通过但单类别失败”。
- [x] 提交：`test: freeze CodeCard evaluation contract`。

## Task 2：强化 CodeCard prompt 与源码合同

**文件：**

- Modify: `services/cloud/internal/agent/native_prompt.go`
- Modify: `services/cloud/internal/agent/native_prompt_test.go`
- Modify: `services/cloud/internal/agent/agent.go`
- Modify: `services/cloud/internal/agent/agent_test.go`
- Create: `tooling/codecard-template/src/agentcard.ts`
- Create: `tooling/codecard-template/scripts/agentcard.test.mjs`

- [x] RED：prompt 未声明可用 Preact API、本地 SDK、禁用模式和完整文件合同；源码包含不可打印字符或非法 UTF-8 时未拒绝。
- [x] 增加最小版本化 JS SDK，提供 context、storage 和 capability invoke，不暴露任意 fetch 包装。
- [x] prompt 明确离线、自包含、无外部资源、无动态代码、无新增依赖，并给出 API 示例。
- [x] 源码 decoder 拒绝非法 UTF-8、NUL、超限行和高风险生成路径。
- [x] 修复反馈只保留稳定诊断，继续限制为 2 KiB。
- [x] 提交：`feat: define bounded CodeCard source API`。

## Task 3：固定并验证 builder 镜像供应链

**文件：**

- Modify: `tooling/sandbox-image/Dockerfile`
- Modify: `tooling/sandbox-image/README.md`
- Create: `.github/workflows/codecard-builder.yml`
- Modify: `tooling/security/validate-workflows.dart`
- Modify: `tooling/security/validate-workflows_test.dart`

- [x] RED：workflow 使用未固定 action/base image、PR 发布、权限过宽或仅 tag 输出时失败。
- [x] 固定 Node 基础镜像 digest和 pnpm；加入 OCI labels 与非 root health-free builder。
- [x] workflow 对 PR 构建/扫描/集成测试，对 develop/手工任务登录 GHCR 并发布 digest。
- [x] 扫描结果 high/critical 非零时失败，保存 digest 和非敏感 SBOM 证据。
- [x] 本地构建镜像并运行 `run-sandbox-integration.sh`。
- [x] 提交：`ci: publish verified CodeCard builder image`。

## Task 4：建立真实模型 CodeCard gate

**文件：**

- Create: `services/cloud/internal/agent/codecard_live_test.go`
- Modify: `.github/workflows/deepseek-live.yml`

- [x] RED：付费 gate 可无限调用、缺少明确确认、未绑定 secret env 或能绕过固定 fixture 时失败。
- [x] 复用生产 CodingAgent 和真实 Docker builder，逐例输出非敏感统计。
- [x] 强制 3 calls/case、60 calls/suite、token、费用和 90 分钟上限；不得自动无限重跑。
- [x] 报告失败类别只使用稳定枚举，不保存 prompt、源码或上游正文。
- [ ] 在明确临时环境变量下注入真实模型凭据运行一次；没有凭据时保持 opt-in skip。
- [x] 提交：`test: add bounded live CodeCard quality gate`。

## Task 5：Linux Chromium 的运行时与离线门禁

**文件：**

- Create: `tooling/codecard-browser/package.json`
- Create: `tooling/codecard-browser/pnpm-lock.yaml`
- Create: `tooling/codecard-browser/tests/runtime.spec.ts`
- Create: `tooling/codecard-browser/fixture_server.dart`
- Modify: `tooling/security/run-security-gate.sh`

- [x] RED：外部导航、跨随机 origin RPC、缺 token、未声明能力、超限请求、过量调用或 session 关闭后请求未被拒绝。
- [x] 用 Dart Runtime Server/测试 adapter 服务真实 bundle，用 Linux Chromium 执行页面和交互断言。
- [x] 验证 storage、context、RPC、事件、CSP、外网失败、reload 和云端断开后继续使用。
- [x] 验证旧 session/token 在应用级重建后失效，新 session 从持久化状态恢复。
- [x] 纳入安全门，固定浏览器/工具版本，所有等待使用条件轮询和总超时。
- [x] 提交：`test: prove CodeCard browser offline runtime`。

## Task 6：真实持久化发布与恢复

**文件：**

- Modify: `services/cloud/internal/bootstrap/runtime_test.go`
- Modify: `services/cloud/internal/worker/production_recovery_test.go`
- Modify: `tooling/persistence/run-p0b-gate.sh`

- [x] RED：真实 CodeCard 构建、上传、版本入库任一点失败会重复版本或改变 artifact hash。
- [x] 在 PostgreSQL/MinIO 门禁中生成、构建、签名、上传并远程式下载 CodeCard。
- [x] 独立验证 manifest、SHA-256、Ed25519、入口文件和 dependency policy。
- [x] lease 过期恢复不得重新调用模型或创建第二个 version。
- [x] 提交：`test: prove persistent CodeCard publication recovery`。

## Task 7：Windows 执行包与文档收口

**文件：**

- Create: `packaging/windows/run-codecard-device-gate.ps1`
- Create: `docs/verification/windows-codecard-evidence-template.md`
- Create: `docs/verification/p1c-codecard-production.md`
- Modify: `docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md`
- Modify: `docs/verification/m4-acceptance.md`

- [x] PowerShell 静态测试覆盖参数、commit/hash 绑定、超时、退出码和敏感信息清理。
- [x] 设备模板覆盖 WebView2、三个 Surface、DPI/IME、离线重启、storage 和单卡崩溃隔离。
- [x] 运行 Go race、Flutter analyze/test、Node、安全门、Docker、Chromium 和持久化门禁。
- [x] 执行敏感信息与构建/容器残留审计。
- [x] 自动化范围完整通过后标记 `HEADLESS AUTOMATION PASS / LIVE QUALITY NOT RUN / WINDOWS DEVICE NOT RUN`。
- [x] 提交：`docs: record P1-C headless evidence`，不 push（`b994849`）。
