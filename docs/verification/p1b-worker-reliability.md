# P1-B Worker Reliability Verification

Date: 2026-07-15 (Asia/Shanghai)
Implementation commits: `7c28f81`, `0808844`, `5d7de51`, `746b560`, `6222dac`, `2579cd3`, `039c4f2`
Verified code commit: `039c4f2a8755de765831bd728fa9a27a81810af3`
Status: **HEADLESS PASS / WINDOWS DEVICE NOT RUN**

## Result

The Go modular monolith keeps PostgreSQL as the only job coordinator. No
message broker, workflow platform or Agent framework was introduced.

| Reliability invariant | Result | Evidence |
|---|---|---|
| Atomic confirm/enqueue | PASS | A PostgreSQL trigger-injected job insert failure rolls back status, frozen requirement and queued event; success exposes queued session and exactly one job together |
| Lease ownership | PASS | `ExtendLease`, complete, fail, retry and publication reservation require running status, current owner and an unexpired lease |
| Heartbeat | PASS | Production defaults are a 90-second lease and 30-second heartbeat; deterministic and race tests prove repeated renewal, clean stop and downstream cancellation after ownership loss |
| Concurrent claim | PASS | Memory and PostgreSQL tests permit only one worker claim; expired lease recovery uses `FOR UPDATE SKIP LOCKED`; stale owner completion is rejected |
| Stable publication identity | PASS | `card_id` and `version_id` are reserved once per job and survive attempts; a different candidate cannot replace them |
| Stable artifact identity | PASS | artifact `createdAt` comes from persisted job creation time; an upload followed by temporary version insert failure retries the same content-addressed object key |
| Bounded infrastructure retry | PASS | Temporary publish/network/PostgreSQL connection/serialization/deadlock errors schedule 1s then 2s `available_at`; permanent validation/version conflicts fail immediately |
| Model call budget | PASS | Provider 408/429/5xx retry remains inside the Agent's existing maximum three calls; an exhausted provider error is not retried as another job attempt |
| Published-version recovery | PASS | When a version exists but session is validating, a reclaimed job skips Agent and only completes session/job; real PostgreSQL/MinIO test leaves exactly one version |
| Ready-session recovery | PASS | If `MarkReady` succeeded but job complete failed, the reclaimed job skips Agent and only completes the existing job |
| Cancellation propagation | PASS | Blocked model provider, CodeCard builder and object upload receive context cancellation after user cancellation; session/job stay cancelled and no version is created |
| Race and cleanup | PASS | Full Go race suite and extended disposable Docker persistence gate exit zero; no matching container, volume or temporary backup remains |

## Partial-failure matrix

| Interruption point | Recovery proof |
|---|---|
| `generating` / model blocked | User cancellation invalidates the lease, heartbeat cancels provider context, and late failure cannot overwrite cancelled |
| CodeCard builder blocked | Builder observes context cancellation; no artifact/version is published |
| Object upload blocked | Object store observes context cancellation; no version is published |
| Object uploaded, version insert failed | Job is queued with bounded delay; retry reuses stable IDs, timestamp and object key, then creates one version |
| Version inserted, session not ready | Reclaimed worker finds the immutable version, performs no model call, marks ready and completes |
| Session ready, job not completed | Reclaimed worker performs no model call or republish and only completes the job |

The opt-in `TestProductionWorkerRecoversPublishedVersionAfterLeaseExpiry`
executes the version-inserted recovery against real PostgreSQL 17 and MinIO.
It resets an isolated schema, publishes a signed artifact, lets the original
lease expire, reclaims with another worker, and asserts ready session,
completed job, zero provider calls and exactly one database version.

## Verification commands

All commands passed on Linux amd64:

```sh
cd services/cloud
test -z "$(gofmt -l .)"
go vet ./...
env -u AGENTCARD_DEEPSEEK_LIVE CGO_ENABLED=1 go test ./... -race -count=1

cd ../..
env -u AGENTCARD_DEEPSEEK_LIVE sh tooling/security/run-contract-gate.sh
env -u AGENTCARD_DEEPSEEK_LIVE sh tooling/security/run-security-gate.sh
sh tooling/persistence/run-p0b-gate.sh
```

The final Docker gate passed repository tests, the real worker recovery test,
normal runtime generation, dependency failure matrix, PostgreSQL/MinIO restart,
paired backup/restore, remote-style HTTP artifact download and independent
SHA-256/manifest/Ed25519 verification. Its last disposable P0-B artifact was
`a32a26c041b01fdb08fa2c10a3b5eaacb77add39607c476f81c4df65d0f2fa9b`;
the temporary backup and all generated credentials were destroyed on exit.

## Remaining boundary

This is Linux/headless evidence. Windows generation consumption, WebView2,
multi-window behavior, installer and physical-device interruption remain
`DEVICE NOT RUN`. A fixed shared server repeat remains part of P0-B remote
deployment evidence, but it does not invalidate the local worker reliability
invariants proven here.
