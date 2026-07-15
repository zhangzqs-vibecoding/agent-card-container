# P0-A Real DeepSeek NativeCard Verification

Date: 2026-07-15
Implementation commits: `808b435`, `0a11ef3`, `41f9894`
Provider base domain: `api.deepseek.com`
Model: `deepseek-v4-pro`
Case sets: `p0a-headless-vertical-v1`, `native_eval_cases.v1.json`

## Current status

| Gate | Status | Evidence |
|---|---|---|
| Linux/headless fake-provider vertical path | PASS | `TestHeadlessVerticalFakeModelProducesVerifiedSignedNativeCard` |
| Artifact tamper and unsafe ZIP rejection | PASS | Bootstrap artifact verifier tests |
| Real DeepSeek three-case vertical path | PASS | 3/3 signed artifacts verified in one serial run |
| Real DeepSeek 20-case quality gate | PASS | 16/20 strict-validator successes; threshold 16 |
| Windows install, interaction, restart and offline device evidence | **NOT RUN** | Must be collected on a Windows device; headless Linux evidence is not a substitute |

The automated headless test exercises the real cloud HTTP handlers, confirmed
requirement snapshot, generation queue, one worker iteration, CodingAgent,
publisher, content-addressed object storage and the signed NativeCard artifact.
It verifies:

- both additional messages remain present and ordered in the frozen snapshot
  before and after worker execution;
- the ready generation, card list, card detail and artifact metadata agree on
  the immutable version;
- the archive SHA-256 agrees with the version and download metadata;
- ZIP paths are safe and unique, and the exact NativeCard file set is present;
- every manifest file size and SHA-256 agrees with the ZIP entry;
- the manifest Ed25519 signature verifies over canonical JSON without the
  `signature` field;
- `payload/native.json` passes the strict catalog-derived NativeCard validator;
- `reports/validation.json` reports a passed native attempt between one and
  three.

## Real-provider evidence

The paid gates ran from an interactive shell with history and tracing disabled.
The credential was read without terminal echo and existed only in that process.
Neither test emitted the credential, request prompts, model output, manifest,
NativeCard payload, archive bytes or Authorization header.

### Three-case signed vertical path

The complete serial suite passed in 28.276 seconds. All three cards succeeded on
their first model call:

| Case | Result | Duration | Attempts | Artifact SHA-256 |
|---|---|---:|---:|---|
| `timer` | PASS | 10,034 ms | 1 | `8b9e5abcf7cde7d4052d66d76668b3853abd97d23a3d73cf28e4dd2a927c77b0` |
| `dashboard` | PASS | 7,939 ms | 1 | `e2b0b7e6240bfcc29e1378bd2ea51464941efe5a44447815eb649298ac300dea` |
| `form-list` | PASS | 10,278 ms | 1 | `dd52e84fc058f6742ffbb943bfdb5d782c23729342b42a23849a66276864d52f` |

The test traversed the real HTTP generation handlers, immutable confirmed
requirements, queue and worker, CodingAgent, strict NativeCard validation,
publisher, download path and signed ZIP verifier. Passing therefore proves the
Linux/headless portion only; it does not prove desktop installation.

### Twenty-case NativeCard quality gate

The serial fixed evaluation passed exactly at its predefined threshold:

| Metric | Result |
|---|---:|
| Successful cases | 16 / 20 |
| Required threshold | 16 / 20 |
| Total duration | 101,459 ms |
| Input tokens | 137,956 |
| Output tokens | 8,347 |
| Attempt distribution | 1 call: 11; 2 calls: 4; 3 calls: 5 |
| Failure categories | provider error: 3; validation expression: 1 |

There were no selector rejections, contract assertion failures, timeouts,
cancellations or internal errors. The three provider failures exhausted the
same bounded three-call policy; the suite was not automatically retried.

## P0-A requirement audit

| Requirement | Status | Evidence |
|---|---|---|
| Frozen confirmed requirement reaches the Agent | PASS | Unit/repository tests plus all three vertical cases verify both additions before and after worker execution |
| Contract-derived prompt and strict validator | PASS | Generated catalog gate, cross-language contract gate and strict NativeCard validation in every signed artifact |
| At least 16/20 fixed real-model cases | PASS | 16/20 with bounded calls and the statistics above |
| Three representative signed artifacts | `HEADLESS PASS` | Timer, dashboard and form/list hashes above; download, ZIP, canonical signature and payload verification passed |
| Safe provider/validation errors | PASS | Retry classification and safe diagnostic tests; live output contains only stable categories and counts |
| Ephemeral credential with no repository leakage | PASS | Interactive-only injection; exact-key and credential-shape scans returned clean |
| Windows install, interaction and offline restart | `DEVICE NOT RUN` | Requires a Windows 11 x64 reference device and is not implied by this record |

## Repeatable automated command

From `services/cloud`:

```bash
env -u AGENTCARD_DEEPSEEK_LIVE \
  go test ./internal/bootstrap \
  -run 'TestHeadlessVertical|TestVerticalArtifact|TestVerticalZIP' \
  -count=1
```

The paid test is opt-in, serial and contains exactly three representative
NativeCard requirements: timer, offline dashboard, and form/list. Each case is
processed once by the vertical harness; CodingAgent retains its hard maximum of
three model attempts. A failed subtest stops the remaining paid cases.

## Ephemeral real-provider procedure

Use an interactive shell only. Do not put the credential in a command argument,
script, file, shell history, documentation, or test log. Keep shell tracing
disabled.

```zsh
unset HISTFILE
set +x
read -rs 'AGENTCARD_MODEL_API_KEY?DeepSeek API key: '
echo
export AGENTCARD_MODEL_API_KEY
export AGENTCARD_MODEL_BASE_URL='https://api.deepseek.com'
export AGENTCARD_MODEL='<supported-model>'
export AGENTCARD_DEEPSEEK_LIVE=1

go test ./internal/bootstrap \
  -run '^TestDeepSeekVerticalLiveProducesRepresentativeSignedNativeCards$' \
  -count=1 -timeout=8m -v

go test ./internal/modelprovider \
  -run '^TestDeepSeekLiveNativeCardQualityGate$' \
  -count=1 -timeout=35m -v

unset AGENTCARD_MODEL_API_KEY AGENTCARD_MODEL_BASE_URL \
  AGENTCARD_MODEL AGENTCARD_DEEPSEEK_LIVE
exit
```

Safe evidence from a real run may contain only the date, commit/worktree
state, provider base domain, model name, case-set version, success count,
attempt distribution, token totals, total duration and artifact hashes. It must
not contain the environment, Authorization header, API key, prompts, model
content, manifest, NativeCard payload or archive bytes.

The vertical test emits only case ID, successful status, duration, validated
attempt count and artifact SHA-256. Token totals remain sourced from the
separate Task 6 20-case quality gate; Task 7 does not widen production APIs only
to duplicate that accounting.

## Remaining evidence

The following remain explicitly **NOT RUN**:

- Windows signed package installation;
- NativeCard interaction, restart persistence and offline behavior on Windows;
- Windows artifact tamper rejection and desktop lifecycle behavior.

These items must not be promoted to PASS from Linux/headless or fake-provider
evidence.
