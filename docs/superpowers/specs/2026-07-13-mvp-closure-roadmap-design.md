# Agent Card Container MVP 收口路线图设计

状态：**已确认，实施中**
记录日期：2026-07-13（Asia/Shanghai）
最近核对：2026-07-15（Asia/Shanghai）
代码基线：`855e118`

## 1. 文档目的

本文档定义 Agent Card Container 从“自动化覆盖较完整的技术原型”推进到“可由真实用户在 Windows 上稳定使用的 MVP”的收口顺序、范围和验收标准。

本文档不新增 M5，也不改变现有 NativeCard、CodeCard、Capability Broker、Flutter 桌面端或 Go 模块化单体的顶层架构。`2026-07-12-agent-card-container-design.md` 仍是架构事实来源；本路线图只安排尚未闭环的 M0–M4 工作。若实施中需要改变安全边界、卡片协议或运行时选择，必须先更新顶层设计。

路线图采用“真实闭环优先”：第一波并行证明真实模型能在 Linux/headless 环境生成、校验、签名和下载 NativeCard 制品，以及持久化测试环境能稳定发布网络可达制品；第二波再在 Windows 实机完成下载、验签、安装、交互和离线重启。随后关闭任务可靠性、版本生命周期、CodeCard 生产链路和可信发行。不得以 CI 编译成功、fake adapter 测试或一次性容器测试代替真实用户链路验收。

顶层设计要求先通过 M0 Windows 技术闸门再继续完整实现，但当前仓库已经在缺少 M0 设备证据的情况下形成了 M1–M4 自动化骨架。本路线图是对这一历史偏差的补救，不追认 M0 已通过：P0-C 完成前，不再以现有自动化结果批准新的平台依赖型扩展，也不得作出发布就绪结论。

## 2. 当前基线

项目已经具备以下基础：

- Flutter 桌面壳、工作区、Agent Studio、NativeCard 渲染器、CodeCard 本地 Runtime Server、多 Surface 抽象和 Capability Broker。
- Go API/worker 模块化单体、生成会话、SSE、模型 provider、NativeCard 校验、CodeCard OCI 沙箱、签名发布和版本查询接口。
- PostgreSQL、S3/MinIO、OIDC、Ed25519、Docker sandbox 等生产 adapter 及集成测试。
- Windows、Linux、macOS CI，Windows portable ZIP，以及 Go、Dart、TypeScript 跨语言安全门禁。
- stdout JSON 结构化日志和 journald 部署验证。

截至最近核对，原始缺口的状态如下。这里的“已关闭”只表示对应代码或自动化缺口已经关闭，不代表包含 Windows 实机证据的完整产品闸门通过。

| # | 缺口 | 状态 | 当前事实与下一闸门 |
|---:|---|---|---|
| 1 | 真实 DeepSeek 产品验收证据 | **HEADLESS PASS** | `808b435`、`0a11ef3`、`41f9894` 已形成脱敏的 3/3 签名垂直链路和 16/20 真实质量门证据；Windows 产品链路仍属 P0-C。 |
| 2 | 补充需求可能未进入 worker | **已关闭（headless）** | `394660b` 已持久化不可变确认需求快照，三例真实垂直测试证明两条追加需求在 worker 前后保持一致。 |
| 3 | NativeCard 上下文不完整、质量不可量化 | **已关闭（headless）** | catalog 派生语义、严格校验、Prompt/传输边界、有界重试和固定质量门均已通过自动化及真实模型验证。 |
| 4 | 远程环境仍可能使用内存 repository 和 `memory://` | **开放** | 生产 adapter 已存在，但 P0-B 的 PostgreSQL/MinIO 持久化部署、重启恢复和远程下载尚未验收。 |
| 5 | Windows WebView2、多窗口、悬浮、DPI、IME 和性能 | **开放，设备依赖** | 当前只能保留 `NOT RUN`；必须由 Windows 参考设备生成绑定 commit 的证据。 |
| 6 | 工作区拖动、缩放、碰撞和布局编辑 | **HEADLESS PASS / DEVICE NOT RUN** | 12 列网格拖缩、first-fit 碰撞避让、300ms 持久化、重启恢复和失败回滚已通过 Flutter/Linux 自动化；Windows DPI、多屏和实际输入仍未执行。 |
| 7 | 同卡迭代、升级和回滚 | **开放** | 归入 P1-A；需要 `baseCardId`/`baseVersionId`、能力差异、同 schema 状态复用及异 schema 备份/重置流程。MVP 不执行 Agent 生成的状态迁移。 |
| 8 | worker heartbeat、事务、重试与发布幂等 | **HEADLESS PASS** | 原子确认/入队、租约 heartbeat/fencing、稳定发布身份、基础设施有限重试及五阶段部分失败恢复已通过 race 与 PostgreSQL/MinIO 门禁；Windows 消费仍属设备范围。 |
| 9 | CodeCard 正式 builder 和真实生成链路 | **HEADLESS AUTOMATION PASS / LIVE、DEVICE NOT RUN** | 受控 builder、固定评测合同、Linux Chromium 离线/RPC、签名发布及 PostgreSQL/MinIO 恢复已通过；真实 CodeCard 20 例质量门和 Windows WebView2 仍未执行。 |
| 10 | Windows/macOS 可信发行 | **开放** | Authenticode、正式安装器、WebView2 缺失引导、macOS 签名和 notarization 归入 P2。 |

