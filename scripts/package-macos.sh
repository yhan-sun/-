#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DIR"

echo "=== Building Hongguo Player (Go + Wails) ==="
/Users/pyu/env/go/bin/go build -ldflags="-s -w" -o "bin/hongguo-player" .

APP_NAME="Hongguo Player"
APP_BUNDLE="bin/${APP_NAME}.app"
CONTENTS="${APP_BUNDLE}/Contents"
MACOS="${CONTENTS}/MacOS"
RESOURCES="${CONTENTS}/Resources"

echo "=== Creating macOS App Bundle ==="
rm -rf "${APP_BUNDLE}"
mkdir -p "${MACOS}" "${RESOURCES}"

cp "bin/hongguo-player" "${MACOS}/hongguo-player"
chmod +x "${MACOS}/hongguo-player"

cat << 'EOF' > "${CONTENTS}/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleName</key>
    <string>Hongguo Player</string>
    <key>CFBundleDisplayName</key>
    <string>红果短剧</string>
    <key>CFBundleIdentifier</key>
    <string>com.hongguo.player</string>
    <key>CFBundleVersion</key>
    <string>7.9.0</string>
    <key>CFBundleShortVersionString</key>
    <string>7.9.0</string>
    <key>CFBundleExecutable</key>
    <string>hongguo-player</string>
    <key>CFBundleIconFile</key>
    <string>appicon</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSRequiresAquaSystemAppearance</key>
    <false/>
</dict>
</plist>
EOF

ICON_SOURCE="/Users/pyu/code/UU an/hongguo-drama/build/icon.png"
if [ -f "$ICON_SOURCE" ]; then
    echo "=== Generating ICNS icon ==="
    ICONSET="bin/app.iconset"
    rm -rf "${ICONSET}"
    mkdir -p "${ICONSET}"

    sips -z 16 16     "$ICON_SOURCE" --out "${ICONSET}/icon_16x16.png" >/dev/null
    sips -z 32 32     "$ICON_SOURCE" --out "${ICONSET}/icon_16x16@2x.png" >/dev/null
    sips -z 32 32     "$ICON_SOURCE" --out "${ICONSET}/icon_32x32.png" >/dev/null
    sips -z 64 64     "$ICON_SOURCE" --out "${ICONSET}/icon_32x32@2x.png" >/dev/null
    sips -z 128 128   "$ICON_SOURCE" --out "${ICONSET}/icon_128x128.png" >/dev/null
    sips -z 256 256   "$ICON_SOURCE" --out "${ICONSET}/icon_128x128@2x.png" >/dev/null
    sips -z 256 256   "$ICON_SOURCE" --out "${ICONSET}/icon_256x256.png" >/dev/null
    sips -z 512 512   "$ICON_SOURCE" --out "${ICONSET}/icon_256x256@2x.png" >/dev/null
    sips -z 512 512   "$ICON_SOURCE" --out "${ICONSET}/icon_512x512.png" >/dev/null
    sips -z 1024 1024 "$ICON_SOURCE" --out "${ICONSET}/icon_512x512@2x.png" >/dev/null

    iconutil -c icns "${ICONSET}" -o "${RESOURCES}/appicon.icns"
    rm -rf "${ICONSET}"
fi

echo "=== Packaging DMG ==="
DMG_PATH="bin/${APP_NAME}-7.9.0-arm64.dmg"
rm -f "${DMG_PATH}"
hdiutil create -volname "${APP_NAME}" -srcfolder "${APP_BUNDLE}" -ov -format UDZO "${DMG_PATH}"

echo "=== Packaging Complete ==="
echo "App Bundle: ${APP_BUNDLE}"
echo "DMG:        ${DMG_PATH}"
ls -lh "${DMG_PATH}"
