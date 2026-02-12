# Homebrew Release Process

Quick reference for updating the Homebrew tap when releasing a new version.

## Prerequisites

- New GitHub release created with macOS binaries
- Release tag pushed (e.g., `v1.0.0`)
- CI completed and uploaded `evoclaw-darwin-amd64.tar.gz` and `evoclaw-darwin-arm64.tar.gz`

## Steps

### 1. Download Release Binaries

```bash
VERSION="1.0.0"
wget https://github.com/clawinfra/evoclaw/releases/download/v${VERSION}/evoclaw-darwin-amd64.tar.gz
wget https://github.com/clawinfra/evoclaw/releases/download/v${VERSION}/evoclaw-darwin-arm64.tar.gz
```

### 2. Calculate SHA256 Checksums

```bash
shasum -a 256 evoclaw-darwin-amd64.tar.gz
shasum -a 256 evoclaw-darwin-arm64.tar.gz
```

Example output:
```
a1b2c3d4e5f6... evoclaw-darwin-amd64.tar.gz
f6e5d4c3b2a1... evoclaw-darwin-arm64.tar.gz
```

### 3. Update Formula

Clone the tap repo:
```bash
git clone https://github.com/clawinfra/homebrew-evoclaw.git
cd homebrew-evoclaw
```

Edit `Formula/evoclaw.rb`:
```ruby
class Evoclaw < Formula
  desc "Self-evolving AI agent framework for edge devices"
  homepage "https://github.com/clawinfra/evoclaw"
  version "1.0.0"  # ← Update this
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/clawinfra/evoclaw/releases/download/v1.0.0/evoclaw-darwin-arm64.tar.gz"
      sha256 "f6e5d4c3b2a1..."  # ← Update this (Apple Silicon)
    else
      url "https://github.com/clawinfra/evoclaw/releases/download/v1.0.0/evoclaw-darwin-amd64.tar.gz"
      sha256 "a1b2c3d4e5f6..."  # ← Update this (Intel)
    end
  end

  # ... rest of formula
end
```

### 4. Test Locally

```bash
# Audit formula
brew audit --new-formula evoclaw

# Test installation (uninstall first if already installed)
brew uninstall evoclaw || true
brew install --build-from-source evoclaw

# Verify
evoclaw --version

# Test upgrade path
brew upgrade evoclaw
```

### 5. Commit and Push

```bash
git add Formula/evoclaw.rb
git commit -m "chore: bump version to v${VERSION}"
git push origin main
```

### 6. Announce

Users will see the update on their next `brew update`. Optionally announce:
- GitHub release notes
- Discord
- Twitter

## Quick One-Liner

For fast updates (after testing locally):

```bash
VERSION="1.0.0" && \
cd /tmp && \
wget -q https://github.com/clawinfra/evoclaw/releases/download/v${VERSION}/evoclaw-darwin-{amd64,arm64}.tar.gz && \
AMD64_SHA=$(shasum -a 256 evoclaw-darwin-amd64.tar.gz | awk '{print $1}') && \
ARM64_SHA=$(shasum -a 256 evoclaw-darwin-arm64.tar.gz | awk '{print $1}') && \
echo "Version: ${VERSION}" && \
echo "AMD64 SHA256: ${AMD64_SHA}" && \
echo "ARM64 SHA256: ${ARM64_SHA}"
```

Then manually update the formula with the printed values.

## Automation (Future)

Consider automating this with GitHub Actions:
- Trigger on new release
- Calculate checksums
- Auto-update formula via PR
- Run brew audit
- Auto-merge if tests pass

## Troubleshooting

**Formula audit fails:**
```bash
brew audit --strict Formula/evoclaw.rb
```

**Installation fails:**
```bash
brew install --verbose --debug evoclaw
```

**Users report issues:**
1. Check GitHub release has correct binaries
2. Verify checksums match
3. Test installation on both Intel and Apple Silicon if possible
4. Check formula syntax with `brew audit`

## Resources

- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Homebrew Taps](https://docs.brew.sh/Taps)
- [Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae)
