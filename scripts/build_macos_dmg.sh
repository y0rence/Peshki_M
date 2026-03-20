#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="VPNClient"
APP_VERSION="${APP_VERSION:-1.0.0}"
APP_BUILD="${APP_BUILD:-1}"
DMG_SUFFIX="${DMG_SUFFIX:-}"
APP_DIR="${ROOT_DIR}/build/${APP_NAME}.app"
DMG_ROOT="${ROOT_DIR}/build/dmg-root"
DMG_NAME="${APP_NAME}-${APP_VERSION}"
if [[ -n "${DMG_SUFFIX}" ]]; then
  DMG_NAME="${DMG_NAME}-${DMG_SUFFIX}"
fi
DMG_PATH="${ROOT_DIR}/build/${DMG_NAME}.dmg"

APP_VERSION="${APP_VERSION}" APP_BUILD="${APP_BUILD}" \
  "${ROOT_DIR}/scripts/build_macos_app.sh"

rm -rf "${DMG_ROOT}"
mkdir -p "${DMG_ROOT}"

ditto "${APP_DIR}" "${DMG_ROOT}/${APP_NAME}.app"
ln -s /Applications "${DMG_ROOT}/Applications"

hdiutil create \
  -volname "${APP_NAME}" \
  -srcfolder "${DMG_ROOT}" \
  -ov \
  -format UDZO \
  "${DMG_PATH}"

echo "Built ${DMG_PATH}"
