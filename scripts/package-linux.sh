#!/bin/bash
# Build Linux .deb and .rpm packages

set -e

VERSION=${1:-0.1.0}
ARCH=${2:-amd64}  # amd64, arm64, or armv7
BUILD_DIR="build/linux-${ARCH}"
OUTPUT_DIR="dist"

# Sanitize version for RPM (cannot contain hyphens in Version field)
# Split version into Version and Release parts
RPM_VERSION=$(echo "$VERSION" | cut -d'-' -f1)
RPM_RELEASE="1"
if [[ "$VERSION" == *"-"* ]]; then
    RPM_SUFFIX=$(echo "$VERSION" | cut -d'-' -f2)
    RPM_RELEASE_NUM=$(echo "$RPM_SUFFIX" | sed 's/[^0-9]//g')
    if [[ -n "$RPM_RELEASE_NUM" ]]; then
        RPM_RELEASE="0.${RPM_RELEASE_NUM}"
    else
        RPM_RELEASE="0.1"
    fi
fi

echo "Building Linux packages for EvoClaw v${VERSION} (${ARCH})"
echo "DEB: Version=${VERSION}"
echo "RPM: Version=${RPM_VERSION} Release=${RPM_RELEASE}"
echo "Debug: VERSION='${VERSION}' RPM_VERSION='${RPM_VERSION}' RPM_RELEASE='${RPM_RELEASE}'"

# Create build directories
mkdir -p "$BUILD_DIR" "$OUTPUT_DIR"

# Download or copy the Linux binary
if [ -f "evoclaw-linux-${ARCH}" ]; then
    cp "evoclaw-linux-${ARCH}" "$BUILD_DIR/evoclaw"
else
    echo "Error: evoclaw-linux-${ARCH} not found"
    echo "Build it first with: GOOS=linux GOARCH=${ARCH} go build -o evoclaw-linux-${ARCH} ./cmd/evoclaw"
    exit 1
fi

chmod +x "$BUILD_DIR/evoclaw"

# Map Go arch to Debian arch
case "$ARCH" in
    amd64)
        DEB_ARCH="amd64"
        RPM_ARCH="x86_64"
        ;;
    arm64)
        DEB_ARCH="arm64"
        RPM_ARCH="aarch64"
        ;;
    armv7)
        DEB_ARCH="armhf"
        RPM_ARCH="armv7hl"
        ;;
    *)
        echo "Unknown architecture: $ARCH"
        exit 1
        ;;
esac

# ─────────────────────────────────────────────────────────
# Build .deb package
# ─────────────────────────────────────────────────────────

DEB_DIR="$BUILD_DIR/deb"
mkdir -p "$DEB_DIR/DEBIAN"
mkdir -p "$DEB_DIR/usr/bin"
mkdir -p "$DEB_DIR/usr/share/applications"
mkdir -p "$DEB_DIR/usr/share/pixmaps"
mkdir -p "$DEB_DIR/lib/systemd/system"

# Copy binary
cp "$BUILD_DIR/evoclaw" "$DEB_DIR/usr/bin/evoclaw"

# Create control file
cat > "$DEB_DIR/DEBIAN/control" <<EOF
Package: evoclaw
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${DEB_ARCH}
Maintainer: ClawInfra <hello@evoclaw.ai>
Homepage: https://github.com/clawinfra/evoclaw
Description: Self-Evolving AI Agent Framework
 EvoClaw is a lightweight, evolution-powered agent orchestration
 framework designed to run on resource-constrained edge devices.
 Every device becomes an agent. Every agent evolves.
EOF

# Create postinst script
cat > "$DEB_DIR/DEBIAN/postinst" <<'EOF'
#!/bin/bash
set -e

# Create evoclaw user if it doesn't exist
if ! id evoclaw >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /bin/false evoclaw
fi

# Create config directory
mkdir -p /etc/evoclaw
chown evoclaw:evoclaw /etc/evoclaw

# Enable and start service
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable evoclaw
    echo "EvoClaw installed. Start with: sudo systemctl start evoclaw"
else
    echo "EvoClaw installed. Run: evoclaw init"
fi

exit 0
EOF

chmod +x "$DEB_DIR/DEBIAN/postinst"

# Create postrm script
cat > "$DEB_DIR/DEBIAN/postrm" <<'EOF'
#!/bin/bash
set -e

if [ "$1" = "purge" ]; then
    # Remove user and config
    if id evoclaw >/dev/null 2>&1; then
        userdel evoclaw
    fi
    rm -rf /etc/evoclaw
fi

exit 0
EOF

chmod +x "$DEB_DIR/DEBIAN/postrm"

# Create .desktop file
cat > "$DEB_DIR/usr/share/applications/evoclaw.desktop" <<EOF
[Desktop Entry]
Name=EvoClaw
Comment=Self-Evolving AI Agent Framework
Exec=evoclaw web
Icon=evoclaw
Terminal=false
Type=Application
Categories=Development;Utility;Network;
StartupNotify=false
EOF

# Create placeholder icon
if [ ! -f "$DEB_DIR/usr/share/pixmaps/evoclaw.png" ]; then
    # You should replace this with a real 48x48 PNG icon
    touch "$DEB_DIR/usr/share/pixmaps/evoclaw.png"
fi

# Create systemd service file
cat > "$DEB_DIR/lib/systemd/system/evoclaw.service" <<EOF
[Unit]
Description=EvoClaw Self-Evolving Agent Framework
After=network.target

