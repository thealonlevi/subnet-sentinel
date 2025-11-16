#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "$REPO_DIR"
make build

sudo systemctl stop subnet-sentinel 2>/dev/null || true

if [ -f "$REPO_DIR/config.yaml" ]; then
  sudo cp "$REPO_DIR/config.yaml" /etc/subnet-sentinel/config.yaml
fi

if ls "$REPO_DIR/sh"/*.sh >/dev/null 2>&1; then
  sudo cp "$REPO_DIR/sh"/*.sh /etc/subnet-sentinel/sh/
  sudo chmod +x /etc/subnet-sentinel/sh/*.sh
fi

sudo cp "$REPO_DIR/bin/subnet-sentinel" /usr/local/bin/subnet-sentinel
sudo chmod 755 /usr/local/bin/subnet-sentinel

sudo systemctl daemon-reload
sudo systemctl start subnet-sentinel
sudo systemctl status subnet-sentinel --no-pager
