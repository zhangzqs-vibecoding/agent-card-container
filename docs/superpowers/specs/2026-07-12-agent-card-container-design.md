# Agent Card Container 顶层设计

状态：设计基线，已确认并实施中
日期：2026-07-12  
目标版本：Windows-first MVP

## 1. 文档目的

本文档是 Agent Card Container 后续实现的架构事实来源。项目从空仓库开始，后续实施计划、代码结构、接口和测试均应遵循本文档；任何影响安全边界、卡片协议或运行时选择的修改，都应先更新本文档，不能在实现中静默改变。

项目目标是构建一个以桌面端为主的跨平台应用：

- 用户通过自然语言让云端 Agent 生成卡片。
- 卡片可放入主工作区、桌面悬浮层或独立窗口。
- 常规卡片使用 Flutter 原生渲染，复杂卡片使用 HTML/CSS/JavaScript。
- 已安装卡片及其本地逻辑在断网后仍可运行。
- 云端后端使用 Go，负责 AI、编码 Agent、构建验证、制品签名和版本管理。

## 2. 已锁定的核心决策

1. 客户端采用 Flutter，首发平台为 Windows 11 x64；macOS 为第二平台，Linux 先支持普通窗口，悬浮层后续专项验证。
2. 卡片采用双运行时：
   - NativeCard：Agent 生成受控声明式结构，由 Flutter 原生组件目录渲染。
   - CodeCard：Agent 生成经过构建和审查的 WebBundle，由桌面 WebView 执行。
3. Agent 默认 NativeCard 优先；只有需求超出原生组件、表达式和本地动作目录时才生成 CodeCard。用户仍可显式指定运行时。
4. 不生成或动态执行 Dart；Flutter release 只执行应用内预编译代码。
5. Go 仅部署在云端，不存在 Go sidecar。
6. Flutter 进程内启动 Dart Local Runtime Server，负责 CodeCard 静态资源、卡片会话和本地能力 RPC。
7. CodeCard 的 MVP 通信方式为 loopback HTTP JSON-RPC，加 WebSocket 事件流。JavaScript SDK 隔离具体传输，未来可增加 WebMessage 适配器。
8. 所有系统能力都通过 Capability Broker，默认拒绝；任何卡片都不能直接获得 Shell、进程执行或任意文件系统访问。
9. 每个 CardInstance 只存在于一个活动 Surface；复制卡片会创建新的实例和独立状态。
10. 云端卡片版本不可变；客户端安装、升级和回滚均以签名制品为单位。

## 3. 范围

### 3.1 MVP 包含

- Windows 主窗口、系统托盘、主工作区。
- 卡片添加、移动、缩放、删除、复制和状态持久化。
- 至少一个透明悬浮宿主窗口；每个显示器最多一个，且每个宿主可容纳多个卡片。
- 将单个卡片分离为独立原生窗口，并可重新停靠。
- NativeCard 生成、渲染、交互和离线运行。
- CodeCard 生成、安装、沙箱运行、离线运行和本地状态持久化。
- 云端生成会话、需求确认、构建进度、预览、安装和版本历史。
- 能力授权、制品校验、网络隔离、结构化错误和基础诊断。

### 3.2 MVP 不包含

- 移动端适配。
- Linux Wayland 下跨桌面环境一致的置顶、绝对定位或点击穿透承诺。
- 卡片市场、公开分享、第三方开发者发布流程。
- 团队空间、组织权限和多人协作编辑。
- 布局、实例状态和能力授权的多设备同步。
- 自动无提示升级卡片。
- 任意 Dart、原生动态库、Node.js、Shell 或本机脚本执行。
- CodeCard 直接访问用户文件、局域网或互联网。
- 完整实现所有 A2UI 标准组件；MVP 使用内部稳定子集。

## 4. 总体架构

~~~text
┌──────────────────────── Flutter Desktop ────────────────────────┐
│                                                                  │
│  Main Workspace      Overlay Host       Detached Windows         │
│       │                    │                    │                  │
│  NativeCard / CodeCard Renderers and CardInstance Controllers    │
│       │                    │                    │                  │
│       └──────────── Capability Broker Client ─────────────────┐   │
│                                                               │   │
│  Local Database      Artifact Cache      Local Runtime Server │   │
│  layouts/state       verified bundles    assets/RPC/events    │   │
│       │                    │                    │              │   │
│       └──────────────── Capability Broker ◀───────────────────┘   │
│                              │                                   │
│                   Flutter services/platform channels             │
└──────────────────────────────┼───────────────────────────────────┘
                               │ HTTPS + SSE
┌──────────────────────────────▼───────────────────────────────────┐
│                         Go Cloud                                 │
│ API/Auth │ Generation Sessions │ Agent Orchestrator │ Job Worker │
│ Postgres │ OCI Build Sandbox   │ Artifact Signer    │ Object Store│
└──────────────────────────────────────────────────────────────────┘
~~~

