# M4 Product Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将已完成的桌面端与云端纵向链路硬化为可诊断、可恢复、可打包，并具有可重复安全与性能证据的 Windows-first MVP。

**Architecture:** 诊断数据由独立 Dart 服务采集并在导出前做 allowlist/redaction，不读取卡片私有状态。安全与性能闸门以可重复命令和机器可读报告实现；Windows 安装器只打包已验证的 release bundle，并在构建或运行前拒绝缺失 WebView2、SQLite 或 libsodium 的环境。平台实机证据与 Linux 自动化证据分开记录，不能互相替代。

**Tech Stack:** Flutter/Dart、Go、SQLite、WebView2、Inno Setup、PowerShell、GitHub Actions、Flutter integration_test。

---

### Task 1: Redacted diagnostics export

**Files:**
- Create: `apps/desktop/lib/src/diagnostics/diagnostic_bundle.dart`
- Create: `apps/desktop/test/diagnostics/diagnostic_bundle_test.dart`
- Modify: `apps/desktop/lib/src/app/desktop_bootstrap.dart`

- [x] **Step 1: Write failing tests for allowlisted diagnostics**

```dart
test('exports runtime metadata without secrets paths or card state', () {
  final bundle = DiagnosticBundleBuilder().build(
    runtime: {'version': '1.0.0', 'apiToken': 'secret'},
    errors: [r'C:\Users\alice\cards\x failed Authorization: Bearer abc'],
  );
  expect(bundle, contains('1.0.0'));
  expect(bundle, isNot(contains('secret')));
  expect(bundle, isNot(contains('alice')));
  expect(bundle, isNot(contains('Bearer abc')));
});
```

- [x] **Step 2: Run the diagnostic test and confirm it fails**

Run: `cd apps/desktop && flutter test test/diagnostics/diagnostic_bundle_test.dart`

Expected: FAIL because `DiagnosticBundleBuilder` does not exist.

- [x] **Step 3: Implement an allowlist-only JSON bundle and redactor**

```dart
class DiagnosticBundleBuilder {
  String build({required Map<String, Object?> runtime, required List<String> errors}) {
    return jsonEncode({
      'schemaVersion': 1,
      'runtime': {'version': runtime['version']},
      'errors': errors.map(redactDiagnosticText).toList(),
    });
  }
}
```

The redactor must replace bearer tokens, authorization headers, API-key-shaped values, Windows user paths and POSIX home paths. It must never accept card state, permission grant values or request bodies as input fields.

- [x] **Step 4: Run formatting, analysis and tests**

Run: `cd apps/desktop && dart format lib test && flutter analyze && flutter test test/diagnostics/diagnostic_bundle_test.dart`

Expected: PASS and no analyzer findings.

- [x] **Step 5: Commit**

```bash
git add apps/desktop
git commit -m "feat: export redacted diagnostics"
```

### Task 2: Crash recovery and persistent launch quarantine

**Files:**
- Create: `apps/desktop/lib/src/recovery/startup_recovery.dart`
- Create: `apps/desktop/test/recovery/startup_recovery_test.dart`
- Modify: `apps/desktop/lib/src/app/desktop_bootstrap.dart`
- Modify: `apps/desktop/lib/src/code_card/code_card_host.dart`
- Modify: `apps/desktop/lib/src/storage/local_database.dart`
- Modify: `apps/desktop/lib/src/surfaces/surface_coordinator.dart`

- [x] **Step 1: Write failing tests for unclean shutdown and quarantine**

```dart
test('an unclean previous run is reported without blocking recovery', () {
  final recovery = StartupRecovery.start(markerFile);
  expect(recovery.previousRunUnclean, isTrue);
});
```

- [x] **Step 2: Run the recovery test and confirm it fails**

Run: `cd apps/desktop && flutter test test/recovery/startup_recovery_test.dart`

Expected: FAIL because startup recovery is not implemented.

- [x] **Step 3: Implement a durable run marker and per-card failure counters**

```dart
class StartupRecovery {
  final bool previousRunUnclean;
  void markClean() {}
}
```

Bootstrap writes the running marker before mounting cards and removes it only during orderly shutdown. SQLite schema v4 persists per-instance launch failures. Main-workspace and child-surface CodeCards clear the counter on success and quarantine that instance after three consecutive mount failures. An unclean previous run is surfaced in redacted recovery diagnostics and does not prevent unaffected cards from restoring.

- [x] **Step 4: Run recovery and existing startup tests**

