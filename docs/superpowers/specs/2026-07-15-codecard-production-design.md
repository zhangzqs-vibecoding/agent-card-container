# P1-C CodeCard 生产链路设计

状态：**已冻结，Linux/headless 实施中**  
日期：2026-07-15（Asia/Shanghai）  
上位设计：`docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md`

## 1. 目标与边界

本工作包把现有 CodeCard 骨架推进为可重复验证的生产链路：真实模型生成限定源码，受控 OCI builder 在隔离环境内构建，云端签名发布，客户端从本地 Runtime Server 加载并通过受控 HTTP RPC 使用本地能力。

本阶段在 Linux 完成 Agent、模板、沙箱、浏览器和离线自动化证据；Windows WebView2、独立 OS 窗口和实机崩溃恢复继续标记 `DEVICE NOT RUN`，不得用 Chromium 或 Flutter widget test 替代。

非目标：第三方依赖市场、任意 npm 安装、服务端运行生成的 JavaScript、Flutter Web 产品端、Agent 生成 Shell/Dart/原生代码、放宽 Capability Broker。

## 2. 已有基础

- `CodingAgent.generateWeb`：最多三次模型调用，严格解码 `src/card.tsx`、`src/card.css`、`src/card.test.tsx`。
- `TemplateWebBuilder`：复制固定模板到一次性 workspace，构建完成后删除。
- `DockerBuilder`：要求镜像 digest，禁网、只读 rootfs、非 root、drop capabilities、资源与时间限制。
- `tooling/codecard-template`：固定 Preact/pnpm 版本，执行依赖校验、测试、typecheck、bundle 和输出校验。
- Flutter `LocalRuntimeServer`：随机 `*.localhost` authority、会话 token、静态资源、storage、事件和 HTTP RPC。
- `InAppWebViewPort`：Windows-only 能力声明，限制导航、资源请求、权限、下载和新窗口。

## 3. 生成合同

模型只允许返回一个 JSON 对象：

```json
{
  "files": {
    "src/card.tsx": "完整源码",
    "src/card.css": "可选样式",
    "src/card.test.tsx": "可选测试"
  }
}
```

约束如下：

- 必须包含 `src/card.tsx`，最多三个文件。
- 单文件不超过 256 KiB，总源码不超过 512 KiB。
- 不允许新增依赖、修改模板、使用远程资源或动态代码执行。
- CodeCard API 由模板中的本地 TypeScript 声明提供；能力调用只能进入 `/v1/rpc`。
- Agent 收到的修复反馈必须稳定化并截断，不回传凭据、完整构建日志或宿主路径。

## 4. 固定质量评测

评测集冻结为 20 个中文用例，每类 4 个：

| 类别 | 必须覆盖 |
|---|---|
| `canvas` | 自由画板、像素画、流程节点、签名板 |
| `game` | 井字棋、记忆翻牌、贪吃蛇简版、2048 简版 |
| `visualization` | 时间序列、分类柱图、进度仪表、交互筛选图 |
| `interaction` | 看板、计算器、表格编辑、拖放排序 |
| `offline-tool` | Markdown 笔记、番茄钟、习惯追踪、JSON 格式化 |

每个用例声明稳定 ID、prompt、类别、必须出现的可观察语义、禁止源码模式、允许能力和交互脚本。fixture 严格拒绝未知字段、重复 ID、类别数量漂移和空断言。

通过门槛在首次付费运行前固定：

- 总通过数至少 16/20。
- 每个类别至少 3/4，避免单一类别掩盖系统性失败。
- 每个用例最多 3 次模型调用；全套最多 60 次调用。
- 全套输入 token 不超过 300,000，输出 token 不超过 160,000。
- 按运行时注入的单价计算，估算费用不得超过 5 美元；未提供可信单价时只能报告 token，不得宣称费用门槛通过。
- 单用例从模型请求到沙箱构建完成不超过 5 分钟；整套不超过 90 分钟。

