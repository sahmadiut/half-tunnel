#!/bin/bash
# Half-Tunnel Installer Script
# This script downloads and installs the Half-Tunnel binaries on Linux systems.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/sahmadiut/half-tunnel/main/scripts/install.sh | bash
#   
# Options:
#   HALFTUNNEL_VERSION - Specify a version to install (default: latest)
#   HALFTUNNEL_INSTALL_DIR - Installation directory (default: /usr/local/bin)
#   HALFTUNNEL_NO_SUDO - Set to 1 to skip sudo (for non-root installs)
#   HALFTUNNEL_INSTALL_SERVICE - Set to 1 to automatically install systemd services (non-interactive)

set -e

# Configuration
REPO="sahmadiut/half-tunnel"
INSTALL_DIR="${HALFTUNNEL_INSTALL_DIR:-/usr/local/bin}"
VERSION="${HALFTUNNEL_VERSION:-}"
USE_SUDO="${HALFTUNNEL_NO_SUDO:-0}"
INSTALL_SERVICE="${HALFTUNNEL_INSTALL_SERVICE:-0}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux)
            OS="linux"
            ;;
        *)
            error "Unsupported operating system: $OS. Half-Tunnel currently only supports Linux."
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    PLATFORM="${OS}-${ARCH}"
    info "Detected platform: $PLATFORM"
}

# Get the latest version from GitHub
get_latest_version() {
    if [ -n "$VERSION" ]; then
        info "Using specified version: $VERSION"
        return
    fi

    info "Fetching latest version..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$VERSION" ]; then
        error "Failed to determine latest version. Please specify HALFTUNNEL_VERSION."
    fi
    
    info "Latest version: $VERSION"
}

# Download and extract the binary
download_and_install() {
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    local filename="half-tunnel-${VERSION}-${PLATFORM}.tar.gz"
    local url="https://github.com/${REPO}/releases/download/${VERSION}/${filename}"

    info "Downloading $filename..."
    if ! curl -fsSL -o "${tmp_dir}/${filename}" "$url"; then
        error "Failed to download $url"
    fi

    info "Extracting archive..."
    tar -xzf "${tmp_dir}/${filename}" -C "$tmp_dir"

    # Verify binaries exist
    for binary in ht-client ht-server half-tunnel; do
        if [ ! -f "${tmp_dir}/${binary}" ]; then
            error "Binary ${binary} not found in archive"
        fi
    done

    # Install binaries
    info "Installing binaries to $INSTALL_DIR..."
    
    if [ "$USE_SUDO" = "0" ] && [ ! -w "$INSTALL_DIR" ]; then
        if command -v sudo >/dev/null 2>&1; then
            SUDO="sudo"
            info "Using sudo for installation"
        else
            error "Cannot write to $INSTALL_DIR and sudo is not available. Set HALFTUNNEL_INSTALL_DIR to a writable directory or run as root."
        fi
    else
        SUDO=""
    fi

    $SUDO mkdir -p "$INSTALL_DIR"
    
    for binary in ht-client ht-server half-tunnel; do
        $SUDO cp "${tmp_dir}/${binary}" "${INSTALL_DIR}/${binary}"
        $SUDO chmod +x "${INSTALL_DIR}/${binary}"
    done
}

# Verify installation
verify_installation() {
    info "Verifying installation..."
    
    local failures=0
    
    for binary in ht-client ht-server half-tunnel; do
        if [ -x "${INSTALL_DIR}/${binary}" ]; then
            version_output=$("${INSTALL_DIR}/${binary}" -version 2>&1 || true)
            success "$binary installed: $version_output"
        else
            warn "$binary not found or not executable"
            failures=$((failures + 1))
        fi
    done
    
    if [ $failures -gt 0 ]; then
        error "Installation verification failed for $failures binary(s)"
    fi
}

