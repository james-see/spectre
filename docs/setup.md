# Spectre setup notes

## Fetch firmware

Flash images are **not** baked into the flash script. Download them from
upstream RuView ESP32 releases:

```bash
./scripts/fetch_firmware.sh              # latest *-esp32 GitHub release
./scripts/fetch_firmware.sh --list       # show tags
./scripts/fetch_firmware.sh v0.8.4-esp32 # pin a tag
```

What it does:

1. Resolves the newest non-draft `*esp32*` release on `ruvnet/RuView` (or uses your tag).
2. Downloads the **8MB flash bundle** zip when available (`bootloader`, `partition-table`, `ota_data_initial`, app).
3. Normalizes the app binary to `firmware/esp32-csi-node.bin` (what `flash_node.sh` expects).
4. Optionally pulls a 4MB app binary when the release includes one.
5. Writes `firmware/SOURCE.txt` with repo, tag, and timestamp.

Requires: `curl`, `unzip`, `python3`. Override repo with `RUVIEW_REPO=owner/name` if needed.

After fetching, flash as usual:

```bash
./scripts/flash_node.sh /dev/cu.wchusbserialXXXX 1
```

## Ports on Mac

| Device | Typical path | Use |
|--------|--------------|-----|
| CH340 / external UART | `/dev/cu.wchusbserial*` | Flash + provision |
| SiLabs CP210x (DevKit UART) | `/dev/cu.SLAB_USBtoUART` | Flash + provision |
| Native USB-JTAG | `/dev/cu.usbmodem*` | Console after boot (optional) |

Charge-only cables enumerate nothing. Prefer a known data cable on the UART path.

## Dual-cable DevKitC-1

Some DevKitC-1 boards do not power from the UART port. Use:

1. **USB** port → power  
2. **UART** (or external CH340) → flash  

Flash only the UART/CH340 device.

## Triad layout

```
NODE_ID 1  tdm-slot 0
NODE_ID 2  tdm-slot 1
NODE_ID 3  tdm-slot 2
TDM_TOTAL=3
```

Spread diagonally across the room; avoid a single line.

## Credentials

Never commit `.env`. Provisioning writes SSID/password into device NVS only.

## Aggregator — local build (recommended)

Prefer a local release build of sensing-server + the Spectre operator UI (`web/`).

```bash
# one-time sensing-server
cd ~/esp/RuView && git submodule update --init --recursive
cd v2 && cargo build -p wifi-densepose-sensing-server --release

# one-time UI
cd ~/p/spectre/web && npm install

# stop Docker if it holds :5005
docker compose -f ~/p/spectre/docker-compose.yml stop

# terminal 1 — engine (UDP :5005, HTTP :3000, WS :3001)
cd ~/p/spectre && ./scripts/run_local.sh

# terminal 2 — Spectre UI (proxies API/WS)
cd ~/p/spectre/web && npm run dev
open http://127.0.0.1:4321/
```

`run_local.sh` sets `WDP_GUARD_INTERVAL_US=200000` for the 3-node TDM mesh.

Do **not** use RuView’s `index.html` Applications/Architecture tabs — that shell is demoware.
Operator routes: Status, Sensing, Pose, Record, Train.

## Aggregator on OrbStack (optional)

```bash
docker compose -f ~/p/spectre/docker-compose.yml up -d
# still prefer Spectre web/ against the container API:
cd ~/p/spectre/web && npm run dev
```

- HTTP/WS: `127.0.0.1:3000` / `:3001`
- CSI UDP: host `:5005` (must match `TARGET_IP` / `TARGET_PORT` in `.env`)
- Only one process can own UDP `:5005` — don't run Docker and local together

Stop Docker: `docker compose -f ~/p/spectre/docker-compose.yml down`  
Stop local: `pkill -f target/release/sensing-server`

## Deco LAN hotspots (Observatory)

Spectre can show TP-Link Deco mesh APs + associated Wi‑Fi clients in Observatory
(separate from ESP32 CSI markers). A small Go service polls the **master Deco**
local admin API.

1. In `.env` set:

```bash
DECO_HOST=https://192.168.68.1   # master Deco (try https:// first)
DECO_USER=admin                   # must be "admin" for local web API (not your email)
DECO_PASS=your-owner-tplink-password
DECO_BIND=127.0.0.1:3002
DECO_POLL_SECONDS=3               # Deco snapshot cadence (Observatory polls UI every 2s)
```

Local Deco web API only accepts the **owner** password with username `admin`
(TP-Link ID email as `DECO_USER` logs in but then 403s on mesh/client lists).
Deco allows one owner web session — if the Deco app is logged in as owner at
the same time, you may see intermittent 403s. Prefer a **manager** account in
the app for day-to-day phone use, and keep owner credentials for Spectre.

