# Cloud Structured Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Emit safe JSON logs for every HTTP request and key cloud generation stages, deploy them to the test server, and prove journald observability.

**Architecture:** Build one `log/slog` JSON logger at the composition root and inject it into HTTP and worker boundaries. HTTP middleware owns request correlation and response status capture; worker logging surrounds generation/model, sandbox-result, and publish stages without logging user/model content. stdout remains the sole sink and journald owns persistence.

**Tech Stack:** Go 1.26 standard `log/slog`, `net/http`, systemd/journald, table-driven/race tests.

---

### Task 1: Observability primitives

**Files:**
- Create: `services/cloud/internal/observability/logger.go`
- Create: `services/cloud/internal/observability/logger_test.go`

- [ ] Write failing tests for JSON construction, request metadata propagation, stable error kinds, UTC timestamps, and forbidden-field absence.
- [ ] Run targeted RED tests.
- [ ] Implement injected `slog.Logger`, request metadata context helpers, and allowlisted error classification.
- [ ] Run GREEN tests and commit `feat: add cloud observability primitives`.

### Task 2: HTTP access middleware

**Files:**
- Create: `services/cloud/internal/httpapi/accesslog.go`
- Create: `services/cloud/internal/httpapi/accesslog_test.go`
- Modify: `services/cloud/internal/httpapi/generation.go`
- Modify: `services/cloud/internal/httpapi/cards.go`
- Modify: `services/cloud/internal/httpapi/server.go`

- [ ] Write failing tests for 200/401/404, duration, request ID, user ID, remote IP, query/header/body redaction, writer failure, and `http.Flusher` preservation.
- [ ] Run targeted RED tests.
- [ ] Implement status writer and middleware; make handlers reuse correlated request ID and publish authenticated user ID into request metadata.
- [ ] Run HTTP API GREEN tests and commit `feat: log cloud HTTP requests`.

### Task 3: Worker and service lifecycle logs

**Files:**
- Modify: `services/cloud/internal/worker/worker.go`
- Modify: `services/cloud/internal/worker/worker_test.go`
- Modify: `services/cloud/internal/bootstrap/runtime.go`
- Modify: `services/cloud/cmd/agentcard/main.go`
- Create: `services/cloud/cmd/agentcard/main_test.go`

- [ ] Write failing worker tests for job/model/sandbox/publish success and failure event sequences with safe fields only.
- [ ] Run targeted RED tests.
- [ ] Inject logger into worker and emit stage events around existing boundaries; emit structured startup/shutdown/worker-loop events in main.
- [ ] Run GREEN tests and commit `feat: log cloud generation lifecycle`.

### Task 4: Full verification and deployment

**Files:**
- Modify: `README.md`
- Modify: `docs/verification/m4-acceptance.md`

- [ ] Document journald queries and stable event names.
- [ ] Run `gofmt`, `go vet ./...`, `go test ./... -race`, security gate, secret scan, and `git diff --check`.
- [ ] Build a static Linux binary and verify its hash.
- [ ] Upload atomically, restart systemd, and verify service health.
- [ ] Send health, valid-auth, invalid-auth, and generation requests; assert expected JSON events/statuses in journald and absence of credentials/content.
- [ ] Record redacted evidence, commit, push, observe CI, and independently recheck server status.