客户端是 local-first：布局、授权、实例状态和已安装制品都由本地负责。云端不可用时，生成和云端数据刷新停止，但已安装的本地卡片继续运行。

## 5. 仓库与模块边界

仓库采用单仓库、模块化单体结构：

~~~text
apps/
  desktop/                 Flutter 桌面应用
services/
  cloud/                   Go API 与 worker，同一代码库、不同启动模式
contracts/
  card/                    CardDefinition、NativeCard 和制品 JSON Schema
  cloud/                   OpenAPI 与 SSE 事件 schema
  local-rpc/               本地 JSON-RPC 方法和事件 schema
tooling/
  codecard-template/       固定的 TypeScript/Preact 生成模板
  sandbox-image/           云端生成与构建镜像
docs/
  architecture/            后续 ADR 和安全说明
~~~

约束：

- contracts 是 Go、Dart、TypeScript 的协议来源，不在三处手工维护重复结构。
- services/cloud 在 MVP 中保持模块化单体，不拆微服务。
- apps/desktop 内部按 feature 和边界组织，不建立全局万能 service。
- 平台插件只能出现在 adapter 层，卡片和领域层不能直接依赖具体窗口或 WebView 插件。

CardRuntime 对两种渲染器提供统一生命周期：mount、updateContext、suspend、resume、unmount 和 dispose。NativeCardRenderer 与 CodeCardHost 只能通过 CapabilityBrokerClient 请求系统能力，不得直接调用平台插件。

## 6. 领域模型

### 6.1 CardDefinition

CardDefinition 描述一个不可变卡片版本：

~~~json
{
  "formatVersion": 1,
  "minHostVersion": "1.0.0",
  "cardId": "card_01...",
  "versionId": "ver_01...",
  "displayVersion": "1.0.0",
  "runtime": "native",
  "stateSchemaVersion": 1,
  "title": "番茄钟",
  "description": "本地番茄钟",
  "entrypoint": "payload/native.json",
  "catalogVersion": "1",
  "minSize": {"width": 240, "height": 160},
  "preferredSize": {"width": 360, "height": 240},
  "maxSize": {"width": 1200, "height": 900},
  "capabilities": ["storage", "notification.show"],
  "networkPolicy": {"mode": "none", "domains": []},
  "files": [],
  "createdAt": "2026-07-12T00:00:00Z"
}
~~~

runtime 只允许 native 或 web。CardDefinition 不保存窗口位置、用户授权和运行时状态。

### 6.2 CardInstance

CardInstance 是用户设备上的可运行实例：

- instanceId：本地生成的 UUID。
- cardId、versionId：固定指向已安装版本。
- surfaceId：当前所属 Surface。
- placement：Surface 内的位置和尺寸，单位为逻辑像素。
- stateNamespace：实例独立的持久状态命名空间。
- status：active、suspended、error 或 quarantined。

升级先安装新 CardDefinition，再原子切换实例的 versionId。失败时保留旧版本，不迁移或破坏旧状态。

### 6.3 Surface

Surface 分为：

- workspace：主窗口内的网格工作区。
- overlay：每个显示器最多一个透明宿主窗口，内部可放多个卡片。
- detached：一个独立窗口承载一个 CardInstance。

一个实例同一时间只能属于一个 Surface。移动、分离和重新停靠只改变 placement，不复制业务状态。

### 6.4 其他持久对象

- CardInstallation：设备上已校验并安装的 versionId、contentHash、安装时间和校验状态。
- PermissionGrant：instanceId、versionId、capability、授权范围、决定和授权时间。
- AgentSession：云端需求确认、消息、生成状态、选择的 runtime 和发布结果。

CardDefinition、CardInstallation 和 CardInstance 必须分离：云端存在的版本不等于已安装，已安装也不等于已创建可运行实例。

## 7. 主工作区与窗口模型

### 7.1 主工作区

- 使用 12 列响应式网格。
- 卡片拖动和缩放时吸附网格。
- 每个卡片遵守定义中的最小和首选尺寸。
- 布局改变先更新内存，再用 300 毫秒防抖写入本地数据库。
- 窗口缩小时保持卡片宽高，不自动改变用户布局；超出区域通过滚动访问。

### 7.2 悬浮层

- 不为每个悬浮卡片创建一个 OS 窗口。
- 每个显示器创建一个透明、无边框、可置顶的 Overlay Host，多个卡片在其中布局。
- 编辑模式允许拖动、缩放和关闭；展示模式可配置点击穿透。
- 点击穿透开启后，必须可通过托盘菜单或全局快捷键恢复编辑模式。
- 显示器消失时，将其卡片移动到主显示器并约束在可见区域内。

