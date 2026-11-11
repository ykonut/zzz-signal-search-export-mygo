#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
APP_NAME="zzz-signal-search-export-mygo"
OUT="$ROOT/dist/$APP_NAME.app"
GO_BIN="${GO_BIN:-$(command -v go || true)}"
if [ -z "$GO_BIN" ] && [ -x /tmp/go/bin/go ]; then GO_BIN=/tmp/go/bin/go; fi
if [ -z "$GO_BIN" ]; then echo "Go toolchain not found"; exit 1; fi
export PATH="$(dirname "$GO_BIN"):$PATH"
export GOPATH="${GOPATH:-/tmp/gopath}"
export GOMODCACHE="${GOMODCACHE:-/tmp/gomodcache}"
export GOCACHE="${GOCACHE:-/tmp/gocache}"

cd "$ROOT"
rm -rf "$OUT"
mkdir -p "$OUT/Contents/MacOS" "$OUT/Contents/Resources"
"$GO_BIN" build -trimpath -ldflags="-s -w" -o "$OUT/Contents/MacOS/zzz-signal-search-export-mygo" .

cat > "$OUT/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>zzz-signal-search-export-mygo</string>
  <key>CFBundleDisplayName</key><string>绝区零调频记录导出</string>
  <key>CFBundleIdentifier</key><string>local.ykonut.zzz-signal-search-export-mygo</string>
  <key>CFBundleVersion</key><string>1.0.0</string>
  <key>CFBundleShortVersionString</key><string>1.0.0</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>zzz-signal-search-export-mygo</string>
  <key>CFBundleIconFile</key><string>icon.icns</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>LSUIElement</key><false/>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSAppTransportSecurity</key>
  <dict>
    <key>NSAllowsArbitraryLoads</key><true/>
    <key>NSAllowsLocalNetworking</key><true/>
  </dict>
</dict>
</plist>
PLIST
echo "APPL????" > "$OUT/Contents/PkgInfo"
if [ -f "$ROOT/assets/icon.icns" ]; then cp "$ROOT/assets/icon.icns" "$OUT/Contents/Resources/icon.icns"; fi
xattr -cr "$OUT" 2>/dev/null || true
codesign --force --deep --sign - "$OUT" >/dev/null 2>&1 || true
echo "built: $OUT"