因此，当前状态应描述为“自动化骨架和安全边界基本成形，MVP 真实闭环尚未通过”，不能描述为发布就绪。

### 2.1 阶段执行看板

| 工作包 | 当前状态 | 已有交付物 | 下一可交付物 | 完整通过的外部依赖 |
|---|---|---|---|---|
| P0-A 真实 DeepSeek NativeCard | **HEADLESS PASS** | 确认快照、catalog 派生语义、严格 validator、有界重试、3/3 签名垂直链路、16/20 真实质量门和脱敏证据 | 保持回归门稳定，等待 P0-C 消费同一协议和制品 | Windows 设备证据属于 P0-C，不反向阻塞 headless 结论 |
| P0-B 持久化测试环境 | **LOCAL HEADLESS PASS / REMOTE NOT RUN** | PostgreSQL/MinIO、`/readyz`、服务与数据依赖重启、网络下载、独立验签和成对备份恢复均通过可重复 Docker 门禁 | 在固定测试服务器复跑并证明 Windows 参考设备网络可达 | 可用测试服务器；Windows 设备消费属于 P0-C |
| P0-C Windows NativeCard 闸门 | **未执行** | Windows 构建/便携包和设备门禁脚本 | 在参考设备完成远程下载、验签、安装、三类 Surface、状态保留、DPI/IME/多屏矩阵 | P0-A `HEADLESS PASS`、P0-B `PASS` 和 Windows 11 x64 参考设备 |
| P1-A 工作区与版本生命周期 | **工作区布局 HEADLESS PASS；其余实施中** | 12 列拖动/缩放、确定性碰撞避让、300ms SQLite 持久化、重启恢复、失败回滚和空状态入口 | 实例复制/删除/卸载与数据选择；同卡升级和回滚 | Windows 布局体验属设备证据；其余 headless 工作无外部依赖 |
| P1-B worker 可靠性 | **HEADLESS PASS** | 原子确认/入队、90s/30s lease heartbeat、owner fencing、稳定 card/version、1s/2s 基础设施退避、取消传播及真实 PostgreSQL/MinIO crash recovery | 保持回归稳定；Windows 产品消费证据随 P0-C/P1-C 收集 | Linux/headless 不再有外部依赖 |
| P1-C CodeCard 生产链路 | **HEADLESS AUTOMATION PASS / LIVE、DEVICE NOT RUN** | 受控 builder、20 例固定质量合同、本地 JS SDK、Linux Chromium 离线/RPC、签名发布和真实 PostgreSQL/MinIO 恢复 | 执行真实 CodeCard 质量门；在 Windows 参考设备完成离线 WebView2 矩阵 | 临时模型凭据与价格配置；Windows 11 x64 参考设备 |
| P2 可信发行与 macOS | **未开始** | 静态安装器/entitlement 结构和跨平台 CI | Windows 签名发行、干净机升级卸载；macOS 签名、notarization 和设备闸门 | P0/P1 功能链路与 Windows M0 设备闸门通过；M4 完整通过是本阶段退出条件 |