# Helper function to write systemd service files
# Arguments: $1 = SUDO command (empty or "sudo")
_write_systemd_services() {
    local SUDO="$1"

    # Create server service
    $SUDO tee /etc/systemd/system/half-tunnel-server.service > /dev/null << EOF
[Unit]
Description=Half-Tunnel Server
Documentation=https://github.com/${REPO}
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/ht-server -config /etc/half-tunnel/server.yml
Restart=always
RestartSec=5
User=root
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    # Create client service
    $SUDO tee /etc/systemd/system/half-tunnel-client.service > /dev/null << EOF
[Unit]
Description=Half-Tunnel Client
Documentation=https://github.com/${REPO}
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/ht-client -config /etc/half-tunnel/client.yml
Restart=always
RestartSec=5
User=root
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    # Create config directory
    $SUDO mkdir -p /etc/half-tunnel

    $SUDO systemctl daemon-reload

    success "Systemd service files created:"
    echo "  - /etc/systemd/system/half-tunnel-server.service"
    echo "  - /etc/systemd/system/half-tunnel-client.service"
    echo ""
    echo "To enable and start:"
    echo "  sudo systemctl enable half-tunnel-server"
    echo "  sudo systemctl start half-tunnel-server"
}

# Create systemd service files (interactive mode)
create_systemd_services() {
    if [ ! -d "/etc/systemd/system" ]; then
        warn "systemd not detected, skipping service file creation"
        return
    fi

    read -p "Would you like to create systemd service files? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        return
    fi

    local SUDO=""
    if [ ! -w "/etc/systemd/system" ]; then
        SUDO="sudo"
    fi

    _write_systemd_services "$SUDO"
}

# Install systemd service files (non-interactive mode)
install_systemd_services_noninteractive() {
    if [ ! -d "/etc/systemd/system" ]; then
        warn "systemd not detected, skipping service file creation"
        return
    fi

    local SUDO=""
    if [ ! -w "/etc/systemd/system" ]; then
        if command -v sudo >/dev/null 2>&1; then
            SUDO="sudo"
        else
            error "Cannot write to /etc/systemd/system and sudo is not available"
        fi
    fi

    _write_systemd_services "$SUDO"
}

# Print usage information
print_usage() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    success "Half-Tunnel $VERSION installed successfully!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Binaries installed to: $INSTALL_DIR"
    echo ""
    echo "Quick Start:"
    echo "  1. Generate a server config:"
    echo "     half-tunnel config generate --type server --output server.yml"
    echo ""
    echo "  2. Generate a client config:"
    echo "     half-tunnel config generate --type client --output client.yml"
    echo ""
    echo "  3. Start the server:"
    echo "     ht-server -config server.yml"
    echo ""
    echo "  4. Start the client:"
    echo "     ht-client -config client.yml"
    echo ""
    echo "  5. Use the SOCKS5 proxy:"
    echo "     curl --socks5 127.0.0.1:1080 https://example.com"
    echo ""
    echo "  Service Management:"
    echo "     # Install systemd service"
    echo "     sudo half-tunnel service install --type server"
    echo "     sudo half-tunnel service install --type client"
    echo ""
    echo "     # Manage services"
    echo "     sudo half-tunnel service start --type server"
    echo "     sudo half-tunnel service status --type server"
    echo "     sudo half-tunnel service logs --type server"
    echo ""
    echo "Documentation: https://github.com/${REPO}"
    echo ""
}

# Main installation flow
main() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "             Half-Tunnel Installer"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    detect_platform
    get_latest_version
    download_and_install
    verify_installation
    
    # Non-interactive service installation if HALFTUNNEL_INSTALL_SERVICE=1
    if [ "$INSTALL_SERVICE" = "1" ]; then
        info "Installing systemd services (non-interactive mode)..."
        install_systemd_services_noninteractive
    elif [ -t 0 ]; then
        # Only prompt for systemd if running interactively
        create_systemd_services
    fi
    
    print_usage
}

main "$@"
