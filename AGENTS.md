# Agent Card Container 工程约定

## 事实来源

- 顶层架构以 docs/superpowers/specs/2026-07-12-agent-card-container-design.md 为准。
- 实现按 M0 到 M4 顺序推进，每个里程碑必须有独立计划、测试证据和小步提交。
- 文档没有规定的普通工程决策，优先选择维护成本最低、依赖最少且安全边界清晰的方案。

## 工程结构

- apps/desktop：Flutter 桌面客户端。
- services/cloud：Go 云端 API 与 worker。
- contracts：跨 Go、Dart、TypeScript 的协议和 fixture。
- tooling：CodeCard 模板及构建沙箱。

## 开发规则

- 新行为使用 TDD：先写失败测试并确认失败，再写最小实现。
- contracts 是协议事实来源，不在多语言代码中手写互相冲突的结构。
- Go 云端保持模块化单体；Flutter 平台插件只能出现在 adapter 层。
- NativeCard 和 CodeCard 统一通过 Capability Broker 请求系统能力。
- 不执行生成 Dart、Shell、本机脚本或未签名制品。
- 不硬编码密钥、token、密码或私钥。
- 真实模型联调凭据只通过临时环境变量注入，不写入仓库、脚本、命令参数或日志。
- Linux 桌面端与 Linux Chromium 是正式测试目标；Windows 设备门禁是额外平台验收，不将 Linux 仅视为 Windows 的替代环境。
- 每次提交前运行受影响项目的格式化、静态检查和测试。

## Git

- 当前工作分支使用 develop。
- 允许按用户要求进行阶段性小步 commit，但不主动 push。
- 不提交构建产物、运行数据、密钥或本地环境文件。
