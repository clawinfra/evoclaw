#!/bin/bash
# Build macOS DMG installer with .app bundle
# Requires: create-dmg (brew install create-dmg)

set -e

VERSION=${1:-0.1.0}
ARCH=${2:-arm64}  # arm64 or amd64
BUILD_DIR="build/macos-${ARCH}"
OUTPUT_DIR="dist"
APP_NAME="EvoClaw"

echo "Building macOS DMG for EvoClaw v${VERSION} (${ARCH})"

# Create build directory
mkdir -p "$BUILD_DIR" "$OUTPUT_DIR"

# Download or copy the macOS binary
if [ -f "evoclaw-darwin-${ARCH}" ]; then
    cp "evoclaw-darwin-${ARCH}" "$BUILD_DIR/evoclaw"
else
    echo "Error: evoclaw-darwin-${ARCH} not found"
    echo "Build it first with: GOOS=darwin GOARCH=${ARCH} go build -o evoclaw-darwin-${ARCH} ./cmd/evoclaw"
    exit 1
fi

chmod +x "$BUILD_DIR/evoclaw"

# Create .app bundle structure
APP_BUNDLE="$BUILD_DIR/${APP_NAME}.app"
mkdir -p "$APP_BUNDLE/Contents/"{MacOS,Resources}

# Copy binary
cp "$BUILD_DIR/evoclaw" "$APP_BUNDLE/Contents/MacOS/evoclaw"

# Create Info.plist
cat > "$APP_BUNDLE/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleDisplayName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleExecutable</key>
    <string>evoclaw</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundleIdentifier</key>
    <string>ai.evoclaw.app</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.13</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>LSUIElement</key>
    <false/>
    <key>CFBundleURLTypes</key>
    <array>
        <dict>
            <key>CFBundleURLSchemes</key>
            <array>
                <string>evoclaw</string>
            </array>
        </dict>
    </array>
</dict>
</plist>
EOF

# Create a launch wrapper that opens web interface
cat > "$APP_BUNDLE/Contents/MacOS/evoclaw-launch.sh" <<'EOF'
#!/bin/bash
# Launch EvoClaw and open web interface
cd "$HOME"
"$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/evoclaw" web &
sleep 2
open "http://localhost:8420"
EOF

chmod +x "$APP_BUNDLE/Contents/MacOS/evoclaw-launch.sh"

# Update Info.plist to use launch wrapper
sed -i '' 's/<string>evoclaw<\/string>/<string>evoclaw-launch.sh<\/string>/' "$APP_BUNDLE/Contents/Info.plist"

# Create placeholder icon (should be replaced with real .icns file)
if [ ! -f "$APP_BUNDLE/Contents/Resources/AppIcon.icns" ]; then
    echo "Warning: AppIcon.icns not found, creating placeholder"
    # You should generate a proper .icns file from PNG using:
    # iconutil -c icns AppIcon.iconset
    touch "$APP_BUNDLE/Contents/Resources/AppIcon.icns"
fi

# Sign the app bundle (requires Apple Developer certificate)
if [ -n "$APPLE_DEVELOPER_ID" ]; then
    echo "Signing app bundle..."
    codesign --force --deep --sign "$APPLE_DEVELOPER_ID" "$APP_BUNDLE"
else
    echo "Warning: APPLE_DEVELOPER_ID not set, skipping code signing"
    echo "Users will see 'unidentified developer' warning"
fi

# Create DMG
DMG_PATH="$OUTPUT_DIR/${APP_NAME}-${VERSION}-${ARCH}.dmg"

if command -v create-dmg &> /dev/null; then
    echo "Creating DMG with create-dmg..."
    create-dmg \
        --volname "${APP_NAME}" \
        --volicon "$APP_BUNDLE/Contents/Resources/AppIcon.icns" \
        --window-pos 200 120 \
        --window-size 600 400 \
        --icon-size 100 \
        --icon "${APP_NAME}.app" 175 120 \
        --hide-extension "${APP_NAME}.app" \
        --app-drop-link 425 120 \
        "$DMG_PATH" \
        "$BUILD_DIR"
else
    echo "create-dmg not found, creating simple DMG..."
    hdiutil create -volname "${APP_NAME}" -srcfolder "$BUILD_DIR" -ov -format UDZO "$DMG_PATH"
fi

# Notarize the DMG (requires Apple ID)
if [ -n "$APPLE_ID" ] && [ -n "$APPLE_PASSWORD" ]; then
    echo "Notarizing DMG..."
    xcrun notarytool submit "$DMG_PATH" \
        --apple-id "$APPLE_ID" \
        --password "$APPLE_PASSWORD" \
        --team-id "$APPLE_TEAM_ID" \
        --wait
    
    echo "Stapling notarization..."
    xcrun stapler staple "$DMG_PATH"
else
    echo "Warning: Apple ID not set, skipping notarization"
    echo "Users will see 'unverified developer' warnings"
fi

echo "✅ DMG created: $DMG_PATH"
echo ""
echo "To complete macOS release:"
echo "1. Generate proper AppIcon.icns from PNG using iconutil"
echo "2. Set APPLE_DEVELOPER_ID for code signing"
echo "3. Set APPLE_ID, APPLE_PASSWORD, APPLE_TEAM_ID for notarization"
