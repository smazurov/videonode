#!/bin/sh
set -e

case "$1" in
    remove|upgrade|deconfigure)
        if [ -d /run/systemd/system ]; then
            deb-systemd-invoke stop videonode.service >/dev/null || true
        fi
        ;;
esac
