# GitHub CI and Portable Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 创建 public GitHub 仓库，建立 Linux/Windows/macOS CI，并产出可下载、解压和启动的 Windows x64 便携 ZIP。

**Architecture:** Ubuntu shared gate 负责跨语言合同、Go/Flutter/CodeCard 安全测试；三个平台 job 各自编译原生 runner。Windows job 从 pinned vcpkg 源码构建 SQLite/libsodium，用独立 PowerShell packager 组装 ZIP，再在解压后重新校验。Tag workflow 仅发布当前 tag 生成的 ZIP 与 checksum；DeepSeek 付费测试是手动、secret-gated 的独立 workflow。

**Tech Stack:** GitHub Actions、Flutter 3.32.8、Go 1.26.x、PowerShell 7、vcpkg、GitHub CLI、GitHub Releases。

---

### Task 1: Portable package verifier

**Files:**
- Create: `packaging/windows/verify-portable-package.ps1`
- Create: `packaging/windows/test-portable-package.ps1`

- [x] **Step 1: Write the failing verifier test**

`test-portable-package.ps1` 在 `$TestDrive` 等价的临时目录中创建最小 bundle，
包含 `agent_card_desktop.exe`、`flutter_windows.dll`、`sqlite3.dll`、
`libsodium.dll`、`data/flutter_assets/AssetManifest.bin`和 `README.txt`，然后压缩成
ZIP。测试必须证明：合法 ZIP 通过，缺 DLL、多 `.env`、路径穿越或
README 缺失的 ZIP 均非零退出。

- [x] **Step 2: Run the test and confirm RED**

Run:

```powershell
pwsh -File packaging/windows/test-portable-package.ps1
```

Expected: FAIL because `verify-portable-package.ps1` does not exist.

- [x] **Step 3: Implement the ZIP verifier**

`verify-portable-package.ps1` 接受 `-Archive`、`-ExpectedVersion`、
`-ExpectedCommit` 和可选 `-RequireSignature`。它必须：

1. 在新的临时目录解压并在 `finally` 删除。
2. 拒绝绝对路径、`..`、重复路径、symlink/reparse point 和超过 1 GiB
   的解压内容。
3. 校验四个必需文件、Flutter assets、`README.txt`、
   `release-manifest.json`，并拒绝 `.env`、database、log、private key 和
   secrets 目录。
4. 校验 manifest 中的 version、commit、每个文件路径与 SHA-256，拒绝未
   列入 manifest 的额外文件。
5. `-RequireSignature` 时要求 EXE Authenticode `Valid`。

- [x] **Step 4: Run GREEN and negative cases**

Run the Step 2 command. Expected: every positive/negative assertion passes and the script exits 0.

- [x] **Step 5: Commit**

```bash
git add packaging/windows/verify-portable-package.ps1 packaging/windows/test-portable-package.ps1
git commit -m "test: verify Windows portable archives"
```

### Task 2: Deterministic portable packager

**Files:**
- Create: `packaging/windows/build-portable-package.ps1`
- Modify: `packaging/windows/test-portable-package.ps1`
- Modify: `packaging/windows/README.md`

- [x] **Step 1: Add a failing packager integration test**

对最小模拟 bundle 执行 packager，断言文件名为
`AgentCardContainer-windows-x64-1.2.3-abcdef0.zip`，manifest 包含完整 commit、
version、`signed: false` 及所有 payload hash，然后用 Task 1 verifier 回验。

- [x] **Step 2: Run RED**

Expected: FAIL because `build-portable-package.ps1` does not exist.

- [x] **Step 3: Implement the packager**

Script parameters: `-Bundle`、`-OutputDirectory`、`-Version`、`-Commit`、
`-RequireSignature`。它先在临时 staging 目录复制 bundle，写入不含 secret 的
`README.txt` 和 canonical `release-manifest.json`，执行敏感文件扫描，再生成
ZIP 和同名 `.zip.sha256` 文件。最后必须调用 Task 1 verifier 回验 ZIP。