2. Run the poller:

```bash
./scripts/run_deco.sh
# GET http://127.0.0.1:3002/health
# GET http://127.0.0.1:3002/api/lan/mesh
# GET http://127.0.0.1:3002/api/lan/clients
```

3. With `npm run dev` in `web/`, `/api/lan/*` is proxied to `:3002`. Open
   Observatory — Deco discs + client orbs appear when the poller is healthy.

ESP32 MACs (`7c:4f:ad:…`) are filtered out of the client layer so they do not
double with CSI node spheres.

## Pretrained model (Hugging Face)

```bash
./scripts/fetch_models.sh          # once (downloads + converts to .rvf)
docker compose up -d
curl -sS -X POST http://127.0.0.1:3000/api/v1/models/load \
  -H 'Content-Type: application/json' \
  -d '{"model_id":"wifi-densepose-pretrained"}'
```

Source: [ruvnet/wifi-densepose-pretrained](https://huggingface.co/ruvnet/wifi-densepose-pretrained)  
Converted artifact: `models/wifi-densepose-pretrained.rvf` (mounted read-only).  
This is a CSI encoder / transfer checkpoint — still fine-tune on your room recordings for best results.

## Wizard (recommended): real limbs

UI **`/wizard`** walks: ESP32 nodes → MM-Fi / pretrained base → MediaPipe camera labels + CSI →
fine-tune → validate.

### MM-Fi (public pose-labeled Wi‑Fi CSI)

[MM-Fi](https://ntu-aiot-lab.github.io/mm-fi) (NeurIPS 2023) provides synced CSI + COCO-17 keypoints.
CSI layout ≠ ESP32 triad — import is a **geometry prior**, then room fine-tune adapts.

```bash
# Download from MM-Fi GitHub (Google Drive / Baidu), then:
python3 scripts/import_mmfi.py --root ~/data/MMFi --env E01 --max-sequences 16
# Smoke without download:
python3 scripts/gen_mmfi_fixture.py
```

Writes `data/recordings/mmfi_*.{csi.jsonl,pose.jsonl,meta.json}` (`has_pose`, `teacher: mmfi`).

### Room MediaPipe teacher

**Record** (or Wizard step 3) with **Vision teacher** on: CSI → `.csi.jsonl`, MediaPipe → `.pose.jsonl`
via `POST /api/v1/recording/:id/pose`. Activity labels remain session metadata for adaptive presence.

### Train (vision-supervised)

Train **requires** pose labels (`allow_heuristic_targets` defaults false). Select MM-Fi sessions to
pretrain, or room sessions to fine-tune with **Base model** = `wifi-densepose-pretrained` / MM-Fi RVF.
Output `.rvf` under `MODELS_DIR` → Load → Pose uses `model_inference`.

Legacy `data/recordings/rec_*.jsonl` sensing dumps remain readable; new captures use `.csi.jsonl`.

### Adaptive presence / motion (not DensePose)

Separate from the RVF limb head. After you have labeled recordings (at least **empty**, a still
activity like **stand/sit**, and **walk**):

1. UI **Train → Presence / motion (adaptive)** → select labeled sessions → **Train adaptive**
2. Server `POST /api/v1/adaptive/train` with optional `{ "dataset_ids": [...] }`
3. Model hot-loads into live classification (`data/adaptive_model.json`) and overrides
   `presence` + `motion_level` until **Unload adaptive**

Label map: `empty`→absent, `stand|sit|lie|pose`→present_still, `walk|gesture`→present_moving,
`multi`→active.

**Heart rate / breathing** still have no train loop — FFT + stillness gate only.

## Node health (CSI vs DEGRADED)

TDM slots do **not** gate CSI — every node should send raw CSI (`0xC5110001`).  
`0xC5110006` (60 B) is ADR-081 feature-state (liveness). If a node only sends that
with `DEGRADED` set, CSI yield is ~0 on that board.

```bash
# Fusion view (only nodes with CSI show up)
python3 scripts/check_nodes.py --ws

# Full UDP decode (OrbStack/Docker host net)
docker run --rm --net=host nicolaka/netshoot \
  tcpdump -ni eth0 udp dst port 5005 -c 200 -nn -tt -X 2>/dev/null \
  | python3 scripts/check_nodes.py --from-tcpdump -
```

If nodes 1/2 are `DEGRADED (no CSI)` while 3 is OK:

1. Plug CH340 UART on the bad node (USB for power if needed)
2. Serial monitor — look for `self-ping started`, `CSI cb`, `MGMT+DATA`
3. Reflash same firmware: `./scripts/flash_node.sh <PORT> <NODE_ID>`
4. Do not set `--filter-mac`; keep `power_duty` at 100
