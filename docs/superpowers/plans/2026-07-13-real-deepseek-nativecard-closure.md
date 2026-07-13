# Real DeepSeek NativeCard Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a confirmed multi-message requirement flow through the Go worker into a contract-constrained real DeepSeek request, produce a strictly valid signed NativeCard, and prove the Linux/headless vertical path with a repeatable 20-case quality gate.

**Architecture:** Freeze a write-once requirement snapshot when a generation is confirmed and make the worker consume only that snapshot. Define NativeCard semantics once in `contracts`, generate a compact Go model context from it, and make both deterministic validation and the Agent prompt obey that catalog. Keep the existing provider interface and three-attempt repair loop; real credentials remain opt-in process environment only.

**Tech Stack:** Go 1.26, PostgreSQL 17 migration, JSON Schema/catalog, `log/slog`, DeepSeek OpenAI-compatible Chat Completions, Flutter contract compatibility tests.

**Repository rule:** The user explicitly authorized small implementation commits. Keep them scoped and verified; do not push automatically.

---

### Task 1: Freeze the complete confirmed requirement

**Files:**
- Modify: `services/cloud/internal/generation/session.go`
- Modify: `services/cloud/internal/generation/session_test.go`
- Modify: `services/cloud/internal/generation/repository.go`
- Modify: `services/cloud/internal/generation/service.go`
- Modify: `services/cloud/internal/generation/service_test.go`
- Modify: `contracts/cloud/openapi.yaml`

- [x] **Step 1: Write failing domain tests**

Add tests that create a session with an initial prompt, add two messages, and confirm it with a capability allowlist. Assert that the frozen value is ordered, write-once, UTC-normalized, and deep-cloned:

```go
func TestSessionConfirmFreezesCompleteRequirement(t *testing.T) {
    session := newTestSession(t, "生成离线番茄钟")
    _, _ = session.AddMessage("增加暂停按钮", time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC))
    _, _ = session.AddMessage("使用中文显示", time.Date(2026, 7, 13, 1, 1, 0, 0, time.UTC))

    _, err := session.Confirm([]string{"storage", "window.manageSelf"}, time.Date(2026, 7, 13, 1, 2, 0, 0, time.UTC))
    if err != nil { t.Fatal(err) }
    got := session.ConfirmedRequirement
    if got == nil || got.InitialPrompt != "生成离线番茄钟" || len(got.AdditionalMessages) != 2 {
        t.Fatalf("snapshot = %#v", got)
    }
}
```

Also test that returned repository clones cannot mutate `AdditionalMessages` or `AllowedCapabilities` inside storage.

- [x] **Step 2: Run RED tests**

Run:

```bash
cd services/cloud
go test ./internal/generation -run 'TestSessionConfirm|TestMemoryRepositoryDeepClonesConfirmed' -count=1
```

Expected: compile failure because `RequirementSnapshot`, `ConfirmedRequirement`, and `Session.Confirm` do not exist.

- [x] **Step 3: Implement the write-once snapshot**

Add this domain value and a `Confirm` method that copies only messages after the initial prompt, sorts/deduplicates capabilities, sets `ConfirmedAt`, and performs the queued transition atomically in the in-memory candidate:

```go
type RequirementSnapshot struct {
    InitialPrompt       string    `json:"initialPrompt"`
    AdditionalMessages []Message `json:"additionalMessages"`
    Target              Target    `json:"target"`
    Locale              string    `json:"locale"`
    AllowedCapabilities []string  `json:"allowedCapabilities"`
    ConfirmedAt         time.Time `json:"confirmedAt"`
}

type Session struct {
    // existing fields remain for backward-compatible API reads
    ConfirmedRequirement *RequirementSnapshot `json:"confirmedRequirement,omitempty"`
}
```

`Session.Confirm` must reject non-`awaiting_confirmation` status and an existing snapshot. `cloneSession` must deep-copy the snapshot, message slice, and capability slice.

- [x] **Step 4: Make Service confirm through the domain method**

Replace the direct queued transition in `Service.Confirm` with:

```go
_, err := session.Confirm(
    []string{"storage", "window.manageSelf"},
    service.now(),
)
```