主要事实依据：

- `docs/verification/m3-production-integration.md`：生产 adapter 已通过一次性集成测试，但真实 DeepSeek 和 Windows WebView2 不在通过范围内。
- `docs/verification/m4-acceptance.md`：Windows 设备矩阵、性能、签名安装器和 macOS 技术闸门的当前状态。
- `services/cloud/internal/generation/service.go` 与 `services/cloud/internal/worker/worker.go`：会话消息、确认入队和 worker 输入边界。
- `services/cloud/internal/modelprovider/http_provider.go`：当前模型请求和响应解析边界。
- `services/cloud/internal/bootstrap/runtime.go`：内存或 PostgreSQL/S3 repository 组合及 CodeCard sandbox 开关。
- `apps/desktop/lib/src/workspace/workspace_screen.dart`：当前工作区交互入口和布局呈现。
- `apps/desktop/lib/src/cloud/card_install_coordinator.dart`：下载、验签、安装和实例落位流程。
- `apps/desktop/lib/src/adapters/in_app_webview_port.dart`：桌面 WebView 平台能力边界。

## 3. 路线选择

评估过三种推进方式：

### 3.1 闭环优先——采用

先完成真实 NativeCard 垂直链路和持久化，再关闭 Windows 实机、可靠性、版本和 CodeCard 缺口。

优点是最早暴露模型输出、协议、签名下载、WebView2 和系统窗口等真实风险；每个阶段都能产生用户可验证的增量。缺点是前期必须同时处理少量云端、客户端和部署问题。

### 3.2 Agent 智能优先——不采用

先引入 Eino、Claude Agent SDK、多模型路由或复杂 tool loop，再补产品链路。

该方案可能快速增加演示能力，但当前主要问题是需求上下文、合同注入、评测和执行链路缺失。更换 Agent 框架不会自动解决这些问题，反而会扩大依赖面和调试范围。

### 3.3 公测基础设施优先——不采用

先实现账号、计费、限流、市场、自动更新和多租户，再验证生成质量与 Windows 运行时。

该方案会在核心价值尚未证明前投入大量外围工作，不符合 KISS 和 YAGNI。

## 4. 总体目标与非目标

### 4.1 MVP 收口目标

- 用户能在 Windows 客户端用中文描述需求、补充要求并确认最终需求。
- Go Agent 使用完整确认上下文调用真实 DeepSeek，生成符合合同的 NativeCard 或 CodeCard。
- 生成结果经过确定性校验、受限构建、Ed25519 签名和持久化发布。
- Windows 客户端能下载、验签、安装、布局、重新打开并在断网后继续使用纯本地卡片。
- 同一张卡片可以生成新版本、预览、升级和回滚，能力扩大时必须重新授权。
- worker 在超时、重试、取消、进程重启和部分失败下不会重复发布或永久卡死。
- Windows M0/M4 实机矩阵和真实性能基线有可复核的机器证据。

### 4.2 本轮非目标

- 公开卡片市场、付费交易、审核后台和开发者分成。
- 组织、多租户管理、团队协作和企业权限模型。
- 多模型路由、模型自动竞价或 Agent 框架迁移。
- 微服务拆分、Kafka、Kubernetes 或独立工作流平台。
- Linux Wayland 悬浮层、全局快捷键和完整桌面环境兼容承诺。
- 在 Windows 风险关闭前全面扩展 macOS 产品功能。

## 5. 分阶段路线图

### 5.1 P0-A：真实 DeepSeek NativeCard Linux/headless 闸门

这是下一份实施计划的唯一首要目标。

#### 数据流

