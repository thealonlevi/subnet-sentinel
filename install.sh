#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

sudo systemctl stop subnet-sentinel 2>/dev/null || true

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

sudo cp "$REPO_DIR/bin/subnet-sentinel" /usr/local/bin/subnet-sentinel
sudo chmod 755 /usr/local/bin/subnet-sentinel

sudo cp "$REPO_DIR/packaging/systemd/subnet-sentinel.service" /etc/systemd/system/subnet-sentinel.service

sudo systemctl daemon-reload
sudo systemctl enable subnet-sentinel
sudo systemctl restart subnet-sentinel
sudo systemctl status subnet-sentinel --no-pager
