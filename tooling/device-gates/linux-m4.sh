#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
OUTPUT=${1:-"$ROOT/docs/verification/linux-m4-evidence.json"}

cd "$ROOT/apps/desktop"
STARTED_AT=$(date +%s)
flutter analyze
flutter test
flutter build linux --debug

COMMIT=$(git -C "$ROOT" rev-parse HEAD)
FLUTTER_VERSION=$(flutter --version --machine | tr -d '\n')
KERNEL=$(uname -srmo)
CPU_CORES=$(getconf _NPROCESSORS_ONLN)
MEMORY_GIB=$(awk '/MemTotal/ { printf "%.2f", $2 / 1024 / 1024 }' /proc/meminfo)
BUNDLE="$ROOT/apps/desktop/build/linux/x64/debug/bundle/agent_card_desktop"
BUNDLE_SHA256=$(sha256sum "$BUNDLE" | awk '{print $1}')
DURATION_SECONDS=$(($(date +%s) - STARTED_AT))

mkdir -p "$(dirname -- "$OUTPUT")"
cat >"$OUTPUT" <<EOF
{
  "schemaVersion": 1,
  "commit": "$COMMIT",
  "platform": "linux",
  "host": {
    "os": "$KERNEL",
    "cpuLogicalCores": $CPU_CORES,
    "memoryGiB": $MEMORY_GIB,
    "gpu": "not exposed by isolated build gate",
    "displayScalePercent": 100
  },
  "runtime": {
    "flutter": $FLUTTER_VERSION
  },
  "durationSeconds": $DURATION_SECONDS,
  "artifacts": {
    "linuxBundleSha256": "$BUNDLE_SHA256"
  },
  "scenarios": {
    "ordinaryWindowBuild": "PASS",
    "unitAndWidgetTests": "PASS",
    "waylandOverlay": "OUT_OF_SCOPE"
  }
}
EOF

dart "$ROOT/tooling/device-gates/validate-evidence.dart" \
  --platform linux \
  --commit "$COMMIT" \
  --evidence "$OUTPUT"

echo "Linux M4 evidence written to $OUTPUT"