- [x] **Step 4: Run tests and PowerShell analyzer-compatible syntax checks**

```powershell
pwsh -NoProfile -File packaging/windows/test-portable-package.ps1
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add packaging/windows
git commit -m "build: create deterministic Windows portable bundles"
```

### Task 3: Shared and platform CI

**Files:**
- Create: `.github/workflows/ci.yml`
- Replace: `.github/workflows/windows-release-gate.yml`
- Create: `tooling/security/validate-workflows.dart`
- Create: `tooling/security/validate-workflows_test.dart`

- [x] **Step 1: Write a workflow policy test**

Create `tooling/security/validate-workflows.dart` and
`tooling/security/validate-workflows_test.dart` using only `dart:io`. The test loads fixture YAML text and
asserts rejection of mutable action tags, write permissions on pull requests, missing timeouts, secret
references in fork-triggered jobs, and Windows jobs that upload a bundle without invoking both package
verifiers.

- [x] **Step 2: Verify RED**

```bash
dart tooling/security/validate-workflows_test.dart
```

Expected: FAIL because the validator does not exist.

- [x] **Step 3: Implement shared CI and platform builds**

`ci.yml` triggers on pull requests, `develop` push and workflow dispatch with minimum read permissions and
concurrency cancellation. Jobs:

- `quality`: pinned checkout/setup actions; Go format/vet/race, Flutter analyze/test, shared contracts and
  `run-security-gate.sh`.
- `linux`: Flutter analyze/test and release Linux build.
- `macos`: Flutter analyze/test and release macOS build.

`windows-release-gate.yml` triggers on PR, `develop`, tags and dispatch. It installs pinned Flutter, runs
analyze/test/release build, builds exact vcpkg commits, copies the two DLLs, runs `verify-release.ps1`, runs
the portable packager, verifies the produced ZIP, uploads ZIP/checksum/manifest with 14-day retention, and
never passes a secret to PR/fork runs.

- [x] **Step 4: Run validator and YAML parser**

```bash
dart tooling/security/validate-workflows_test.dart
dart tooling/security/validate-workflows.dart .github/workflows/*.yml
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add .github tooling/security
git commit -m "ci: build and test all desktop platforms"
```

### Task 4: Tagged GitHub Release and DeepSeek live gate

**Files:**
- Create: `.github/workflows/release.yml`
- Create: `.github/workflows/deepseek-live.yml`
- Modify: `tooling/security/validate-workflows.dart`
- Modify: `README.md`

- [x] **Step 1: Add failing workflow policy cases**

Tests require release workflow to run only for `v*` tags or manual dispatch, grant `contents: write` only at
the release job, verify ZIP before upload, publish `SHA256SUMS.txt`, and mark unsigned releases as prerelease.
Tests require DeepSeek workflow to be `workflow_dispatch` only, use the `deepseek-live` environment, and map
the API key directly from `secrets.AGENTCARD_MODEL_API_KEY` without printing it.

- [x] **Step 2: Verify RED**

Run Task 3 validator test. Expected: FAIL for missing workflows.

- [x] **Step 3: Implement release workflow**

The release workflow calls the reusable Windows build or repeats the pinned build without downloading
untrusted artifacts from another commit. It creates `SHA256SUMS.txt`, verifies hashes, and uses a full-SHA
pinned GitHub release action or `gh release create`. Without signing secrets it sets `prerelease: true` and
labels notes `UNSIGNED`; it never claims the artifact is signed.

- [x] **Step 4: Implement DeepSeek workflow**

Inputs include an explicit boolean `confirm_paid_test`. The job has `if: inputs.confirm_paid_test`, environment
`deepseek-live`, no artifact upload, no shell tracing, a five-minute timeout, and invokes only:

