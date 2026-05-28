#!/bin/sh
set -e

case "$1" in
    configure|abort-upgrade|abort-deconfigure|abort-remove)
        systemd-sysusers videonode.conf
        systemd-tmpfiles --create videonode.conf

        if [ -x "/usr/bin/deb-systemd-helper" ]; then
            deb-systemd-helper unmask videonode.service >/dev/null || true
            if [ -z "$2" ]; then
                deb-systemd-helper enable videonode.service >/dev/null || true
            elif deb-systemd-helper --quiet was-enabled videonode.service; then
                deb-systemd-helper enable videonode.service >/dev/null || true
            else
                deb-systemd-helper update-state videonode.service >/dev/null || true
            fi
        fi

        if [ -d /run/systemd/system ]; then
            systemctl --system daemon-reload >/dev/null || true
            if [ -n "$2" ]; then
                deb-systemd-invoke restart videonode.service >/dev/null || true
            else
                deb-systemd-invoke start videonode.service >/dev/null || true
            fi
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
        ;;
esac