Build `Summary` from the complete frozen requirement rather than overwriting it with the last message. Keep message additions restricted to `awaiting_confirmation`.

- [x] **Step 5: Define the response contract**

Add explicit `GenerationMessage`, `RequirementSnapshot`, and `GenerationSession` component schemas to `contracts/cloud/openapi.yaml`, and reference `GenerationSession` from create/get/message/confirm responses. The new response field is optional so existing Dart decoding remains compatible.

- [x] **Step 6: Run GREEN and contract tests**

Run:

```bash
cd services/cloud
go test ./internal/generation -count=1
cd ../..
sh tooling/security/run-contract-gate.sh
git diff --check
```

Expected: all pass.

- [x] **Step 7: Record a diff checkpoint**

Run `git diff --stat` and confirm only Task 1 files changed. Do not commit without explicit authorization.

### Task 2: Persist and backfill confirmed requirements

**Files:**
- Create: `services/cloud/migrations/003_confirmed_requirement_column.sql`
- Create: `services/cloud/migrations/004_confirmed_requirement_backfill.sql`
- Create: `services/cloud/migrations/005_confirmed_requirement_constraint.sql`
- Create: `services/cloud/migrations/006_validate_confirmed_requirement_constraint.sql`
- Modify: `services/cloud/internal/generation/postgres_repository.go`
- Create: `services/cloud/internal/generation/postgres_repository_test.go`
- Modify: `services/cloud/internal/generation/postgres_repository_integration_test.go`
- Modify: `services/cloud/internal/bootstrap/runtime_test.go`

- [x] **Step 1: Write failing PostgreSQL integration assertions**

Extend the repository integration test to confirm a session with two messages, reopen it through a new repository instance, and assert every snapshot field. Mutate the loaded snapshot, reload, and assert the stored copy is unchanged.

- [x] **Step 2: Run RED integration test**

Run the existing disposable PostgreSQL gate described in `docs/verification/m3-production-integration.md`, selecting `PostgresRepository` tests.

Expected: failure because the column and repository mapping do not exist.

- [x] **Step 3: Add the compatible migration**

Use four ordered migrations so short `ACCESS EXCLUSIVE` DDL transactions do not span the full-table backfill:

```sql
ALTER TABLE generation_sessions
  ADD COLUMN confirmed_requirement_json jsonb;
```

The backfill migration populates queued/generating/validating/ready/failed rows with initial prompt, ordered additional user messages, target, locale, the legacy capability allowlist, and the queued event timestamp (falling back to `updated_at`). A cancelled row is backfilled only when it has a queued event because cancellation is also legal before confirmation. Add the status-aware check as `NOT VALID`, then validate it in the final migration: queued/generating/validating/ready/failed sessions must be non-NULL; draft/awaiting/cancelled sessions may be NULL.

- [x] **Step 4: Persist the snapshot in repository operations**

Marshal/unmarshal `ConfirmedRequirement` in insert, update, and load paths. Reject malformed stored JSON and never silently reconstruct from `Session.Prompt` in the worker.

- [x] **Step 5: Run GREEN integration and bootstrap tests**

Run:

```bash
cd services/cloud
go test ./internal/generation -run Postgres -count=1
go test ./internal/bootstrap -run ProductionRuntimes -count=1
```

Expected: pass against the disposable PostgreSQL/MinIO environment used by the existing integration gate.

- [x] **Step 6: Record a diff checkpoint**

Run `git diff --check` and inspect the migration for secrets and irreversible destructive statements.

### Task 3: Make the worker consume only the frozen snapshot

**Files:**
- Modify: `services/cloud/internal/worker/worker.go`
- Modify: `services/cloud/internal/worker/worker_test.go`
- Modify: `services/cloud/internal/agent/agent.go`
- Modify: `services/cloud/internal/agent/agent_test.go`

- [x] **Step 1: Write failing worker tests**

Use a capture provider and assert all additions reach the model. Add tests for missing snapshots and manifest capabilities:

