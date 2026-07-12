#!/bin/sh
set -eu

if [ "$(uname -s)" != "Darwin" ]; then
  echo "macOS M4 gate must run on macOS" >&2
  exit 2
fi

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
EVIDENCE=${1:-}

if [ -z "$EVIDENCE" ] || [ ! -f "$EVIDENCE" ]; then
  echo "usage: macos-m4.sh <device-evidence.json>" >&2
  exit 64
fi

cd "$ROOT/apps/desktop"
flutter analyze
flutter test
flutter build macos --release
codesign --verify --deep --strict build/macos/Build/Products/Release/agent_card_desktop.app

dart "$ROOT/tooling/device-gates/validate-evidence.dart" \
  --platform macos \
  --commit "$(git -C "$ROOT" rev-parse HEAD)" \
  --evidence "$EVIDENCE"
