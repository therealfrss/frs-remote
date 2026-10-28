#!/bin/bash
# Construiește "FRS Remote.app" (cu Moonlight inclus) și DMG-ul. Rulează pe macOS.
# Folosire: build_app.sh <versiune> <cale/frsremote> <cale/Moonlight.app>
set -euo pipefail

VERSION="$1"
BIN="$2"
MOONLIGHT_APP="$3"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$ROOT/dist"
APP="$OUT/FRS Remote.app"

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources" "$OUT"

sed "s/__VERSION__/$VERSION/g" "$ROOT/packaging/mac/Info.plist" > "$APP/Contents/Info.plist"
cp "$BIN" "$APP/Contents/MacOS/frsremote"
chmod +x "$APP/Contents/MacOS/frsremote"
ditto "$MOONLIGHT_APP" "$APP/Contents/Resources/Moonlight.app"

# iconiță
ICONSET="$(mktemp -d)/AppIcon.iconset"
mkdir -p "$ICONSET"
SRC="$ROOT/assets/app_icon_1024.png"
for s in 16 32 128 256 512; do
  sips -z $s $s "$SRC" --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
  d=$((s * 2))
  sips -z $d $d "$SRC" --out "$ICONSET/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"

# semnătură locală (ad-hoc) — suficientă pentru uz personal
codesign --force --deep --sign - "$APP"

# DMG cu scurtătură spre Applications
STAGE="$(mktemp -d)"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
rm -f "$OUT/FRS-Remote-$VERSION.dmg"
hdiutil create -volname "FRS Remote" -srcfolder "$STAGE" -ov -format UDZO "$OUT/FRS-Remote-$VERSION.dmg"
echo "Gata: $OUT/FRS-Remote-$VERSION.dmg"