```go
func TestWorkerUsesOnlyConfirmedRequirementSnapshot(t *testing.T) {
    // Create + add two messages + confirm, then run the worker.
    // The captured request must contain both additions in order.
    // The published manifest capabilities must equal the snapshot allowlist.
}

func TestWorkerRejectsMissingConfirmedRequirement(t *testing.T) {
    // Insert a queued legacy session without a snapshot.
    // RunOnce must fail with GENERATION_REQUIREMENTS_MISSING.
}
```

- [x] **Step 2: Run RED worker tests**

Run:

```bash
cd services/cloud
go test ./internal/worker -run 'TestWorkerUsesOnly|TestWorkerRejectsMissing|TestWorkerPublishesCapabilities' -count=1
```

Expected: failure because the worker still uses `session.Prompt` and hard-coded capabilities.

- [x] **Step 3: Change the Agent request boundary**

Use a snapshot rather than four independently supplied legacy fields:

```go
type Request struct {
    SessionID   string
    Requirement generation.RequirementSnapshot
}
```

The Agent owns deterministic formatting of `InitialPrompt`, ordered additions, target, locale, and allowlisted capabilities. The worker must fail closed when the snapshot is nil.

- [x] **Step 4: Publish snapshot-derived metadata**

Use `InitialPrompt` for the title, the aggregate confirmed summary for the description, and a copied/sorted `AllowedCapabilities` slice for `CardDefinition.Capabilities`. Remove the hard-coded manifest list.

- [x] **Step 5: Run GREEN worker and Agent tests**

Run:

```bash
cd services/cloud
go test ./internal/worker ./internal/agent -count=1
```

Expected: all pass and logs remain free of requirement/model content.

### Task 4: Establish one NativeCard semantic catalog and strict validation

**Files:**
- Create: `contracts/card/native-card-catalog.schema.json`
- Create: `contracts/card/native-card-catalog.v1.json`
- Modify: `contracts/card/native-card.schema.json`
- Create: `services/cloud/cmd/native-context-gen/main.go`
- Create: `services/cloud/internal/agent/native_contract_context.go`
- Create: `services/cloud/internal/agent/native_contract_context_generated.go`
- Create: `services/cloud/internal/agent/native_contract_context_test.go`
- Modify: `services/cloud/internal/agent/native_validator.go`
- Create: `services/cloud/internal/agent/native_validator_test.go`
- Modify: `tooling/security/run-contract-gate.sh`

- [x] **Step 1: Inventory actual Flutter semantics**

Read the NativeCard renderer, expression evaluator, and action executor. Encode only behavior already supported by Flutter: allowed/required props per component, legal events, action fields, expression operators/arity, capability method mapping, and documented unsupported forms. Do not add catalog behavior that the renderer cannot execute.

- [x] **Step 2: Write failing catalog and validator tests**

Tests must reject unknown props, missing required props, invalid events, incomplete action fields, invalid timer configuration, unknown expression operators/arity, and capability methods outside the snapshot allowlist. Existing `pomodoro-native.json` and `native-card.json` fixtures must remain valid.

- [x] **Step 3: Run RED tests**

Run:

```bash
cd services/cloud
go test ./internal/agent -run 'TestNativeContractContext|TestNativeValidator' -count=1
```

Expected: the current permissive validator accepts at least one invalid case.

- [x] **Step 4: Add the catalog and generator**

The generator reads the root catalog/schema and writes a compact Go constant. It supports `-check`, which exits non-zero when generated output differs. The generated file begins with `// Code generated by native-context-gen; DO NOT EDIT.`

- [x] **Step 5: Make validation consume the catalog**

Replace duplicated `nativeComponents`/`nativeActions` sets with parsed generated catalog data. Validate recursively with existing node/depth/size limits plus catalog prop/event/action/expression rules. Pass the confirmed capability allowlist into validation so `capability.invoke` cannot exceed the manifest.

- [x] **Step 6: Strengthen the JSON schema and cross-language fixture gate**

Make `native-card.schema.json` reference the fixed catalog semantics where JSON Schema can express them. Keep runtime validation for cross-field and expression rules that JSON Schema cannot express cleanly.

- [x] **Step 7: Run GREEN generation and contract gates**

Run:

