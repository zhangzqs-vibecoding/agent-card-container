# Desktop Cloud Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add secure, restart-applied Go cloud configuration to the Flutter desktop settings page and verify it against a deployed test server.

**Architecture:** Keep `DesktopBootstrap` as the single startup composition root. A repository stores only non-secret JSON, a `SecretStore` adapter stores the token in the operating-system credential vault, and a service resolves environment overrides, tests candidate connections, and performs transactional save/clear operations. The settings UI edits a controller-owned draft and requests a safe process restart only after a successful save.

**Tech Stack:** Flutter/Dart, `flutter_secure_storage`, `dart:io` HTTP and process APIs, Flutter widget tests, Go cloud service, SSH deployment.

---

### Task 1: Configuration model and non-secret repository

**Files:**
- Create: `apps/desktop/lib/src/cloud/cloud_connection_settings.dart`
- Create: `apps/desktop/lib/src/cloud/cloud_settings_repository.dart`
- Test: `apps/desktop/test/cloud/cloud_settings_repository_test.dart`

- [ ] **Step 1: Write failing tests** for HTTPS validation, explicitly enabled loopback HTTP, trusted-key parsing, corrupt-file recovery, atomic replacement, and proof that serialized JSON contains no access token field or value.
- [ ] **Step 2: Run RED:** `cd apps/desktop && flutter test test/cloud/cloud_settings_repository_test.dart`; expect missing model/repository failures.
- [ ] **Step 3: Implement the minimal immutable settings types and repository.** Use schema version 1, bounded file reads, a same-directory temporary file, flush, rename/replace, restrictive validation, and no secret-valued constructor parameter in repository serialization.
- [ ] **Step 4: Run GREEN:** rerun the targeted test and `dart format lib/src/cloud test/cloud`.
- [ ] **Step 5: Commit:** `feat: persist non-secret cloud settings`.

### Task 2: Secure token adapter and source resolution

**Files:**
- Modify: `apps/desktop/pubspec.yaml`
- Modify: `apps/desktop/pubspec.lock`
- Create: `apps/desktop/lib/src/adapters/secure_token_store.dart`
- Create: `apps/desktop/lib/src/cloud/cloud_settings_service.dart`
- Test: `apps/desktop/test/cloud/cloud_settings_service_test.dart`

- [ ] **Step 1: Write failing tests** using an in-memory `SecretStore`: complete environment configuration wins; environment and user sources never mix; user config requires a secure token; secret-store read failure returns an offline/settings-visible state; token is absent from exceptions and diagnostics metadata.
- [ ] **Step 2: Run RED:** `cd apps/desktop && flutter test test/cloud/cloud_settings_service_test.dart`; expect missing service failures.
- [ ] **Step 3: Add the compatible pinned `flutter_secure_storage` dependency** and implement `SecretStore`, its plugin-backed adapter, and environment/user/none resolution. Platform plugin imports remain in `src/adapters`.
- [ ] **Step 4: Run GREEN** for the service tests and `flutter analyze`.
- [ ] **Step 5: Commit:** `feat: store cloud access tokens securely`.

### Task 3: Authenticated connection test and transactional save/clear

**Files:**
- Modify: `apps/desktop/lib/src/cloud/cloud_api_client.dart`
- Modify: `apps/desktop/lib/src/cloud/cloud_settings_service.dart`
- Test: `apps/desktop/test/cloud/cloud_api_client_test.dart`
- Test: `apps/desktop/test/cloud/cloud_settings_service_test.dart`

- [ ] **Step 1: Write failing API tests** with a loopback HTTP server showing `testConnection` accepts only the expected `/healthz` payload and a successful authenticated `/v1/cards` response, rejects redirect/auth/protocol errors, and never sends authorization to another origin.
- [ ] **Step 2: Run RED:** targeted API tests must fail because `testConnection` does not exist.
- [ ] **Step 3: Implement the bounded health request plus existing authenticated list call** with stable redacted error codes.
- [ ] **Step 4: Write failing service tests** showing save validates and tests before mutation, secure-store failure preserves old files and token, repository failure restores the prior token, and clear reports partial failure accurately.
- [ ] **Step 5: Implement transactional save/clear orchestration** with rollback and no plaintext fallback; rerun targeted tests to GREEN.
- [ ] **Step 6: Commit:** `feat: validate and save cloud connections`.

### Task 4: Bootstrap consumes user settings

**Files:**
- Modify: `apps/desktop/lib/src/app/desktop_bootstrap.dart`
- Modify: `apps/desktop/lib/main.dart`
- Test: `apps/desktop/test/app/desktop_bootstrap_test.dart`

