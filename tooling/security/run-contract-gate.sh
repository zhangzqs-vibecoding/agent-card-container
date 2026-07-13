#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
OUTPUT=$(mktemp -d)
trap 'rm -rf "$OUTPUT"' EXIT

(
  cd "$ROOT/services/cloud"
  go run ./cmd/native-context-gen -check
)

(
  cd "$ROOT/apps/desktop"
  flutter test test/native_card/native_card_catalog_compatibility_test.dart
)

"$ROOT/tooling/codecard-template/node_modules/.bin/tsc" \
  --strict \
  --target ES2022 \
  --module ES2022 \
  --moduleResolution bundler \
  --outDir "$OUTPUT" \
  "$ROOT/contracts/typescript/decoder.ts"

node "$ROOT/contracts/typescript/contract.test.mjs" \
  "$OUTPUT/decoder.js" \
  "$ROOT"