1. 客户端创建生成会话。
2. 用户追加的每条消息进入会话事实记录。
3. 确认操作生成不可变的“确认需求快照”，其中包含初始需求、补充消息、target、locale 和服务端固定能力候选集。
4. worker 只读取该快照，不再只读取初始 `Prompt`。
5. Agent 根据 runtime 使用由 contracts 派生的精简协议说明、组件目录、动作目录和安全规则调用模型。
6. 模型输出进入现有确定性 validator；单个任务最多调用模型三次，即首次生成加最多两次携带稳定校验错误类别的修复。
7. 成功结果生成不可变签名制品，通过下载接口交给独立 headless 验证器复核 hash、manifest、签名和 NativeCard 内容。

#### Agent 决策

- MVP 保留现有 `modelprovider.Provider` 和有界“生成—校验—修复”循环，总调用上限为三次，不引入新的 Agent 框架。
- runtime 选择继续遵循 NativeCard 优先原则。选择器必须输出稳定理由，不能让模型直接决定安全边界。
- 模型输入中的卡片协议必须从 contracts 或同一份受控目录派生，不能在 provider 中维护另一份可能漂移的协议事实。
- P0-A 使用服务端固定能力候选集，当前为 `storage` 和 `window.manageSelf`；模型只能在确认快照的候选范围内请求，不能自行扩大权限。按需求动态推导能力及其授权 UX 不在 P0-A 内实现，后续必须先形成独立策略和测试再放宽。

#### 质量评测

建立至少 20 个固定评测需求，覆盖计时器、待办、信息面板、简单表单、统计卡片、离线状态和明确禁止能力。评测记录只保存非敏感测试需求、结果类别、attempt、耗时、token 数和稳定校验错误类别，不保存真实用户内容。

P0-A 的 `HEADLESS PASS` 条件：

- 追加需求在单元测试和真实模型请求中均可证明生效。
- 20 个固定 NativeCard 需求中至少 16 个能在最多三次模型调用内生成合法结果。
- 至少 3 个代表性卡片完成真实“生成—确定性校验—签名—下载—headless 独立复核”链路。
- 失败时 API 或测试消费者收到可理解的稳定错误，不暴露上游响应、模型输出或凭据。
- 真实模型凭据只通过临时进程环境注入，不进入仓库、环境文件、脚本、命令参数、日志或测试证据。

P0-A 不包含 Windows 安装、交互、状态保留或离线重启；这些项目在 P0-C 完成前统一标记为 `DEVICE NOT RUN`。

### 5.2 P0-B：持久化测试环境

P0-B 可以与 P0-A 的本地 TDD 并行，但必须在远程 Windows 端到端测试前完成。

测试服务器使用现有 Go 模块化单体和一体模式，部署 PostgreSQL 与 MinIO/S3 持久卷。MVP 不因为生产 adapter 已存在而提前拆分 API/worker 服务；只有在多实例并发验收需要时再启用现有分离模式。

测试环境必须具备：

- PostgreSQL 持久化 generation session、message、job、card 和 version 元数据。
- MinIO/S3 持久化签名制品，并提供 Windows 参考设备所在网络可达的 HTTPS 或受控局域网下载地址。
- 稳定的签名 key ID；私钥由服务器安全配置提供，不进入仓库或日志。
- 服务重启、数据库重启和对象存储重启后的恢复验证。
- 最小备份与恢复演练，至少证明元数据和制品可以成对恢复。

P0-B 通过条件：

- Go 服务重启后生成会话、job、卡片和版本仍可查询。
- 独立验证器能从远程 URL 下载制品，并通过 SHA-256、manifest 和 Ed25519 校验；Windows 客户端行为留给 P0-C。
- 数据库与对象存储任一不可用时 `/readyz` 失败，但进程 `/healthz` 仍能表达存活状态。
- 不再向远程客户端返回 `memory://` 制品 URL。

### 5.3 P0-C：Windows NativeCard 实机闸门

P0-A 达到 `HEADLESS PASS` 且 P0-B 通过后，在指定 Windows 11 x64 参考设备上运行真实产品链路，而不是仅运行 widget test 或 fake adapter。

必须验证：

