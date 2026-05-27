#!/bin/sh
set -e

if [ "$1" = "purge" ]; then
    rm -rf /etc/videonode
fi

if [ -d /run/systemd/system ]; then
    systemctl --system daemon-reload || true
fi
