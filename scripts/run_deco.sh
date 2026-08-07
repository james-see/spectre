#!/usr/bin/env bash
# Run Spectre Deco LAN poller (Go/Gin) on DECO_BIND (default 127.0.0.1:3002).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -f "$ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/.env"
  set +a
fi

export DECO_HOST="${DECO_HOST:-https://192.168.68.1}"
export DECO_USER="${DECO_USER:-admin}"
export DECO_BIND="${DECO_BIND:-127.0.0.1:3002}"
export DECO_POLL_SECONDS="${DECO_POLL_SECONDS:-3}"

if [[ -z "${DECO_PASS:-}" ]]; then
  echo "warning: DECO_PASS is empty — set it in $ROOT/.env" >&2
fi

cd "$ROOT/services/deco"
exec go run .
