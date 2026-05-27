#!/bin/bash
set -e

BIN_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.config/videonode"
SYSTEMD_DIR="$HOME/.config/systemd/user"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}$1${NC}"; }
warn()  { echo -e "${YELLOW}$1${NC}"; }

info "[1/4] Stopping service..."
if systemctl --user is-active videonode >/dev/null 2>&1; then
    systemctl --user stop videonode
    echo "      Stopped videonode.service"
else
    echo "      Service not running"
fi

info "[2/4] Disabling and removing service..."
if [ -f "$SYSTEMD_DIR/videonode.service" ]; then
    systemctl --user disable videonode 2>/dev/null || true
    rm -f "$SYSTEMD_DIR/videonode.service"
    rm -rf "$SYSTEMD_DIR/videonode.service.d"
    systemctl --user daemon-reload
    echo "      Removed $SYSTEMD_DIR/videonode.service"
else
    echo "      No user service found"
fi

info "[3/4] Removing binaries..."
removed=0
for bin in videonode videonode-source videonode-sink videonode-composer; do
    if [ -f "$BIN_DIR/$bin" ]; then
        rm -f "$BIN_DIR/$bin"
        echo "      Removed $BIN_DIR/$bin"
        removed=$((removed + 1))
    fi
done
[ $removed -eq 0 ] && echo "      No binaries found in $BIN_DIR"

info "[4/4] Config files..."
if [ -d "$CONFIG_DIR" ]; then
    warn "      Config directory preserved: $CONFIG_DIR"
    warn "      Remove manually if no longer needed: rm -rf $CONFIG_DIR"
else
    echo "      No config directory found"
fi

echo ""
info "Uninstall complete."
echo ""
echo "If switching to the .deb package:"
echo "  sudo apt update && sudo apt install videonode"
echo ""
echo "The .deb installs to /usr/bin and /etc/videonode with a system service."
echo "Your existing config in $CONFIG_DIR can be migrated to /etc/videonode/."
