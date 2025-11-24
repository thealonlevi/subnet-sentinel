#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="subnet-sentinel"

sudo systemctl stop "$SERVICE_NAME" 2>/dev/null || true
sudo systemctl disable "$SERVICE_NAME" 2>/dev/null || true
sudo systemctl reset-failed "$SERVICE_NAME" 2>/dev/null || true

sudo rm -f /usr/local/bin/subnet-sentinel /usr/bin/subnet-sentinel

sudo rm -f /etc/systemd/system/subnet-sentinel.service /lib/systemd/system/subnet-sentinel.service

sudo rm -rf /etc/subnet-sentinel

sudo systemctl daemon-reload

echo "subnet-sentinel has been uninstalled."