- [ ] **Step 1: Write failing bootstrap tests** showing restart loads user JSON plus secure token, environment remains authoritative, corrupt/missing secure data keeps the app offline, and trusted keys reach the installer without exposing their contents in diagnostics.
- [ ] **Step 2: Run RED:** targeted bootstrap tests must fail because bootstrap reads only process environment.
- [ ] **Step 3: Inject/create `CloudSettingsService` at the composition root** and replace `_cloudConfiguration(environment, ...)` with the resolved `CloudConnectionSettings`; preserve the current one-time controller lifecycle.
- [ ] **Step 4: Run GREEN:** bootstrap tests, format, and analyze.
- [ ] **Step 5: Commit:** `feat: load saved cloud settings at startup`.

### Task 5: Settings controller and UI

**Files:**
- Create: `apps/desktop/lib/src/cloud/cloud_settings_controller.dart`
- Create: `apps/desktop/lib/src/cloud/cloud_settings_section.dart`
- Modify: `apps/desktop/lib/src/workspace/workspace_screen.dart`
- Modify: `apps/desktop/lib/src/app/agent_card_app.dart`
- Modify: `apps/desktop/lib/main.dart`
- Test: `apps/desktop/test/cloud/cloud_settings_controller_test.dart`
- Test: `apps/desktop/test/app/agent_card_app_test.dart`

- [ ] **Step 1: Write failing controller tests** for initial loading, environment-managed read-only state, hidden saved token, validation, test status, save-pending-restart status, clear, and secret-store failure messaging.
- [ ] **Step 2: Run RED, then implement the minimal `ChangeNotifier` controller** without platform dependencies and run GREEN.
- [ ] **Step 3: Write failing widget tests** for navigation to settings, form fields, password visibility, advanced trusted-key editor, test/save/clear buttons, environment lock state, and diagnostic export remaining available.
- [ ] **Step 4: Implement a focused scrollable `CloudSettingsSection`** and compose it above diagnostics; never render or retain the existing stored token text.
- [ ] **Step 5: Run widget tests and analyze to GREEN.**
- [ ] **Step 6: Commit:** `feat: configure cloud service from desktop settings`.

### Task 6: Safe restart adapter

**Files:**
- Create: `apps/desktop/lib/src/adapters/application_restarter.dart`
- Modify: `apps/desktop/lib/src/cloud/cloud_settings_controller.dart`
- Modify: `apps/desktop/lib/src/cloud/cloud_settings_section.dart`
- Modify: `apps/desktop/lib/main.dart`
- Test: `apps/desktop/test/adapters/application_restarter_test.dart`
- Test: `apps/desktop/test/cloud/cloud_settings_controller_test.dart`

- [ ] **Step 1: Write failing tests** proving the adapter starts the same executable detached and controller invokes normal runtime shutdown only after launch succeeds; launch failure leaves the current process active.
- [ ] **Step 2: Run RED, implement the injectable process-launch boundary, and run GREEN.**
- [ ] **Step 3: Wire “立即重启” and “稍后重启” to the saved state**, with no restart option before a successful save.
- [ ] **Step 4: Run targeted widget/controller tests and commit:** `feat: restart after cloud configuration changes`.

### Task 7: Documentation and full local verification

**Files:**
- Modify: `apps/desktop/README.md`
- Modify: `README.md`
- Modify: `docs/verification/m4-acceptance.md`

- [ ] **Step 1: Document GUI setup, environment precedence, secure-store behavior, local HTTP restriction, and restart requirement** without sample real secrets.
- [ ] **Step 2: Run fresh gates:** `dart format --output=none --set-exit-if-changed apps/desktop/lib apps/desktop/test`; `cd apps/desktop && flutter analyze && flutter test`; `sh tooling/security/run-security-gate.sh`.
- [ ] **Step 3: Scan tracked files and history for passwords/tokens/private keys** and record only pass/fail evidence.
- [ ] **Step 4: Commit:** `docs: document desktop cloud configuration`.

### Task 8: Test-server deployment and end-to-end verification

**Files:**
- Modify: `docs/verification/m4-acceptance.md`

- [ ] **Step 1: Inspect the remote host read-only** for OS, architecture, Docker/Go availability, ports, firewall, and existing services. Do not overwrite unrelated state.
- [ ] **Step 2: Build or upload a pinned cloud binary/config** using a dedicated `/opt/agent-card-container` directory and a systemd service. Generate development authentication/signing material on the server; never copy it into the repository or command output.
- [ ] **Step 3: Start the service and verify locally on the server:** health endpoint, authenticated card list, service status, and restart persistence.
- [ ] **Step 4: Verify from the development host** that the advertised server URL is reachable. Use the Flutter service/controller integration test against the remote API without printing the token.
- [ ] **Step 5: Build the Windows portable artifact through CI, download it, run the package verifier, and record the run/artifact URLs.**
- [ ] **Step 6: Update acceptance evidence with redacted server/CI results, run documentation checks, and commit:** `docs: record cloud settings acceptance evidence`.
