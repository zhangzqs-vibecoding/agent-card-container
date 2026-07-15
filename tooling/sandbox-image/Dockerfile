ARG NODE_IMAGE=node:22.17.0-bookworm-slim@sha256:b04ce4ae4e95b522112c2e5c52f781471a5cbc3b594527bcddedee9bc48c03a0
FROM ${NODE_IMAGE}

ARG BUILD_REVISION=unknown
ARG BUILD_SOURCE=unknown
LABEL org.opencontainers.image.title="Agent Card CodeCard Builder" \
      org.opencontainers.image.description="Offline, non-root CodeCard verification and build environment" \
      org.opencontainers.image.revision="${BUILD_REVISION}" \
      org.opencontainers.image.source="${BUILD_SOURCE}"

ARG NPM_REGISTRY=https://registry.npmjs.org
RUN npm install --global --registry=${NPM_REGISTRY} pnpm@10.33.0 && \
    pnpm config set registry ${NPM_REGISTRY}
WORKDIR /opt/codecard-template
COPY tooling/codecard-template/package.json tooling/codecard-template/pnpm-lock.yaml ./
RUN pnpm install --prod=false --frozen-lockfile --ignore-scripts
RUN mkdir -p node_modules/.vite-temp && chmod 1777 node_modules/.vite-temp
COPY tooling/codecard-template/dependency-policy.json ./

USER 65532:65532
WORKDIR /workspace
