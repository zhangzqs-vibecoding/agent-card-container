ARG NODE_IMAGE=node:22.23.1-alpine3.23@sha256:8516dce0483394d5708d4b2ee6cacb79fb1d617ea4e2787c2120bcca92ce372e
FROM ${NODE_IMAGE}

ARG BUILD_REVISION=unknown
ARG BUILD_SOURCE=unknown
LABEL org.opencontainers.image.title="Agent Card CodeCard Builder" \
      org.opencontainers.image.description="Offline, non-root CodeCard verification and build environment" \
      org.opencontainers.image.revision="${BUILD_REVISION}" \
      org.opencontainers.image.source="${BUILD_SOURCE}"

ARG NPM_REGISTRY=https://registry.npmjs.org
RUN npm install --global --registry=${NPM_REGISTRY} pnpm@11.13.0 && \
    pnpm config set registry ${NPM_REGISTRY} && \
    rm -rf /usr/local/lib/node_modules/npm /usr/local/bin/npm /usr/local/bin/npx
WORKDIR /opt/codecard-template
COPY tooling/codecard-template/package.json tooling/codecard-template/pnpm-lock.yaml ./
RUN pnpm install --prod=false --frozen-lockfile --ignore-scripts
RUN mkdir -p node_modules/.vite-temp && chmod 1777 node_modules/.vite-temp
COPY tooling/codecard-template/dependency-policy.json ./

USER 65532:65532
WORKDIR /workspace
