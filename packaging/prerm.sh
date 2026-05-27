#!/bin/sh
set -e

if [ -d /run/systemd/system ]; then
    deb-systemd-invoke stop videonode.service || true
    deb-systemd-helper disable videonode.service || true
fi
