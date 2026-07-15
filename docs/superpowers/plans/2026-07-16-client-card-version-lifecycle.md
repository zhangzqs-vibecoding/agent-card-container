# Flutter 客户端卡片版本生命周期实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Flutter 客户端对同一 `cardId` 的已验证版本执行可恢复的升级和回滚，正确处理状态 schema、权限差异和内存 runtime 切换。

**Architecture:** 下载与验签仍由 `CardInstallCoordinator` 负责，但只登记目标安装，不提前改变实例。新增 `CardVersionLifecycle` 计算版本差异并调用 `LocalDatabase` 的单一事务入口；数据库完成状态备份、namespace 切换和 grants 替换后，客户端才通过 `InstalledWorkspaceCardFactory` 重建一次目标卡片并替换 `WorkspaceController` 中的同一实例。

**Tech Stack:** Flutter/Dart、SQLite FFI、flutter_test、现有 ArtifactInstaller、Capability Broker、WorkspaceController。

---

## 文件职责

- `apps/desktop/lib/src/storage/local_database.dart`：SQLite v5 migration、状态备份模型、事务化版本切换。
- `apps/desktop/lib/src/cloud/card_version_lifecycle.dart`：版本差异、授权决策和升级/回滚编排。
- `apps/desktop/lib/src/cloud/card_install_coordinator.dart`：下载并验签目标版本，但支持“仅登记安装”。
- `apps/desktop/lib/src/workspace/workspace_controller.dart`：成功后原位替换同一实例的 runtime card。
- `apps/desktop/lib/src/cloud/card_catalog_controller.dart`：向 UI 暴露实例版本切换状态和稳定错误。
- `apps/desktop/lib/src/workspace/workspace_screen.dart`：版本差异确认、升级/回滚入口。
- 对应 `apps/desktop/test/**`：数据库、服务、controller 和 widget 行为证据。

### Task 1：SQLite v5 状态备份

**Files:**
- Modify: `apps/desktop/lib/src/storage/local_database.dart`
- Modify: `apps/desktop/test/storage/local_database_test.dart`

- [x] **Step 1: 写 migration 和 snapshot 的失败测试**

测试新库 `schemaVersion == 5`，并用 `backupState(...)` 后断言 `stateBackups(instanceId, versionId)` 返回原 namespace、schema 和完整 JSON；构造超过 1 MiB 的 JSON 时断言抛出 `StateError('card state snapshot exceeds 1 MiB')`。

- [x] **Step 2: 运行测试确认失败**

Run: `cd apps/desktop && flutter test test/storage/local_database_test.dart`

Expected: FAIL，schema 仍为 4，备份 API 尚不存在。

- [x] **Step 3: 实现最小 v5 schema 和有界 API**

增加不可变 `CardStateBackup`；migration 创建：

```sql
CREATE TABLE card_state_backups (
  backup_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  card_id TEXT NOT NULL,
  version_id TEXT NOT NULL,
  state_schema_version INTEGER NOT NULL CHECK(state_schema_version > 0),
  state_namespace TEXT NOT NULL,
  snapshot_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX card_state_backups_lookup
ON card_state_backups(instance_id, version_id, created_at DESC);
```

`backupState` 使用规范 JSON 的 UTF-8 长度执行 1 MiB 上限，在调用者事务内写入；查询时严格解析为 `Map<String,Object?>`。

- [x] **Step 4: 增加每卡只保留最近 10 条测试和实现**

连续写入 11 条，断言最旧记录被删除；删除 SQL 按同一 `card_id` 的 `created_at DESC, backup_id DESC` 保留 10 条。

- [x] **Step 5: 格式化、测试并提交**

Run: `cd apps/desktop && dart format lib/src/storage/local_database.dart test/storage/local_database_test.dart && flutter test test/storage/local_database_test.dart`

Expected: PASS。

Commit: `feat: persist bounded card state backups`

### Task 2：数据库原子版本切换

**Files:**
- Modify: `apps/desktop/lib/src/storage/local_database.dart`
- Modify: `apps/desktop/test/storage/local_database_test.dart`

- [x] **Step 1: 写兼容 schema 切换失败测试**

准备同卡 v1/v2、实例状态和 v1 grants，调用 `switchInstalledInstanceVersion`。断言 version 改为 v2、namespace/state 不变，并且只有目标仍声明的 capability/domain 被复制为 v2 grant。

- [x] **Step 2: 写不兼容 schema 重置及恢复测试**

断言切换前产生 v1 备份、新 namespace 为空、旧 grants 清除；随后回滚 v1 时优先恢复最近 v1 备份。取消决策不得调用数据库入口，因此不产生任何变更。

- [x] **Step 3: 运行测试确认失败**

Run: `cd apps/desktop && flutter test test/storage/local_database_test.dart`

Expected: FAIL，事务入口尚不存在。

- [x] **Step 4: 实现单一事务入口**

