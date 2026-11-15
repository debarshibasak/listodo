#!/bin/sh
set -eu
bundle="build/Listodo.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
cp build/listodo "$bundle/Contents/MacOS/listodo"
cp assets/Listodo.icns "$bundle/Contents/Resources/Listodo.icns"
cat > "$bundle/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>Listodo</string>
<key>CFBundleDisplayName</key><string>Listodo</string>
<key>CFBundleExecutable</key><string>listodo</string>
<key>CFBundleIconFile</key><string>Listodo</string>
<key>CFBundleIdentifier</key><string>com.adaptivescale.listodo</string>
<key>CFBundleVersion</key><string>2</string>
<key>CFBundleShortVersionString</key><string>0.2.0</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>NSHighResolutionCapable</key><true/>
<key>NSSupportsAutomaticGraphicsSwitching</key><true/>
<key>LSMinimumSystemVersion</key><string>13.3</string>
<key>NSAppTransportSecurity</key><dict><key>NSAllowsLocalNetworking</key><true/></dict>
</dict></plist>
PLIST
codesign --force --deep --sign - "$bundle"
echo "Built $bundle"