- Agent Studio 从设置好的云端创建、补充、确认和跟踪生成会话。
- 客户端从 P0-B 环境下载制品并通过 SHA-256、manifest 和 Ed25519 校验；篡改或未签名制品必须拒绝。
- 生成结果可预览并明确安装；失败、取消和 SSE 重连均有用户反馈。
- NativeCard 能放入主工作区、独立窗口和悬浮层。
- 卡片状态、窗口位置和布局在应用重启、云端断开和机器重启后保留。
- 中文 IME、Tab/焦点、剪贴板权限、外部链接确认、150% DPI 和多显示器移动符合设计。
- 生成按钮、空状态入口和失败恢复不再存在空回调。

P0-C 的证据必须绑定精确 commit、Windows/Flutter/WebView2 版本和设备信息。任何未执行场景继续标记 `NOT RUN`，不得由 CI 构建结果替代。

### 5.4 P1-A：工作区编辑和卡片生命周期

工作区补齐用户实际管理卡片所需的最小编辑能力：

- 拖动、缩放、吸附网格、碰撞处理和可恢复自动落位。
- 300ms 防抖持久化，写入失败时回滚 UI 或明确提示。
- 分离为独立窗口、切换悬浮层、移回工作区且状态不丢失。
- 复制实例、删除实例、卸载卡片及“保留数据/删除数据”的明确选择。

版本生命周期增加：

- 创建 generation session 时可选 `baseCardId` 和 `baseVersionId`。
- Agent 读取上一版声明或源码与本次修改要求。
- 保持稳定 `cardId`，生成新的 `versionId` 和递增显示版本。
- 安装新版前展示能力差异、状态 schema 兼容性和回滚点。
- `stateSchemaVersion` 相同时复用原实例状态；版本不同时先备份旧状态，再让用户选择保留旧版或安装新版并重置状态。MVP 不执行 Agent 生成的状态迁移。
- 版本切换和权限更新必须作为一个可恢复操作；失败后旧版本及其状态继续可用。
- 如需超出 P0-A 固定能力候选集，先独立定义需求到 capability 的服务端策略、安装授权 UX 和拒绝/回退测试，模型本身仍无权扩大候选集。

### 5.5 P1-B：worker 可靠性与幂等

保持 PostgreSQL job store，不引入消息中间件。

必须增加：

- 确认会话和 job 入队的事务一致性，避免永久 `queued` 会话。
- lease heartbeat；租约长度不能小于模型和沙箱阶段的最坏可接受执行时间。
- 可重试错误与永久错误分类，针对 429、上游 5xx、网络超时和临时存储故障使用有上限的指数退避。
- job 级稳定 card/version 标识和发布幂等键，避免重试产生重复版本。
- 模型调用、沙箱构建和发布阶段的 context 取消传播。
- 对“制品已上传但版本未入库”“版本已入库但会话未 ready”等部分失败的恢复测试。

P1-B 通过条件是：并发 worker、进程强制重启、租约超时和故障注入测试中，不出现重复发布、孤儿永久占用、错误终态覆盖成功结果或无法取消的构建。

### 5.6 P1-C：CodeCard 生产链路与 Windows 闸门

CodeCard 复用现有无网络、只读 root、非 root、资源限制和固定基础镜像 digest 的 OCI 沙箱。不得放宽沙箱来换取模型输出兼容性。

实施范围：

- builder 镜像由受控 CI 构建、扫描并发布到受控 registry，运行配置固定到 digest。
- Agent 获得精简的 CodeCard 模板 API、允许文件列表和依赖策略。
- 至少覆盖自由画板、小游戏、可视化、复杂交互和纯离线工具五类固定评测。
- CodeCard 独立设计必须在实施前锁定评测用例总数、通过率、单例调用上限和费用上限；没有预先确定的门槛不得宣称质量通过。
- 产物在 Windows WebView2 中通过随机 localhost 子域加载，本地 JavaScript、storage 和 RPC 正常运行。
- 断网和应用重启后继续可用，WebView 崩溃只影响单卡并触发现有恢复/隔离策略。
- 重跑导航、私网访问、RPC 越权、ZIP、制品篡改和资源超限攻击矩阵。

### 5.7 P2：可信发行和第二平台

