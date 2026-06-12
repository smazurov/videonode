#!/bin/sh
set -e

case "$1" in
    configure|abort-upgrade|abort-deconfigure|abort-remove)
        systemd-sysusers videonode.conf
        systemd-tmpfiles --create videonode.conf

        # Pre-privsep packages granted the daemon the shadow group so it could
        # read /etc/shadow in-process; password checks now run in the setuid
        # videonode-session helper, so revoke the leftover grant on upgrade.
        if id -nG videonode 2>/dev/null | tr ' ' '\n' | grep -qx shadow; then
            gpasswd -d videonode shadow >/dev/null 2>&1 || true
        fi

        # Add the invoking admin (sudo apt install/upgrade) to the videonode
        # group so they can log in via the web UI. Idempotent — runs on both
        # install and upgrade, but skipped if SUDO_USER is already a member.
        if [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ] \
                && id "$SUDO_USER" >/dev/null 2>&1 \
                && ! id -nG "$SUDO_USER" 2>/dev/null | tr ' ' '\n' | grep -qx videonode; then
            if adduser --quiet "$SUDO_USER" videonode >/dev/null 2>&1; then
                echo "videonode: added $SUDO_USER to the 'videonode' group (web UI login enabled)"
            fi
        elif [ -z "$2" ] && [ -z "${SUDO_USER:-}" ]; then
            echo "videonode: to grant web UI access to a user, run: sudo adduser <username> videonode"
        fi

        if [ -x "/usr/bin/deb-systemd-helper" ]; then
            deb-systemd-helper unmask videonode.service >/dev/null || true
            if deb-systemd-helper --quiet was-enabled videonode.service; then
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
