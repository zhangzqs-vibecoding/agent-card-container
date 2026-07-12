# M4 acceptance evidence

Overall status: **DEVICE EVIDENCE REQUIRED**  
Latest automated code commit: `55cbbbe974bbe18331e126f860bf23ec66f38a44`

Recorded: 2026-07-12 (Asia/Shanghai)

The desktop cloud-settings extension is governed by
`docs/superpowers/specs/2026-07-12-desktop-cloud-settings-design.md`. Its local
acceptance includes repository, secure-store, authenticated connection,
bootstrap, widget, and restart tests plus the complete Flutter and shared
security gates. Remote deployment and Windows portable-package evidence are
recorded only after those checks run against the dedicated test host; this
record never includes tokens, passwords, or signing private keys.

This record distinguishes reproducible host-independent evidence from physical
desktop-device evidence. `NOT RUN` is not a pass and the MVP is not
release-ready until every mandatory Windows scenario passes for the release
commit.

## Automated gates

| Requirement | Command | Result | Evidence |
|---|---|---|---|
| Flutter formatting/static correctness | `cd apps/desktop && flutter analyze` | PASS | No analyzer findings |
| Flutter unit/widget suite | `cd apps/desktop && flutter test` | PASS | 191 tests passed |
| Go formatting | `cd services/cloud && test -z "$(gofmt -l .)"` | PASS | Empty formatter diff |
| Go static analysis | `cd services/cloud && go vet ./...` | PASS | Exit 0 |
| Go race suite | `cd services/cloud && CGO_ENABLED=1 go test ./... -race` | PASS | All packages passed |
| Malicious artifact/runtime gate | `sh tooling/security/run-security-gate.sh` | PASS | TypeScript shared fixtures, Flutter security suite 66 tests, Go race suite, CodeCard dependency/typecheck/tests/build/bundle validation and `pnpm audit` passed |
| Performance evaluator | `cd apps/desktop && flutter test test/diagnostics/performance_sample_test.dart` | PASS | Budgets, percentile calculation and minimum sample populations tested |
| Insufficient performance evidence | `dart tooling/performance/summarize.dart tooling/performance/fixtures/insufficient.json` | PASS (rejected) | Exit 1 with four `:samples` failures and `passed: false` |
| Repository whitespace | `git diff --check` | PASS | No findings |
| Credential scan | `rg -l 'sk-[A-Za-z0-9]{20,}|BEGIN ... PRIVATE KEY|Authorization: Bearer ...' ...` | PASS | No files matched |
| GitHub workflow policy | `dart tooling/security/validate-workflows_test.dart` and `actionlint` | PASS | Four workflows use immutable action SHAs, bounded permissions/timeouts, PR secret isolation and verified Windows uploads |
| Windows portable-package logic | `pwsh packaging/windows/test-portable-package.ps1` in the official PowerShell container | PASS | Valid archive round trip passed; missing DLL/README, hidden `.env` and traversal entries were rejected |
| GitHub hosted platform execution | GitHub Actions after public repository push | PASS | Run `29193727198`: quality, Linux release and macOS release passed for `55cbbbe` |
| Desktop cloud-settings repository and secure-source logic | `cd apps/desktop && flutter test test/cloud/cloud_settings_repository_test.dart test/cloud/cloud_settings_service_test.dart` | PASS | Non-secret atomic JSON, strict URL/key validation, environment precedence, secure-token failures and rollback covered |
| Desktop cloud-settings UI and restart | `cd apps/desktop && flutter test test/cloud/cloud_settings_controller_test.dart test/app/agent_card_app_test.dart test/adapters/application_restarter_test.dart` | PASS | Settings form, authenticated test/save, no token echo, restart-required state and launch-before-shutdown covered |
| Dedicated cloud test host | systemd status plus Python HTTP assertions on Ubuntu 24.04 x86_64 | PASS | Static binary hash matched upload; health, authenticated catalog, invalid-token rejection and restart persistence passed on `192.168.242.105` |
| Flutter-to-remote cloud integration | SSH loopback tunnel plus opt-in `cloud_settings_live_test.dart` | PASS | Production `CloudApiClient.testConnection()` reached remote health and authenticated catalog; temporary local credential file was removed afterward |
| Windows cloud-settings portable package | GitHub run `29193727219` plus independent downloaded-artifact verification | PASS | Windows tests/release/native dependencies/archive upload passed; `0.1.0-dev.5` ZIP contains 31 files and independently verified SHA-256 `54eb662f23fea7fb576181ae4de6f61327b91bccd981c3d50046af0bf0b653bf` |

## M4 product-hardening requirements