### 7.3 独立窗口

- 用户显式选择“分离”时才创建独立窗口。
- 关闭独立窗口默认重新停靠到主工作区，不删除实例。
- 窗口边界、显示器标识、置顶状态和上次焦点被持久化。
- 坐标使用 Flutter 逻辑像素；平台 adapter 负责 DPI 和原生坐标转换。

### 7.4 窗口实现

Windows MVP 使用社区多窗口与窗口管理插件，并统一封装在 WindowBackend 后：

- 多窗口创建、销毁和窗口间消息由 WindowBackend 提供。
- 透明、无边框、置顶、任务栏可见性和点击穿透由 WindowStyleController 提供。
- 主 Flutter Engine 持续驻留并拥有托盘、Local Runtime Server 和全局状态。
- 子窗口可以使用独立 Flutter Engine，但不得各自启动 Runtime Server。
- 主 Engine 是 SQLite、CardInstance 状态和 PermissionGrant 的唯一写入者。
- 子 Engine 通过 SurfaceBridge 调用主 Engine；不得直接打开数据库进行写操作，也不得假设 Dart 内存跨 Engine 共享。
- Flutter 官方多窗口 API 稳定后，可替换 adapter，不改变领域模型。

SurfaceBridge 只提供 mount、unmount、placementChanged、focusChanged、invokeCapability 和 hostEvent 六类消息。消息必须携带 windowId 和 instanceId，由主 Engine 校验二者关系；子 Engine 不能通过消息访问其他窗口的实例。

## 8. NativeCard 运行时

### 8.1 定位

NativeCard 是默认运行时，适用于信息面板、表单、计时器、待办、快捷操作、列表和常规图表。Agent 生成声明式 JSON，Flutter 只渲染应用预编译的可信组件。

NativeCard 协议采用内部 AgentCard Native Schema v1，并保持与 A2UI 概念兼容，但不直接暴露实验性 SDK 类型。未来通过 adapter 接受标准 A2UI 消息。

### 8.2 MVP 组件目录

- 布局：Container、Row、Column、Stack、Grid、Scroll、Divider。
- 展示：Text、Icon、Image、Badge、Progress、Chart。
- 交互：Button、TextInput、Checkbox、Select、Slider。
- 数据：List、KeyValue、EmptyState、ErrorState。

每个组件都有固定属性 schema、默认值、尺寸限制和无障碍语义。未知组件、未知属性和超出限制的树必须在安装前拒绝。

客户端只接受自身明确支持的 catalogVersion。版本不支持时显示“需要更新宿主应用”，不能静默忽略组件或猜测兼容映射。MVP 不接收云端增量 patch；修改通过新的不可变 CardVersion 发布。

### 8.3 状态与动作

NativeCard 支持：

- initialState 和实例级持久状态。
- 单向数据绑定。
- 条件显示和有限列表映射。
- 纯表达式 AST：算术、比较、布尔、字符串与日期格式化。
- 本地动作：set、increment、toggle、append、remove、startTimer、stopTimer。
- capability.invoke：调用 Capability Broker。

表达式不允许循环、递归、动态函数、文件和网络访问。节点深度上限 32，组件节点上限 500，单次列表渲染上限 200。

### 8.4 离线行为

定义、组件、表达式执行器和状态都在本地，因此 NativeCard 可完全离线。需要互联网的能力返回 OFFLINE，卡片继续展示缓存数据和本地状态。

## 9. CodeCard 运行时

### 9.1 制品

CodeCard 是已经构建完成的静态 WebBundle，而不是源代码运行环境。云端默认使用固定 TypeScript、Preact 和 Vite 模板生成，但运行时只要求输出符合协议的 HTML、CSS、JavaScript 和静态资源。

制品限制：

- 压缩包最大 8 MiB，解压后最大 32 MiB。
- 文件数最大 512，单文件最大 8 MiB，路径深度最大 8。
- 入口固定为 payload/web/index.html。
- 禁止远程脚本、远程样式、动态 import URL、Node API 和原生扩展。
- MVP 不支持 WebAssembly、Service Worker 和浏览器扩展 API。
- 所有依赖必须在构建阶段打入制品。

### 9.2 Dart Local Runtime Server

主 Flutter Engine 启动唯一的 Local Runtime Server：

- 使用 HttpServer 绑定 InternetAddress.loopbackIPv4 和端口 0。
- shared 为 false，不监听局域网地址。
- 生命周期与应用进程一致；主窗口关闭但托盘仍运行时保持存活。
- 静态文件只从已校验、只读的内容寻址缓存中读取。
- 禁止目录列表、符号链接和路径穿越。

每次挂载 CodeCard 都创建随机 128 位 session：

~~~text
http://<random-session>.localhost:<port>/bundle/<content-hash>/index.html
~~~