```bash
cd services/cloud
go run ./cmd/native-context-gen -check
go test ./internal/agent -run 'TestNativeContractContext|TestNativeValidator' -count=1
cd ../..
sh tooling/security/run-contract-gate.sh
```

Expected: all pass; changing the root catalog without regeneration fails the gate.

### Task 5: Move AgentCard prompting into the Agent and harden JSON transport

**Files:**
- Create: `services/cloud/internal/agent/native_prompt.go`
- Create: `services/cloud/internal/agent/native_prompt_test.go`
- Modify: `services/cloud/internal/agent/agent.go`
- Modify: `services/cloud/internal/modelprovider/provider.go`
- Modify: `services/cloud/internal/modelprovider/http_provider.go`
- Modify: `services/cloud/internal/modelprovider/http_provider_test.go`

- [x] **Step 1: Write failing prompt boundary tests**

Assert the Agent-created system prompt includes the generated catalog context; the user prompt includes initial/additional requirements in order, locale, target and allowed capabilities; repair attempts append only stable validation feedback. Assert provider code contains no AgentCard-specific wording.

- [x] **Step 2: Define transport-only provider input**

Refactor the provider request to transport fields:

```go
type Request struct {
    SystemPrompt string
    UserPrompt   string
    JSONOutput   bool
    MaxTokens    int
}
```

Session ID, runtime, locale, attempt and validation feedback remain Agent concerns and are rendered into `UserPrompt` before the provider call.

- [x] **Step 3: Build the NativeCard prompt in `agent`**

The system prompt must require one JSON object, include catalog-derived semantics, forbid undeclared capabilities and Markdown fences, and state that validation feedback supersedes the prior invalid answer. The user prompt uses stable labeled sections without exposing secrets.

- [x] **Step 4: Harden the HTTP request**

For `JSONOutput`, send the provider-supported JSON-object response format. Set an explicit max token bound, normalize Base URL without duplicating `/v1`, reject non-stop/truncated responses using `finish_reason`, preserve the 2 MiB response cap, and never include upstream bodies or credentials in errors.

- [x] **Step 5: Run RED/GREEN provider and Agent tests**

Run:

```bash
cd services/cloud
go test ./internal/agent -run 'TestNativePrompt|TestCodingAgent' -count=1
go test ./internal/modelprovider -run TestHTTPProvider -count=1
```

Expected: pass, including base URLs both with and without `/v1` and a truncated-response rejection case.

### Task 6: Preserve usage data and add the 20-case quality gate

**Files:**
- Modify: `services/cloud/internal/agent/agent.go`
- Modify: `services/cloud/internal/agent/agent_test.go`
- Create: `services/cloud/internal/agent/testdata/native_eval_cases.v1.json`
- Create: `services/cloud/internal/agent/eval_cases_test.go`
- Modify: `services/cloud/internal/modelprovider/deepseek_live_test.go`

- [ ] **Step 1: Write failing usage aggregation test**

Return two invalid/valid responses with usage values and assert the final `agent.Result` contains total input/output tokens and the actual attempt count. Logs must not contain prompt or response content.

- [ ] **Step 2: Implement usage aggregation**

Add `InputTokens` and `OutputTokens` to `agent.Result`; accumulate every provider response, including invalid outputs that enter repair. Publish only counts and duration to the live evaluation result, not normal user content logs.

- [ ] **Step 3: Add exactly 20 fixed, non-sensitive cases**

The JSON fixture contains `id`, `category`, `prompt`, `requiredComponents`, and `forbiddenCapabilities`. Cover timers (3), todo/list (4), dashboards (3), forms (3), charts/statistics (3), offline state (2), and explicit no-network/file/clipboard constraints (2). Forbidden Shell/filesystem attack prompts remain deterministic selector tests and do not enter the 16/20 denominator.

- [ ] **Step 4: Add fixture validation tests**

Assert IDs are unique, category counts match, prompts are non-empty, forbidden capabilities are absent from expected outputs, and every case is forced NativeCard-compatible.

- [ ] **Step 5: Implement the opt-in paid evaluation**

