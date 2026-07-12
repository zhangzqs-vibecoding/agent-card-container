# 桌面端云端服务设置设计

## 1. 目标与范围

为 Flutter 桌面客户端增加可视化的云端服务配置入口，使直接下载 Windows 便携包的用户不需要先配置进程环境变量，也能填写 Go 后端地址和访问令牌。

本功能采用“保存后重启生效”。现有 `DesktopBootstrap` 仍只在进程启动时构造一次云端客户端、Agent Studio 和卡片库控制器，不引入运行时热切换、全局状态管理或依赖注入重构。

本期包含：

- 编辑、验证、测试和保存云端地址与访问令牌。
- 配置本地 HTTP 开关和可信 Ed25519 公钥环。
- 将访问令牌保存到操作系统安全凭据库。
- 保存后提示重启，并提供立即重启操作。
- 清除用户保存的配置和凭据。
- 保留环境变量作为优先级最高的部署配置。

本期不包含：

- 保存后热切换后端。
- OIDC 浏览器登录或令牌刷新。
- 在客户端配置模型供应商密钥。
- 自动发现后端、同步多套环境或管理配置档案。

## 2. 用户体验

现有设置面板新增“云端服务”区域：

- 服务地址：必填，接受 HTTPS；仅在显式开启本地 HTTP 时接受 loopback HTTP。
- Access Token：密码输入框，默认隐藏，不回显已保存令牌的内容，只显示“已保存凭据”。
- 允许本地 HTTP：默认关闭，仅用于 `localhost`、`127.0.0.0/8` 或 `::1`。
- 可信公钥：折叠在“高级设置”中，输入 `keyId -> base64 Ed25519 公钥` 的 JSON 对象。
- 测试连接：使用当前表单值验证后端可达性和认证，不保存配置。
- 保存：只有本地校验和连接测试成功后才写入；成功后显示“重启应用后生效”。
- 立即重启：先请求正常关闭当前运行时，再重新启动当前可执行文件。
- 清除配置：删除普通配置和安全凭据，重启后恢复为未配置状态；环境变量配置不受影响。

状态反馈使用明确文本：未配置、测试中、连接正常、连接失败、已保存待重启、凭据仅本次会话可用。

当 URL 和 Token 均由环境变量提供时，设置区显示“由环境变量管理”，相关输入和保存、清除操作为只读或禁用。客户端不得显示环境变量中的 Token。

## 3. 配置与优先级

新增不可变的 `CloudConnectionSettings`，描述：

- `baseUrl`
- `accessToken`（仅存在于内存对象）
- `allowInsecureLoopback`
- `trustedKeys`
- `source`：`environment`、`user` 或 `none`
- `credentialPersistence`：`secure`、`sessionOnly` 或 `none`

启动时按以下顺序解析：

1. 若 `AGENTCARD_CLOUD_URL` 和 `AGENTCARD_ACCESS_TOKEN` 同时非空，使用完整环境变量配置。
2. 否则读取用户配置文件和安全凭据库。
3. URL 或 Token 任一缺失时不启用云端能力。

环境变量不与用户配置混合，避免 URL 来自部署而 Token 意外来自个人配置。环境变量模式下继续读取 `AGENTCARD_ALLOW_INSECURE_CLOUD` 和 `AGENTCARD_TRUSTED_KEYS_JSON`。

非敏感配置保存到应用数据目录的 `cloud-config.json`：

```json
{
  "schemaVersion": 1,
  "baseUrl": "https://agent-card.example.com",
  "allowInsecureLoopback": false,
  "trustedKeys": {
    "release-2026": "base64-ed25519-public-key"
  }
}
```

该文件不得包含 Token、Authorization header、模型密钥或其他凭据。写入采用同目录临时文件加原子替换；校验或连接测试失败时不修改旧配置。

## 4. 安全凭据