随机 hostname 为每个实例提供独立 origin。Server 必须严格校验 Host；session hostname 永不复用。Windows PoC 必须验证 WebView2 对随机子域 localhost 的解析和存储隔离。如果目标 WebView 不支持该方式，WindowBackend 为该实例创建独立随机端口，不能退化为仅按 path 隔离。

### 9.3 JavaScript SDK

每个页面在文档启动阶段获得 window.agentCard：

~~~typescript
interface AgentCardContext {
  instanceId: string;
  cardId: string;
  versionId: string;
  locale: string;
  theme: "light" | "dark" | "system";
  surface: "workspace" | "overlay" | "detached";
  online: boolean;
}

interface AgentCardSdk {
  getContext(): Promise<AgentCardContext>;
  invoke<T>(method: string, params?: unknown): Promise<T>;
  subscribe(event: string, handler: (payload: unknown) => void): () => void;
}
~~~

SDK 只暴露领域方法，不暴露端口、Flutter 对象或 Dart 反射。实现使用闭包持有短期 session token，不将 token 写入 URL、日志、localStorage 或卡片持久状态。

固定 CodeCard 模板要求 index.html 在其他卡片脚本之前加载 /runtime/bootstrap.js。Server 根据随机 Host 动态提供该只读脚本，在响应中创建 SDK 闭包并注入 session token。该脚本不写入 Artifact Cache；session 关闭后立即失效。卡片代码本身可以调用 SDK，但不能借此扩大 manifest 和用户授权赋予的权限。

### 9.4 本地 RPC

请求使用 JSON-RPC 2.0：

~~~text
POST /v1/rpc
Authorization: Bearer <session-token>
Origin: http://<random-session>.localhost:<port>
Content-Type: application/json
X-AgentCard-RPC-Version: 1
~~~

服务端从 token 得到 instanceId、versionId 和 grants，不信任请求中的身份字段。事件使用同 origin 的 /v1/events WebSocket，带单调递增 seq；断线后 SDK 重新连接并重新获取 context，不承诺重放瞬时事件。

约束：

- 请求体最大 256 KiB，普通响应最大 1 MiB；network.fetch 响应单独限制为 5 MiB。
- 单实例持续 30 次/秒、突发 60 次；超限返回 RATE_LIMITED。
- 默认调用超时 10 秒。
- 不返回 Dart 堆栈、绝对路径、token 或内部错误。
- CORS 默认关闭；只接受完全匹配的 Origin 和 Host。

MVP 方法名固定为：

- runtime.getContext
- storage.get、storage.set、storage.delete、storage.list
- notification.show
- clipboard.write、clipboard.read
- host.openExternal
- network.fetch
- system.metrics.get、system.metrics.subscribe、system.metrics.unsubscribe
- window.getState、window.detach、window.dock、window.setAlwaysOnTop、window.requestAttention

事件名固定为 context.changed、theme.changed、online.changed、surface.changed、permission.changed、system.metrics、runtime.suspend 和 runtime.resume。新增方法或事件必须提升 local-rpc contract 版本，不能让卡片依赖未登记的字符串方法。

### 9.5 WebView 策略

Local Runtime Server 为页面强制设置 CSP：

~~~text
default-src 'none';
script-src 'self';
style-src 'self' 'unsafe-inline';
img-src 'self' data: blob:;
font-src 'self';
connect-src 'self';
media-src 'self' blob:;
worker-src 'self' blob:;
frame-src 'none';
object-src 'none';
base-uri 'none';
form-action 'none'
~~~

同时在 WebView adapter 层：

- 阻止离开当前随机 origin 的顶层导航和子资源请求。
- 阻止 popup、下载、摄像头、麦克风、定位和通知权限。
- 外部链接只能通过 host.openExternal，并由系统浏览器打开。
- 不开启 file URL 通用访问。
- 开发者工具只在开发构建和显式诊断模式中启用。
- session 关闭后清理该 WebView；随机 origin 永不复用，避免历史存储影响其他实例。
- 所有静态响应设置 X-Content-Type-Options: nosniff、Referrer-Policy: no-referrer、Cache-Control: immutable，并使用按扩展名白名单确定的 MIME。

## 10. Capability Broker

Capability Broker 是 NativeCard 和 CodeCard 唯一的系统能力入口。能力由卡片 manifest 声明、用户授权，并在每次调用时重新校验。

### 10.1 MVP 能力

| 能力 | 默认策略 | 限制 |
|---|---|---|
| storage | 安装时自动授予 | 每实例 5 MiB，JSON 值，命名空间隔离 |
| notification.show | 首次使用确认 | 只能发本卡片署名通知，频率限制 |
| clipboard.write | 首次使用确认 | 最大 1 MiB |
| clipboard.read | 每次用户手势确认 | 不允许后台读取 |
| host.openExternal | 每域首次确认 | 只允许 http/https |
| network.fetch | 按域授权 | 由宿主代理，禁止私网和 loopback，响应最大 5 MiB |
| system.metrics.read | 首次使用确认 | 只提供预定义聚合指标 |
| window.manageSelf | 安装时自动授予 | 只能管理自身 Surface 和窗口 |