新增 `VersionStatePolicy.reuse/reset/restore` 和 `switchInstalledInstanceVersion(...)`。入口必须验证实例、当前/目标安装均存在且 `card_id` 一致；在一个 `_connection.transaction` 内完成备份、目标 namespace 状态写入、实例 version/namespace 更新及该实例 grants 全量替换。任何异常必须回滚。

- [x] **Step 5: 注入中途失败并证明回滚**

测试传入无效 grant 或故意冲突 namespace，使事务在备份后失败；断言 instance、state、grants 和 backups 均保持原样。

- [x] **Step 6: 格式化、测试并提交**

Run: `cd apps/desktop && dart format lib/src/storage/local_database.dart test/storage/local_database_test.dart && flutter test test/storage/local_database_test.dart`

Expected: PASS。

Commit: `feat: switch installed card versions atomically`

### Task 3：版本差异与生命周期服务

**Files:**
- Create: `apps/desktop/lib/src/cloud/card_version_lifecycle.dart`
- Create: `apps/desktop/test/cloud/card_version_lifecycle_test.dart`
- Modify: `apps/desktop/lib/src/cloud/card_install_coordinator.dart`
- Modify: `apps/desktop/test/cloud/card_install_coordinator_test.dart`

- [ ] **Step 1: 写差异模型测试**

`CardVersionDifference` 必须精确给出 added/removed capabilities、added/removed network domains、当前与目标 schema，以及 `stateCompatible`。集合必须不可变且稳定排序供 UI 使用。

- [ ] **Step 2: 写 prepare 测试并确认失败**

`prepare(instanceId, InstalledArtifact target)` 应拒绝未登记、未验证、不同 card 或与当前相同 version；合法目标返回差异但不修改数据库。

Run: `cd apps/desktop && flutter test test/cloud/card_version_lifecycle_test.dart`

Expected: FAIL，生命周期服务尚不存在。

- [ ] **Step 3: 将下载验签与创建实例拆开**

在 `CardInstallCoordinator` 增加 `downloadAndRegisterCardVersion(cardId, versionId)`，复用现有下载、元数据核对、签名和文件校验，只调用 `registerInstallation` 并返回 `InstalledArtifact`；现有安装新实例流程调用该方法后保持原行为。

- [ ] **Step 4: 实现 prepare/apply**

`apply(prepared, decision, approvedGrants)` 再次核对实例当前版本以避免陈旧确认；新增能力/域必须完全被显式批准，批准范围不得超过目标 manifest。schema 相同使用 reuse；不同 schema 只有 reset 或可用备份时 restore。成功返回更新后的 `CardInstance`。

- [ ] **Step 5: 覆盖拒绝、陈旧确认、授权扩大和回滚恢复**

断言所有拒绝路径数据库零变更；旧 grant 只在目标仍覆盖时复制，新增项仅来自本次批准。

- [ ] **Step 6: 格式化、测试并提交**

Run: `cd apps/desktop && dart format lib/src/cloud test/cloud && flutter test test/cloud/card_install_coordinator_test.dart test/cloud/card_version_lifecycle_test.dart`

Expected: PASS。

Commit: `feat: orchestrate verified card version changes`

### Task 4：runtime 单次替换

**Files:**
- Modify: `apps/desktop/lib/src/workspace/workspace_controller.dart`
- Modify: `apps/desktop/test/workspace/workspace_controller_test.dart`
- Modify: `apps/desktop/lib/src/cloud/card_version_lifecycle.dart`
- Modify: `apps/desktop/test/cloud/card_version_lifecycle_test.dart`

- [ ] **Step 1: 写原位替换测试**

新增 `replaceInstance(WorkspaceCard card)`：必须要求 instanceId 已存在且 cardId 相同，保持列表位置，只通知一次；未知实例或不同 card 抛出稳定 `StateError`。

- [ ] **Step 2: 运行测试确认失败并实现最小方法**

Run: `cd apps/desktop && flutter test test/workspace/workspace_controller_test.dart`

Expected: 先 FAIL，实施后 PASS。

- [ ] **Step 3: 生命周期成功后才创建目标 runtime**

生命周期服务在数据库提交后调用 `InstalledWorkspaceCardFactory.create(target, updatedInstance)`，再调用 controller 的 `replaceInstance`。数据库失败时 factory 调用次数为 0；成功时 factory 和 replace 各为 1。

- [ ] **Step 4: 处理提交后 runtime 构建失败**

在内存替换前若 factory 失败，使用同一数据库入口回切原版本和原状态快照；若补偿也失败，抛出 `CardVersionLifecycleException('VERSION_SWITCH_RECOVERY_FAILED')`，不得伪装成功。

- [ ] **Step 5: 格式化、测试并提交**

Run: `cd apps/desktop && dart format lib/src/workspace lib/src/cloud test/workspace test/cloud && flutter test test/workspace/workspace_controller_test.dart test/cloud/card_version_lifecycle_test.dart`

Expected: PASS。