[Service]
Type=simple
User=evoclaw
Group=evoclaw
WorkingDirectory=/etc/evoclaw
ExecStart=/usr/bin/evoclaw --config /etc/evoclaw/config.json
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# Build .deb
dpkg-deb --build "$DEB_DIR" "$OUTPUT_DIR/evoclaw_${VERSION}_${DEB_ARCH}.deb"

echo "✅ .deb package created: $OUTPUT_DIR/evoclaw_${VERSION}_${DEB_ARCH}.deb"

# ─────────────────────────────────────────────────────────
# Build .rpm package
# ─────────────────────────────────────────────────────────

# Skip RPM for ARM - rpmbuild on Ubuntu x86_64 doesn't have ARM arch definitions
if [ "$ARCH" = "arm64" ] || [ "$ARCH" = "armv7" ]; then
    echo "⚠️  Skipping .rpm package for ${ARCH} (cross-compilation not supported by Ubuntu rpmbuild)"
    echo "Note: .deb package is available for ${ARCH}"
elif command -v rpmbuild &> /dev/null; then
    RPM_DIR="$BUILD_DIR/rpm"
    mkdir -p "$RPM_DIR"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
    
    # Create tarball (use sanitized version)
    RPM_TARBALL_NAME="evoclaw-${RPM_VERSION}.tar.gz"
    tar -czf "$RPM_DIR/SOURCES/${RPM_TARBALL_NAME}" -C "$BUILD_DIR" evoclaw
    
    # Create spec file
    cat > "$RPM_DIR/SPECS/evoclaw.spec" <<EOF
Name:           evoclaw
Version:        ${RPM_VERSION}
Release:        ${RPM_RELEASE}%{?dist}
Summary:        Self-Evolving AI Agent Framework
License:        MIT
URL:            https://github.com/clawinfra/evoclaw
Source0:        %{name}-${RPM_VERSION}.tar.gz
BuildArch:      ${RPM_ARCH}

%description
EvoClaw is a lightweight, evolution-powered agent orchestration
framework designed to run on resource-constrained edge devices.
Every device becomes an agent. Every agent evolves.

%prep
%setup -q -c -T
tar -xzf %{SOURCE0}

%install
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/usr/lib/systemd/system
mkdir -p %{buildroot}/usr/share/applications
mkdir -p %{buildroot}/usr/share/pixmaps

install -m 0755 evoclaw %{buildroot}/usr/bin/evoclaw

cat > %{buildroot}/usr/lib/systemd/system/evoclaw.service <<'SYSTEMD'
[Unit]
Description=EvoClaw Self-Evolving Agent Framework
After=network.target

[Service]
Type=simple
User=evoclaw
Group=evoclaw
WorkingDirectory=/etc/evoclaw
ExecStart=/usr/bin/evoclaw --config /etc/evoclaw/config.json
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SYSTEMD

cat > %{buildroot}/usr/share/applications/evoclaw.desktop <<'DESKTOP'
[Desktop Entry]
Name=EvoClaw
Comment=Self-Evolving AI Agent Framework
Exec=evoclaw web
Icon=evoclaw
Terminal=false
Type=Application
Categories=Development;Utility;Network;
DESKTOP

%files
/usr/bin/evoclaw
/usr/lib/systemd/system/evoclaw.service
/usr/share/applications/evoclaw.desktop

%post
getent group evoclaw >/dev/null || groupadd -r evoclaw
getent passwd evoclaw >/dev/null || useradd -r -g evoclaw -s /sbin/nologin evoclaw
mkdir -p /etc/evoclaw
chown evoclaw:evoclaw /etc/evoclaw
systemctl daemon-reload

%postun
if [ \$1 -eq 0 ]; then
    userdel evoclaw 2>/dev/null || true
    rm -rf /etc/evoclaw
fi

%changelog
* $(date '+%a %b %d %Y') ClawInfra <hello@evoclaw.ai> - ${RPM_VERSION}-${RPM_RELEASE}
- Release ${RPM_VERSION}-${RPM_RELEASE}

EOF
    
    # Debug: Show what was written to spec file
    echo "Debug: Spec file Version line:"
    grep -E "^(Name|Version|Release):" "$RPM_DIR/SPECS/evoclaw.spec" || true
    
    # Build RPM
    rpmbuild --define "_topdir $RPM_DIR" -bb "$RPM_DIR/SPECS/evoclaw.spec"
    
    # Copy to output
    cp "$RPM_DIR/RPMS/${RPM_ARCH}/evoclaw-${RPM_VERSION}-${RPM_RELEASE}."*".${RPM_ARCH}.rpm" "$OUTPUT_DIR/"
    
    echo "✅ .rpm package created: $OUTPUT_DIR/evoclaw-${RPM_VERSION}-${RPM_RELEASE}.*.${RPM_ARCH}.rpm"
else
    echo "⚠️  rpmbuild not found, skipping .rpm package"
    echo "Install with: apt-get install rpm (Debian/Ubuntu) or dnf install rpm-build (Fedora)"
fi

echo ""
echo "Linux packages created successfully!"
echo "Install .deb: sudo dpkg -i $OUTPUT_DIR/evoclaw_${VERSION}_${DEB_ARCH}.deb"
echo "Install .rpm: sudo rpm -i $OUTPUT_DIR/evoclaw-${RPM_VERSION}-${RPM_RELEASE}.*.${RPM_ARCH}.rpm"
