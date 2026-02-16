#!/bin/bash
# Build Windows MSI installer using WiX Toolset
# Requires: wixl (Linux), WiX Toolset (Windows), or msitools

set -e

VERSION=${1:-0.1.0}
ARCH=${2:-amd64}
BUILD_DIR="build/windows-${ARCH}"
OUTPUT_DIR="dist"

echo "Building Windows MSI installer for EvoClaw v${VERSION} (${ARCH})"

# Create build directory
mkdir -p "$BUILD_DIR" "$OUTPUT_DIR"

# Download or copy the Windows binary
if [ -f "evoclaw-windows-${ARCH}.exe" ]; then
    cp "evoclaw-windows-${ARCH}.exe" "$BUILD_DIR/evoclaw.exe"
else
    echo "Error: evoclaw-windows-${ARCH}.exe not found"
    echo "Build it first with: GOOS=windows GOARCH=${ARCH} go build -o evoclaw-windows-${ARCH}.exe ./cmd/evoclaw"
    exit 1
fi

# Generate WiX XML
cat > "$BUILD_DIR/evoclaw.wxs" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi">
  <Product Id="*" 
           Name="EvoClaw" 
           Language="1033" 
           Version="VERSION_PLACEHOLDER" 
           Manufacturer="ClawInfra" 
           UpgradeCode="A1B2C3D4-E5F6-47A8-B9C0-D1E2F3A4B5C6">
    
    <Package InstallerVersion="200" Compressed="yes" InstallScope="perMachine" />
    
    <MajorUpgrade DowngradeErrorMessage="A newer version of [ProductName] is already installed." />
    
    <MediaTemplate EmbedCab="yes" />

    <Feature Id="ProductFeature" Title="EvoClaw" Level="1">
      <ComponentGroupRef Id="ProductComponents" />
      <ComponentRef Id="ApplicationShortcut" />
      <ComponentRef Id="EnvironmentPath" />
    </Feature>

    <Icon Id="evoclaw.ico" SourceFile="evoclaw.ico" />
    <Property Id="ARPPRODUCTICON" Value="evoclaw.ico" />
    <Property Id="ARPHELPLINK" Value="https://github.com/clawinfra/evoclaw" />
    
    <Directory Id="TARGETDIR" Name="SourceDir">
      <Directory Id="ProgramFilesFolder">
        <Directory Id="INSTALLFOLDER" Name="EvoClaw" />
      </Directory>
      
      <Directory Id="ProgramMenuFolder">
        <Directory Id="ApplicationProgramsFolder" Name="EvoClaw"/>
      </Directory>
    </Directory>

    <DirectoryRef Id="INSTALLFOLDER">
      <Component Id="evoclaw.exe" Guid="B2C3D4E5-F6A7-48B9-C0D1-E2F3A4B5C6D7">
        <File Id="evoclaw.exe" Source="evoclaw.exe" KeyPath="yes" />
        <Environment Id="PATH" Name="PATH" Value="[INSTALLFOLDER]" Permanent="no" Part="last" Action="set" System="yes" />
      </Component>
    </DirectoryRef>

    <DirectoryRef Id="ApplicationProgramsFolder">
      <Component Id="ApplicationShortcut" Guid="C3D4E5F6-A7B8-49C0-D1E2-F3A4B5C6D7E8">
        <Shortcut Id="ApplicationStartMenuShortcut"
                  Name="EvoClaw"
                  Description="Self-Evolving AI Agent Framework"
                  Target="[INSTALLFOLDER]evoclaw.exe"
                  Arguments="web"
                  WorkingDirectory="INSTALLFOLDER"
                  Icon="evoclaw.ico" />
        <RemoveFolder Id="CleanUpShortCut" Directory="ApplicationProgramsFolder" On="uninstall"/>
        <RegistryValue Root="HKCU" Key="Software\EvoClaw\EvoClaw" Name="installed" Type="integer" Value="1" KeyPath="yes"/>
      </Component>
      
      <Component Id="EnvironmentPath" Guid="D4E5F6A7-B8C9-40D1-E2F3-A4B5C6D7E8F9">
        <Environment Id="PATH_ENV" Name="PATH" Value="[INSTALLFOLDER]" Permanent="no" Part="last" Action="set" System="yes" />
        <RegistryValue Root="HKCU" Key="Software\EvoClaw\EvoClaw" Name="path" Type="integer" Value="1" KeyPath="yes"/>
      </Component>
    </DirectoryRef>

    <ComponentGroup Id="ProductComponents" Directory="INSTALLFOLDER">
      <ComponentRef Id="evoclaw.exe" />
    </ComponentGroup>
  </Product>
</Wix>
EOF

# Replace version placeholder
sed -i "s/VERSION_PLACEHOLDER/${VERSION}/g" "$BUILD_DIR/evoclaw.wxs"

# Create a placeholder icon if not exists
if [ ! -f "$BUILD_DIR/evoclaw.ico" ]; then
    echo "Warning: evoclaw.ico not found, using placeholder"
    # Create a minimal ICO file (you should replace this with a real icon)
    touch "$BUILD_DIR/evoclaw.ico"
fi

# Build MSI
echo "Checking for MSI build tools..."
if command -v wixl &> /dev/null; then
    # Linux (msitools)
    echo "✓ Found wixl at: $(command -v wixl)"
    echo "Building with wixl (msitools)..."
    cd "$BUILD_DIR"
    wixl -v evoclaw.wxs -o "../../$OUTPUT_DIR/EvoClaw-${VERSION}-${ARCH}.msi"
elif [ -x "/usr/bin/wixl" ]; then
    # Direct path check (sometimes PATH isn't updated immediately after install)
    echo "✓ Found wixl at: /usr/bin/wixl"
    echo "Building with wixl (msitools)..."
    cd "$BUILD_DIR"
    /usr/bin/wixl -v evoclaw.wxs -o "../../$OUTPUT_DIR/EvoClaw-${VERSION}-${ARCH}.msi"
elif command -v candle &> /dev/null && command -v light &> /dev/null; then
    # Windows (WiX Toolset)
    echo "✓ Found WiX Toolset"
    echo "Building with WiX Toolset..."
    cd "$BUILD_DIR"
    candle evoclaw.wxs
    light -ext WixUIExtension evoclaw.wixobj -out "../../$OUTPUT_DIR/EvoClaw-${VERSION}-${ARCH}.msi"
else
    echo "⚠️  Warning: Neither wixl (msitools) nor WiX Toolset found"
    echo "Searched:"
    echo "  - command -v wixl: $(command -v wixl || echo 'not found')"
    echo "  - /usr/bin/wixl: $([ -x /usr/bin/wixl ] && echo 'not found')"
    echo "  - command -v candle: $(command -v candle || echo 'not found')"
    echo ""
    echo "MSI building requires wixl (msitools) or WiX Toolset."
    echo "On Ubuntu: apt-get install msitools"
    echo "Note: msitools may not be available on all Ubuntu versions."
    echo ""
    echo "Skipping MSI build. The binary can still be distributed as .exe"
    exit 0
fi

echo "✅ MSI installer created: $OUTPUT_DIR/EvoClaw-${VERSION}-${ARCH}.msi"
