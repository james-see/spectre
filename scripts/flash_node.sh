#!/usr/bin/env bash
# Flash + provision one Spectre CSI node.
# Usage:
#   ./scripts/flash_node.sh <PORT> <NODE_ID>
# NODE_ID: 1..TDM_TOTAL  (tdm-slot = NODE_ID-1)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck disable=SC1091
[[ -f "$ROOT/.env" ]] && source "$ROOT/.env"

PORT="${1:?port required e.g. /dev/cu.wchusbserial...}"
NODE_ID="${2:?node id required (1-based)}"
TDM_TOTAL="${TDM_TOTAL:-3}"

if [[ "$NODE_ID" -lt 1 || "$NODE_ID" -gt "$TDM_TOTAL" ]]; then
  echo "NODE_ID must be between 1 and $TDM_TOTAL"
  exit 1
fi
SLOT=$((NODE_ID - 1))

SSID="${WIFI_SSID:?Set WIFI_SSID in .env}"
PASS="${WIFI_PASS:?Set WIFI_PASS in .env}"
TARGET_IP="${TARGET_IP:?Set TARGET_IP in .env}"
TARGET_PORT="${TARGET_PORT:-5005}"
ZONE="${ZONE:-room}"
EDGE_TIER="${EDGE_TIER:-2}"
BINS="$ROOT/firmware"

export PYENV_VERSION="${PYENV_VERSION:-system}"
# Prefer ESP-IDF venv esptool when present
if [[ -d "$HOME/.espressif/python_env" ]]; then
  IDF_PY="$(ls -d "$HOME"/.espressif/python_env/idf*_py*_env/bin 2>/dev/null | sort | tail -1 || true)"
  [[ -n "${IDF_PY:-}" ]] && export PATH="$IDF_PY:$PATH"
fi

echo "=== Spectre · flash node $NODE_ID on $PORT (tdm $SLOT/$TDM_TOTAL) → $TARGET_IP:$TARGET_PORT ==="
python -m esptool --chip esp32s3 -p "$PORT" -b 460800 \
  --before default_reset --after hard_reset write_flash \
  --flash_mode dio --flash_freq 80m --flash_size detect \
  0x0     "$BINS/bootloader.bin" \
  0x8000  "$BINS/partition-table.bin" \
  0xf000  "$BINS/ota_data_initial.bin" \
  0x20000 "$BINS/esp32-csi-node.bin"

echo "=== Spectre · provision node $NODE_ID ==="
python3 "$ROOT/scripts/provision.py" \
  --port "$PORT" --chip esp32s3 --reset \
  --ssid "$SSID" --password "$PASS" \
  --target-ip "$TARGET_IP" --target-port "$TARGET_PORT" \
  --node-id "$NODE_ID" --tdm-slot "$SLOT" --tdm-total "$TDM_TOTAL" \
  --edge-tier "$EDGE_TIER" --zone "$ZONE"

echo "Node $NODE_ID ready. Unplug, place in room, plug next board."
