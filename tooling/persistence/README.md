# P0-B persistence gate

`run-p0b-gate.sh` creates a disposable PostgreSQL 17 and MinIO environment on
an isolated Docker network. It runs the production repository tests, rebuilds
the Go runtime against persisted data, exercises `/healthz` and `/readyz` while
each dependency is unavailable, restarts both data services, and performs a
paired metadata/artifact backup and restore.

The gate uses digest-pinned images, loopback-only random host ports, named
volumes and process-generated test credentials. Credentials, signing keys,
database dumps, object mirrors and logs stay under a temporary directory and
are removed by the exit trap. Do not use the generated values or this
disposable deployment as production configuration.

Run the prerequisite check:

```sh
sh tooling/persistence/run-p0b-gate.sh --check
```

Run the complete gate from the repository root:

```sh
sh tooling/persistence/run-p0b-gate.sh
```

Required host commands are Docker, Go, curl, OpenSSL, PostgreSQL `psql`, GNU
core utilities and a working Docker daemon. A failed dependency probe, test,
backup checksum, restore, download, SHA-256 check, manifest check or Ed25519
verification causes a non-zero exit. Cleanup still removes the containers,
network, volumes and temporary backup data.

When refreshing an image, select an explicitly reviewed release, resolve its
repository digest with `docker image inspect`, update the constant in the
script, and rerun the entire gate. Never replace a digest with `latest`.
