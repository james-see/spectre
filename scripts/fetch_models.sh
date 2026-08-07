#!/usr/bin/env bash
# Download RuView HF pretrained weights and convert to native .rvf for Spectre.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$ROOT/models"
HF_DIR="$DEST/hf"
RVF="$DEST/wifi-densepose-pretrained.rvf"
REPO="${HF_REPO:-ruvnet/wifi-densepose-pretrained}"

mkdir -p "$HF_DIR"
export PYENV_VERSION="${PYENV_VERSION:-3.14.6}"

python3 - <<PY
from huggingface_hub import hf_hub_download
import os
dest = "$HF_DIR"
repo = "$REPO"
for f in ("model.safetensors", "model.rvf.jsonl", "config.json", "presence-head.json"):
    p = hf_hub_download(repo_id=repo, filename=f, local_dir=dest)
    print(f"downloaded {f} -> {p} ({os.path.getsize(p)} bytes)")
PY

echo "Converting safetensors → RVF…"
docker run --rm \
  -e RUVIEW_ALLOW_UNAUTHENTICATED=1 \
  -v "$DEST:/models" \
  --entrypoint /app/sensing-server \
  ruvnet/wifi-densepose:latest \
  --convert-model /models/hf/model.safetensors \
  --convert-out /models/wifi-densepose-pretrained.rvf

ls -la "$RVF"
echo "Done. Restart: docker compose -f $ROOT/docker-compose.yml up -d"
echo "Then: curl -sS http://127.0.0.1:3000/api/v1/models | jq"
