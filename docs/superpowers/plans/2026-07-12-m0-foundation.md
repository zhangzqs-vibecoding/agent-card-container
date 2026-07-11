# M0 Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 建立可测试的 Flutter/Go 双项目骨架，验证共享协议和 Dart loopback Runtime Server，为 Windows WebView2 与多窗口实机闸门提供可运行基础。

**Architecture:** Flutter 客户端位于 apps/desktop，Go 模块化单体位于 services/cloud，共享 JSON Schema 和 fixtures 位于 contracts。M0 只实现最小垂直链路：桌面壳、健康 API、CardDefinition 契约、本地资源服务和安全会话；Windows WebView2 与悬浮窗口代码放在 adapter 边界后。

**Tech Stack:** Flutter 3.32.8、Dart 3.8.1、Go 1.26.3、JSON Schema、package:test、Go 标准库。

---

### Task 1: Repository baseline

**Files:**
- Create: .gitignore
- Create: AGENTS.md
- Create: README.md
- Create: docs/superpowers/plans/2026-07-12-m0-foundation.md

- [x] **Step 1: Initialize repository**

Run: git init -b develop

Expected: repository initialized on develop.

- [x] **Step 2: Verify baseline files**

Run: rg --files . | sort

Expected: design, plan, README, AGENTS and ignore files are present.

- [x] **Step 3: Commit baseline**

Run:

~~~bash
git add .gitignore AGENTS.md README.md docs
git commit -m "docs: establish agent card architecture"
~~~

Expected: one root commit on develop.

### Task 2: Scaffold Flutter and Go projects