文件选择、日历、邮件、Shell、进程控制、全局键盘监听不属于 MVP 能力。

### 10.2 授权规则

- manifest 未声明的能力永远不能调用。
- 用户授权不得超过 manifest 请求的范围。
- PermissionGrant 按 instanceId、versionId 和 capability 保存。升级后，能力和范围完全相同或更窄的授权可以继承；新增能力、扩大域名范围或改变授权模式必须重新确认。clipboard.read 仍然要求每次用户手势确认。
- network.fetch 使用规范化域名 allowlist，禁止 IP 字面量、localhost、私有地址和重定向到未授权域。
- 敏感凭据保存在宿主安全存储中，只允许宿主代执行操作，不返回给卡片。
- 用户撤销能力后立即终止相关订阅并返回 PERMISSION_DENIED。

### 10.3 统一错误

能力错误使用稳定 code：

- INVALID_PARAMS
- PERMISSION_REQUIRED
- PERMISSION_DENIED
- CAPABILITY_UNAVAILABLE
- OFFLINE
- RATE_LIMITED
- TIMEOUT
- SESSION_EXPIRED
- INTERNAL

卡片可以展示安全 message；debugDetails 只进入本地开发日志。

## 11. 本地存储与离线

Flutter 使用 SQLite 保存：

- 已安装 CardDefinition 索引和当前版本。
- CardInstance、Surface、placement 和窗口状态。
- 能力授权及域约束。
- NativeCard 状态。
- CodeCard 通过 storage RPC 保存的状态。
- 生成会话的本地摘要和最近错误。

WebView localStorage、IndexedDB 和 Cookie 不作为持久状态契约。CodeCard 必须通过 storage RPC 保存需要跨重启的数据。

stateSchemaVersion 相同的升级复用现有实例状态。MVP 不执行 Agent 生成的状态迁移；版本改变 stateSchemaVersion 时，升级界面必须先备份旧状态，再让用户选择继续使用旧版本或安装新版本并重置该实例状态。

制品按 sha256 内容寻址保存：

~~~text
app-data/artifacts/sha256/<hash>/
~~~

安装流程为：

1. 下载到临时文件。
2. 校验归档大小、路径和文件数量。
3. 校验 manifest 签名及全部文件 hash。
4. 解压到同文件系统临时目录。
5. 原子重命名到内容地址。
6. 在数据库事务中登记版本。

失败时删除临时数据，不修改现有安装。应用离线启动时不得访问云端才能恢复布局。

## 12. 云端 Go 后端

### 12.1 部署形态

MVP 为一个 Go 模块、两个启动模式：

- api：鉴权、REST、SSE、卡片元数据和制品下载。
- worker：生成任务、Agent 工具循环、沙箱构建、验证和签名。

基础设施：

- PostgreSQL：用户、会话、消息、卡片、版本、任务和审计事件。
- S3 兼容对象存储：源码快照、构建日志、预览和签名制品。
- PostgreSQL job table：使用 SKIP LOCKED 领取任务，MVP 不引入 Redis。
- OIDC Bearer Token：API 鉴权；MVP 只有个人账户，不包含组织模型。

### 12.2 模块

- api：HTTP、鉴权、参数验证、错误映射。
- generation：需求确认、会话状态机和事件流。
- agent：模型调用、工具循环、运行时选择和修复尝试。
- sandbox：OCI 容器生命周期、资源限制和输出提取。
- validator：NativeCard schema、WebBundle policy 和测试报告。
- artifact：manifest、hash、签名、对象存储和版本发布。
- store：PostgreSQL repositories 和事务。
- modelprovider：最小模型接口，密钥只来自部署环境。

### 12.3 编码 Agent

编码 Agent 由 Go 编排，不在 Go 进程内执行生成代码：

1. 根据用户输入生成结构化需求摘要。
2. 用户确认摘要后，按 Native-first 策略选择运行时。
3. 创建隔离工作区并加载固定模板、协议和组件目录说明。
4. Agent 只能读取模板、修改工作区、运行允许的 build/test/validate 命令。
5. NativeCard 产出 schema JSON；CodeCard 产出 TypeScript/Preact 源码并在沙箱构建。
6. CodeCard 依次通过类型检查、单元测试、生产构建、依赖许可与漏洞检查、隔离浏览器加载、控制台错误和外部请求检查。
7. 每个任务总共最多调用模型三次：首次生成一次，验证失败后最多自动修复两次。
8. 通过所有门禁后生成预览、manifest、文件 hash 和签名制品。
9. 发布不可变 CardVersion，并通知桌面端可安装。

