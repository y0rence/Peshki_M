#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="VPNClient"
APP_VERSION="${APP_VERSION:-1.0.0}"
APP_BUILD="${APP_BUILD:-1}"
APP_DIR="${ROOT_DIR}/build/${APP_NAME}.app"
CONTENTS_DIR="${APP_DIR}/Contents"
MACOS_DIR="${CONTENTS_DIR}/MacOS"
RESOURCES_DIR="${CONTENTS_DIR}/Resources"
BACKEND_DIR="${RESOURCES_DIR}/bin"
XRAY_ASSETS_DIR="${RESOURCES_DIR}/xray-assets"
CONFIG_SOURCE="${ROOT_DIR}/configs/PeshkiM.json"
CONFIG_DEST="${RESOURCES_DIR}/PeshkiM.json"

find_xray_binary() {
  if [[ -n "${XRAY_BINARY:-}" && -x "${XRAY_BINARY}" ]]; then
    printf '%s\n' "${XRAY_BINARY}"
    return 0
  fi

  if command -v xray >/dev/null 2>&1; then
    local wrapper_path wrapper_real cellar_dir candidate
    wrapper_path="$(command -v xray)"
    wrapper_real="$(realpath "${wrapper_path}")"
    cellar_dir="$(cd "$(dirname "${wrapper_real}")/.." && pwd)"
    candidate="${cellar_dir}/libexec/xray"
    if [[ -x "${candidate}" ]]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  fi

  return 1
}

find_xray_assets_dir() {
  if [[ -n "${XRAY_ASSETS_DIR_SOURCE:-}" && -d "${XRAY_ASSETS_DIR_SOURCE}" ]]; then
    printf '%s\n' "${XRAY_ASSETS_DIR_SOURCE}"
    return 0
  fi

  if command -v xray >/dev/null 2>&1; then
    local wrapper_path wrapper_real cellar_dir candidate
    wrapper_path="$(command -v xray)"
    wrapper_real="$(realpath "${wrapper_path}")"
    cellar_dir="$(cd "$(dirname "${wrapper_real}")/.." && pwd)"
    candidate="${cellar_dir}/share/xray"
    if [[ -d "${candidate}" ]]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  fi

  return 1
}

find_hysteria_binary() {
  if [[ -n "${HYSTERIA_BINARY:-}" && -x "${HYSTERIA_BINARY}" ]]; then
    printf '%s\n' "${HYSTERIA_BINARY}"
    return 0
  fi

  if [[ -x "${ROOT_DIR}/hysteria-darwin-arm64" ]]; then
    printf '%s\n' "${ROOT_DIR}/hysteria-darwin-arm64"
    return 0
  fi

  if command -v hysteria >/dev/null 2>&1; then
    printf '%s\n' "$(command -v hysteria)"
    return 0
  fi

  return 1
}

write_bundled_config() {
  perl -0pe \
    's/"binary"\s*:\s*"[^"]*xray[^"]*"/"binary": "bin\/xray"/g;
     s/"binary"\s*:\s*"[^"]*hysteria[^"]*"/"binary": "bin\/hysteria"/g' \
    "${CONFIG_SOURCE}" > "${CONFIG_DEST}"
}

write_xray_wrapper() {
  cat > "${BACKEND_DIR}/xray" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOURCES_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

export XRAY_LOCATION_ASSET="${XRAY_LOCATION_ASSET:-${RESOURCES_DIR}/xray-assets}"
exec "${SCRIPT_DIR}/xray-real" "$@"
EOF
  chmod +x "${BACKEND_DIR}/xray"
}

rm -rf "${APP_DIR}"
mkdir -p "${MACOS_DIR}" "${BACKEND_DIR}" "${XRAY_ASSETS_DIR}"

cat > "${CONTENTS_DIR}/Info.plist" <<EOF
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
  <string>${APP_VERSION}</string>
  <key>CFBundleVersion</key>
  <string>${APP_BUILD}</string>
  <key>LSMinimumSystemVersion</key>
  <string>13.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

env GOCACHE="${GOCACHE:-/tmp/gocache}" GOTMPDIR="${GOTMPDIR:-/tmp}" \
  go build \
    -trimpath \
    -ldflags="-s -w" \
    -o "${BACKEND_DIR}/vpnclient-ui" \
    ./cmd/vpnclient-ui

xray_binary="$(find_xray_binary)" || {
  echo "xray binary not found. Set XRAY_BINARY or install xray first." >&2
  exit 1
}

xray_assets_source="$(find_xray_assets_dir)" || {
  echo "xray assets directory not found. Set XRAY_ASSETS_DIR_SOURCE or install xray first." >&2
  exit 1
}

cp "${xray_binary}" "${BACKEND_DIR}/xray-real"
ditto "${xray_assets_source}" "${XRAY_ASSETS_DIR}"
write_xray_wrapper
write_bundled_config

if hysteria_binary="$(find_hysteria_binary)"; then
  cp "${hysteria_binary}" "${BACKEND_DIR}/hysteria"
  chmod +x "${BACKEND_DIR}/hysteria"
else
  echo "warning: hysteria binary not found; skipping bundled hysteria runtime" >&2
fi

clang++ \
  -std=c++20 \
  -O2 \
  -DNDEBUG \
  -fobjc-arc \
  -framework AppKit \
  -framework Foundation \
  -framework UniformTypeIdentifiers \
  -framework WebKit \
  -Wl,-dead_strip \
  -o "${MACOS_DIR}/VPNClient" \
  "${ROOT_DIR}/native/src/vpnclient_macos_app.mm"

chmod +x "${MACOS_DIR}/VPNClient" "${BACKEND_DIR}/vpnclient-ui" "${BACKEND_DIR}/xray-real"

echo "Built ${APP_DIR}"
