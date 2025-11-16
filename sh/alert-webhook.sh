#!/usr/bin/env bash
set -euo pipefail

URL="${FLASH_ALERT_WEBHOOK_URL:?FLASH_ALERT_WEBHOOK_URL env var is required}"
SERVICE="${SERVICE:-api}"
EVENTS_HMAC_SECRET="${EVENTS_HMAC_SECRET:?EVENTS_HMAC_SECRET env var is required}"

ts_ms="$(date +%s%3N)"
idempotency_key="$(uuidgen 2>/dev/null || echo "${ts_ms}-$$")"
host="$(hostname)"

err="${ERROR:-unknown}"
err="${err//\\/\\\\}"
err="${err//\"/\\\"}"

subnet="${SUBNET_CIDR:-unknown}"
ip="${IP:-unknown}"
target="${TARGET:-unknown}"
ts_iso_now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ts_event="${TIMESTAMP:-$ts_iso_now}"

body=$(printf '{"severity":"HIGH","topic":"subnet-sentinel.failure","alert_key":"subnet-sentinel:%s:%s","message":"Subnet sentinel failure on %s for %s","labels":{"env":"prod","service":"subnet-sentinel","source_host":"%s","subnet_cidr":"%s","target":"%s"},"details":{"error":"%s","timestamp":"%s"},"occurred_at":"%s","idempotency_key":"%s"}' \
  "$subnet" "$ip" "$subnet" "$target" "$host" "$subnet" "$target" "$err" "$ts_event" "$ts_iso_now" "$idempotency_key")

msg="${ts_ms}.${body}"
sig=$(printf '%s' "$msg" | openssl dgst -binary -sha256 -hmac "$EVENTS_HMAC_SECRET" | base64)

curl -sS -X POST "$URL" \
  -H "content-type: application/json" \
  -H "x-service: $SERVICE" \
  -H "x-timestamp: $ts_ms" \
  -H "x-signature: $sig" \
  -d "$body" >/dev/null || true
