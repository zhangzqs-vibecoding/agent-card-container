# M1 NativeCard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 实现 NativeCard v1 的可信协议、表达式与动作、Flutter 渲染器、能力代理和本地实例持久化，使固定 NativeCard 能在工作区离线交互。

**Architecture:** contracts/card/native-card.schema.json 是 NativeCard 协议来源。Flutter 解析后构造不可变节点树，由 catalog renderer 映射为预编译 Widget；状态和动作由 NativeCardController 管理，系统能力只能进入 CapabilityBroker。实例、Surface、授权和状态由主 Engine 的 SQLite repository 保存。

**Tech Stack:** Flutter、Dart、JSON Schema、sqlite3、path_provider、Flutter widget tests。

---

### Task 1: NativeCard schema and parser

**Files:**
- Create: contracts/card/native-card.schema.json
- Create: contracts/card/fixtures/pomodoro-native.json
- Create: apps/desktop/lib/src/native_card/native_card_spec.dart
- Test: apps/desktop/test/native_card/native_card_spec_test.dart

- [x] **Step 1: Write fixture and failing parser tests**

Tests parse a Container/Column/Text/Button card, reject unknown components, reject trees deeper than 32 and reject more than 500 nodes.

- [x] **Step 2: Run red test**

Run: cd apps/desktop && flutter --no-version-check test test/native_card/native_card_spec_test.dart

Expected: FAIL because NativeCardSpec is missing.

- [x] **Step 3: Implement strict parser**

Implement NativeCardSpec, NativeNode, BindingValue and NativeAction with explicit allowed keys and component type enum.

- [x] **Step 4: Verify and commit**

Run: flutter --no-version-check analyze && flutter --no-version-check test

Expected: PASS.

Commit: feat: define native card schema

### Task 2: Expression and local action engine

**Files:**
- Create: apps/desktop/lib/src/native_card/expression.dart
- Create: apps/desktop/lib/src/native_card/native_card_controller.dart
- Test: apps/desktop/test/native_card/expression_test.dart
- Test: apps/desktop/test/native_card/native_card_controller_test.dart

- [x] **Step 1: Write failing expression tests**

Cover path lookup, arithmetic, comparison, boolean and string formatting. Unknown op, recursion beyond 32 and wrong operand types throw NativeCardEvaluationException.

- [x] **Step 2: Implement minimal expression evaluator**

Use a closed switch over op names; never use eval, mirrors or dynamic function lookup.

- [x] **Step 3: Write failing action tests**

Cover set, increment, toggle, append, remove, startTimer and stopTimer against isolated instance state.

- [x] **Step 4: Implement controller and verify**

Use immutable state snapshots and StreamController broadcasts. Timers are cancelled on dispose.

Commit: feat: add native card state engine

### Task 3: Trusted Flutter renderer

**Files:**
- Create: apps/desktop/lib/src/native_card/native_card_renderer.dart
- Create: apps/desktop/lib/src/native_card/catalog_renderer.dart
- Test: apps/desktop/test/native_card/native_card_renderer_test.dart

- [x] **Step 1: Write failing widget tests**

Cover Container, Row, Column, Stack, Grid, Scroll, Divider, Text, Icon, Badge, Progress, Button, TextInput, Checkbox, Select, Slider, List, KeyValue, EmptyState and ErrorState.

- [x] **Step 2: Implement catalog renderer**

Each component has one focused render method. Bound values resolve from controller state. Unknown components cannot reach rendering because parsing rejects them.

- [x] **Step 3: Verify interactions**

Tap Button and edit fields, then assert controller state and rebuilt text.

Commit: feat: render trusted native cards

### Task 4: Capability Broker

**Files:**
- Create: apps/desktop/lib/src/capabilities/capability.dart
- Create: apps/desktop/lib/src/capabilities/capability_broker.dart
- Test: apps/desktop/test/capabilities/capability_broker_test.dart

- [x] **Step 1: Write failing policy tests**

Reject undeclared capability, missing grant, wrong instance/version, expanded network domain and clipboard.read without user gesture.

- [x] **Step 2: Implement invoke boundary**

Expose invoke(CardContext, method, params). Register typed handlers for storage, window.manageSelf and safe host actions; other adapters return CAPABILITY_UNAVAILABLE.

- [x] **Step 3: Connect NativeCard capability.invoke**

Controller passes its immutable CardContext to the broker and maps stable error codes into card state.

Commit: feat: enforce card capabilities

### Task 5: Surface domain and SQLite persistence

**Files:**
- Modify: apps/desktop/pubspec.yaml
- Create: apps/desktop/lib/src/surfaces/surface.dart
- Create: apps/desktop/lib/src/cards/card_instance.dart
- Create: apps/desktop/lib/src/storage/local_database.dart
- Test: apps/desktop/test/storage/local_database_test.dart
- Modify: apps/desktop/lib/src/workspace/workspace_screen.dart

- [x] **Step 1: Add SQLite runtime binding**

The configured pub mirror was unavailable, so M1 uses a minimal Dart FFI binding
to the operating-system SQLite library without adding third-party packages.
Linux tests load `libsqlite3.so.0`; Windows packaging must bundle and verify
`sqlite3.dll` before the Windows release gate can pass.

Expected: pubspec and lock contain compatible pinned versions.

- [x] **Step 2: Write failing repository tests**

Cover CardInstallation, CardInstance, workspace/overlay/detached Surface, PermissionGrant, placement updates, state namespaces and atomic version switches.

- [x] **Step 3: Implement schema and repositories**

Only the main Engine opens the database for writes. Enable foreign keys and WAL. Apply numbered migrations in one transaction.

- [x] **Step 4: Render a persisted NativeCard in the workspace**

Replace the empty state when repository contains an instance; preserve the empty state for a fresh database.

- [x] **Step 5: Verify and commit**

Run: flutter analyze, flutter test and flutter build bundle.

Commit: feat: persist card instances and surfaces
