# P1-A 工作区布局编辑验收记录

状态：**P1-A WORKSPACE HEADLESS PASS / WINDOWS DEVICE NOT RUN**  
记录日期：2026-07-15（Asia/Shanghai）  
验证提交：`855e1188194f015c2d596b5919ef048f18b4bd64`

## 结论

P1-A 的工作区布局编辑子项已在 Flutter/Linux headless 范围闭环：主工作区卡片支持 12 列网格拖动、缩放、确定性碰撞避让、300ms 防抖持久化、重启恢复和失败回滚。空状态生成入口也已接通 Agent Studio。

本记录不代表完整 P1-A 通过。复制实例、删除/卸载与数据保留选择，以及同卡生成、升级和回滚仍为开放项；Windows 鼠标、触控、DPI 与多屏体验尚未在参考设备执行。

## 实施提交

| 范围 | 提交 | 结果 |
|---|---|---|
| 工作区布局设计 | `03044d2` | PASS |
| TDD 实施计划 | `be05e7a` | PASS |
| 纯布局引擎 | `f97aed2` | PASS |
| 防抖持久化和失败回滚 | `4dabdd8` | PASS |
| SQLite 生产接线与重启恢复 | `691ef0b` | PASS |
| 拖动、缩放和碰撞 Widget | `06603a7` | PASS |
| 稳定错误提示和空状态入口 | `855e118` | PASS |

## 已证明行为

- placement 吸附到 12 列整数网格，宽高最小为 2×2，横向不会越界。
- 非有限数值和非正布局 fail closed；边界接触不算碰撞，面积相交才避让。
- 拖动或缩放发生碰撞时，只把当前卡片逐行向下移到第一个空位，不级联移动其他卡片。
- 拖动和缩放不会改变 `cardId`、`versionId`、`stateNamespace`、状态或授权。
- 连续编辑立即更新内存 UI，同实例只在 300ms 后持久化最后一个 placement。
- 持久化使用 revision 隔离；旧异步写入完成不能覆盖较新的编辑。
- SQLite 写入未知实例会失败，不再静默成功；关闭并重开数据库可以恢复 placement。
- 写入失败恢复最后成功位置，用户只看到“布局保存失败，已恢复上次位置”，底层异常正文不进入 UI。
- 空工作区“生成卡片”按钮会展开 Agent Studio 并聚焦 prompt 输入框。

## 自动化证据

| 门禁 | 结果 | 摘要 |
|---|---|---|
| Dart format | PASS | 124 files，0 changed |
| Flutter analyze | PASS | No issues found |
| Flutter tests | PASS | 238 passed，1 个既有预期 skip |
| 共享安全门 | PASS | Go race、Flutter 安全、跨语言合同、模板、Chromium 与依赖审计均通过 |
| Linux Chromium | PASS | 3/3 CodeCard 离线与 RPC 回归通过 |
| 依赖审计 | PASS | 模板 164 packages、浏览器 4 packages，high/critical 均为 0 |
| 仓库卫生 | PASS | `git diff --check`、敏感信息、运行数据和容器残留审计通过 |

首次共享安全门运行在 npm 官方 bulk advisory 请求处因瞬时 `fetch failed` 按 fail-closed 退出；未将该次结果计为通过。随后完整重跑成功，两个依赖集合均从官方端点取得零 high/critical 结果。

## 剩余闸门

1. P1-A 实例管理：复制、删除、卸载，以及保留数据/删除数据的明确选择。
2. P1-A 版本生命周期：`baseCardId/baseVersionId`、同卡新版本、能力差异、兼容状态复用、非兼容状态备份/重置和可恢复回滚。
3. Windows 11 x64 参考设备：实际鼠标/触控拖缩、150% DPI、多显示器和跨 Surface 状态保持。

在上述范围完成前，只能引用本记录的 **WORKSPACE HEADLESS PASS**，不得写成完整 `P1-A PASS`。
