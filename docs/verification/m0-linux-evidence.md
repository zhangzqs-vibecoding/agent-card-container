# M0 Linux-hosted verification evidence

日期：2026-07-12  
主机：Linux x64  
Flutter：3.32.8 stable  
Dart：3.8.1  
Go：1.26.3

## 已验证

- Flutter 工程同时包含 Windows 和 Linux runner。
- AgentCardApp 在 1440×900 widget surface 上无 overflow。
- 桌面壳包含导航、12 列网格背景、空工作区、Agent Studio 和本地 Runtime 状态。
- Agent Studio 可收起并重新打开。
- Local Runtime Server 只绑定 IPv4 loopback 随机端口。
- CodeCard session 使用独立 hostname 和 token。
- 静态资源只对精确 Host 提供，并强制 CSP、nosniff 和 no-referrer。
- JSON-RPC 校验 Host、Origin、Bearer token 和协议版本。
- session 关闭即时失效，存储按 session 隔离且限制为 5 MiB。
- Go health API 通过真实 loopback 监听返回稳定 JSON。
- Flutter analyze、Flutter tests、Go tests 和 Go vet 均已执行。
- 1440×900 workspace golden 已生成并人工检查布局。

## 当前主机无法验证

以下是设计文档明确的 Windows M0 闸门，本文件不将它们标记为通过：

- WebView2 inline 合成、焦点、中文 IME 和混合 DPI。
- CodeCard 在 WebView2 中通过随机 localhost hostname 加载。
- 透明无边框 Overlay Host、置顶和点击穿透。
- Detached Window 创建、关闭和重新停靠。
- Windows 休眠、锁屏、多显示器拔插和虚拟桌面行为。
- WebView2 Runtime 缺失时的安装引导。

## Linux native build 环境

Flutter Dart 层可分析并执行测试；当前主机没有 Ninja 和 GTK3 development package，且当前用户没有免密 sudo，因此 flutter build linux 无法生成原生 runner。失败发生在 CMake 配置阶段，信息为：

~~~text
CMake was unable to find a build program corresponding to "Ninja".
Package gtk+-3.0 was not found in the pkg-config search path.
~~~

这属于主机构建依赖缺失，不作为 Windows M0 通过或失败的证据。

