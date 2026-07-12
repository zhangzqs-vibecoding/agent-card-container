ARG NODE_IMAGE
FROM ${NODE_IMAGE}

ARG NPM_REGISTRY=https://registry.npmmirror.com
RUN npm install --global --registry=${NPM_REGISTRY} pnpm@10.33.0 && \
    pnpm config set registry ${NPM_REGISTRY}
WORKDIR /opt/codecard-template
COPY tooling/codecard-template/package.json tooling/codecard-template/pnpm-lock.yaml ./
RUN pnpm install --prod=false --frozen-lockfile
RUN mkdir -p node_modules/.vite-temp && chmod 1777 node_modules/.vite-temp
COPY tooling/codecard-template/dependency-policy.json ./

USER 65532:65532
WORKDIR /workspace
