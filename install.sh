#!/bin/sh
# EvoClaw Installer
# Usage: curl -fsSL https://evoclaw.win/install.sh | sh
set -e

# ── Colors ───────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

info()  { printf "${CYAN}▸${RESET} %s\n" "$1"; }
ok()    { printf "${GREEN}✓${RESET} %s\n" "$1"; }
warn()  { printf "${YELLOW}⚠${RESET} %s\n" "$1"; }
fail()  { printf "${RED}✗${RESET} %s\n" "$1" >&2; exit 1; }

# ── Banner ───────────────────────────────────────────────────────
printf "\n${BOLD}🧬 EvoClaw Installer${RESET}\n"
printf "   Self-Evolving Agent Framework\n\n"

# ── Detect OS / Arch ─────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)      fail "Unsupported OS: $OS (need linux or darwin)" ;;
esac

case "$ARCH" in
  x86_64|amd64)       ARCH="amd64" ;;
  aarch64|arm64)      ARCH="arm64" ;;
  *)                   fail "Unsupported architecture: $ARCH (need amd64 or arm64)" ;;
esac

info "Detected ${OS}/${ARCH}"

# ── Install directory ────────────────────────────────────────────
INSTALL_DIR="/usr/local/bin"
SUDO=""
if [ ! -w "$INSTALL_DIR" ]; then
  if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
    info "Will use sudo to install to ${INSTALL_DIR}"
  else
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
    warn "No root access — installing to ${INSTALL_DIR}"
    warn "Make sure ${INSTALL_DIR} is in your PATH"
  fi
fi

# ── GitHub Release Download ──────────────────────────────────────
REPO="clawinfra/evoclaw"
BINARY="evoclaw"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

download_release() {
  info "Fetching latest release from GitHub..."

  # Get latest release tag
  LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
  if command -v curl >/dev/null 2>&1; then
    RELEASE_JSON="$(curl -fsSL "$LATEST_URL" 2>/dev/null)" || return 1
  elif command -v wget >/dev/null 2>&1; then
    RELEASE_JSON="$(wget -qO- "$LATEST_URL" 2>/dev/null)" || return 1
  else
    return 1
  fi

  TAG="$(printf '%s' "$RELEASE_JSON" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"
  [ -z "$TAG" ] && return 1

  info "Latest release: ${TAG}"

  # Try common archive naming patterns
  for PATTERN in \
    "${BINARY}_${TAG#v}_${OS}_${ARCH}.tar.gz" \
    "${BINARY}-${TAG#v}-${OS}-${ARCH}.tar.gz" \
    "${BINARY}_${OS}_${ARCH}.tar.gz"; do
    URL="https://github.com/${REPO}/releases/download/${TAG}/${PATTERN}"
    info "Trying ${PATTERN}..."
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL "$URL" -o "$TMPDIR/evoclaw.tar.gz" 2>/dev/null && break
    elif command -v wget >/dev/null 2>&1; then
      wget -q "$URL" -O "$TMPDIR/evoclaw.tar.gz" 2>/dev/null && break
    fi
    rm -f "$TMPDIR/evoclaw.tar.gz"
  done

  [ ! -f "$TMPDIR/evoclaw.tar.gz" ] && return 1

  # Extract
  tar -xzf "$TMPDIR/evoclaw.tar.gz" -C "$TMPDIR" 2>/dev/null || return 1

  # Find the binary
  EXTRACTED="$(find "$TMPDIR" -name evoclaw -type f | head -1)"
  [ -z "$EXTRACTED" ] && return 1

  chmod +x "$EXTRACTED"
  $SUDO mv "$EXTRACTED" "${INSTALL_DIR}/${BINARY}"
  return 0
}

# ── Build from Source ────────────────────────────────────────────
build_from_source() {
  info "Building from source..."

  if ! command -v go >/dev/null 2>&1; then
    fail "Go is required to build from source. Install Go 1.24+ from https://go.dev/dl/"
  fi

  GO_VER="$(go version | grep -oE '[0-9]+\.[0-9]+' | head -1)"
  GO_MAJOR="$(echo "$GO_VER" | cut -d. -f1)"
  GO_MINOR="$(echo "$GO_VER" | cut -d. -f2)"
  if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 24 ]; }; then
    fail "Go 1.24+ required (found ${GO_VER}). Update at https://go.dev/dl/"
  fi

  ok "Go ${GO_VER} detected"

  if ! command -v git >/dev/null 2>&1; then
    fail "git is required to build from source"
  fi

  CLONE_DIR="$TMPDIR/evoclaw-src"
  git clone --depth 1 "https://github.com/${REPO}.git" "$CLONE_DIR" 2>/dev/null || \
    fail "Failed to clone repository"

  cd "$CLONE_DIR"
  info "Compiling (this may take a minute)..."
  CGO_ENABLED=0 go build -ldflags="-s -w" -o "$TMPDIR/evoclaw" ./cmd/evoclaw || \
    fail "Build failed"

  chmod +x "$TMPDIR/evoclaw"
  $SUDO mv "$TMPDIR/evoclaw" "${INSTALL_DIR}/${BINARY}"
}

# ── Main ─────────────────────────────────────────────────────────
if download_release; then
  ok "Installed from release binary"
else
  warn "No pre-built release found — building from source"
  build_from_source
  ok "Installed from source"
fi

ok "evoclaw installed to ${INSTALL_DIR}/${BINARY}"

# Verify
if command -v evoclaw >/dev/null 2>&1; then
  VERSION="$(evoclaw version 2>/dev/null || echo 'unknown')"
  ok "Version: ${VERSION}"
elif [ -x "${INSTALL_DIR}/${BINARY}" ]; then
  VERSION="$("${INSTALL_DIR}/${BINARY}" version 2>/dev/null || echo 'unknown')"
  ok "Version: ${VERSION}"
fi

# ── Next Steps ───────────────────────────────────────────────────
printf "\n${BOLD}🚀 Next steps:${RESET}\n"
printf "   ${CYAN}evoclaw init${RESET}        # Initialize config\n"
printf "   ${CYAN}evoclaw${RESET}             # Start the orchestrator\n"
printf "   ${CYAN}evoclaw tui${RESET}         # Terminal dashboard\n"
printf "\n   Docs: ${CYAN}https://github.com/${REPO}${RESET}\n\n"