沙箱使用预构建 OCI 镜像：

- 默认无网络。
- 非 root 用户、只读根文件系统、临时工作目录。
- CPU 2 核、内存 2 GiB、总时限 5 分钟。
- 不注入模型密钥、数据库凭据、签名私钥或对象存储凭据。
- 只提取白名单输出目录；沙箱不能直接发布制品。

### 12.4 运行时选择

auto 请求遵循以下顺序：

1. 使用 NativeCard 组件、表达式和能力能完整实现时，选择 native。
2. 需要新算法、小游戏、自由绘制或原生目录无法表达的交互时，选择 web。
3. 需要 Shell、未授权文件系统、浏览器扩展或其他禁止能力时，拒绝生成并解释限制。

Agent 必须在生成事件中报告选择的 runtime 和原因。用户可在确认前改为 native 或 web；强制 native 但无法表达时返回 UNSUPPORTED_REQUIREMENT，不静默生成残缺卡片。

## 13. 云端接口

云端采用 HTTPS JSON REST 和 SSE，不在 MVP 引入 gRPC。

### 13.1 端点

- POST /v1/generations：创建生成会话。
- POST /v1/generations/{id}/messages：补充需求或提出修改。
- POST /v1/generations/{id}/confirm：确认需求并开始生成。
- POST /v1/generations/{id}/cancel：取消排队或运行任务。
- GET /v1/generations/{id}：获取当前快照。
- GET /v1/generations/{id}/events：SSE 进度流，支持 Last-Event-ID。
- GET /v1/cards：当前用户的卡片定义。
- GET /v1/cards/{cardId}：卡片和版本列表。
- GET /v1/cards/{cardId}/versions/{versionId}/artifact：获取短期下载地址及签名元数据。

创建请求：

~~~json
{
  "prompt": "做一个离线番茄钟",
  "target": "auto",
  "locale": "zh-CN"
}
~~~

target 允许 auto、native、web，默认 auto。

### 13.2 生成状态

状态机固定为：

~~~text
draft → awaiting_confirmation → queued → generating
      → validating → ready
      → failed | cancelled
~~~

SSE 事件至少包含 eventId、sessionId、type、stage、message、progress、timestamp。ready 事件只携带 versionId 和预览元数据，不携带签名私钥或对象存储内部路径。

### 13.3 云端错误

API 使用统一 envelope：

~~~json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "生成的卡片未通过安全校验",
    "requestId": "req_01..."
  }
}
~~~

用户输入错误返回 4xx；模型、构建和基础设施错误映射为稳定业务 code。客户端可重试 GET 和幂等取消操作，不能自动重复创建生成会话。

## 14. 制品、签名和版本

制品扩展名为 .agentcard，本质为 ZIP：

~~~text
manifest.json
payload/native.json
或
payload/web/index.html
payload/web/assets/...
reports/validation.json
~~~

manifest 使用规范化 JSON。签名算法为 Ed25519，覆盖去除 signature 字段后的 manifest 以及其中记录的全部文件 sha256。私钥由 KMS 或等价托管密钥服务保存，Agent worker 和构建沙箱只能提交摘要请求签名，不能读取私钥；客户端内置可轮换的公钥和 keyId 列表，公钥不是秘密。

版本规则：

- cardId 表示同一张逻辑卡片。
- versionId 为不可变 ULID。
- displayVersion 使用语义化版本，仅用于用户展示。
- 修改卡片始终创建新版本。
- MVP 不自动升级；用户预览差异后手动应用。
- 每个 CardInstance 可独立固定版本或回滚。

## 15. 安全模型

### 15.1 信任边界

- 云端模型输出不可信。
- 沙箱构建结果不可信，直到通过校验和签名。
- 已签名 CodeCard 仍按不可信 Web 内容运行；签名只证明来源和完整性，不提升权限。
- NativeCard schema 只有通过组件、表达式和限制校验后才可信。
- 本地 RPC 端口对同机其他进程可见，因此每次调用都必须验证 session、Host、Origin 和权限。
- 威胁模型防御恶意生成卡片、恶意网页、端口扫描、跨卡调用和制品篡改；不声称防御已经控制用户操作系统账户、Flutter 进程或进程内存的攻击者。

### 15.2 必须实施的门禁

- ZIP bomb、绝对路径、父目录、符号链接和重复路径检测。
- manifest JSON Schema 和未知字段策略校验。
- 文件 hash 与 Ed25519 签名校验。
- CSP 和 WebView 请求双层网络限制。
- RPC schema、大小、速率、超时和能力检查。
- 日志中的 token、Authorization、模型密钥和用户敏感字段脱敏。
- network.fetch 防 SSRF，并在每次重定向后重新校验目标。
- 卡片连续三次启动崩溃后进入 quarantined，需用户手动重试或回滚。