| Requirement | Status | Exact evidence or remaining gate |
|---|---|---|
| Redacted diagnostics export | PASS | Allowlist/redaction tests plus atomic file export and the Settings > Diagnostics user flow in `desktop_bootstrap_test.dart` and `agent_card_app_test.dart` |
| Crash marker and unaffected-card recovery | PASS | `apps/desktop/test/recovery/startup_recovery_test.dart`, `apps/desktop/test/app/desktop_bootstrap_test.dart` |
| Persistent three-failure CodeCard quarantine | PASS | `apps/desktop/test/code_card/code_card_host_test.dart`, SQLite schema v4 tests |
| Bounded ZIP extraction and signed artifact enforcement | PASS | artifact tests plus security gate |
| Capability permission UX and persisted grants | PASS | capability broker and permission prompt widget tests |
| CodeCard runtime context and fixed event contract | PASS | context/theme/online/surface/permission/metrics/suspend/resume tests; contract version remains 1 |
| Active-card budgets | PASS (automated) | Tests enforce 20 NativeCard/8 CodeCard limits, offscreen exclusion, window lifecycle suspend/resume, and child-to-host lifecycle events |
| Main-window/tray lifecycle | PASS (automated) | Adapter tests cover close-to-hide, tray show/quit, and runtime ownership; Windows interaction remains in the device matrix |
| Cross-language contract fixtures | PASS | Go, Dart and TypeScript decode the same CardDefinition, NativeCard and local-RPC fixtures in the security gate |
| CodeCard dependency supply chain | PASS (automated) | Exact dependency/license allowlist, immutable lockfile, Vite/Vitest security upgrade and zero-vulnerability audit |
| Repeatable performance capture tooling | PASS (tooling) | `tooling/performance/capture-windows.ps1` and `integration_test/performance_baseline_test.dart`; physical results remain NOT RUN |
| Windows installer and release verifier structure | PASS (static only) | Stable AppId/version, signed executable check, approved DLL allowlist and official WebView2 registry detection are present |
| macOS runner and loopback entitlements | PASS (static only) | Generated runner, last-window lifecycle policy and client/server network entitlements are present; target-device build/sign/notarization remains NOT RUN |
| Signed Windows bundle/installer | NOT RUN | Run `packaging/windows/verify-release.ps1` and the release workflow on Windows with protected signing credentials |
| WebView2 missing-runtime user flow | NOT RUN | Exercise installer on a clean Windows device |
| macOS signing/notarization technical gate | NOT RUN | Run `sh tooling/device-gates/macos-m4.sh` on macOS |
| Linux ordinary-window native build | PASS | Ubuntu 24.04 x64 isolated gate passed analyze, 191 tests and `flutter build linux --debug`; validated evidence includes the bundle SHA-256 for commit `1d4c0a86eff3cc29f088c52bb182fb3b0a50d926` |

## Design section 18.5 — Windows end-to-end matrix

| # | Mandatory scenario | Status | Required evidence |
|---:|---|---|---|
| 1 | NativeCard generate/install/interact/offline restart/state | NOT RUN | Windows device-gate JSON for this commit |
| 2 | CodeCard with local JavaScript runs offline | NOT RUN | Windows WebView2 device evidence |
| 3 | Workspace/detached/overlay/redock without state loss | NOT RUN | Windows multi-window evidence |
| 4 | Main-window close while tray/overlay/runtime survive | NOT RUN | Windows lifecycle evidence |
| 5 | Multi-monitor, mixed DPI, cross-screen, sleep/lock/unplug | NOT RUN | Reference Windows hardware evidence |
| 6 | Chinese IME, focus, Tab, clipboard and external URL | NOT RUN | Interactive Windows evidence |
| 7 | Tampering/navigation/private network/RPC/ZIP attacks blocked | PASS (automated), NOT RUN (device) | Security gate passed; WebView2 device attack matrix remains |
| 8 | 1/3/10 windows and 1/5/20 cards resource baseline | NOT RUN | Windows performance/device JSON |

Run the fail-closed matrix with:

```powershell
pwsh tooling/device-gates/windows-m0.ps1
```

Validate its evidence against the exact commit with
`tooling/device-gates/validate-evidence.dart`. Missing or skipped mandatory
scenarios must remain a non-zero result.

## Design section 18.6 — performance budgets

| Budget | Status | Evidence |
|---|---|---|
| Cold startup P50 <= 3 s, P95 <= 5 s (20 launches) | NOT RUN | Windows capture required |
| Cached NativeCard mount P50 <= 300 ms (30 mounts) | NOT RUN | Windows capture required |
| Cached CodeCard mount P50 <= 1.5 s (20 mounts) | NOT RUN | Windows capture required |
| 10 Native + 3 Code steady CPU < 3%, RSS < 1 GiB for 60 s | NOT RUN | Windows capture required |
| Moving/resizing P95 frame time <= 16.7 ms (1000 frames) | NOT RUN | Windows capture required |
| Active maximum 20 Native / 8 Code; excess or invisible Code suspended | PASS (automated) | Runtime budget, viewport visibility, window lifecycle and suspend/resume event tests |

Capture and fail-closed evaluation command:

```powershell
pwsh tooling/performance/capture-windows.ps1 `
  -Bundle apps/desktop/build/windows/x64/runner/Release `
  -Output docs/verification/performance-baseline.json
```

## Repository hygiene

- `git status --short` was empty before this evidence record was updated.
- No generated release bundle, runtime database, `.env`, token, private key or
  card state is tracked.
- `services/cloud/.env.example` is intentionally tracked and contains no
  credential value.
- Security-gate-generated CodeCard `dist/` output is ignored and not tracked.

## Release decision

Automated M4 implementation gates pass, but release acceptance is **withheld**.
The Windows M0/M4 device matrix, reference-device performance capture, signed
release verification, and macOS technical gate are mandatory remaining
evidence. This document must not be changed to PASS until those artifacts refer
to the exact release commit and pass the fail-closed validators.