只有 P0/P1 功能链路和 Windows M0 平台设备闸门通过、M4 自动化基线可用后，才推进。Windows M4 完整验收是 P2 的退出条件，不是进入条件：

- 修复性能采样对象和场景，使其测量真实 Flutter 进程、OS Surface 和 WebView，而不是测试壳或 PowerShell 进程。
- 完成 Windows Authenticode、Inno Setup、WebView2 Evergreen 缺失引导、升级和卸载清理。
- 在干净 Windows 设备上复验安装、SmartScreen、升级、回滚和卸载。
- 增加 macOS 运行时 capability probe，启用真实 WKWebView adapter，完成 App Sandbox entitlement、签名、notarization 和窗口行为闸门。
- Linux 继续只承诺普通窗口；托盘、热键或悬浮能力不可用时必须按 feature 降级，不能阻止主窗口启动。

## 6. 关键路径与并行波次

| 波次 | 工作包 | 前置条件 | 可并行项 | 相对工作量 |
|---:|---|---|---|---|
| 1 | P0-A 真实 DeepSeek NativeCard | 临时模型凭据 | P0-B | M |
| 1 | P0-B PostgreSQL/MinIO | 测试服务器资源 | P0-A | S–M |
| 2 | P0-C Windows NativeCard 闸门 | P0-A `HEADLESS PASS`、P0-B `PASS` | P1-B | M–L |
| 3 | P1-A 工作区与版本生命周期 | P0-C 暴露的问题已分类 | P1-B | L |
| 2 | P1-B worker 可靠性 | P0-B `PASS` | P0-C、P1-A | M–L |
| 4 | P1-C CodeCard 生产链路 | P0-A、P0-B、P1-B | 部分 P1-A Windows UI 工作 | M–L |
| 5 | P2 发行与 macOS | P0/P1 功能链路、Windows M0 设备闸门、M4 自动化基线 | Windows 发行与 macOS 闸门可并行 | L |

波次表示关键依赖，不表示所有工作必须串行。P0-A 与 P0-B 从第一波并行；P1-B 在 P0-B 通过后即可开始，并可与 P0-C/P1-A 并行。只有明确列出的前置条件全部满足，工作包才可形成通过结论。

每个工作包必须拥有独立实施计划、TDD 红绿证据、受影响测试、部署或设备证据和小步提交。不得创建一份跨越全部工作包的一次性实现计划。

## 7. 协议与组件边界

- `contracts` 继续作为 Go、Dart、TypeScript 的唯一协议事实来源。
- generation 的确认需求快照、基于版本生成字段和能力差异若进入 API，必须先修改 schema/OpenAPI/fixture，再生成或同步各语言 adapter。
- `modelprovider` 只处理供应商兼容协议、超时和响应解析；不得承担业务选择、权限审批或制品校验。
- `agent` 负责 runtime 选择、受控模型上下文、修复循环和 deterministic validation。
- `worker` 负责编排 job、幂等标识、构建和发布，不拼装 UI 逻辑。
- Flutter 平台插件继续只出现在 adapter 层；workspace、surface 和 card 生命周期通过端口依赖平台能力。
- NativeCard 和 CodeCard 的所有系统能力仍只能通过 Capability Broker。

## 8. 错误处理与可观测性

在现有 JSON 日志基础上补充最小可运营信号：

- `/healthz` 只表示进程存活；新增 `/readyz` 检查当前模式所需的 PostgreSQL、S3 和 worker 依赖。
- 记录 generation 成功率、校验 attempt 分布、各阶段耗时、队列深度、租约过期、重试次数、token 数和估算成本。
- 指标和日志只使用关联 ID、模型名、runtime、阶段、状态和稳定错误类别。
- 不记录真实 prompt、模型输出、卡片状态、Authorization、API Key、Cookie、签名材料或上游原始错误正文。
- 客户端错误分为配置、认证、网络、生成、校验、构建、发布、下载、验签和安装类别，并提供用户可执行的恢复动作。

MVP 先使用现有结构化日志、聚合查询和少量指标端点；在单实例测试阶段不引入完整 tracing 平台。

