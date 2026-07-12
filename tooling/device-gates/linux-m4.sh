#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
OUTPUT=${1:-"$ROOT/docs/verification/linux-m4-evidence.json"}

cd "$ROOT/apps/desktop"
flutter analyze
flutter test
flutter build linux --debug

COMMIT=$(git -C "$ROOT" rev-parse HEAD)
FLUTTER_VERSION=$(flutter --version --machine | tr -d '\n')
KERNEL=$(uname -srmo)

mkdir -p "$(dirname -- "$OUTPUT")"
cat >"$OUTPUT" <<EOF
{
  "schemaVersion": 1,
  "commit": "$COMMIT",
  "platform": "linux",
  "kernel": "$KERNEL",
  "flutter": $FLUTTER_VERSION,
  "scenarios": {
    "ordinaryWindowBuild": "PASS",
    "unitAndWidgetTests": "PASS",
    "waylandOverlay": "OUT_OF_SCOPE"
  }
}
EOF

echo "Linux M4 evidence written to $OUTPUT"
