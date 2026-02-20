#!/bin/bash
# Build Windows MSI installer
# Supports: WiX Toolset v4 (wix), WiX v3 (candle+light), wixl (msitools)
# Recommended: run on windows-latest with `dotnet tool install --global wix`

set -e

VERSION=${1:-0.1.0}
ARCH=${2:-amd64}
WIX_ARCH=${3:-x64}   # WiX architecture: x64 | arm64 (for -arch flag)
BUILD_DIR="build/windows-${ARCH}"
OUTPUT_DIR="dist"

echo "Building Windows MSI installer for EvoClaw v${VERSION} (${ARCH})"

mkdir -p "$BUILD_DIR" "$OUTPUT_DIR"

# Copy the Windows binary
if [ -f "evoclaw-windows-${ARCH}.exe" ]; then
    cp "evoclaw-windows-${ARCH}.exe" "$BUILD_DIR/evoclaw.exe"
else
    echo "Error: evoclaw-windows-${ARCH}.exe not found"
    exit 1
fi

# Generate WiX v4 XML (compatible with `wix build`)
cat > "$BUILD_DIR/evoclaw.wxs" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://wixtoolset.org/schemas/v4/wxs">
  <Package Name="EvoClaw"
           Version="${VERSION}"
           Manufacturer="ClawInfra"
           UpgradeCode="A1B2C3D4-E5F6-47A8-B9C0-D1E2F3A4B5C6"
           Scope="perMachine">

    <SummaryInformation Description="EvoClaw - Self-Evolving AI Agent Framework"
                        Manufacturer="ClawInfra" />

    <MajorUpgrade DowngradeErrorMessage="A newer version of EvoClaw is already installed." />
    <MediaTemplate EmbedCab="yes" />

    <Feature Id="ProductFeature" Title="EvoClaw" Level="1">
      <ComponentGroupRef Id="ProductComponents" />
    </Feature>

    <StandardDirectory Id="ProgramFiles64Folder">
      <Directory Id="INSTALLFOLDER" Name="EvoClaw">
        <ComponentGroup Id="ProductComponents">
          <Component Id="MainBinary">
            <File Id="evoclaw.exe" Source="evoclaw.exe" KeyPath="yes" />
            <Environment Id="AddToPath"
                         Name="PATH"
                         Value="[INSTALLFOLDER]"
                         Permanent="no"
                         Part="last"
                         Action="set"
                         System="yes" />
          </Component>
        </ComponentGroup>
      </Directory>
    </StandardDirectory>

  </Package>
</Wix>
EOF

echo "Checking for MSI build tools..."

if command -v wix &> /dev/null; then
    # WiX Toolset v4 (dotnet tool) — preferred
    echo "✓ Found WiX v4 at: $(command -v wix)"
    cd "$BUILD_DIR"
    wix build -arch "${WIX_ARCH}" evoclaw.wxs -o "../../$OUTPUT_DIR/EvoClaw-${VERSION}-${ARCH}.msi"

elif command -v candle &> /dev/null && command -v light &> /dev/null; then
    # WiX v3 (candle + light) — convert to v3 format
    echo "✓ Found WiX v3"
    # Generate v3-compatible WXS
    cat > "$BUILD_DIR/evoclaw-v3.wxs" <<WIXEOF
<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi">
  <Product Id="*"
           Name="EvoClaw"
           Language="1033"
           Version="${VERSION}"
           Manufacturer="ClawInfra"
           UpgradeCode="A1B2C3D4-E5F6-47A8-B9C0-D1E2F3A4B5C6">
    <Package InstallerVersion="500" Compressed="yes" InstallScope="perMachine" />
    <MajorUpgrade DowngradeErrorMessage="A newer version of EvoClaw is already installed." />
    <MediaTemplate EmbedCab="yes" />
    <Feature Id="ProductFeature" Title="EvoClaw" Level="1">
      <ComponentGroupRef Id="ProductComponents" />
    </Feature>
    <Directory Id="TARGETDIR" Name="SourceDir">
      <Directory Id="ProgramFiles64Folder">
        <Directory Id="INSTALLFOLDER" Name="EvoClaw" />
      </Directory>
    </Directory>
    <ComponentGroup Id="ProductComponents" Directory="INSTALLFOLDER">
      <Component Id="MainBinary" Guid="B2C3D4E5-F6A7-48B9-C0D1-E2F3A4B5C6D7">
        <File Id="evoclaw.exe" Source="evoclaw.exe" KeyPath="yes" />
        <Environment Id="AddToPath" Name="PATH" Value="[INSTALLFOLDER]" Permanent="no" Part="last" Action="set" System="yes" />
      </Component>
    </ComponentGroup>
  </Product>
</Wix>
WIXEOF
    cd "$BUILD_DIR"
    candle evoclaw-v3.wxs -o evoclaw.wixobj
    light evoclaw.wixobj -o "../../$OUTPUT_DIR/EvoClaw-${VERSION}-${ARCH}.msi"

elif command -v wixl &> /dev/null; then
    # wixl from msitools (Linux fallback, v3 WXS)
    echo "✓ Found wixl (msitools)"
    cat > "$BUILD_DIR/evoclaw-v3.wxs" <<WIXEOF
<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi">
  <Product Id="*"
           Name="EvoClaw"
           Language="1033"
           Version="${VERSION}"
           Manufacturer="ClawInfra"
           UpgradeCode="A1B2C3D4-E5F6-47A8-B9C0-D1E2F3A4B5C6">
    <Package InstallerVersion="200" Compressed="yes" InstallScope="perMachine" />
    <MajorUpgrade DowngradeErrorMessage="A newer version of EvoClaw is already installed." />
    <MediaTemplate EmbedCab="yes" />
    <Feature Id="ProductFeature" Title="EvoClaw" Level="1">
      <ComponentGroupRef Id="ProductComponents" />
    </Feature>
    <Directory Id="TARGETDIR" Name="SourceDir">
      <Directory Id="ProgramFilesFolder">
        <Directory Id="INSTALLFOLDER" Name="EvoClaw" />
      </Directory>
    </Directory>
    <ComponentGroup Id="ProductComponents" Directory="INSTALLFOLDER">
      <Component Id="MainBinary" Guid="B2C3D4E5-F6A7-48B9-C0D1-E2F3A4B5C6D7">
        <File Id="evoclaw.exe" Source="evoclaw.exe" KeyPath="yes" />
      </Component>
    </ComponentGroup>
  </Product>
</Wix>
WIXEOF
    cd "$BUILD_DIR"
    wixl -v evoclaw-v3.wxs -o "../../$OUTPUT_DIR/EvoClaw-${VERSION}-${ARCH}.msi"

else
    echo "❌ No MSI build tool found."
    echo "   Install one of:"
    echo "   • WiX v4 (recommended): dotnet tool install --global wix"
    echo "   • msitools (Ubuntu):    apt-get install msitools"
    exit 1
fi

echo "✅ MSI installer created: $OUTPUT_DIR/EvoClaw-${VERSION}-${ARCH}.msi"
