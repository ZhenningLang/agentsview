#!/usr/bin/env bash
# Ad-hoc reseal the bundled macOS .app when the bundler left it
# unsealed (e.g. updater signing failed for lack of
# TAURI_SIGNING_PRIVATE_KEY). macOS 26.4+ ecosystemd flags
# unsealed bundles at launch with the misleading "Intel app
# support is ending" notification even when every binary is
# arm64-native. Idempotent: a properly sealed bundle is left
# untouched.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP="$SCRIPT_DIR/../src-tauri/target/release/bundle/macos/AgentsView.app"

if [ ! -d "$APP" ]; then
  echo "reseal: no bundle at $APP (skipped)"
  exit 0
fi

if codesign -dv "$APP" 2>&1 | grep -q "Sealed Resources version=2"; then
  echo "reseal: $APP already sealed (skipped)"
  exit 0
fi

codesign --force --deep --sign - "$APP"
echo "reseal: ad-hoc sealed $APP"