```bash
AGENTCARD_DEEPSEEK_LIVE=1 go test ./internal/modelprovider \
  -run TestDeepSeekLiveGeneratesValidNativeCard -count=1 -v
```

Model base URL/name and API key come from protected variables/secrets.

- [x] **Step 5: Validate and commit**

```bash
dart tooling/security/validate-workflows_test.dart
dart tooling/security/validate-workflows.dart .github/workflows/*.yml
git add .github README.md tooling/security
git commit -m "ci: publish portable Windows releases"
```

### Task 5: Local final verification

**Files:**
- Modify: `docs/verification/m4-acceptance.md`

- [x] **Step 1: Run complete local gates**

```bash
git diff --check
sh tooling/security/run-security-gate.sh
dart tooling/security/validate-workflows_test.dart
dart tooling/security/validate-workflows.dart .github/workflows/*.yml
pwsh -NoProfile -File packaging/windows/test-portable-package.ps1
```

Expected: all exit 0. If PowerShell is unavailable locally, run it on GitHub Windows and keep local status as
NOT RUN until that evidence exists.

- [x] **Step 2: Scan the full tracked history tip**

```bash
git ls-files -z | xargs -0 rg -l 'sk-[A-Za-z0-9]{20,}|BEGIN [A-Z ]*PRIVATE KEY|Authorization: Bearer [^<]' || true
git status --short
```

Expected: no credential file match and a clean worktree after the evidence commit.

- [x] **Step 3: Update evidence and commit**

Record exact commands and distinguish local PASS from GitHub/target-device NOT RUN.

```bash
git add docs/verification/m4-acceptance.md
git commit -m "docs: record CI and portable release gates"
```

### Task 6: Create, push, and verify the public repository

**Files:** None

- [ ] **Step 1: Create the repository without generated history**

```bash
gh repo create zhangzqs-vibecoding/agent-card-container \
  --public --description "Agent-driven cross-platform desktop card container" \
  --source . --remote origin --push
```

- [ ] **Step 2: Set and verify the default branch**

```bash
gh repo edit zhangzqs-vibecoding/agent-card-container --default-branch develop
git ls-remote --heads origin develop
```

Expected: remote SHA equals local `develop` SHA.

- [ ] **Step 3: Watch required workflow runs**

```bash
gh run list --repo zhangzqs-vibecoding/agent-card-container --limit 20
WINDOWS_RUN_ID=$(gh run list --repo zhangzqs-vibecoding/agent-card-container \
  --workflow windows-release-gate.yml --json databaseId --jq '.[0].databaseId')
gh run watch "$WINDOWS_RUN_ID" \
  --repo zhangzqs-vibecoding/agent-card-container --exit-status
```

Expected: shared CI and Windows portable workflow pass. Diagnose and fix failures with TDD; commit and push each
independent correction.

- [ ] **Step 4: Download and independently verify Windows ZIP**

```bash
WINDOWS_RUN_ID=$(gh run list --repo zhangzqs-vibecoding/agent-card-container \
  --workflow windows-release-gate.yml --json databaseId --jq '.[0].databaseId')
gh run download "$WINDOWS_RUN_ID" \
  --repo zhangzqs-vibecoding/agent-card-container --dir /tmp/agent-card-download
ARCHIVE=$(find /tmp/agent-card-download -type f -name '*.zip' -print -quit)
pwsh -NoProfile -File packaging/windows/verify-portable-package.ps1 \
  -Archive "$ARCHIVE" \
  -ExpectedCommit "$(git rev-parse HEAD)" \
  -ExpectedVersion 0.1.0
```

Expected: downloaded artifact passes verification and contains `agent_card_desktop.exe` at archive root.

- [ ] **Step 5: Final repository audit**

Verify public visibility, Actions URLs, downloadable artifact name, local/remote SHA equality, no secrets, and
remaining unsigned/target-device limitations. Do not mark the overall MVP complete merely because CI compiles.