## 9. 安全与凭据

- 真实模型凭据只通过临时进程环境变量注入；不得写入仓库、`.env`、脚本、命令参数、systemd 环境文件、日志、诊断包或验收文档。
- 客户端永远不保存模型 API Key，只保存访问 Go 云端所需的 Token，并使用操作系统安全凭据库。
- 测试服务器继续使用受限开发用户和 Token；对外公测前必须切换 OIDC、TLS、限流和审计策略。
- 签名私钥与模型凭据分离；生产阶段使用托管 signer 或等价受控密钥设施。
- CodeCard 生成源码只进入一次性受限 workspace，构建后销毁；构建网络保持关闭。
- capability 扩大、网络域变化和敏感能力必须在安装或升级前由用户重新确认。

## 10. 测试与证据策略

### 10.1 自动化层

- Go：`gofmt`、`go vet ./...`、`CGO_ENABLED=1 go test ./... -race`。
- Flutter：`flutter analyze`、`flutter test`，并为新增桌面交互增加 controller/widget 测试。
- contracts：Go、Dart、TypeScript 继续消费同一 fixture。
- sandbox：固定镜像 digest 的真实 Docker 集成和恶意输入矩阵。
- 安全门禁：运行 `tooling/security/run-security-gate.sh`。

### 10.2 外部服务层

- 真实 DeepSeek 测试默认关闭，仅在明确的人工 gate 中以临时环境变量启用。
- 付费测试设置调用次数和 token 上限；失败不得自动无限重试。
- 评测结果记录模型名、日期、用例版本和非敏感统计，不将偶然单次成功作为通过。

### 10.3 设备层

- Windows 证据必须由真实应用、真实 WebView2 和真实 OS 窗口产生。
- 性能采集必须确认采样 PID 是目标 Flutter 应用，并保存原始采样和统计摘要。
- 每份证据绑定 commit SHA、设备、OS、DPI、WebView2、Flutter 和构建制品 hash。
- `NOT RUN` 永远不是 PASS；自动化模拟不能替代必须的设备场景。

## 11. 发布与回滚

- P0 阶段只部署测试环境，不向公众开放。
- 每次后端部署采用原子二进制替换，先验证 hash，再重启并检查 `/healthz`、`/readyz` 和关键日志。
- 数据库 migration 必须向前兼容当前客户端；涉及不可逆数据变化时先做备份恢复演练。
- builder 镜像按 digest 部署，保留上一已验证 digest 供回滚。
- 客户端版本升级失败时保留上一安装包和本地卡片数据；卡片升级失败时保留旧版本与状态快照。
- 对外公测必须在 Windows 实机矩阵、持久化恢复、真实生成评测、签名安装器和凭据方案全部通过后单独决策。

## 12. MVP 收口完成定义

只有同时满足以下条件，现有设计中的 MVP 才能从 withheld 改为通过：

1. 真实 DeepSeek NativeCard 和 CodeCard 固定评测达到各自门槛，并有可复核统计。
2. 完整确认需求进入 Agent，生成、校验、构建、签名、发布和安装链路可追踪。
3. PostgreSQL/S3 持久化、备份恢复和远程制品下载通过。
4. Windows NativeCard、CodeCard、主工作区、独立窗口、悬浮层、离线重启和状态保留通过。
5. job heartbeat、重试、幂等、取消和部分失败恢复通过。
6. 同卡版本迭代、能力差异确认、同 schema 状态复用、异 schema 备份/重置选择、升级和回滚通过；MVP 不执行 Agent 生成的状态迁移。
7. Windows 设备安全矩阵和真实性能预算通过。
8. 签名安装器、WebView2 缺失引导、升级和卸载清理通过。
9. 所有证据绑定同一候选 release commit；没有被错误标记为 PASS 的 `NOT RUN` 项。

## 13. 下一步

P0-A 已完成 Linux/headless 范围的独立实施计划，状态为 `HEADLESS PASS`。当前按“关键路径 + 并行泳道”继续推进：

