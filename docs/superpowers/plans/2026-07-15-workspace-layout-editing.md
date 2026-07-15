# 工作区布局编辑实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Flutter 主工作区实现可测试的网格拖动、缩放、碰撞避让、300ms 持久化和失败回滚。

**Architecture:** 用纯 Dart `WorkspaceLayoutEngine` 统一布局规则；`WorkspaceController` 管理 revision、防抖持久化和回滚；`WorkspaceScreen` 只负责像素到网格 delta 的手势适配。生产持久化仍调用现有 `LocalDatabase.moveInstance`，不引入平台插件或第三方布局包。

**Tech Stack:** Dart 3、Flutter widget test、`ChangeNotifier`、`Timer`、SQLite FFI、现有 `CardPlacement`/`WorkspaceCard`。

---

## Task 1：纯布局引擎

**Files:**

- Create: `apps/desktop/lib/src/workspace/workspace_layout_engine.dart`
- Create: `apps/desktop/test/workspace/workspace_layout_engine_test.dart`

- [x] RED：测试 `resolve` 将小数吸附为整数、限制 12 列、最小 2×2，并拒绝 NaN/Infinity/非正尺寸。
- [x] 运行 `flutter test test/workspace/workspace_layout_engine_test.dart`，确认因 `WorkspaceLayoutEngine` 不存在而失败。
- [x] GREEN：实现 `WorkspaceLayoutEngine(columns: 12, minWidth: 2, minHeight: 2)` 和 `resolve(candidate, occupied)`；只返回有限的整数网格值。
- [x] RED：增加边界接触不碰撞、面积相交碰撞、忽略当前实例、逐行向下 first-fit 和从 `(0,0)` 自动落位测试。
- [x] GREEN：实现私有矩形相交与有限 first-fit；不移动其他实例。
- [x] 运行布局引擎测试和 `dart format`，确认全部通过。
- [x] 提交：`feat: add deterministic workspace layout engine`。

## Task 2：控制器的内存编辑与防抖持久化

**Files:**

- Modify: `apps/desktop/lib/src/workspace/workspace_controller.dart`
- Modify: `apps/desktop/test/workspace/workspace_controller_test.dart`

- [x] RED：构造带 `Future<void> Function(instanceId, surfaceId, placement)` 持久化端口的控制器，断言 `editPlacement` 立即更新内存，但在 299ms 前不写入，300ms 后连续更新只写最后值。
- [x] 运行 controller 测试，确认缺少 API 的预期失败。
- [x] GREEN：注入布局引擎、持久化端口和 debounce；为每个实例维护 timer、revision 和最后成功 placement。
- [x] RED：增加持久化失败恢复最后成功 placement、设置稳定 `layoutErrorMessage`，后续成功清错，以及旧异步 revision 完成不覆盖新编辑的测试。
- [x] GREEN：实现 revision 检查、失败回滚和错误 getter；不暴露底层异常正文。
- [x] RED：增加未知实例、非 workspace 实例、dispose 后 timer 不写入和不通知测试。
- [x] GREEN：fail closed 并在 dispose 取消 timer。
- [x] 运行 controller 测试和完整 workspace 测试。
- [x] 提交：`feat: persist recoverable workspace placement edits`。

## Task 3：SQLite 生产端口与重启恢复

**Files:**

- Modify: `apps/desktop/lib/src/storage/local_database.dart`
- Modify: `apps/desktop/test/storage/local_database_test.dart`
- Modify: `apps/desktop/lib/src/app/desktop_bootstrap.dart`
- Modify: `apps/desktop/test/app/desktop_bootstrap_test.dart`

- [x] RED：用临时数据库写入 placement、关闭并重开，断言同一实例恢复相同 surface/placement；未知实例更新必须失败而不是静默成功。
- [x] GREEN：让 `moveInstance` 检查 SQLite affected rows；不存在时抛稳定 `StateError`。
- [x] RED：bootstrap 测试断言工作区 controller 的防抖写入最终进入生产数据库。
- [x] GREEN：创建 controller 时注入 `database.moveInstance` 异步包装；SurfaceCoordinator 的外部移动继续使用不防抖的 `moveInstance`，避免回调循环。
- [x] 运行 storage/bootstrap 测试。
- [x] 提交：`feat: wire workspace placement persistence`。

## Task 4：拖动与缩放 Widget

**Files:**

- Modify: `apps/desktop/lib/src/workspace/workspace_screen.dart`
- Modify: `apps/desktop/test/app/agent_card_app_test.dart`

- [x] RED：widget test 用稳定 key 拖动 `card-drag-<instanceId>` 一个列宽和一行，断言 controller 得到吸附 placement，`versionId/stateNamespace` 不变。
- [x] GREEN：把 controller 编辑回调、列宽和行高传入 `_WorkspaceCardView`；标题拖动把手累计 delta 后调用 `editPlacement`。
- [x] RED：拖动到已占位置时断言向下 first-fit，不与另一卡重叠。
- [x] GREEN：复用布局引擎结果，不在 Widget 重写碰撞算法。
- [x] RED：拖动 `card-resize-<instanceId>`，断言尺寸吸附、最小 2×2、12 列边界和碰撞避让。
- [x] GREEN：增加右下角语义化缩放把手与 tooltip；手势开始固定初始 placement，update 使用累计 delta。
- [x] 运行相关 widget test，确认 NativeCard 和 CodeCard runtime 未因 placement-only 更新而重建。
- [x] 提交：`feat: add workspace drag and resize gestures`。

## Task 5：错误提示与空状态入口

**Files:**

- Modify: `apps/desktop/lib/src/workspace/workspace_controller.dart`
- Modify: `apps/desktop/lib/src/workspace/workspace_screen.dart`
- Modify: `apps/desktop/test/app/agent_card_app_test.dart`

- [ ] RED：模拟持久化失败，断言 Widget 回到旧位置，显示“布局保存失败，已恢复上次位置”，且不包含注入的数据库异常正文。
- [ ] GREEN：工作区监听 controller 稳定错误并提供关闭按钮；关闭只清 UI 错误，不改 placement。
- [ ] RED：空工作区点击 `generate-card-empty-state` 后 Agent Studio 从折叠状态展开并聚焦 prompt 输入框。
- [ ] GREEN：把空状态按钮接到现有 `onOpenAgentPanel`，删除空回调。
- [ ] 运行相关 widget test。
- [ ] 提交：`fix: surface recoverable workspace edit failures`。

## Task 6：验收与文档收口

**Files:**

- Create: `docs/verification/p1a-workspace-layout.md`
- Modify: `docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md`
- Modify: `docs/verification/m4-acceptance.md`
- Modify: `docs/superpowers/plans/2026-07-15-workspace-layout-editing.md`

- [ ] 运行 `dart format --output=none --set-exit-if-changed lib test`。
- [ ] 运行 `flutter analyze` 和完整 `flutter test`。
- [ ] 运行 `sh tooling/security/run-security-gate.sh` 和 `git diff --check`。
- [ ] 审计敏感信息、运行数据、构建和容器残留；测试用故意假凭据必须作为明确 fixture 处理。
- [ ] 记录精确 commit、测试数量、布局规则和未执行 Windows 项，状态只能是 `P1-A WORKSPACE HEADLESS PASS / WINDOWS DEVICE NOT RUN`。
- [ ] 更新路线图：仅关闭 P1-A 工作区布局子项，复制/删除/卸载和版本生命周期保持开放。
- [ ] 提交：`docs: record workspace layout headless evidence`，不 push。
