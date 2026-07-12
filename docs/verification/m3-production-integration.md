# M3 production integration evidence

Status: **PASS (local production adapters)**  
Commit: `59e7e4eb014cfaaafbe17bfc6d268c3bf63a5d66`  
Recorded: 2026-07-12 (Asia/Shanghai)

This evidence uses disposable local containers and repository integration
tests. It proves the production persistence and sandbox adapters; it does not
claim a real third-party model call or Windows WebView2 acceptance.

## PostgreSQL 17 and MinIO

Disposable `postgres:17-alpine`, `minio/minio` and `minio/mc` containers ran on
an isolated Docker network with random loopback host ports. The bucket was
created solely for the test. Tests ran serially because repository integration
tests deliberately reset the PostgreSQL public schema.

| Package | Command selector | Result |
|---|---|---|
| generation | `go test ./internal/generation -run Postgres -count=1` | PASS |
| jobs | `go test ./internal/jobs -run Postgres -count=1` | PASS |
| publish | `go test ./internal/publish -run 'Postgres|S3' -count=1` | PASS |
| bootstrap | `go test ./internal/bootstrap -run ProductionRuntimes -count=1` | PASS |

The bootstrap test connects separate API and worker runtimes to the same
PostgreSQL job/session/version repositories and MinIO artifact store, uses a
deterministic local model server, signs the generated artifact, processes a
queued generation, and observes the ready card through the API runtime.
Containers and their network were removed by a shell trap after the run.

## Locked-down OCI CodeCard builder

The builder used the immutable base manifest
`node@sha256:16e22a550f3863206a3f701448c45f7912c6896a62de43add43bb9c86130c3e2`.
The resulting temporary builder image was
`sha256:f0f2b8550462e0e1d5441d1da95d0b758d8105821d7cf3a9bb1a1d83384ee31b`.

```sh
NODE_IMAGE="node@sha256:16e22a550f3863206a3f701448c45f7912c6896a62de43add43bb9c86130c3e2" \
  sh tooling/security/run-sandbox-integration.sh
```

Result: PASS. The real `DockerBuilder` and `ExecRunner` ran template typecheck,
tests, Vite build and bundle policy with `--network none`, a read-only root,
non-root UID/GID, 2 CPUs, 2 GiB memory, pids limit 256, all capabilities
dropped, no-new-privileges, and bounded noexec tmpfs mounts. The output
collector accepted the built `index.html` and dependency policy under its file,
path and expanded-size limits. The temporary image tag was removed on exit.

## Deliberate remaining boundaries

- No user or production credential was used or persisted.
- The model provider was exercised with the repository's deterministic local
  HTTP model fixture. A real DeepSeek call remains a separate external-service
  gate.
- Windows WebView2 origin/isolation and the complete desktop E2E matrix remain
  NOT RUN and are tracked in `m4-acceptance.md`.
