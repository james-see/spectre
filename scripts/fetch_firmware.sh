#!/usr/bin/env bash
# Download ESP32-S3 CSI node flash images from upstream RuView releases.
#
# Usage:
#   ./scripts/fetch_firmware.sh              # latest *-esp32 release
#   ./scripts/fetch_firmware.sh v0.8.4-esp32 # specific tag
#   ./scripts/fetch_firmware.sh --list       # show recent esp32 tags
#
# Writes into firmware/ (overwrites existing bins) and records the tag
# in firmware/SOURCE.txt.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/firmware"
REPO="${RUVIEW_REPO:-ruvnet/RuView}"
API="https://api.github.com/repos/${REPO}"

mkdir -p "$OUT"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing dependency: $1" >&2
    exit 1
  }
}

need curl
need python3

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

list_tags() {
  curl -fsSL "${API}/releases?per_page=40" -o "$TMP/releases.json"
  python3 - "$TMP/releases.json" <<'PY'
import json,sys
rels=json.load(open(sys.argv[1]))
for r in rels:
    tag=r.get("tag_name") or ""
    if "esp32" in tag.lower():
        print(f"{tag}\t{r.get('published_at','')}\t{r.get('name','')}")
PY
}

latest_esp32_tag() {
  curl -fsSL "${API}/releases?per_page=60" -o "$TMP/releases.json"
  python3 - "$TMP/releases.json" <<'PY'
import json,sys
rels=json.load(open(sys.argv[1]))
for r in rels:
    tag=r.get("tag_name") or ""
    if "esp32" in tag.lower() and not r.get("draft") and not r.get("prerelease"):
        print(tag)
        raise SystemExit(0)
for r in rels:
    tag=r.get("tag_name") or ""
    if "esp32" in tag.lower():
        print(tag)
        raise SystemExit(0)
print("ERROR: no *-esp32 release found", file=sys.stderr)
raise SystemExit(1)
PY
}

if [[ "${1:-}" == "--list" || "${1:-}" == "-l" ]]; then
  echo "Recent RuView ESP32 releases (${REPO}):"
  list_tags
  exit 0
fi

TAG="${1:-}"
if [[ -z "$TAG" ]]; then
  TAG="$(latest_esp32_tag)"
  echo "Using latest ESP32 release: ${TAG}"
else
  echo "Using requested tag: ${TAG}"
fi

echo "Fetching release metadata..."
curl -fsSL "${API}/releases/tags/${TAG}" >"$TMP/release.json"

python3 - "$TMP/release.json" "$TMP" <<'PY'
import json,sys,re
meta=json.load(open(sys.argv[1]))
outdir=sys.argv[2]
assets=meta.get("assets") or []
# Prefer 8MB flash bundle zip
bundle=None
app_bin=None
for a in assets:
    n=a["name"].lower()
    url=a["browser_download_url"]
    if "8mb" in n and n.endswith(".zip") and "bundle" in n:
        bundle=(a["name"], url)
    if "8mb" in n and n.endswith(".bin") and "csi-node" in n and "4mb" not in n:
        app_bin=(a["name"], url)
open(f"{outdir}/pick.txt","w").write("")
if bundle:
    open(f"{outdir}/bundle_name","w").write(bundle[0])
    open(f"{outdir}/bundle_url","w").write(bundle[1])
elif app_bin:
    open(f"{outdir}/app_name","w").write(app_bin[0])
    open(f"{outdir}/app_url","w").write(app_bin[1])
else:
    names=", ".join(a["name"] for a in assets) or "(none)"
    print(f"ERROR: no 8MB bundle/bin in release assets: {names}", file=sys.stderr)
    raise SystemExit(1)
# optional 4mb app for smaller boards
for a in assets:
    n=a["name"].lower()
    if "4mb" in n and n.endswith(".bin") and "csi-node" in n:
        open(f"{outdir}/app4_name","w").write(a["name"])
        open(f"{outdir}/app4_url","w").write(a["browser_download_url"])
        break
PY

if [[ -f "$TMP/bundle_url" ]]; then
  BNAME="$(cat "$TMP/bundle_name")"
  BURL="$(cat "$TMP/bundle_url")"
  echo "Downloading ${BNAME}..."
  curl -fL --progress-bar -o "$TMP/bundle.zip" "$BURL"
  echo "Extracting..."
  need unzip
  unzip -qo "$TMP/bundle.zip" -d "$TMP/extract"
  # Normalize names into firmware/
  cp "$TMP/extract/bootloader.bin" "$OUT/bootloader.bin"
  cp "$TMP/extract/partition-table.bin" "$OUT/partition-table.bin"
  cp "$TMP/extract/ota_data_initial.bin" "$OUT/ota_data_initial.bin"
  APP_SRC="$(ls "$TMP/extract"/esp32-csi-node*.bin | head -1)"
  cp "$APP_SRC" "$OUT/esp32-csi-node.bin"
else
  echo "No zip bundle; downloading app bin + companion files from repo tag..."
  AURL="$(cat "$TMP/app_url")"
  ANAME="$(cat "$TMP/app_name")"
  curl -fL --progress-bar -o "$OUT/esp32-csi-node.bin" "$AURL"
  # companions from same tag tree
  BASE="https://raw.githubusercontent.com/${REPO}/${TAG}/firmware/esp32-csi-node/release_bins"
  for f in bootloader.bin partition-table.bin ota_data_initial.bin; do
    echo "Downloading ${f}..."
    curl -fL --progress-bar -o "$OUT/$f" "${BASE}/${f}" || \
      curl -fL --progress-bar -o "$OUT/$f" "https://raw.githubusercontent.com/${REPO}/main/firmware/esp32-csi-node/release_bins/${f}"
  done
fi

# Optional 4MB app binary
if [[ -f "$TMP/app4_url" ]]; then
  echo "Downloading 4MB variant..."
  curl -fL --progress-bar -o "$OUT/esp32-csi-node-4mb.bin" "$(cat "$TMP/app4_url")"
fi

# Try partition-table-4mb from repo if missing
if [[ ! -f "$OUT/partition-table-4mb.bin" ]]; then
  curl -fsSL -o "$OUT/partition-table-4mb.bin" \
    "https://raw.githubusercontent.com/${REPO}/${TAG}/firmware/esp32-csi-node/release_bins/partition-table-4mb.bin" \
    2>/dev/null || true
fi

{
  echo "source_repo=${REPO}"
  echo "release_tag=${TAG}"
  echo "fetched_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "app=esp32-csi-node.bin"
  ls -la "$OUT"/*.bin | awk '{print "file="$NF" bytes="$5}'
} >"$OUT/SOURCE.txt"

# Friendly version stamp if upstream version.txt exists in extract or repo
if [[ -f "$TMP/extract/version.txt" ]]; then
  cp "$TMP/extract/version.txt" "$OUT/version.txt"
else
  curl -fsSL -o "$OUT/version.txt" \
    "https://raw.githubusercontent.com/${REPO}/${TAG}/firmware/esp32-csi-node/release_bins/version.txt" \
    2>/dev/null || echo "tag: ${TAG}" >"$OUT/version.txt"
fi

echo
echo "Firmware ready in ${OUT}/"
echo "  tag: ${TAG}"
cat "$OUT/SOURCE.txt" | sed 's/^/  /'
echo
echo "Next: ./scripts/flash_node.sh <PORT> <NODE_ID>"
