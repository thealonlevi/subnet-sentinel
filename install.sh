#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Clean up any previous installation
sudo systemctl stop subnet-sentinel 2>/dev/null || true

# Disable old unit if it exists (unit file might be in /etc or /lib)
if systemctl list-unit-files | grep -q '^subnet-sentinel.service'; then
  sudo systemctl disable subnet-sentinel 2>/dev/null || true
fi

# Remove old binaries from common locations
sudo rm -f /usr/local/bin/subnet-sentinel /usr/bin/subnet-sentinel 2>/dev/null || true

# Remove old unit files from common locations
sudo rm -f /etc/systemd/system/subnet-sentinel.service /lib/systemd/system/subnet-sentinel.service 2>/dev/null || true

# Remove old failure scripts but keep config/.env directory
if [ -d /etc/subnet-sentinel/sh ]; then
  sudo rm -f /etc/subnet-sentinel/sh/* 2>/dev/null || true
fi

cd "$REPO_DIR"
make build

sudo mkdir -p /usr/local/bin
sudo mkdir -p /etc/subnet-sentinel /etc/subnet-sentinel/sh

if [ ! -f /etc/subnet-sentinel/config.yaml ]; then
  sudo cp "$REPO_DIR/config.yaml" /etc/subnet-sentinel/config.yaml
fi

if ls "$REPO_DIR/sh"/*.sh >/dev/null 2>&1; then
  sudo cp "$REPO_DIR/sh"/*.sh /etc/subnet-sentinel/sh/
  sudo chmod +x /etc/subnet-sentinel/sh/*.sh
fi

if [ ! -f /etc/subnet-sentinel/.env ]; then
  sudo touch /etc/subnet-sentinel/.env
  sudo chmod 640 /etc/subnet-sentinel/.env
fi

sudo cp "$REPO_DIR/bin/subnet-sentinel" /usr/local/bin/subnet-sinel
sudo chmod 755 /usr/local/bin/subnet-sentinel

sudo cp "$REPO_DIR/packaging/systemd/subnet-sentinel.service" /etc/systemd/system/subnet-sentinel.service

sudo systemctl daemon-reload
sudo systemctl enable subnet-sentinel
sudo systemctl restart subnet-sentinel
sudo systemctl status subnet-sentinel --no-pager
