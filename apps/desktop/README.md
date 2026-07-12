# Agent Card Desktop

Flutter 桌面客户端，Windows 11 x64 为首发平台。macOS 使用
WKWebView adapter；Linux 当前只承诺普通窗口，不承诺 Wayland
overlay 或全局快捷键。

## 开发环境

- Flutter 3.32.8 stable / Dart 3.8.1。
- Windows：Visual Studio C++ desktop workload 和 Evergreen WebView2 Runtime。
- macOS：Xcode，签名、sandbox entitlement 和 notarization 需在目标设备验证。
- Linux：Clang、CMake、Ninja、GTK3 development headers、libsodium，以及
  tray/hotkey 插件需要的 Ayatana AppIndicator、Keybinder 和 libnotify。

## 运行与测试

~~~bash
flutter pub get
flutter analyze
flutter test
flutter run -d linux
~~~

在 Windows 或 macOS 上将最后一条的 device 替换为对应桌面设备。
Linux 可重复门禁为：

~~~bash
sh ../../tooling/device-gates/linux-m4.sh
~~~

## 连接 Go 云端

仅当 URL 和 access token 同时存在时启用 Agent Studio 和云端卡片库：

~~~text
AGENTCARD_CLOUD_URL=https://agent-card.example.com
AGENTCARD_ACCESS_TOKEN=<OIDC-or-development-token>
AGENTCARD_ALLOW_INSECURE_CLOUD=false
AGENTCARD_TRUSTED_KEYS_JSON={"<key-id>":"<base64-ed25519-public-key>"}
~~~

`AGENTCARD_ALLOW_INSECURE_CLOUD=true` 只用于 loopback/local development。没有
trusted key 时仍可浏览云端数据，但客户端不会安装制品。客户端不需要、
也不应获取模型供应商 API key。

## 本地运行时

NativeCard 和 CodeCard 制品、SQLite 状态及权限授予保存在应用数据
目录。CodeCard 资源由客户端内的随机 loopback server 提供，
JavaScript 通过受认证的 HTTP/WebSocket JSON-RPC 请求 Capability Broker。
已安装的纯本地卡片断网并重启应用后仍可运行。

设置页可导出脱敏诊断包；诊断包不包含 token、授权头、绝对用户路径或
卡片私有状态。
