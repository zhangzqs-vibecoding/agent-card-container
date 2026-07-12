#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

cd "$ROOT/apps/desktop"
flutter test \
  test/artifacts \
  test/runtime \
  test/capabilities \
  test/surfaces/surface_bridge_test.dart

cd "$ROOT/services/cloud"
UNFORMATTED=$(gofmt -l .)
test -z "$UNFORMATTED"
go vet ./...
CGO_ENABLED=1 go test ./... -race

cd "$ROOT/tooling/codecard-template"
pnpm run verify
pnpm audit --audit-level high