定义 `SecretStore` 接口，将平台插件限制在 adapter 层。生产 adapter 使用 `flutter_secure_storage`，由 Windows Credential Manager、macOS Keychain 和 Linux Secret Service 等平台机制保护 Token。业务与测试代码只依赖接口。

保存顺序为：

1. 校验全部输入。
2. 完成连接测试。
3. 写入安全凭据。
4. 原子写入普通配置。

若安全凭据写入失败：

- 不把 Token 回退写入 JSON、SQLite、日志或诊断包。
- 用户可选择仅在当前进程内继续测试，但“保存”视为失败，不声称重启后可用。
- 保留此前成功保存的配置和凭据。

清除顺序先删除安全凭据，再删除普通配置。任一步失败都应报告部分清理状态，不能静默声称已清除。

## 5. 连接测试

连接测试复用 `CloudApiClient` 的地址约束和认证头构造，但使用临时客户端，不替换当前运行时连接。测试包含：

1. `GET /healthz` 验证服务可达且确认为 Agent Card Cloud。
2. 调用一个现有的只读认证接口验证 Token 有效。

两个请求都成功才显示“连接正常”。错误映射为可操作的用户信息：地址格式错误、本地 HTTP 未允许、网络不可达、TLS 错误、认证失败、响应协议不兼容。错误消息不得包含 Token、Authorization header 或完整响应体。

## 6. 重启行为

保存完成后不改变当前 `DesktopRuntime`。界面展示待重启状态：

- “稍后重启”只关闭提示，当前连接继续工作。
- “立即重启”通过 adapter 启动当前可执行文件，然后走现有的正常关闭流程，确保数据库、loopback server、子窗口和托盘资源被释放。

若拉起新进程失败，当前进程继续运行并显示错误，不主动退出。测试环境通过 `ApplicationRestarter` 接口注入 fake，不直接启动进程。

## 7. 组件边界

- `CloudSettingsRepository`：只负责非敏感 JSON 的读取、校验、原子写入和删除。
- `SecretStore`：只负责 Token 的安全读写和删除。
- `CloudSettingsService`：合并环境变量与用户配置，协调测试、保存和清除。
- `CloudSettingsController`：维护表单异步状态，不持有平台插件。
- `CloudSettingsSection`：设置页 UI，只与 controller 交互。
- `ApplicationRestarter`：隔离桌面进程重启能力。
- `DesktopBootstrap`：启动时消费解析后的 `CloudConnectionSettings`，其余生命周期不变。

这些边界确保普通配置、秘密存储、网络验证、UI 和平台进程操作可以分别测试。

## 8. 错误处理与诊断

- 配置文件损坏时隔离为无效配置并在设置页提示，不导致应用启动失败。
- 安全凭据库锁定或不可用时保持离线启动，并提示重新输入或检查系统凭据服务。
- 所有异常在进入 UI 或诊断前统一脱敏。
- 诊断包只允许记录配置来源、是否已配置、URL 的 scheme/host、最近一次连接错误码；不记录 Token、完整 query、可信公钥内容或表单输入。

## 9. 测试与验收

所有新行为采用 TDD。最低测试集合：

- URL 规则只允许 HTTPS 或显式授权的 loopback HTTP。
- 环境变量完整配置覆盖用户配置，且两种来源不混合。
- Token 从不出现在 `cloud-config.json`、日志和诊断包。
- 安全凭据写入失败时旧配置保持不变。
- 连接测试同时验证健康接口和认证接口。
- 保存成功后仅进入待重启状态，不替换当前云端控制器。
- 立即重启仅在新进程拉起成功后关闭当前运行时。
- 清除操作删除配置与凭据，并准确报告部分失败。
- Bootstrap 重启后能从用户配置和安全凭据恢复云端能力。
- 环境变量管理模式下设置表单不可编辑且不泄露 Token。

提交前运行 Flutter 格式化、静态检查、完整测试和受影响的三平台 CI。Windows 便携包应能从设置页完成本地 Go 后端连接配置，并在重启后显示 Agent Studio 和云端卡片库。
