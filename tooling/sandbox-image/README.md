# CodeCard sandbox image

Build from the repository root with a Node 22 image pinned by digest:

    docker build \
      --build-arg NODE_IMAGE=node:22-bookworm-slim@sha256:<verified-digest> \
      -f tooling/sandbox-image/Dockerfile \
      -t agent-card-builder:<version> .

`NPM_REGISTRY` defaults to the configured public mirror and may be overridden
with an approved internal registry. The lockfile integrity hashes remain
mandatory regardless of registry.

Production configuration must reference the resulting image by its digest.
The worker never forwards model credentials, signing keys, database credentials,
or object-store credentials into this container.

The image installs dependencies with `pnpm --frozen-lockfile`. Refresh
`dependency-policy.json` only after the production license list and
`pnpm audit --prod --audit-level high` both pass review.
