# P0-B Persistent Test Environment Verification

Date: 2026-07-15 (Asia/Shanghai)
Implementation commits: `4a96d48`, `0c196c0`, `bbbc0f9`, `8f70243`, `f956109`, `5ce5155`, `b77b677`
Verified code commit: `b77b677acc6f091c2c65a1bd1a15607d07c191fa`
Status: **LOCAL HEADLESS PASS / REMOTE NOT RUN**

## Environment

| Component | Version or immutable identity |
|---|---|
| Host | Linux amd64 |
| Go | `go1.26.3 linux/amd64` |
| Docker Engine | `29.1.3` |
| PostgreSQL | `postgres@sha256:742f40ea20b9ff2ff31db5458d127452988a2164df9e17441e191f3b72252193` |
| MinIO | `minio/minio@sha256:a1ea29fa28355559ef137d71fc570e508a214ec84ff8083e39bc5428980b015e` |
| MinIO client | `minio/mc@sha256:a7fe349ef4bd8521fb8497f55c6042871b2ae640607cf99d9bede5e9bdf11727` |

`tooling/persistence/run-p0b-gate.sh` created a unique Docker network, named
PostgreSQL and MinIO volumes, and loopback-only random host ports. Database,
object-store and Ed25519 test credentials were generated in process, passed by
environment, never printed, and removed with containers, volumes and temporary
backup data by the exit trap. No real model call or model credential was used.

## Gate result

The complete command exited zero:

```sh
sh tooling/persistence/run-p0b-gate.sh
```

| Requirement | Result | Evidence |
|---|---|---|
| PostgreSQL repositories | PASS | Generation, immutable confirmation snapshot, job and version integration tests ran against PostgreSQL 17 |
| S3 object store | PASS | Real MinIO upload, idempotent put, signed URL and network download passed |
| Go service restart | PASS | Separate API/worker runtimes were closed; a newly constructed runtime queried the same ready session, completed job, card and version |
| Network artifact verification | PASS | Signed HTTP URL was not `memory://`; bounded independent download, archive SHA-256, safe ZIP, manifest file hashes, canonical JSON and Ed25519 verification passed |
| Dependency readiness | PASS | PostgreSQL paused: `/healthz=200`, `/readyz=503`; MinIO paused: `/healthz=200`, `/readyz=503`; each returned to `200` after recovery |
| Data-service restart | PASS | Both PostgreSQL and MinIO containers restarted on persistent volumes; the original session and artifact remained queryable and verifiable |
| Paired backup and restore | PASS | PostgreSQL custom dump and complete MinIO object mirror shared one backup ID; after deleting schema and bucket content, both were restored and the original artifact passed query/download/signature verification again |
| Cleanup | PASS | No matching container, network, volume or temporary backup directory remained after exit |

Safe verbose evidence from the final run:

| Field | Value |
|---|---|
| Session | `gen_4c2d9cb6945e02f20960b30e5913f6df` |
| Job | `job_b70e70f35f9f3e6ea7dd04b04992c29c` (`completed`) |
| Card | `card_a15060f9d1b2921f394dd6f450790680` |
| Version | `ver_8520a2110905d0b077515214d34d7d7b` |
| Artifact SHA-256 | `87d3171b98a4fbfee44e16242e629cd4a0b68ef84d277c22050ca67bbf5e6146` |
| Signing key ID | `test-key` |
| Backup ID | `backup-20260715T131003Z` |
| Backup manifest SHA-256 | `1b9882904f30813937aa389f1a576254c070f2a0a59d41809548a1bb03db2e33` |

The backup itself was deliberately destroyed after verification. The digest is
evidence of the tested backup generation, not a claim that a retained recovery
point exists.

## Security and failure boundaries

- `AGENTCARD_PERSISTENCE_REQUIRED=true` rejects missing PostgreSQL/S3
  configuration and invalid boolean values instead of silently selecting
  process-local repositories.
- `/healthz` remains a liveness signal. `/readyz` checks both dependencies in
  parallel with a two-second bound and returns only stable public status.
- Readiness responses and access logs do not expose DSNs, endpoints, provider
  errors or credentials.
- The independent artifact verifier rejects cross-origin redirects, oversized
  responses, unsafe or duplicate ZIP paths, inconsistent file metadata,
  incorrect hashes and invalid signatures.

## Remaining external evidence

This local gate proves the Linux/headless implementation and repeatability. It
does not prove a fixed shared test server, HTTPS/public routing, or reachability
from the Windows reference device. Those items remain **REMOTE NOT RUN** and
must be collected before P0-C uses the environment. Windows installation,
interaction, offline restart, WebView2 and OS Surface behavior remain
**DEVICE NOT RUN**.