When `AGENTCARD_DEEPSEEK_LIVE != 1`, the test skips. When enabled, run serially with one suite-level timeout, at most three model calls per case, no automatic suite retry, and require at least 16 successes. Output only case ID, success/error category, attempts, duration, token counts, aggregate success and attempt distribution.

- [ ] **Step 6: Run ordinary tests with live gate disabled**

Run:

```bash
cd services/cloud
go test ./internal/agent ./internal/modelprovider -count=1
```

Expected: unit tests pass and live evaluation reports SKIP.

### Task 7: Prove the Linux/headless real-model vertical path

**Files:**
- Create: `services/cloud/internal/bootstrap/deepseek_vertical_live_test.go`
- Modify: `services/cloud/internal/bootstrap/runtime_test.go`
- Create: `docs/verification/p0a-real-deepseek-nativecard.md`

- [ ] **Step 1: Write an opt-in vertical integration test**

Compose the real runtime with an in-memory repository, deterministic test-only Ed25519 seed, test bearer token, real model provider, and `httptest.Server`. Through HTTP: create a generation, add two messages, confirm it, run one worker iteration, fetch the ready session/card/artifact, and verify ZIP hash, manifest signature, NativeCard strict validation, and the presence of both additions in the frozen snapshot.

- [ ] **Step 2: Keep credentials out of test artifacts**

Read only `AGENTCARD_MODEL_API_KEY`, `AGENTCARD_MODEL_BASE_URL`, and `AGENTCARD_MODEL` from the process environment. Skip without the live flag. Never print the environment, HTTP Authorization, system/user prompts, model content, or artifact payload.

- [ ] **Step 3: Run targeted real smoke with ephemeral injection**

Start an interactive shell, disable terminal echo while reading the key, export it only inside that process, run the single vertical test, then unset the key and exit the shell. Do not place the key in an `exec_command` argument, script, file, shell history, documentation, or log.

Expected: ready session and verified signed NativeCard artifact. If the provider rejects the configured model, query official provider metadata/documentation without exposing the key and rerun with the supported model supplied through `AGENTCARD_MODEL`.

- [ ] **Step 4: Run the 20-case live quality gate**

Use the same ephemeral shell and limits. Expected: at least 16/20 strict-validator successes, with only safe aggregate statistics emitted.

- [ ] **Step 5: Record redacted evidence**

Document date, commit/worktree state, provider base domain, model name, case-set version, success count, attempt distribution, token totals, duration, artifact hash, and commands with credential placeholders only. Explicitly state that Windows device evidence remains `NOT RUN`.

### Task 8: Full verification and P0-A audit

**Files:**
- Modify: `docs/verification/m4-acceptance.md`
- Modify: `docs/superpowers/plans/2026-07-13-real-deepseek-nativecard-closure.md`

- [ ] **Step 1: Run all affected quality gates**

Run:

```bash
test -z "$(gofmt -l services/cloud)"
cd services/cloud
go vet ./...
CGO_ENABLED=1 go test ./... -race
cd ../..
sh tooling/security/run-contract-gate.sh
sh tooling/security/run-security-gate.sh
git diff --check
```

Expected: all pass. A transient external package audit failure must be distinguished from a code failure and retried only within the security gate's bounded policy.

- [ ] **Step 2: Scan for credential leakage**

Scan tracked and untracked repository files for the exact supplied credential and common private-key/token patterns without printing matching secret values. The scan must return no match outside deliberate redaction fixtures.

- [ ] **Step 3: Audit every P0-A requirement**

Map the roadmap P0-A requirements to current code/tests/evidence: complete frozen context, catalog-derived prompt, strict validator, 20-case threshold, three signed/installable representative artifacts where Linux/headless can prove them, safe errors, and ephemeral credentials. Mark Windows install/offline evidence `NOT RUN`, not PASS.

- [ ] **Step 4: Update plan checkboxes and acceptance evidence**

Mark only executed items complete. Do not claim the full P0-A Windows criterion until a real Windows device verifies it.

- [ ] **Step 5: Leave a clean handoff**

Report modified files, tests, live-call cost/usage totals, remaining Windows-only evidence, and whether a commit/push is awaiting user authorization. Do not automatically commit or push.