### 15.3 明确禁止

- 任意命令执行和通用 host object proxy。
- 将云端密钥下发到客户端或卡片。
- 将 session token 放入 URL、数据库或普通日志。
- 使用正则扫描替代运行时沙箱和权限校验。
- CodeCard 直接访问 file URL、原生插件或 Flutter MethodChannel。

## 16. 生命周期与故障恢复

### 16.1 客户端

- 主 Engine 启动后依次打开数据库、恢复已安装索引、启动 Runtime Server、恢复窗口。
- 单张卡片加载失败不影响其他卡片；显示 ErrorState，提供重载、回滚和删除。
- WebView 无响应、renderer 崩溃或加载超过 10 秒时终止该会话并允许重启；连续三次失败后隔离卡片。MVP 不宣称能够精确限制任意 JavaScript 的瞬时 CPU 使用。
- Runtime Server 意外停止时先停止所有 CodeCard 会话，重启 server，再为可见实例创建新 session。
- 休眠或锁屏时暂停不可见 CodeCard 和动画；恢复后重新发送 context。
- 退出前刷新布局写入；异常退出依赖事务和启动恢复，不依赖最后一次正常回调。

### 16.2 云端

- SSE 断开不取消任务；客户端用 Last-Event-ID 重连。
- worker 崩溃后，租约超时的任务重新入队；同一任务发布版本必须幂等。
- 用户取消后终止 Agent 循环和沙箱；已发布版本不受取消影响。
- 验证或签名失败绝不创建 ready 版本。

## 17. 可观测性与隐私

云端记录：

- requestId、sessionId、jobId、stage、耗时、模型用量、构建结果和错误 code。
- 不在普通日志记录完整 prompt、生成源码、Authorization 或本地卡片状态。
- prompt 和源码作为用户会话数据存储，并遵循账户删除流程。

客户端记录：

- 结构化本地日志，默认保留七天并限制总大小。
- CodeCard console 输出默认不上传。
- 用户主动导出诊断包时，先移除 token、授权头、绝对用户路径和卡片私有状态。

## 18. 测试策略

### 18.1 合同测试

- Go、Dart、TypeScript 对同一份 CardDefinition、NativeCard 和 RPC fixture 解码结果一致。
- schema 拒绝未知 runtime、越界尺寸、未知组件、非法 capability 和错误 hash。
- 每次 contracts 变更必须运行向后兼容 fixture。

### 18.2 Flutter 单元与组件测试

- NativeCard 每个 catalog 组件、绑定、表达式和动作。
- CardInstance 在 workspace、overlay、detached 之间移动时状态不丢失。
- 授权、撤销、离线和错误状态。
- 布局持久化、显示器缺失恢复和版本回滚。

### 18.3 Local Runtime Server 安全测试

- 只监听 loopback 和随机端口。
- 错误 Host、Origin、token、过期 session 和跨实例 token 全部拒绝。
- 无通配 CORS。
- 路径穿越、符号链接、非法 MIME、超大请求和速率超限被拒绝。
- 两张 CodeCard 的 localStorage、Cookie、RPC 和 storage namespace 相互隔离。
- 外部页面和另一张卡片不能调用目标实例能力。

### 18.4 Go 后端测试

- 生成状态机和 SSE 重连。
- 任务领取、租约恢复、取消和幂等发布。
- NativeCard 验证失败、CodeCard 构建失败和单任务三次模型调用上限。
- 沙箱无网络、无秘密、资源限制和白名单输出。
- 签名、篡改检测、短期下载地址和版本不可变。

### 18.5 Windows 端到端测试

必须覆盖：

1. 创建 NativeCard，安装到工作区，交互后断网重启，状态仍存在。
2. 创建含本地 JavaScript 逻辑的 CodeCard，安装后断网运行。
3. 将两类卡片分离、置于悬浮层、重新停靠，状态不丢失。
4. 主窗口关闭后托盘、悬浮层和 Runtime Server 继续工作。
5. 多显示器、混合 DPI、跨屏、休眠、锁屏和显示器拔插。
6. 中文输入法、键盘焦点、Tab 遍历、剪贴板和外链。
7. 篡改制品、恶意导航、私网请求、RPC 越权和 ZIP bomb 被阻止。
8. 1、3、10 个窗口及 1、5、20 张卡片的 CPU、内存、GPU、启动和恢复基线。

### 18.6 性能预算

参考设备为 Windows 11 x64、8 个逻辑核心、16 GiB 内存、NVMe、集成显卡、150% DPI，并已安装 WebView2 Runtime：

