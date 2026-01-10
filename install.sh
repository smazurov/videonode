#!/bin/bash
set -e

REPO="smazurov/videonode"
BIN_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.config/videonode"
SYSTEMD_DIR="$HOME/.config/systemd/user"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}$1${NC}"
}

warn() {
    echo -e "${YELLOW}$1${NC}"
}

error() {
    echo -e "${RED}$1${NC}" >&2
}

# Step 1: Detect architecture
info "[1/5] Detecting architecture..."
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        error "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac
echo "      $ARCH"

# Step 2: Download binary
info "[2/5] Downloading videonode..."
DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/videonode_linux_${ARCH}.tar.gz"
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

if ! curl -fsSL -o "$TEMP_DIR/videonode.tar.gz" "$DOWNLOAD_URL"; then
    error "Failed to download from $DOWNLOAD_URL"
    error "Make sure a release exists with the archive: videonode_linux_${ARCH}.tar.gz"
    exit 1
fi

# Step 3: Install binary
info "[3/5] Installing to $BIN_DIR/videonode..."
mkdir -p "$BIN_DIR"
tar -xzf "$TEMP_DIR/videonode.tar.gz" -C "$TEMP_DIR"
mv "$TEMP_DIR/videonode" "$BIN_DIR/videonode"
chmod +x "$BIN_DIR/videonode"

# Check if ~/.local/bin is in PATH
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    warn "      Warning: $BIN_DIR is not in your PATH"
    warn "      Add this to your shell profile (~/.bashrc or ~/.zshrc):"
    echo "      export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
fi

# Step 4: First-run configuration
info "[4/5] Setting up configuration..."
mkdir -p "$CONFIG_DIR"

CONFIG_FILE="$CONFIG_DIR/config.toml"
STREAMS_FILE="$CONFIG_DIR/streams.toml"

if [ -f "$CONFIG_FILE" ]; then
    echo "      Config already exists, skipping (config.toml)"
else
    echo "      Downloading config.example.toml..."
    if curl -fsSL -o "$CONFIG_FILE" "https://raw.githubusercontent.com/$REPO/main/config.example.toml"; then
        echo "      Created $CONFIG_FILE"
    else
        warn "      Warning: Failed to download config template"
    fi
fi

if [ -f "$STREAMS_FILE" ]; then
    echo "      Streams already configured, skipping encoder validation (streams.toml)"
else
    echo "      Running encoder validation (this may take a minute)..."
    # Run validate-encoders from the config directory so it writes streams.toml there
    pushd "$CONFIG_DIR" > /dev/null
    if "$BIN_DIR/videonode" validate-encoders --quiet 2>/dev/null; then
        if [ -f "$STREAMS_FILE" ]; then
            # Extract working encoders from streams.toml
            H264=$(grep -A1 '\[validation.h264\]' "$STREAMS_FILE" 2>/dev/null | grep 'working' | sed "s/.*\[\(.*\)\].*/\1/" | tr -d "'" | tr ',' ' ' || echo "")
            if [ -n "$H264" ]; then
                echo "      Found encoders: $H264"
            fi
        fi
    else
        warn "      Warning: Encoder validation failed (this is OK, you can run it manually later)"
        warn "      Run: cd $CONFIG_DIR && videonode validate-encoders"
    fi
    popd > /dev/null
fi

# Step 5: Systemd service
info "[5/5] Setting up systemd service..."
if command -v systemctl &> /dev/null && systemctl --user status 2>/dev/null; then
    mkdir -p "$SYSTEMD_DIR"

    cat > "$SYSTEMD_DIR/videonode.service" << EOF
[Unit]
Description=VideoNode streaming server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$CONFIG_DIR
ExecStart=$BIN_DIR/videonode
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF

    echo "      Created $SYSTEMD_DIR/videonode.service"

    # Reload systemd and enable service
    systemctl --user daemon-reload
    systemctl --user enable videonode.service 2>/dev/null || true
    echo "      Enabled videonode.service"
else
    warn "      Systemd user services not available, skipping"
fi

echo ""
info "Installation complete!"
echo ""
echo "To start videonode now:"
echo "  systemctl --user start videonode"
echo ""
echo "To view logs:"
echo "  journalctl --user -u videonode -f"
echo ""
echo "Config files: $CONFIG_DIR/"
