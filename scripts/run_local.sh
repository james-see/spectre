#!/usr/bin/env bash
# Run RuView sensing-server from a local release build (no Docker).
#
# Prereqs:
#   cd ~/esp/RuView && git submodule update --init --recursive
#   cd v2 && cargo build -p wifi-densepose-sensing-server --release
#
# Usage:
#   ./scripts/run_local.sh
#   ./scripts/run_local.sh --no-model
#
# UI (Spectre operator app):
#   cd web && npm run dev   → http://127.0.0.1:4321
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck disable=SC1091
[[ -f "$ROOT/.env" ]] && source "$ROOT/.env"

RUVIEW="${RUVIEW_DIR:-$HOME/esp/RuView}"
BIN="${SENSING_BIN:-$RUVIEW/v2/target/release/sensing-server}"
UI="${UI_PATH:-$RUVIEW/ui}"
MODELS_DIR="${MODELS_DIR:-$ROOT/models}"
DATA_DIR="${DATA_DIR:-$ROOT/data}"
MODEL_RVF="${MODEL_RVF:-$MODELS_DIR/wifi-densepose-pretrained.rvf}"

if [[ ! -x "$BIN" ]]; then
  echo "Missing $BIN — build first:" >&2
  echo "  cd $RUVIEW && git submodule update --init --recursive" >&2
  echo "  cd $RUVIEW/v2 && cargo build -p wifi-densepose-sensing-server --release" >&2
  exit 1
fi

mkdir -p "$DATA_DIR/recordings" "$DATA_DIR/models" "$MODELS_DIR"
# Server defaults recordings to ./data/recordings relative to cwd
cd "$ROOT"

export RUST_LOG="${RUST_LOG:-info}"
export MODELS_DIR
export RUVIEW_ALLOW_UNAUTHENTICATED="${RUVIEW_ALLOW_UNAUTHENTICATED:-1}"
export RUVIEW_BIND_ADDR="${RUVIEW_BIND_ADDR:-127.0.0.1}"

# Spectre triad TDM: nodes often arrive 60–100ms apart; stock 60ms hard guard
# thrash-fails fusion. Prefer a direct hard guard for Wi‑Fi / ESP-NOW sync.
export WDP_TDM_SLOTS="${WDP_TDM_SLOTS:-3}"
export WDP_TDM_SLOT_US="${WDP_TDM_SLOT_US:-20000}"
export WDP_GUARD_INTERVAL_US="${WDP_GUARD_INTERVAL_US:-200000}"
export WDP_SOFT_GUARD_US="${WDP_SOFT_GUARD_US:-80000}"

ARGS=(
  --source esp32
  --udp-port "${TARGET_PORT:-5005}"
  --http-port 3000
  --ws-port 3001
  --bind-addr "$RUVIEW_BIND_ADDR"
  --ui-path "$UI"
  --tick-ms 100
)

if [[ "${1:-}" != "--no-model" && -f "$MODEL_RVF" ]]; then
  ARGS+=(--model "$MODEL_RVF")
fi

echo "=== Spectre local sensing-server ==="
echo "  bin:    $BIN"
echo "  ui:     Spectre web → npm run dev in web/ (API :3000)"
echo "  models: $MODELS_DIR"
echo "  guard:  WDP_GUARD_INTERVAL_US=$WDP_GUARD_INTERVAL_US"
echo "  cwd:    $ROOT (recordings → data/recordings)"
echo "  args:   ${ARGS[*]}"
exec "$BIN" "${ARGS[@]}"
