# P1-A 客户端卡片版本生命周期验证

状态：**P1-A VERSION LIFECYCLE HEADLESS PASS / INSTANCE LIFECYCLE OPEN / WINDOWS DEVICE NOT RUN**

验证日期：2026-07-16（Asia/Shanghai）  
代码基线：`develop` / `84b4d33`

## 结论

Flutter/Linux headless 已闭环同卡客户端版本生命周期：用户可以从当前实例发起 Agent 修改，客户端下载并重新验签同卡目标版本，展示 capability、network domain 和状态 schema 差异，明确确认后原子升级或回滚同一实例。

相同 `stateSchemaVersion` 复用原 namespace 和状态；不同时先保存最大 1 MiB 的有界快照，再重置或恢复目标历史快照。SQLite 事务同时切换 version、namespace 和 grants；目标 runtime 只有在数据库提交后构建并原位替换一次，构建失败会恢复旧 version、namespace、状态和 grants。

实例复制、删除、卸载及“保留/删除数据”选择仍未实现。Windows WebView2、独立窗口、悬浮层、DPI、多屏和 IME 真实交互没有设备证据，因此不属于本次 PASS。

## 实现证据

| 能力 | 提交 | 结果 |
|---|---|---|
| SQLite v5、1 MiB 快照、每卡最近 10 条 | `312a963` | PASS |
| 原子版本、状态、namespace 和 grants 切换 | `eadda2d` | PASS |
| 已验证目标版本登记与差异准备 | `9c9e08c` | PASS |
| 显式授权、grant 收缩、取消和陈旧确认 | `fbbad7b` | PASS |
| Workspace 同实例原位单次替换 | `1a80c72` | PASS |
| runtime 构建失败补偿恢复 | `062b399` | PASS |
| 升级/回滚确认 UI 与 bootstrap 接线 | `ac8d358` | PASS |
| “基于此版本修改”与 base IDs | `84b4d33` | PASS |

## 自动化验证

### Flutter 全门禁

```text
dart format --output=none --set-exit-if-changed lib test
Formatted 126 files (0 changed)

flutter analyze
No issues found!

env -u AGENTCARD_MODEL_API_KEY -u DEEPSEEK_API_KEY flutter test
239 passed, 1 expected skip
```

覆盖证据包括：

- SQLite v5 migration、快照大小和保留上限；
- 同 schema 状态复用和 grant 收缩；
- 异 schema 取消零写入、备份/重置、回滚恢复；
- 错误 grant 导致整个事务回滚；
- capability/domain 扩大必须显式批准；
- prepare 后实例发生变化时拒绝陈旧确认；
- runtime 构建和内存替换成功路径各执行一次；
- runtime 构建失败后恢复旧数据库状态且不替换内存卡；
- 版本历史显示当前、升级、回滚和新安装动作；
- 不兼容确认框不宣称执行状态迁移；
- 卡片菜单向 Agent Studio 绑定 `baseCardId/baseVersionId`。

### 跨项目安全门禁

```text
env -u AGENTCARD_MODEL_API_KEY -u DEEPSEEK_API_KEY \
  sh tooling/security/run-security-gate.sh
```

结果：PASS。覆盖 Dart/TypeScript 共享合同、workflow policy、Flutter 制品/runtime/capability 安全测试、Go 全量测试、CodeCard 模板 typecheck/test/build/bundle policy、164 个模板依赖和 4 个浏览器门禁依赖的 high/critical 审计，以及 Chromium 3/3 离线、重启和鉴权场景。

## 未覆盖边界

- 未在 Windows 实机验证升级或回滚时主工作区、独立窗口和悬浮层的视觉连续性。
- 未验证 WebView2、Windows 多屏 DPI、IME、焦点和窗口层级。
- 未实现实例复制、删除、卸载以及本地数据保留选择。
- 未实现 Agent 生成或执行状态迁移；这是 MVP 明确禁止项，不是遗漏。
