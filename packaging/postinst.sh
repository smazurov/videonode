#!/bin/sh
set -e

systemd-sysusers videonode.conf
systemd-tmpfiles --create videonode.conf

if [ -d /run/systemd/system ]; then
    deb-systemd-helper enable videonode.service || true
    systemctl --system daemon-reload || true
fi

# Rockchip stack checks (advisory, non-fatal)
if command -v ffmpeg >/dev/null 2>&1; then
    if ! ffmpeg -encoders 2>/dev/null | grep -q rkmpp; then
        echo "WARNING: ffmpeg found but missing rkmpp encoders"
        echo "  Install the Rockchip ffmpeg stack via videonode-sbc-config"
    fi
else
    echo "WARNING: ffmpeg not found on PATH"
fi
if ! test -e /usr/lib/librga.so && ! test -e /usr/lib/aarch64-linux-gnu/librga.so; then
    echo "WARNING: librga not found — RGA acceleration unavailable"
fi
if ! test -e /usr/lib/librockchip_mpp.so && ! test -e /usr/lib/aarch64-linux-gnu/librockchip_mpp.so; then
    echo "WARNING: librockchip_mpp not found — hardware encoding unavailable"
fi