Commit: `feat: replace upgraded card runtime once`

### Task 5：Controller 与版本历史 UI

**Files:**
- Modify: `apps/desktop/lib/src/cloud/card_catalog_controller.dart`
- Modify: `apps/desktop/test/cloud/card_catalog_controller_test.dart`
- Modify: `apps/desktop/lib/src/workspace/workspace_screen.dart`
- Modify: `apps/desktop/test/workspace/workspace_screen_test.dart`

- [ ] **Step 1: 写 controller 状态测试**

选择一个现有同卡实例后，版本历史应区分当前、可升级和可回滚；prepare/apply 期间禁用重复操作；拒绝不视为错误，异常映射为稳定中文恢复提示。

- [ ] **Step 2: 写差异确认 widget 测试**

对话框展示新增/移除 capability、domain 和 schema 结论。兼容 schema 提供确认/取消；不兼容 schema 只能选择“继续使用当前版本”或“安装并重置”，不得出现“迁移”。

- [ ] **Step 3: 运行测试确认失败**

Run: `cd apps/desktop && flutter test test/cloud/card_catalog_controller_test.dart test/workspace/workspace_screen_test.dart`

Expected: FAIL，现有 UI 只有“安装此版本”。

- [ ] **Step 4: 实现最小 controller/UI 接线**

版本按钮按目标与当前 display version 显示“升级到此版本”或“回滚到此版本”；无同卡实例时保留“安装此版本”。用户确认后才调用 apply，成功 snackbar 显示目标版本，失败保留旧卡。

- [ ] **Step 5: 格式化、测试并提交**

Run: `cd apps/desktop && dart format lib/src/cloud lib/src/workspace test/cloud test/workspace && flutter test test/cloud/card_catalog_controller_test.dart test/workspace/workspace_screen_test.dart`

Expected: PASS。

Commit: `feat: expose card upgrade and rollback controls`

### Task 6：基于当前版本修改入口

**Files:**
- Modify: `apps/desktop/lib/src/generation/generation_controller.dart`
- Modify: `apps/desktop/lib/src/generation/agent_studio_screen.dart`
- Modify: `apps/desktop/lib/src/workspace/workspace_screen.dart`
- Modify: corresponding tests under `apps/desktop/test/generation` and `apps/desktop/test/workspace`

- [ ] **Step 1: 写 base identity controller 测试**

`startFromVersion(cardId, versionId, displayVersion)` 设置成对 base IDs；新建普通卡时清空；确认区显示基线；创建 generation 请求携带字段。

- [ ] **Step 2: 写卡片菜单 widget 测试**

“基于此版本修改”从当前实例打开 Agent Studio，绑定当前 `cardId/versionId`；重复进入不会沿用上一次其他卡片的基线。

- [ ] **Step 3: 运行失败测试并实现最小接线**

Run: `cd apps/desktop && flutter test test/generation test/workspace`

Expected: 先 FAIL，实施后 PASS。

- [ ] **Step 4: 格式化、测试并提交**

Run: `cd apps/desktop && dart format lib test && flutter test test/generation test/workspace`

Expected: PASS。

Commit: `feat: iterate cards from installed versions`

### Task 7：全量 headless 闸门与证据

**Files:**
- Create: `docs/verification/p1a-client-version-lifecycle.md`
- Modify: `docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md`
- Modify: `docs/superpowers/specs/2026-07-15-card-version-lifecycle-design.md`

- [ ] **Step 1: 运行 Flutter 全门禁**

Run: `cd apps/desktop && dart format --output=none --set-exit-if-changed lib test && flutter analyze && flutter test`

Expected: format 0 changes、analyze 0 issues、全部 headless 测试通过，Windows 专属测试仅允许已有明确 skip。

- [ ] **Step 2: 运行跨项目安全门禁**

Run: `env -u AGENTCARD_MODEL_API_KEY -u DEEPSEEK_API_KEY sh tooling/security/run-security-gate.sh`

Expected: Go、Dart、TypeScript 合同、离线 CodeCard 和依赖审计全部通过；输出不含密钥。

- [ ] **Step 3: 记录证据和边界**

报告逐项列出 migration、备份上限、兼容切换、不兼容取消/重置、grant 收缩、事务失败回滚、runtime 单次重建、UI 升级/回滚测试。只有全部通过时标记 `P1-A VERSION LIFECYCLE HEADLESS PASS`；Windows 三 Surface、DPI、多屏、IME 继续标记 `DEVICE NOT RUN`。

- [ ] **Step 4: 扫描敏感信息和运行残留**

Run: `git status --short && git grep -nE 'sk-[A-Za-z0-9]{16,}|BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY' -- . ':!docs/superpowers/plans/2026-07-16-client-card-version-lifecycle.md'`

Expected: 无密钥、私钥、数据库、日志或构建产物待提交。

- [ ] **Step 5: 提交验证文档**

Commit: `docs: record client version lifecycle evidence`
