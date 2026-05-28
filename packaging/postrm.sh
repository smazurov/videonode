#!/bin/sh
set -e

case "$1" in
    remove)
        if [ -x "/usr/bin/deb-systemd-helper" ]; then
            deb-systemd-helper mask videonode.service >/dev/null || true
        fi
        ;;
    purge)
        if [ -x "/usr/bin/deb-systemd-helper" ]; then
            deb-systemd-helper purge videonode.service >/dev/null || true
            deb-systemd-helper unmask videonode.service >/dev/null || true
        fi
        rm -rf /etc/videonode
        ;;
esac

if [ -d /run/systemd/system ]; then
    systemctl --system daemon-reload >/dev/null || true
fi