- 冷启动到工作区可交互：中位数不超过 3 秒，P95 不超过 5 秒。
- 缓存 NativeCard 从 mount 到首帧：中位数不超过 300 毫秒。
- 缓存 CodeCard 从 mount 到首帧：中位数不超过 1.5 秒。
- 10 张静态 NativeCard 加 3 张静态 CodeCard 稳定一分钟后，进程 CPU 中位数低于 3%，总常驻内存低于 1 GiB。
- 拖动和缩放悬浮卡片时，至少 95% 帧耗时不超过 16.7 毫秒。
- 同时活跃上限为 20 张 NativeCard 和 8 张 CodeCard；超出或不可见的 CodeCard 必须挂起。

## 19. 实施里程碑

### M0：Windows 技术闸门

- Flutter 主窗口中嵌 WebView2。
- Dart loopback server 提供多文件 WebBundle。
- 随机 localhost 子域的 origin、存储和 RPC 隔离。
- 一个透明悬浮宿主窗口和一个独立窗口。
- 离线重启、中文输入法、混合 DPI 和焦点验证。

只有 M0 通过后才继续完整产品实现；若 Flutter WebView 或窗口方案无法满足闸门，先更新本设计再更换技术路线。

M0、M1、M2、M3 和 M4 必须分别形成可独立验收的实施计划。后续实现按里程碑顺序推进，不生成一份覆盖整个系统的一次性代码任务清单。

### M1：客户端骨架与 NativeCard

- contracts、数据库、Artifact Installer、Capability Broker。
- 主工作区、Surface、窗口 adapter。
- NativeCard schema、catalog、状态和基础能力。

### M2：CodeCard 本地运行时

- .agentcard 校验和内容寻址缓存。
- Local Runtime Server、SDK、JSON-RPC、WebSocket。
- WebView 策略、隔离、资源限制和故障恢复。

### M3：Go 云端与生成 Agent

- API、SSE、PostgreSQL job、对象存储。
- NativeCard 生成链路。
- CodeCard OCI 编码、构建、验证、签名和下载。
- 桌面端生成、预览、安装和版本历史。

### M4：产品硬化

- Windows 安装、代码签名、升级、卸载清理、崩溃恢复和诊断。
- 安装器检测 WebView2 Runtime；缺失时提供官方 Evergreen Runtime 安装引导。
- 性能基线、恶意制品测试和权限 UX。
- macOS 技术闸门；Linux 仅启动普通窗口适配。

## 20. MVP 验收标准

MVP 完成必须同时满足：

- 用户可用中文自然语言生成 NativeCard 和 CodeCard。
- auto 模式优先生成 NativeCard，并能说明选择原因。
- 生成前有结构化需求确认，生成中有可恢复进度，失败有可理解错误。
- 两类卡片均可放入主工作区、悬浮层和独立窗口。
- 断开互联网并重启应用后，已安装的纯本地卡片仍可使用且状态保留。
- 任何卡片无法绕过 Capability Broker 访问网络、文件、进程或系统能力。
- 篡改、未签名、越界和超限制品不能安装。
- 单卡失败、WebView 崩溃或云端不可用不导致整个桌面应用不可用。
- Windows 端到端测试矩阵通过，且 M0 记录了可接受的资源基线。

## 21. 风险与默认假设

- Flutter 桌面主界面可用于生产；多窗口与 Windows WebView 依赖社区插件，因此必须先完成 M0。
- Windows-first 允许 MVP 针对 WebView2 和 Win32 行为优化，但领域协议不得依赖 Windows。
- macOS 可复用 WKWebView 和大部分 Flutter 代码，但 loopback server 的 App Sandbox entitlement、签名和窗口行为需要独立闸门。
- Linux 普通窗口可以支持；Wayland 悬浮和全局快捷键不进入 MVP 承诺。
- 用户数量按个人账户设计；组织、多租户管理界面和公开市场不提前建设。
- 云端模型供应商可替换，但 MVP 只接入一个供应商；modelprovider 接口不暴露供应商特有类型。
- 设计遵循 KISS、YAGNI、DRY 和 SOLID；任何为“未来可能需要”引入的新服务、协议或运行时都应有当前验收场景支撑。

## 22. 参考

- 参考项目：https://github.com/xyr723/agent-widget/tree/develop
- Flutter Desktop：https://docs.flutter.dev/platform-integration/desktop
- Flutter 多窗口示例：https://github.com/flutter/flutter/tree/3.44.0/examples/multiple_windows
- Flutter GenUI：https://docs.flutter.dev/ai/genui
- A2UI：https://github.com/a2ui-project/a2ui
- Dart HttpServer：https://api.dart.dev/dart-io/HttpServer/bind.html
- WebView2 本地内容：https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/working-with-local-content
- WebView2 安全指南：https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/security
- MCP Apps：https://modelcontextprotocol.io/extensions/apps/overview
