#!/bin/bash
# EvoClaw Installation Script
# Usage: curl -fsSL https://evoclaw.win/install.sh | sh

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Constants
REPO="clawinfra/evoclaw"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="$HOME/.evoclaw"

# Detect OS and architecture
detect_platform() {
    OS="$(uname -s)"
    ARCH="$(uname -m)"
    
    case "$OS" in
        Linux*)
            OS_TYPE="linux"
            ;;
        Darwin*)
            OS_TYPE="darwin"
            ;;
        MINGW*|MSYS*|CYGWIN*)
            OS_TYPE="windows"
            ;;
        *)
            echo -e "${RED}Unsupported OS: $OS${NC}"
            exit 1
            ;;
    esac
    
    case "$ARCH" in
        x86_64|amd64)
            ARCH_TYPE="amd64"
            ;;
        aarch64|arm64)
            ARCH_TYPE="arm64"
            ;;
        armv7l)
            ARCH_TYPE="armv7"
            ;;
        *)
            echo -e "${RED}Unsupported architecture: $ARCH${NC}"
            exit 1
            ;;
    esac
    
    BINARY_NAME="evoclaw-${OS_TYPE}-${ARCH_TYPE}"
    if [ "$OS_TYPE" = "windows" ]; then
        BINARY_NAME="${BINARY_NAME}.exe"
    fi
    
    echo -e "${GREEN}Detected platform: $OS_TYPE/$ARCH_TYPE${NC}"
}

# Get latest release version
get_latest_version() {
    echo -e "${YELLOW}Fetching latest version...${NC}"
    LATEST_VERSION=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$LATEST_VERSION" ]; then
        echo -e "${RED}Failed to fetch latest version${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}Latest version: $LATEST_VERSION${NC}"
}

# Download binary
download_binary() {
    DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_VERSION/$BINARY_NAME.tar.gz"
    TMP_DIR=$(mktemp -d)
    
    echo -e "${YELLOW}Downloading from $DOWNLOAD_URL${NC}"
    
    if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/evoclaw.tar.gz"; then
        echo -e "${RED}Download failed${NC}"
        rm -rf "$TMP_DIR"
        exit 1
    fi
    
    echo -e "${YELLOW}Extracting...${NC}"
    tar -xzf "$TMP_DIR/evoclaw.tar.gz" -C "$TMP_DIR"
    
    EXTRACTED_BINARY="$TMP_DIR/$BINARY_NAME"
    if [ ! -f "$EXTRACTED_BINARY" ]; then
        # Try without extension (for non-Windows)
        EXTRACTED_BINARY="$TMP_DIR/evoclaw-${OS_TYPE}-${ARCH_TYPE}"
    fi
    
    if [ ! -f "$EXTRACTED_BINARY" ]; then
        echo -e "${RED}Binary not found in archive${NC}"
        rm -rf "$TMP_DIR"
        exit 1
    fi
    
    echo "$TMP_DIR"
}

# Install binary
install_binary() {
    TMP_DIR=$1
    EXTRACTED_BINARY="$TMP_DIR/$BINARY_NAME"
    
    echo -e "${YELLOW}Installing to $INSTALL_DIR/evoclaw${NC}"
    
    # Check if we need sudo
    if [ -w "$INSTALL_DIR" ]; then
        cp "$EXTRACTED_BINARY" "$INSTALL_DIR/evoclaw"
        chmod +x "$INSTALL_DIR/evoclaw"
    else
        echo -e "${YELLOW}Installing requires sudo${NC}"
        sudo cp "$EXTRACTED_BINARY" "$INSTALL_DIR/evoclaw"
        sudo chmod +x "$INSTALL_DIR/evoclaw"
    fi
    
    rm -rf "$TMP_DIR"
    
    echo -e "${GREEN}✅ Binary installed${NC}"
}

# Initialize config
init_config() {
    echo -e "${YELLOW}Initializing configuration...${NC}"
    
    if [ ! -d "$CONFIG_DIR" ]; then
        mkdir -p "$CONFIG_DIR"
        echo -e "${GREEN}Created config directory: $CONFIG_DIR${NC}"
    fi
    
    # Run evoclaw init if config doesn't exist
    if [ ! -f "$CONFIG_DIR/config.json" ]; then
        echo -e "${YELLOW}Running evoclaw init...${NC}"
        evoclaw init
    else
        echo -e "${GREEN}Config already exists${NC}"
    fi
}

# Setup service (optional)
setup_service() {
    echo ""
    read -p "Do you want to set up EvoClaw as a background service? (y/N) " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${YELLOW}Setting up service...${NC}"
        
        case "$OS_TYPE" in
            linux)
                if command -v systemctl &> /dev/null; then
                    evoclaw gateway install
                    sudo systemctl enable evoclaw
                    sudo systemctl start evoclaw
                    echo -e "${GREEN}✅ Service installed and started${NC}"
                    echo -e "${GREEN}Check status: sudo systemctl status evoclaw${NC}"
                else
                    echo -e "${YELLOW}systemd not found, skipping service setup${NC}"
                fi
                ;;
            darwin)
                evoclaw gateway install
                launchctl load ~/Library/LaunchAgents/com.evoclaw.agent.plist
                echo -e "${GREEN}✅ Service installed and started${NC}"
                echo -e "${GREEN}Check status: launchctl list | grep evoclaw${NC}"
                ;;
            windows)
                echo -e "${YELLOW}Please run 'evoclaw gateway install' as Administrator${NC}"
                ;;
        esac
    else
        echo -e "${YELLOW}Skipping service setup${NC}"
        echo -e "${GREEN}You can run 'evoclaw gateway install' later${NC}"
    fi
}

# Create desktop integration
create_desktop_integration() {
    case "$OS_TYPE" in
        linux)
            # Create .desktop file
            DESKTOP_FILE="$HOME/.local/share/applications/evoclaw.desktop"
            mkdir -p "$(dirname "$DESKTOP_FILE")"
            
            cat > "$DESKTOP_FILE" <<EOF
[Desktop Entry]
Name=EvoClaw
Comment=Self-Evolving AI Agent Framework
Exec=evoclaw web
Icon=evoclaw
Terminal=false
Type=Application
Categories=Development;Utility;
EOF
            echo -e "${GREEN}✅ Desktop launcher created${NC}"
            ;;
        darwin)
            # Create app alias
            echo -e "${YELLOW}macOS: Use 'evoclaw web' to launch web interface${NC}"
            ;;
    esac
}

# Main installation flow
main() {
    echo -e "${GREEN}╔══════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║   EvoClaw Installation Script       ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════╝${NC}"
    echo ""
    
    detect_platform
    get_latest_version
    TMP_DIR=$(download_binary)
    install_binary "$TMP_DIR"
    init_config
    create_desktop_integration
    setup_service
    
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║   Installation Complete! 🚀          ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════╝${NC}"
    echo ""
    echo -e "Run ${GREEN}evoclaw --help${NC} to get started"
    echo -e "Run ${GREEN}evoclaw web${NC} to launch web interface"
    echo -e "Config: ${GREEN}$CONFIG_DIR${NC}"
    echo ""
}

main "$@"