Run: `cd apps/desktop && flutter test test/recovery test/app/desktop_bootstrap_test.dart test/code_card/code_card_host_test.dart && flutter analyze`

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add apps/desktop
git commit -m "feat: recover safely from desktop crashes"
```

### Task 3: Malicious artifact and runtime regression gate

**Files:**
- Modify: `apps/desktop/test/artifacts/zip_archive_test.dart`
- Existing gate: `apps/desktop/test/runtime/local_runtime_server_test.dart`
- Existing gate: `apps/desktop/test/capabilities/secure_network_fetcher_test.dart`
- Create: `tooling/security/run-security-gate.sh`
- Modify: `apps/desktop/lib/src/artifacts/zip_archive.dart`

- [x] **Step 1: Add failing adversarial cases**

```dart
for (final fixture in ['zip-slip', 'zip-bomb', 'unsigned', 'tampered', 'symlink']) {
  test('rejects $fixture artifact', () async {
    await expectLater(installFixture(fixture), throwsA(isA<ArtifactInstallException>()));
  });
}
```

Also cover remote navigation, popup, download, camera/microphone/location permission, cross-session RPC, forged Origin/Host, private DNS answers, redirects and bodies over their configured limits.

- [x] **Step 2: Run the security suite and record every initial failure**

Run: `cd apps/desktop && flutter test test/artifacts test/runtime test/capabilities`

Expected: at least the newly introduced unsupported adversarial cases fail.

- [x] **Step 3: Implement only the missing bounded checks**

Artifact extraction must reject absolute paths, `..`, symlinks, duplicate normalized paths, excessive file count, excessive single-file size and excessive expanded total bytes before writing into the content-addressed cache.

- [x] **Step 4: Add one deterministic security-gate command**

```sh
#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
(cd apps/desktop && flutter test test/security test/artifacts test/runtime test/capabilities)
(cd services/cloud && go test ./... -race)
```

- [x] **Step 5: Run and commit**

Run: `sh tooling/security/run-security-gate.sh`

Expected: PASS.

```bash
git add apps/desktop tooling/security
git commit -m "test: gate malicious card artifacts"
```

### Task 4: Repeatable performance harness

**Files:**
- Create: `apps/desktop/integration_test/performance_baseline_test.dart`
- Create: `apps/desktop/lib/src/diagnostics/performance_sample.dart`
- Create: `tooling/performance/summarize.dart`
- Create: `docs/verification/performance-baseline-template.md`

- [x] **Step 1: Test percentile and budget evaluation**

```dart
test('reports p50 p95 and failed budgets', () {
  final report = PerformanceReport.fromSamples([100, 200, 300, 400, 500]);
  expect(report.p50, 300);
  expect(report.p95, 500);
  expect(report.withinBudget(p95: 450), isFalse);
});
```

- [x] **Step 2: Run and confirm the missing report fails**

Run: `cd apps/desktop && flutter test test/diagnostics/performance_sample_test.dart`

Expected: FAIL because the report type is missing.

- [x] **Step 3: Implement deterministic sampling and scenarios**

The integration harness must emit JSON for cold startup, cached NativeCard first frame, cached CodeCard first frame, 1/5/20 NativeCards, 1/3/8 CodeCards and frame timings while moving/resizing. It records the host description and never hard-codes a passing result.

- [x] **Step 4: Run host-independent evaluator tests**

Run: `cd apps/desktop && flutter test test/diagnostics/performance_sample_test.dart`

Expected: PASS. Windows device measurements remain pending until run on the reference class of device.

- [x] **Step 5: Commit**

```bash
git add apps/desktop tooling/performance docs/verification
git commit -m "test: add desktop performance baseline harness"
```

### Task 5: Windows packaging and prerequisite gate

**Files:**
- Create: `packaging/windows/agent-card-container.iss`
- Create: `packaging/windows/verify-release.ps1`
- Create: `packaging/windows/README.md`
- Create: `.github/workflows/windows-release-gate.yml`

- [x] **Step 1: Write release verification checks**

```powershell
$required = @('agent_card_desktop.exe', 'flutter_windows.dll', 'sqlite3.dll', 'libsodium.dll')
foreach ($name in $required) {
  if (-not (Test-Path (Join-Path $Bundle $name))) { throw "Missing $name" }
}
if (-not (Get-AuthenticodeSignature (Join-Path $Bundle 'agent_card_desktop.exe')).Status -eq 'Valid') {
  throw 'Desktop executable is not signed'
}
```

The script must also verify the package signature, version, stable upgrade AppId, absence of `.env`/database/log/private-key files, and WebView2 Runtime detection behavior.

- [x] **Step 2: Add the Inno Setup definition**

Use per-user installation, a stable AppId, semantic `AppVersion`, atomic replacement, uninstall entries and explicit preservation/removal choices for user data. If Evergreen WebView2 is missing, show a link to Microsoft's official installer and abort rather than silently installing an unverified binary.

- [x] **Step 3: Add a Windows CI release gate**

The workflow runs `flutter test`, `flutter analyze`, `flutter build windows --release`, copies approved native DLLs, signs when protected signing secrets are available, runs `verify-release.ps1`, builds the installer, and uploads checksums plus evidence. Pull requests run unsigned structural checks but cannot be marked release-ready.

- [ ] **Step 4: Run static checks on this host and device checks on Windows**

Run on Linux: `git diff --check && rg -n "AppId|AppVersion|WebView2|libsodium.dll|sqlite3.dll" packaging/windows`

Run on Windows: `pwsh packaging/windows/verify-release.ps1 -Bundle apps/desktop/build/windows/x64/runner/Release`

Expected: Linux static checks pass; Windows release verification must pass before this task is checked complete.

- [x] **Step 5: Commit**

```bash
git add packaging .github/workflows/windows-release-gate.yml
git commit -m "build: gate signed Windows releases"
```

### Task 6: Platform device gates and evidence

**Files:**
- Create: `tooling/device-gates/windows-m0.ps1`
- Create: `tooling/device-gates/macos-m4.sh`
- Create: `tooling/device-gates/linux-m4.sh`
- Create: `docs/verification/windows-m0-m4-evidence-template.md`
- Create: `docs/verification/macos-m4-evidence-template.md`
- Modify: `docs/verification/m0-linux-evidence.md`

- [x] **Step 1: Encode device matrices as fail-closed scripts**

Windows must exercise WebView2 random localhost hostnames, storage isolation, Chinese IME, 150% mixed DPI, transparent overlay, click-through recovery, multi-monitor unplug, sleep/lock, offline restart, 1/3/10 windows and 1/5/20 cards. macOS must exercise WKWebView loopback access, signing/notarization entitlements and normal/detached windows. Linux must verify only ordinary windows and explicitly skip unsupported Wayland overlay guarantees.

- [x] **Step 2: Make every evidence record machine-readable**

Each script writes JSON containing commit SHA, OS/build, CPU, memory, GPU, display scale, runtime versions, scenario result, duration and artifact hashes. Missing or skipped mandatory Windows/macOS scenarios must produce a non-zero exit.

- [x] **Step 3: Run the Linux gate**

Run: `sh tooling/device-gates/linux-m4.sh`

Expected: ordinary-window build/tests pass; Wayland overlay is recorded out of scope rather than passed.

- [ ] **Step 4: Run Windows and macOS gates on their target devices**

Run: `pwsh tooling/device-gates/windows-m0.ps1` and `sh tooling/device-gates/macos-m4.sh`.

Expected: all mandatory scenarios pass and evidence files contain the tested commit SHA. These steps cannot be completed on a Linux-only host.

- [x] **Step 5: Commit platform evidence tooling**

```bash
git add tooling/device-gates docs/verification
git commit -m "test: add desktop platform device gates"
```

### Task 7: Final M4 acceptance audit

**Files:**
- Create: `docs/verification/m4-acceptance.md`
- Modify: `docs/superpowers/plans/2026-07-12-m4-product-hardening.md`

- [x] **Step 1: Run all automated gates**

```bash
(cd apps/desktop && flutter analyze && flutter test)
(cd services/cloud && test -z "$(gofmt -l .)" && go vet ./... && go test ./... -race)
sh tooling/security/run-security-gate.sh
git diff --check
```

- [x] **Step 2: Build the requirement-to-evidence table**

For every M4 bullet and every design section 18.5/18.6 item, record an exact command, evidence artifact, commit SHA and status of PASS, FAIL or NOT RUN. NOT RUN is never treated as PASS.

- [x] **Step 3: Verify repository hygiene**

Run: `git status --short && git ls-files | rg '(\.env|\.db$|private|secret|token|build/)'`

Expected: no generated bundle, runtime database, credential, private key or local environment file is tracked.

- [x] **Step 4: Check off only proven plan items**

Windows and macOS device-only boxes remain unchecked until their evidence files exist for the current commit. Automated Linux-hosted boxes may be checked from current command output.

- [x] **Step 5: Commit the acceptance record**

```bash
git add docs/verification docs/superpowers/plans/2026-07-12-m4-product-hardening.md
git commit -m "docs: record M4 acceptance evidence"
```

## Self-review

- Spec coverage: tasks cover diagnostics, crash recovery, malicious artifacts, performance budgets, Windows installation/signing/upgrade/uninstall/WebView2 prerequisite, macOS gate, Linux ordinary-window scope and the Windows E2E matrix.
- Deliberate device boundary: no Linux command is allowed to prove WebView2, Win32 DPI/IME/overlay, Windows signing, WKWebView entitlement or macOS notarization behavior.
- Placeholder scan: the plan contains no unresolved placeholder markers; every incomplete item has an exact task, file, command and expected evidence.
- Type consistency: diagnostics, recovery, performance and platform-gate types are introduced once and referenced by their declared names.