1. P0-B 已达到 `LOCAL HEADLESS PASS`；在固定测试服务器复跑相同门禁并证明 Windows 参考设备可达后，才更新为远程 `PASS`。
2. P1-B 已达到 `HEADLESS PASS`；保持扩展后的持久化门禁稳定，Windows 消费证据不反向替代或扩大该结论。
3. P0-B 为 `PASS` 后，在 Windows 11 x64 参考设备执行 P0-C。此前 Windows 安装、交互、状态保留和断网重启保持 `DEVICE NOT RUN`。
4. P1-A 的具体修正由 P0-C 设备结果排序；没有 Windows 设备证据时只推进与平台无关的 controller、repository 和布局算法测试。
5. P1-C 必须等待 P0-A、P0-B 和 P1-B 的关键安全与可靠性边界稳定；P2 的进入与退出条件按第 5.7 节执行。

P0-B、P0-C 和所有 P1/P2 工作分别在其前置条件满足后创建独立设计补充或实施计划，不在 P0-A 计划中提前实现。

当前不引入 Eino、Claude Agent SDK、复杂多 Agent 编排、消息中间件、微服务拆分、市场或计费系统。只有现有 provider + 有界修复循环被可复现证据证明无法满足质量或可靠性目标时，才重新评估 Agent 框架；评估前必须先写决策记录，不能在实现中静默迁移。

## 14. 证据与执行文档索引

| 文档 | 职责 | 更新时机 |
|---|---|---|
| `docs/superpowers/specs/2026-07-12-agent-card-container-design.md` | 顶层架构、安全边界和运行时事实来源 | 仅当协议、安全边界或运行时选择改变时 |
| `docs/superpowers/plans/2026-07-13-real-deepseek-nativecard-closure.md` | P0-A 的 TDD 实施任务与命令 | 每个任务有实际测试证据后 |
| `docs/verification/p0a-real-deepseek-nativecard.md` | P0-A 真实模型、签名制品和未执行设备项证据 | live gate 或相关审计实际运行后 |
| `docs/superpowers/plans/2026-07-15-p0b-persistent-test-environment.md` | P0-B 的 TDD 实施任务与可重复门禁 | 每个任务有实际测试证据后 |
| `docs/verification/p0b-persistent-test-environment.md` | P0-B 重启、readiness、下载、验签和备份恢复证据 | 本地或固定远程环境复验后 |
| `docs/superpowers/plans/2026-07-15-p1b-worker-reliability.md` | P1-B 的 TDD、heartbeat、幂等和故障恢复任务 | worker 可靠性行为改变后 |
| `docs/verification/p1b-worker-reliability.md` | P1-B race、事务、租约、重试、取消和 crash recovery 证据 | worker 或持久化门禁复验后 |
| `docs/superpowers/specs/2026-07-15-next-stage-product-roadmap.md` | 下一阶段产品优先级、Agent 边界和实施顺序 | 阶段目标、依赖或产品边界改变后 |
| `docs/superpowers/specs/2026-07-15-codecard-production-design.md` | P1-C 固定评测、builder、浏览器/RPC 和安全边界 | CodeCard 合同、门槛或运行边界改变后 |
| `docs/superpowers/plans/2026-07-15-p1c-codecard-production.md` | P1-C 的 TDD 实施任务和小步提交序列 | 每项产生实际实现和验证证据后 |
| `docs/verification/m3-production-integration.md` | PostgreSQL、S3 和 sandbox adapter 集成证据 | P0-B 固定环境复验后 |
| `docs/verification/m4-acceptance.md` | 跨里程碑总验收与 release withheld 决策 | 新的自动化、设备或发行证据产生后 |
| `docs/verification/windows-m0-m4-evidence-template.md` | Windows 设备矩阵证据模板 | P0-C/P2 真实设备执行时 |

证据文档只记录实际执行结果：`NOT RUN`、`FAIL` 和 `PASS` 必须严格区分。设计完成、代码存在、CI 编译成功、fake provider 或 Linux/headless 结果均不能替代 Windows 设备证据。任何阶段状态变化都必须能从上述计划或验证文档追溯到命令、commit 和脱敏结果。
