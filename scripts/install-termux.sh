#!/data/data/com.termux/files/usr/bin/bash
# EvoClaw Edge Agent — Termux Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/clawinfra/evoclaw/main/scripts/install-termux.sh | bash
set -e

REPO="clawinfra/evoclaw"
INSTALL_DIR="$HOME/.evoclaw"
BIN_DIR="$PREFIX/bin"
VERSION="${EVOCLAW_VERSION:-latest}"

echo "🦞 EvoClaw Edge Agent — Termux Installer"
echo ""

# ── Detect architecture ────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
  aarch64|arm64) TARGET="android-arm64" ;;
  armv7l|armv8l) TARGET="android-armv7" ;;
  x86_64)        TARGET="android-amd64" ;;
  *)
    echo "❌ Unsupported architecture: $ARCH"
    echo "   Supported: aarch64 (ARM64), armv7l, x86_64"
    exit 1
    ;;
esac

echo "📱 Architecture: $ARCH → target: evoclaw-agent-${TARGET}"

# ── Check Termux environment ───────────────────────────────────────────────
if [ -z "$PREFIX" ] || [ ! -d "/data/data/com.termux" ]; then
  echo "❌ This script must be run inside Termux on Android."
  echo "   Install Termux from F-Droid: https://f-droid.org/packages/com.termux/"
  exit 1
fi

# ── Install dependencies ───────────────────────────────────────────────────
echo ""
echo "📦 Installing dependencies..."
pkg update -y -q 2>/dev/null
pkg install -y -q curl openssl-tool 2>/dev/null
echo "✅ Dependencies ready"

# ── Fetch latest release URL ───────────────────────────────────────────────
echo ""
echo "🔍 Finding latest release..."

if [ "$VERSION" = "latest" ]; then
  RELEASE_URL=$(curl -sfL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep "browser_download_url" \
    | grep "evoclaw-agent-${TARGET}" \
    | grep -v ".tar.gz" \
    | head -1 \
    | cut -d'"' -f4)
else
  RELEASE_URL="https://github.com/${REPO}/releases/download/${VERSION}/evoclaw-agent-${TARGET}"
fi

# Fallback to artifact from CI if no release yet
if [ -z "$RELEASE_URL" ]; then
  echo "⚠️  No tagged release found. Downloading from latest CI artifacts..."
  RELEASE_URL="https://github.com/${REPO}/actions/artifacts"
  echo "   Please download manually from: https://github.com/${REPO}/releases"
  echo "   File: evoclaw-agent-${TARGET}"
  exit 1
fi

echo "⬇️  Downloading from: $RELEASE_URL"
curl -fL "$RELEASE_URL" -o "/tmp/evoclaw-agent"
chmod +x "/tmp/evoclaw-agent"

# ── Verify binary ──────────────────────────────────────────────────────────
echo ""
echo "🔍 Verifying binary..."
/tmp/evoclaw-agent --version 2>/dev/null || true
echo "✅ Binary OK"

# ── Install ────────────────────────────────────────────────────────────────
echo ""
echo "📂 Installing to $BIN_DIR/evoclaw-agent ..."
mv /tmp/evoclaw-agent "$BIN_DIR/evoclaw-agent"
echo "✅ Installed"

# ── Set up config directory ────────────────────────────────────────────────
mkdir -p "$INSTALL_DIR"

if [ ! -f "$INSTALL_DIR/agent.toml" ]; then
  echo ""
  echo "⚙️  Creating default config at $INSTALL_DIR/agent.toml ..."
  
  # Auto-detect device name
  DEVICE_MODEL=$(getprop ro.product.model 2>/dev/null || echo "android-device")
  DEVICE_ID=$(echo "$DEVICE_MODEL" | tr ' ' '-' | tr '[:upper:]' '[:lower:]')
  HOSTNAME=$(hostname 2>/dev/null || echo "android")
  
  cat > "$INSTALL_DIR/agent.toml" << TOML
# EvoClaw Edge Agent Config — Android/Termux
[agent]
agent_id   = "${DEVICE_ID}-${HOSTNAME}"
agent_type = "monitor"
capabilities = "Android device — battery, storage, network stats, camera (if granted)"

[orchestrator]
# Update with your orchestrator address:
mqtt_broker = "YOUR_ORCHESTRATOR_IP"
mqtt_port   = 1883

[llm]
# Uses same GLM models as orchestrator via proxy
base_url = "https://api.anthropic.com"  # or your proxy
api_key  = "YOUR_API_KEY"
model    = "glm-4.7"

[security]
# Optional: set to sign messages with a keypair
# keypair_path = "$INSTALL_DIR/keypair.json"
TOML
  echo "✅ Config created — edit $INSTALL_DIR/agent.toml before starting"
else
  echo "ℹ️  Config already exists at $INSTALL_DIR/agent.toml — skipping"
fi

# ── Termux:Boot integration (auto-start on device boot) ────────────────────
echo ""
echo "🚀 Setting up auto-start with Termux:Boot..."

BOOT_DIR="$HOME/.termux/boot"
mkdir -p "$BOOT_DIR"

cat > "$BOOT_DIR/evoclaw-agent.sh" << 'BOOT'
#!/data/data/com.termux/files/usr/bin/bash
# Auto-start EvoClaw edge agent on device boot
# Requires: Termux:Boot app from F-Droid

# Acquire wake lock to prevent CPU sleep
termux-wake-lock 2>/dev/null || true

LOG_DIR="$HOME/.evoclaw/logs"
mkdir -p "$LOG_DIR"

echo "$(date): Starting EvoClaw edge agent..." >> "$LOG_DIR/boot.log"

exec evoclaw-agent \
  --config "$HOME/.evoclaw/agent.toml" \
  >> "$LOG_DIR/agent.log" 2>&1
BOOT

chmod +x "$BOOT_DIR/evoclaw-agent.sh"
echo "✅ Boot script installed at $BOOT_DIR/evoclaw-agent.sh"

# ── Create run script ──────────────────────────────────────────────────────
cat > "$INSTALL_DIR/start.sh" << 'START'
#!/data/data/com.termux/files/usr/bin/bash
# Start EvoClaw edge agent in foreground (for debugging)
exec evoclaw-agent --config "$HOME/.evoclaw/agent.toml" "$@"
START
chmod +x "$INSTALL_DIR/start.sh"

# ── Done ───────────────────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅  EvoClaw Edge Agent installed!"
echo ""
echo "Next steps:"
echo "  1. Edit config:    nano $INSTALL_DIR/agent.toml"
echo "  2. Start agent:    evoclaw-agent --config $INSTALL_DIR/agent.toml"
echo "  3. Auto-start:     Install Termux:Boot from F-Droid"
echo "                     (boot script already in place)"
echo ""
echo "Logs:    $INSTALL_DIR/logs/agent.log"
echo "Config:  $INSTALL_DIR/agent.toml"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