**Files:**
- Create: apps/desktop/**
- Create: services/cloud/go.mod
- Create: services/cloud/cmd/agentcard/main.go
- Create: services/cloud/internal/app/app.go
- Test: services/cloud/internal/app/app_test.go

- [x] **Step 1: Generate Flutter project**

Run:

~~~bash
FLUTTER_NO_VERSION_CHECK=true flutter create --platforms=windows,linux --org dev.agentcard --project-name agent_card_desktop apps/desktop
~~~

Expected: Flutter desktop project with Windows and Linux runners.

- [x] **Step 2: Write failing Go app test**

The test constructs app.New("test") and expects Name() to return "test".

Run: cd services/cloud && go test ./internal/app

Expected: FAIL because package app does not exist.

- [x] **Step 3: Implement minimal Go app and command**

Create a focused app.App type with Name and Run methods. Run starts the HTTP server supplied by later tasks; the command currently parses AGENTCARD_ADDR and builds the app.

- [x] **Step 4: Verify projects**

Run:

~~~bash
cd services/cloud && go test ./...
cd ../../apps/desktop && FLUTTER_NO_VERSION_CHECK=true flutter test
~~~

Expected: both commands pass.

- [x] **Step 5: Commit scaffold**

Run:

~~~bash
git add apps services
git commit -m "build: scaffold desktop and cloud projects"
~~~

### Task 3: Shared CardDefinition contract

**Files:**
- Create: contracts/card/card-definition.schema.json
- Create: contracts/card/fixtures/native-card.json
- Create: contracts/card/fixtures/web-card.json
- Create: services/cloud/internal/contracts/card_definition.go
- Test: services/cloud/internal/contracts/card_definition_test.go
- Create: apps/desktop/lib/src/contracts/card_definition.dart
- Test: apps/desktop/test/contracts/card_definition_test.dart

- [x] **Step 1: Add valid fixtures and failing Go tests**

Tests decode both fixtures and assert runtime, version, size and capability fields. A separate test rejects an unknown runtime.

Run: cd services/cloud && go test ./internal/contracts

Expected: FAIL because CardDefinition and DecodeCardDefinition are missing.

- [x] **Step 2: Implement Go contract**

Implement strict JSON decoding with DisallowUnknownFields and explicit validation for formatVersion, runtime, identifiers, sizes and entrypoint.

Run: cd services/cloud && go test ./internal/contracts

Expected: PASS.

- [x] **Step 3: Add failing Dart contract tests**

Tests parse the same repository fixtures and assert identical semantics; invalid runtime throws FormatException.

Run: cd apps/desktop && FLUTTER_NO_VERSION_CHECK=true flutter test test/contracts/card_definition_test.dart

Expected: FAIL because CardDefinition is missing.

- [x] **Step 4: Implement Dart contract**

Implement immutable value types and strict fromJson validation without depending on code generation.

Run: cd apps/desktop && FLUTTER_NO_VERSION_CHECK=true flutter test test/contracts/card_definition_test.dart

Expected: PASS.

- [x] **Step 5: Commit contract**

Run:

~~~bash
git add contracts apps/desktop services/cloud
git commit -m "feat: define shared card contract"
~~~

### Task 4: Go health API

**Files:**
- Create: services/cloud/internal/httpapi/server.go
- Test: services/cloud/internal/httpapi/server_test.go
- Modify: services/cloud/internal/app/app.go
- Modify: services/cloud/cmd/agentcard/main.go

- [x] **Step 1: Write failing HTTP test**

Use httptest to call GET /healthz and assert status 200 plus JSON fields status=ok and service=agent-card-cloud.

Run: cd services/cloud && go test ./internal/httpapi

Expected: FAIL because NewHandler is missing.

- [x] **Step 2: Implement minimal handler**

Use net/http ServeMux, explicit methods, JSON content type and a stable response struct.

- [x] **Step 3: Verify**

Run:

~~~bash
cd services/cloud && go test ./...
go vet ./...
~~~

Expected: both commands pass.

- [x] **Step 4: Commit API**

Run:

~~~bash
git add services/cloud
git commit -m "feat: add cloud health api"
~~~

### Task 5: Dart Local Runtime Server

**Files:**
- Create: apps/desktop/lib/src/runtime/runtime_session.dart
- Create: apps/desktop/lib/src/runtime/local_runtime_server.dart
- Test: apps/desktop/test/runtime/local_runtime_server_test.dart

- [x] **Step 1: Write failing loopback tests**

Tests start the server, assert loopback address and nonzero random port, create two sessions, verify different hosts/tokens, reject an incorrect Host, and successfully fetch a registered in-memory index.html using the correct Host.

Run: cd apps/desktop && FLUTTER_NO_VERSION_CHECK=true flutter test test/runtime/local_runtime_server_test.dart

Expected: FAIL because LocalRuntimeServer is missing.

- [x] **Step 2: Implement server**

Use HttpServer.bind(InternetAddress.loopbackIPv4, 0, shared: false). Session identity is generated with Random.secure. Requests are accepted only for an active exact Host. Serve only registered immutable byte resources with explicit MIME, CSP, nosniff and no-referrer headers.

- [x] **Step 3: Add RPC rejection tests**

Tests verify missing/incorrect bearer token, Origin mismatch, expired session, oversized JSON and unknown method return stable JSON-RPC errors.

- [x] **Step 4: Implement minimal runtime methods**

Implement runtime.getContext and storage.get/set/delete/list with per-session in-memory storage and size checks. Persistent storage is added in M1.

- [x] **Step 5: Verify**

Run:

~~~bash
cd apps/desktop
dart format --output=none --set-exit-if-changed lib test
FLUTTER_NO_VERSION_CHECK=true flutter analyze
FLUTTER_NO_VERSION_CHECK=true flutter test
~~~

Expected: all commands pass.

- [x] **Step 6: Commit runtime**

Run:

~~~bash
git add apps/desktop
git commit -m "feat: add local card runtime server"
~~~

### Task 6: Desktop shell and M0 evidence

**Files:**
- Modify: apps/desktop/lib/main.dart
- Create: apps/desktop/lib/src/app/agent_card_app.dart
- Create: apps/desktop/lib/src/workspace/workspace_screen.dart
- Test: apps/desktop/test/app/agent_card_app_test.dart
- Create: docs/verification/m0-linux-evidence.md

- [x] **Step 1: Write failing widget test**

Pump AgentCardApp and assert the title, Agent panel affordance, empty workspace text, add-card action and runtime status indicator.

Run: cd apps/desktop && FLUTTER_NO_VERSION_CHECK=true flutter test test/app/agent_card_app_test.dart

Expected: FAIL because AgentCardApp is missing.

- [x] **Step 2: Implement desktop shell**

Build a Material 3 dark/light shell with navigation rail, an explicit empty 12-column workspace grid, collapsible Agent panel and runtime status. Keep state local and components focused.

- [x] **Step 3: Verify all M0 automated gates**

Run:

~~~bash
cd apps/desktop && FLUTTER_NO_VERSION_CHECK=true flutter analyze && FLUTTER_NO_VERSION_CHECK=true flutter test
cd ../../services/cloud && go test ./... && go vet ./...
~~~

Expected: all commands pass.

- [x] **Step 4: Record platform evidence**

Document the Linux-hosted automated results and explicitly list Windows-only WebView2, transparent overlay, DPI and IME checks as not yet executable on this host. Do not mark the Windows M0 gate passed.

- [x] **Step 5: Commit shell**

Run:

~~~bash
git add apps/desktop docs/verification
git commit -m "feat: add desktop workspace shell"
~~~