“通过”必须同时满足：源码合同、受控构建、bundle 安全校验、页面启动、用例语义断言、无未处理异常、无外网请求。只生成漂亮页面但缺失要求行为视为失败。

## 5. Builder 镜像供应链

builder 由独立 GitHub Actions workflow 构建，不能在运行时安装依赖或临时选择基础镜像。

- Node 基础镜像按 digest 固定。
- pnpm、生产依赖和开发依赖由 lockfile 固定。
- CI 构建镜像后执行 Trivy（或等价扫描）和真实恶意输入集成测试。
- 只有 `develop` 或显式手工任务可以发布到 GHCR；PR 只构建和验证，不发布。
- workflow 使用最小 `contents: read`、`packages: write` 权限；不得向 fork PR 暴露写权限或 secret。
- 运行配置保存完整 `repository@sha256:<digest>`，禁止 tag-only 镜像。
- 保留上一已验证 digest 作为回滚点。

## 6. 浏览器与本地运行时

Linux 自动化使用真实 Chromium 加载构建后的静态资源，验证 Web 语义；产品端仍由 Flutter 启动 Dart `HttpServer`。

每个卡片实例获得：

- 随机 128-bit 子域标识，authority 为 `<random>.localhost:<port>`。
- 至少 256-bit、仅存内存的会话 token。
- 独立 storage namespace、声明能力集、locale/theme/surface/online context。
- 只允许同 origin 静态资源、RPC 和事件通道的 CSP。

JS SDK 通过相对 URL 调用 `/v1/rpc`，以 header 携带 token。Runtime Server 必须同时校验 Host、token、method、声明能力、参数 schema、请求体大小、速率和执行超时。客户端关闭 session 后 token 立即失效。

离线指“云端不可达时仍可使用已安装卡片”，不代表 Runtime Server 可以关闭。应用重启后从本地签名制品和持久化状态重建新的随机 origin/session；旧 token 和旧 authority 不复用。

## 7. 安全不变量

- 构建网络始终为 `none`，不能为了生成成功率临时开放。
- 输出 bundle 不得包含外部 URL、source map、内联可执行脚本、`eval`、`new Function`、动态 import 或超限 data URI。
- ZIP 解包、manifest、SHA-256 和 Ed25519 验证先于 Runtime Server 注册资源。
- 页面禁止跨 origin 导航、弹窗、下载、文件访问、摄像头、麦克风、定位和未声明权限。
- RPC 拒绝私网访问、任意文件路径、任意进程和未经 Capability Broker 注册的方法。
- 卡片异常、超时、过量 RPC 或 WebView 崩溃只隔离当前实例。
- 日志不记录生成源码、prompt、卡片状态、token、Authorization 或构建环境凭据。

## 8. 验证分层

1. Go 单元测试：fixture、源码解码、prompt、调用预算、错误反馈和结果统计。
2. Node 测试：SDK、模板、依赖政策、bundle 校验和语义探针。
3. Docker 集成：固定 digest、禁网、非 root、只读、资源限制、取消和恶意输出。
4. Linux Chromium：交互、storage、RPC、CSP、导航拒绝、断网和页面重载。
5. Go PostgreSQL/MinIO：真实生成后的签名发布和幂等恢复。
6. Windows 设备：WebView2、三个 Surface、DPI/IME、断网重启和单卡崩溃隔离。

Linux/headless 全部通过后，本工作包最多标记 `HEADLESS PASS / WINDOWS DEVICE NOT RUN`。

## 9. 交付物

- 固定 20 用例 fixture、验证器、报告器和 opt-in 真实模型 gate。
- 精简且版本化的 CodeCard JS SDK 与模板 API 文档。
- 可重复构建、扫描、发布和按 digest 使用的 builder 镜像。
- Linux Chromium 端到端测试和离线/RPC 攻击矩阵。
- CodeCard 真实 PostgreSQL/MinIO 发布证据。
- Windows P1-C 设备执行包与证据模板。

