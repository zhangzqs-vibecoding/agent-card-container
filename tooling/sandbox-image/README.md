# CodeCard sandbox image

Build with a Node 22 image pinned by digest:

    docker build +      --build-arg NODE_IMAGE=node:22-bookworm-slim@sha256:<verified-digest> +      -f tooling/sandbox-image/Dockerfile +      -t agent-card-builder:<version> .

Production configuration must reference the resulting image by its digest.
The worker never forwards model credentials, signing keys, database credentials,
or object-store credentials into this container.
