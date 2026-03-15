#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="VPNClient"
APP_DIR="${ROOT_DIR}/build/${APP_NAME}.app"
CONTENTS_DIR="${APP_DIR}/Contents"
MACOS_DIR="${CONTENTS_DIR}/MacOS"
RESOURCES_DIR="${CONTENTS_DIR}/Resources"
BACKEND_DIR="${RESOURCES_DIR}/bin"
CONFIG_SOURCE="${ROOT_DIR}/configs/PeshkiM.json"
CONFIG_DEST="${RESOURCES_DIR}/PeshkiM.json"

mkdir -p "${MACOS_DIR}" "${BACKEND_DIR}"

cat > "${CONTENTS_DIR}/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleExecutable</key>
  <string>VPNClient</string>
  <key>CFBundleIdentifier</key>
  <string>local.vpnclient.macos</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>VPNClient</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>1.0.0</string>
  <key>CFBundleVersion</key>
  <string>1</string>
  <key>LSMinimumSystemVersion</key>
  <string>13.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

env GOCACHE="${GOCACHE:-/tmp/gocache}" GOTMPDIR="${GOTMPDIR:-/tmp}" \
  go build -o "${BACKEND_DIR}/vpnclient-ui" ./cmd/vpnclient-ui

cp "${CONFIG_SOURCE}" "${CONFIG_DEST}"

clang++ \
  -std=c++20 \
  -fobjc-arc \
  -framework AppKit \
  -framework Foundation \
  -framework UniformTypeIdentifiers \
  -framework WebKit \
  -o "${MACOS_DIR}/VPNClient" \
  "${ROOT_DIR}/native/src/vpnclient_macos_app.mm"

chmod +x "${MACOS_DIR}/VPNClient" "${BACKEND_DIR}/vpnclient-ui"

echo "Built ${APP_DIR}"
