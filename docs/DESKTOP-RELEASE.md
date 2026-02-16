# Desktop Release Guide

This guide covers the mainstream desktop release process for EvoClaw.

## Release Checklist

### 1. Pre-Release

- [ ] Version bump in `cmd/evoclaw/main.go`
- [ ] Update `CHANGELOG.md` with release notes
- [ ] Run full test suite: `go test ./... && cd edge-agent && cargo test`
- [ ] Build all platforms locally to verify
- [ ] Create release icons (see Icons section)
- [ ] Test installation on clean VMs (Windows, macOS, Ubuntu)

### 2. Build Artifacts

Artifacts are built automatically by GitHub Actions on tag push:

```bash
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```

CI builds:
- **Binaries**: Linux (amd64, arm64, armv7), macOS (amd64, arm64), Windows (amd64, arm64)
- **Packages**: .deb, .rpm (Linux), .dmg (macOS), .msi (Windows)
- **Archives**: .tar.gz, .zip for manual installation

### 3. Code Signing

#### macOS

Requires Apple Developer Program membership ($99/year):

1. Get Developer ID Application certificate from Apple
2. Set environment variables in CI:
   ```bash
   APPLE_DEVELOPER_ID="Developer ID Application: Your Name (TEAM_ID)"
   APPLE_ID="your.email@example.com"
   APPLE_PASSWORD="app-specific-password"
   APPLE_TEAM_ID="YOUR_TEAM_ID"
   ```
3. Scripts automatically sign and notarize

#### Windows

Requires code signing certificate (from DigiCert, Sectigo, etc.):

1. Purchase certificate (~$200-400/year)
2. Install certificate in CI runner
3. Use `signtool.exe` to sign MSI:
   ```powershell
   signtool sign /f cert.pfx /p password /tr http://timestamp.digicert.com evoclaw.msi
   ```

**For initial release**: Ship unsigned (users will see warnings, but can still install)

### 4. Package Testing

Test each package on clean systems:

#### macOS (.dmg)
```bash
# Download and mount
curl -LO https://github.com/clawinfra/evoclaw/releases/download/v0.2.0/EvoClaw-0.2.0-arm64.dmg
open EvoClaw-0.2.0-arm64.dmg
# Drag to Applications
# Launch and verify
```

#### Windows (.msi)
```powershell
# Download
Invoke-WebRequest -Uri "https://github.com/clawinfra/evoclaw/releases/download/v0.2.0/EvoClaw-0.2.0-amd64.msi" -OutFile evoclaw.msi
# Install
msiexec /i evoclaw.msi
# Verify
evoclaw --version
```

#### Linux (.deb)
```bash
# Ubuntu/Debian
wget https://github.com/clawinfra/evoclaw/releases/download/v0.2.0/evoclaw_0.2.0_amd64.deb
sudo dpkg -i evoclaw_0.2.0_amd64.deb
evoclaw --version
```

#### Linux (.rpm)
```bash
# Fedora/RHEL
wget https://github.com/clawinfra/evoclaw/releases/download/v0.2.0/evoclaw-0.2.0-1.x86_64.rpm
sudo rpm -i evoclaw-0.2.0-1.x86_64.rpm
evoclaw --version
```

### 5. Installation Script

Test the one-liner install:

```bash
# Linux/macOS
curl -fsSL https://evoclaw.win/install.sh | sh
```

Requirements:
- Host `scripts/install.sh` at https://evoclaw.win/install.sh
- Set up DNS: evoclaw.win → GitHub Pages or your server
- Optionally, set up APT/YUM repos for automatic updates

### 6. Release Announcement

Update these locations:
- [ ] GitHub Releases page with full changelog
- [ ] README.md installation instructions
- [ ] Website (evoclaw.win)
- [ ] Discord announcement
- [ ] Twitter/social media

## Icons

### Requirements

- **macOS**: `AppIcon.icns` (1024x1024 source)
- **Windows**: `evoclaw.ico` (256x256 source)
- **Linux**: `evoclaw.png` (48x48, 256x256, 512x512)

### Generation

From a 1024x1024 PNG (`icon-1024.png`):

