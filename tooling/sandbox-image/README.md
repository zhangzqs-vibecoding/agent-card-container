# CodeCard sandbox image

The Dockerfile defaults to Node 22.17.0 pinned by registry digest. Build from
the repository root without overriding it:

    docker build \
      -f tooling/sandbox-image/Dockerfile \
      -t agent-card-builder:<version> .

`NODE_IMAGE` may only be changed to another reviewed full digest. The CI policy
and sandbox integration gate reject a mutable base image. `NPM_REGISTRY`
defaults to the official npm registry and may be overridden with an approved
internal registry; lockfile integrity remains mandatory.

Production configuration must reference the resulting image by its digest.
The worker never forwards model credentials, signing keys, database credentials,
or object-store credentials into this container.

The image installs dependencies with `pnpm --frozen-lockfile --ignore-scripts`.
The workflow scans both pull-request images and the independently rebuilt image
before publishing `ghcr.io/<owner>/agent-card-codecard-builder@sha256:<digest>`.
Runtime configuration must copy that digest, never the commit tag.

Refresh
`dependency-policy.json` only after the production license list and
the fail-closed bulk advisory audit both pass review.
