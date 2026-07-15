# Windows CodeCard device evidence

Status: **NOT RUN**  
Commit: `<full lowercase 40-character SHA>`  
Desktop executable SHA-256: `<64 lowercase hex>`

该模板只能由 Windows 11 x64 参考设备上的真实 Flutter 应用和 WebView2 运行结果填写。Linux Chromium、widget test、CI 构建成功或人工口头确认都不能替代设备证据。

## 执行前

1. 使用候选 commit 构建 Windows release bundle，并记录 `agent_card_desktop.exe` 的 SHA-256。
2. 确认测试服务 `/readyz` 正常，准备一个已签名 CodeCard；客户端中不得配置模型 API Key。
3. 记录 Windows build、CPU、内存、GPU、显示器/DPI、Flutter 和 WebView2 Evergreen 版本。
4. 将下面 JSON 复制到独立 evidence 文件；先保留 `NOT RUN`，实际逐项执行后才能改为 `PASS`。

```json
{
  "schemaVersion": 1,
  "platform": "windows",
  "commit": "<full lowercase commit>",
  "durationSeconds": 0,
  "host": {
    "os": "Windows 11 <build> x64",
    "displayScalePercent": 150
  },
  "runtime": {
    "flutter": "3.32.8 / Dart 3.8.1",
    "webView2": "<Evergreen runtime version>"
  },
  "artifact": {
    "desktopExeSha256": "<64 lowercase hex>"
  },
  "scenarios": {
    "webView2Embedded": "NOT RUN",
    "randomLocalhostOrigin": "NOT RUN",
    "originStorageIsolation": "NOT RUN",
    "localRpc": "NOT RUN",
    "offlineRestart": "NOT RUN",
    "workspaceSurface": "NOT RUN",
    "detachedWindow": "NOT RUN",
    "overlaySurface": "NOT RUN",
    "chineseIme": "NOT RUN",
    "mixedDpi150": "NOT RUN",
    "webViewCrashIsolation": "NOT RUN",
    "tamperedArtifactRejected": "NOT RUN",
    "privateNetworkRejected": "NOT RUN"
  }
}
```

## 场景要求

- `webView2Embedded`：真实 WebView2 显示 CodeCard，本地 JavaScript 和交互正常，无 DevTools 注入。
- `randomLocalhostOrigin`：不同实例使用不同随机 `*.localhost` authority，URL 中没有 token。
- `originStorageIsolation`：两个实例不能读取对方 storage，重载不丢失自身状态。
- `localRpc`：context、storage、事件和一个已声明 capability 正常；缺 token、错 Origin 和未声明 capability 被拒绝。
- `offlineRestart`：断开云端网络并重启应用后，已安装卡片从本地签名制品恢复。
- 三种 Surface：工作区、独立窗口、悬浮层均可交互，往返切换不丢状态。
- `chineseIme`、`mixedDpi150`：中文输入、焦点、150% DPI 和跨显示器移动无阻断问题。
- `webViewCrashIsolation`：结束单个 WebView 进程只隔离该卡片，主应用和其他卡片存活并可恢复。
- 安全项：篡改制品、私网/loopback 网络请求必须被拒绝。

保存截图、录屏或原始日志时只记录关联 ID，不记录 prompt、源码、卡片状态、Authorization、Cookie、token、API Key、DSN 或签名私钥。

## 验证命令

```powershell
pwsh -NoProfile -File packaging/windows/run-codecard-device-gate.ps1 `
  -Evidence <evidence.json> `
  -Bundle <release-bundle-directory> `
  -ExpectedCommit <full-commit> `
  -ExpectedExecutableSHA256 <sha256> `
  -TimeoutSeconds 900
```

脚本会再次验证 release bundle、WebView2、commit、可执行文件 hash、执行时长、敏感信息和全部场景。任意 `NOT RUN` 都会失败。