```bash
# macOS .icns
mkdir AppIcon.iconset
sips -z 16 16     icon-1024.png --out AppIcon.iconset/icon_16x16.png
sips -z 32 32     icon-1024.png --out AppIcon.iconset/icon_16x16@2x.png
sips -z 32 32     icon-1024.png --out AppIcon.iconset/icon_32x32.png
sips -z 64 64     icon-1024.png --out AppIcon.iconset/icon_32x32@2x.png
sips -z 128 128   icon-1024.png --out AppIcon.iconset/icon_128x128.png
sips -z 256 256   icon-1024.png --out AppIcon.iconset/icon_128x128@2x.png
sips -z 256 256   icon-1024.png --out AppIcon.iconset/icon_256x256.png
sips -z 512 512   icon-1024.png --out AppIcon.iconset/icon_256x256@2x.png
sips -z 512 512   icon-1024.png --out AppIcon.iconset/icon_512x512.png
sips -z 1024 1024 icon-1024.png --out AppIcon.iconset/icon_512x512@2x.png
iconutil -c icns AppIcon.iconset

# Windows .ico (requires ImageMagick)
convert icon-1024.png -resize 256x256 evoclaw.ico

# Linux .png
convert icon-1024.png -resize 48x48 evoclaw-48.png
convert icon-1024.png -resize 256x256 evoclaw-256.png
convert icon-1024.png -resize 512x512 evoclaw-512.png
```

## Auto-Update

EvoClaw checks for updates every 24 hours (configurable).

**User commands:**
```bash
evoclaw update check          # Check for updates
evoclaw update install        # Install latest
evoclaw update enable         # Enable auto-check
evoclaw update enable --auto-install  # Enable auto-install
evoclaw update disable        # Disable
```

**Config (`config.json`):**
```json
{
  "updates": {
    "enabled": true,
    "autoInstall": false,
    "checkInterval": 86400,
    "includePrereleases": false
  }
}
```

## Distribution Channels

### 1. GitHub Releases (Primary)

All platforms, all architectures. Always available.

### 2. Homebrew (macOS)

```bash
brew tap clawinfra/evoclaw
brew install evoclaw
```

Already set up. Auto-updated via `homebrew-release.yml`.

### 3. APT Repository (Ubuntu/Debian) - Future

```bash
curl -fsSL https://apt.evoclaw.win/gpg | sudo gpg --dearmor -o /usr/share/keyrings/evoclaw.gpg
echo "deb [signed-by=/usr/share/keyrings/evoclaw.gpg] https://apt.evoclaw.win stable main" | sudo tee /etc/apt/sources.list.d/evoclaw.list
sudo apt update
sudo apt install evoclaw
```

### 4. Snap Store (Linux) - Future

```bash
sudo snap install evoclaw
```

### 5. Microsoft Store (Windows) - Future

Requires Microsoft Partner account and app certification.

### 6. Mac App Store - Future

Requires Apple Developer Program and app review (strict sandboxing).

## Rollback Plan

If a release has critical bugs:

1. **Immediate**: Remove GitHub release (mark as draft)
2. **Homebrew**: Revert formula commit
3. **Notify users**: Post issue, Discord announcement
4. **Hotfix**: Create patch release (e.g., v0.2.1)

## Security

### Vulnerability Disclosure

- Email: security@evoclaw.ai
- Private GitHub security advisories

### Supply Chain

- [ ] Enable GitHub Actions OIDC for artifact signing
- [ ] Generate SBOM (Software Bill of Materials)
- [ ] Sign binaries with Sigstore/Cosign
- [ ] Publish checksums (SHA256) with releases

## Metrics

Track via GitHub API:
- Download counts per platform
- Update adoption rate
- Installation success rate (via telemetry if enabled)

## Licensing

All packages include:
- MIT License in `LICENSE` file
- Third-party attributions in `THIRD_PARTY_NOTICES`
- Privacy policy (if telemetry enabled)

---

**Next Steps for Mainstream Release:**

1. **Create icon** (1024x1024 PNG) and generate all formats
2. **Get code signing certificates** (Apple Developer ID + Windows cert)
3. **Set up evoclaw.win domain** and host install script
4. **Test on clean VMs** (Windows 11, macOS Sonoma, Ubuntu 24.04)
5. **Write user-friendly onboarding docs** (non-technical audience)
6. **Create quick start video** (2-3 minutes)
7. **Set up APT/YUM repos** for automatic updates
8. **Consider GUI setup wizard** (Electron or Tauri app)

**Estimated timeline**: 1-2 weeks for full mainstream polish (with certificates).
