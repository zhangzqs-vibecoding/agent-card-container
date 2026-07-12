#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
: "${NODE_IMAGE:?NODE_IMAGE must be an immutable sha256 image ID or digest}"

case "$NODE_IMAGE" in
  sha256:*|*@sha256:*) ;;
  *) echo "NODE_IMAGE must be pinned by digest" >&2; exit 64 ;;
esac

tag="agent-card-sandbox-integration:local"
cleanup() {
  docker image rm "$tag" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker build \
  --build-arg "NODE_IMAGE=$NODE_IMAGE" \
  --tag "$tag" \
  --file "$ROOT/tooling/sandbox-image/Dockerfile" \
  "$ROOT"
image_id=$(docker image inspect "$tag" --format '{{.Id}}')

cd "$ROOT/services/cloud"
AGENTCARD_SANDBOX_TEST_IMAGE="$image_id" \
  go test ./internal/sandbox -run DockerBuilderRunsLockedDownContainer -count=1

echo "Sandbox integration passed with image $image_id"
