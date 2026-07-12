ARG NODE_IMAGE
FROM ${NODE_IMAGE}

RUN corepack enable
WORKDIR /opt/codecard-template
COPY tooling/codecard-template/package.json ./
RUN pnpm install --prod=false

USER 65532:65532
WORKDIR /workspace
